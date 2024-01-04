//go:build !noslurm
// +build !noslurm

package collector

import (
	"fmt"
	"io/fs"
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
	"github.com/mahendrapaipuri/batchjob_monitoring/internal/helpers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const slurmCollectorSubsystem = "slurm_job"

var (
	metricLock      = sync.RWMutex{}
	collectJobSteps = BatchJobExporterApp.Flag(
		"collector.slurm.jobsteps.metrics",
		`Enables collection of metrics of all slurm job steps and tasks (default: disabled).
[WARNING: This option can result in very high cardinality of metrics]`,
	).Default("false").Bool()
	collectSwapMemoryStats = BatchJobExporterApp.Flag(
		"collector.slurm.swap.memory.metrics",
		"Enables collection of swap memory metrics (default: disabled)",
	).Default("false").Bool()
	collectPSIStats = BatchJobExporterApp.Flag(
		"collector.slurm.psi.metrics",
		"Enables collection of PSI metrics (default: disabled)",
	).Default("false").Bool()
	useJobIdHash = BatchJobExporterApp.Flag(
		"collector.slurm.create.unique.jobids",
		`Enables calculation of a unique hash based job UUID (default: disabled). 
UUID is calculated based on SLURM_JOBID, SLURM_JOB_UID, SLURM_JOB_ACCOUNT, SLURM_JOB_NODELIST.`,
	).Default("false").Bool()
	jobStatPath = BatchJobExporterApp.Flag(
		"collector.slurm.job.props.path",
		`Directory containing files with job properties. Files should be named after SLURM_JOBID 
with contents as "$SLURM_JOB_UID $SLURM_JOB_ACCOUNT $SLURM_JOB_NODELIST" in the same order.`,
	).Default("/run/slurmjobprops").String()
	gpuStatPath = BatchJobExporterApp.Flag(
		"collector.slurm.nvidia.gpu.job.map.path",
		"Path to file that maps GPU ordinals to job IDs.",
	).Default("/run/gpujobmap").String()
	nvidiaSmiPath = BatchJobExporterApp.Flag(
		"collector.slurm.nvidia.smi.path",
		"Absolute path to nvidia-smi executable.",
	).Default("/usr/bin/nvidia-smi").String()
	forceCgroupsVersion = BatchJobExporterApp.Flag(
		"collector.slurm.force.cgroups.version",
		"Set cgroups version manually. Used only for testing.",
	).Hidden().Enum("v1", "v2")
)

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
	userslice       bool
	batch           string
	hostname        string
	jobuid          string
	jobaccount      string
	jobid           string
	jobuuid         string
	jobGpuOrdinals  []string
	step            string
	task            string
	err             bool
}

type slurmCollector struct {
	cgroups          string // v1 or v2
	cgroupsRootPath  string
	slurmCgroupsPath string
	hostname         string
	gpuDevs          map[int]Device
	cpuUser          *prometheus.Desc
	cpuSystem        *prometheus.Desc
	cpuTotal         *prometheus.Desc
	cpus             *prometheus.Desc
	cpuPressure      *prometheus.Desc
	memoryRSS        *prometheus.Desc
	memoryCache      *prometheus.Desc
	memoryUsed       *prometheus.Desc
	memoryTotal      *prometheus.Desc
	memoryFailCount  *prometheus.Desc
	memswUsed        *prometheus.Desc
	memswTotal       *prometheus.Desc
	memswFailCount   *prometheus.Desc
	memoryPressure   *prometheus.Desc
	gpuJobMap        *prometheus.Desc
	gpuJobFlag       *prometheus.Desc
	collectError     *prometheus.Desc
	logger           log.Logger
}

func init() {
	RegisterCollector(slurmCollectorSubsystem, defaultEnabled, NewSlurmCollector)
}

