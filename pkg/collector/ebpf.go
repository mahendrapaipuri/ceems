//go:build !noebpf
// +build !noebpf

package collector

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/containerd/cgroups/v3"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"
)

// Embed the entire objs directory.
//
//go:embed bpf/objs/*.o
var objsFS embed.FS

const (
	ebpfCollectorSubsystem = "ebpf"
)

// CLI options.
var (
	collectNetMetrics = CEEMSExporterApp.Flag(
		"collector.ebpf.network-metrics",
		"Enables collection of network metrics by epf (default: enabled)",
	).Default("true").Bool()
	collectVFSMetrics = CEEMSExporterApp.Flag(
		"collector.ebpf.vfs-metrics",
		"Enables collection of VFS metrics by epf (default: enabled)",
	).Default("true").Bool()
)

// bpfConfig is a container for the config that is passed to bpf progs.
type bpfConfig struct {
	CgrpSubsysIdx uint64
	CgrpFsMagic   uint64
}

// bpfNetEvent is value struct for storing network events in the bpf maps.
type bpfNetEvent struct {
	Packets uint64
	Bytes   uint64
}

// bpfNetEventKey is key struct for storing network events in the bpf maps.
type bpfNetEventKey struct {
	Cid uint32
	Dev [16]uint8
}

// bpfVfsInodeEvent is value struct for storing VFS inode related
// events in the bpf maps.
type bpfVfsInodeEvent struct {
	Calls  uint64
	Errors uint64
}

// bpfVfsRwEvent is value struct for storing VFS read/write related
// events in the bpf maps.
type bpfVfsRwEvent struct {
	Bytes  uint64
	Calls  uint64
	Errors uint64
}

// bpfVfsEventKey is key struct for storing VFS events in the bpf maps.
type bpfVfsEventKey struct {
	Cid uint32
	Mnt [64]uint8
}

type ebpfCollector struct {
	logger            log.Logger
	hostname          string
	cgroupFS          cgroupFS
	inodesMap         map[uint64]string
	inodesRevMap      map[string]uint64
	activeCgroups     []uint64
	netColl           *ebpf.Collection
	vfsColl           *ebpf.Collection
	links             map[string]link.Link
	vfsWriteRequests  *prometheus.Desc
	vfsWriteBytes     *prometheus.Desc
	vfsWriteErrors    *prometheus.Desc
	vfsReadRequests   *prometheus.Desc
	vfsReadBytes      *prometheus.Desc
	vfsReadErrors     *prometheus.Desc
	vfsOpenRequests   *prometheus.Desc
	vfsOpenErrors     *prometheus.Desc
	vfsCreateRequests *prometheus.Desc
	vfsCreateErrors   *prometheus.Desc
	vfsUnlinkRequests *prometheus.Desc
	vfsUnlinkErrors   *prometheus.Desc
	netIngressPackets *prometheus.Desc
	netIngressBytes   *prometheus.Desc
	netEgressPackets  *prometheus.Desc
	netEgressBytes    *prometheus.Desc
}

func init() {
	RegisterCollector(ebpfCollectorSubsystem, defaultDisabled, NewEbpfCollector)
}

