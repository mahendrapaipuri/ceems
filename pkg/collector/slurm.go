//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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
	slurmGPUStatPath = CEEMSExporterApp.Flag(
		"collector.slurm.gpu-job-map-path",
		"Path to directory that maps GPU ordinals to job IDs.",
	).Default("").String()
)

// Security context names.
const (
	slurmReadProcCtx = "slurm_read_procs"
)

// slurmReadProcSecurityCtxData contains the input/output data for
// reading processes inside a security context.
type slurmReadProcSecurityCtxData struct {
	procfs      procfs.FS
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
}

type slurmCollector struct {
	logger           log.Logger
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
func NewSlurmCollector(logger log.Logger) (Collector, error) {
	// Log deprecation notices
	if *slurmCollectPSIStatsDepre {
		level.Warn(logger).
			Log("msg", "flag --collector.slurm.psi.metrics has been deprecated. Use --collector.slurm.psi-metrics instead")
	}

	if *slurmCollectSwapMemoryStatsDepre {
		level.Warn(logger).
			Log("msg", "flag --collector.slurm.swap.memory.metrics has been deprecated. Use --collector.slurm.swap-memory-metrics instead")
	}

	// Get SLURM's cgroup details
	cgroupManager, err := NewCgroupManager("slurm")
	if err != nil {
		level.Info(logger).Log("msg", "Failed to create cgroup manager", "err", err)

		return nil, err
	}

	level.Info(logger).Log("cgroup", cgroupManager)

	// Set cgroup options
	opts := cgroupOpts{
		collectSwapMemStats: *slurmCollectSwapMemoryStatsDepre || *slurmCollectSwapMemoryStats,
		collectPSIStats:     *slurmCollectPSIStatsDepre || *slurmCollectPSIStats,
		collectBlockIOStats: false, // SLURM does not support blkio controller.
	}

	// Start new instance of cgroupCollector
	cgCollector, err := NewCgroupCollector(log.With(logger, "sub_collector", "cgroup"), cgroupManager, opts)
	if err != nil {
		level.Info(logger).Log("msg", "Failed to create cgroup collector", "err", err)

		return nil, err
	}

	// Start new instance of perfCollector
	var perfCollector *perfCollector

	if perfCollectorEnabled() {
		perfCollector, err = NewPerfCollector(log.With(logger, "sub_collector", "perf"), cgroupManager)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create perf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of ebpfCollector
	var ebpfCollector *ebpfCollector

	if ebpfCollectorEnabled() {
		ebpfCollector, err = NewEbpfCollector(log.With(logger, "sub_collector", "ebpf"), cgroupManager)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create ebpf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of rdmaCollector
	var rdmaCollector *rdmaCollector

	if rdmaCollectorEnabled() {
		rdmaCollector, err = NewRDMACollector(log.With(logger, "sub_collector", "rdma"), cgroupManager)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create RDMA collector", "err", err)

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
			level.Info(logger).Log("gpu", gpuType)

			break
		}
	}

	// Correct GPU ordering based on CLI flag when provided
	if *slurmGPUOrdering != "" {
		gpuDevs = reindexGPUs(*slurmGPUOrdering, gpuDevs)

		level.Debug(logger).Log("msg", "GPU reindexed based")
	}

	// Instantiate a new Proc FS
	procFS, err := procfs.NewFS(*procfsPath)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	// Setup necessary capabilities. These are the caps we need to read
	// env vars in /proc file system to get SLURM job GPU indices
	caps := setupCollectorCaps(logger, slurmCollectorSubsystem, []string{"cap_sys_ptrace", "cap_dac_read_search"})

	// Setup new security context(s)
	securityCtx, err := security.NewSecurityContext(slurmReadProcCtx, caps, readProcEnvirons, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create a security context", "err", err)

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
	// Discover all active cgroups
	metrics, err := c.discoverCgroups()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNoData, err)
	}

	// Start a wait group
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Update cgroup metrics
		if err := c.cgroupCollector.Update(ch, metrics.cgMetrics); err != nil {
			level.Error(c.logger).Log("msg", "Failed to update cgroup stats", "err", err)
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
			if err := c.perfCollector.Update(ch, nil); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update perf stats", "err", err)
			}
		}()
	}

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch, nil); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update IO and/or network stats", "err", err)
			}
		}()
	}

	if rdmaCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update RDMA metrics
			if err := c.rdmaCollector.Update(ch, nil); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update RDMA stats", "err", err)
			}
		}()
	}

	// Wait for all go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *slurmCollector) Stop(ctx context.Context) error {
	level.Debug(c.logger).Log("msg", "Stopping", "collector", slurmCollectorSubsystem)

	// Stop all sub collectors
	// Stop cgroupCollector
	if err := c.cgroupCollector.Stop(ctx); err != nil {
		level.Error(c.logger).Log("msg", "Failed to stop cgroup collector", "err", err)
	}

	// Stop perfCollector
	if perfCollectorEnabled() {
		if err := c.perfCollector.Stop(ctx); err != nil {
			level.Error(c.logger).Log("msg", "Failed to stop perf collector", "err", err)
		}
	}

	// Stop ebpfCollector
	if ebpfCollectorEnabled() {
		if err := c.ebpfCollector.Stop(ctx); err != nil {
			level.Error(c.logger).Log("msg", "Failed to stop ebpf collector", "err", err)
		}
	}

	// Stop rdmaCollector
	if rdmaCollectorEnabled() {
		if err := c.rdmaCollector.Stop(ctx); err != nil {
			level.Error(c.logger).Log("msg", "Failed to stop RDMA collector", "err", err)
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

// discoverCgroups finds active cgroup paths and returns initialised metric structs.
func (c *slurmCollector) discoverCgroups() (slurmMetrics, error) {
	// Get currently active jobs and set them in activeJobs state variable
	var activeJobUUIDs []string

	var jProps []jobProps

	var cgMetrics []cgMetric

	var gpuOrdinals []string

	// Walk through all cgroups and get cgroup paths
	if err := filepath.WalkDir(c.cgroupManager.mountPoint, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore step jobs
		if !info.IsDir() || c.cgroupManager.pathFilter(p) {
			return nil
		}

		// Get relative path of cgroup
		rel, err := filepath.Rel(c.cgroupManager.root, p)
		if err != nil {
			level.Error(c.logger).Log("msg", "Failed to resolve relative path for cgroup", "path", p, "err", err)

			return nil
		}

		// Get cgroup ID which is job ID
		cgroupIDMatches := c.cgroupManager.idRegex.FindStringSubmatch(p)
		if len(cgroupIDMatches) <= 1 {
			return nil
		}

		jobuuid := strings.TrimSpace(cgroupIDMatches[1])
		if jobuuid == "" {
			level.Error(c.logger).Log("msg", "Empty job ID", "path", p)

			return nil
		}

		// Check if we already passed through this job
		if slices.Contains(activeJobUUIDs, jobuuid) {
			return nil
		}

		// Get GPU ordinals of the job
		if len(c.gpuDevs) > 0 {
			if jobPropsCached, ok := c.jobPropsCache[jobuuid]; !ok || (ok && jobPropsCached.emptyGPUOrdinals()) {
				gpuOrdinals = c.gpuOrdinals(jobuuid)
				c.jobPropsCache[jobuuid] = jobProps{uuid: jobuuid, gpuOrdinals: gpuOrdinals}
				jProps = append(jProps, c.jobPropsCache[jobuuid])
			} else {
				jProps = append(jProps, jobPropsCached)
			}
		}

		activeJobUUIDs = append(activeJobUUIDs, jobuuid)
		cgMetrics = append(cgMetrics, cgMetric{uuid: jobuuid, path: "/" + rel})

		level.Debug(c.logger).Log("msg", "cgroup path", "path", p)

		return nil
	}); err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.cgroupManager.mountPoint, "err", err)

		return slurmMetrics{}, err
	}

	// Remove expired jobs from jobPropsCache
	for uuid := range c.jobPropsCache {
		if !slices.Contains(activeJobUUIDs, uuid) {
			delete(c.jobPropsCache, uuid)
		}
	}

	return slurmMetrics{cgMetrics: cgMetrics, jobProps: jProps}, nil
}

