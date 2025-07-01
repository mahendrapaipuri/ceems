package collector

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/osexec"
	ceems_k8s "github.com/mahendrapaipuri/ceems/pkg/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Used for e2e tests.
var (
	gpuVendor = CEEMSExporterApp.Flag(
		"collector.gpu.type",
		"GPU device type. Currently only nvidia and amd devices are supported.",
	).Hidden().Enum("nvidia", "amd", "nogpu")
	nvidiaSmiPath = CEEMSExporterApp.Flag(
		"collector.gpu.nvidia-smi-path",
		"Absolute path to nvidia-smi binary. Use only for testing.",
	).Hidden().Default("").String()
	rocmSmiPath = CEEMSExporterApp.Flag(
		"collector.gpu.rocm-smi-path",
		"Absolute path to rocm-smi binary. Use only for testing.",
	).Hidden().Default("").String()
	amdSmiPath = CEEMSExporterApp.Flag(
		"collector.gpu.amd-smi-path",
		"Absolute path to amd-smi binary. Use only for testing.",
	).Hidden().Default("").String()
)

// Custom errors.
var (
	errUnsupportedGPUVendor = errors.New("unsupported gpu vendor")
)

// Regexes.
var (
	pciBusIDRegex = regexp.MustCompile(`(?P<domain>[0-9a-fA-F]+):(?P<bus>[0-9a-fA-F]+):(?P<slot>[0-9a-fA-F]+)\.(?P<function>[0-9a-fA-F]+)`)
	mdevRegexes   = map[string]*regexp.Regexp{
		"pciAddr":   regexp.MustCompile(`GPU ([a-fA-F0-9.:]+)`),
		"mdevUUID":  regexp.MustCompile(`^\s+MDEV UUID\s+: ([a-zA-Z0-9\-]+)`),
		"gpuInstID": regexp.MustCompile(`^\s+GPU Instance ID\s+: ([0-9]+|N/A)`),
	}
	fullGPURegex = regexp.MustCompile(`GPU (\d+):(?:.*)\(UUID: (GPU-[\w\-]+)\)`)
	migGPURegex  = regexp.MustCompile(`\s+MIG (?:.*)\s+Device\s+(\d+): \(UUID: (MIG-[\w\-]+)\)`)
)

// Different SMI commands.
var (
	nvidiaSMIQueryCmd     = []string{"nvidia-smi", "--query", "--xml-format"}
	nvidiaSMIListCmd      = []string{"nvidia-smi", "-L"}
	nvidiaSMIQueryvGPUCmd = []string{"nvidia-smi", "vgpu", "--query"}
	rocmSMIQueryCmd       = []string{"rocm-smi", "--showproductname", "--showserial", "--showbus", "--showcomputepartition", "--showmemorypartition", "--json"}
	amdSMIQueryCmd        = []string{"amd-smi", "static", "-a", "-B", "-b", "-p", "--json"}
)

// Nvidia SM count for different architectures
// Fetched from https://www.techpowerup.com/gpu-specs/
var (
	nvidiaSMCount = map[string]uint64{
		"V100": 80,
		"A10":  72,
		"A16":  10,
		"A100": 108,
		"A2":   10,
		"A30":  56,
		"A40":  84,
		"A800": 108,
		"H100": 132,
		"H200": 132,
		"H800": 114,
		"B100": 132,
		"B200": 132,
	}
)

// If compute partitioning is set to CPX, it means GPU is divided into 8 equal
// slices. They will be exposed as separate GPUs on the server. However, they all
// belong to the same physical GPU. This is equivalent of NVIDIA's MIG instances
// Ref: https://rocm.blogs.amd.com/software-tools-optimization/compute-memory-modes/README.html
//
// Different types of partitions are described here:
// https://rocm.docs.amd.com/projects/amdsmi/en/docs-6.0.0/doxygen/docBin/html/amdsmi_8h.html#af5c57809078d4345b08b59b4162cd6ba
var (
	amdPartitionModes = []string{"dpx", "tpx", "qpx", "cpx"}
)

type vendorID int

// String implements Stringer interface of the vendorID type.
func (v vendorID) String() string {
	switch v {
	case nvidia:
		return "NVIDIA"
	case amd:
		return "AMD"
	default:
		return "Unknown"
	}
}

// Device vendors enum.
const (
	_ vendorID = iota
	nvidia
	amd
)

type vendor struct {
	name         string
	id           vendorID
	pciID        uint64
	smiCmd       string
	smiQueryCmd  []string
	k8sNS        string
	k8sPod       string
	k8sContainer string
}

// PCI Device vendor IDs.
var (
	devVendors = []vendor{
		nvidia: {
			name:  "nvidia",
			id:    nvidia,
			pciID: uint64(0x10de),
		},
		amd: {
			name:  "amd",
			id:    amd,
			pciID: uint64(0x1002),
		},
	}
)

// BusID is a struct that contains PCI bus address of GPU device.
type BusID struct {
	domain   uint64
	bus      uint64
	device   uint64
	function uint64
	pathName string // This is the EXACT name under which device details are stored in /sys and /devices
}

// String implements Stringer interface of the BusID struct.
func (b BusID) String() string {
	return fmt.Sprintf(
		"%s:%s:%s.%s",
		strconv.FormatUint(b.domain, 16),
		strconv.FormatUint(b.bus, 16),
		strconv.FormatUint(b.device, 16),
		strconv.FormatUint(b.function, 16),
	)
}

// Compare compares the provided bus ID with current bus ID and
// returns true if they match and false in all other cases.
func (b *BusID) Compare(bTest BusID) bool {
	// Check equality component per component in ID
	if b.domain == bTest.domain && b.bus == bTest.bus && b.device == bTest.device && b.function == bTest.function {
		return true
	} else {
		return false
	}
}

type Memory struct {
	Total string `xml:"total"`
}

type DeviceAttrsShared struct {
	XMLName  xml.Name `xml:"shared"`
	SMCount  uint64   `xml:"multiprocessor_count"`
	CECount  uint64   `xml:"copy_engine_count"`
	EncCount uint64   `xml:"encoder_count"`
	DecCount uint64   `xml:"decoder_count"`
}

type DeviceAttrs struct {
	XMLName xml.Name          `xml:"device_attributes"`
	Shared  DeviceAttrsShared `xml:"shared"`
}

type MIGDevice struct {
	XMLName       xml.Name    `xml:"mig_device"`
	Index         uint64      `xml:"index"`
	GPUInstID     uint64      `xml:"gpu_instance_id"`
	ComputeInstID uint64      `xml:"compute_instance_id"`
	DeviceAttrs   DeviceAttrs `xml:"device_attributes"`
	FBMemory      Memory      `xml:"fb_memory_usage"`
	Bar1Memory    Memory      `xml:"bar1_memory_usage"`
	UUID          string
}

