//go:build !noslurm
// +build !noslurm

package collector

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	"github.com/containerd/cgroups/v3/cgroup2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/helpers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const (
	slurmCollectorSubsystem = "slurm"
	genericSubsystem        = "compute"
)

var (
	metricLock             = sync.RWMutex{}
	collectSwapMemoryStats = CEEMSExporterApp.Flag(
		"collector.slurm.swap.memory.metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Bool()
	collectPSIStats = CEEMSExporterApp.Flag(
		"collector.slurm.psi.metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Bool()
	useJobIDHash = CEEMSExporterApp.Flag(
		"collector.slurm.create.unique.jobids",
		`Enables calculation of a unique hash based job UUID (default: disabled). 
UUID is calculated based on SLURM_JOBID, SLURM_JOB_USER, SLURM_JOB_ACCOUNT, SLURM_JOB_NODELIST.`,
	).Default("false").Hidden().Bool()
	gpuType = CEEMSExporterApp.Flag(
		"collector.slurm.gpu.type",
		"GPU device type. Currently only nvidia and amd devices are supported.",
	).Enum("nvidia", "amd")
	jobStatPath = CEEMSExporterApp.Flag(
		"collector.slurm.job.props.path",
		`Directory containing files with job properties. Files should be named after SLURM_JOBID 
with contents as "$SLURM_JOB_USER $SLURM_JOB_ACCOUNT $SLURM_JOB_NODELIST" in the same order.`,
	).Default("/run/slurmjobprops").Hidden().String()
	gpuStatPath = CEEMSExporterApp.Flag(
		"collector.slurm.gpu.job.map.path",
		"Path to file that maps GPU ordinals to job IDs.",
	).Default("/run/gpujobmap").Hidden().String()
	forceCgroupsVersion = CEEMSExporterApp.Flag(
		"collector.slurm.force.cgroups.version",
		"Set cgroups version manually. Used only for testing.",
	).Hidden().Enum("v1", "v2")
	nvidiaSmiPath = CEEMSExporterApp.Flag(
		"collector.slurm.nvidia.smi.path",
		"Absolute path to nvidia-smi binary. Use only for testing.",
	).Hidden().Default("").String()
	rocmSmiPath = CEEMSExporterApp.Flag(
		"collector.slurm.rocm.smi.path",
		"Absolute path to rocm-smi binary. Use only for testing.",
	).Hidden().Default("").String()
)

// SLURM cgroup names patterns
/*
	For v2 possibilities are /system.slice/slurmstepd.scope/job_211
							/system.slice/slurmstepd.scope/job_211/step_interactive
							/system.slice/slurmstepd.scope/job_211/step_extern/user/task_0
*/
var (
	slurmPatterns = map[string]*regexp.Regexp{
		"v1": regexp.MustCompile(
			"^/slurm/uid_(?P<uid>[0-9]+)/job_(?P<jobid>[0-9]+)(/step_(?P<setpid>[^/]+)(/task_(?P<taskid>[[0-9]+))?)?$",
		),
		"v2": regexp.MustCompile(
			"^/system.slice/slurmstepd.scope/job_(?P<jobid>[0-9]+)(/step_(?P<stepid>[^/]+)(/user/task_(?P<taskid>[[0-9]+))?)?$",
		),
	}
)

// JobProps contains SLURM job properties
type JobProps struct {
	jobUUID        string
	jobUser        string
	jobAccount     string
	jobNodelist    string
	jobGPUOrdinals []string
}

// CgroupMetric contains metrics returned by cgroup
type CgroupMetric struct {
	name            string
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
	jobuser         string
	jobaccount      string
	jobid           string
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
	jobPropsCache      sync.Map
	logger             log.Logger
}

func init() {
	RegisterCollector(slurmCollectorSubsystem, defaultEnabled, NewSlurmCollector)
}

// NewSlurmCollector returns a new Collector exposing a summary of cgroups.
func NewSlurmCollector(logger log.Logger) (Collector, error) {
	var cgroupsVersion string
	var cgroupsRootPath string
	var slurmCgroupsPath string

	// Set cgroups root path based on cgroups version
	if cgroups.Mode() == cgroups.Unified {
		cgroupsVersion = "v2"
		level.Info(logger).Log("msg", "Cgroup version v2 detected", "mount", *cgroupfsPath)
		cgroupsRootPath = *cgroupfsPath
		slurmCgroupsPath = fmt.Sprintf("%s/system.slice/slurmstepd.scope", *cgroupfsPath)
	} else {
		cgroupsVersion = "v1"
		level.Info(logger).Log("msg", "Cgroup version v2 not detected, will proceed with v1.")
		cgroupsRootPath = fmt.Sprintf("%s/cpuacct", *cgroupfsPath)
		slurmCgroupsPath = fmt.Sprintf("%s/slurm", cgroupsRootPath)
	}

	// If cgroup version is set via CLI flag for testing override the one we got earlier
	if *forceCgroupsVersion != "" {
		cgroupsVersion = *forceCgroupsVersion
		if cgroupsVersion == "v2" {
			cgroupsRootPath = *cgroupfsPath
			slurmCgroupsPath = fmt.Sprintf("%s/system.slice/slurmstepd.scope", *cgroupfsPath)
		} else if cgroupsVersion == "v1" {
			cgroupsRootPath = fmt.Sprintf("%s/cpuacct", *cgroupfsPath)
			slurmCgroupsPath = fmt.Sprintf("%s/slurm", cgroupsRootPath)
		}
	}

	// Attempt to get GPU devices
	var gpuDevs map[int]Device
	var err error
	gpuDevs, err = GetGPUDevices(*gpuType, logger)
	if err == nil {
		level.Info(logger).Log("msg", "GPU devices found")
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
		numJobs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "units"),
			"Total number of jobs",
			[]string{"manager", "hostname"},
			nil,
		),
		jobCPUUser: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_user_seconds_total"),
			"Total job CPU user seconds",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobCPUSystem: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_system_seconds_total"),
			"Total job CPU system seconds",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		// cpuTotal: prometheus.NewDesc(
		// 	prometheus.BuildFQName(Namespace, genericSubsystem, "job_cpu_total_seconds"),
		// 	"Total job CPU total seconds",
		// 	[]string{"manager", "hostname", "user", "project", "uuid"},
		// 	nil,
		// ),
		jobCPUs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpus"),
			"Total number of job CPUs",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobCPUPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_psi_seconds"),
			"Total CPU PSI in seconds",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemoryRSS: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_rss_bytes"),
			"Memory RSS used in bytes",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemoryCache: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_cache_bytes"),
			"Memory cache used in bytes",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemoryUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_used_bytes"),
			"Memory used in bytes",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemoryTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_total_bytes"),
			"Memory total in bytes",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemoryFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_fail_count"),
			"Memory fail count",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemswUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_used_bytes"),
			"Swap used in bytes",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemswTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_total_bytes"),
			"Swap total in bytes",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemswFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_fail_count"),
			"Swap fail count",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobMemoryPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_psi_seconds"),
			"Total memory PSI in seconds",
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		jobRDMAHCAHandles: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_handles"),
			"Current number of RDMA HCA handles",
			[]string{"manager", "hostname", "user", "project", "uuid", "device"},
			nil,
		),
		jobRDMAHCAObjects: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_objects"),
			"Current number of RDMA HCA objects",
			[]string{"manager", "hostname", "user", "project", "uuid", "device"},
			nil,
		),
		jobGpuFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_gpu_index_flag"),
			"Indicates running job on GPU, 1=job running",
			[]string{
				"manager",
				"hostname",
				"user",
				"account",
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
			[]string{"manager", "hostname", "user", "project", "uuid"},
			nil,
		),
		logger: logger,
	}, nil
}

