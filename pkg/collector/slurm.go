//go:build !noslurm
// +build !noslurm

package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/internal/helpers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const slurmCollectorSubsystem = "slurm_job"

var (
	cgroupsV2       = false
	metricLock      = sync.RWMutex{}
	collectJobSteps = kingpin.Flag(
		"collector.slurm.jobsteps.metrics",
		`Whether to collect metrics of all slurm job steps and tasks 
[WARNING: This option can result in very high cardinality of metrics].`,
	).Default("false").Bool()
	useJobIdHash = kingpin.Flag(
		"collector.slurm.create.unique.jobids",
		`Whether to calculate a hash based job ID based on SLURM_JOBID, SLURM_JOB_UID, 
SLURM_JOB_ACCOUNT, SLURM_JOB_NODELIST to get unique job identifier.`,
	).Default("false").Bool()
	jobStatPath = kingpin.Flag(
		"collector.slurm.job.props.path",
		`Path to jobstat files that contains a file for each job with line 
\"$SLURM_JOB_UID $SLURM_JOB_ACCOUNT $SLURM_JOB_NODELIST\". 
An deterministic UUID is computed on the variables in this file and job ID to get an 
unique job identifier.`,
	).Default("/run/slurmjobprops").String()
)

type CgroupMetric struct {
	name            string
	cpuUser         float64
	cpuSystem       float64
	cpuTotal        float64
	cpus            int
	memoryRSS       float64
	memoryCache     float64
	memoryUsed      float64
	memoryTotal     float64
	memoryFailCount float64
	memswUsed       float64
	memswTotal      float64
	memswFailCount  float64
	userslice       bool
	jobuid          string
	jobaccount      string
	jobid           string
	jobuuid         string
	step            string
	task            string
	batch           string
	err             bool
}

type slurmCollector struct {
	cgroups          string // v1 or v2
	cgroupsRootPath  string
	slurmCgroupsPath string
	cpuUser          *prometheus.Desc
	cpuSystem        *prometheus.Desc
	cpuTotal         *prometheus.Desc
	cpus             *prometheus.Desc
	memoryRSS        *prometheus.Desc
	memoryCache      *prometheus.Desc
	memoryUsed       *prometheus.Desc
	memoryTotal      *prometheus.Desc
	memoryFailCount  *prometheus.Desc
	memswUsed        *prometheus.Desc
	memswTotal       *prometheus.Desc
	memswFailCount   *prometheus.Desc
	collectError     *prometheus.Desc
	logger           log.Logger
}

func init() {
	registerCollector(slurmCollectorSubsystem, defaultEnabled, NewSlurmCollector)
}

// NewSlurmCollector returns a new Collector exposing a summary of cgroups.
func NewSlurmCollector(logger log.Logger) (Collector, error) {
	var cgroupsVer string
	var cgroupsRootPath string
	var slurmCgroupsPath string

	if cgroups.Mode() == cgroups.Unified {
		cgroupsVer = "v2"
		level.Info(logger).Log("msg", "Cgroup version v2 detected", "mount", *cgroupfsPath)
		cgroupsRootPath = *cgroupfsPath
		slurmCgroupsPath = fmt.Sprintf("%s/system.slice/slurmstepd.scope", *cgroupfsPath)
	} else {
		cgroupsVer = "v1"
		level.Info(logger).Log("msg", "Cgroup version v2 not detected, will proceed with v1.")
		cgroupsRootPath = fmt.Sprintf("%s/cpuacct", *cgroupfsPath)
		slurmCgroupsPath = fmt.Sprintf("%s/slurm", cgroupsRootPath)
	}

	// Snippet for testing e2e tests for cgroups v1
	// cgroupsVer = "v1"
	// level.Info(logger).Log("msg", "Cgroup version v2 not detected, will proceed with v1.")
	// cgroupsRootPath = fmt.Sprintf("%s/cpuacct", *cgroupfsPath)
	// slurmCgroupsPath = fmt.Sprintf("%s/slurm", cgroupsRootPath)

	// Dont fail starting collector. Let it fail during scraping
	// Check if cgroups exist
	// if _, err := os.Stat(slurmCgroupsPath); err != nil {
	// 	level.Error(logger).Log("msg", "Slurm cgroups hierarchy not found", "path", slurmCgroupsPath, "err", err)
	// 	return nil, err
	// }

	return &slurmCollector{
		cgroups:          cgroupsVer,
		cgroupsRootPath:  cgroupsRootPath,
		slurmCgroupsPath: slurmCgroupsPath,
		cpuUser: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "cpu", "user_seconds"),
			"Cumulative CPU user seconds",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpuSystem: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "cpu", "system_seconds"),
			"Cumulative CPU system seconds",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpuTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "cpu", "total_seconds"),
			"Cumulative CPU total seconds",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		cpus: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "cpus"),
			"Number of CPUs",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryRSS: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "rss_bytes"),
			"Memory RSS used in bytes",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryCache: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "cache_bytes"),
			"Memory cache used in bytes",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "used_bytes"),
			"Memory used in bytes",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "total_bytes"),
			"Memory total in bytes",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memoryFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memory", "fail_count"),
			"Memory fail count",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memswUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memsw", "used_bytes"),
			"Swap used in bytes",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memswTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memsw", "total_bytes"),
			"Swap total in bytes",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		memswFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "memsw", "fail_count"),
			"Swap fail count",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
			nil,
		),
		collectError: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "exporter", "collect_error"),
			"Indicates collection error, 0=no error, 1=error",
			[]string{"batch", "jobid", "jobaccount", "jobuuid", "step", "task"},
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
		if m.err {
			ch <- prometheus.MustNewConstMetric(c.collectError, prometheus.GaugeValue, 1, m.name)
		}
		ch <- prometheus.MustNewConstMetric(c.cpuUser, prometheus.GaugeValue, m.cpuUser, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.cpuSystem, prometheus.GaugeValue, m.cpuSystem, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.cpuTotal, prometheus.GaugeValue, m.cpuTotal, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		cpus := m.cpus
		if cpus == 0 {
			dir := filepath.Dir(n)
			cpus = metrics[dir].cpus
			if cpus == 0 {
				cpus = metrics[filepath.Dir(dir)].cpus
			}
		}
		ch <- prometheus.MustNewConstMetric(c.cpus, prometheus.GaugeValue, float64(cpus), m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryRSS, prometheus.GaugeValue, m.memoryRSS, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryCache, prometheus.GaugeValue, m.memoryCache, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryUsed, prometheus.GaugeValue, m.memoryUsed, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryTotal, prometheus.GaugeValue, m.memoryTotal, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memoryFailCount, prometheus.GaugeValue, m.memoryFailCount, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memswUsed, prometheus.GaugeValue, m.memswUsed, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memswTotal, prometheus.GaugeValue, m.memswTotal, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
		ch <- prometheus.MustNewConstMetric(c.memswFailCount, prometheus.GaugeValue, m.memswFailCount, m.batch, m.jobid, m.jobaccount, m.jobuuid, m.step, m.task)
	}
	return nil
}