type MIGDevices struct {
	XMLName xml.Name    `xml:"mig_devices"`
	Devices []MIGDevice `xml:"mig_device"`
}

type ProcessInfo struct {
	XMLName       xml.Name `xml:"process_info"`
	GPUInstID     uint64   `xml:"gpu_instance_id"`
	ComputeInstID uint64   `xml:"compute_instance_id"`
	PID           uint64   `xml:"pid"`
}

type Processes struct {
	XMLName      xml.Name      `xml:"processes"`
	ProcessInfos []ProcessInfo `xml:"process_info"`
}

type VirtMode struct {
	XMLName  xml.Name `xml:"gpu_virtualization_mode"`
	Mode     string   `xml:"virtualization_mode"`
	HostMode string   `xml:"host_vgpu_mode"`
}

type MIGMode struct {
	XMLName    xml.Name `xml:"mig_mode"`
	CurrentMIG string   `xml:"current_mig"`
}

type NvidiaGPU struct {
	XMLName      xml.Name   `xml:"gpu"`
	ID           string     `xml:"id,attr"`
	ProductName  string     `xml:"product_name"`
	ProductBrand string     `xml:"product_brand"`
	ProductArch  string     `xml:"product_architecture"`
	MIGMode      MIGMode    `xml:"mig_mode"`
	VirtMode     VirtMode   `xml:"gpu_virtualization_mode"`
	MIGDevices   MIGDevices `xml:"mig_devices"`
	UUID         string     `xml:"uuid"`
	MinorNumber  string     `xml:"minor_number"`
	Processes    Processes  `xml:"processes"`
}

type NVIDIASMILog struct {
	XMLName xml.Name    `xml:"nvidia_smi_log"`
	GPUs    []NvidiaGPU `xml:"gpu"`
}

type AMDNodeProperties struct {
	DevID          uint64 // Unique ID for each physical GPU
	RenderID       uint64 // renderDx for each GPU (physical or partition)
	DevicePluginID string // ID used in k8s device plugin
	NumCUs         uint64
}

type AMDASIC struct {
	NumCUs uint64 `json:"num_compute_units"`
}

type AMDBus struct {
	BDF string `json:"bdf"`
}

type AMDBoard struct {
	Serial string `json:"product_serial"`
	Name   string `json:"product_name"`
}

type AMDPartition struct {
	Compute string `json:"compute_partition"`
	Memory  string `json:"memory_partition"`
	ID      uint64 `json:"partition_id"`
}

type AMDGPU struct {
	ID        int64         `json:"gpu"`
	ASIC      *AMDASIC      `json:"asic"`
	Bus       *AMDBus       `json:"bus"`
	Board     *AMDBoard     `json:"board"`
	Partition *AMDPartition `json:"partition"`
}

type ROCMSMI struct {
	Bus              string `json:"PCI Bus"`
	Serial           string `json:"Serial Number"`
	Name             string `json:"Card Vendor"`
	Node             string `json:"Node ID"`
	ComputePartition string `json:"Compute Partition"`
	MemoryPartition  string `json:"Memory Partition"`
}

// ComputeUnit contains the unit details that will be associated with each GPU.
type ComputeUnit struct {
	UUID      string
	Hostname  string // Only applicable to SLURM when multiple daemons are enabled on same physical host
	NumShares uint64 // In case of time slicing/shards/MPS
}

// GPUInstance is abstraction for NVIDIA MIG instance or AMD GPU partition.
type GPUInstance struct {
	InstanceIndex   uint64
	Index           string
	UUID            string
	ComputeInstID   uint64
	GPUInstID       uint64
	SMFraction      float64
	NumSMs          uint64
	ComputeUnits    []ComputeUnit
	CurrentShares   uint64 // A share can be time slicing or MPS
	AvailableShares uint64
	MdevUUIDs       []string
}

// String implements Stringer interface of the Device struct.
func (d GPUInstance) String() string {
	return fmt.Sprintf(
		"index: %s; uuid: %s; instance_index: %d; gpu_instance_id: %d; compute_instance_id: %d; num_sm: %d; "+
			"available_shares: %d",
		d.Index, d.UUID, d.InstanceIndex, d.GPUInstID, d.ComputeInstID, d.NumSMs, d.AvailableShares,
	)
}

// ID return instance ID that will be used by k8s requests.
func (d GPUInstance) ID() string {
	return d.UUID
}

// ResetUnits will remove existing compute unit UUIDs.
func (d *GPUInstance) ResetUnits() {
	d.ComputeUnits = make([]ComputeUnit, 0)
	d.CurrentShares = 0
}

// Device contains the details of physical GPU devices.
type Device struct {
	vendorID         vendorID
	Minor            string
	Index            string
	Name             string
	UUID             string
	BusID            BusID
	NumSMs           uint64
	ComputeUnits     []ComputeUnit
	CurrentShares    uint64 // A share can be time slicing or MPS
	AvailableShares  uint64
	MdevUUIDs        []string
	Instances        []GPUInstance
	InstancesEnabled bool
	VGPUEnabled      bool
}

// String implements Stringer interface of the Device struct.
func (d Device) String() string {
	return fmt.Sprintf(
		"name: %s; minor: %s; index: %s; uuid: %s; bus_id: %s; "+
			"instances_enabled: %t; num_instances: %d; available_shares; %d; "+
			"vgpu_enabled: %t",
		d.Name, d.Minor, d.Index, d.UUID, d.BusID,
		d.InstancesEnabled, len(d.Instances),
		d.AvailableShares, d.VGPUEnabled,
	)
}

// ID return device ID that will be used by k8s requests.
func (d Device) ID() string {
	switch d.vendorID {
	case nvidia:
		return d.UUID
	case amd:
		return d.BusID.pathName
	default:
		return ""
	}
}

// ResetUnits will remove existing compute unit UUIDs.
func (d *Device) ResetUnits() {
	d.ComputeUnits = make([]ComputeUnit, 0)
	d.CurrentShares = 0

	for iinst := range d.Instances {
		d.Instances[iinst].ResetUnits()
	}
}

// CompareBusID compares the provided bus ID with device bus ID and
// returns true if they match and false in all other cases.
func (d *Device) CompareBusID(id string) bool {
	// Parse bus id that needs to be compared
	BusID, err := parseBusID(id)
	if err != nil {
		return false
	}

	// Check equality component per component in ID
	return d.BusID.Compare(BusID)
}