// Return cgroups v1 subsystem
func subsystem() ([]cgroup1.Subsystem, error) {
	s := []cgroup1.Subsystem{
		cgroup1.NewCpuacct(*cgroupfsPath),
		cgroup1.NewMemory(*cgroupfsPath),
		cgroup1.NewRdma(*cgroupfsPath),
	}
	return s, nil
}

// Update implements Collector and update job metrics
func (c *slurmCollector) Update(ch chan<- prometheus.Metric) error {
	// Send job level metrics
	metrics, err := c.getJobsMetrics()
	if err != nil {
		return err
	}

	// First send num jobs on the current host
	ch <- prometheus.MustNewConstMetric(c.numJobs, prometheus.GaugeValue, float64(len(metrics)), c.manager, c.hostname)

	// Send metrics of each cgroup
	for n, m := range metrics {
		if m.err {
			ch <- prometheus.MustNewConstMetric(c.collectError, prometheus.GaugeValue, 1, m.name)
		}

		// CPU stats
		ch <- prometheus.MustNewConstMetric(c.jobCPUUser, prometheus.CounterValue, m.cpuUser, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobCPUSystem, prometheus.CounterValue, m.cpuSystem, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		// ch <- prometheus.MustNewConstMetric(c.cpuTotal, prometheus.GaugeValue, m.cpuTotal, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		cpus := m.cpus
		if cpus == 0 {
			dir := filepath.Dir(n)
			cpus = metrics[dir].cpus
			if cpus == 0 {
				cpus = metrics[filepath.Dir(dir)].cpus
			}
		}
		ch <- prometheus.MustNewConstMetric(c.jobCPUs, prometheus.GaugeValue, float64(cpus), c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)

		// Memory stats
		ch <- prometheus.MustNewConstMetric(c.jobMemoryRSS, prometheus.GaugeValue, m.memoryRSS, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryCache, prometheus.GaugeValue, m.memoryCache, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryUsed, prometheus.GaugeValue, m.memoryUsed, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryTotal, prometheus.GaugeValue, m.memoryTotal, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		ch <- prometheus.MustNewConstMetric(c.jobMemoryFailCount, prometheus.GaugeValue, m.memoryFailCount, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)

		// PSI stats. Push them only if they are available
		if *collectSwapMemoryStats {
			ch <- prometheus.MustNewConstMetric(c.jobMemswUsed, prometheus.GaugeValue, m.memswUsed, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
			ch <- prometheus.MustNewConstMetric(c.jobMemswTotal, prometheus.GaugeValue, m.memswTotal, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
			ch <- prometheus.MustNewConstMetric(c.jobMemswFailCount, prometheus.GaugeValue, m.memswFailCount, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		}
		if *collectPSIStats {
			ch <- prometheus.MustNewConstMetric(c.jobCPUPressure, prometheus.GaugeValue, m.cpuPressure, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
			ch <- prometheus.MustNewConstMetric(c.jobMemoryPressure, prometheus.GaugeValue, m.memoryPressure, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid)
		}

		// RDMA stats
		for device, handles := range m.rdmaHCAHandles {
			if handles > 0 {
				ch <- prometheus.MustNewConstMetric(c.jobRDMAHCAHandles, prometheus.GaugeValue, handles, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid, device)
			}
		}
		for device, objects := range m.rdmaHCAHandles {
			if objects > 0 {
				ch <- prometheus.MustNewConstMetric(c.jobRDMAHCAObjects, prometheus.GaugeValue, objects, c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid, device)
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
				ch <- prometheus.MustNewConstMetric(c.jobGpuFlag, prometheus.GaugeValue, float64(1), c.manager, c.hostname, m.jobuser, m.jobaccount, m.jobuuid, gpuOrdinal, fmt.Sprintf("%s-gpu-%s", c.hostname, gpuOrdinal), uuid)
			}
		}
	}
	return nil
}

// Get current Jobs metrics from cgroups
func (c *slurmCollector) getJobsMetrics() (map[string]CgroupMetric, error) {
	var names []string
	var metrics = make(map[string]CgroupMetric)

	level.Debug(c.logger).Log("msg", "Loading cgroup", "path", c.slurmCgroupsPath)

	// Walk through all cgroups and get cgroup paths
	if err := filepath.WalkDir(c.slurmCgroupsPath, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.Contains(p, "/job_") && !strings.HasSuffix(p, "/slurm") &&
			!strings.HasSuffix(p, "/user") {
			// Ignore step jobs
			if strings.Contains(p, "/step_") {
				return nil
			}
			rel, _ := filepath.Rel(c.cgroupsRootPath, p)
			level.Debug(c.logger).Log("msg", "cgroup path", "path", p, "rel", rel)
			names = append(names, "/"+rel)
		}
		return nil
	}); err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.slurmCgroupsPath, "err", err)
		return metrics, nil
	}

	// Get currently active jobs and set them in activeJobs state variable
	var activeJobIDs []string
	for _, name := range names {
		if matches := findNamedMatches(slurmPatterns[c.cgroups], name); matches["jobid"] != "" {
			jobid := matches["jobid"]
			activeJobIDs = append(activeJobIDs, jobid)
			c.jobPropsCache.LoadOrStore(jobid, JobProps{})
		}
	}

	// Remove all jobs from activeJobs which are not in activeJobIDs. These are generally
	// finished jobs
	c.jobPropsCache.Range(func(jobid, jobProps interface{}) bool {
		if !slices.Contains(activeJobIDs, jobid.(string)) {
			c.jobPropsCache.Delete(jobid)
			level.Debug(c.logger).Log("msg", "Removed job from jobPropsCache", "jobid", jobid)
		}
		return true
	})

	wg := &sync.WaitGroup{}
	wg.Add(len(names))
	for _, name := range names {
		go func(n string) {
			metric, _ := c.getMetrics(n)
			if !metric.err {
				metricLock.Lock()
				metrics[metric.jobid] = metric
				metricLock.Unlock()
			}
			wg.Done()
		}(name)
	}
	wg.Wait()
	return metrics, nil
}

// Get metrics of a given SLURM cgroups path
func (c *slurmCollector) getMetrics(name string) (CgroupMetric, error) {
	if c.cgroups == "v2" {
		return c.getCgroupsV2Metrics(name)
	} else {
		return c.getCgroupsV1Metrics(name)
	}
}

// Parse cpuset.cpus file to return a list of CPUs in the cgroup
func (c *slurmCollector) parseCPUSet(cpuset string) ([]string, error) {
	var cpus []string
	var start, end int
	var err error
	if cpuset == "" {
		return nil, nil
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

// Get list of CPUs in the cgroup
func (c *slurmCollector) getCPUs(name string) ([]string, error) {
	var cpusPath string
	if c.cgroups == "v2" {
		cpusPath = fmt.Sprintf("%s%s/cpuset.cpus.effective", *cgroupfsPath, name)
	} else {
		cpusPath = fmt.Sprintf("%s/cpuset%s/cpuset.cpus", *cgroupfsPath, name)
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

// Read prolog generated run time files to get job properties
func (c *slurmCollector) readJobPropsFromProlog(jobid string, jobProps *JobProps) JobProps {
	var gpuJobID string

	// Read SLURM job properties
	var slurmJobInfo = filepath.Join(*jobStatPath, jobid)
	if _, err := os.Stat(slurmJobInfo); err == nil {
		content, err := os.ReadFile(slurmJobInfo)
		if err != nil {
			level.Error(c.logger).
				Log("msg", "Failed to get job properties from prolog generated files", "file", slurmJobInfo, "err", err)
		} else {
			fmt.Sscanf(string(content), "%s %s %s", &jobProps.jobUser, &jobProps.jobAccount, &jobProps.jobNodelist)
		}
	}

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
	for i := 0; i < len(c.gpuDevs); i++ {
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
			fmt.Sscanf(string(content), "%s", &gpuJobID)
			if gpuJobID == jobid {
				jobProps.jobGPUOrdinals = append(jobProps.jobGPUOrdinals, dev.index)
			}
		}
	}
	return *jobProps
}

// Read job properties from env vars
func (c *slurmCollector) readJobPropsFromEnviron(jobid string, pids []uint64, jobProps *JobProps) JobProps {
	var jobUser string
	var jobAccount string
	var jobNodelist string
	var jobGPUOrdinals []string

	// Attempt to get UID, Account, Nodelist from /proc file system by looking into
	// environ for the process that has same SLURM_JOB_ID
	//
	// Instantiate a new Proc FS
	procFS, err := procfs.NewFS(*procfsPath)
	if err != nil {
		level.Error(c.logger).Log("msg", "Unable to open procfs", "path", *procfsPath, "err", err)
		return *jobProps
	}

	// Get all procs from current proc fs if passed pids slice is nil
	if pids == nil {
		allProcs, err := procFS.AllProcs()
		if err != nil {
			level.Error(c.logger).Log("msg", "Failed to read /proc", "err", err)
			return *jobProps
		}
		pids = make([]uint64, len(allProcs))
		for idx, proc := range allProcs {
			pids[idx] = uint64(proc.PID)
		}
	}

	// Env var that we will search
	jobIDEnv := fmt.Sprintf("SLURM_JOB_ID=%s", jobid)

	// Initialize a waitgroup for all go routines that we will spawn later
	wg := &sync.WaitGroup{}
	wg.Add(len(pids))

	// Iterate through all procs and look for SLURM_JOB_ID env entry
	for _, pid := range pids {
		go func(p int) {
			// Read process environment variables
			// NOTE: This needs CAP_SYS_PTRACE and CAP_DAC_READ_SEARCH caps
			// on the current process
			proc, err := procFS.Proc(p)
			if err != nil {
				wg.Done()
				return
			}
			environments, err := proc.Environ()

			// Skip if we cannot read file or job ID env var is not found
			if err != nil || !slices.Contains(environments, jobIDEnv) {
				wg.Done()
				return
			}

			// When env var entry found, get all necessary env vars
			// NOTE: This is not really concurrent safe. Multiple go routines might
			// overwrite the variables. But I think we can live with it as for a gievn
			// job cgroup these env vars should be identical in all procs
			for _, env := range environments {
				if strings.Contains(env, "SLURM_JOB_USER") {
					jobUser = strings.Split(env, "=")[1]
				}
				if strings.Contains(env, "SLURM_JOB_ACCOUNT") {
					jobAccount = strings.Split(env, "=")[1]
				}
				if strings.Contains(env, "SLURM_JOB_NODELIST") {
					jobNodelist = strings.Split(env, "=")[1]
				}
				if strings.Contains(env, "SLURM_STEP_GPUS") || strings.Contains(env, "SLURM_JOB_GPUS") {
					jobGPUOrdinals = strings.Split(strings.Split(env, "=")[1], ",")
				}
			}

			// Mark routine as done
			wg.Done()

		}(int(pid))
	}
	// Wait for all go routines to finish
	wg.Wait()

	// Set jobProps fields
	jobProps.jobUser = jobUser
	jobProps.jobAccount = jobAccount
	jobProps.jobNodelist = jobNodelist
	jobProps.jobGPUOrdinals = jobGPUOrdinals
	return *jobProps
}

// Check if job properties are set
func (c *slurmCollector) emptyJobProps(jobProps JobProps) bool {
	return jobProps.jobUser == "" || jobProps.jobAccount == "" || jobProps.jobNodelist == "" ||
		(len(jobProps.jobGPUOrdinals) == 0 && len(c.gpuDevs) > 0)
}

// Get different properties of Job
func (c *slurmCollector) getJobProperties(name string, metric *CgroupMetric, pids []uint64) {
	var jobProps JobProps
	var jobid string
	var err error

	// Get jobid first
	if matches := findNamedMatches(slurmPatterns[c.cgroups], name); matches["jobid"] != "" {
		jobid = matches["jobid"]
	} else {
		// If no job ID found, skip rest as we cannot get properties of an unknown job
		level.Warn(c.logger).Log("msg", "Unable to get job ID for cgroup", "path", name)
		return
	}
	metric.jobid = jobid
	metric.jobuuid = jobid

	// Attempt to get props from jobPropsCache state variable
	if value, ok := c.jobPropsCache.Load(jobid); ok {
		jobProps = value.(JobProps)
	}
	level.Debug(c.logger).Log(
		"msg", "Job properties from jobPropsCache", "jobid", jobid,
		"job_user", jobProps.jobUser, "job_account", jobProps.jobAccount,
		"job_nodelist", jobProps.jobNodelist,
	)

	// First try to read files that might be created by SLURM prolog scripts
	if c.emptyJobProps(jobProps) {
		jobProps = c.readJobPropsFromProlog(jobid, &jobProps)
		level.Debug(c.logger).Log(
			"msg", "Updated job properties from prolog generated files", "jobid", jobid,
			"job_user", jobProps.jobUser, "job_account", jobProps.jobAccount,
			"job_nodelist", jobProps.jobNodelist,
			"job_gpus", strings.Join(jobProps.jobGPUOrdinals, ","),
		)
	}

	// If we fail to get any of the job properties or if there are atleast one GPU devices
	// and if we fail to get gpu ordinals for that job, try to get these properties
	// by looking into environment variables
	if c.emptyJobProps(jobProps) {
		jobProps = c.readJobPropsFromEnviron(jobid, pids, &jobProps)
		level.Debug(c.logger).Log(
			"msg", "Updated job properties from environ", "jobid", jobid,
			"job_user", jobProps.jobUser, "job_account", jobProps.jobAccount,
			"job_nodelist", jobProps.jobNodelist,
			"job_gpus", strings.Join(jobProps.jobGPUOrdinals, ","),
		)
	}

	// Emit a warning if we could not get all job properties
	if jobProps.jobUser == "" || jobProps.jobAccount == "" {
		level.Warn(c.logger).Log(
			"msg", "Failed to get at leats one job property", "jobid", jobid, "job_user",
			jobProps.jobUser, "job_account", jobProps.jobAccount,
			"job_gpus", strings.Join(jobProps.jobGPUOrdinals, ","),
		)
	}
	// Emit warning when there are GPUs but no job to GPU map found
	if len(c.gpuDevs) > 0 && len(jobProps.jobGPUOrdinals) == 0 {
		level.Warn(c.logger).
			Log("msg", "Failed to get GPU ordinals for job", "jobid", jobid, "job_user", jobProps.jobUser)
	}

	// Compute a UUID using job properties if asked. If not set UUID to job ID
	if *useJobIDHash && jobProps.jobUUID == "" {
		jobProps.jobUUID, err = helpers.GetUUIDFromString(
			[]string{
				strings.TrimSpace(jobid),
				strings.TrimSpace(jobProps.jobUser),
				strings.ToLower(strings.TrimSpace(jobProps.jobAccount)),
				strings.ToLower(strings.TrimSpace(jobProps.jobNodelist)),
			},
		)
		if err != nil {
			level.Error(c.logger).
				Log("msg", "Failed to generate UUID for job", "jobid", "job_user", jobid, "err", err)
			jobProps.jobUUID = jobid
		}
	} else {
		jobProps.jobUUID = jobid
	}
	metric.jobuser = jobProps.jobUser
	metric.jobuuid = jobProps.jobUUID
	metric.jobaccount = jobProps.jobAccount
	metric.jobgpuordinals = jobProps.jobGPUOrdinals

	// Finally add jobProps to jobPropsCache
	c.jobPropsCache.Swap(jobid, jobProps)
}

// Get metrics from cgroups v1
func (c *slurmCollector) getCgroupsV1Metrics(name string) (CgroupMetric, error) {
	metric := CgroupMetric{name: name}
	metric.err = false
	level.Debug(c.logger).Log("msg", "Loading cgroup v1", "path", name)
	ctrl, err := cgroup1.Load(cgroup1.StaticPath(name), cgroup1.WithHierarchy(subsystem))
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to load cgroups", "path", name, "err", err)
		metric.err = true
		return metric, err
	}

	// Load cgroup stats
	stats, err := ctrl.Stat(cgroup1.IgnoreNotExist)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to stat cgroups", "path", name, "err", err)
		return metric, err
	}
	if stats == nil {
		level.Error(c.logger).Log("msg", "Cgroup stats are nil", "path", name)
		return metric, err
	}

	// Get CPU stats
	if stats.CPU != nil {
		if stats.CPU.Usage != nil {
			metric.cpuUser = float64(stats.CPU.Usage.User) / 1000000000.0
			metric.cpuSystem = float64(stats.CPU.Usage.Kernel) / 1000000000.0
			metric.cpuTotal = float64(stats.CPU.Usage.Total) / 1000000000.0
		}
	}
	if cpus, err := c.getCPUs(name); err == nil {
		metric.cpus = len(cpus)
	}

	// Get memory stats
	if stats.Memory != nil {
		metric.memoryRSS = float64(stats.Memory.TotalRSS)
		metric.memoryCache = float64(stats.Memory.TotalCache)
		if stats.Memory.Usage != nil {
			metric.memoryUsed = float64(stats.Memory.Usage.Usage)
			// If memory usage limit is set as "max", cgroups lib will set it to
			// math.MaxUint64. Here we replace it with total system memory
			if stats.Memory.Usage.Limit == math.MaxUint64 && c.hostMemTotal != 0 {
				metric.memoryTotal = c.hostMemTotal
			} else {
				metric.memoryTotal = float64(stats.Memory.Usage.Limit)
			}
			metric.memoryFailCount = float64(stats.Memory.Usage.Failcnt)
		}
		if stats.Memory.Swap != nil {
			metric.memswUsed = float64(stats.Memory.Swap.Usage)
			// If memory usage limit is set as "max", cgroups lib will set it to
			// math.MaxUint64. Here we replace it with total system memory
			if stats.Memory.Swap.Limit == math.MaxUint64 && c.hostMemTotal != 0 {
				metric.memswTotal = c.hostMemTotal
			} else {
				metric.memswTotal = float64(stats.Memory.Swap.Limit)
			}
			metric.memswFailCount = float64(stats.Memory.Swap.Failcnt)
		}
	}

	// Get RDMA metrics if available
	if stats.Rdma != nil {
		metric.rdmaHCAHandles = make(map[string]float64)
		metric.rdmaHCAObjects = make(map[string]float64)
		for _, device := range stats.Rdma.Current {
			metric.rdmaHCAHandles[device.Device] = float64(device.HcaHandles)
			metric.rdmaHCAObjects[device.Device] = float64(device.HcaObjects)
		}
	}

	// Get job Info
	c.getJobProperties(name, &metric, nil)
	return metric, nil
}

// Get Job metrics from cgroups v2
func (c *slurmCollector) getCgroupsV2Metrics(name string) (CgroupMetric, error) {
	metric := CgroupMetric{name: name}
	metric.err = false
	level.Debug(c.logger).Log("msg", "Loading cgroup v2", "path", name)

	// Load cgroups
	ctrl, err := cgroup2.Load(name, cgroup2.WithMountpoint(*cgroupfsPath))
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to load cgroups", "path", name, "err", err)
		metric.err = true
		return metric, err
	}

	// Get stats from cgroup
	stats, err := ctrl.Stat()
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to stat cgroups", "path", name, "err", err)
		return metric, err
	}
	if stats == nil {
		level.Error(c.logger).Log("msg", "Cgroup stats are nil", "path", name)
		return metric, err
	}

	// Get CPU stats
	if stats.CPU != nil {
		metric.cpuUser = float64(stats.CPU.UserUsec) / 1000000.0
		metric.cpuSystem = float64(stats.CPU.SystemUsec) / 1000000.0
		metric.cpuTotal = float64(stats.CPU.UsageUsec) / 1000000.0
		if stats.CPU.PSI != nil {
			metric.cpuPressure = float64(stats.CPU.PSI.Full.Total) / 1000000.0
		}
	}
	if cpus, err := c.getCPUs(name); err == nil {
		metric.cpus = len(cpus)
	}

	// Get memory stats
	// cgroups2 does not expose swap memory events. So we dont set memswFailCount
	if stats.Memory != nil {
		metric.memoryUsed = float64(stats.Memory.Usage)
		// If memory usage limit is set as "max", cgroups lib will set it to
		// math.MaxUint64. Here we replace it with total system memory
		if stats.Memory.UsageLimit == math.MaxUint64 && c.hostMemTotal > 0 {
			metric.memoryTotal = c.hostMemTotal
		} else {
			metric.memoryTotal = float64(stats.Memory.UsageLimit)
		}
		metric.memoryCache = float64(stats.Memory.File) // This is page cache
		metric.memoryRSS = float64(stats.Memory.Anon)
		metric.memswUsed = float64(stats.Memory.SwapUsage)
		// If memory usage limit is set as "max", cgroups lib will set it to
		// math.MaxUint64. Here we replace it with total system memory
		if stats.Memory.SwapLimit == math.MaxUint64 && c.hostMemTotal > 0 {
			metric.memswTotal = c.hostMemTotal
		} else {
			metric.memswTotal = float64(stats.Memory.SwapLimit)
		}
		if stats.Memory.PSI != nil {
			metric.memoryPressure = float64(stats.Memory.PSI.Full.Total) / 1000000.0
		}
	}
	// Get memory events
	if stats.MemoryEvents != nil {
		metric.memoryFailCount = float64(stats.MemoryEvents.Oom)
	}

	// Get RDMA stats
	if stats.Rdma != nil {
		metric.rdmaHCAHandles = make(map[string]float64)
		metric.rdmaHCAObjects = make(map[string]float64)
		for _, device := range stats.Rdma.Current {
			metric.rdmaHCAHandles[device.Device] = float64(device.HcaHandles)
			metric.rdmaHCAObjects[device.Device] = float64(device.HcaObjects)
		}
	}

	// Get job Info
	cgroupProcPids, err := ctrl.Procs(true)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to get proc pids in cgroup", "path", name)
	}

	// Get job Info
	c.getJobProperties(name, &metric, cgroupProcPids)
	return metric, nil
}
