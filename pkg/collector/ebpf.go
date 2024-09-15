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

// Custom errors.
var (
	errMapNotFound = errors.New("map not found")
)

// Network enum maps.
var (
	protoMap = map[int]string{
		unix.IPPROTO_TCP: "tcp",
		unix.IPPROTO_UDP: "udp",
	}
	familyMap = map[int]string{
		unix.AF_INET:  "ipv4",
		unix.AF_INET6: "ipv6",
	}
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
	Cid   uint32
	Proto uint16
	Fam   uint16
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

// promVfsEventKey is translated bpfVfsEventKey to Prometheus labels.
type promVfsEventKey struct {
	UUID  string
	Mount string
}

// promNetEventKey is translated bpfNetEventKey to Prometheus labels.
type promNetEventKey struct {
	UUID   string
	Proto  string
	Family string
}

type ebpfOpts struct {
	vfsStatsEnabled bool
	netStatsEnabled bool
	vfsMountPoints  []string
}

type ebpfCollector struct {
	logger            log.Logger
	hostname          string
	opts              ebpfOpts
	cgroupManager     *cgroupManager
	cgroupIDUUIDCache map[uint64]string
	cgroupPathIDCache map[string]uint64
	activeCgroupIDs   []uint64
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
	netRetransPackets *prometheus.Desc
	netRetransBytes   *prometheus.Desc
}

// NewEbpfCollector returns a new instance of ebpf collector.
func NewEbpfCollector(logger log.Logger, cgManager *cgroupManager, opts ebpfOpts) (*ebpfCollector, error) {
	var netColl, vfsColl *ebpf.Collection

	var configMap *ebpf.Map

	bpfProgs := make(map[string]*ebpf.Program)

	var err error

	// Remove resource limits for kernels <5.11.
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("error removing memlock: %w", err)
	}

	// Load network programs
	if opts.netStatsEnabled {
		objFile, err := bpfNetObjs()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get current kernel version", "err", err)

			return nil, err
		}

		netColl, err = loadObject("bpf/objs/" + objFile)
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
	if opts.vfsStatsEnabled {
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
	if cgManager.mode == cgroups.Unified {
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
			if cgroupController.name == strings.TrimSpace(cgManager.activeController) {
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

				level.Debug(logger).Log("msg", "kprobe linked", "prog", name, "func", kernFuncName)
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

				level.Debug(logger).Log("msg", "kretprobe linked", "prog", name, "func", kernFuncName)
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

			level.Debug(logger).Log("msg", "fentry linked", "prog", name, "func", kernFuncName)
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

			level.Debug(logger).Log("msg", "fexit linked", "prog", name, "func", kernFuncName)
		}
	}

	return &ebpfCollector{
		logger:            logger,
		hostname:          hostname,
		cgroupManager:     cgManager,
		opts:              opts,
		cgroupIDUUIDCache: make(map[uint64]string),
		cgroupPathIDCache: make(map[string]uint64),
		netColl:           netColl,
		vfsColl:           vfsColl,
		links:             links,
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
			[]string{"manager", "hostname", "uuid", "proto", "family"},
			nil,
		),
		netIngressBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "ingress_bytes_total"),
			"Total number of ingress bytes from a cgroup",
			[]string{"manager", "hostname", "uuid", "proto", "family"},
			nil,
		),
		netEgressPackets: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "egress_packets_total"),
			"Total number of egress packets from a cgroup",
			[]string{"manager", "hostname", "uuid", "proto", "family"},
			nil,
		),
		netEgressBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "egress_bytes_total"),
			"Total number of egress bytes from a cgroup",
			[]string{"manager", "hostname", "uuid", "proto", "family"},
			nil,
		),
		netRetransPackets: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "retrans_packets_total"),
			"Total number of retransmission packets from a cgroup",
			[]string{"manager", "hostname", "uuid", "proto", "family"},
			nil,
		),
		netRetransBytes: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, ebpfCollectorSubsystem, "retrans_bytes_total"),
			"Total number of retransmission bytes from a cgroup",
			[]string{"manager", "hostname", "uuid", "proto", "family"},
			nil,
		),
	}, nil
}

