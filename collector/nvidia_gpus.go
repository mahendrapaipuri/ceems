//go:build !nonvidia
// +build !nonvidia

package collector

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const nvidiaGpuJobMapCollectorSubsystem = "nvidia_gpu"

var (
	gpuStatPath = kingpin.Flag("collector.nvidia.gpu.stat.path", "Path to gpustat file that maps GPU ordinals to job IDs.").Default("/run/gpustat").String()
)

type Device struct {
	name  string
	uuid  string
	isMig bool
}

type nvidiaGpuJobMapCollector struct {
	devices       []Device
	logger        log.Logger
	gpuJobMapDesc *prometheus.Desc
}

func init() {
	registerCollector(nvidiaGpuJobMapCollectorSubsystem, defaultDisabled, NewNvidiaGpuJobMapCollector)
}

// Get all physical or MIG devices using nvidia-smi command
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
func getAllDevices(logger log.Logger) ([]Device, error) {
	args := []string{"--query-gpu=name,uuid", "--format=csv"}
	nvidiaSmiOutput, err := Execute("nvidia-smi", args, logger)
	if err != nil {
		level.Error(logger).Log("msg", "nvidia-smi command to get list of devices failed", "err", err)
		return nil, err
	}
	allDevices := []Device{}
	for _, line := range strings.Split(string(nvidiaSmiOutput), "\n") {
		// Header line
		if strings.HasPrefix(line, "name") {
			continue
		}
		devDetails := strings.Split(line, ",")
		if len(devDetails) < 2 {
			level.Error(logger).Log("msg", "Cannot parse output from nvidia-smi command", "output", line)
			continue
		}
		devName := strings.TrimSpace(devDetails[0])
		devUuid := strings.TrimSpace(devDetails[1])
		isMig := false
		if strings.HasPrefix(devUuid, "MIG") {
			isMig = true
		}
		level.Debug(logger).Log("msg", "Found nVIDIA GPU", "name", devName, "UUID", devUuid, "isMig:", isMig)
		allDevices = append(allDevices, Device{name: devName, uuid: devUuid, isMig: isMig})
	}
	return allDevices, nil
}

// NewNvidiaGpuJobMapCollector returns a new Collector exposing batch jobs to nVIDIA GPU ordinals mapping.
func NewNvidiaGpuJobMapCollector(logger log.Logger) (Collector, error) {
	allDevices, _ := getAllDevices(logger)
	gpuJobMapDesc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, nvidiaGpuJobMapCollectorSubsystem, "jobid"),
		"Batch Job ID of current nVIDIA GPU",
		[]string{"uuid"}, nil,
	)

	collector := nvidiaGpuJobMapCollector{
		devices:       allDevices,
		logger:        logger,
		gpuJobMapDesc: gpuJobMapDesc,
	}
	return &collector, nil
}

// Update implements Collector and exposes IPMI DCMI power related metrics.
func (c *nvidiaGpuJobMapCollector) Update(ch chan<- prometheus.Metric) error {
	gpuJobMapper, _ := c.getJobId()
	for _, dev := range c.devices {
		ch <- prometheus.MustNewConstMetric(c.gpuJobMapDesc, prometheus.GaugeValue, gpuJobMapper[dev.uuid], dev.uuid)
	}
	return nil
}

// Read gpustat file and get job ID of each GPU
func (c *nvidiaGpuJobMapCollector) getJobId() (map[string]float64, error) {
	gpuJobMapper := make(map[string]float64)
	for _, dev := range c.devices {
		var jobId int64 = 0
		var slurmInfo string = fmt.Sprintf("%s/%s", *gpuStatPath, dev.uuid)

		if _, err := os.Stat(slurmInfo); err == nil {
			content, err := os.ReadFile(slurmInfo)
			if err != nil {
				level.Error(c.logger).Log("msg", "Failed to get job ID for GPU", "name", dev.uuid, "err", err)
				gpuJobMapper[dev.uuid] = float64(0)
			}
			fmt.Sscanf(string(content), "%d", &jobId)
			gpuJobMapper[dev.uuid] = float64(jobId)
		}
	}
	return gpuJobMapper, nil
}
