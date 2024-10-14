package collector

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/osexec"
)

// Used for e2e tests.
var (
	gpuType = CEEMSExporterApp.Flag(
		"collector.gpu.type",
		"GPU device type. Currently only nvidia and amd devices are supported.",
	).Hidden().Enum("nvidia", "amd")
	nvidiaSmiPath = CEEMSExporterApp.Flag(
		"collector.gpu.nvidia-smi-path",
		"Absolute path to nvidia-smi binary. Use only for testing.",
	).Hidden().Default("").String()
	rocmSmiPath = CEEMSExporterApp.Flag(
		"collector.gpu.rocm-smi-path",
		"Absolute path to rocm-smi binary. Use only for testing.",
	).Hidden().Default("").String()
)

// Regexes.
var (
	pciBusIDRegex = regexp.MustCompile(`(?P<domain>[0-9a-fA-F]+):(?P<bus>[0-9a-fA-F]+):(?P<slot>[0-9a-fA-F]+)\.(?P<function>[0-9a-fA-F]+)`)
	mdevRegexes   = map[string]*regexp.Regexp{
		"pciAddr":   regexp.MustCompile(`GPU ([a-fA-F0-9.:]+)`),
		"mdevUUID":  regexp.MustCompile(`^\s+MDEV UUID\s+: ([a-zA-Z0-9\-]+)`),
		"gpuInstID": regexp.MustCompile(`^\s+GPU Instance ID\s+: ([0-9]+|N/A)`),
	}
)

// BusID is a struct that contains PCI bus address of GPU device.
type BusID struct {
	domain   uint64
	bus      uint64
	device   uint64
	function uint64
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
}

