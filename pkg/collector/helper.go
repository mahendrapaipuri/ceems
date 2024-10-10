package collector

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/prometheus/procfs"
)

type BusID struct {
	domain   uint64
	bus      uint64
	slot     uint64
	function uint64
}

// Compare compares the provided bus ID with current bus ID and
// returns true if they match and false in all other cases.
func (b *BusID) Compare(bTest BusID) bool {
	// Check equality component per component in ID
	if b.domain == bTest.domain && b.bus == bTest.bus && b.slot == bTest.slot && b.function == bTest.function {
		return true
	} else {
		return false
	}
}

// Device contains the details of GPU devices.
type Device struct {
	index  string
	name   string
	uuid   string
	busID  BusID
	isMig  bool
	isvGPU bool
}

// String implements Stringer interface of the Device struct.
func (d Device) String() string {
	return fmt.Sprintf(
		"name: %s; index: %s; uuid: %s; bus_id: %v; is_mig_instance: %t; is_vgpu_instance: %t",
		d.name, d.index, d.uuid, d.busID, d.isMig, d.isvGPU,
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

var (
	metricNameRegex = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)
	reParens        = regexp.MustCompile(`\((.*)\)`)
	pciBusIDRegex   = regexp.MustCompile(`(?P<domain>[0-9a-fA-F]+):(?P<bus>[0-9a-fA-F]+):(?P<slot>[0-9a-fA-F]+)\.(?P<function>[0-9a-fA-F]+)`)
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

// SanitizeMetricName sanitize the given metric name by replacing invalid characters by underscores.
//
// OpenMetrics and the Prometheus exposition format require the metric name
// to consist only of alphanumericals and "_", ":" and they must not start
// with digits. Since colons in MetricFamily are reserved to signal that the
// MetricFamily is the result of a calculation or aggregation of a general
// purpose monitoring system, colons will be replaced as well.
//
// Note: If not subsequently prepending a namespace and/or subsystem (e.g.,
// with prometheus.BuildFQName), the caller must ensure that the supplied
// metricName does not begin with a digit.
func SanitizeMetricName(metricName string) string {
	return metricNameRegex.ReplaceAllString(metricName, "_")
}

// GetGPUDevices returns GPU devices.
func GetGPUDevices(gpuType string, logger log.Logger) (map[int]Device, error) {
	if gpuType == "nvidia" {
		return GetNvidiaGPUDevices(*nvidiaSmiPath, logger)
	} else if gpuType == "amd" {
		return GetAMDGPUDevices(*rocmSmiPath, logger)
	}

	return nil, fmt.Errorf("unknown GPU Type %s. Only nVIDIA and AMD GPU devices are supported", gpuType)
}

// Parse nvidia-smi output and return GPU Devices map.
func parseNvidiaSmiOutput(cmdOutput string, logger log.Logger) map[int]Device {
	// Get all devices
	gpuDevices := map[int]Device{}
	devIndxInt := 0

	for _, line := range strings.Split(strings.TrimSpace(cmdOutput), "\n") {
		// Header line, empty line and newlines are ignored
		if line == "" || line == "\n" || strings.HasPrefix(line, "index") {
			continue
		}

		devDetails := strings.Split(line, ",")
		if len(devDetails) < 4 {
			continue
		}

		// Get device index, name and UUID
		devIndx := strings.TrimSpace(devDetails[0])
		devName := strings.TrimSpace(devDetails[1])
		devUUID := strings.TrimSpace(devDetails[2])
		devBusID := strings.TrimSpace(devDetails[3])

		// Parse bus ID
		busID, err := parseBusID(devBusID)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to parse GPU bus ID", "bus_id", devBusID, "err", err)
		}

		// Check if device is in MiG mode
		isMig := false
		if strings.HasPrefix(devUUID, "MIG") {
			isMig = true
		}

		gpuDevices[devIndxInt] = Device{index: devIndx, name: devName, uuid: devUUID, busID: busID, isMig: isMig}
		level.Debug(logger).Log("msg", "Found nVIDIA GPU", "gpu", gpuDevices[devIndxInt])

		devIndxInt++
	}

	return gpuDevices
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
func GetNvidiaGPUDevices(nvidiaSmiPath string, logger log.Logger) (map[int]Device, error) {
	// Check if nvidia-smi binary exists
	var nvidiaSmiCmd string

	if nvidiaSmiPath != "" {
		if _, err := os.Stat(nvidiaSmiPath); err != nil {
			return nil, err
		}

		nvidiaSmiCmd = nvidiaSmiPath
	} else {
		nvidiaSmiCmd = "nvidia-smi"
		if _, err := exec.LookPath(nvidiaSmiCmd); err != nil {
			return nil, err
		}
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--query-gpu=index,name,uuid,gpu_bus_id", "--format=csv"}

	nvidiaSmiOutput, err := osexec.Execute(nvidiaSmiCmd, args, nil)
	if err != nil {
		return nil, err
	}

	return parseNvidiaSmiOutput(string(nvidiaSmiOutput), logger), nil
}

func parseAmdSmioutput(cmdOutput string, logger log.Logger) map[int]Device {
	gpuDevices := map[int]Device{}
	devIndxInt := 0

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

		// Set isMig to false as it does not apply for AMD GPUs
		isMig := false

		gpuDevices[devIndxInt] = Device{index: devIndx, name: devName, uuid: devUUID, busID: busID, isMig: isMig}
		level.Debug(logger).Log("msg", "Found AMD GPU", "gpu", gpuDevices[devIndxInt])

		devIndxInt++
	}

	return gpuDevices
}

// GetAMDGPUDevices returns all GPU devices using rocm-smi command
// Example output:
// bash-4.4$ rocm-smi --showproductname --showserial --showbus --csv
// device,Serial Number,Card series,Card model,Card vendor,Card SKU
// card0,20170000800c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
// card1,20170003580c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
// card2,20180003050c,0000:C5:00.0,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317.
func GetAMDGPUDevices(rocmSmiPath string, logger log.Logger) (map[int]Device, error) {
	// Check if rocm-smi binary exists
	var rocmSmiCmd string

	if rocmSmiPath != "" {
		if _, err := os.Stat(rocmSmiPath); err != nil {
			return nil, err
		}

		rocmSmiCmd = rocmSmiPath
	} else {
		rocmSmiCmd = "rocm-smi"

		if _, err := exec.LookPath(rocmSmiCmd); err != nil {
			return nil, err
		}
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--showproductname", "--showserial", "--showbus", "--csv"}

	rocmSmiOutput, err := osexec.Execute(rocmSmiCmd, args, nil)
	if err != nil {
		return nil, err
	}

	return parseAmdSmioutput(string(rocmSmiOutput), logger), nil
}

// cgroupProcs returns a map of active cgroups and processes contained in each cgroup.
func cgroupProcs(fs procfs.FS, idRegex *regexp.Regexp, targetEnvVars []string, procFilter func(string) bool) (map[string][]procfs.Proc, error) {
	// Get all active procs
	allProcs, err := fs.AllProcs()
	if err != nil {
		return nil, err
	}

	// If no idRegex provided, return empty
	if idRegex == nil {
		return nil, errors.New("cgroup IDs cannot be retrieved due to empty regex")
	}

	cgroups := make(map[string][]procfs.Proc)

	for _, proc := range allProcs {
		// Get cgroup ID from regex
		var cgroupID string

		cgrps, err := proc.Cgroups()
		if err != nil || len(cgrps) == 0 {
			continue
		}

		for _, cgrp := range cgrps {
			// If cgroup path is root, skip
			if cgrp.Path == "/" {
				continue
			}

			// Unescape UTF-8 characters in cgroup path
			sanitizedPath, err := unescapeString(cgrp.Path)
			if err != nil {
				continue
			}

			cgroupIDMatches := idRegex.FindStringSubmatch(sanitizedPath)
			if len(cgroupIDMatches) <= 1 {
				continue
			}

			cgroupID = cgroupIDMatches[1]

			break
		}

		// If no cgroupID found, ignore
		if cgroupID == "" {
			continue
		}

		// If targetEnvVars is not empty check if this env vars is present for the process
		// We dont check for the value of env var. Presence of env var is enough to
		// trigger the profiling of that process
		if len(targetEnvVars) > 0 {
			environ, err := proc.Environ()
			if err != nil {
				continue
			}

			for _, env := range environ {
				for _, targetEnvVar := range targetEnvVars {
					if strings.HasPrefix(env, targetEnvVar) {
						goto check_process
					}
				}
			}

			// If target env var(s) is not found, return
			continue
		}

	check_process:
		// Ignore processes where command line matches the regex
		if procFilter != nil {
			procCmdLine, err := proc.CmdLine()
			if err != nil || len(procCmdLine) == 0 {
				continue
			}

			// Ignore process if matches found
			if procFilter(strings.Join(procCmdLine, " ")) {
				continue
			}
		}

		cgroups[cgroupID] = append(cgroups[cgroupID], proc)
	}

	return cgroups, nil
}

// fileExists checks if given file exists or not.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

// lookPath is like exec.LookPath but looks only in /sbin, /usr/sbin,
// /usr/local/sbin which are reserved for super user.
func lookPath(f string) (string, error) {
	locations := []string{
		"/sbin",
		"/usr/sbin",
		"/usr/local/sbin",
	}

	for _, path := range locations {
		fullPath := filepath.Join(path, f)
		if fileExists(fullPath) {
			return fullPath, nil
		}
	}

	return "", errors.New("path does not exist")
}

// inode returns the inode of a given path.
func inode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("error running stat(%s): %w", path, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("missing syscall.Stat_t in FileInfo for %s", path)
	}

	return stat.Ino, nil
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
		return BusID{domain: values[0], bus: values[1], slot: values[2], function: values[3]}, nil
	}

	return BusID{}, fmt.Errorf("error parsing PCIe bus ID: %s", id)
}

// unescapeString sanitizes the string by unescaping UTF-8 characters.
func unescapeString(s string) (string, error) {
	sanitized, err := strconv.Unquote("\"" + s + "\"")
	if err != nil {
		return "", err
	}

	return sanitized, nil
}

// readUintFromFile reads a file and attempts to parse a uint64 from it.
func readUintFromFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}
