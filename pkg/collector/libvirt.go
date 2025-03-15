//go:build !nolibvirt
// +build !nolibvirt

package collector

import (
	"context"
	"encoding/xml"
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

// libvirtReadXMLSecurityCtxData contains the input/output data for
// reading XML files inside a security context.
type libvirtReadXMLSecurityCtxData struct {
	xmlPath       string
	instanceID    string
	devices       []Device
	instanceProps instanceProps
}

// instanceProps contains VM properties.
type instanceProps struct {
	uuid        string   // This is Openstack's specific UUID
	gpuOrdinals []string // GPU ordinals bound to instance
}

type libvirtMetrics struct {
	instanceProps []instanceProps
	cgroups       []cgroup
}

type libvirtCollector struct {
	logger                      *slog.Logger
	cgroupManager               *cgroupManager
	cgroupCollector             *cgroupCollector
	perfCollector               *perfCollector
	ebpfCollector               *ebpfCollector
	rdmaCollector               *rdmaCollector
	hostname                    string
	gpuDevs                     []Device
	vGPUActivated               bool
	instanceGpuFlag             *prometheus.Desc
	collectError                *prometheus.Desc
	instancePropsCache          map[string]instanceProps
	instancePropsCacheTTL       time.Duration
	instancePropslastUpdateTime time.Time
	securityContexts            map[string]*security.SecurityContext
}

func init() {
	RegisterCollector(libvirtCollectorSubsystem, defaultDisabled, NewLibvirtCollector)
}

// NewLibvirtCollector returns a new libvirt collector exposing a summary of cgroups.
func NewLibvirtCollector(logger *slog.Logger) (Collector, error) {
	// Get libvirt's cgroup details
	cgroupManager, err := NewCgroupManager("libvirt", logger)
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

	// Attempt to get GPU devices
	var gpuTypes []string

	var gpuDevs []Device

	if *gpuType != "" {
		gpuTypes = []string{*gpuType}
	} else {
		gpuTypes = []string{"nvidia", "amd"}
	}

	for _, gpuType := range gpuTypes {
		gpuDevs, err = GetGPUDevices(gpuType, logger)
		if err == nil {
			logger.Info("GPU devices found", "type", gpuType, "num_devs", len(gpuDevs))

			break
		}
	}

	// Check if vGPU is activated on atleast one GPU
	vGPUActivated := false

	for _, gpu := range gpuDevs {
		if gpu.vgpuEnabled {
			vGPUActivated = true

			break
		}
	}

	// Setup necessary capabilities. These are the caps we need to read
	// XML files in /etc/libvirt/qemu folder that contains GPU devs used by guests.
	caps := setupCollectorCaps(logger, libvirtCollectorSubsystem, []string{"cap_dac_read_search"})

	// Setup new security context(s)
	securityCtx, err := security.NewSecurityContext(libvirtReadXMLCtx, caps, readLibvirtXMLFile, logger)
	if err != nil {
		logger.Error("Failed to create a security context", "err", err)

		return nil, err
	}

	return &libvirtCollector{
		cgroupManager:               cgroupManager,
		cgroupCollector:             cgCollector,
		perfCollector:               perfCollector,
		ebpfCollector:               ebpfCollector,
		rdmaCollector:               rdmaCollector,
		hostname:                    hostname,
		gpuDevs:                     gpuDevs,
		vGPUActivated:               vGPUActivated,
		instancePropsCache:          make(map[string]instanceProps),
		instancePropsCacheTTL:       3 * time.Hour,
		instancePropslastUpdateTime: time.Now(),
		securityContexts:            map[string]*security.SecurityContext{libvirtReadXMLCtx: securityCtx},
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
	metrics, err := c.instanceMetrics()
	if err != nil {
		return err
	}

	// Start a wait group
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Update cgroup metrics
		if err := c.cgroupCollector.Update(ch, metrics.cgroups); err != nil {
			c.logger.Error("Failed to update cgroup stats", "err", err)
		}

		// Update instance GPU ordinals
		if len(c.gpuDevs) > 0 {
			c.updateGPUOrdinals(ch, metrics.instanceProps)
		}
	}()

	// if perfCollectorEnabled() {
	// 	wg.Add(1)

	// 	go func() {
	// 		defer wg.Done()

	// 		// Update perf metrics
	// 		if err := c.perfCollector.Update(ch, metrics.cgroups); err != nil {
	// 			c.logger.Error("Failed to update perf stats", "err", err)
	// 		}
	// 	}()
	// }

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch, metrics.cgroups, libvirtCollectorSubsystem); err != nil {
				c.logger.Error("Failed to update IO and/or network stats", "err", err)
			}
		}()
	}

	if rdmaCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update RDMA metrics
			if err := c.rdmaCollector.Update(ch, metrics.cgroups, libvirtCollectorSubsystem); err != nil {
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

// updateGPUOrdinals updates the metrics channel with GPU ordinals for instance.
func (c *libvirtCollector) updateGPUOrdinals(ch chan<- prometheus.Metric, instanceProps []instanceProps) {
	// Update instance properties
	for _, p := range instanceProps {
		// GPU instance mapping
		for _, gpuOrdinal := range p.gpuOrdinals {
			var gpuuuid, miggid string

			flagValue := float64(1)
			// Check the int index of devices where gpuOrdinal == dev.index
			for _, dev := range c.gpuDevs {
				// If the device has MIG enabled loop over them as well
				for _, mig := range dev.migInstances {
					if gpuOrdinal == mig.globalIndex {
						gpuuuid = dev.uuid
						miggid = strconv.FormatUint(mig.gpuInstID, 10)

						// For MIG, we export SM fraction as flag value
						// For vGPU enabled GPUs this fraction must be
						// further divided by number of active vGPU instances
						if dev.vgpuEnabled && len(mig.mdevUUIDs) > 1 {
							flagValue = mig.smFraction / float64(len(mig.mdevUUIDs))
						} else {
							flagValue = mig.smFraction
						}

						goto update_chan
					}
				}

				if gpuOrdinal == dev.globalIndex {
					gpuuuid = dev.uuid

					if dev.vgpuEnabled && len(dev.mdevUUIDs) > 1 {
						flagValue = 1.0 / float64(len(dev.mdevUUIDs))
					}

					goto update_chan
				}
			}

		update_chan:
			// On the DCGM side, we need to use relabel magic to rename UUID
			// and GPU_I_ID labels to gpuuuid and gpuiid and make operations
			// on(gpuuuid,gpuiid)
			ch <- prometheus.MustNewConstMetric(
				c.instanceGpuFlag,
				prometheus.GaugeValue,
				flagValue,
				c.cgroupManager.manager,
				c.hostname,
				"", // This empty label will be dropped by Prom anyways. Just for consistency!
				p.uuid,
				gpuOrdinal,
				fmt.Sprintf("%s/gpu-%s", c.hostname, gpuOrdinal),
				gpuuuid,
				miggid,
			)
		}
	}
}

// instanceProperties finds properties for each cgroup and returns initialised metric structs.
func (c *libvirtCollector) instanceProperties(cgroups []cgroup) libvirtMetrics {
	// Get currently active instances and set them in activeInstanceIDs state variable
	var activeInstanceIDs []string

	var instnProps []instanceProps

	// It is possible from Openstack to resize instances by changing flavour. It means
	// it is possible to add GPUs to non-GPU instances, so we need to invalidate
	// instancePropsCache once in a while to ensure we capture any changes in instance
	// flavours
	if time.Since(c.instancePropslastUpdateTime) > c.instancePropsCacheTTL {
		c.instancePropsCache = make(map[string]instanceProps)
		c.instancePropslastUpdateTime = time.Now()
	}

	for icgrp := range cgroups {
		instanceID := cgroups[icgrp].id

		// Get instance details
		if iProps, ok := c.instancePropsCache[instanceID]; !ok {
			c.instancePropsCache[instanceID] = c.getInstanceProperties(instanceID)
			instnProps = append(instnProps, c.instancePropsCache[instanceID])
			cgroups[icgrp].uuid = c.instancePropsCache[instanceID].uuid
		} else {
			instnProps = append(instnProps, iProps)
			cgroups[icgrp].uuid = iProps.uuid
		}

		// Check if we already passed through this instance
		if !slices.Contains(activeInstanceIDs, instanceID) {
			activeInstanceIDs = append(activeInstanceIDs, instanceID)
		}
	}

	// Remove terminated instances from instancePropsCache
	for uuid := range c.instancePropsCache {
		if !slices.Contains(activeInstanceIDs, uuid) {
			delete(c.instancePropsCache, uuid)
		}
	}

	return libvirtMetrics{instanceProps: instnProps, cgroups: cgroups}
}

// getInstanceProperties returns instance properties parsed from XML file.
func (c *libvirtCollector) getInstanceProperties(instanceID string) instanceProps {
	// If vGPU is activated on atleast one GPU, update mdevs
	if c.vGPUActivated {
		if updatedGPUDevs, err := updateGPUMdevs(c.gpuDevs); err == nil {
			c.gpuDevs = updatedGPUDevs
			c.logger.Debug("GPU mdevs updated")
		}
	}

	// Read XML file in a security context that raises necessary capabilities
	dataPtr := &libvirtReadXMLSecurityCtxData{
		xmlPath:    *libvirtXMLDir,
		devices:    c.gpuDevs,
		instanceID: instanceID,
	}

	if securityCtx, ok := c.securityContexts[libvirtReadXMLCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			c.logger.Error(
				"Failed to run inside security contxt", "instance_id", instanceID, "err", err,
			)

			return instanceProps{}
		}
	} else {
		c.logger.Error(
			"Security context not found", "name", libvirtReadXMLCtx, "instance_id", instanceID,
		)

		return instanceProps{}
	}

	return dataPtr.instanceProps
}