type MIGDevices struct {
	XMLName xml.Name    `xml:"mig_devices"`
	Devices []MIGDevice `xml:"mig_device"`
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

type GPU struct {
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
}

type NVIDIASMILog struct {
	XMLName xml.Name `xml:"nvidia_smi_log"`
	GPUs    []GPU    `xml:"gpu"`
}

type MIGInstance struct {
	localIndex    uint64
	globalIndex   string
	computeInstID uint64
	gpuInstID     uint64
	smFraction    float64
	mdevUUIDs     []string
}

// Device contains the details of GPU devices.
type Device struct {
	localIndex   string
	globalIndex  string
	name         string
	uuid         string
	busID        BusID
	mdevUUIDs    []string
	migInstances []MIGInstance
	migEnabled   bool
	vgpuEnabled  bool
}

// String implements Stringer interface of the Device struct.
func (d Device) String() string {
	return fmt.Sprintf(
		"name: %s; local_index: %s; global_index: %s; uuid: %s; bus_id: %s; "+
			"num_mdevs: %d; mig_enabled: %t; num_migs: %d; vgpu_enabled: %t",
		d.name, d.localIndex, d.globalIndex, d.uuid, d.busID,
		len(d.mdevUUIDs), d.migEnabled, len(d.migInstances), d.vgpuEnabled,
	)
}

// CompareBusID compares the provided bus ID with device bus ID and
// returns true if they match and false in all other cases.
func (d *Device) CompareBusID(id string) bool {
	// Parse bus id that needs to be compared
	busID, err := parseBusID(id)
	if err != nil {
		return false
	}

	// Check equality component per component in ID
	return d.busID.Compare(busID)
}

// GetGPUDevices returns GPU devices.
func GetGPUDevices(gpuType string, logger log.Logger) ([]Device, error) {
	if gpuType == "nvidia" {
		return GetNvidiaGPUDevices(logger)
	} else if gpuType == "amd" {
		return GetAMDGPUDevices(logger)
	}

	return nil, fmt.Errorf("unknown GPU Type %s. Only nVIDIA and AMD GPU devices are supported", gpuType)
}

// GetNvidiaGPUDevices returns all physical or MIG devices using nvidia-smi command
// Example output:
// bash-4.4$ nvidia-smi --query-gpu=name,uuid --format=csv
// name, uuid
// Tesla V100-SXM2-32GB, GPU-f124aa59-d406-d45b-9481-8fcd694e6c9e
// Tesla V100-SXM2-32GB, GPU-61a65011-6571-a6d2-5ab8-66cbb6f7f9c3
//
// Here we are using nvidia-smi to avoid having build issues if we use
// nvml go bindings. This way we dont have deps on nvidia stuff and keep
// exporter simple.
//
// NOTE: This command does not return MIG devices.
func GetNvidiaGPUDevices(logger log.Logger) ([]Device, error) {
	// Look up nvidia-smi command
	nvidiaSmiCmd, err := lookupNvidiaSmiCmd()
	if err != nil {
		return nil, fmt.Errorf("failed to find nvidia-smi command: %w", err)
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--query", "--xml-format"}

	nvidiaSmiOutput, err := osexec.Execute(nvidiaSmiCmd, args, nil)
	if err != nil {
		return nil, err
	}

	return parseNvidiaSmiOutput(nvidiaSmiOutput, logger)
}

// GetAMDGPUDevices returns all GPU devices using rocm-smi command
// Example output:
// bash-4.4$ rocm-smi --showproductname --showserial --showbus --csv
// device,Serial Number,Card series,Card model,Card vendor,Card SKU
// card0,20170000800c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
// card1,20170003580c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
// card2,20180003050c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317.
func GetAMDGPUDevices(logger log.Logger) ([]Device, error) {
	// Look up nvidia-smi command
	rocmSmiCmd, err := lookupRocmSmiCmd()
	if err != nil {
		return nil, fmt.Errorf("failed to find rocm-smi command: %w", err)
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--showproductname", "--showserial", "--showbus", "--csv"}

	rocmSmiOutput, err := osexec.Execute(rocmSmiCmd, args, nil)
	if err != nil {
		return nil, err
	}

	return parseAmdSmioutput(string(rocmSmiOutput), logger), nil
}

// lookupNvidiaSmiCmd checks if nvidia-smi path provided by CLI exists and falls back
// to `nvidia-smi` command on host.
func lookupNvidiaSmiCmd() (string, error) {
	if *nvidiaSmiPath != "" {
		if _, err := os.Stat(*nvidiaSmiPath); err != nil {
			return "", err
		}

		return *nvidiaSmiPath, nil
	} else {
		nvidiaSmiCmd := "nvidia-smi"
		if _, err := exec.LookPath(nvidiaSmiCmd); err != nil {
			return "", err
		} else {
			return nvidiaSmiCmd, nil
		}
	}
}

// lookupRocmSmiCmd checks if rocm-smi path provided by CLI exists and falls back
// to `rocm-smi` command on host.
func lookupRocmSmiCmd() (string, error) {
	if *rocmSmiPath != "" {
		if _, err := os.Stat(*rocmSmiPath); err != nil {
			return "", err
		}

		return *rocmSmiPath, nil
	} else {
		rocmSmiCmd := "rocm-smi"
		if _, err := exec.LookPath(rocmSmiCmd); err != nil {
			return "", err
		} else {
			return rocmSmiCmd, nil
		}
	}
}

// parseNvidiaSmiOutput parses nvidia-smi output and return GPU Devices map.
func parseNvidiaSmiOutput(cmdOutput []byte, logger log.Logger) ([]Device, error) {
	// Get all devices
	var gpuDevices []Device

	// Read XML byte array into gpu
	var nvidiaSMILog NVIDIASMILog
	if err := xml.Unmarshal(cmdOutput, &nvidiaSMILog); err != nil {
		return nil, err
	}

	// NOTE: Ensure that we sort the devices using PCI address
	// Seems like nvidia-smi most of the times returns them in correct order.
	var globalIndex uint64

	for igpu, gpu := range nvidiaSMILog.GPUs {
		var err error

		dev := Device{
			localIndex: strconv.FormatInt(int64(igpu), 10),
			uuid:       gpu.UUID,
			name:       fmt.Sprintf("%s %s %s", gpu.ProductName, gpu.ProductBrand, gpu.ProductArch),
		}

		// Parse bus ID
		dev.busID, err = parseBusID(gpu.ID)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to parse GPU bus ID", "bus_id", gpu.ID, "err", err)
		}

		// Check MIG stats
		if gpu.MIGMode.CurrentMIG == "Enabled" {
			dev.migEnabled = true
		} else {
			dev.globalIndex = strconv.FormatUint(globalIndex, 10)
			globalIndex++
		}

		// If MIG is enabled, get all MIG devices
		var totalSMs float64

		var migDevs []MIGInstance

		for _, mig := range gpu.MIGDevices.Devices {
			migDev := MIGInstance{
				localIndex:    mig.Index,
				globalIndex:   strconv.FormatUint(globalIndex, 10),
				computeInstID: mig.ComputeInstID,
				gpuInstID:     mig.GPUInstID,
			}

			totalSMs += float64(mig.DeviceAttrs.Shared.SMCount)

			migDevs = append(migDevs, migDev)
			globalIndex++
		}

		// Now we have total SMs get fraction for each instance.
		// We will use it for splitting total power between instances.
		for imig, mig := range gpu.MIGDevices.Devices {
			migDevs[imig].smFraction = float64(mig.DeviceAttrs.Shared.SMCount) / totalSMs
		}

		dev.migInstances = migDevs

		// Check vGPU status
		// Options can be VGPU and VSGA. VSGA is some vSphere stuff so we
		// dont worry about it. Only check if mode is VGPU
		if gpu.VirtMode.Mode == "VGPU" {
			dev.vgpuEnabled = true
		}

		gpuDevices = append(gpuDevices, dev)
		level.Debug(logger).Log("msg", "Found nVIDIA GPU", "gpu", dev)
	}

	return gpuDevices, nil
}

// parseAmdSmioutput parses rocm-smi output and return AMD devices.
func parseAmdSmioutput(cmdOutput string, logger log.Logger) []Device {
	var gpuDevices []Device

	for _, line := range strings.Split(strings.TrimSpace(cmdOutput), "\n") {
		// Header line, empty line and newlines are ignored
		if line == "" || line == "\n" || strings.HasPrefix(line, "device") {
			continue
		}

		devDetails := strings.Split(line, ",")
		if len(devDetails) < 7 {
			continue
		}

		// Get device index, name and UUID
		devIndx := strings.TrimPrefix(devDetails[0], "card")
		devUUID := strings.TrimSpace(devDetails[1])
		devBusID := strings.TrimSpace(devDetails[2])
		devName := strings.TrimSpace(devDetails[3])

		// Parse bus ID
		busID, err := parseBusID(devBusID)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to parse GPU bus ID", "bus_id", devBusID, "err", err)
		}

		dev := Device{localIndex: devIndx, globalIndex: devIndx, name: devName, uuid: devUUID, busID: busID, migEnabled: false}
		level.Debug(logger).Log("msg", "Found AMD GPU", "gpu", dev)

		gpuDevices = append(gpuDevices, dev)
	}

	return gpuDevices
}

// reindexGPUs reindexes GPU globalIndex based on orderMap string.
func reindexGPUs(orderMap string, devs []Device) []Device {
	for _, gpuMap := range strings.Split(orderMap, ",") {
		orderMap := strings.Split(gpuMap, ":")
		if len(orderMap) < 2 {
			continue
		}

		// Check if MIG instance ID is present
		devIndx := strings.Split(orderMap[1], ".")
		for idev := range devs {
			if devs[idev].localIndex == strings.TrimSpace(devIndx[0]) {
				if len(devIndx) == 2 {
					for imig := range devs[idev].migInstances {
						if strconv.FormatUint(devs[idev].migInstances[imig].gpuInstID, 10) == strings.TrimSpace(devIndx[1]) {
							devs[idev].migInstances[imig].globalIndex = strings.TrimSpace(orderMap[0])

							break
						}
					}
				} else {
					devs[idev].globalIndex = strings.TrimSpace(orderMap[0])
				}
			}
		}
	}

	return devs
}

// updateGPUMdevs updates GPU devices slice with mdev UUIDs.
func updateGPUMdevs(devs []Device) ([]Device, error) {
	// Look up nvidia-smi command
	nvidiaSmiCmd, err := lookupNvidiaSmiCmd()
	if err != nil {
		return nil, fmt.Errorf("failed to find nvidia-smi command: %w", err)
	}

	// Execute command
	stdOut, err := osexec.Execute(nvidiaSmiCmd, []string{"vgpu", "--query"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to execute nvidia-smi vgpu command: %w", err)
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

		var uuids, instIDs []string

		for i := gpuIndx[indx]; i < gpuIndx[indx+1]; i++ {
			if matches := mdevRegexes["pciAddr"].FindStringSubmatch(lines[i]); len(matches) > 1 {
				addr = strings.TrimSpace(matches[1])
			}

			if matches := mdevRegexes["mdevUUID"].FindStringSubmatch(lines[i]); len(matches) > 1 {
				uuids = append(uuids, strings.TrimSpace(matches[1]))
			}

			if matches := mdevRegexes["gpuInstID"].FindStringSubmatch(lines[i]); len(matches) > 1 {
				instIDs = append(instIDs, strings.TrimSpace(matches[1]))
			}
		}

		for imdev := range uuids {
			gpuMdevs[addr] = append(gpuMdevs[addr], []string{uuids[imdev], instIDs[imdev]})
		}
	}

	// Loop over each mdev and add them to devs
	for addr, mdevs := range gpuMdevs {
		// Loop over all devs to find GPU that has same busID
		for idev := range devs {
			if devs[idev].CompareBusID(addr) {
				// Remove existing mdevs
				if devs[idev].migEnabled {
					for imig := range devs[idev].migInstances {
						devs[idev].migInstances[imig].mdevUUIDs = make([]string, 0)
					}
				} else {
					devs[idev].mdevUUIDs = make([]string, 0)
				}

				for _, mdev := range mdevs {
					// If MIG is enabled, loop over all MIG instances to compare instance ID
					if devs[idev].migEnabled {
						for imig := range devs[idev].migInstances {
							if strconv.FormatUint(devs[idev].migInstances[imig].gpuInstID, 10) == mdev[1] {
								// Ensure not to duplicate mdevs
								if !slices.Contains(devs[idev].migInstances[imig].mdevUUIDs, mdev[0]) {
									devs[idev].migInstances[imig].mdevUUIDs = append(devs[idev].migInstances[imig].mdevUUIDs, mdev[0])
								}
							}
						}
					} else {
						if !slices.Contains(devs[idev].mdevUUIDs, mdev[0]) {
							devs[idev].mdevUUIDs = append(devs[idev].mdevUUIDs, mdev[0])
						}
					}
				}
			}
		}
	}

	return devs, nil
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
		return BusID{domain: values[0], bus: values[1], device: values[2], function: values[3]}, nil
	}

	return BusID{}, fmt.Errorf("error parsing PCIe bus ID: %s", id)
}