// Get current Jobs metrics from cgroups
func (c *slurmCollector) getJobsMetrics() (map[string]CgroupMetric, error) {
	var names []string
	var metrics = make(map[string]CgroupMetric)

	level.Debug(c.logger).Log("msg", "Loading cgroup", "path", c.slurmCgroupsPath)

	err := filepath.Walk(c.slurmCgroupsPath, func(p string, info os.FileInfo, err error) error {
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

	// if memory.max = "max" case we set memory max to -1
	// fix it by looking at the parent
	// we loop through names once as it was the result of Walk so top paths are seen first
	// also some cgroups we ignore, like path=/system.slice/slurmstepd.scope/job_216/step_interactive/user, hence the need to loop through multiple parents
	if c.cgroups == "v2" {
		for _, name := range names {
			metric, ok := metrics[name]
			if ok && metric.memoryTotal < 0 {
				for upName := name; len(upName) > 1; {
					upName = filepath.Dir(upName)
					upMetric, ok := metrics[upName]
					if ok {
						metric.memoryTotal = upMetric.memoryTotal
						metrics[name] = metric
					}
				}
			}
		}
	}
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

// Get different labels of Job
func (c *slurmCollector) getJobLabels(jobid string) (string, string, string) {
	var jobUuid string
	var jobUid string = ""
	var jobAccount string = ""
	var jobNodelist string = ""

	// If useJobIdHash is false return with empty strings
	if !*useJobIdHash {
		return jobUuid, jobUid, jobAccount
	}

	var slurmJobInfo = fmt.Sprintf("%s/%s", *jobStatPath, jobid)
	if _, err := os.Stat(slurmJobInfo); err == nil {
		content, err := os.ReadFile(slurmJobInfo)
		if err != nil {
			level.Error(c.logger).
				Log("msg", "Failed to get metadata for job", "jobid", jobid, "err", err)
		} else {
			fmt.Sscanf(string(content), "%s %s %s", &jobUid, &jobAccount, &jobNodelist)
		}
	} else {
		// Attempt to get UID, Account, Nodelist from /proc file system by looking into
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
			goto outside
		}

		// Env var that we will search
		jobIDEnv := fmt.Sprintf("SLURM_JOB_ID=%s", jobid)

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
				}

				// Mark routine as done
				wg.Done()

			}(proc)
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

	// Get UUID using job properties
	jobUuid, err := helpers.GetUuidFromString(
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
	return jobUuid, jobUid, jobAccount
}

// Get job details from cgroups v1
func (c *slurmCollector) getInfoV1(name string, metric *CgroupMetric) {
	// var err error
	pathBase := filepath.Base(name)
	userSlicePattern := regexp.MustCompile("^user-([0-9]+).slice$")
	userSliceMatch := userSlicePattern.FindStringSubmatch(pathBase)
	if len(userSliceMatch) == 2 {
		metric.userslice = true
		// metric.jobuid, err = userSliceMatch[1]
		// if err != nil {
		// 	level.Error(c.logger).Log("msg", "Error getting slurm job's uid number", "uid", pathBase, "err", err)
		// }
		// return
	}
	slurmPattern := regexp.MustCompile(
		"^/slurm/uid_([0-9]+)/job_([0-9]+)(/step_([^/]+)(/task_([[0-9]+))?)?$",
	)
	slurmMatch := slurmPattern.FindStringSubmatch(name)
	level.Debug(c.logger).
		Log("msg", "Got for match", "name", name, "len(slurmMatch)", len(slurmMatch), "slurmMatch", fmt.Sprintf("%v", slurmMatch))
	if len(slurmMatch) >= 3 {
		// metric.jobuid, err = slurmMatch[1]
		// if err != nil {
		// 	level.Error(c.logger).Log("msg", "Error getting slurm job's uid number", "uid", name, "err", err)
		// }
		metric.jobid = slurmMatch[2]
		metric.step = slurmMatch[4]
		metric.task = slurmMatch[6]
		return
	}
}

// Get metrics from cgroups v1
func (c *slurmCollector) getCgroupsV1Metrics(name string) (CgroupMetric, error) {
	metric := CgroupMetric{name: name, batch: "slurm"}
	metric.err = false
	level.Debug(c.logger).Log("msg", "Loading cgroup v1", "path", name)
	ctrl, err := cgroup1.Load(cgroup1.StaticPath(name), cgroup1.WithHiearchy(subsystem))
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to load cgroups", "path", name, "err", err)
		metric.err = true
		return metric, err
	}
	stats, err := ctrl.Stat(cgroup1.IgnoreNotExist)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to stat cgroups", "path", name, "err", err)
		return metric, err
	}
	if stats == nil {
		level.Error(c.logger).Log("msg", "Cgroup stats are nil", "path", name)
		return metric, err
	}
	if stats.CPU != nil {
		if stats.CPU.Usage != nil {
			metric.cpuUser = float64(stats.CPU.Usage.User) / 1000000000.0
			metric.cpuSystem = float64(stats.CPU.Usage.Kernel) / 1000000000.0
			metric.cpuTotal = float64(stats.CPU.Usage.Total) / 1000000000.0
		}
	}
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
	if cpus, err := c.getCPUs(name); err == nil {
		metric.cpus = len(cpus)
	}
	c.getInfoV1(name, &metric)
	metric.jobuuid, metric.jobuid, metric.jobaccount = c.getJobLabels(metric.jobid)
	return metric, nil
}