// GPUSMI is a vendor neutral SMI interface for GPUs.
type GPUSMI struct {
	logger    *slog.Logger
	vendors   []vendor
	k8sClient *ceems_k8s.Client
	Devices   []Device
}

// NewGPUSMI returns a new instance of GPUSMI struct to query GPUs.
func NewGPUSMI(k8sClient *ceems_k8s.Client, logger *slog.Logger) (*GPUSMI, error) {
	var vendors []vendor

	var err error

	// Get GPU vendors found in devices
	if *gpuVendor != "" {
		switch *gpuVendor {
		case "nvidia":
			vendors = []vendor{devVendors[nvidia]}
		case "amd":
			vendors = []vendor{devVendors[amd]}
		case "nogpu":
			vendors = nil
		}
	} else {
		// Detect GPU device vendors
		vendors, err = detectVendors()
		if err != nil {
			logger.Warn("Failed to detect GPU devices")

			return nil, fmt.Errorf("failed to detect devices: %w", err)
		}
	}

	// If no vendors found return early
	if len(vendors) == 0 {
		return &GPUSMI{logger: logger}, nil
	}

	// If GPUs are found, emit DEBUG logs
	vendorNames := make([]string, len(vendors))
	for iv, vendor := range vendors {
		vendorNames[iv] = vendor.name
	}

	logger.Debug("Detected GPU vendors: " + strings.Join(vendorNames, ","))

	// Find smi commands paths if they exist
	for iv, v := range vendors {
		switch v.id {
		case nvidia:
			// Look up nvidia-smi command
			if smiCmd, err := lookupSmiCmd(*nvidiaSmiPath, nvidiaSMIQueryCmd[0]); err == nil {
				vendors[iv].smiCmd = smiCmd
				vendors[iv].smiQueryCmd = nvidiaSMIQueryCmd
			}
		case amd:
			// Look up amd-smi command
			// Always look for amd-smi command first and if not found fallback to rocm-smi
			// Always prefer amd-smi to rocm-smi. This is preferred way
			// to query for AMD GPUs
			// Ref: https://rocm.blogs.amd.com/software-tools-optimization/amd-smi-overview/README.html#transitioning-from-rocm-smi
			if smiCmd, err := lookupSmiCmd(*amdSmiPath, amdSMIQueryCmd[0]); err == nil {
				vendors[iv].smiCmd = smiCmd
				vendors[iv].smiQueryCmd = amdSMIQueryCmd
			} else {
				if smiCmd, err := lookupSmiCmd(*rocmSmiPath, rocmSMIQueryCmd[0]); err == nil {
					vendors[iv].smiCmd = smiCmd
					vendors[iv].smiQueryCmd = rocmSMIQueryCmd
				}
			}
		}
	}

	// If k8sClient is not nil, figure out which containers are running the drivers
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if k8sClient != nil {
		for iv, v := range vendors {
			var contName, smiCmd string

			var smiQueryCmd []string

			var opts v1.ListOptions

			switch v.id {
			case nvidia:
				contName = "gpu-feature-discovery"
				opts = v1.ListOptions{LabelSelector: "app=gpu-feature-discovery"}
				smiCmd = nvidiaSMIQueryCmd[0]
				smiQueryCmd = nvidiaSMIQueryCmd
			case amd:
				contName = "metrics-exporter-container"
				opts = v1.ListOptions{LabelSelector: "app.kubernetes.io/name=metrics-exporter"}
				smiCmd = amdSMIQueryCmd[0]
				smiQueryCmd = amdSMIQueryCmd
			}

			if pods, err := k8sClient.Pods(ctx, "", opts); err == nil && len(pods) > 0 {
				vendors[iv].k8sNS = pods[0].Namespace
				vendors[iv].k8sPod = pods[0].Name

				for _, cont := range pods[0].Containers {
					if cont.Name == contName {
						vendors[iv].k8sContainer = cont.Name
					}
				}

				// If smi commands is not found locally, set them to defaults
				if vendors[iv].smiCmd == "" {
					vendors[iv].smiCmd = smiCmd
					vendors[iv].smiQueryCmd = smiQueryCmd
				}
			}
		}
	}

	return &GPUSMI{
		logger:    logger,
		vendors:   vendors,
		k8sClient: k8sClient,
	}, nil
}