// NewEbpfCollector returns a new instance of ebpf collector.
func NewEbpfCollector(logger log.Logger) (Collector, error) {
	var netColl, vfsColl *ebpf.Collection

	var configMap *ebpf.Map

	bpfProgs := make(map[string]*ebpf.Program)

	var err error

	// Atleast one of network or VFS events must be enabled
	if !*collectNetMetrics && !*collectVFSMetrics {
		level.Error(logger).Log("msg", "Enable atleast one of --collector.ebpf.network-metrics or --collector.ebpf.vfs-metrics")

		return nil, errors.New("invalid CLI options for ebpf collector")
	}

	// Get cgroups based on the enabled collector
	var cgroupFS cgroupFS
	if *collectorState[slurmCollectorSubsystem] {
		cgroupFS = slurmCgroupFS(*cgroupfsPath, *cgroupsV1Subsystem, *forceCgroupsVersion)
	}

	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("error removing memlock: %w", err)
	}

	// Load network programs
	if *collectNetMetrics {
		netColl, err = loadObject("bpf/objs/bpf_network.o")
		if err != nil {
			level.Error(logger).Log("msg", "Unable to load network bpf objects", "err", err)

			return nil, err
		}

		for name, prog := range netColl.Programs {
			bpfProgs[name] = prog
		}

		// Set configMap
		configMap = netColl.Maps["conf_map"]
	}

	// Load VFS programs
	if *collectVFSMetrics {
		objFile, err := bpfVFSObjs()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get current kernel version", "err", err)

			return nil, err
		}

		vfsColl, err = loadObject("bpf/objs/" + objFile)
		if err != nil {
			level.Error(logger).Log("msg", "Unable to load VFS bpf objects", "err", err)

			return nil, err
		}

		for name, prog := range vfsColl.Programs {
			bpfProgs[name] = prog
		}

		// Set configMap if not already done
		if configMap == nil {
			configMap = vfsColl.Maps["conf_map"]
		}
	}

	// Update config map
	var config bpfConfig
	if cgroupFS.mode == cgroups.Unified {
		config = bpfConfig{
			CgrpSubsysIdx: uint64(0), // Irrelevant for cgroups v2
			CgrpFsMagic:   uint64(unix.CGROUP2_SUPER_MAGIC),
		}
	} else {
		var cgrpSubSysIdx uint64

		// Get all cgroup subsystems
		cgroupControllers, err := parseCgroupSubSysIds()
		if err != nil {
			level.Warn(logger).Log("msg", "Error fetching cgroup controllers", "err", err)
		}

		for _, cgroupController := range cgroupControllers {
			if cgroupController.name == strings.TrimSpace(cgroupFS.subsystem) {
				cgrpSubSysIdx = cgroupController.idx
			}
		}

		config = bpfConfig{
			CgrpSubsysIdx: cgrpSubSysIdx,
			CgrpFsMagic:   uint64(unix.CGROUP_SUPER_MAGIC),
		}
	}

	if err := configMap.Update(uint32(0), config, ebpf.UpdateAny); err != nil {
		return nil, fmt.Errorf("failed to update bpf config: %w", err)
	}

	// Instantiate ksyms to setup correct kernel names
	ksyms, err := NewKsyms()
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate ksyms: %w", err)
	}

	// Attach programs by replacing names with the ones from current kernel
	links := make(map[string]link.Link)

	for name, prog := range bpfProgs {
		// kprobe/* programs
		if strings.HasPrefix(name, "kprobe") {
			if funcName := strings.TrimPrefix(name, "kprobe_"); funcName != "" {
				kernFuncName, err := ksyms.GetArchSpecificName(funcName)
				if err != nil {
					level.Error(logger).Log("msg", "Failed to find kernel specific function name", "func", funcName, "err", err)

					continue
				}

				if links[kernFuncName], err = link.Kprobe(kernFuncName, prog, nil); err != nil {
					level.Error(logger).Log("msg", "Failed to open kprobe", "func", kernFuncName, "err", err)
				}
			}
		}

		// kretprobe/* programs
		if strings.HasPrefix(name, "kretprobe") {
			if funcName := strings.TrimPrefix(name, "kretprobe_"); funcName != "" {
				kernFuncName, err := ksyms.GetArchSpecificName(funcName)
				if err != nil {
					level.Error(logger).Log("msg", "Failed to find kernel specific function name", "func", funcName, "err", err)

					continue
				}

				if links[kernFuncName], err = link.Kretprobe(kernFuncName, prog, nil); err != nil {
					level.Error(logger).Log("msg", "Failed to open kretprobe", "func", kernFuncName, "err", err)
				}
			}
		}

		// fentry/* programs
		if strings.HasPrefix(name, "fentry") {
			kernFuncName := strings.TrimPrefix(name, "fentry_")
			if links[kernFuncName], err = link.AttachTracing(link.TracingOptions{
				Program:    prog,
				AttachType: ebpf.AttachTraceFEntry,
			}); err != nil {
				level.Error(logger).Log("msg", "Failed to open fentry", "func", kernFuncName, "err", err)
			}
		}

		// fexit/* programs
		if strings.HasPrefix(name, "fexit") {
			kernFuncName := strings.TrimPrefix(name, "fexit_")
			if links[kernFuncName], err = link.AttachTracing(link.TracingOptions{
				Program:    prog,
				AttachType: ebpf.AttachTraceFExit,
			}); err != nil {
				level.Error(logger).Log("msg", "Failed to open fexit", "func", kernFuncName, "err", err)
			}
		}
	}

	return &ebpfCollector{
		logger:       logger,
		hostname:     hostname,
		cgroupFS:     cgroupFS,
		inodesMap:    make(map[uint64]string),
		inodesRevMap: make(map[string]uint64),
		netColl:      netColl,
		vfsColl:      vfsColl,
		links:        links,
		vfsWriteBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "write_bytes_total"),
			"Total number of bytes written from a cgroup in bytes",
			[]string{"manager", "hostname", "uuid", "mountpoint"},
			nil,
		),
		vfsWriteRequests: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "write_requests_total"),
			"Total number of write requests from a cgroup",
			[]string{"manager", "hostname", "uuid", "mountpoint"},
			nil,
		),
		vfsWriteErrors: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "write_errors_total"),
			"Total number of write errors from a cgroup",
			[]string{"manager", "hostname", "uuid", "mountpoint"},
			nil,
		),
		vfsReadBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "read_bytes_total"),
			"Total number of bytes read from a cgroup in bytes",
			[]string{"manager", "hostname", "uuid", "mountpoint"},
			nil,
		),
		vfsReadRequests: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "read_requests_total"),
			"Total number of read requests from a cgroup",
			[]string{"manager", "hostname", "uuid", "mountpoint"},
			nil,
		),
		vfsReadErrors: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "read_errors_total"),
			"Total number of read errors from a cgroup",
			[]string{"manager", "hostname", "uuid", "mountpoint"},
			nil,
		),
		vfsOpenRequests: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "open_requests_total"),
			"Total number of open requests from a cgroup",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		vfsOpenErrors: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "open_errors_total"),
			"Total number of open errors from a cgroup",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		vfsCreateRequests: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "create_requests_total"),
			"Total number of create requests from a cgroup",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		vfsCreateErrors: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "open_create_total"),
			"Total number of create errors from a cgroup",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		vfsUnlinkRequests: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "unlink_requests_total"),
			"Total number of unlink requests from a cgroup",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		vfsUnlinkErrors: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "unlink_errors_total"),
			"Total number of unlink errors from a cgroup",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		netIngressPackets: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "ingress_packets_total"),
			"Total number of ingress packets from a cgroup",
			[]string{"manager", "hostname", "uuid", "dev"},
			nil,
		),
		netIngressBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "ingress_bytes_total"),
			"Total number of ingress bytes from a cgroup",
			[]string{"manager", "hostname", "uuid", "dev"},
			nil,
		),
		netEgressPackets: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "egress_packets_total"),
			"Total number of egress packets from a cgroup",
			[]string{"manager", "hostname", "uuid", "dev"},
			nil,
		),
		netEgressBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "egress_bytes_total"),
			"Total number of egress bytes from a cgroup",
			[]string{"manager", "hostname", "uuid", "dev"},
			nil,
		),
	}, nil
}