// Update implements Collector and update job metrics.
func (c *ebpfCollector) Update(ch chan<- prometheus.Metric) error {
	// Fetch all active cgroups
	if err := c.discoverCgroups(); err != nil {
		return err
	}

	// Start wait group
	wg := sync.WaitGroup{}
	wg.Add(8)

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

	go func() {
		defer wg.Done()

		if err := c.updateNetRetrans(ch); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update network retransmission stats", "err", err)
		}
	}()

	// Wait for all go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *ebpfCollector) Stop(_ context.Context) error {
	level.Debug(c.logger).Log("msg", "Stopping", "collector", ebpfCollectorSubsystem)

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

// containsMount returns true if any of configured mount points is a substring to mount path
// returned by map.
// If there are no mount points configured it returns true to allow all mount points.
func (c *ebpfCollector) containsMount(mount string) bool {
	if len(c.opts.vfsMountPoints) <= 0 {
		return true
	}

	// Check if any of configured mount points is a sub string
	// of actual mount point
	for _, m := range c.opts.vfsMountPoints {
		if strings.Contains(mount, m) {
			return true
		}
	}

	return false
}

// aggVFSRWStats aggregates the VFS read/write metrics based on UUID.
func (c *ebpfCollector) aggVFSRWStats(mapName string) (map[promVfsEventKey]bpfVfsRwEvent, error) {
	var key bpfVfsEventKey

	var value bpfVfsRwEvent

	aggMetric := make(map[promVfsEventKey]bpfVfsRwEvent)

	if m, ok := c.vfsColl.Maps[mapName]; ok {
		entries := m.Iterate()
		for entries.Next(&key, &value) {
			if slices.Contains(c.activeCgroupIDs, uint64(key.Cid)) {
				mount := unix.ByteSliceToString(key.Mnt[:])
				if !c.containsMount(mount) {
					continue
				}

				promKey := promVfsEventKey{
					UUID:  c.cgroupIDUUIDCache[uint64(key.Cid)],
					Mount: mount,
				}
				if v, ok := aggMetric[promKey]; ok {
					aggMetric[promKey] = bpfVfsRwEvent{
						Calls:  v.Calls + value.Calls,
						Bytes:  v.Bytes + value.Bytes,
						Errors: v.Errors + value.Errors,
					}
				} else {
					aggMetric[promKey] = value
				}
			}
		}
	} else {
		return nil, errMapNotFound
	}

	return aggMetric, nil
}

// updateVFSWrite updates VFS write metrics.
func (c *ebpfCollector) updateVFSWrite(ch chan<- prometheus.Metric) error {
	if c.vfsColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggVFSRWStats("write_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for key, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.vfsWriteRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupManager.manager, c.hostname, key.UUID, key.Mount)
		ch <- prometheus.MustNewConstMetric(c.vfsWriteBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupManager.manager, c.hostname, key.UUID, key.Mount)
		ch <- prometheus.MustNewConstMetric(c.vfsWriteErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupManager.manager, c.hostname, key.UUID, key.Mount)
	}

	return nil
}

// updateVFSRead updates VFS read metrics.
func (c *ebpfCollector) updateVFSRead(ch chan<- prometheus.Metric) error {
	if c.vfsColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggVFSRWStats("read_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for key, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.vfsReadRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupManager.manager, c.hostname, key.UUID, key.Mount)
		ch <- prometheus.MustNewConstMetric(c.vfsReadBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupManager.manager, c.hostname, key.UUID, key.Mount)
		ch <- prometheus.MustNewConstMetric(c.vfsReadErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupManager.manager, c.hostname, key.UUID, key.Mount)
	}

	return nil
}