// Convenience function that will check if name+metric exists in the data
// and log an error if it does not. It returns 0 in such case but otherwise
// returns the value
func (c *slurmCollector) getOneMetric(
	name string,
	metric string,
	required bool,
	data map[string]float64,
) float64 {
	val, ok := data[metric]
	if !ok && required {
		level.Error(c.logger).Log("msg", "Failed to load", "metric", metric, "cgroup", name)
	}
	return val
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
	metric := CgroupMetric{name: name, batch: "slurm"}
	metric.err = false
	level.Debug(c.logger).Log("msg", "Loading cgroup v2", "path", name)
	// Files to parse out of the cgroup
	controllers := []string{
		"cpu.stat",
		"memory.current",
		"memory.events",
		"memory.max",
		"memory.stat",
	}
	data, err := LoadCgroupsV2Metrics(name, *cgroupfsPath, controllers)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to load cgroups v2", "path", name, "err", err)
		metric.err = true
		return metric, err
	}
	metric.cpuUser = c.getOneMetric(name, "cpu.stat.user_usec", true, data) / 1000000.0
	metric.cpuSystem = c.getOneMetric(name, "cpu.stat.system_usec", true, data) / 1000000.0
	metric.cpuTotal = c.getOneMetric(name, "cpu.stat.usage_usec", true, data) / 1000000.0
	// we use Oom entry from memory.events - it maps most closely to FailCount
	// TODO: add oom_kill as a separate value
	metric.memoryFailCount = c.getOneMetric(name, "memory.events.oom", true, data)
	// taking Slurm's cgroup v2 as inspiration, swapcached could be missing if swap is off so OK to ignore that case
	metric.memoryRSS = c.getOneMetric(
		name,
		"memory.stat.anon",
		true,
		data,
	) + c.getOneMetric(
		name,
		"memory.stat.swapcached",
		false,
		data,
	)
	// I guess?
	metric.memoryCache = c.getOneMetric(name, "memory.stat.file", true, data)
	metric.memoryUsed = c.getOneMetric(name, "memory.current", true, data)
	metric.memoryTotal = c.getOneMetric(name, "memory.max", true, data)
	metric.memswUsed = 0.0
	metric.memswTotal = 0.0
	metric.memswFailCount = 0.0
	if cpus, err := c.getCPUs(name); err == nil {
		metric.cpus = len(cpus)
	}
	c.getInfoV2(name, &metric)
	metric.jobuuid, metric.jobuid, metric.jobaccount = c.getJobLabels(metric.jobid)
	return metric, nil
}
