//go:build !nolibvirt
// +build !nolibvirt

package collector

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	libvirtCollectorSubsystem = "libvirt"
)

// CLI opts.
var (
	// cgroup opts.
	libvirtCollectSwapMemoryStats = CEEMSExporterApp.Flag(
		"collector.libvirt.swap-memory-metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Bool()
	libvirtCollectPSIStats = CEEMSExporterApp.Flag(
		"collector.libvirt.psi-metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Bool()

	// testing flags.
	libvirtXMLDir = CEEMSExporterApp.Flag(
		"collector.libvirt.xml-dir",
		"Directory containing XML files of instances",
	).Default("").Hidden().String()
)

// Security context names.
const (
	libvirtReadXMLCtx = "libvirt_read_xml"
)

// Domain is the top level XML field for libvirt XML schema.
type Domain struct {
	Devices Devices `xml:"devices"`
	Name    string  `xml:"name"`
	UUID    string  `xml:"uuid"`
}

type Devices struct {
	HostDevs []HostDev `xml:"hostdev"`
}

type HostDev struct {
	XMLName xml.Name `xml:"hostdev"`
	Mode    string   `xml:"mode,attr"`
	Type    string   `xml:"type,attr"`
	Managed string   `xml:"managed,attr"`
	Model   string   `xml:"model,attr"`
	Display string   `xml:"display,attr"`
	Source  Source   `xml:"source"`
	Address Address  `xml:"address"`
}

type Source struct {
	XMLName xml.Name `xml:"source"`
	Address Address  `xml:"address"`
}

type Address struct {
	XMLName  xml.Name `xml:"address"`
	UUID     string   `xml:"uuid,attr"`
	Type     string   `xml:"type,attr"`
	Domain   string   `xml:"domain,attr"`
	Bus      string   `xml:"bus,attr"`
	Slot     string   `xml:"slot,attr"`
	Function string   `xml:"function,attr"`
}

// libvirtReadXMLSecurityCtxData contains the input/output data for
// reading XML files inside a security context.
type libvirtReadXMLSecurityCtxData struct {
	xmlPath       string
	instanceID    string
	devices       map[int]Device
	instanceProps instanceProps
}

// instanceProps contains VM properties.
type instanceProps struct {
	uuid        string   // This is Openstack's specific UUID
	gpuOrdinals []string // GPU ordinals bound to instance
}

type libvirtMetrics struct {
	cgMetrics     []cgMetric
	instanceProps []instanceProps
}

type libvirtCollector struct {
	logger             log.Logger
	cgroupManager      *cgroupManager
	cgroupCollector    *cgroupCollector
	perfCollector      *perfCollector
	ebpfCollector      *ebpfCollector
	rdmaCollector      *rdmaCollector
	hostname           string
	gpuDevs            map[int]Device
	instanceGpuFlag    *prometheus.Desc
	collectError       *prometheus.Desc
	instancePropsCache map[string]instanceProps
	securityContexts   map[string]*security.SecurityContext
}

func init() {
	RegisterCollector(libvirtCollectorSubsystem, defaultDisabled, NewLibvirtCollector)
}

// NewLibvirtCollector returns a new libvirt collector exposing a summary of cgroups.
func NewLibvirtCollector(logger log.Logger) (Collector, error) {
	// Get SLURM's cgroup details
	cgroupManager, err := NewCgroupManager("libvirt")
	if err != nil {
		level.Info(logger).Log("msg", "Failed to create cgroup manager", "err", err)

		return nil, err
	}

	level.Info(logger).Log("cgroup", cgroupManager)

	// Set cgroup options
	opts := cgroupOpts{
		collectSwapMemStats: *libvirtCollectSwapMemoryStats,
		collectPSIStats:     *libvirtCollectPSIStats,
	}

	// Start new instance of cgroupCollector
	cgCollector, err := NewCgroupCollector(logger, cgroupManager, opts)
	if err != nil {
		level.Info(logger).Log("msg", "Failed to create cgroup collector", "err", err)

		return nil, err
	}

	// Start new instance of perfCollector
	var perfCollector *perfCollector

	if perfCollectorEnabled() {
		perfCollector, err = NewPerfCollector(logger, cgroupManager)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create perf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of ebpfCollector
	var ebpfCollector *ebpfCollector

	if ebpfCollectorEnabled() {
		ebpfCollector, err = NewEbpfCollector(logger, cgroupManager)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create ebpf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of rdmaCollector
	var rdmaCollector *rdmaCollector

	if rdmaCollectorEnabled() {
		rdmaCollector, err = NewRDMACollector(logger, cgroupManager)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create RDMA collector", "err", err)

			return nil, err
		}
	}

	// Attempt to get GPU devices
	var gpuTypes []string

	var gpuDevs map[int]Device

	if *gpuType != "" {
		gpuTypes = []string{*gpuType}
	} else {
		gpuTypes = []string{"nvidia", "amd"}
	}

	for _, gpuType := range gpuTypes {
		gpuDevs, err = GetGPUDevices(gpuType, logger)
		if err == nil {
			level.Info(logger).Log("gpu", gpuType)

			break
		}
	}

	// Setup necessary capabilities. These are the caps we need to read
	// XML files in /etc/libvirt/qemu folder that contains GPU devs used by guests.
	caps := setupCollectorCaps(logger, libvirtCollectorSubsystem, []string{"cap_dac_read_search"})

	// Setup new security context(s)
	securityCtx, err := security.NewSecurityContext(libvirtReadXMLCtx, caps, readLibvirtXMLFile, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create a security context", "err", err)

		return nil, err
	}

	return &libvirtCollector{
		cgroupManager:      cgroupManager,
		cgroupCollector:    cgCollector,
		perfCollector:      perfCollector,
		ebpfCollector:      ebpfCollector,
		rdmaCollector:      rdmaCollector,
		hostname:           hostname,
		gpuDevs:            gpuDevs,
		instancePropsCache: make(map[string]instanceProps),
		securityContexts:   map[string]*security.SecurityContext{libvirtReadXMLCtx: securityCtx},
		instanceGpuFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_index_flag"),
			"Indicates running instance on GPU, 1=instance running",
			[]string{
				"manager",
				"hostname",
				"uuid",
				"index",
				"hindex",
				"gpuuuid",
			},
			nil,
		),
		collectError: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "collect_error"),
			"Indicates collection error, 0=no error, 1=error",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		logger: logger,
	}, nil
}

// Update implements Collector and update instance metrics.
func (c *libvirtCollector) Update(ch chan<- prometheus.Metric) error {
	// Discover all active cgroups
	metrics, err := c.discoverCgroups()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoData, err)
	}

	// Start a wait group
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Update cgroup metrics
		if err := c.cgroupCollector.Update(ch, metrics.cgMetrics); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update cgroup stats", "err", err)
		}

		// Update instance GPU ordinals
		if len(c.gpuDevs) > 0 {
			c.updateGPUOrdinals(ch, metrics.instanceProps)
		}
	}()

	if perfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update perf metrics
			if err := c.perfCollector.Update(ch); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update perf stats", "err", err)
			}
		}()
	}

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update IO and/or network stats", "err", err)
			}
		}()
	}

	if rdmaCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update RDMA metrics
			if err := c.rdmaCollector.Update(ch); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update RDMA stats", "err", err)
			}
		}()
	}

	// Wait for all go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *libvirtCollector) Stop(ctx context.Context) error {
	level.Debug(c.logger).Log("msg", "Stopping", "collector", libvirtCollectorSubsystem)

	// Stop all sub collectors
	// Stop cgroupCollector
	if err := c.cgroupCollector.Stop(ctx); err != nil {
		level.Error(c.logger).Log("msg", "Failed to stop cgroup collector", "err", err)
	}

	// Stop perfCollector
	if perfCollectorEnabled() {
		if err := c.perfCollector.Stop(ctx); err != nil {
			level.Error(c.logger).Log("msg", "Failed to stop perf collector", "err", err)
		}
	}

	// Stop ebpfCollector
	if ebpfCollectorEnabled() {
		if err := c.ebpfCollector.Stop(ctx); err != nil {
			level.Error(c.logger).Log("msg", "Failed to stop ebpf collector", "err", err)
		}
	}

	// Stop rdmaCollector
	if rdmaCollectorEnabled() {
		if err := c.rdmaCollector.Stop(ctx); err != nil {
			level.Error(c.logger).Log("msg", "Failed to stop RDMA collector", "err", err)
		}
	}

	return nil
}