// Discover finds devices on the host.
func (g *GPUSMI) Discover() error {
	var err, errs error

	for _, vendor := range g.vendors {
		var devs []Device

		// Keep checking for GPU devices with a timeout of 1 minute
		// When GPU drivers are not loaded yet, this strategy can be
		// handy to wait for the drivers to load and for SMI commands to
		// enumurate GPUs.
		for start := time.Now(); time.Since(start) < time.Minute; {
			// If errored out, sleep for a while and attempt to get devices again
			devs, err = g.gpuDevices(vendor)
			if err != nil && !errors.Is(err, errUnsupportedGPUVendor) {
				time.Sleep(10 * time.Second)
			} else {
				g.Devices = append(g.Devices, devs...)

				break
			}
		}

		// If we end up here with non nil error, we could not find GPUs for
		// this vendor. Add to errs
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	// Emit debug logs
	g.print()

	return errs
}

// UpdateGPUMdevs updates GPU devices slice with mdev UUIDs.
func (g *GPUSMI) UpdateGPUMdevs() error {
	var stdOut []byte

	var err error

	// Currently only nvidia devices support vGPU
	for _, vendor := range g.vendors {
		if vendor.id == nvidia {
			stdOut, err = g.execute(vendor, nvidiaSMIQueryvGPUCmd)
			if err != nil {
				return err
			} else {
				break
			}
		}
	}

	// If there are no devices that support vGPU, return same devices
	if stdOut == nil {
		g.logger.Debug("No devices with vGPU support found")

		return nil
	}

	vGPUQueryOut := string(stdOut)

	// Get all GPU addresses
	allGPUs := mdevRegexes["pciAddr"].FindAllString(vGPUQueryOut, -1)

	// Split all lines
	lines := strings.Split(vGPUQueryOut, "\n")

	// Get range of lines for each GPU
	var gpuIndx []int

	for iline, line := range lines {
		if slices.Contains(allGPUs, line) {
			gpuIndx = append(gpuIndx, iline)
		}
	}

	gpuIndx = append(gpuIndx, len(lines))

	// For each GPU output extract GPU addr, mdev UUID and GPU instance ID
	gpuMdevs := make(map[string][][]string)

	for indx := range len(gpuIndx) - 1 {
		var addr string

		var UUIDs, instIDs []string

		for i := gpuIndx[indx]; i < gpuIndx[indx+1]; i++ {
			if matches := mdevRegexes["pciAddr"].FindStringSubmatch(lines[i]); len(matches) > 1 {
				addr = strings.TrimSpace(matches[1])
			}

			if matches := mdevRegexes["mdevUUID"].FindStringSubmatch(lines[i]); len(matches) > 1 {
				UUIDs = append(UUIDs, strings.TrimSpace(matches[1]))
			}

			if matches := mdevRegexes["gpuInstID"].FindStringSubmatch(lines[i]); len(matches) > 1 {
				instIDs = append(instIDs, strings.TrimSpace(matches[1]))
			}
		}

		for imdev := range UUIDs {
			gpuMdevs[addr] = append(gpuMdevs[addr], []string{UUIDs[imdev], instIDs[imdev]})
		}
	}

	// Loop over each mdev and add them to devs
	for addr, mdevs := range gpuMdevs {
		// Loop over all devs to find GPU that has same BusID
		for idev := range g.Devices {
			if g.Devices[idev].CompareBusID(addr) {
				// Remove existing mdevs
				if g.Devices[idev].InstancesEnabled {
					for imig := range g.Devices[idev].Instances {
						g.Devices[idev].Instances[imig].MdevUUIDs = make([]string, 0)
					}
				} else {
					g.Devices[idev].MdevUUIDs = make([]string, 0)
				}

				for _, mdev := range mdevs {
					// If MIG is enabled, loop over all MIG instances to compare instance ID
					if g.Devices[idev].InstancesEnabled {
						for imig := range g.Devices[idev].Instances {
							if strconv.FormatUint(g.Devices[idev].Instances[imig].GPUInstID, 10) == mdev[1] {
								// Ensure not to duplicate mdevs
								if !slices.Contains(g.Devices[idev].Instances[imig].MdevUUIDs, mdev[0]) {
									g.Devices[idev].Instances[imig].MdevUUIDs = append(g.Devices[idev].Instances[imig].MdevUUIDs, mdev[0])
								}
							}
						}
					} else {
						if !slices.Contains(g.Devices[idev].MdevUUIDs, mdev[0]) {
							g.Devices[idev].MdevUUIDs = append(g.Devices[idev].MdevUUIDs, mdev[0])
						}
					}
				}
			}
		}
	}

	return nil
}

// ReindexGPUs reindexes GPU globalIndex based on orderMap string.
func (g *GPUSMI) ReindexGPUs(orderMap string) {
	// When orderMap is empty, return
	if orderMap == "" {
		g.logger.Warn("No order map provided to reindex GPUs")

		return
	}

	for _, gpuMap := range strings.Split(orderMap, ",") {
		orderMap := strings.Split(gpuMap, ":")
		if len(orderMap) < 2 {
			continue
		}

		// Check if MIG instance ID is present
		devIndx := strings.Split(orderMap[1], ".")
		for idev := range g.Devices {
			if g.Devices[idev].Minor == strings.TrimSpace(devIndx[0]) {
				if len(devIndx) == 2 {
					for imig := range g.Devices[idev].Instances {
						if strconv.FormatUint(g.Devices[idev].Instances[imig].GPUInstID, 10) == strings.TrimSpace(devIndx[1]) {
							g.Devices[idev].Instances[imig].Index = strings.TrimSpace(orderMap[0])

							break
						}
					}
				} else {
					g.Devices[idev].Index = strings.TrimSpace(orderMap[0])
				}
			}
		}
	}

	// Emit debug logs to show GPU ordering after reindexing
	g.logger.Debug("GPU order after reindexing")
	g.print()
}

// print emits debug logs with GPU details.
func (g *GPUSMI) print() {
	for _, gpu := range g.Devices {
		g.logger.Debug("GPU device", "vendor", gpu.vendorID, "details", gpu)

		for _, inst := range gpu.Instances {
			g.logger.Debug("GPU device", "vendor", gpu.vendorID, "details", inst)
		}
	}
}

// gpuDevices returns GPU devices from a given vendor.
func (g *GPUSMI) gpuDevices(vendor vendor) ([]Device, error) {
	switch vendor.id {
	case nvidia:
		return g.nvidiaGPUDevices(vendor)
	case amd:
		return g.amdGPUDevices(vendor)
	default:
		return nil, fmt.Errorf("only NVIDIA and AMD GPU devices are supported: %w", errUnsupportedGPUVendor)
	}
}

// nvidiaGPUDevices returns all physical or MIG devices using nvidia-smi command.
func (g *GPUSMI) nvidiaGPUDevices(vendor vendor) ([]Device, error) {
	// Execute nvidia-smi query command either natively or in pod
	nvidiaSmiOutput, err := g.execute(vendor, vendor.smiQueryCmd)
	if err != nil {
		return nil, err
	}

	// Parse nvidia-smi output and build devices structs
	devices, err := parseNvidiaSmiOutput(nvidiaSmiOutput)
	if err != nil {
		if len(devices) == 0 {
			return nil, err
		}

		g.logger.Warn("Errors found while querying GPU devices", "err", err)
	}

	// Add vendorID to devices
	for idev := range devices {
		devices[idev].vendorID = vendor.id
	}

	// Check if MIG is enabled on at least one GPU
	for _, device := range devices {
		if device.InstancesEnabled {
			// Get MIG UUIDs from nvidia-smi -L command output
			nvidiaSmiListOutput, err := g.execute(vendor, nvidiaSMIListCmd)
			if err != nil {
				return nil, err
			}

			return parseNvidiaSmiListOutput(string(nvidiaSmiListOutput), devices), nil
		}
	}

	return devices, nil
}

// amdGPUDevices returns all GPU devices using rocm-smi command.
func (g *GPUSMI) amdGPUDevices(vendor vendor) ([]Device, error) {
	// Execute amd-smi/rocm-smi command based on that one that is found
	smiOutput, err := g.execute(vendor, vendor.smiQueryCmd)
	if err != nil {
		return nil, err
	}

	var devices []Device

	// Parse output based on command used
	switch {
	case strings.Contains(vendor.smiCmd, "amd-smi"):
		devices, err = parseAmdSmioutput(smiOutput)
	default:
		devices, err = parseRocmSmioutput(smiOutput)
	}

	if err != nil {
		if len(devices) == 0 {
			return nil, err
		}

		g.logger.Warn("Errors found while querying GPU devices", "err", err)
	}

	// Add vendorID to devices
	for idev := range devices {
		devices[idev].vendorID = vendor.id
	}

	// Add k8s device plugin IDs to devices
	// This is largely inspired from devices-plugin of AMD. We are not yet sure
	// how the GPU instances will be exposed on the sysfs completely but this should
	// be a good starting point.
	// A playground that is useful: https://goplay.tools/snippet/gvpJ3B5Resq
	// This is taken from unit tests of upstream!!
	if deviceProperties, err := parseAMDDevPropertiesFromPCIDevices(); err == nil {
		for idev, dev := range devices {
			// Ensure that we device properties corresponding to physical bus ID
			if devProperties, ok := deviceProperties[strings.ToLower(dev.BusID.pathName)]; ok {
				// If there are GPU partitions
				if len(dev.Instances) > 0 {
					// Ensure that we got same number of properties as instances
					// Properties are already sorted based on renderD and so we
					// "assume" that they are in partition index order
					if len(dev.Instances) == len(devProperties) {
						for iinst, props := range devProperties {
							devices[idev].Instances[iinst].NumSMs = props.NumCUs
							devices[idev].Instances[iinst].UUID = props.DevicePluginID
						}
					}
				} else {
					if len(devProperties) == 1 {
						devices[idev].NumSMs = devProperties[0].NumCUs
						devices[idev].BusID.pathName = devProperties[0].DevicePluginID
					}
				}
			}
		}
	}

	return devices, nil
}

// execute a command natively or inside a container.
func (g *GPUSMI) execute(vendor vendor, cmd []string) ([]byte, error) {
	// Use a context with timeout
	// After making some tests on new nodes on Grid5000 with MI300X GPUs,
	// we noticed that amd-smi command takes a while to return. So, use
	// a longer timeout to ensure that we get the list of GPUs
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// If smi command is found, always prefer to execute it natively
	if vendor.smiCmd != "" {
		if cmdOut, err := osexec.ExecuteContext(ctx, vendor.smiCmd, cmd[1:], nil); err == nil {
			g.logger.Debug("GPU query command executed natively", "vendor", vendor.name, "cmd", strings.Join(cmd, " "))

			return cmdOut, nil
		}
	}

	// If k8sclient is available, attempt to execute command inside container
	if g.k8sClient != nil {
		if stdout, _, err := g.k8sClient.Exec(ctx, vendor.k8sNS, vendor.k8sPod, vendor.k8sContainer, cmd); err == nil {
			g.logger.Debug("GPU query command executed inside pod", "vendor", vendor.name, "cmd", strings.Join(cmd, " "), "pod", fmt.Sprintf("%s/%s/%s", vendor.k8sNS, vendor.k8sPod, vendor.k8sContainer))

			return stdout, nil
		}
	}

	return nil, fmt.Errorf("failed to execute command %s natively or inside a pod", strings.Join(cmd, " "))
}

// detectVendors walks through all PCI devices and return a slice of device
// vendor (nvidia and/or amd) for display controllers, if exist.
func detectVendors() ([]vendor, error) {
	// Glob to get all devices
	pciDevs, err := filepath.Glob(sysFilePath("bus/pci/devices/*"))
	if err != nil {
		return nil, fmt.Errorf("failed to get PCI devices: %w", err)
	}

	// Get all vendor IDs
	var vendorIDs []uint64

	for _, devPath := range pciDevs {
		// PCI class 0x03 is for display controllers AKA GPUs
		// PCI class 0x12 is for processing accelerators. MI300X GPUs seems to have this class ID.
		// Check if class starts with "0x03" and if it does, it means it is a GPU device
		// Ref: https://pcisig.com/sites/default/files/files/PCI_Code-ID_r_1_11__v24_Jan_2019.pdf
		if classBytes, err := os.ReadFile(filepath.Join(devPath, "class")); err == nil {
			if class := strings.TrimSpace(strings.Trim(string(classBytes), "\n")); !strings.HasPrefix(class, "0x03") && !strings.HasPrefix(class, "0x12") {
				continue
			}

			if idBytes, err := os.ReadFile(filepath.Join(devPath, "vendor")); err == nil {
				// Strip new lines and spaces
				idString := strings.TrimSpace(strings.Trim(string(idBytes), "\n"))

				// Let Go pick the base as vendor IDs will be prefixed by "0x"
				if id, err := strconv.ParseUint(idString, 0, 16); err == nil {
					vendorIDs = append(vendorIDs, id)
				}
			}
		}
	}

	// Now check if either nvidia or AMD devices are in slice
	var foundDevices []vendor

	for _, vendor := range devVendors {
		if slices.Contains(vendorIDs, vendor.pciID) {
			foundDevices = append(foundDevices, vendor)
		}
	}

	// Remove duplicates and sort slice
	foundDevices = slices.CompactFunc(foundDevices, func(a, b vendor) bool {
		return a.id == b.id
	})
	slices.SortFunc(foundDevices, func(a, b vendor) int {
		return cmp.Compare(a.id, b.id)
	})

	return foundDevices, nil
}

// lookupSmiCmd checks if nvidia-smi/rocm-smi path provided by CLI exists and falls back
// to `nvidia-smi`/`rocm-smi` command on host.
func lookupSmiCmd(customCmd string, fallbackCmd string) (string, error) {
	if customCmd != "" {
		if _, err := os.Stat(customCmd); err != nil {
			return "", err
		}

		return customCmd, nil
	} else {
		if _, err := exec.LookPath(fallbackCmd); err != nil {
			return "", err
		} else {
			return fallbackCmd, nil
		}
	}
}

// parseNvidiaSmiOutput parses nvidia-smi output and return GPU Devices map.
func parseNvidiaSmiOutput(cmdOutput []byte) ([]Device, error) {
	// Get all devices
	var gpuDevices []Device

	var errs error

	// Read XML byte array into gpu
	var nvidiaSMILog NVIDIASMILog
	if err := xml.Unmarshal(cmdOutput, &nvidiaSMILog); err != nil { //nolint:musttag
		return nil, err
	}

	// NOTE: Ensure that we sort the devices using PCI address
	// Seems like nvidia-smi most of the times returns them in correct order.
	var globalIndex uint64

	for igpu, gpu := range nvidiaSMILog.GPUs {
		var err error

		dev := Device{
			Minor: strconv.FormatInt(int64(igpu), 10),
			UUID:  gpu.UUID,
			Name:  fmt.Sprintf("%s %s %s", gpu.ProductName, gpu.ProductBrand, gpu.ProductArch),
		}

		// Attempt to get total number of SMs
		for model, numSMs := range nvidiaSMCount {
			// Model names can be as follows:
			// Telsa V100-PCIe-40GB
			// NVIDIA A100-SXM-80GB
			// NVIDIA H100 80GB HB3
			// NVIDIA B100 and so on..
			// So we try to split by "space" and hypen and attempt to test
			// against model names
			for _, s := range strings.Split(gpu.ProductName, " ") {
				for _, ss := range strings.Split(s, "-") {
					if strings.TrimSpace(ss) == model {
						dev.NumSMs = numSMs

						break
					}
				}
			}
		}

		// Parse bus ID
		dev.BusID, err = parseBusID(gpu.ID)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to parse bus ID %s for GPU %s: %w", gpu.ID, gpu.UUID, err))
		}

		// Check MIG stats
		if strings.ToLower(gpu.MIGMode.CurrentMIG) == "enabled" {
			dev.InstancesEnabled = true
		} else {
			dev.Index = strconv.FormatUint(globalIndex, 10)
			globalIndex++
		}

		// If MIG is enabled, get all MIG devices
		var totalSMs float64

		var migDevs []GPUInstance

		for _, mig := range gpu.MIGDevices.Devices {
			migDev := GPUInstance{
				InstanceIndex: mig.Index,
				Index:         strconv.FormatUint(globalIndex, 10),
				ComputeInstID: mig.ComputeInstID,
				GPUInstID:     mig.GPUInstID,
				NumSMs:        mig.DeviceAttrs.Shared.SMCount,
			}

			totalSMs += float64(mig.DeviceAttrs.Shared.SMCount)

			migDevs = append(migDevs, migDev)
			globalIndex++
		}

		// Now we have total SMs get fraction for each instance.
		// We will use it for splitting total power between instances.
		for imig, mig := range gpu.MIGDevices.Devices {
			// If we found number of SMs on the GPU, use it. Else use sum
			// of MIG SMs
			if dev.NumSMs > 0 {
				totalSMs = float64(dev.NumSMs)
			}

			migDevs[imig].SMFraction = float64(mig.DeviceAttrs.Shared.SMCount) / totalSMs
		}

		dev.Instances = migDevs

		// Check vGPU status
		// Options can be VGPU and VSGA. VSGA is some vSphere stuff so we
		// dont worry about it. Only check if mode is VGPU
		if strings.ToLower(gpu.VirtMode.Mode) == "vgpu" {
			dev.VGPUEnabled = true
		}

		gpuDevices = append(gpuDevices, dev)
	}

	return gpuDevices, errs
}

