//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const (
	slurmCollectorSubsystem = "slurm"
)

// CLI opts.
var (
	// Perf opts.
	slurmPerfHwProfilersFlag = CEEMSExporterApp.Flag(
		"collector.slurm.perf-hardware-events",
		"Enables collection of perf hardware events (default: disabled)",
	).Default("false").Bool()
	slurmPerfHwProfilers = CEEMSExporterApp.Flag(
		"collector.slurm.perf-hardware-profilers",
		"perf hardware profilers to collect",
	).Strings()
	slurmPerfSwProfilersFlag = CEEMSExporterApp.Flag(
		"collector.slurm.perf-software-events",
		"Enables collection of perf software events (default: disabled)",
	).Default("false").Bool()
	slurmPerfSwProfilers = CEEMSExporterApp.Flag(
		"collector.slurm.perf-software-profilers",
		"perf software profilers to collect",
	).Strings()
	slurmPerfCacheProfilersFlag = CEEMSExporterApp.Flag(
		"collector.slurm.perf-cache-events",
		"Enables collection of perf cache events (default: disabled)",
	).Default("false").Bool()
	slurmPerfCacheProfilers = CEEMSExporterApp.Flag(
		"collector.slurm.perf-cache-profilers",
		"perf cache profilers to collect",
	).Strings()
	slurmPerfProfilersEnvVars = CEEMSExporterApp.Flag(
		"collector.slurm.perf-env-var",
		"Processes having any of these environment variables set will be profiled. If empty, all processes will be profiled.",
	).Strings()

	// ebpf opts.
	slurmIOMetricsFlag = CEEMSExporterApp.Flag(
		"collector.slurm.io-metrics",
		"Enables collection of IO metrics using ebpf (default: disabled)",
	).Default("false").Bool()
	slurmNetMetricsFlag = CEEMSExporterApp.Flag(
		"collector.slurm.network-metrics",
		"Enables collection of network metrics using ebpf (default: disabled)",
	).Default("false").Bool()
	slurmFSMountPoints = CEEMSExporterApp.Flag(
		"collector.slurm.fs-mount-point",
		"File system mount points to monitor for IO stats. If empty all mount points are monitored. It is strongly advised to choose appropriate mount points to reduce cardinality.",
	).Strings()

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

	// Generic.
	slurmGPUStatPath = CEEMSExporterApp.Flag(
		"collector.slurm.gpu-job-map-path",
		"Path to file that maps GPU ordinals to job IDs.",
	).Default("/run/gpujobmap").String()

	// Used for e2e tests.
	gpuType = CEEMSExporterApp.Flag(
		"collector.slurm.gpu-type",
		"GPU device type. Currently only nvidia and amd devices are supported.",
	).Hidden().Enum("nvidia", "amd")
	nvidiaSmiPath = CEEMSExporterApp.Flag(
		"collector.slurm.nvidia-smi-path",
		"Absolute path to nvidia-smi binary. Use only for testing.",
	).Hidden().Default("").String()
	rocmSmiPath = CEEMSExporterApp.Flag(
		"collector.slurm.rocm-smi-path",
		"Absolute path to rocm-smi binary. Use only for testing.",
	).Hidden().Default("").String()
)

// props contains SLURM job properties.
type props struct {
	uuid        string   // This is SLURM's job ID
	gpuOrdinals []string // GPU ordinals bound to job
}

// emptyGPUOrdinals returns true if gpuOrdinals is empty.
func (p *props) emptyGPUOrdinals() bool {
	return len(p.gpuOrdinals) == 0
}

type slurmMetrics struct {
	cgMetrics []cgMetric
	jobProps  []props
}

