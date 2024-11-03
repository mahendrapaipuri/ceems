//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const (
	slurmCollectorSubsystem = "slurm"
)

// CLI opts.
var (
	// cgroup opts.
	slurmCollectSwapMemoryStatsDepre = CEEMSExporterApp.Flag(
		"collector.slurm.swap.memory.metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Hidden().Bool()
	slurmCollectSwapMemoryStats = CEEMSExporterApp.Flag(
		"collector.slurm.swap-memory-metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Bool()
	slurmCollectPSIStatsDepre = CEEMSExporterApp.Flag(
		"collector.slurm.psi.metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Hidden().Bool()
	slurmCollectPSIStats = CEEMSExporterApp.Flag(
		"collector.slurm.psi-metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Bool()

	// GPU opts.
	slurmGPUOrdering = CEEMSExporterApp.Flag(
		"collector.slurm.gpu-order-map",
		`GPU order mapping between SLURM and NVIDIA SMI/ROCm SMI tools. 
It should be of format <slurm_gpu_index>: <nvidia_or_rocm_smi_index>[.<mig_gpu_instance_id>] delimited by ",".`,
	).Default("").PlaceHolder("0:1,1:0.3,2:0.4,3:0.5,4:0.6").String()
)

// Security context names.
const (
	slurmReadProcCtx = "slurm_read_procs"
)

// slurmReadProcSecurityCtxData contains the input/output data for
// reading processes inside a security context.
type slurmReadProcSecurityCtxData struct {
	procs       []procfs.Proc
	uuid        string
	gpuOrdinals []string
}

// jobProps contains SLURM job properties.
type jobProps struct {
	uuid        string   // This is SLURM's job ID
	gpuOrdinals []string // GPU ordinals bound to job
}

// emptyGPUOrdinals returns true if gpuOrdinals is empty.
func (p *jobProps) emptyGPUOrdinals() bool {
	return len(p.gpuOrdinals) == 0
}

type slurmMetrics struct {
	cgMetrics []cgMetric
	jobProps  []jobProps
	cgroups   []cgroup
}

type slurmCollector struct {
	logger           *slog.Logger
	cgroupManager    *cgroupManager
	cgroupCollector  *cgroupCollector
	perfCollector    *perfCollector
	ebpfCollector    *ebpfCollector
	rdmaCollector    *rdmaCollector
	hostname         string
	gpuDevs          []Device
	procFS           procfs.FS
	jobGpuFlag       *prometheus.Desc
	collectError     *prometheus.Desc
	jobPropsCache    map[string]jobProps
	securityContexts map[string]*security.SecurityContext
}

func init() {
	RegisterCollector(slurmCollectorSubsystem, defaultDisabled, NewSlurmCollector)
}

// NewSlurmCollector returns a new Collector exposing a summary of cgroups.
func NewSlurmCollector(logger *slog.Logger) (Collector, error) {
	// Log deprecation notices
	if *slurmCollectPSIStatsDepre {
		logger.Warn("flag --collector.slurm.psi.metrics has been deprecated. Use --collector.slurm.psi-metrics instead")
	}

	if *slurmCollectSwapMemoryStatsDepre {
		logger.Warn("flag --collector.slurm.swap.memory.metrics has been deprecated. Use --collector.slurm.swap-memory-metrics instead")
	}

	// Get SLURM's cgroup details
	cgroupManager, err := NewCgroupManager("slurm", logger)
	if err != nil {
		logger.Info("Failed to create cgroup manager", "err", err)

		return nil, err
	}

	logger.Info("cgroup: " + cgroupManager.String())

	// Set cgroup options
	opts := cgroupOpts{
		collectSwapMemStats: *slurmCollectSwapMemoryStatsDepre || *slurmCollectSwapMemoryStats,
		collectPSIStats:     *slurmCollectPSIStatsDepre || *slurmCollectPSIStats,
		collectBlockIOStats: false, // SLURM does not support blkio controller.
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
			logger.Info("gpu: " + gpuType)

			break
		}
	}

	// Correct GPU ordering based on CLI flag when provided
	if *slurmGPUOrdering != "" {
		gpuDevs = reindexGPUs(*slurmGPUOrdering, gpuDevs)

		logger.Debug("GPUs reindexed")
	}

	// Instantiate a new Proc FS
	procFS, err := procfs.NewFS(*procfsPath)
	if err != nil {
		logger.Error("Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	// Setup necessary capabilities. These are the caps we need to read
	// env vars in /proc file system to get SLURM job GPU indices
	caps := setupCollectorCaps(logger, slurmCollectorSubsystem, []string{"cap_sys_ptrace", "cap_dac_read_search"})

	// Setup new security context(s)
	securityCtx, err := security.NewSecurityContext(slurmReadProcCtx, caps, readProcEnvirons, logger)
	if err != nil {
		logger.Error("Failed to create a security context", "err", err)

		return nil, err
	}

	return &slurmCollector{
		cgroupManager:    cgroupManager,
		cgroupCollector:  cgCollector,
		perfCollector:    perfCollector,
		ebpfCollector:    ebpfCollector,
		rdmaCollector:    rdmaCollector,
		hostname:         hostname,
		gpuDevs:          gpuDevs,
		procFS:           procFS,
		jobPropsCache:    make(map[string]jobProps),
		securityContexts: map[string]*security.SecurityContext{slurmReadProcCtx: securityCtx},
		jobGpuFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_index_flag"),
			"A value > 0 indicates the job using current GPU",
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

// Update implements Collector and update job metrics.
func (c *slurmCollector) Update(ch chan<- prometheus.Metric) error {
	// Initialise job metrics
	metrics, err := c.jobMetrics()
	if err != nil {
		return err
	}

	// Start a wait group
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Update cgroup metrics
		if err := c.cgroupCollector.Update(ch, metrics.cgMetrics); err != nil {
			c.logger.Error("Failed to update cgroup stats", "err", err)
		}

		// Update slurm job GPU ordinals
		if len(c.gpuDevs) > 0 {
			c.updateGPUOrdinals(ch, metrics.jobProps)
		}
	}()

	if perfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update perf metrics
			if err := c.perfCollector.Update(ch, metrics.cgroups); err != nil {
				c.logger.Error("Failed to update perf stats", "err", err)
			}
		}()
	}

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch, metrics.cgroups); err != nil {
				c.logger.Error("Failed to update IO and/or network stats", "err", err)
			}
		}()
	}

	if rdmaCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update RDMA metrics
			if err := c.rdmaCollector.Update(ch, metrics.cgroups); err != nil {
				c.logger.Error("Failed to update RDMA stats", "err", err)
			}
		}()
	}

	// Wait for all go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *slurmCollector) Stop(ctx context.Context) error {
	c.logger.Debug("Stopping", "collector", slurmCollectorSubsystem)

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