// parseNvidiaSmiListOutput parses nvidia-smi -L output and add MIG UUIDs.
func parseNvidiaSmiListOutput(cmdOutput string, devices []Device) []Device {
	// Get GPU lines from output
	allGPUs := fullGPURegex.FindAllString(cmdOutput, -1)

	// Split all lines
	lines := strings.Split(strings.TrimSpace(cmdOutput), "\n")

	// Get range of lines for each GPU
	var gpuIndx []int

	for iline, line := range lines {
		if slices.Contains(allGPUs, line) {
			gpuIndx = append(gpuIndx, iline)
		}
	}

	gpuIndx = append(gpuIndx, len(lines))

	// A map of full GPU UUID to MIG index to UUID map
	gpuMIGUUIDs := make(map[string]map[string]string)

	for indx := range len(gpuIndx) - 1 {
		var fullGPUUUID, UUID, index string

		for i := gpuIndx[indx]; i < gpuIndx[indx+1]; i++ {
			if matches := fullGPURegex.FindStringSubmatch(lines[i]); len(matches) > 2 {
				fullGPUUUID = strings.TrimSpace(matches[2])
				if gpuMIGUUIDs[fullGPUUUID] == nil {
					gpuMIGUUIDs[fullGPUUUID] = make(map[string]string)
				}
			}

			if matches := migGPURegex.FindStringSubmatch(lines[i]); len(matches) > 2 {
				UUID = strings.TrimSpace(matches[2])
				index = strings.TrimSpace(matches[1])
				gpuMIGUUIDs[fullGPUUUID][index] = UUID
			}
		}
	}

	// Now add MIG UUIDs to devices struct
	for idev, device := range devices {
		if migUUIDs := gpuMIGUUIDs[device.UUID]; len(migUUIDs) > 0 {
			for imig, mig := range device.Instances {
				devices[idev].Instances[imig].UUID = migUUIDs[strconv.FormatUint(mig.InstanceIndex, 10)]
			}
		}
	}

	return devices
}

