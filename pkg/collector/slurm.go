//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	"github.com/containerd/cgroups/v3/cgroup2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const (
	slurmCollectorSubsystem = "slurm"
	genericSubsystem        = "compute"
)

var (
	cgroupsV1Subsystem = CEEMSExporterApp.Flag(
		"collector.slurm.cgroups-v1-subsystem",
		"Active cgroup subsystem for cgroups v1.",
	).Default("cpuacct").String()
	collectSwapMemoryStatsDepre = CEEMSExporterApp.Flag(
		"collector.slurm.swap.memory.metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Hidden().Bool()
	collectSwapMemoryStats = CEEMSExporterApp.Flag(
		"collector.slurm.swap-memory-metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Bool()
	collectPSIStatsDepre = CEEMSExporterApp.Flag(
		"collector.slurm.psi.metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Hidden().Bool()
	collectPSIStats = CEEMSExporterApp.Flag(
		"collector.slurm.psi-metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Bool()
	gpuType = CEEMSExporterApp.Flag(
		"collector.slurm.gpu-type",
		"GPU device type. Currently only nvidia and amd devices are supported.",
	).Hidden().Enum("nvidia", "amd")
	gpuStatPath = CEEMSExporterApp.Flag(
		"collector.slurm.gpu-job-map-path",
		"Path to file that maps GPU ordinals to job IDs.",
	).Default("/run/gpujobmap").Hidden().String()
	forceCgroupsVersion = CEEMSExporterApp.Flag(
		"collector.slurm.force-cgroups-version",
		"Set cgroups version manually. Used only for testing.",
	).Hidden().Enum("v1", "v2")
	nvidiaSmiPath = CEEMSExporterApp.Flag(
		"collector.slurm.nvidia-smi-path",
		"Absolute path to nvidia-smi binary. Use only for testing.",
	).Hidden().Default("").String()
	rocmSmiPath = CEEMSExporterApp.Flag(
		"collector.slurm.rocm-smi-path",
		"Absolute path to rocm-smi binary. Use only for testing.",
	).Hidden().Default("").String()
)

// jobProps contains cachable SLURM job properties.
type jobProps struct {
	uuid        string   // This is SLURM's job ID
	gpuOrdinals []string // GPU ordinals bound to job
}

// CgroupMetric contains metrics returned by cgroup.
type CgroupMetric struct {
	path            string
	cpuUser         float64
	cpuSystem       float64
	cpuTotal        float64
	cpus            int
	cpuPressure     float64
	memoryRSS       float64
	memoryCache     float64
	memoryUsed      float64
	memoryTotal     float64
	memoryFailCount float64
	memswUsed       float64
	memswTotal      float64
	memswFailCount  float64
	memoryPressure  float64
	rdmaHCAHandles  map[string]float64
	rdmaHCAObjects  map[string]float64
	jobuuid         string
	jobgpuordinals  []string
	err             bool
}

type slurmCollector struct {
	cgroups            string // v1 or v2
	cgroupsRootPath    string
	slurmCgroupsPath   string
	manager            string
	hostname           string
	gpuDevs            map[int]Device
	hostMemTotal       float64
	procFS             procfs.FS
	numJobs            *prometheus.Desc
	jobCPUUser         *prometheus.Desc
	jobCPUSystem       *prometheus.Desc
	jobCPUs            *prometheus.Desc
	jobCPUPressure     *prometheus.Desc
	jobMemoryRSS       *prometheus.Desc
	jobMemoryCache     *prometheus.Desc
	jobMemoryUsed      *prometheus.Desc
	jobMemoryTotal     *prometheus.Desc
	jobMemoryFailCount *prometheus.Desc
	jobMemswUsed       *prometheus.Desc
	jobMemswTotal      *prometheus.Desc
	jobMemswFailCount  *prometheus.Desc
	jobMemoryPressure  *prometheus.Desc
	jobRDMAHCAHandles  *prometheus.Desc
	jobRDMAHCAObjects  *prometheus.Desc
	jobGpuFlag         *prometheus.Desc
	collectError       *prometheus.Desc
	jobsCache          map[string]jobProps
	logger             log.Logger
}

func init() {
	RegisterCollector(slurmCollectorSubsystem, defaultDisabled, NewSlurmCollector)
}

// NewSlurmCollector returns a new Collector exposing a summary of cgroups.
func NewSlurmCollector(logger log.Logger) (Collector, error) {
	// Log deprecation notices
	if *collectPSIStatsDepre {
		level.Warn(logger).
			Log("msg", "flag --collector.slurm.psi.metrics has been deprecated. Use --collector.slurm.psi-metrics instead")
	}

	if *collectSwapMemoryStatsDepre {
		level.Warn(logger).
			Log("msg", "flag --collector.slurm.swap.memory.metrics has been deprecated. Use --collector.slurm.swap-memory-metrics instead")
	}

	var cgroupsVersion string

	var cgroupsRootPath string

	var slurmCgroupsPath string

	// Set cgroups root path based on cgroups version
	if cgroups.Mode() == cgroups.Unified {
		cgroupsVersion = "v2"
		cgroupsRootPath = *cgroupfsPath
		slurmCgroupsPath = filepath.Join(*cgroupfsPath, "system.slice/slurmstepd.scope")
	} else {
		cgroupsVersion = "v1"
		cgroupsRootPath = filepath.Join(*cgroupfsPath, *cgroupsV1Subsystem)
		slurmCgroupsPath = filepath.Join(cgroupsRootPath, "slurm")
	}

	level.Info(logger).Log("cgroup", cgroupsVersion, "mount", slurmCgroupsPath)

	// If cgroup version is set via CLI flag for testing override the one we got earlier
	if *forceCgroupsVersion != "" {
		cgroupsVersion = *forceCgroupsVersion
		if cgroupsVersion == "v2" {
			cgroupsRootPath = *cgroupfsPath
			slurmCgroupsPath = filepath.Join(*cgroupfsPath, "system.slice/slurmstepd.scope")
		} else if cgroupsVersion == "v1" {
			cgroupsRootPath = filepath.Join(*cgroupfsPath, "cpuacct")
			slurmCgroupsPath = filepath.Join(cgroupsRootPath, "slurm")
		}
	}

	// Attempt to get GPU devices
	var gpuTypes []string

	var gpuDevs map[int]Device

	var err error

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

	// Get total memory of host
	var memTotal float64

	file, err := os.Open(procFilePath("meminfo"))
	if err == nil {
		if memInfo, err := parseMemInfo(file); err == nil {
			memTotal = memInfo["MemTotal_bytes"]
		}
	} else {
		level.Error(logger).Log("msg", "Failed to get total memory of the host", "err", err)
	}

	defer file.Close()

	return &slurmCollector{
		cgroups:          cgroupsVersion,
		cgroupsRootPath:  cgroupsRootPath,
		slurmCgroupsPath: slurmCgroupsPath,
		manager:          slurmCollectorSubsystem,
		hostname:         hostname,
		gpuDevs:          gpuDevs,
		hostMemTotal:     memTotal,
		procFS:           procFS,
		jobsCache:        make(map[string]jobProps),
		numJobs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "units"),
			"Total number of jobs",
			[]string{"manager", "hostname"},
			nil,
		),
		jobCPUUser: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_user_seconds_total"),
			"Total job CPU user seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobCPUSystem: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_system_seconds_total"),
			"Total job CPU system seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		// cpuTotal: prometheus.NewDesc(
		// 	prometheus.BuildFQName(Namespace, genericSubsystem, "job_cpu_total_seconds"),
		// 	"Total job CPU total seconds",
		// 	[]string{"manager", "hostname", "uuid"},
		// 	nil,
		// ),
		jobCPUs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpus"),
			"Total number of job CPUs",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobCPUPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_psi_seconds"),
			"Total CPU PSI in seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemoryRSS: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_rss_bytes"),
			"Memory RSS used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemoryCache: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_cache_bytes"),
			"Memory cache used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemoryUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_used_bytes"),
			"Memory used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemoryTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_total_bytes"),
			"Memory total in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemoryFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_fail_count"),
			"Memory fail count",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemswUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_used_bytes"),
			"Swap used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemswTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_total_bytes"),
			"Swap total in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemswFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_fail_count"),
			"Swap fail count",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobMemoryPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_psi_seconds"),
			"Total memory PSI in seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		jobRDMAHCAHandles: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_handles"),
			"Current number of RDMA HCA handles",
			[]string{"manager", "hostname", "uuid", "device"},
			nil,
		),
		jobRDMAHCAObjects: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_objects"),
			"Current number of RDMA HCA objects",
			[]string{"manager", "hostname", "uuid", "device"},
			nil,
		),
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
	// Send job level metrics
	metrics, err := c.getJobsMetrics()
	if err != nil {
		return err
	}

	// First send num jobs on the current host
	ch <- prometheus.MustNewConstMetric(c.numJobs, prometheus.GaugeValue, float64(len(metrics)), c.manager, c.hostname)

	// Send metrics of each cgroup
	for _, m := range metrics {
		if m.err {
			ch <- prometheus.MustNewConstMetric(c.collectError, prometheus.GaugeValue, 1, m.path)
		}

		// CPU stats
		ch <- prometheus.MustNewConstMetric(c.jobCPUUser, prometheus.CounterValue, m.cpuUser, c.manager, c.hostname, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobCPUSystem, prometheus.CounterValue, m.cpuSystem, c.manager, c.hostname, m.jobuuid)
		// ch <- prometheus.MustNewConstMetric(c.cpuTotal, prometheus.GaugeValue, m.cpuTotal, c.manager, c.hostname, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobCPUs, prometheus.GaugeValue, float64(m.cpus), c.manager, c.hostname, m.jobuuid)

		// Memory stats
		ch <- prometheus.MustNewConstMetric(c.jobMemoryRSS, prometheus.GaugeValue, m.memoryRSS, c.manager, c.hostname, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryCache, prometheus.GaugeValue, m.memoryCache, c.manager, c.hostname, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryUsed, prometheus.GaugeValue, m.memoryUsed, c.manager, c.hostname, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryTotal, prometheus.GaugeValue, m.memoryTotal, c.manager, c.hostname, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryFailCount, prometheus.GaugeValue, m.memoryFailCount, c.manager, c.hostname, m.jobuuid)

		// PSI stats. Push them only if they are available
		if *collectSwapMemoryStatsDepre || *collectSwapMemoryStats {
			ch <- prometheus.MustNewConstMetric(c.jobMemswUsed, prometheus.GaugeValue, m.memswUsed, c.manager, c.hostname, m.jobuuid)
			ch <- prometheus.MustNewConstMetric(c.jobMemswTotal, prometheus.GaugeValue, m.memswTotal, c.manager, c.hostname, m.jobuuid)
			ch <- prometheus.MustNewConstMetric(c.jobMemswFailCount, prometheus.GaugeValue, m.memswFailCount, c.manager, c.hostname, m.jobuuid)
		}

		if *collectPSIStatsDepre || *collectPSIStats {
			ch <- prometheus.MustNewConstMetric(c.jobCPUPressure, prometheus.GaugeValue, m.cpuPressure, c.manager, c.hostname, m.jobuuid)
			ch <- prometheus.MustNewConstMetric(c.jobMemoryPressure, prometheus.GaugeValue, m.memoryPressure, c.manager, c.hostname, m.jobuuid)
		}

		// RDMA stats
		for device, handles := range m.rdmaHCAHandles {
			if handles > 0 {
				ch <- prometheus.MustNewConstMetric(c.jobRDMAHCAHandles, prometheus.GaugeValue, handles, c.manager, c.hostname, m.jobuuid, device)
			}
		}

		for device, objects := range m.rdmaHCAHandles {
			if objects > 0 {
				ch <- prometheus.MustNewConstMetric(c.jobRDMAHCAObjects, prometheus.GaugeValue, objects, c.manager, c.hostname, m.jobuuid, device)
			}
		}

		// GPU job mapping
		if len(c.gpuDevs) > 0 {
			for _, gpuOrdinal := range m.jobgpuordinals {
				var uuid string
				// Check the int index of devices where gpuOrdinal == dev.index
				for _, dev := range c.gpuDevs {
					if gpuOrdinal == dev.index {
						uuid = dev.uuid

						break
					}
				}
				ch <- prometheus.MustNewConstMetric(c.jobGpuFlag, prometheus.GaugeValue, float64(1), c.manager, c.hostname, m.jobuuid, gpuOrdinal, fmt.Sprintf("%s-gpu-%s", c.hostname, gpuOrdinal), uuid)
			}
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *slurmCollector) Stop(_ context.Context) error {
	level.Debug(c.logger).Log("msg", "Stopping", "collector", slurmCollectorSubsystem)

	return nil
}

// Get current Jobs metrics from cgroups.
func (c *slurmCollector) getJobsMetrics() ([]CgroupMetric, error) {
	// Get currently active jobs and set them in activeJobs state variable
	var activeJobUUIDs []string

	var metrics []CgroupMetric

	var gpuOrdinals []string

	level.Debug(c.logger).Log("msg", "Loading cgroup", "path", c.slurmCgroupsPath)

	// Walk through all cgroups and get cgroup paths
	if err := filepath.WalkDir(c.slurmCgroupsPath, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore step jobs
		if !info.IsDir() || strings.Contains(p, "/step_") {
			return nil
		}

		// Get relative path of cgroup
		rel, err := filepath.Rel(c.cgroupsRootPath, p)
		if err != nil {
			level.Error(c.logger).Log("msg", "Failed to resolve relative path for cgroup", "path", p, "err", err)

			return nil
		}

		// Get cgroup ID which is job ID
		cgroupIDMatches := slurmCgroupPathRegex.FindStringSubmatch(p)
		if len(cgroupIDMatches) <= 1 {
			return nil
		}

		jobuuid := strings.TrimSpace(cgroupIDMatches[1])
		if jobuuid == "" {
			level.Error(c.logger).Log("msg", "Empty job ID", "path", p)

			return nil
		}

		// Get GPU ordinals of the job
		if props, ok := c.jobsCache[jobuuid]; !ok || (ok && !c.containsGPUOrdinals(props)) {
			gpuOrdinals = c.gpuOrdinals(jobuuid)
			c.jobsCache[jobuuid] = jobProps{uuid: jobuuid, gpuOrdinals: gpuOrdinals}
		} else {
			gpuOrdinals = c.jobsCache[jobuuid].gpuOrdinals
		}

		activeJobUUIDs = append(activeJobUUIDs, jobuuid)
		metrics = append(metrics, CgroupMetric{jobuuid: jobuuid, path: "/" + rel, jobgpuordinals: gpuOrdinals})

		level.Debug(c.logger).Log("msg", "cgroup path", "path", p)

		return nil
	}); err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.slurmCgroupsPath, "err", err)

		return nil, err
	}

	// Remove expired jobs from jobsCache
	for uuid := range c.jobsCache {
		if !slices.Contains(activeJobUUIDs, uuid) {
			delete(c.jobsCache, uuid)
		}
	}

	// Start wait group for go routines
	wg := &sync.WaitGroup{}
	wg.Add(len(metrics))

	// No need for any lock primitives here as we read/write
	// a different element of slice in each go routine
	for i := range len(metrics) {
		go func(idx int) {
			defer wg.Done()

			c.getMetrics(&metrics[idx])
		}(i)
	}

	// Wait for all go routines
	wg.Wait()

	return metrics, nil
}