// Update implements Collector and update job metrics.
func (c *ebpfCollector) Update(ch chan<- prometheus.Metric) error {
	// Fetch all active cgroups
	if err := c.getActiveCgroups(); err != nil {
		return err
	}

	// Start wait group
	wg := sync.WaitGroup{}
	wg.Add(7)

	// Update different metrics in go routines
	go func() {
		defer wg.Done()

		if err := c.updateVFSWrite(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update VFS write stats", "err", err)
		}
	}()

	go func() {
		defer wg.Done()

		if err := c.updateVFSRead(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update VFS read stats", "err", err)
		}
	}()

	go func() {
		defer wg.Done()

		if err := c.updateVFSOpen(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update VFS open stats", "err", err)
		}
	}()

	go func() {
		defer wg.Done()

		if err := c.updateVFSCreate(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update VFS create stats", "err", err)
		}
	}()

	go func() {
		defer wg.Done()

		if err := c.updateVFSUnlink(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update VFS unlink stats", "err", err)
		}
	}()

	go func() {
		defer wg.Done()

		if err := c.updateNetEgress(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update network egress stats", "err", err)
		}
	}()

	go func() {
		defer wg.Done()

		if err := c.updateNetIngress(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update network ingress stats", "err", err)
		}
	}()

	// Wait for all go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *ebpfCollector) Stop(_ context.Context) error {
	// Close all probes
	for name, link := range c.links {
		if err := link.Close(); err != nil {
			level.Error(c.logger).Log("msg", "Failed to close link", "func", name, "err", err)
		}
	}

	// Close network collection
	if c.netColl != nil {
		c.netColl.Close()
	}

	// Close VFS collection
	if c.vfsColl != nil {
		c.vfsColl.Close()
	}

	return nil
}