// updateGPUOrdinals updates the metrics channel with GPU ordinals for instance.
func (c *libvirtCollector) updateGPUOrdinals(ch chan<- prometheus.Metric, instanceProps []instanceProps) {
	// Update instance properties
	for _, p := range instanceProps {
		// GPU instance mapping
		for _, gpuOrdinal := range p.gpuOrdinals {
			var gpuuuid string
			// Check the int index of devices where gpuOrdinal == dev.index
			for _, dev := range c.gpuDevs {
				if gpuOrdinal == dev.index {
					gpuuuid = dev.uuid

					break
				}
			}
			ch <- prometheus.MustNewConstMetric(c.instanceGpuFlag, prometheus.GaugeValue, float64(1), c.cgroupManager.manager, c.hostname, p.uuid, gpuOrdinal, fmt.Sprintf("%s-gpu-%s", c.hostname, gpuOrdinal), gpuuuid)
		}
	}
}

// discoverCgroups finds active cgroup paths and returns initialised metric structs.
func (c *libvirtCollector) discoverCgroups() (libvirtMetrics, error) {
	// Get currently active instances and set them in activeInstanceIDs state variable
	var activeInstanceIDs []string

	var instnProps []instanceProps

	var cgMetrics []cgMetric

	// Walk through all cgroups and get cgroup paths
	// https://goplay.tools/snippet/coVDkIozuhg
	if err := filepath.WalkDir(c.cgroupManager.mountPoint, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore inner cgroups of instances
		if !info.IsDir() || c.cgroupManager.pathFilter(p) {
			return nil
		}

		// Get relative path of cgroup
		rel, err := filepath.Rel(c.cgroupManager.root, p)
		if err != nil {
			level.Error(c.logger).Log("msg", "Failed to resolve relative path for cgroup", "path", p, "err", err)

			return nil
		}

		// Unescape UTF-8 characters in cgroup path
		sanitizedPath, err := strconv.Unquote("\"" + p + "\"")
		if err != nil {
			level.Error(c.logger).Log("msg", "Failed to unquote cgroup path", "path", p, "err", err)

			return nil
		}

		// Get cgroup ID which is instance ID
		cgroupIDMatches := c.cgroupManager.idRegex.FindStringSubmatch(sanitizedPath)
		if len(cgroupIDMatches) <= 1 {
			return nil
		}

		instanceID := strings.TrimSpace(cgroupIDMatches[1])
		if instanceID == "" {
			level.Error(c.logger).Log("msg", "Empty instance ID", "path", p)

			return nil
		}

		// Check if we already passed through this instance
		if slices.Contains(activeInstanceIDs, instanceID) {
			return nil
		}

		// Get instance details
		var instanceUUID string
		if iProps, ok := c.instancePropsCache[instanceID]; !ok {
			c.instancePropsCache[instanceID] = c.instanceProperties(instanceID)
			instnProps = append(instnProps, c.instancePropsCache[instanceID])
			instanceUUID = c.instancePropsCache[instanceID].uuid
		} else {
			instnProps = append(instnProps, iProps)
			instanceUUID = iProps.uuid
		}

		activeInstanceIDs = append(activeInstanceIDs, instanceID)
		cgMetrics = append(cgMetrics, cgMetric{uuid: instanceUUID, path: "/" + rel})

		level.Debug(c.logger).Log("msg", "cgroup path", "path", p)

		return nil
	}); err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.cgroupManager.mountPoint, "err", err)

		return libvirtMetrics{}, err
	}

	// Remove terminated instances from instancePropsCache
	for uuid := range c.instancePropsCache {
		if !slices.Contains(activeInstanceIDs, uuid) {
			delete(c.instancePropsCache, uuid)
		}
	}

	return libvirtMetrics{cgMetrics: cgMetrics, instanceProps: instnProps}, nil
}