type slurmCollector struct {
	logger          log.Logger
	cgroupManager   *cgroupManager
	cgroupCollector *cgroupCollector
	perfCollector   *perfCollector
	ebpfCollector   *ebpfCollector
	hostname        string
	gpuDevs         map[int]Device
	procFS          procfs.FS
	jobGpuFlag      *prometheus.Desc
	collectError    *prometheus.Desc
	jobPropsCache   map[string]props
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
	}

	// Start new instance of cgroupCollector
	cgCollector, err := NewCgroupCollector(logger, cgroupManager, opts)
	if err != nil {
		level.Info(logger).Log("msg", "Failed to create cgroup collector", "err", err)

		return nil, err
	}

	// Start new instance of perfCollector
	var perfCollector *perfCollector

	if perfCollectorEnabled() {
		perfOpts := perfOpts{
			perfHwProfilersEnabled:    *slurmPerfHwProfilersFlag,
			perfSwProfilersEnabled:    *slurmPerfSwProfilersFlag,
			perfCacheProfilersEnabled: *slurmPerfCacheProfilersFlag,
			perfHwProfilers:           *slurmPerfHwProfilers,
			perfSwProfilers:           *slurmPerfSwProfilers,
			perfCacheProfilers:        *slurmPerfCacheProfilers,
			targetEnvVars:             *slurmPerfProfilersEnvVars,
		}

		perfCollector, err = NewPerfCollector(logger, cgroupManager, perfOpts)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create perf collector", "err", err)

			return nil, err
		}
	}

	// Start new instance of ebpfCollector
	var ebpfCollector *ebpfCollector

	if ebpfCollectorEnabled() {
		ebpfOpts := ebpfOpts{
			vfsStatsEnabled: *slurmIOMetricsFlag,
			netStatsEnabled: *slurmNetMetricsFlag,
			vfsMountPoints:  *slurmFSMountPoints,
		}

		ebpfCollector, err = NewEbpfCollector(logger, cgroupManager, ebpfOpts)
		if err != nil {
			level.Info(logger).Log("msg", "Failed to create ebpf collector", "err", err)

			return nil, err
		}
	}

	// Attempt to get GPU devices
	var gpuTypes []string

	var gpuDevs map[int]Device

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

	// Instantiate a new Proc FS
	procFS, err := procfs.NewFS(*procfsPath)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	return &slurmCollector{
		cgroupManager:   cgroupManager,
		cgroupCollector: cgCollector,
		perfCollector:   perfCollector,
		ebpfCollector:   ebpfCollector,
		hostname:        hostname,
		gpuDevs:         gpuDevs,
		procFS:          procFS,
		jobPropsCache:   make(map[string]props),
		jobGpuFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_index_flag"),
			"Indicates running job on GPU, 1=job running",
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
		return err
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
			if err := c.perfCollector.Update(ch); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update perf stats", "err", err)
			}
		}()
	}

	if ebpfCollectorEnabled() {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Update ebpf metrics
			if err := c.ebpfCollector.Update(ch); err != nil {
				level.Error(c.logger).Log("msg", "Failed to update IO and/or network stats", "err", err)
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

	return nil
}

// updateGPUOrdinals updates the metrics channel with GPU ordinals for SLURM job.
func (c *slurmCollector) updateGPUOrdinals(ch chan<- prometheus.Metric, jobProps []props) {
	// Update slurm job properties
	for _, p := range jobProps {
		// GPU job mapping
		for _, gpuOrdinal := range p.gpuOrdinals {
			var gpuuuid string
			// Check the int index of devices where gpuOrdinal == dev.index
			for _, dev := range c.gpuDevs {
				if gpuOrdinal == dev.index {
					gpuuuid = dev.uuid

					break
				}
			}
			ch <- prometheus.MustNewConstMetric(c.jobGpuFlag, prometheus.GaugeValue, float64(1), c.cgroupManager.manager, c.hostname, p.uuid, gpuOrdinal, fmt.Sprintf("%s-gpu-%s", c.hostname, gpuOrdinal), gpuuuid)
		}
	}
}

// discoverCgroups finds active cgroup paths and returns initialised metric structs.
func (c *slurmCollector) discoverCgroups() (slurmMetrics, error) {
	// Get currently active jobs and set them in activeJobs state variable
	var activeJobUUIDs []string

	var jobProps []props

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
			if jProps, ok := c.jobPropsCache[jobuuid]; !ok || (ok && jProps.emptyGPUOrdinals()) {
				gpuOrdinals = c.gpuOrdinals(jobuuid)
				c.jobPropsCache[jobuuid] = props{uuid: jobuuid, gpuOrdinals: gpuOrdinals}
				jobProps = append(jobProps, c.jobPropsCache[jobuuid])
			} else {
				jobProps = append(jobProps, jProps)
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

	return slurmMetrics{cgMetrics: cgMetrics, jobProps: jobProps}, nil
}

// gpuOrdinalsFromProlog returns GPU ordinals of jobs from prolog generated run time files by SLURM.
func (c *slurmCollector) gpuOrdinalsFromProlog(uuid string) []string {
	var gpuJobID string

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
	for i := range c.gpuDevs {
		dev := c.gpuDevs[i]
		gpuJobMapInfo := fmt.Sprintf("%s/%s", *slurmGPUStatPath, dev.index)

		// NOTE: Look for file name with UUID as it will be more appropriate with
		// MIG instances.
		// If /run/gpustat/0 file is not found, check for the file with UUID as name?
		if _, err := os.Stat(gpuJobMapInfo); err == nil {
			content, err := os.ReadFile(gpuJobMapInfo)
			if err != nil {
				level.Error(c.logger).Log(
					"msg", "Failed to get job ID for GPU",
					"index", dev.index, "uuid", dev.uuid, "err", err,
				)

				continue
			}

			if _, err := fmt.Sscanf(string(content), "%s", &gpuJobID); err != nil {
				level.Error(c.logger).Log(
					"msg", "Failed to scan job ID for GPU",
					"index", dev.index, "uuid", dev.uuid, "err", err,
				)

				continue
			}

			if gpuJobID == uuid {
				gpuOrdinals = append(gpuOrdinals, dev.index)
			}
		}
	}

	return gpuOrdinals
}

// gpuOrdinalsFromEnviron returns GPU ordinals of jobs by reading environment variables of jobs.
func (c *slurmCollector) gpuOrdinalsFromEnviron(uuid string) []string {
	var gpuOrdinals []string

	// Attempt to get GPU ordinals from /proc file system by looking into
	// environ for the process that has same SLURM_JOB_ID
	// Get all procs from current proc fs if passed pids slice is nil
	allProcs, err := c.procFS.AllProcs()
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to read /proc", "err", err)

		return nil
	}

	// Env var that we will search
	jobIDEnv := "SLURM_JOB_ID=" + uuid

	// Initialize a waitgroup for all go routines that we will spawn later
	wg := &sync.WaitGroup{}
	wg.Add(allProcs.Len())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure it's called to release resources

	// Iterate through all procs and look for SLURM_JOB_ID env entry
	for _, proc := range allProcs {
		go func(p procfs.Proc) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			// Default is must to avoid blocking
			default:
				// Read process environment variables
				// NOTE: This needs CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH caps
				// on the current process
				// Skip if we cannot read file or job ID env var is not found
				environments, err := p.Environ()
				if err != nil || !slices.Contains(environments, jobIDEnv) {
					return
				}

				// When env var entry found, get all necessary env vars
				// NOTE: This is not really concurrent safe. Multiple go routines might
				// overwrite the variables. But I think we can live with it as for a gievn
				// job cgroup these env vars should be identical in all procs
				for _, env := range environments {
					if strings.Contains(env, "SLURM_STEP_GPUS") || strings.Contains(env, "SLURM_JOB_GPUS") {
						gpuOrdinals = strings.Split(strings.Split(env, "=")[1], ",")

						cancel() // Cancel context so that all go routines will exit

						return
					}
				}
			}
		}(proc)
	}

	// Wait for all go routines to finish
	wg.Wait()

	return gpuOrdinals
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

// perfCollectorEnabled returns true if any of perf profilers are enabled.
func perfCollectorEnabled() bool {
	return *slurmPerfHwProfilersFlag || *slurmPerfSwProfilersFlag || *slurmPerfCacheProfilersFlag
}

// ebpfCollectorEnabled returns true if any of ebpf stats are enabled.
func ebpfCollectorEnabled() bool {
	return *slurmIOMetricsFlag || *slurmNetMetricsFlag
}
