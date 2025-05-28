//go:build !nolibvirt
// +build !nolibvirt

package collector

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/security"
	ceems_k8s "github.com/mahendrapaipuri/ceems/pkg/k8s"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/rest"
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
	libvirtCollectBlkIOStats = CEEMSExporterApp.Flag(
		"collector.libvirt.blkio-metrics",
		"Enables collection of block IO metrics (default: disabled)",
	).Default("false").Bool()
	libvirtCollectPSIStats = CEEMSExporterApp.Flag(
		"collector.libvirt.psi-metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Bool()

	// testing flags.
	libvirtXMLDir = CEEMSExporterApp.Flag(
		"collector.libvirt.xml-dir",
		"Directory containing XML files of instances",
	).Default("/etc/libvirt/qemu").Hidden().String()
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

type instanceProperties struct {
	deviceIDs []string
	uuid      string
}

// libvirtReadXMLSecurityCtxData contains the input/output data for
// reading XML files inside a security context.
type libvirtReadXMLSecurityCtxData struct {
	xmlPath    string
	instanceID string
	devices    []Device
	properties *instanceProperties
}

type libvirtCollector struct {
	logger                        *slog.Logger
	cgroupManager                 *cgroupManager
	cgroupCollector               *cgroupCollector
	perfCollector                 *perfCollector
	ebpfCollector                 *ebpfCollector
	rdmaCollector                 *rdmaCollector
	hostname                      string
	gpuSMI                        *GPUSMI
	vGPUActivated                 bool
	instanceGpuFlag               *prometheus.Desc
	instanceGpuNumSMs             *prometheus.Desc
	collectError                  *prometheus.Desc
	previousInstanceIDs           []string
	instanceDevicesCacheTTL       time.Duration
	instanceDeviceslastUpdateTime time.Time
	securityContexts              map[string]*security.SecurityContext
}

func init() {
	RegisterCollector(libvirtCollectorSubsystem, defaultDisabled, NewLibvirtCollector)
}

// NewLibvirtCollector returns a new libvirt collector exposing a summary of cgroups.
func NewLibvirtCollector(logger *slog.Logger) (Collector, error) {
	// Get libvirt's cgroup details
	cgroupManager, err := NewCgroupManager(libvirt, logger)
	if err != nil {
		logger.Info("Failed to create cgroup manager", "err", err)

		return nil, err
	}

	logger.Info("cgroup: " + cgroupManager.String())

	// Set cgroup options
	opts := cgroupOpts{
		collectSwapMemStats: *libvirtCollectSwapMemoryStats,
		collectBlockIOStats: *libvirtCollectBlkIOStats,
		collectPSIStats:     *libvirtCollectPSIStats,
	}

	// Start new instance of cgroupCollector
	cgCollector, err := NewCgroupCollector(logger.With("sub_collector", "cgroup"), cgroupManager, opts)
	if err != nil {
		logger.Info("Failed to create cgroup collector", "err", err)

		return nil, err
	}

	// Start new instance of perfCollector
	var perfCollector *perfCollector

	if perfCollectorEnabled() {
		perfCollector, err = NewPerfCollector(logger.With("sub_collector", "perf"), cgroupManager)
		if err != nil {
			logger.Info("Failed to create perf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of ebpfCollector
	var ebpfCollector *ebpfCollector

	if ebpfCollectorEnabled() {
		ebpfCollector, err = NewEbpfCollector(logger.With("sub_collector", "ebpf"), cgroupManager)
		if err != nil {
			logger.Info("Failed to create ebpf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of rdmaCollector
	var rdmaCollector *rdmaCollector

	if rdmaCollectorEnabled() {
		rdmaCollector, err = NewRDMACollector(logger.With("sub_collector", "rdma"), cgroupManager)
		if err != nil {
			logger.Info("Failed to create RDMA collector", "err", err)

			return nil, err
		}
	}

	// Create a new k8s client
	client, err := ceems_k8s.New("", "", logger)
	if err != nil && !errors.Is(err, rest.ErrNotInCluster) {
		logger.Error("Failed to create k8s client", "err", err)

		return nil, err
	}

	// Instantiate a new instance of gpuSMI struct
	gpuSMI, err := NewGPUSMI(client, logger)
	if err != nil {
		logger.Error("Error creating GPU SMI instance", "err", err)
	}

	// Attempt to get GPU devices
	if err := gpuSMI.Discover(); err != nil {
		logger.Error("Error fetching GPU devices", "err", err)
	}

	// Check if vGPU is activated on atleast one GPU
	vGPUActivated := false

	for _, gpu := range gpuSMI.Devices {
		if gpu.VGPUEnabled {
			vGPUActivated = true

			break
		}
	}

	// Setup necessary capabilities. These are the caps we need to read
	// XML files in /etc/libvirt/qemu folder that contains GPU devs used by guests.
	caps, err := setupAppCaps([]string{"cap_dac_read_search"})
	if err != nil {
		logger.Warn("Failed to parse capability name(s)", "err", err)
	}

	// Setup security context
	cfg := &security.SCConfig{
		Name:         libvirtReadXMLCtx,
		Caps:         caps,
		Func:         readLibvirtXMLFile,
		Logger:       logger,
		ExecNatively: disableCapAwareness,
	}

	securityCtx, err := security.NewSecurityContext(cfg)
	if err != nil {
		logger.Error("Failed to create a security context", "err", err)

		return nil, err
	}

	return &libvirtCollector{
		cgroupManager:                 cgroupManager,
		cgroupCollector:               cgCollector,
		perfCollector:                 perfCollector,
		ebpfCollector:                 ebpfCollector,
		rdmaCollector:                 rdmaCollector,
		hostname:                      hostname,
		gpuSMI:                        gpuSMI,
		vGPUActivated:                 vGPUActivated,
		instanceDevicesCacheTTL:       3 * time.Hour,
		instanceDeviceslastUpdateTime: time.Now(),
		securityContexts:              map[string]*security.SecurityContext{libvirtReadXMLCtx: securityCtx},
		instanceGpuFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_index_flag"),
			"A value > 0 indicates running instance using current GPU",
			[]string{
				"manager",
				"hostname",
				"cgrouphostname",
				"uuid",
				"index",
				"hindex",
				"gpuuuid",
				"gpuiid",
			},
			nil,
		),
		instanceGpuNumSMs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_sm_count"),
			"Number of SMs/CUs in the GPU instance",
			[]string{
				"manager",
				"hostname",
				"cgrouphostname",
				"uuid",
				"index",
				"hindex",
				"gpuuuid",
				"gpuiid",
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
	cgroups, err := c.instanceCgroups()
	if err != nil {
		return err
	}

	// Start a wait group
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Update cgroup metrics
		if err := c.cgroupCollector.Update(ch, cgroups); err != nil {
			c.logger.Error("Failed to update cgroup stats", "err", err)
		}

		// Update instance GPU ordinals
		if len(c.gpuSMI.Devices) > 0 {
			c.updateDeviceMappers(ch)
		}
	}()

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch, cgroups, libvirtCollectorSubsystem); err != nil {
				c.logger.Error("Failed to update IO and/or network stats", "err", err)
			}
		}()
	}

	if rdmaCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update RDMA metrics
			if err := c.rdmaCollector.Update(ch, cgroups, libvirtCollectorSubsystem); err != nil {
				c.logger.Error("Failed to update RDMA stats", "err", err)
			}
		}()
	}

	// Wait for all go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *libvirtCollector) Stop(ctx context.Context) error {
	c.logger.Debug("Stopping", "collector", libvirtCollectorSubsystem)

	// Stop all sub collectors
	// Stop cgroupCollector
	if err := c.cgroupCollector.Stop(ctx); err != nil {
		c.logger.Error("Failed to stop cgroup collector", "err", err)
	}

	// Stop perfCollector
	if perfCollectorEnabled() {
		if err := c.perfCollector.Stop(ctx); err != nil {
			c.logger.Error("Failed to stop perf collector", "err", err)
		}
	}

	// Stop ebpfCollector
	if ebpfCollectorEnabled() {
		if err := c.ebpfCollector.Stop(ctx); err != nil {
			c.logger.Error("Failed to stop ebpf collector", "err", err)
		}
	}

	// Stop rdmaCollector
	if rdmaCollectorEnabled() {
		if err := c.rdmaCollector.Stop(ctx); err != nil {
			c.logger.Error("Failed to stop RDMA collector", "err", err)
		}
	}

	return nil
}

// updateDeviceMappers updates the device mapper metrics.
func (c *libvirtCollector) updateDeviceMappers(ch chan<- prometheus.Metric) {
	for _, gpu := range c.gpuSMI.Devices {
		// Update mappers for physical GPUs
		for _, unit := range gpu.ComputeUnits {
			// If sharing is available, estimate coefficient
			value := 1.0
			if gpu.CurrentShares > 0 && unit.NumShares > 0 {
				value = float64(unit.NumShares) / float64(gpu.CurrentShares)
			}

			ch <- prometheus.MustNewConstMetric(
				c.instanceGpuFlag,
				prometheus.GaugeValue,
				value,
				c.cgroupManager.name,
				c.hostname,
				"",
				unit.UUID,
				gpu.Index,
				fmt.Sprintf("%s/gpu-%s", c.hostname, gpu.Index),
				gpu.UUID,
				"",
			)

			// Export number of SMs/CUs as well
			// Currently we are not using them for AMD GPUs, so they
			// will be zero.
			if gpu.NumSMs > 0 {
				ch <- prometheus.MustNewConstMetric(
					c.instanceGpuNumSMs,
					prometheus.GaugeValue,
					float64(gpu.NumSMs),
					c.cgroupManager.name,
					c.hostname,
					unit.Hostname,
					unit.UUID,
					gpu.Index,
					fmt.Sprintf("%s/gpu-%s", c.hostname, gpu.Index),
					gpu.UUID,
					"",
				)
			}
		}

		// Update mappers for instance GPUs
		for _, inst := range gpu.Instances {
			for _, unit := range inst.ComputeUnits {
				// If sharing is available, estimate coefficient
				value := 1.0
				if inst.CurrentShares > 0 && unit.NumShares > 0 {
					value = float64(unit.NumShares) / float64(inst.CurrentShares)
				}

				ch <- prometheus.MustNewConstMetric(
					c.instanceGpuFlag,
					prometheus.GaugeValue,
					value,
					c.cgroupManager.name,
					c.hostname,
					"",
					unit.UUID,
					inst.Index,
					fmt.Sprintf("%s/gpu-%s", c.hostname, inst.Index),
					gpu.UUID,
					strconv.FormatUint(inst.GPUInstID, 10),
				)

				// For GPU instances, export number of SMs/CUs as well
				if inst.NumSMs > 0 {
					ch <- prometheus.MustNewConstMetric(
						c.instanceGpuNumSMs,
						prometheus.GaugeValue,
						float64(inst.NumSMs),
						c.cgroupManager.name,
						c.hostname,
						"",
						unit.UUID,
						inst.Index,
						fmt.Sprintf("%s/gpu-%s", c.hostname, inst.Index),
						gpu.UUID,
						strconv.FormatUint(inst.GPUInstID, 10),
					)
				}
			}
		}
	}
}

// instanceDevices returns devices attached to instance by parsing libvirt XML file.
func (c *libvirtCollector) instanceProperties(instanceID string) *instanceProperties {
	// Read XML file in a security context that raises necessary capabilities
	dataPtr := &libvirtReadXMLSecurityCtxData{
		xmlPath:    *libvirtXMLDir,
		devices:    c.gpuSMI.Devices,
		instanceID: instanceID,
	}

	if securityCtx, ok := c.securityContexts[libvirtReadXMLCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			c.logger.Error(
				"Failed to run inside security contxt", "instance_id", instanceID, "err", err,
			)

			return nil
		}
	} else {
		c.logger.Error(
			"Security context not found", "name", libvirtReadXMLCtx, "instance_id", instanceID,
		)

		return nil
	}

	if len(dataPtr.properties.deviceIDs) > 0 {
		c.logger.Debug("GPU ordinals", "instanceID", instanceID, "ordinals", strings.Join(dataPtr.properties.deviceIDs, ","))
	}

	return dataPtr.properties
}

// updateDeviceInstances updates devices with instance UUIDs.
func (c *libvirtCollector) updateDeviceInstances(cgroups []cgroup) {
	// Get current instance UUIDs on the node
	currentInstanceIDs := make([]string, len(cgroups))
	for icgroup, cgroup := range cgroups {
		currentInstanceIDs[icgroup] = cgroup.uuid
	}

	// Check if there are any new/deleted instance between current and previous
	// scrapes.
	// It is possible from Openstack to resize instances by changing flavour. It means
	// it is possible to add GPUs to non-GPU instances, so we need to invalidate
	// instancePropsCache once in a while to ensure we capture any changes in instance
	// flavours
	if areEqual(currentInstanceIDs, c.previousInstanceIDs) && time.Since(c.instanceDeviceslastUpdateTime) < c.instanceDevicesCacheTTL {
		return
	}

	// Reset instance IDs in devices
	for igpu := range c.gpuSMI.Devices {
		c.gpuSMI.Devices[igpu].ResetUnits()
	}

	// If vGPU is activated on atleast one GPU, update mdevs
	if c.vGPUActivated {
		if err := c.gpuSMI.UpdateGPUMdevs(); err != nil {
			c.logger.Error("Failed to update GPU mdevs", "err", err)
		}
	}

	// Make a map from instance UUID to devices
	instanceDeviceMapper := make(map[string][]string)

	// Iterate over all active cgroups and get instance devices
	for icgrp, cgrp := range cgroups {
		properties := c.instanceProperties(cgrp.id)
		if properties == nil {
			continue
		}

		cgroups[icgrp].uuid = properties.uuid

		for _, id := range properties.deviceIDs {
			instanceDeviceMapper[id] = append(instanceDeviceMapper[id], properties.uuid)
		}
	}

	// Iterate over devices to find which device corresponds to this id
	for igpu, gpu := range c.gpuSMI.Devices {
		// If device is physical GPU
		if uids, ok := instanceDeviceMapper[gpu.Index]; ok {
			for handle, count := range elementCounts(uids) {
				c.gpuSMI.Devices[igpu].ComputeUnits = append(c.gpuSMI.Devices[igpu].ComputeUnits, ComputeUnit{UUID: handle.Value(), NumShares: count})
			}

			c.gpuSMI.Devices[igpu].CurrentShares += uint64(len(uids))
		}

		// If device is instance GPU
		for iinst, inst := range gpu.Instances {
			if uids, ok := instanceDeviceMapper[inst.Index]; ok {
				for handle, count := range elementCounts(uids) {
					c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits = append(c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits, ComputeUnit{UUID: handle.Value(), NumShares: count})
				}

				c.gpuSMI.Devices[igpu].Instances[iinst].CurrentShares += uint64(len(uids))
			}
		}
	}

	// Update instance IDs state variable
	c.previousInstanceIDs = currentInstanceIDs
	c.instanceDeviceslastUpdateTime = time.Now()
}

// instanceCgroups returns cgroups of active instances.
func (c *libvirtCollector) instanceCgroups() ([]cgroup, error) {
	// Get active cgroups
	cgroups, err := c.cgroupManager.discover()
	if err != nil {
		return nil, fmt.Errorf("failed to discover cgroups: %w", err)
	}

	// Update instance devices
	c.updateDeviceInstances(cgroups)

	return cgroups, nil
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

	// Initialise resources pointer
	d.properties = &instanceProperties{}

	// Loop over hostdevs to get GPU IDs
	var gpuOrdinals []string

	for _, hostDev := range domain.Devices.HostDevs {
		// PCIe pass through
		if hostDev.Type == "pci" {
			gpuBusID := fmt.Sprintf(
				"%s:%s:%s.%s",
				strings.TrimPrefix(hostDev.Source.Address.Domain, "0x"),
				strings.TrimPrefix(hostDev.Source.Address.Bus, "0x"),
				strings.TrimPrefix(hostDev.Source.Address.Slot, "0x"),
				strings.TrimPrefix(hostDev.Source.Address.Function, "0x"),
			)

			// Check if the current Bus ID matches with any existing GPUs
			for _, dev := range d.devices {
				if dev.CompareBusID(gpuBusID) {
					gpuOrdinals = append(gpuOrdinals, dev.Index)

					break
				}
			}
		} else if hostDev.Type == "mdev" {
			mdevUUID := hostDev.Source.Address.UUID

			// Check which GPU has this mdev UUID
			for _, dev := range d.devices {
				if len(dev.Instances) > 0 {
					for _, mig := range dev.Instances {
						if slices.Contains(mig.MdevUUIDs, mdevUUID) {
							gpuOrdinals = append(gpuOrdinals, mig.Index)

							goto outer_loop
						}
					}
				} else {
					if slices.Contains(dev.MdevUUIDs, mdevUUID) {
						gpuOrdinals = append(gpuOrdinals, dev.Index)

						goto outer_loop
					}
				}
			}
		}
	outer_loop:
	}

	// Read instance properties into dataPointer
	d.properties.deviceIDs = gpuOrdinals
	d.properties.uuid = domain.UUID

	return nil
}