// readGPUMapFile reads file created by prolog script to retrieve job ID of a given GPU.
func (c *slurmCollector) readGPUMapFile(index string) string {
	gpuJobMapInfo := fmt.Sprintf("%s/%s", *slurmGPUStatPath, index)

	// NOTE: Look for file name with UUID as it will be more appropriate with
	// MIG instances.
	// If /run/gpustat/0 file is not found, check for the file with UUID as name?
	var uuid string

	if _, err := os.Stat(gpuJobMapInfo); err == nil {
		content, err := os.ReadFile(gpuJobMapInfo)
		if err != nil {
			level.Error(c.logger).Log(
				"msg", "Failed to get job ID for GPU",
				"index", index, "err", err,
			)

			return ""
		}

		if _, err := fmt.Sscanf(string(content), "%s", &uuid); err != nil {
			level.Error(c.logger).Log(
				"msg", "Failed to scan job ID for GPU",
				"index", index, "err", err,
			)

			return ""
		}

		return uuid
	}

	return ""
}

// gpuOrdinalsFromProlog returns GPU ordinals of jobs from prolog generated run time files by SLURM.
func (c *slurmCollector) gpuOrdinalsFromProlog(uuid string) []string {
	var gpuOrdinals []string

	// If there are no GPUs this loop will be skipped anyways
	// NOTE: In go loop over map is not reproducible. The order is undefined and thus
	// we might end up with a situation where jobGPUOrdinals will [1 2] or [2 1] if
	// current Job has two GPUs. This will fail unit tests as order in Slice is important
	// in Go
	//
	// So we use map[int]Device to have int indices for devices which we use internally
	// We are not using device index as it might be a non-integer. We are not sure about
	// it but just to be safe. This will have a small overhead as we need to check the
	// correct integer index for each device index. We can live with it as there are
	// typically 2/4/8 GPUs per node.
	for _, dev := range c.gpuDevs {
		if dev.migEnabled {
			for _, mig := range dev.migInstances {
				if c.readGPUMapFile(mig.globalIndex) == uuid {
					gpuOrdinals = append(gpuOrdinals, mig.globalIndex)
				}
			}
		} else {
			if c.readGPUMapFile(dev.globalIndex) == uuid {
				gpuOrdinals = append(gpuOrdinals, dev.globalIndex)
			}
		}
	}

	return gpuOrdinals
}