// updateGPUOrdinals updates the metrics channel with GPU ordinals for SLURM job.
func (c *slurmCollector) updateGPUOrdinals(ch chan<- prometheus.Metric, jobProps []jobProps) {
	// Update slurm job properties
	for _, p := range jobProps {
		// GPU job mapping
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
						flagValue = mig.smFraction

						goto update_chan
					}
				}

				if gpuOrdinal == dev.globalIndex {
					gpuuuid = dev.uuid

					goto update_chan
				}
			}

		update_chan:
			// We set label of gpuuuid of format <gpu_uuid>/<mig_instance_id>
			// On the DCGM side, we need to use relabel magic to merge UUID
			// and GPU_I_ID labels and set them exactly as <uuid>/<gpu_i_id>
			// as well
			ch <- prometheus.MustNewConstMetric(
				c.jobGpuFlag,
				prometheus.GaugeValue,
				flagValue,
				c.cgroupManager.manager,
				c.hostname,
				p.uuid,
				gpuOrdinal,
				fmt.Sprintf("%s/gpu-%s", c.hostname, gpuOrdinal),
				fmt.Sprintf("%s/%s", gpuuuid, miggid),
			)
		}
	}
}

// jobProperties finds job properties for each active cgroup and returns initialised metric structs.
func (c *slurmCollector) jobProperties(cgroups []cgroup) slurmMetrics {
	// Get currently active jobs and set them in activeJobs state variable
	var activeJobUUIDs []string

	var jProps []jobProps

	var cgMetrics []cgMetric

	var gpuOrdinals []string

	// Iterate over all active cgroups and get job properties
	for _, cgrp := range cgroups {
		jobuuid := cgrp.uuid

		// Get GPU ordinals of the job
		if len(c.gpuDevs) > 0 {
			if jobPropsCached, ok := c.jobPropsCache[jobuuid]; !ok || (ok && jobPropsCached.emptyGPUOrdinals()) {
				gpuOrdinals = c.gpuOrdinals(jobuuid, cgrp.procs)
				c.jobPropsCache[jobuuid] = jobProps{uuid: jobuuid, gpuOrdinals: gpuOrdinals}
				jProps = append(jProps, c.jobPropsCache[jobuuid])
			} else {
				jProps = append(jProps, c.jobPropsCache[jobuuid])
			}
		}

		// Check if we already passed through this job
		if !slices.Contains(activeJobUUIDs, jobuuid) {
			activeJobUUIDs = append(activeJobUUIDs, jobuuid)
		}

		// Add to cgroups only if it is a root cgroup
		cgMetrics = append(cgMetrics, cgMetric{uuid: jobuuid, path: "/" + cgrp.path.rel})
	}

	// Remove expired jobs from jobPropsCache
	for uuid := range c.jobPropsCache {
		if !slices.Contains(activeJobUUIDs, uuid) {
			delete(c.jobPropsCache, uuid)
		}
	}

	return slurmMetrics{cgMetrics: cgMetrics, jobProps: jProps, cgroups: cgroups}
}