// getMetrics fetches metrics of a given SLURM cgroups path.
func (c *slurmCollector) getMetrics(metric *CgroupMetric) {
	if c.cgroups == "v2" {
		c.getCgroupsV2Metrics(metric)
	} else {
		c.getCgroupsV1Metrics(metric)
	}
}

// parseCPUSet parses cpuset.cpus file to return a list of CPUs in the cgroup.
func (c *slurmCollector) parseCPUSet(cpuset string) ([]string, error) {
	var cpus []string

	var start, end int

	var err error

	if cpuset == "" {
		return nil, errors.New("empty cpuset file")
	}

	ranges := strings.Split(cpuset, ",")
	for _, r := range ranges {
		boundaries := strings.Split(r, "-")
		if len(boundaries) == 1 {
			start, err = strconv.Atoi(boundaries[0])
			if err != nil {
				return nil, err
			}

			end = start
		} else if len(boundaries) == 2 {
			start, err = strconv.Atoi(boundaries[0])
			if err != nil {
				return nil, err
			}

			end, err = strconv.Atoi(boundaries[1])
			if err != nil {
				return nil, err
			}
		}

		for e := start; e <= end; e++ {
			cpu := strconv.Itoa(e)
			cpus = append(cpus, cpu)
		}
	}

	return cpus, nil
}