// instanceMetrics returns initialised instance metrics structs.
func (c *libvirtCollector) instanceMetrics() (libvirtMetrics, error) {
	// Get active cgroups
	cgroups, err := c.cgroupManager.discover()
	if err != nil {
		return libvirtMetrics{}, fmt.Errorf("failed to discover cgroups: %w", err)
	}

	// Get all instance properties and initialise metric structs
	return c.instanceProperties(cgroups), nil
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
				strings.TrimPrefix(hostDev.Source.Address.Domain, "0x"),
				strings.TrimPrefix(hostDev.Source.Address.Bus, "0x"),
				strings.TrimPrefix(hostDev.Source.Address.Slot, "0x"),
				strings.TrimPrefix(hostDev.Source.Address.Function, "0x"),
			)

			// Check if the current Bus ID matches with any existing GPUs
			for _, dev := range d.devices {
				if dev.CompareBusID(gpuBusID) {
					gpuOrdinals = append(gpuOrdinals, dev.globalIndex)

					break
				}
			}
		} else if hostDev.Type == "mdev" {
			mdevUUID := hostDev.Source.Address.UUID

			// Check which GPU has this mdev UUID
			for _, dev := range d.devices {
				if dev.migEnabled {
					for _, mig := range dev.migInstances {
						if slices.Contains(mig.mdevUUIDs, mdevUUID) {
							gpuOrdinals = append(gpuOrdinals, mig.globalIndex)

							break
						}
					}
				} else {
					if slices.Contains(dev.mdevUUIDs, mdevUUID) {
						gpuOrdinals = append(gpuOrdinals, dev.globalIndex)

						break
					}
				}
			}
		}
	}

	// Read instance properties into dataPointer
	d.instanceProps = instanceProps{
		uuid:        domain.UUID,
		gpuOrdinals: gpuOrdinals,
	}

	return nil
}