// NewSlurmCollector returns a new Collector exposing a summary of cgroups.
func NewSlurmCollector(logger log.Logger) (Collector, error) {
	var cgroupsVersion string
	var cgroupsRootPath string
	var slurmCgroupsPath string
	var hostname string
	var err error

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

	// Get hostname
	if !*emptyHostnameLabel {
		hostname, err = os.Hostname()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get hostname", "err", err)
		}
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
	if _, err := os.Stat(*nvidiaSmiPath); err == nil {
		gpuDevs, err = GetNvidiaGPUDevices(*nvidiaSmiPath, logger)
		if err == nil {
			level.Info(logger).Log("msg", "nVIDIA GPU devices found")
		}
	}
	return &slurmCollector{
		cgroups:          cgroupsVersion,
		cgroupsRootPath:  cgroupsRootPath,
		slurmCgroupsPath: slurmCgroupsPath,
		hostname:         hostname,
		gpuDevs:          gpuDevs,
		cpuUser: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "cpu_user_seconds"),
			"Cumulative CPU user seconds",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpuSystem: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "cpu_system_seconds"),
			"Cumulative CPU system seconds",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpuTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "cpu_total_seconds"),
			"Cumulative CPU total seconds",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpus: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "cpus"),
			"Number of CPUs",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpuPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "cpu_psi_seconds"),
			"Cumulative CPU PSI seconds",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryRSS: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memory_rss_bytes"),
			"Memory RSS used in bytes",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryCache: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memory_cache_bytes"),
			"Memory cache used in bytes",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memory_used_bytes"),
			"Memory used in bytes",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memory_total_bytes"),
			"Memory total in bytes",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memory_fail_count"),
			"Memory fail count",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memswUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memsw_used_bytes"),
			"Swap used in bytes",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memswTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memsw_total_bytes"),
			"Swap total in bytes",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memswFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memsw_fail_count"),
			"Swap fail count",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "memory_psi_seconds"),
			"Cumulative memory PSI seconds",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		collectError: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "collect_error"),
			"Indicates collection error, 0=no error, 1=error",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		gpuJobFlag: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "nvidia_gpu_jobid_flag"),
			"Indicates running job on GPU, 1=job running",
			[]string{"batch", "hostname", "jobid", "jobaccount", "jobuuid", "index", "uuid", "UUID"}, nil,
		),
		gpuJobMap: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, slurmCollectorSubsystem, "nvidia_gpu_jobid"),
			"Batch Job ID of current nVIDIA GPU",
			[]string{"batch", "hostname", "index", "uuid", "UUID"}, nil,
		),
		logger: logger,
	}, nil
}

// Return cgroups v1 subsystem
func subsystem() ([]cgroup1.Subsystem, error) {
	s := []cgroup1.Subsystem{
		cgroup1.NewCpuacct(*cgroupfsPath),
		cgroup1.NewMemory(*cgroupfsPath),
	}
	return s, nil
}

