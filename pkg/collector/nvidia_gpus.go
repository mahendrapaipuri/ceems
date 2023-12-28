//go:build !nonvidia
// +build !nonvidia

package collector

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/internal/helpers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const nvidiaGpuJobMapCollectorSubsystem = "nvidia_gpu"

var (
	jobMapLock    = sync.RWMutex{}
	nvidiaSmiPath = BatchJobExporterApp.Flag(
		"collector.nvidia.smi.path",
		"Absolute path to nvidia-smi executable.",
	).Default("/usr/bin/nvidia-smi").String()
	gpuStatPath = BatchJobExporterApp.Flag(
		"collector.nvidia.gpu.job.map.path",
		"Path to file that maps GPU ordinals to job IDs.",
	).Default("/run/gpujobmap").String()
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
	RegisterCollector(
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
	// Check if nvidia-smi binary exists
	if _, err := os.Stat(*nvidiaSmiPath); err != nil {
		level.Error(logger).Log("msg", "Failed to open nvidia-smi executable", "path", *nvidiaSmiPath, "err", err)
		return nil, err
	}

	// Execute nvidia-smi command to get available GPUs
	args := []string{"--query-gpu=index,name,uuid", "--format=csv"}
	nvidiaSmiOutput, err := helpers.Execute(*nvidiaSmiPath, args, logger)
	if err != nil {
		level.Error(logger).
			Log("msg", "nvidia-smi command to get list of devices failed", "err", err)
		return nil, err
	}

	// Get all devices
	allDevices := []Device{}
	for _, line := range strings.Split(string(nvidiaSmiOutput), "\n") {
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
		prometheus.BuildFQName(Namespace, nvidiaGpuJobMapCollectorSubsystem, "jobid"),
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
			//
			// Instantiate a new Proc FS
			procFS, err := procfs.NewFS(*procfsPath)
			if err != nil {
				level.Error(c.logger).Log("msg", "Unable to open procfs", "path", *procfsPath)
				goto outside
			}

			// Get all procs from current proc fs
			allProcs, err := procFS.AllProcs()
			if err != nil {
				level.Error(c.logger).Log("msg", "Failed to read /proc", "err", err)
				gpuJobMapper[dev.uuid] = float64(0)

				// If we cannot read procfs break
				goto outside
			}

			// Initialize a waitgroup for all go routines that we will spawn later
			wg := &sync.WaitGroup{}
			wg.Add(len(allProcs))

			// Iterate through all procs and look for SLURM_JOB_ID env entry
			for _, proc := range allProcs {
				go func(p procfs.Proc) {
					// Read process environment variables
					// NOTE: This needs CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH caps
					// on the current process
					environments, err := p.Environ()

					// Skip if we cannot read file
					if err != nil {
						wg.Done()
						return
					}

					var gpuIndices []string
					var slurmJobId string = ""

					// Loop through all env vars and get SLURM_SETP_GPUS/SLURM_JOB_GPUS
					// and SLURM_JOB_ID
					for _, env := range environments {
						// Check both SLURM_SETP_GPUS and SLURM_JOB_GPUS vars
						if strings.Contains(env, "SLURM_STEP_GPUS") || strings.Contains(env, "SLURM_JOB_GPUS") {
							gpuIndices = strings.Split(strings.Split(env, "=")[1], ",")
						}
						if strings.Contains(env, "SLURM_JOB_ID") {
							slurmJobId = strings.Split(env, "=")[1]
						}
					}

					// If gpuIndices has current GPU index, assign the jobID and break loop
					if slices.Contains(gpuIndices, dev.index) {
						jobMapLock.Lock()
						jid, err := strconv.Atoi(slurmJobId)
						if err != nil {
							gpuJobMapper[dev.uuid] = float64(0)
						}
						gpuJobMapper[dev.uuid] = float64(jid)
						jobMapLock.Unlock()
					}

					// Mark routine as done
					wg.Done()

				}(proc)
			}

			// Wait for all go routines
			wg.Wait()
		}
	outside:
		if gpuJobMapper[dev.uuid] == 0 {
			level.Error(c.logger).Log(
				"msg", "Failed to get job ID for GPU", "index", dev.index, "uuid", dev.uuid,
			)
		} else {
			level.Debug(c.logger).Log(
				"msg", "Foung job ID for GPU", "index", dev.index, "uuid", dev.uuid,
				"jobid", gpuJobMapper[dev.uuid],
			)
		}
	}
	return gpuJobMapper, nil
}
