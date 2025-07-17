//go:build !noslurm
// +build !noslurm

package collector

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/internal/security"
	ceems_k8s "github.com/mahendrapaipuri/ceems/pkg/k8s"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"k8s.io/client-go/rest"
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

	// slurm opts.
	slurmGresConfigFile = CEEMSExporterApp.Flag(
		"collector.slurm.gres-config-file",
		"Path to SLURM's GRES configuration file.",
	).Default("/etc/slurm/gres.conf").String()

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

type gres struct {
	deviceIDs []string
	numShares uint64
}

// slurmReadProcSecurityCtxData contains the input/output data for
// reading processes inside a security context.
type slurmReadProcSecurityCtxData struct {
	procs        []procfs.Proc
	shardEnabled bool
	mpsEnabled   bool
	uuid         string
	gres         *gres
}

type slurmCollector struct {
	logger           *slog.Logger
	cgroupManager    *cgroupManager
	cgroupCollector  *cgroupCollector
	perfCollector    *perfCollector
	ebpfCollector    *ebpfCollector
	rdmaCollector    *rdmaCollector
	hostname         string
	gpuSMI           *GPUSMI
	previousJobIDs   []string
	procFS           procfs.FS
	shardEnabled     bool
	mpsEnabled       bool
	jobGpuFlag       *prometheus.Desc
	jobGpuNumSMs     *prometheus.Desc
	collectError     *prometheus.Desc
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
	cgroupManager, err := NewCgroupManager(slurm, logger)
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

	// Create a new k8s client
	client, err := ceems_k8s.New("", "", logger)
	if err != nil && !errors.Is(err, rest.ErrNotInCluster) {
		logger.Error("Failed to create k8s client", "err", err)

		return nil, err
	}

	// Instantiate a new instance of GPUSMI struct
	gpuSMI, err := NewGPUSMI(client, logger)
	if err != nil {
		logger.Error("Error creating GPU SMI instance", "err", err)
	}

	// Attempt to get GPU devices
	if err := gpuSMI.Discover(); err != nil {
		logger.Error("Error fetching GPU devices", "err", err)
	}

	// Correct GPU ordering based on CLI flag when provided
	if *slurmGPUOrdering != "" {
		gpuSMI.ReindexGPUs(*slurmGPUOrdering)

		logger.Debug("GPUs reindexed")
	}

	// Instantiate a new Proc FS
	procFS, err := procfs.NewFS(*procfsPath)
	if err != nil {
		logger.Error("Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	// Check if sharding and/or mps is enabled
	var shardEnabled bool

	var mpsEnabled bool

	if _, err := os.Stat(*slurmGresConfigFile); err == nil {
		// Read gres.conf file and split file by lines
		if out, err := os.ReadFile(*slurmGresConfigFile); err == nil {
			// If Name=shard is in the line, sharding is enabled
			gpuSMI.Devices, shardEnabled = updateGPUAvailableShares(string(out), "shard", hostname, gpuSMI.Devices)
			if shardEnabled {
				logger.Info("Sharding is enabled on GPU(s)")
			}

			// If Name=mps is in the line, mps is enabled
			gpuSMI.Devices, mpsEnabled = updateGPUAvailableShares(string(out), "mps", hostname, gpuSMI.Devices)
			if mpsEnabled {
				logger.Info("MPS is enabled on GPU(s)")
			}
		}
	}

	// Setup necessary capabilities. These are the caps we need to read
	// env vars in /proc file system to get SLURM job GPU indices
	caps, err := setupAppCaps([]string{"cap_sys_ptrace", "cap_dac_read_search"})
	if err != nil {
		logger.Warn("Failed to parse capability name(s)", "err", err)
	}

	// Setup new security context(s)
	cfg := &security.SCConfig{
		Name:         slurmReadProcCtx,
		Caps:         caps,
		Func:         readProcEnvirons,
		Logger:       logger,
		ExecNatively: disableCapAwareness,
	}

	securityCtx, err := security.NewSecurityContext(cfg)
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
		gpuSMI:           gpuSMI,
		procFS:           procFS,
		shardEnabled:     shardEnabled,
		mpsEnabled:       mpsEnabled,
		securityContexts: map[string]*security.SecurityContext{slurmReadProcCtx: securityCtx},
		jobGpuFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_index_flag"),
			"A value > 0 indicates the job using current GPU",
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
		jobGpuNumSMs: prometheus.NewDesc(
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

// Update implements Collector and update job metrics.
func (c *slurmCollector) Update(ch chan<- prometheus.Metric) error {
	// Initialise job metrics
	cgroups, err := c.jobCgroups()
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

		// Update slurm job GPU ordinals
		if len(c.gpuSMI.Devices) > 0 {
			c.updateDeviceMappers(ch)
		}
	}()

	if perfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update perf metrics
			if err := c.perfCollector.Update(ch, cgroups, slurmCollectorSubsystem); err != nil {
				c.logger.Error("Failed to update perf stats", "err", err)
			}
		}()
	}

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch, cgroups, slurmCollectorSubsystem); err != nil {
				c.logger.Error("Failed to update IO and/or network stats", "err", err)
			}
		}()
	}

	if rdmaCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update RDMA metrics
			if err := c.rdmaCollector.Update(ch, cgroups, slurmCollectorSubsystem); err != nil {
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

// updateDeviceMappers updates the device mapper metrics.
func (c *slurmCollector) updateDeviceMappers(ch chan<- prometheus.Metric) {
	// On the DCGM side, we need to use relabel magic to rename UUID
	// and GPU_I_ID labels to gpuuuid and gpuiid and make operations
	// on(gpuuuid,gpuiid)
	for _, gpu := range c.gpuSMI.Devices {
		// Update mappers for physical GPUs
		for _, unit := range gpu.ComputeUnits {
			// If sharing is available, estimate coefficient
			value := 1.0
			if gpu.CurrentShares > 0 && unit.NumShares > 0 {
				value = float64(unit.NumShares) / float64(gpu.CurrentShares)
			}

			ch <- prometheus.MustNewConstMetric(
				c.jobGpuFlag,
				prometheus.GaugeValue,
				value,
				c.cgroupManager.name,
				c.hostname,
				unit.Hostname,
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
					c.jobGpuNumSMs,
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
					c.jobGpuFlag,
					prometheus.GaugeValue,
					value,
					c.cgroupManager.name,
					c.hostname,
					unit.Hostname,
					unit.UUID,
					inst.Index,
					fmt.Sprintf("%s/gpu-%s", c.hostname, inst.Index),
					gpu.UUID,
					strconv.FormatUint(inst.GPUInstID, 10),
				)

				// For GPU instances, export number of SMs/CUs as well
				// Currently we are not using them for AMD GPUs, so they
				// will be zero.
				if inst.NumSMs > 0 {
					ch <- prometheus.MustNewConstMetric(
						c.jobGpuNumSMs,
						prometheus.GaugeValue,
						float64(inst.NumSMs),
						c.cgroupManager.name,
						c.hostname,
						unit.Hostname,
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

// updateGRESShares updates the compute unit's GRES shares based on available shares on each
// GRES resource.
func (c *slurmCollector) updateGRESShares(gresResources []*gres, jobGRESMap map[*gres]string) {
	// Make a map of global available shares
	availableShares := make(map[string]uint64)

	for _, gpu := range c.gpuSMI.Devices {
		// For MIG sliced GPUs, Index will be empty. So, set shares
		// only for physical GPUs
		if gpu.Index != "" {
			availableShares[gpu.Index] = gpu.AvailableShares
		}

		// Set shares on MIG instances
		for _, inst := range gpu.Instances {
			availableShares[inst.Index] = inst.AvailableShares
		}
	}

	// Sort gresResources based on length of deviceIDs.
	// The idea here is to satisfy the shares of the jobs that reserved a single
	// GRES resource first, then two resources, then three and so on...
	// This way we ensure that the available GRES shares are distributed correctly
	// among running units.
	slices.SortFunc(gresResources, func(a, b *gres) int {
		return cmp.Compare(len(a.deviceIDs), len(b.deviceIDs))
	})

	// Loop over each GRES resource and distribute the total shares among each
	// GRES resource in the job.
	for _, gres := range gresResources {
		for igpu, gpu := range c.gpuSMI.Devices {
			// First verify if there are jobs associated with physical GPUs
			for iunit, unit := range gpu.ComputeUnits {
				if unit.UUID == jobGRESMap[gres] {
					shares := uint64(math.Min(float64(gres.numShares), float64(availableShares[gpu.Index])))
					c.gpuSMI.Devices[igpu].ComputeUnits[iunit].NumShares = shares
					c.gpuSMI.Devices[igpu].CurrentShares += shares

					availableShares[gpu.Index] -= shares
					gres.numShares -= shares
				}
			}

			// Check the jobs associated with GPU instances
			for iinst, inst := range gpu.Instances {
				for iunit, unit := range inst.ComputeUnits {
					if unit.UUID == jobGRESMap[gres] {
						shares := uint64(math.Min(float64(gres.numShares), float64(availableShares[inst.Index])))
						c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits[iunit].NumShares = shares
						c.gpuSMI.Devices[igpu].Instances[iinst].CurrentShares += shares

						availableShares[inst.Index] -= shares
						gres.numShares -= shares
					}
				}
			}
		}
	}
}

// jobDevices updates devices with job IDs.
func (c *slurmCollector) jobDevices(cgroups []cgroup) {
	// If there are no GPU devices, nothing to do here. Return
	if len(c.gpuSMI.Devices) == 0 {
		return
	}

	// Get current job IDs on the node
	currentJobIDs := make([]string, len(cgroups))
	for icgroup, cgroup := range cgroups {
		currentJobIDs[icgroup] = cgroup.uuid
	}

	// Check if there are any new/deleted jobs between current and previous
	if areEqual(currentJobIDs, c.previousJobIDs) {
		return
	}

	// Reset job IDs in devices
	for igpu := range c.gpuSMI.Devices {
		c.gpuSMI.Devices[igpu].ResetUnits()
	}

	var gresResources []*gres

	jobGRESMap := make(map[*gres]string)

	// Iterate over all active cgroups and get job properties
	for _, cgrp := range cgroups {
		gres := c.jobGRESResources(cgrp.uuid, cgrp.procs)
		if gres == nil {
			continue
		}

		gresResources = append(gresResources, gres)
		jobGRESMap[gres] = cgrp.uuid

		// Get GPU ordinals of the job
		for _, index := range gres.deviceIDs {
			// Iterate over devices to find which device corresponds to this id
			for igpu, gpu := range c.gpuSMI.Devices {
				// If device is physical GPU
				if gpu.Index == index {
					c.gpuSMI.Devices[igpu].ComputeUnits = append(c.gpuSMI.Devices[igpu].ComputeUnits, ComputeUnit{cgrp.uuid, cgrp.hostname, 0})
				}

				// If device is instance GPU
				for iinst, inst := range gpu.Instances {
					if inst.Index == index {
						c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits = append(c.gpuSMI.Devices[igpu].Instances[iinst].ComputeUnits, ComputeUnit{cgrp.uuid, cgrp.hostname, 0})
					}
				}
			}
		}
	}

	// If sharding or MPS is enabled, we need to update shares for each job GRES resource
	if c.shardEnabled || c.mpsEnabled {
		c.updateGRESShares(gresResources, jobGRESMap)
	}

	// Update job IDs state variable
	c.previousJobIDs = currentJobIDs
}

// jobCgroups returns cgroups of active jobs.
func (c *slurmCollector) jobCgroups() ([]cgroup, error) {
	// Get current cgroups
	cgroups, err := c.cgroupManager.discover()
	if err != nil {
		return nil, fmt.Errorf("failed to discover cgroups: %w", err)
	}

	// Sometimes SLURM daemon fails to clean up cgroups for
	// terminated jobs. In that case our current cgroup slice will
	// contain terminated jobs and it is not desirable. We clean
	// up current cgroups by looking at number of procs inside each
	// cgroup. When there are no procs associated with cgroup, it is
	// terminated job
	var activeCgroups []cgroup

	var staleCgroupIDs []string

	for _, cgroup := range cgroups {
		if len(cgroup.procs) > 0 {
			activeCgroups = append(activeCgroups, cgroup)
		} else {
			staleCgroupIDs = append(staleCgroupIDs, cgroup.uuid)
		}
	}

	// If stale cgroups found, emit a warning log
	if len(staleCgroupIDs) > 0 {
		c.logger.Warn(
			"Stale cgroups without any processes found", "ids", strings.Join(staleCgroupIDs, ","),
			"num_cgroups", len(staleCgroupIDs),
		)
	}

	// Update devices
	c.jobDevices(activeCgroups)

	return activeCgroups, nil
}

// jobGRESResources returns GRES resources bound to current job.
func (c *slurmCollector) jobGRESResources(uuid string, procs []procfs.Proc) *gres {
	// Read env vars in a security context that raises necessary capabilities
	dataPtr := &slurmReadProcSecurityCtxData{
		procs:        procs,
		uuid:         uuid,
		shardEnabled: c.shardEnabled,
		mpsEnabled:   c.mpsEnabled,
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
	if len(dataPtr.gres.deviceIDs) == 0 {
		c.logger.Warn("Failed to get GPU ordinals or job does not request GPU resources", "jobid", uuid)
	} else {
		c.logger.Debug(
			"GPU ordinals", "jobid", uuid, "ordinals", strings.Join(dataPtr.gres.deviceIDs, ","),
			"shards/mps", dataPtr.gres.numShares,
		)
	}

	return dataPtr.gres
}

// readProcEnvirons reads the environment variables of processes and returns
// GPU ordinals of job. This function will be executed in a security context.
func readProcEnvirons(data interface{}) error {
	// Assert data is of slurmSecurityCtxData
	var d *slurmReadProcSecurityCtxData

	var ok bool
	if d, ok = data.(*slurmReadProcSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	var stepGPUs, jobGPUs []string

	var numShares string

	// Initialise gres to avoid accessing nil pointers downstream
	d.gres = &gres{}

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
		// If SLURM_JOB_GPUS env var is found, exit loop
		if (len(jobGPUs) > 0 && !d.shardEnabled && !d.mpsEnabled) ||
			(d.shardEnabled && len(jobGPUs) > 0 && numShares != "") ||
			(d.mpsEnabled && len(jobGPUs) > 0 && numShares != "") {
			break
		}

		// Read process environment variables
		// NOTE: This needs CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH caps
		// on the current process
		// Skip if we cannot read file or job ID env var is not found
		environments, err := proc.Environ()
		if err != nil || !slices.Contains(environments, jobIDEnv) {
			continue
		}

		// When env var entry found, get all necessary env vars
		for _, env := range environments {
			if strings.Contains(env, "SLURM_STEP_GPUS") {
				stepGPUs = strings.Split(strings.Split(env, "=")[1], ",")
			}

			if strings.Contains(env, "SLURM_JOB_GPUS") {
				jobGPUs = strings.Split(strings.Split(env, "=")[1], ",")
			}

			// When GPU sharding is enabled, number of shards allocated to a job step
			// is stored in this env var
			if strings.Contains(env, "SLURM_SHARDS_ON_NODE") {
				numShares = strings.Split(env, "=")[1]
			}

			// When MPS is enabled, number of threads allocated to a job step
			// is stored in this env var
			if strings.Contains(env, "CUDA_MPS_ACTIVE_THREAD_PERCENTAGE") {
				numShares = strings.Split(env, "=")[1]
			}
		}
	}

	// If both SLURM_STEP_GPUS and SLURM_JOB_GPUS are found, proritize
	// SLURM_JOB_GPUS. We noticed that when both env vars are found,
	// SLURM_STEP_GPUS is not correctly set.
	// Technically SLURM_JOB_GPUS should be set for jobs and SLURM_STEP_GPUS
	// should be set for steps like `srun`. When they are both set
	// SLURM_STEP_GPUS should be a subset of SLURM_JOB_GPUS but this is not
	// the case eversince we migrated to SLURM 23.11 on JZ. Maybe it is a
	// side effect of Atos' patches?
	// Relevant SLURM src: https://github.com/SchedMD/slurm/blob/d3e78848f72745ceb80e2a6bebdbcf3cfd7462b1/src/plugins/gres/common/gres_common.c#L262-L265
	switch {
	case len(jobGPUs) > 0:
		d.gres.deviceIDs = jobGPUs
	case len(stepGPUs) > 0:
		d.gres.deviceIDs = stepGPUs
	default:
		d.gres.deviceIDs = []string{}
	}

	// Convert numShares to uint64
	if val, err := strconv.ParseUint(numShares, 10, 64); err == nil {
		d.gres.numShares = val
	}

	return nil
}

// updateGPUAvailableShares parses SLURM's gres.conf file and returns updated devices slice.
func updateGPUAvailableShares(content string, gresType string, hostname string, gpus []Device) ([]Device, bool) {
	var enabled bool

	// Split file content by new line
	for _, line := range strings.Split(content, "\n") {
		// Convert to lower cases for better comparison
		line = strings.ToLower(line)

		// If Name=<gresType> is in the line, gresType is enabled
		if strings.Contains(line, "name="+gresType) {
			// If we find gres type, set enabled to true
			enabled = true

			var minors []string

			gpuInstIDs := make(map[string][]uint64)

			computeInstIDs := make(map[string][]uint64)

			var count uint64

			for _, d := range strings.Split(line, " ") {
				// Check if NodeName=compute[000-010] is available
				// If it does, check if the current hostname is in list of
				// node names.
				// NOTE that if the hostname does not match "exactly" with
				// NodeName range, this does not work!!
				if strings.Contains(d, "nodename") {
					if p := strings.Split(d, "="); len(p) >= 2 {
						if !slices.Contains(common.NodelistParser(p[1]), hostname) {
							continue
						}
					}
				}

				// Check if File= or MultipleFiles= is available
				if strings.Contains(d, "file") || strings.Contains(d, "multiplefiles") {
					if p := strings.Split(d, "="); len(p) >= 2 && len(p[1]) >= 1 {
						// Strip first character "/" from device path
						// ie make /dev/nvidia0 to dev/nvidia0
						// This is because we do splitting by ,/ as splitting
						// just by comma will split ranges as well. For examples
						// when /dev/nvidia[0,3],/dev/nvidia[1,2] is present in the file
						dps := p[1][1:]

						// When MultipleFiles= is found, we need to split by comma
						for _, dp := range strings.Split(dps, ",/") {
							// Trim all spaces
							dp = strings.TrimSpace(dp)

							// Check if device is physical GPU or MIG based on
							// device path
							switch {
							// dev/nvidia is a subset to dev/nvidia-caps so we need to make
							// sure that we are matching only dev/nvidia
							case strings.Contains(dp, "dev/nvidia") && !strings.Contains(dp, "dev/nvidia-caps"):
								// For physical GPUs, it will be /dev/nvidia0, /dev/nvidia[0-3], etc
								minorString := strings.Split(dp, "dev/nvidia")[1]
								if val, err := parseRange(strings.TrimSuffix(strings.TrimPrefix(minorString, "["), "]")); err == nil {
									minors = val
								}
							case strings.Contains(dp, "dev/nvidia-caps/nvidia-cap"):
								// For MIG backed shards, it will be File=/dev/nvidia-caps/nvidia-cap21
								migMinorString := strings.Split(dp, "dev/nvidia-caps/nvidia-cap")[1]
								if val, err := parseRange(strings.TrimSuffix(strings.TrimPrefix(migMinorString, "["), "]")); err == nil {
									for _, v := range val {
										minorString, gpuInstID, computeInstID := migInstanceIDFromDevMinor(v)
										minors = append(minors, minorString)
										gpuInstIDs[minorString] = append(gpuInstIDs[minorString], gpuInstID)
										computeInstIDs[minorString] = append(computeInstIDs[minorString], computeInstID)
									}
								}
							}
						}
					}
				}

				// Get num shards for this device
				if strings.Contains(d, "count") {
					if p := strings.Split(d, "="); len(p) >= 2 {
						if v, err := strconv.ParseUint(p[1], 10, 64); err == nil {
							count = v
						}
					}
				}
			}

			// If minors is nil, it means a global count value is
			// configured.
			if len(minors) == 0 && count > 0 {
				for igpu, gpu := range gpus {
					// If MIG is enabled, update shares of MIG instances
					if len(gpu.Instances) > 0 {
						for iinst := range gpu.Instances {
							gpus[igpu].Instances[iinst].AvailableShares = count
						}
					} else {
						gpus[igpu].AvailableShares = count
					}
				}

				return gpus, enabled
			}

			// Update available shares for discovered GPUs
			for _, minor := range minors {
				for igpu, gpu := range gpus {
					if gpu.Minor != minor {
						continue
					}

					// Update MIG instances when found
					if len(gpuInstIDs[minor]) > 0 {
						for i := range len(gpuInstIDs[minor]) {
							for iinst, inst := range gpu.Instances {
								if inst.GPUInstID == gpuInstIDs[minor][i] && inst.ComputeInstID == computeInstIDs[minor][i] {
									gpus[igpu].Instances[iinst].AvailableShares = count

									break
								}
							}
						}
					} else {
						gpus[igpu].AvailableShares = count
					}
				}
			}
		}
	}

	return gpus, enabled
}

// migInstanceIDFromDevMinor returns MIG GPU and compute instance IDs and physical device minor based
// on MIG minor number.
func migInstanceIDFromDevMinor(migMinor string) (string, uint64, uint64) {
	var gpuMinor string

	var gpuInstID uint64

	var computeInstID uint64

	if b, err := os.ReadFile(procFilePath("driver/nvidia-caps/mig-minors")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if path := strings.Split(line, " "); len(path) >= 2 && path[1] == migMinor {
				for _, p := range strings.Split(path[0], "/") {
					if strings.Contains(p, "gpu") {
						gpuMinor = strings.Split(p, "gpu")[1]
					}

					if strings.Contains(p, "gi") {
						if v, err := strconv.ParseUint(strings.Split(p, "gi")[1], 10, 64); err == nil {
							gpuInstID = v
						}
					}

					if strings.Contains(p, "ci") {
						if v, err := strconv.ParseUint(strings.Split(p, "ci")[1], 10, 64); err == nil {
							computeInstID = v
						}
					}
				}

				return gpuMinor, gpuInstID, computeInstID
			}
		}
	}

	return gpuMinor, gpuInstID, computeInstID
}