// getCPUs returns list of CPUs in the cgroup.
func (c *slurmCollector) getCPUs(path string) ([]string, error) {
	var cpusPath string
	if c.cgroups == "v2" {
		cpusPath = fmt.Sprintf("%s%s/cpuset.cpus.effective", *cgroupfsPath, path)
	} else {
		cpusPath = fmt.Sprintf("%s/cpuset%s/cpuset.cpus", *cgroupfsPath, path)
	}

	if !fileExists(cpusPath) {
		return nil, fmt.Errorf("cpuset file %s not found", cpusPath)
	}

	cpusData, err := os.ReadFile(cpusPath)
	if err != nil {
		level.Error(c.logger).Log("msg", "Error reading cpuset", "cpuset", cpusPath, "err", err)

		return nil, err
	}

	cpus, err := c.parseCPUSet(strings.TrimSuffix(string(cpusData), "\n"))
	if err != nil {
		level.Error(c.logger).Log("msg", "Error parsing cpuset", "cpuset", cpusPath, "err", err)

		return nil, err
	}

	return cpus, nil
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
		gpuJobMapInfo := fmt.Sprintf("%s/%s", *gpuStatPath, dev.index)

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

	// Set jobProps fields
	return gpuOrdinals
}

// containsGPUOrdinals returns true if jobProps has gpuOrdinals populated.
func (c *slurmCollector) containsGPUOrdinals(p jobProps) bool {
	return len(c.gpuDevs) > 0 && len(p.gpuOrdinals) == 0
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

// Get metrics from cgroups v1.
func (c *slurmCollector) getCgroupsV1Metrics(metric *CgroupMetric) {
	path := metric.path
	// metric := CgroupMetric{path: path, jobuuid: job.uuid}

	level.Debug(c.logger).Log("msg", "Loading cgroup v1", "path", path)

	ctrl, err := cgroup1.Load(cgroup1.StaticPath(path), cgroup1.WithHierarchy(subsystem))
	if err != nil {
		metric.err = true

		level.Error(c.logger).Log("msg", "Failed to load cgroups", "path", path, "err", err)

		return
	}

	// Load cgroup stats
	stats, err := ctrl.Stat(cgroup1.IgnoreNotExist)
	if err != nil {
		metric.err = true

		level.Error(c.logger).Log("msg", "Failed to stat cgroups", "path", path, "err", err)

		return
	}

	if stats == nil {
		metric.err = true

		level.Error(c.logger).Log("msg", "Cgroup stats are nil", "path", path)

		return
	}

	// Get CPU stats
	if stats.GetCPU() != nil {
		if stats.GetCPU().GetUsage() != nil {
			metric.cpuUser = float64(stats.GetCPU().GetUsage().GetUser()) / 1000000000.0
			metric.cpuSystem = float64(stats.GetCPU().GetUsage().GetKernel()) / 1000000000.0
			metric.cpuTotal = float64(stats.GetCPU().GetUsage().GetTotal()) / 1000000000.0
		}
	}

	if cpus, err := c.getCPUs(path); err == nil {
		metric.cpus = len(cpus)
	}

	// Get memory stats
	if stats.GetMemory() != nil {
		metric.memoryRSS = float64(stats.GetMemory().GetTotalRSS())
		metric.memoryCache = float64(stats.GetMemory().GetTotalCache())

		if stats.GetMemory().GetUsage() != nil {
			metric.memoryUsed = float64(stats.GetMemory().GetUsage().GetUsage())
			// If memory usage limit is set as "max", cgroups lib will set it to
			// math.MaxUint64. Here we replace it with total system memory
			if stats.GetMemory().GetUsage().GetLimit() == math.MaxUint64 && c.hostMemTotal != 0 {
				metric.memoryTotal = c.hostMemTotal
			} else {
				metric.memoryTotal = float64(stats.GetMemory().GetUsage().GetLimit())
			}

			metric.memoryFailCount = float64(stats.GetMemory().GetUsage().GetFailcnt())
		}

		if stats.GetMemory().GetSwap() != nil {
			metric.memswUsed = float64(stats.GetMemory().GetSwap().GetUsage())
			// If memory usage limit is set as "max", cgroups lib will set it to
			// math.MaxUint64. Here we replace it with total system memory
			if stats.GetMemory().GetSwap().GetLimit() == math.MaxUint64 && c.hostMemTotal != 0 {
				metric.memswTotal = c.hostMemTotal
			} else {
				metric.memswTotal = float64(stats.GetMemory().GetSwap().GetLimit())
			}

			metric.memswFailCount = float64(stats.GetMemory().GetSwap().GetFailcnt())
		}
	}

	// Get RDMA metrics if available
	if stats.GetRdma() != nil {
		metric.rdmaHCAHandles = make(map[string]float64)
		metric.rdmaHCAObjects = make(map[string]float64)

		for _, device := range stats.GetRdma().GetCurrent() {
			metric.rdmaHCAHandles[device.GetDevice()] = float64(device.GetHcaHandles())
			metric.rdmaHCAObjects[device.GetDevice()] = float64(device.GetHcaObjects())
		}
	}
}

// Get Job metrics from cgroups v2.
func (c *slurmCollector) getCgroupsV2Metrics(metric *CgroupMetric) {
	path := metric.path
	// metric := CgroupMetric{path: path, jobuuid: job.uuid}

	level.Debug(c.logger).Log("msg", "Loading cgroup v2", "path", path)

	// Load cgroups
	ctrl, err := cgroup2.Load(path, cgroup2.WithMountpoint(*cgroupfsPath))
	if err != nil {
		metric.err = true

		level.Error(c.logger).Log("msg", "Failed to load cgroups", "path", path, "err", err)

		return
	}

	// Get stats from cgroup
	stats, err := ctrl.Stat()
	if err != nil {
		metric.err = true

		level.Error(c.logger).Log("msg", "Failed to stat cgroups", "path", path, "err", err)

		return
	}

	if stats == nil {
		metric.err = true

		level.Error(c.logger).Log("msg", "Cgroup stats are nil", "path", path)

		return
	}

	// Get CPU stats
	if stats.GetCPU() != nil {
		metric.cpuUser = float64(stats.GetCPU().GetUserUsec()) / 1000000.0
		metric.cpuSystem = float64(stats.GetCPU().GetSystemUsec()) / 1000000.0
		metric.cpuTotal = float64(stats.GetCPU().GetUsageUsec()) / 1000000.0

		if stats.GetCPU().GetPSI() != nil {
			metric.cpuPressure = float64(stats.GetCPU().GetPSI().GetFull().GetTotal()) / 1000000.0
		}
	}

	if cpus, err := c.getCPUs(path); err == nil {
		metric.cpus = len(cpus)
	}

	// Get memory stats
	// cgroups2 does not expose swap memory events. So we dont set memswFailCount
	if stats.GetMemory() != nil {
		metric.memoryUsed = float64(stats.GetMemory().GetUsage())
		// If memory usage limit is set as "max", cgroups lib will set it to
		// math.MaxUint64. Here we replace it with total system memory
		if stats.GetMemory().GetUsageLimit() == math.MaxUint64 && c.hostMemTotal > 0 {
			metric.memoryTotal = c.hostMemTotal
		} else {
			metric.memoryTotal = float64(stats.GetMemory().GetUsageLimit())
		}

		metric.memoryCache = float64(stats.GetMemory().GetFile()) // This is page cache
		metric.memoryRSS = float64(stats.GetMemory().GetAnon())
		metric.memswUsed = float64(stats.GetMemory().GetSwapUsage())
		// If memory usage limit is set as "max", cgroups lib will set it to
		// math.MaxUint64. Here we replace it with total system memory
		if stats.GetMemory().GetSwapLimit() == math.MaxUint64 && c.hostMemTotal > 0 {
			metric.memswTotal = c.hostMemTotal
		} else {
			metric.memswTotal = float64(stats.GetMemory().GetSwapLimit())
		}

		if stats.GetMemory().GetPSI() != nil {
			metric.memoryPressure = float64(stats.GetMemory().GetPSI().GetFull().GetTotal()) / 1000000.0
		}
	}
	// Get memory events
	if stats.GetMemoryEvents() != nil {
		metric.memoryFailCount = float64(stats.GetMemoryEvents().GetOom())
	}

	// Get RDMA stats
	if stats.GetRdma() != nil {
		metric.rdmaHCAHandles = make(map[string]float64)
		metric.rdmaHCAObjects = make(map[string]float64)

		for _, device := range stats.GetRdma().GetCurrent() {
			metric.rdmaHCAHandles[device.GetDevice()] = float64(device.GetHcaHandles())
			metric.rdmaHCAObjects[device.GetDevice()] = float64(device.GetHcaObjects())
		}
	}
}

// subsystem returns cgroups v1 subsystems.
func subsystem() ([]cgroup1.Subsystem, error) {
	s := []cgroup1.Subsystem{
		cgroup1.NewCpuacct(*cgroupfsPath),
		cgroup1.NewMemory(*cgroupfsPath),
		cgroup1.NewRdma(*cgroupfsPath),
	}

	return s, nil
}