// Update implements Collector and exposes cgroup statistics.
func (c *slurmCollector) Update(ch chan<- prometheus.Metric) error {
	metrics, err := c.getJobsMetrics()
	if err != nil {
		return err
	}
	for n, m := range metrics {
		// Convert job id to int
		jid, err := strconv.Atoi(m.jobid)
		if err != nil {
			level.Debug(c.logger).Log("msg", "Failed to convert SLURM jobID to int", "jobID", m.jobid)
			jid = 0
		}
		if m.err {
			ch <- prometheus.MustNewConstMetric(c.collectError, prometheus.GaugeValue, 1, m.name)
		}
		ch <- prometheus.MustNewConstMetric(c.cpuUser, prometheus.GaugeValue, m.cpuUser, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.cpuSystem, prometheus.GaugeValue, m.cpuSystem, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.cpuTotal, prometheus.GaugeValue, m.cpuTotal, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		cpus := m.cpus
		if cpus == 0 {
			dir := filepath.Dir(n)
			cpus = metrics[dir].cpus
			if cpus == 0 {
				cpus = metrics[filepath.Dir(dir)].cpus
			}
		}
		ch <- prometheus.MustNewConstMetric(c.cpus, prometheus.GaugeValue, float64(cpus), m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryRSS, prometheus.GaugeValue, m.memoryRSS, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryCache, prometheus.GaugeValue, m.memoryCache, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryUsed, prometheus.GaugeValue, m.memoryUsed, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryTotal, prometheus.GaugeValue, m.memoryTotal, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryFailCount, prometheus.GaugeValue, m.memoryFailCount, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		if *collectSwapMemoryStats {
			ch <- prometheus.MustNewConstMetric(c.memswUsed, prometheus.GaugeValue, m.memswUsed, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
			ch <- prometheus.MustNewConstMetric(c.memswTotal, prometheus.GaugeValue, m.memswTotal, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
			ch <- prometheus.MustNewConstMetric(c.memswFailCount, prometheus.GaugeValue, m.memswFailCount, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		}
		if *collectPSIStats {
			ch <- prometheus.MustNewConstMetric(c.cpuPressure, prometheus.GaugeValue, m.cpuPressure, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
			ch <- prometheus.MustNewConstMetric(c.memoryPressure, prometheus.GaugeValue, m.memoryPressure, m.batch, m.hostname, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		}
		for _, gpuOrdinal := range m.jobGpuOrdinals {
			var uuid string
			// Check the int index of devices where gpuOrdinal == dev.index
			for _, dev := range c.gpuDevs {
				if gpuOrdinal == dev.index {
					uuid = dev.uuid
					break
				}
			}
			ch <- prometheus.MustNewConstMetric(c.gpuJobMap, prometheus.GaugeValue, float64(jid), m.batch, c.hostname, gpuOrdinal, uuid, uuid)
			ch <- prometheus.MustNewConstMetric(c.gpuJobFlag, prometheus.GaugeValue, float64(1), m.batch, c.hostname, m.jobid, m.jobaccount, m.jobuuid, gpuOrdinal, uuid, uuid)
		}
	}
	return nil
}

// Get current Jobs metrics from cgroups
func (c *slurmCollector) getJobsMetrics() (map[string]CgroupMetric, error) {
	var names []string
	var metrics = make(map[string]CgroupMetric)

	level.Debug(c.logger).Log("msg", "Loading cgroup", "path", c.slurmCgroupsPath)

	err := filepath.WalkDir(c.slurmCgroupsPath, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.Contains(p, "/job_") && !strings.HasSuffix(p, "/slurm") &&
			!strings.HasSuffix(p, "/user") {
			if !*collectJobSteps && strings.Contains(p, "/step_") {
				return nil
			}
			rel, _ := filepath.Rel(c.cgroupsRootPath, p)
			level.Debug(c.logger).Log("msg", "Get cgroup Name", "name", p, "rel", rel)
			names = append(names, "/"+rel)
		}
		return nil
	})
	if err != nil {
		level.Error(c.logger).
			Log("msg", "Error walking cgroup subsystem", "path", c.slurmCgroupsPath, "err", err)
		return metrics, nil
	}

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
func (c *slurmCollector) parseCpuSet(cpuset string) ([]string, error) {
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
		return nil, nil
	}
	cpusData, err := os.ReadFile(cpusPath)
	if err != nil {
		level.Error(c.logger).Log("msg", "Error reading cpuset", "cpuset", cpusPath, "err", err)
		return nil, err
	}
	cpus, err := c.parseCpuSet(strings.TrimSuffix(string(cpusData), "\n"))
	if err != nil {
		level.Error(c.logger).Log("msg", "Error parsing cpu set", "cpuset", cpusPath, "err", err)
		return nil, err
	}
	return cpus, nil
}

// Get different properties of Job
func (c *slurmCollector) getJobProperties(metric *CgroupMetric, pids []uint64) {
	jobid := metric.jobid
	var jobUuid string
	var jobUid string = ""
	var jobAccount string = ""
	var jobNodelist string = ""
	var gpuJobId string = ""
	var jobGpuOrdinals []string
	var err error

	// First try to read files that might be created by SLURM prolog scripts
	var slurmJobInfo = fmt.Sprintf("%s/%s", *jobStatPath, jobid)
	if _, err := os.Stat(slurmJobInfo); err == nil {
		content, err := os.ReadFile(slurmJobInfo)
		if err != nil {
			level.Error(c.logger).
				Log("msg", "Failed to get metadata for job", "jobid", jobid, "err", err)
		} else {
			fmt.Sscanf(string(content), "%s %s %s", &jobUid, &jobAccount, &jobNodelist)
		}
	}

	// If there are no GPUs this loop will be skipped anyways
	// NOTE: In go loop over map is not reproducible. The order is undefined and thus
	// we might end up with a situation where jobGpuOrdinals will [1 2] or [2 1] if
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
			fmt.Sscanf(string(content), "%s", &gpuJobId)
			if gpuJobId == jobid {
				jobGpuOrdinals = append(jobGpuOrdinals, dev.index)
			}
		}
	}

	// If we fail to get any of the job properties or if there are atleast one GPU devices
	// and if we fail to get gpu ordinals for that job, try to get these properties
	// by looking into environment variables
	if jobUid == "" || jobAccount == "" || jobNodelist == "" || (len(jobGpuOrdinals) == 0 && len(c.gpuDevs) > 0) {
		// Attempt to get UID, Account, Nodelist from /proc file system by looking into
		// environ for the process that has same SLURM_JOB_ID
		//
		// Instantiate a new Proc FS
		procFS, err := procfs.NewFS(*procfsPath)
		if err != nil {
			level.Error(c.logger).Log("msg", "Unable to open procfs", "path", *procfsPath)
			goto outside
		}

		// Get all procs from current proc fs if passed pids slice is nil
		if pids == nil {
			allProcs, err := procFS.AllProcs()
			if err != nil {
				level.Error(c.logger).Log("msg", "Failed to read /proc", "err", err)
				goto outside
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
				for _, env := range environments {
					if strings.Contains(env, "SLURM_JOB_UID") {
						jobUid = strings.Split(env, "=")[1]
					}
					if strings.Contains(env, "SLURM_JOB_ACCOUNT") {
						jobAccount = strings.Split(env, "=")[1]
					}
					if strings.Contains(env, "SLURM_JOB_NODELIST") {
						jobNodelist = strings.Split(env, "=")[1]
					}
					if strings.Contains(env, "SLURM_STEP_GPUS") || strings.Contains(env, "SLURM_JOB_GPUS") {
						jobGpuOrdinals = strings.Split(strings.Split(env, "=")[1], ",")
					}
				}

				// Mark routine as done
				wg.Done()

			}(int(pid))
		}
		// Wait for all go routines to finish
		wg.Wait()
	}

outside:
	// Emit a warning if we could not get all job properties
	if jobUid == "" && jobAccount == "" && jobNodelist == "" {
		level.Warn(c.logger).
			Log("msg", "Failed to get job properties", "jobid", jobid)
	}
	// Emit warning when there are GPUs but no job to GPU map found
	if len(c.gpuDevs) > 0 && len(jobGpuOrdinals) == 0 {
		level.Warn(c.logger).
			Log("msg", "Failed to get GPU ordinals for job", "jobid", jobid)
	}

	// Get UUID using job properties
	if *useJobIdHash {
		jobUuid, err = helpers.GetUuidFromString(
			[]string{
				strings.TrimSpace(jobid),
				strings.TrimSpace(jobUid),
				strings.ToLower(strings.TrimSpace(jobAccount)),
				strings.ToLower(strings.TrimSpace(jobNodelist)),
			},
		)
		if err != nil {
			level.Error(c.logger).
				Log("msg", "Failed to generate UUID for job", "jobid", jobid, "err", err)
			jobUuid = jobid
		}
	}
	metric.jobuid = jobUid
	metric.jobuuid = jobUuid
	metric.jobaccount = jobAccount
	metric.jobGpuOrdinals = jobGpuOrdinals
}

// Get job details from cgroups v1
func (c *slurmCollector) getInfoV1(name string, metric *CgroupMetric) {
	// var err error
	pathBase := filepath.Base(name)
	userSlicePattern := regexp.MustCompile("^user-([0-9]+).slice$")
	userSliceMatch := userSlicePattern.FindStringSubmatch(pathBase)
	if len(userSliceMatch) == 2 {
		metric.userslice = true
	}

	// Get job ID, step and task
	slurmPattern := regexp.MustCompile(
		"^/slurm/uid_([0-9]+)/job_([0-9]+)(/step_([^/]+)(/task_([[0-9]+))?)?$",
	)
	slurmMatch := slurmPattern.FindStringSubmatch(name)
	level.Debug(c.logger).
		Log("msg", "Got for match", "name", name, "len(slurmMatch)", len(slurmMatch), "slurmMatch", fmt.Sprintf("%v", slurmMatch))
	if len(slurmMatch) >= 3 {
		metric.jobid = slurmMatch[2]
		metric.step = slurmMatch[4]
		metric.task = slurmMatch[6]
		return
	}
}

// Get metrics from cgroups v1
func (c *slurmCollector) getCgroupsV1Metrics(name string) (CgroupMetric, error) {
	metric := CgroupMetric{name: name, batch: "slurm", hostname: c.hostname}
	metric.err = false
	level.Debug(c.logger).Log("msg", "Loading cgroup v1", "path", name)
	ctrl, err := cgroup1.Load(cgroup1.StaticPath(name), cgroup1.WithHiearchy(subsystem))
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
			metric.memoryTotal = float64(stats.Memory.Usage.Limit)
			metric.memoryFailCount = float64(stats.Memory.Usage.Failcnt)
		}
		if stats.Memory.Swap != nil {
			metric.memswUsed = float64(stats.Memory.Swap.Usage)
			metric.memswTotal = float64(stats.Memory.Swap.Limit)
			metric.memswFailCount = float64(stats.Memory.Swap.Failcnt)
		}
	}

	// Get cgroup info
	c.getInfoV1(name, &metric)

	// Get job Info
	c.getJobProperties(&metric, nil)
	return metric, nil
}

// Get Job info for cgroups v2
func (c *slurmCollector) getInfoV2(name string, metric *CgroupMetric) {
	// possibilities are /system.slice/slurmstepd.scope/job_211
	//                   /system.slice/slurmstepd.scope/job_211/step_interactive
	//                   /system.slice/slurmstepd.scope/job_211/step_extern/user/task_0
	// we dont get userslice
	metric.userslice = false
	slurmPattern := regexp.MustCompile(
		"^/system.slice/slurmstepd.scope/job_([0-9]+)(/step_([^/]+)(/user/task_([[0-9]+))?)?$",
	)
	slurmMatch := slurmPattern.FindStringSubmatch(name)
	level.Debug(c.logger).
		Log("msg", "Got for match", "name", name, "len(slurmMatch)", len(slurmMatch), "slurmMatch", fmt.Sprintf("%v", slurmMatch))
	if len(slurmMatch) == 6 {
		metric.jobid = slurmMatch[1]
		metric.step = slurmMatch[3]
		metric.task = slurmMatch[5]
	}
}

// Get Job metrics from cgroups v2
func (c *slurmCollector) getCgroupsV2Metrics(name string) (CgroupMetric, error) {
	metric := CgroupMetric{name: name, batch: "slurm", hostname: c.hostname}
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
		metric.memoryTotal = float64(stats.Memory.UsageLimit)
		metric.memoryCache = float64(stats.Memory.File) // This is page cache
		metric.memoryRSS = float64(stats.Memory.Anon)
		metric.memswUsed = float64(stats.Memory.SwapUsage)
		metric.memswTotal = float64(stats.Memory.SwapLimit)
		if stats.Memory.PSI != nil {
			metric.memoryPressure = float64(stats.Memory.PSI.Full.Total) / 1000000.0
		}
	}
	// Get memory events
	if stats.MemoryEvents != nil {
		metric.memoryFailCount = float64(stats.MemoryEvents.Oom)
	}

	// Get cgroup Info
	c.getInfoV2(name, &metric)

	// Get job Info
	cgroupProcPids, err := ctrl.Procs(true)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to get proc pids in cgroup", "path", name)
	}

	// Get job Info
	c.getJobProperties(&metric, cgroupProcPids)
	return metric, nil
}