// aggVFSInodeStats aggregates the VFS inode metrics based on UUID.
func (c *ebpfCollector) aggVFSInodeStats(mapName string) (map[string]bpfVfsInodeEvent, error) {
	var key uint32

	var value bpfVfsInodeEvent

	aggMetric := make(map[string]bpfVfsInodeEvent)

	if m, ok := c.vfsColl.Maps[mapName]; ok {
		entries := m.Iterate()
		for entries.Next(&key, &value) {
			if slices.Contains(c.activeCgroupIDs, uint64(key)) {
				uuid := c.cgroupIDUUIDCache[uint64(key)]
				if v, ok := aggMetric[uuid]; ok {
					aggMetric[uuid] = bpfVfsInodeEvent{
						Calls:  v.Calls + value.Calls,
						Errors: v.Errors + value.Errors,
					}
				} else {
					aggMetric[uuid] = value
				}
			}
		}
	} else {
		return nil, errMapNotFound
	}

	return aggMetric, nil
}

// updateVFSOpen updates VFS open stats.
func (c *ebpfCollector) updateVFSOpen(ch chan<- prometheus.Metric) error {
	if c.vfsColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggVFSInodeStats("open_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for uuid, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.vfsOpenRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupManager.manager, c.hostname, uuid)
		ch <- prometheus.MustNewConstMetric(c.vfsOpenErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupManager.manager, c.hostname, uuid)
	}

	return nil
}

// updateVFSCreate updates VFS create stats.
func (c *ebpfCollector) updateVFSCreate(ch chan<- prometheus.Metric) error {
	if c.vfsColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggVFSInodeStats("create_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for uuid, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.vfsCreateRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupManager.manager, c.hostname, uuid)
		ch <- prometheus.MustNewConstMetric(c.vfsCreateErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupManager.manager, c.hostname, uuid)
	}

	return nil
}

// updateVFSUnlink updates VFS unlink stats.
func (c *ebpfCollector) updateVFSUnlink(ch chan<- prometheus.Metric) error {
	if c.vfsColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggVFSInodeStats("unlink_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for uuid, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.vfsUnlinkRequests, prometheus.CounterValue, float64(value.Calls), c.cgroupManager.manager, c.hostname, uuid)
		ch <- prometheus.MustNewConstMetric(c.vfsUnlinkErrors, prometheus.CounterValue, float64(value.Errors), c.cgroupManager.manager, c.hostname, uuid)
	}

	return nil
}

// aggNetStats aggregates the network metrics based on UUID.
func (c *ebpfCollector) aggNetStats(mapName string) (map[promNetEventKey]bpfNetEvent, error) {
	var key bpfNetEventKey

	var value bpfNetEvent

	aggMetric := make(map[promNetEventKey]bpfNetEvent)

	if m, ok := c.netColl.Maps[mapName]; ok {
		entries := m.Iterate()
		for entries.Next(&key, &value) {
			if slices.Contains(c.activeCgroupIDs, uint64(key.Cid)) {
				promKey := promNetEventKey{
					UUID:   c.cgroupIDUUIDCache[uint64(key.Cid)],
					Proto:  protoMap[int(key.Proto)],
					Family: familyMap[int(key.Fam)],
				}
				if v, ok := aggMetric[promKey]; ok {
					aggMetric[promKey] = bpfNetEvent{
						Packets: v.Packets + value.Packets,
						Bytes:   v.Bytes + value.Bytes,
					}
				} else {
					aggMetric[promKey] = value
				}
			}
		}
	} else {
		return nil, errMapNotFound
	}

	return aggMetric, nil
}

// updateNetIngress updates network ingress stats.
func (c *ebpfCollector) updateNetIngress(ch chan<- prometheus.Metric) error {
	if c.netColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggNetStats("ingress_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for key, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.netIngressPackets, prometheus.CounterValue, float64(value.Packets), c.cgroupManager.manager, c.hostname, key.UUID, key.Proto, key.Family)
		ch <- prometheus.MustNewConstMetric(c.netIngressBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupManager.manager, c.hostname, key.UUID, key.Proto, key.Family)
	}

	return nil
}

// updateNetEgress updates network egress stats.
func (c *ebpfCollector) updateNetEgress(ch chan<- prometheus.Metric) error {
	if c.netColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggNetStats("egress_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for key, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.netEgressPackets, prometheus.CounterValue, float64(value.Packets), c.cgroupManager.manager, c.hostname, key.UUID, key.Proto, key.Family)
		ch <- prometheus.MustNewConstMetric(c.netEgressBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupManager.manager, c.hostname, key.UUID, key.Proto, key.Family)
	}

	return nil
}