// updateVFSWrite updates VFS write metrics.
func (c *ebpfCollector) updateVFSWrite(ch chan<- prometheus.Metric) error {
	var key bpfVfsEventKey

	var value bpfVfsRwEvent

	if c.vfsColl != nil {
		if m, ok := c.vfsColl.Maps["write_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key.Cid)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					mount := unix.ByteSliceToString(key.Mnt[:])
					ch <- prometheus.MustNewConstMetric(c.vfsWriteRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupFS.manager, c.hostname, uuid, mount)
					ch <- prometheus.MustNewConstMetric(c.vfsWriteBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupFS.manager, c.hostname, uuid, mount)
					ch <- prometheus.MustNewConstMetric(c.vfsWriteErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupFS.manager, c.hostname, uuid, mount)
				}
			}
		}
	}

	return nil
}

// updateVFSRead updates VFS read metrics.
func (c *ebpfCollector) updateVFSRead(ch chan<- prometheus.Metric) error {
	var key bpfVfsEventKey

	var value bpfVfsRwEvent

	if c.vfsColl != nil {
		if m, ok := c.vfsColl.Maps["read_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key.Cid)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					mount := unix.ByteSliceToString(key.Mnt[:])
					ch <- prometheus.MustNewConstMetric(c.vfsReadRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupFS.manager, c.hostname, uuid, mount)
					ch <- prometheus.MustNewConstMetric(c.vfsReadBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupFS.manager, c.hostname, uuid, mount)
					ch <- prometheus.MustNewConstMetric(c.vfsReadErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupFS.manager, c.hostname, uuid, mount)
				}
			}
		}
	}

	return nil
}

// updateVFSOpen updates VFS open stats.
func (c *ebpfCollector) updateVFSOpen(ch chan<- prometheus.Metric) error {
	var key uint32

	var value bpfVfsInodeEvent

	if c.vfsColl != nil {
		if m, ok := c.vfsColl.Maps["open_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					ch <- prometheus.MustNewConstMetric(c.vfsOpenRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupFS.manager, c.hostname, uuid)
					ch <- prometheus.MustNewConstMetric(c.vfsOpenErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupFS.manager, c.hostname, uuid)
				}
			}
		}
	}

	return nil
}

// updateVFSCreate updates VFS create stats.
func (c *ebpfCollector) updateVFSCreate(ch chan<- prometheus.Metric) error {
	var key uint32

	var value bpfVfsInodeEvent

	if c.vfsColl != nil {
		if m, ok := c.vfsColl.Maps["create_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					ch <- prometheus.MustNewConstMetric(c.vfsOpenRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupFS.manager, c.hostname, uuid)
					ch <- prometheus.MustNewConstMetric(c.vfsOpenErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupFS.manager, c.hostname, uuid)
				}
			}
		}
	}

	return nil
}

// updateVFSUnlink updates VFS unlink stats.
func (c *ebpfCollector) updateVFSUnlink(ch chan<- prometheus.Metric) error {
	var key uint32

	var value bpfVfsInodeEvent

	if c.vfsColl != nil {
		if m, ok := c.vfsColl.Maps["unlink_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					ch <- prometheus.MustNewConstMetric(c.vfsOpenRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupFS.manager, c.hostname, uuid)
					ch <- prometheus.MustNewConstMetric(c.vfsOpenErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupFS.manager, c.hostname, uuid)
				}
			}
		}
	}

	return nil
}

// updateNetIngress updates network ingress stats.
func (c *ebpfCollector) updateNetIngress(ch chan<- prometheus.Metric) error {
	var key bpfNetEventKey

	var value bpfNetEvent

	if c.netColl != nil {
		if m, ok := c.netColl.Maps["ingress_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key.Cid)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					device := unix.ByteSliceToString(key.Dev[:])
					ch <- prometheus.MustNewConstMetric(c.netIngressPackets, prometheus.CounterValue, float64(value.Packets), c.cgroupFS.manager, c.hostname, uuid, device)
					ch <- prometheus.MustNewConstMetric(c.netIngressBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupFS.manager, c.hostname, uuid, device)
				}
			}
		}
	}

	return nil
}