// gpuOrdinalsFromEnviron returns GPU ordinals of jobs by reading environment variables of jobs.
func (c *slurmCollector) gpuOrdinalsFromEnviron(uuid string) []string {
	// Read env vars in a security context that raises necessary capabilities
	dataPtr := &slurmReadProcSecurityCtxData{
		procfs: c.procFS,
		uuid:   uuid,
	}

	if securityCtx, ok := c.securityContexts[slurmReadProcCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			level.Error(c.logger).Log(
				"msg", "Failed to run inside security contxt", "jobid", uuid, "err", err,
			)

			return nil
		}
	} else {
		level.Error(c.logger).Log(
			"msg", "Security context not found", "name", slurmReadProcCtx, "jobid", uuid,
		)

		return nil
	}

	return dataPtr.gpuOrdinals
}

// gpuOrdinals returns GPU ordinals bound to current job.
func (c *slurmCollector) gpuOrdinals(uuid string) []string {
	var gpuOrdinals []string

	// First try to read files that might be created by SLURM prolog scripts
	gpuOrdinals = c.gpuOrdinalsFromProlog(uuid)

	// If we fail to get necessary job properties, try to get these properties
	// by looking into environment variables
	if len(gpuOrdinals) == 0 {
		gpuOrdinals = c.gpuOrdinalsFromEnviron(uuid)
	}

	// Emit warning when there are GPUs but no job to GPU map found
	if len(gpuOrdinals) == 0 {
		level.Warn(c.logger).
			Log("msg", "Failed to get GPU ordinals for job", "jobid", uuid)
	} else {
		level.Debug(c.logger).Log(
			"msg", "GPU ordinals", "jobid", uuid, "ordinals", strings.Join(gpuOrdinals, ","),
		)
	}

	return gpuOrdinals
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

	// Attempt to get GPU ordinals from /proc file system by looking into
	// environ for the process that has same SLURM_JOB_ID
	// Get all procs from current proc fs if passed pids slice is nil
	allProcs, err := d.procfs.AllProcs()
	if err != nil {
		return fmt.Errorf("failed to read /proc: %w", err)
	}

	// Env var that we will search
	jobIDEnv := "SLURM_JOB_ID=" + d.uuid

	// Iterate through all procs and look for SLURM_JOB_ID env entry
	// Here we have to sacrifice multi-threading for security. We cannot
	// spawn go-routines inside as we will execute this function inside
	// a security context locked to OS thread. Any new go routines spawned
	// WILL NOT BE scheduled on this locked thread and hence will not
	// have capabilities to read environment variables. So, we just do
	// old school loop on procs and attempt to find target env variables.
	for _, proc := range allProcs {
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
