//go:build !nonvidia
// +build !nonvidia

package collector

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/internal/helpers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const nvidiaGpuJobMapCollectorSubsystem = "nvidia_gpu"

var (
	gpuStatPath = kingpin.Flag(
		"collector.nvidia.gpu.stat.path",
		"Path to gpustat file that maps GPU ordinals to job IDs.",
	).Default("/run/gpustat").String()
)

type Device struct {
	index string
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
	registerCollector(
		nvidiaGpuJobMapCollectorSubsystem,
		defaultDisabled,
		NewNvidiaGpuJobMapCollector,
	)
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
	args := []string{"--query-gpu=index,name,uuid", "--format=csv"}
	nvidiaSmiOutput, err := helpers.Execute("nvidia-smi", args, logger)
	if err != nil {
		level.Error(logger).
			Log("msg", "nvidia-smi command to get list of devices failed", "err", err)
		return nil, err
	}

	// Get all devices
	allDevices := []Device{}
	for _, line := range strings.Split(string(nvidiaSmiOutput), "\n") {
		// Header line
		if strings.HasPrefix(line, "index") {
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
		devUuid := strings.TrimSpace(devDetails[2])

		// Check if device is in MiG mode
		isMig := false
		if strings.HasPrefix(devUuid, "MIG") {
			isMig = true
		}
		level.Debug(logger).
			Log("msg", "Found nVIDIA GPU", "name", devName, "UUID", devUuid, "isMig:", isMig)

		allDevices = append(allDevices, Device{index: devIndx, name: devName, uuid: devUuid, isMig: isMig})
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
		var slurmInfo string = fmt.Sprintf("%s/%s", *gpuStatPath, dev.index)

		// NOTE: Look for file name with UUID as it will be more appropriate with
		// MIG instances.
		// If /run/gpustat/0 file is not found, check for the file with UUID as name?
		if _, err := os.Stat(slurmInfo); err == nil {
			content, err := os.ReadFile(slurmInfo)
			if err != nil {
				level.Error(c.logger).Log(
					"msg", "Failed to get job ID for GPU",
					"index", dev.index, "uuid", dev.uuid, "err", err,
				)
				gpuJobMapper[dev.uuid] = float64(0)
			}
			fmt.Sscanf(string(content), "%d", &jobId)
			gpuJobMapper[dev.uuid] = float64(jobId)
		} else {
			// Attempt to get GPU dev indices from /proc file system by looking into
			// environ for the process that has same SLURM_JOB_ID
			allProcs, err := procfs.AllProcs()
			if err != nil {
				level.Error(c.logger).Log("msg", "Failed to read /proc", "err", err)
				gpuJobMapper[dev.uuid] = float64(0)

				// If we cannot read procfs break
				goto outside
			}

			// Iterate through all procs and look for SLURM_JOB_ID env entry
			for _, proc := range allProcs {
				environments, err := proc.Environ()
				if err != nil {
					continue
				}

				var gpuIndices []string
				var slurmJobId string = ""

				// Loop through all env vars and get SLURM_SETP_GPUS/SLURM_JOB_GPUS
				// and SLURM_JOB_ID
				for _, env := range environments {
					// Check both SLURM_SETP_GPUS and SLURM_JOB_GPUS vars and only when
					// gpuIndices is empty.
					// We dont want an empty env var to override already populated
					// gpuIndices slice
					if (strings.Contains(env, "SLURM_STEP_GPUS") || strings.Contains(env, "SLURM_JOBS_GPUS")) && len(gpuIndices) == 0 {
						gpuIndices = strings.Split(strings.Split(env, "=")[1], ",")
					}
					if strings.Contains(env, "SLURM_JOB_ID") {
						slurmJobId = strings.Split(env, "=")[1]
					}
				}

				// If gpuIndices has current GPU index, assign the jobID and break loop
				if slices.Contains(gpuIndices, dev.index) {
					jid, err := strconv.Atoi(slurmJobId)
					if err != nil {
						gpuJobMapper[dev.uuid] = float64(0)
					}
					gpuJobMapper[dev.uuid] = float64(jid)
					goto outside
				}
			}
		}
	outside:
		level.Debug(c.logger).Log(
			"msg", "Foung job ID for GPU", "index", dev.index, "uuid", dev.uuid,
			"jobid", gpuJobMapper[dev.uuid],
		)
	}
	return gpuJobMapper, nil
}