// parseRocmSmioutput parses rocm-smi output and return AMD devices.
// Example output:
// bash-4.4$ rocm-smi --showproductname --showserial --showbus --json
//
//	{
//	  "card0": {
//	    "Serial Number": "20170000800c",
//	    "PCI Bus": "0000:C5:00.0",
//	    "Card Series": "N/A",
//	    "Card Model": "0x66a1",
//	    "Card Vendor": "Advanced Micro Devices, Inc. [AMD/ATI]",
//	    "Card SKU": "D1631700",
//	    "Subsystem ID": "0x0834",
//	    "Device Rev": "0x02",
//	    "Node ID": "3",
//	    "Compute Partition": "SPX",
//	    "GUID": "61749",
//	    "GFX Version": "gfx906"
//	  }
//	}.
func parseRocmSmioutput(cmdOutput []byte) ([]Device, error) {
	var gpuDevices []Device

	var errs error

	// Unmarshall output into AMDSMILog struct
	amdDevs := make(map[string]ROCMSMI)
	if err := json.Unmarshal(cmdOutput, &amdDevs); err != nil {
		return nil, fmt.Errorf("failed to parse ROCM SMI output: %w", err)
	}

	// As map's order is undefined, first collect all the keys and sort them
	var cardIDs []string
	for id := range amdDevs {
		cardIDs = append(cardIDs, id)
	}

	// Sort cards slices based on index
	slices.SortFunc(cardIDs, func(a, b string) int {
		if aIndx, err := strconv.ParseInt(strings.TrimPrefix(a, "card"), 10, 64); err == nil {
			if bIndx, err := strconv.ParseInt(strings.TrimPrefix(b, "card"), 10, 64); err == nil {
				return cmp.Compare(aIndx, bIndx)
			}
		}

		return 0
	})

	gpuPartitions := make(map[string][]GPUInstance)

	var gpuUUIDs []string

	// var numGPUPartitions int64

	for cardIndx, card := range cardIDs {
		// Get device details
		gpu := amdDevs[card]

		// Get device index, name and UUID
		var devUUID, devBusID, devName string

		var globalIndex string

		var instancesEnabled bool

		// This corresponds to physical GPU index
		// But amd-smi output considers partition as GPU
		// and increment the index. Here we ensure we get
		// physical device indexes
		//
		// Set local and global indexes to dev indes
		globalIndex = strconv.FormatInt(int64(cardIndx), 10)

		devUUID = gpu.Serial
		devName = gpu.Name
		devBusID = gpu.Bus

		// If current GPU is partitioned into more than one instances, add them to
		// GPU partitions
		if slices.Contains(amdPartitionModes, strings.ToLower(gpu.ComputePartition)) {
			instancesEnabled = true

			gpuPartitions[devUUID] = append(gpuPartitions[devUUID], GPUInstance{
				InstanceIndex: uint64(len(gpuPartitions[devUUID])),
				GPUInstID:     uint64(len(gpuPartitions[devUUID])),
				Index:         globalIndex,
			})

			// If GPU partitioning is enabled, we should not set global index for physical
			// GPU
			globalIndex = ""
		}

		// Already added to devices slice.
		if slices.Contains(gpuUUIDs, devUUID) {
			continue
		}

		// Parse bus ID
		BusID, err := parseBusID(devBusID)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to parse bus ID %s for GPU %s: %w", devBusID, devUUID, err))
		}

		dev := Device{Index: globalIndex, Name: devName, UUID: devUUID, BusID: BusID, InstancesEnabled: instancesEnabled}

		gpuDevices = append(gpuDevices, dev)
		gpuUUIDs = append(gpuUUIDs, dev.UUID)
	}

	// Now add all the GPU instances to corresponding physical GPU
	// var xcpIndx int

	for idev, dev := range gpuDevices {
		if partitions, ok := gpuPartitions[dev.UUID]; ok {
			for _, partition := range partitions {
				partition.SMFraction = float64(1.0) / float64(len(partitions))

				// // When GPU partitions are enabled, each partition ID will be
				// // setup in /sys/devices/platform with fake PCIe addresses. They
				// // will be of form `amdgpu_xcp.0`, `amdgpu_xcp.1`, etc. Anyways we are doing an
				// // educated guess here and it will be eventually overwritten by
				// // "actual" IDs parsed from topology and sys devices later.
				// // Ref: https://github.com/ROCm/k8s-device-plugin/blob/af51ed1db60839d47c59027e896ac106f8b813a7/internal/pkg/amdgpu/amdgpu.go#L142-L262
				// if ipartition == 0 {
				// 	partition.UUID = dev.BusID.pathName
				// } else {
				// 	partition.UUID = fmt.Sprintf("amdgpu_xcp.%d", xcpIndx)
				// 	xcpIndx++
				// }

				gpuDevices[idev].Instances = append(gpuDevices[idev].Instances, partition)
			}
		}

		gpuDevices[idev].Minor = strconv.FormatInt(int64(idev), 10)
	}

	return gpuDevices, errs
}