// instanceProperties returns instance properties parsed from XML file.
func (c *libvirtCollector) instanceProperties(instanceID string) instanceProps {
	// Read XML file in a security context that raises necessary capabilities
	dataPtr := &libvirtReadXMLSecurityCtxData{
		xmlPath:    *libvirtXMLDir,
		devices:    c.gpuDevs,
		instanceID: instanceID,
	}

	if securityCtx, ok := c.securityContexts[libvirtReadXMLCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			level.Error(c.logger).Log(
				"msg", "Failed to run inside security contxt", "instance_id", instanceID, "err", err,
			)

			return instanceProps{}
		}
	} else {
		level.Error(c.logger).Log(
			"msg", "Security context not found", "name", libvirtReadXMLCtx, "instance_id", instanceID,
		)

		return instanceProps{}
	}

	return dataPtr.instanceProps
}

// readLibvirtXMLFile reads the libvirt's XML file inside a security context.
func readLibvirtXMLFile(data interface{}) error {
	// Assert data
	var d *libvirtReadXMLSecurityCtxData

	var ok bool
	if d, ok = data.(*libvirtReadXMLSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	// Get full file path
	xmlFilePath := filepath.Join(d.xmlPath, d.instanceID+".xml")

	// If file does not exist return error
	if _, err := os.Stat(xmlFilePath); err != nil {
		return err
	}

	// Read XML file contents
	xmlByteArray, err := os.ReadFile(xmlFilePath)
	if err != nil {
		return err
	}

	// Read XML byte array into domain
	var domain Domain
	if err := xml.Unmarshal(xmlByteArray, &domain); err != nil {
		return err
	}

	// Loop over hostdevs to get GPU IDs
	var gpuOrdinals []string

	for _, hostDev := range domain.Devices.HostDevs {
		// PCIe pass through
		if hostDev.Type == "pci" {
			gpuBusID := fmt.Sprintf(
				"%s:%s:%s.%s",
				strings.TrimPrefix(hostDev.Address.Domain, "0x"),
				strings.TrimPrefix(hostDev.Address.Bus, "0x"),
				strings.TrimPrefix(hostDev.Address.Slot, "0x"),
				strings.TrimPrefix(hostDev.Address.Function, "0x"),
			)

			// Check if the current Bus ID matches with any existing GPUs
			for idx, dev := range d.devices {
				if dev.CompareBusID(gpuBusID) {
					gpuOrdinals = append(gpuOrdinals, strconv.FormatInt(int64(idx), 10))

					break
				}
			}
		}
	}

	// Read instance properties into dataPointer
	d.instanceProps = instanceProps{uuid: domain.UUID, gpuOrdinals: gpuOrdinals}

	return nil
}
