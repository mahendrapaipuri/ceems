package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"regexp"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/osexec"
)

type Device struct {
	index string
	name  string
	uuid  string
	isMig bool
}

var (
	metricNameRegex = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)
	reParens        = regexp.MustCompile(`\((.*)\)`)
)

// Check if file exists
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// Find named matches in regex groups and return a map
func findNamedMatches(regex *regexp.Regexp, str string) map[string]string {
	match := regex.FindStringSubmatch(str)

	results := map[string]string{}
	for i, name := range match {
		results[regex.SubexpNames()[i]] = name
	}
	return results
}

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

// LoadCgroupsV2Metrics returns cgroup metrics from a given path
func LoadCgroupsV2Metrics(
	name string,
	cgroupfsPath string,
	controllers []string,
) (map[string]float64, error) {
	data := make(map[string]float64)

	for _, fName := range controllers {
		contents, err := os.ReadFile(filepath.Join(cgroupfsPath, name, fName))
		if err != nil {
			return data, err
		}
		for _, line := range strings.Split(string(contents), "\n") {
			// Some of the above have a single value and others have a "data_name 123"
			parts := strings.Fields(line)
			indName := fName
			indData := 0
			if len(parts) == 1 || len(parts) == 2 {
				if len(parts) == 2 {
					indName += "." + parts[0]
					indData = 1
				}
				if parts[indData] == "max" {
					data[indName] = -1.0
				} else {
					f, err := strconv.ParseFloat(parts[indData], 64)
					if err == nil {
						data[indName] = f
					} else {
						return data, err
					}
				}
			}
		}
	}
	return data, nil
}

// GetGPUDevices returns GPU devices
func GetGPUDevices(gpuType string, logger log.Logger) (map[int]Device, error) {
	if gpuType == "nvidia" {
		return GetNvidiaGPUDevices(*nvidiaSmiPath, logger)
	} else if gpuType == "amd" {
		return GetAMDGPUDevices(*rocmSmiPath, logger)
	}
	return nil, fmt.Errorf("unknown GPU Type %s. Only nVIDIA and AMD GPU devices are supported", gpuType)
}

// Parse nvidia-smi output and return GPU Devices map
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
		if len(devDetails) < 3 {
			level.Error(logger).
				Log("msg", "Cannot parse output from nvidia-smi command", "output", line)
			continue
		}

		// Get device index, name and UUID
		devIndx := strings.TrimSpace(devDetails[0])
		devName := strings.TrimSpace(devDetails[1])
		devUUID := strings.TrimSpace(devDetails[2])

		// Check if device is in MiG mode
		isMig := false
		if strings.HasPrefix(devUUID, "MIG") {
			isMig = true
		}
		level.Debug(logger).
			Log("msg", "Found nVIDIA GPU", "name", devName, "UUID", devUUID, "isMig:", isMig)

		gpuDevices[devIndxInt] = Device{index: devIndx, name: devName, uuid: devUUID, isMig: isMig}
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
// NOTE: Hoping this command returns MIG devices too
func GetNvidiaGPUDevices(nvidiaSmiPath string, logger log.Logger) (map[int]Device, error) {
	// Check if nvidia-smi binary exists
	var nvidiaSmiCmd string
	if nvidiaSmiPath != "" {
		if _, err := os.Stat(nvidiaSmiPath); err != nil {
			level.Error(logger).Log("msg", "Failed to open nvidia-smi executable", "path", nvidiaSmiPath, "err", err)
			return nil, err
		}
		nvidiaSmiCmd = nvidiaSmiPath
	} else {
		nvidiaSmiCmd = "nvidia-smi"
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--query-gpu=index,name,uuid", "--format=csv"}
	nvidiaSmiOutput, err := osexec.Execute(nvidiaSmiCmd, args, nil, logger)
	if err != nil {
		level.Error(logger).
			Log("msg", "nvidia-smi command to get list of devices failed", "err", err)
		return nil, err
	}
	// Get all devices
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
		if len(devDetails) < 6 {
			level.Error(logger).
				Log("msg", "Cannot parse output from rocm-smi command", "output", line)
			continue
		}

		// Get device index, name and UUID
		devIndx := strings.TrimPrefix(devDetails[0], "card")
		devUUID := strings.TrimSpace(devDetails[1])
		devName := strings.TrimSpace(devDetails[2])

		// Set isMig to false as it does not apply for AMD GPUs
		isMig := false
		level.Debug(logger).
			Log("msg", "Found AMD GPU", "name", devName, "UUID", devUUID)

		gpuDevices[devIndxInt] = Device{index: devIndx, name: devName, uuid: devUUID, isMig: isMig}
		devIndxInt++
	}
	return gpuDevices
}

// GetAMDGPUDevices returns all GPU devices using rocm-smi command
// Example output:
// bash-4.4$ rocm-smi --showproductname --showserial --csv
// device,Serial Number,Card series,Card model,Card vendor,Card SKU
// card0,20170000800c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
// card1,20170003580c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
// card2,20180003050c,deon Instinct MI50 32GB,0x0834,Advanced Micro Devices Inc. [AMD/ATI],D16317
func GetAMDGPUDevices(rocmSmiPath string, logger log.Logger) (map[int]Device, error) {
	// Check if rocm-smi binary exists
	var rocmSmiCmd string
	if rocmSmiPath != "" {
		if _, err := os.Stat(rocmSmiPath); err != nil {
			level.Error(logger).Log("msg", "Failed to open rocm-smi executable", "path", rocmSmiPath, "err", err)
			return nil, err
		}
		rocmSmiCmd = rocmSmiPath
	} else {
		rocmSmiCmd = "rocm-smi"
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--showproductname", "--showserial", "--csv"}
	rocmSmiOutput, err := osexec.Execute(rocmSmiCmd, args, nil, logger)
	if err != nil {
		level.Error(logger).
			Log("msg", "rocm-smi command to get list of devices failed", "err", err)
		return nil, err
	}
	// Get all devices
	return parseAmdSmioutput(string(rocmSmiOutput), logger), nil
}