// updateNetEgress updates network egress stats.
func (c *ebpfCollector) updateNetEgress(ch chan<- prometheus.Metric) error {
	var key bpfNetEventKey

	var value bpfNetEvent

	if c.netColl != nil {
		if m, ok := c.netColl.Maps["egress_accumulator"]; ok {
			defer m.Close()

			for m.Iterate().Next(&key, &value) {
				cgroupID := uint64(key.Cid)
				if slices.Contains(c.activeCgroups, cgroupID) {
					uuid := c.inodesMap[cgroupID]
					device := unix.ByteSliceToString(key.Dev[:])
					ch <- prometheus.MustNewConstMetric(c.netEgressPackets, prometheus.CounterValue, float64(value.Packets), c.cgroupFS.manager, c.hostname, uuid, device)
					ch <- prometheus.MustNewConstMetric(c.netEgressBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupFS.manager, c.hostname, uuid, device)
				}
			}
		}
	}

	return nil
}

func (c *ebpfCollector) getActiveCgroups() error {
	// Get currently active jobs and set them in activeJobs state variable
	var activeUUIDs []string

	// Reset activeCgroups from last scrape
	c.activeCgroups = make([]uint64, 0)

	// Walk through all cgroups and get cgroup paths
	if err := filepath.WalkDir(c.cgroupFS.mount, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore irrelevant cgroup paths
		if !info.IsDir() || c.cgroupFS.pathFilter(p) {
			return nil
		}

		// Get cgroup ID
		cgroupIDMatches := c.cgroupFS.idRegex.FindStringSubmatch(p)
		if len(cgroupIDMatches) <= 1 {
			return nil
		}

		uuid := strings.TrimSpace(cgroupIDMatches[1])
		if uuid == "" {
			level.Error(c.logger).Log("msg", "Empty UUID", "path", p)

			return nil
		}

		// Check if we already passed through this cgroup
		if slices.Contains(activeUUIDs, uuid) {
			return nil
		}

		// Get inode of the cgroup
		if _, ok := c.inodesRevMap[uuid]; !ok {
			if inode, err := inode(p); err == nil {
				c.inodesRevMap[uuid] = inode
				c.inodesMap[inode] = p
			}
		}

		activeUUIDs = append(activeUUIDs, uuid)
		c.activeCgroups = append(c.activeCgroups, c.inodesRevMap[uuid])

		level.Debug(c.logger).Log("msg", "cgroup path", "path", p)

		return nil
	}); err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.cgroupFS.mount, "err", err)

		return err
	}

	// Remove expired uuids from inodeMap and inodeRevMap
	for uuid, inode := range c.inodesRevMap {
		if !slices.Contains(activeUUIDs, uuid) {
			delete(c.inodesRevMap, uuid)
			delete(c.inodesMap, inode)
		}
	}

	return nil
}

// bpfVFSObjs returns the VFS bpf objects based on current kernel version.
func bpfVFSObjs() (string, error) {
	// Get current kernel version
	currentKernelVer, err := KernelVersion()
	if err != nil {
		return "", err
	}

	// Return appropriate bpf object file based on kernel version
	if currentKernelVer > KernelStringToNumeric("6.2") {
		return "bpf_vfs.o", nil
	} else if currentKernelVer > KernelStringToNumeric("5.11") && currentKernelVer <= KernelStringToNumeric("6.2") {
		return "bpf_vfs_v62.o", nil
	} else {
		return "bpf_vfs_v511.o", nil
	}
}

// loadObject loads a BPF ELF file and returns a Collection.
func loadObject(path string) (*ebpf.Collection, error) {
	// Read ELF file
	file, err := objsFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read object file: %w", err)
	}

	// Make a reader and get CollectionSpec
	reader := bytes.NewReader(file)

	spec, err := ebpf.LoadCollectionSpecFromReader(reader)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			err = fmt.Errorf("%+v", ve) //nolint:errorlint
		}

		return nil, fmt.Errorf("failed to load object: %w", err)
	}

	// Instantiate a Collection from a CollectionSpec.
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate collection: %w", err)
	}

	return coll, nil
}