// jobMetrics returns initialised metric structs.
func (c *slurmCollector) jobMetrics() (slurmMetrics, error) {
	// Get active cgroups
	cgroups, err := c.cgroupManager.discover()
	if err != nil {
		return slurmMetrics{}, fmt.Errorf("failed to discover cgroups: %w", err)
	}

	// Get job properties and initialise metric structs
	return c.jobProperties(cgroups), nil
}

// gpuOrdinals returns GPU ordinals bound to current job.
func (c *slurmCollector) gpuOrdinals(uuid string, procs []procfs.Proc) []string {
	var gpuOrdinals []string

	// Read env vars in a security context that raises necessary capabilities
	dataPtr := &slurmReadProcSecurityCtxData{
		procs: procs,
		uuid:  uuid,
	}

	if securityCtx, ok := c.securityContexts[slurmReadProcCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			c.logger.Error(
				"Failed to run inside security contxt", "jobid", uuid, "err", err,
			)

			return nil
		}
	} else {
		c.logger.Error(
			"Security context not found", "name", slurmReadProcCtx, "jobid", uuid,
		)

		return nil
	}

	// Emit warning when there are GPUs but no job to GPU map found
	if len(dataPtr.gpuOrdinals) == 0 {
		c.logger.Warn("Failed to get GPU ordinals for job", "jobid", uuid)
	} else {
		c.logger.Debug(
			"GPU ordinals", "jobid", uuid, "ordinals", strings.Join(gpuOrdinals, ","),
		)
	}

	return dataPtr.gpuOrdinals
}

// readProcEnvirons reads the environment variables of processes and returns
// GPU ordinals of job. This function will be executed in a security context.
func readProcEnvirons(data interface{}) error {
	// Assert data is of slurmSecurityCtxData
	var d *slurmReadProcSecurityCtxData

	var ok bool
	if d, ok = data.(*slurmReadProcSecurityCtxData); !ok {
		return errors.New("data type cannot be asserted")
	}

	var gpuOrdinals []string

	// Env var that we will search
	jobIDEnv := "SLURM_JOB_ID=" + d.uuid

	// Iterate through all procs and look for SLURM_JOB_ID env entry
	// Here we have to sacrifice multi-threading for security. We cannot
	// spawn go-routines inside as we will execute this function inside
	// a security context locked to OS thread. Any new go routines spawned
	// WILL NOT BE scheduled on this locked thread and hence will not
	// have capabilities to read environment variables. So, we just do
	// old school loop on procs and attempt to find target env variables.
	for _, proc := range d.procs {
		// Read process environment variables
		// NOTE: This needs CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH caps
		// on the current process
		// Skip if we cannot read file or job ID env var is not found
		environments, err := proc.Environ()
		if err != nil || !slices.Contains(environments, jobIDEnv) {
			continue
		}

		// When env var entry found, get all necessary env vars
		// NOTE: This is not really concurrent safe. Multiple go routines might
		// overwrite the variables. But I think we can live with it as for a gievn
		// job cgroup these env vars should be identical in all procs
		for _, env := range environments {
			if strings.Contains(env, "SLURM_STEP_GPUS") || strings.Contains(env, "SLURM_JOB_GPUS") {
				gpuOrdinals = strings.Split(strings.Split(env, "=")[1], ",")

				goto outside
			}
		}
	}

outside:

	// Set found gpuOrdinals on ctxData
	d.gpuOrdinals = gpuOrdinals

	return nil
}