// parseAmdSmioutput parses amd-smi output and return AMD devices.
// Example output:
// bash-4.4$ amd-smi static -B -b --json
// [
//
//	{
//	    "gpu": 0,
//	    "bus": {
//	        "bdf": "0000:06:00.0",
//	        "max_pcie_width": "N/A",
//	        "max_pcie_speed": "N/A",
//	        "pcie_interface_version": "N/A",
//	        "slot_type": "N/A"
//	    },
//	    "board": {
//	        "model_number": "2-D16317-10ct",
//	        "product_serial": "20080004260c",
//	        "fru_id": "N/A",
//	        "product_name": "deon Instinct MI50 32GB",
//	        "manufacturer_name": "0x1002"
//	    }
//	}
//
// ].
func parseAmdSmioutput(cmdOutput []byte) ([]Device, error) {
	var gpuDevices []Device

	var errs error

	// Unmarshall output into AMDSMILog struct
	var amdDevs []AMDGPU
	if err := json.Unmarshal(cmdOutput, &amdDevs); err != nil {
		return nil, fmt.Errorf("failed to parse AMD SMI output: %w", err)
	}

	// Sort devices based on gpu.ID
	slices.SortFunc(amdDevs, func(a, b AMDGPU) int {
		return cmp.Compare(a.ID, b.ID)
	})

	gpuPartitions := make(map[string][]GPUInstance)

	var gpuUUIDs []string

	var numGPUPartitions int64

	for _, gpu := range amdDevs {
		// Get device index, name and UUID
		var devUUID, devBusID, devName string

		var numCUs uint64

		var globalIndex string

		var instancesEnabled bool

		// This corresponds to physical GPU index
		// But amd-smi output considers partition as GPU
		// and increment the index. Here we ensure we get
		// physical device indexes
		//
		// Set local and global indexes to dev indes
		globalIndex = strconv.FormatInt(gpu.ID, 10)

		// This value is not available from ROCM-SMI interface
		// out of the box. So, do not rely on it for the moment
		// if gpu.ASIC != nil {
		// 	numCUs = gpu.ASIC.NumCUs
		// }

		if gpu.Board != nil {
			devUUID = gpu.Board.Serial
			devName = gpu.Board.Name
		}

		if gpu.Bus != nil {
			devBusID = gpu.Bus.BDF
		}

		// If current GPU is partitioned into more than one instances, add them to
		// GPU partitions
		if gpu.Partition != nil {
			if slices.Contains(amdPartitionModes, strings.ToLower(gpu.Partition.Compute)) {
				instancesEnabled = true

				gpuPartitions[devUUID] = append(gpuPartitions[devUUID], GPUInstance{
					InstanceIndex: gpu.Partition.ID,
					GPUInstID:     gpu.Partition.ID,
					Index:         globalIndex,
					// numSMs:      numCUs,
				})

				// If GPU partitioning is enabled, we should not set global index for physical
				// GPU
				globalIndex = ""

				numGPUPartitions++
			}
		}

		// Already added to devices slice.
		if slices.Contains(gpuUUIDs, devUUID) {
			continue
		}

		// Parse bus ID
		BusID, err := parseBusID(devBusID)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to parse bus ID %s for GPU %s: %w", devBusID, devUUID, err))
		}

		dev := Device{Index: globalIndex, Name: devName, UUID: devUUID, BusID: BusID, InstancesEnabled: instancesEnabled, NumSMs: numCUs}

		gpuDevices = append(gpuDevices, dev)
		gpuUUIDs = append(gpuUUIDs, dev.UUID)
	}

	// Now add all the GPU instances to corresponding physical GPU
	// var xcpIndx int

	for idev, dev := range gpuDevices {
		if partitions, ok := gpuPartitions[dev.UUID]; ok {
			for _, partition := range partitions {
				partition.SMFraction = float64(1.0) / float64(len(partitions))

				// // When GPU partitions are enabled, each partition ID will be
				// // setup in /sys/devices/platform with fake PCIe addresses. They
				// // will be of form `amdgpu_xcp_1`, `amdgpu_xcp_2`, etc. We are
				// // not sure if index starts with 0 or 1. Anyways we are doing an
				// // educated guess here and it will be eventually overwritten by
				// // "actual" IDs parsed from topology and sys devices later.
				// // Ref: https://github.com/ROCm/k8s-device-plugin/blob/af51ed1db60839d47c59027e896ac106f8b813a7/internal/pkg/amdgpu/amdgpu.go#L142-L262
				// if ipartition == 0 {
				// 	partition.UUID = dev.BusID.pathName
				// } else {
				// 	partition.UUID = fmt.Sprintf("amdgpu_xcp_%d", xcpIndx)
				// }

				// xcpIndx++

				gpuDevices[idev].Instances = append(gpuDevices[idev].Instances, partition)
			}
		}

		gpuDevices[idev].Minor = strconv.FormatInt(int64(idev), 10)
	}

	return gpuDevices, errs
}