// updateNetRetrans updates network retransmission stats.
func (c *ebpfCollector) updateNetRetrans(ch chan<- prometheus.Metric) error {
	if c.netColl == nil {
		return nil
	}

	// Aggregate metrics
	aggMetric, err := c.aggNetStats("retrans_accumulator")
	if err != nil {
		return err
	}

	// Update metrics to the channel
	for key, value := range aggMetric {
		ch <- prometheus.MustNewConstMetric(c.netRetransPackets, prometheus.CounterValue, float64(value.Packets), c.cgroupManager.manager, c.hostname, key.UUID, key.Proto, key.Family)
		ch <- prometheus.MustNewConstMetric(c.netRetransBytes, prometheus.CounterValue, float64(value.Bytes), c.cgroupManager.manager, c.hostname, key.UUID, key.Proto, key.Family)
	}

	return nil
}

func (c *ebpfCollector) discoverCgroups() error {
	// Get currently active uuids and cgroup paths to evict older entries in caches
	var activeCgroupUUIDs []string

	var activeCgroupPaths []string

	// Reset activeCgroups from last scrape
	c.activeCgroupIDs = make([]uint64, 0)

	// Walk through all cgroups and get cgroup paths
	if err := filepath.WalkDir(c.cgroupManager.mountPoint, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore irrelevant cgroup paths
		if !info.IsDir() {
			return nil
		}

		// Get cgroup ID
		cgroupIDMatches := c.cgroupManager.idRegex.FindStringSubmatch(p)
		if len(cgroupIDMatches) <= 1 {
			return nil
		}

		uuid := strings.TrimSpace(cgroupIDMatches[1])
		if uuid == "" {
			level.Error(c.logger).Log("msg", "Empty UUID", "path", p)

			return nil
		}

		// Get inode of the cgroup path if not already present in the cache
		if _, ok := c.cgroupPathIDCache[p]; !ok {
			if inode, err := inode(p); err == nil {
				c.cgroupPathIDCache[p] = inode
				c.cgroupIDUUIDCache[inode] = uuid
			}
		}
		if _, ok := c.cgroupIDUUIDCache[c.cgroupPathIDCache[p]]; !ok {
			c.cgroupIDUUIDCache[c.cgroupPathIDCache[p]] = uuid
		}

		// Populate activeCgroupUUIDs, activeCgroupIDs and activeCgroupPaths
		activeCgroupPaths = append(activeCgroupPaths, p)
		activeCgroupUUIDs = append(activeCgroupUUIDs, uuid)
		c.activeCgroupIDs = append(c.activeCgroupIDs, c.cgroupPathIDCache[p])

		level.Debug(c.logger).Log("msg", "cgroup path", "path", p)

		return nil
	}); err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.cgroupManager.mountPoint, "err", err)

		return err
	}

	// Evict older entries from caches
	for cid, uuid := range c.cgroupIDUUIDCache {
		if !slices.Contains(activeCgroupUUIDs, uuid) {
			delete(c.cgroupIDUUIDCache, cid)
		}
	}

	for path := range c.cgroupPathIDCache {
		if !slices.Contains(activeCgroupPaths, path) {
			delete(c.cgroupPathIDCache, path)
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

// bpfNetObjs returns the network bpf objects based on current kernel version.
func bpfNetObjs() (string, error) {
	// Get current kernel version
	currentKernelVer, err := KernelVersion()
	if err != nil {
		return "", err
	}

	// Return appropriate bpf object file based on kernel version
	if currentKernelVer > KernelStringToNumeric("6.4") {
		return "bpf_network.o", nil
	} else if currentKernelVer >= KernelStringToNumeric("5.19") && currentKernelVer <= KernelStringToNumeric("6.4") {
		return "bpf_network_v64.o", nil
	} else {
		return "bpf_network_v519.o", nil
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