// parseAMDDevPropertiesFromPCIDevices parses PCI devices and returns properties.
func parseAMDDevPropertiesFromPCIDevices() (map[string][]AMDNodeProperties, error) {
	// First get map of renderID to properties from topologies
	renderDevIDs, err := parseAMDDevPropertiesFromTopology()
	if err != nil {
		return nil, fmt.Errorf("failed to get AMD device properties from topology: %w", err)
	}

	// Get AMD devices based on PCIe addresses
	// For eg /sys/module/amdgpu/drivers/pci:amdgpu/0000:19:00.0
	devMatches, err := filepath.Glob(sysFilePath("module/amdgpu/drivers/pci:amdgpu/[0-9a-fA-F][0-9a-fA-F][0-9a-fA-F][0-9a-fA-F]:*"))
	if err != nil {
		return nil, err
	}

	// When GPU partitioning is enabled (such as MI300's partitions), we will have more
	// devices
	// For eg /sys/devices/platform/amdgpu_xcp.30
	platformMatches, err := filepath.Glob(sysFilePath("devices/platform/amdgpu_xcp*"))
	if err != nil {
		return nil, err
	}

	// Merge all path matches
	allMatches := append(devMatches, platformMatches...)

	// This is a map from device plugin ID used in k8s to device properties
	devicePluginDevIDs := make(map[string]AMDNodeProperties)

	devBDFs := make(map[uint64]string)

	// Loop over all GPU devices (physical and partitions)
	for _, path := range allMatches {
		// Get render minor IDs for this device
		devPaths, err := filepath.Glob(path + "/drm/*")
		if err != nil {
			continue
		}

		// Loop over all DRMs available for this device
		var id string

		for _, devPath := range devPaths {
			pBase := filepath.Base(devPath)

			if strings.Contains(pBase, "renderD") {
				id = strings.Split(pBase, "renderD")[1]
			}
		}

		// Convert render ID to uint64
		if rID, err := strconv.ParseUint(id, 10, 64); err == nil {
			if val, ok := renderDevIDs[rID]; ok {
				bdf := filepath.Base(path)
				devicePluginDevIDs[bdf] = val

				// Only make a map for physical GPUs
				if !strings.Contains(bdf, "amdgpu_xcp") {
					devBDFs[val.DevID] = bdf
				}
			}
		}
	}

	// Now loop over all devices and assign BDF to both physical
	// and partitioned GPUs
	deviceProperties := make(map[string][]AMDNodeProperties)

	for devID, dev := range devicePluginDevIDs {
		if bdf, ok := devBDFs[dev.DevID]; ok {
			dev.DevicePluginID = devID
			deviceProperties[strings.ToLower(bdf)] = append(deviceProperties[strings.ToLower(bdf)], dev)
		}
	}

	// Finally sort any partitions based on renderID
	for bdf, devs := range deviceProperties {
		slices.SortFunc(devs, func(a, b AMDNodeProperties) int {
			return cmp.Compare(a.RenderID, b.RenderID)
		})

		deviceProperties[bdf] = devs
	}

	return deviceProperties, nil
}

// parseAMDDevPropertiesFromTopology parses node topology files and returns properties.
func parseAMDDevPropertiesFromTopology() (map[uint64]AMDNodeProperties, error) {
	// Map of render IDs to unique IDs of GPUs
	renderDevIDs := make(map[uint64]AMDNodeProperties)

	// Get properties of all nodes
	nodeFiles, err := filepath.Glob(sysFilePath("class/kfd/kfd/topology/nodes/*/properties"))
	if err != nil {
		return nil, err
	}

	// Loop over all node files and read properties
	for _, nodeFile := range nodeFiles {
		// Fetch render minor ID
		rID, err := parseTopologyProperties(nodeFile, regexp.MustCompile(`drm_render_minor\s(\d+)`))
		if err != nil || rID == 0 {
			continue
		}

		// Fetch unique_id value from properties file.
		// This unique_id is the same for the real gpu as well as its partitions so it will be used to associate the partitions to the real gpu
		devID, err := parseTopologyProperties(nodeFile, regexp.MustCompile(`unique_id\s(\d+)`))
		if err != nil {
			continue
		}

		// Fetch simd_count value from properties file.
		simdCount, err := parseTopologyProperties(nodeFile, regexp.MustCompile(`simd_count\s(\d+)`))
		if err != nil {
			continue
		}

		// Fetch simd_per_cu value from properties file.
		simdPerCU, err := parseTopologyProperties(nodeFile, regexp.MustCompile(`simd_per_cu\s(\d+)`))
		if err != nil {
			continue
		}

		renderDevIDs[rID] = AMDNodeProperties{
			RenderID: rID,
			DevID:    devID,
			NumCUs:   simdCount / simdPerCU,
		}
	}

	return renderDevIDs, nil
}

// parseTopologyProperties parse for a property value in kfd topology file
// The format is usually one entry per line <name> <value>.
func parseTopologyProperties(path string, re *regexp.Regexp) (uint64, error) {
	// Open properties file
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		match := re.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}

		return strconv.ParseUint(match[1], 10, 64)
	}

	return 0, fmt.Errorf("no property found in the file match %s", re.String())
}

// parseBusID parses PCIe bus ID string to BusID struct.
func parseBusID(id string) (BusID, error) {
	// Bus ID is in form of <domain>:<bus>:<slot>.<function>
	matches := pciBusIDRegex.FindStringSubmatch(id)

	var values []uint64

	for i, match := range matches {
		if i != 0 {
			value, err := strconv.ParseUint(match, 16, 16)
			if err != nil {
				return BusID{}, err
			}

			values = append(values, value)
		}
	}

	if len(values) == 4 {
		return BusID{domain: values[0], bus: values[1], device: values[2], function: values[3], pathName: strings.ToLower(id)}, nil
	}

	return BusID{}, fmt.Errorf("error parsing PCIe bus ID: %s", id)
}
