package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/cgroups/v3"
	"github.com/containerd/cgroups/v3/cgroup1"
	"github.com/containerd/cgroups/v3/cgroup2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Max cgroup subsystems count that is used from BPF side
	// to define a max index for the default controllers on tasks.
	// For further documentation check BPF part.
	cgroupSubSysCount = 15
	genericSubsystem  = "compute"
)

// Resource Managers.
const (
	slurm   = "slurm"
	libvirt = "libvirt"
)

// Regular expressions of cgroup paths for different resource managers.
/*
	For v1 possibilities are /cpuacct/slurm/uid_1000/job_211
							 /memory/slurm/uid_1000/job_211

	For v2 possibilities are /system.slice/slurmstepd.scope/job_211
							/system.slice/slurmstepd.scope/job_211/step_interactive
							/system.slice/slurmstepd.scope/job_211/step_extern/user/task_0
*/
var (
	slurmCgroupPathRegex  = regexp.MustCompile("^.*/slurm(?:.*?)/job_([0-9]+)(?:.*$)")
	slurmIgnoreProcsRegex = regexp.MustCompile("slurmstepd:(.*)|sleep ([0-9]+)|/bin/bash (.*)/slurm_script")
)

// Ref: https://libvirt.org/cgroups.html#legacy-cgroups-layout
// Take escaped unicode characters in regex
/*
	For v1 possibilities are /cpuacct/machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope
							 /memory/machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope

	For v2 possibilities are /machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope
							 /machine.slice/machine-qemu\x2d2\x2dinstance\x2d00000001.scope/libvirt
*/
var (
	libvirtCgroupPathRegex = regexp.MustCompile("^.*/(?:.+?)-qemu-(?:[0-9]+)-(instance-[0-9]+)(?:.*$)")
)

// CLI options.
var (
	activeController = CEEMSExporterApp.Flag(
		"collector.cgroup.active-subsystem",
		"Active cgroup subsystem for cgroups v1.",
	).Default("cpuacct").String()

	// Hidden opts for e2e and unit tests.
	forceCgroupsVersion = CEEMSExporterApp.Flag(
		"collector.cgroups.force-version",
		"Set cgroups version manually. Used only for testing.",
	).Hidden().Enum("v1", "v2")
)

// cgroupManager is the container that have cgroup information of resource manager.
type cgroupManager struct {
	mode             cgroups.CGMode    // cgroups mode: unified, legacy, hybrid
	root             string            // cgroups root
	slice            string            // Slice under which cgroups are managed eg system.slice, machine.slice
	scope            string            // Scope under which cgroups are managed eg slurmstepd.scope, machine-qemu\x2d1\x2dvm1.scope
	activeController string            // Active controller for cgroups v1
	mountPoint       string            // Path under which resource manager creates cgroups
	manager          string            // cgroup manager
	idRegex          *regexp.Regexp    // Regular expression to capture cgroup ID set by resource manager
	pathFilter       func(string) bool // Function to filter cgroup paths. Function must return true if cgroup path must be ignored
	procFilter       func(string) bool // Function to filter processes in cgroup based on cmdline. Function must return true if process must be ignored
}

// String implements stringer interface of the struct.
func (c *cgroupManager) String() string {
	return fmt.Sprintf(
		"mode: %d root: %s slice: %s scope: %s mount: %s manager: %s",
		c.mode,
		c.root,
		c.slice,
		c.scope,
		c.mountPoint,
		c.manager,
	)
}

// setMountPoint sets mountPoint for thc cgroupManager struct.
func (c *cgroupManager) setMountPoint() {
	switch c.manager {
	case slurm:
		switch c.mode { //nolint:exhaustive
		case cgroups.Unified:
			// /sys/fs/cgroup/system.slice/slurmstepd.scope
			c.mountPoint = filepath.Join(c.root, c.slice, c.scope)
		default:
			// /sys/fs/cgroup/cpuacct/slurm
			c.mountPoint = filepath.Join(c.root, c.activeController, c.manager)

			// For cgroups v1 we need to shift root to /sys/fs/cgroup/cpuacct
			c.root = filepath.Join(c.root, c.activeController)
		}
	case libvirt:
		switch c.mode { //nolint:exhaustive
		case cgroups.Unified:
			// /sys/fs/cgroup/machine.slice
			c.mountPoint = filepath.Join(c.root, c.slice)
		default:
			// /sys/fs/cgroup/cpuacct/machine.slice
			c.mountPoint = filepath.Join(c.root, c.activeController, c.slice)

			// For cgroups v1 we need to shift root to /sys/fs/cgroup/cpuacct
			c.root = filepath.Join(c.root, c.activeController)
		}
	default:
		c.mountPoint = c.root
	}
}

// NewCgroupManager returns an instance of cgroupManager based on resource manager.
func NewCgroupManager(name string) (*cgroupManager, error) {
	var manager *cgroupManager

	switch name {
	case slurm:
		if (*forceCgroupsVersion == "" && cgroups.Mode() == cgroups.Unified) || *forceCgroupsVersion == "v2" {
			manager = &cgroupManager{
				mode:  cgroups.Unified,
				root:  *cgroupfsPath,
				slice: "system.slice",
				scope: "slurmstepd.scope",
			}
		} else {
			var mode cgroups.CGMode
			if *forceCgroupsVersion == "v1" {
				mode = cgroups.Legacy
			} else {
				mode = cgroups.Mode()
			}

			manager = &cgroupManager{
				mode:             mode,
				root:             *cgroupfsPath,
				activeController: *activeController,
				slice:            slurm,
			}
		}

		// Add manager field
		manager.manager = slurm

		// Add path regex
		manager.idRegex = slurmCgroupPathRegex

		// Add filter functions
		manager.pathFilter = func(p string) bool {
			return strings.Contains(p, "/step_")
		}
		manager.procFilter = func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		}

		// Set mountpoint
		manager.setMountPoint()

		return manager, nil

	case libvirt:
		if (*forceCgroupsVersion == "" && cgroups.Mode() == cgroups.Unified) || *forceCgroupsVersion == "v2" {
			manager = &cgroupManager{
				mode:  cgroups.Unified,
				root:  *cgroupfsPath,
				slice: "machine.slice",
			}
		} else {
			var mode cgroups.CGMode
			if *forceCgroupsVersion == "v1" {
				mode = cgroups.Legacy
			} else {
				mode = cgroups.Mode()
			}

			manager = &cgroupManager{
				mode:             mode,
				root:             *cgroupfsPath,
				activeController: *activeController,
				slice:            "machine.slice",
			}
		}

		// Add manager field
		manager.manager = libvirt

		// Add path regex
		manager.idRegex = libvirtCgroupPathRegex

		// Add filter functions
		manager.pathFilter = func(p string) bool {
			return strings.Contains(p, "/libvirt")
		}
		manager.procFilter = func(p string) bool {
			return false
		}

		// Set mountpoint
		manager.setMountPoint()

		return manager, nil

	default:
		return nil, errors.New("unknown resource manager")
	}
}

// cgMetric contains metrics returned by cgroup.
type cgMetric struct {
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
	uuid            string
	err             bool
}

// cgroupCollector collects cgroup metrics for different resource managers.
type cgroupCollector struct {
	logger            log.Logger
	cgroupManager     *cgroupManager
	opts              cgroupOpts
	hostname          string
	hostMemTotal      float64
	numCgs            *prometheus.Desc
	cgCPUUser         *prometheus.Desc
	cgCPUSystem       *prometheus.Desc
	cgCPUs            *prometheus.Desc
	cgCPUPressure     *prometheus.Desc
	cgMemoryRSS       *prometheus.Desc
	cgMemoryCache     *prometheus.Desc
	cgMemoryUsed      *prometheus.Desc
	cgMemoryTotal     *prometheus.Desc
	cgMemoryFailCount *prometheus.Desc
	cgMemswUsed       *prometheus.Desc
	cgMemswTotal      *prometheus.Desc
	cgMemswFailCount  *prometheus.Desc
	cgMemoryPressure  *prometheus.Desc
	cgRDMAHCAHandles  *prometheus.Desc
	cgRDMAHCAObjects  *prometheus.Desc
	collectError      *prometheus.Desc
}

type cgroupOpts struct {
	collectSwapMemStats bool
	collectPSIStats     bool
}

// NewCgroupCollector returns a new cgroupCollector exposing a summary of cgroups.
func NewCgroupCollector(logger log.Logger, cgManager *cgroupManager, opts cgroupOpts) (*cgroupCollector, error) {
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

	return &cgroupCollector{
		logger:        logger,
		cgroupManager: cgManager,
		opts:          opts,
		hostMemTotal:  memTotal,
		hostname:      hostname,
		numCgs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "units"),
			"Total number of jobs",
			[]string{"manager", "hostname"},
			nil,
		),
		cgCPUUser: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_user_seconds_total"),
			"Total job CPU user seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgCPUSystem: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_system_seconds_total"),
			"Total job CPU system seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgCPUs: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpus"),
			"Total number of job CPUs",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgCPUPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_cpu_psi_seconds"),
			"Total CPU PSI in seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemoryRSS: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_rss_bytes"),
			"Memory RSS used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemoryCache: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_cache_bytes"),
			"Memory cache used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemoryUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_used_bytes"),
			"Memory used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemoryTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_total_bytes"),
			"Memory total in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemoryFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_fail_count"),
			"Memory fail count",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemswUsed: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_used_bytes"),
			"Swap used in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemswTotal: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_total_bytes"),
			"Swap total in bytes",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemswFailCount: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memsw_fail_count"),
			"Swap fail count",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgMemoryPressure: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_memory_psi_seconds"),
			"Total memory PSI in seconds",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		cgRDMAHCAHandles: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_handles"),
			"Current number of RDMA HCA handles",
			[]string{"manager", "hostname", "uuid", "device"},
			nil,
		),
		cgRDMAHCAObjects: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "unit_rdma_hca_objects"),
			"Current number of RDMA HCA objects",
			[]string{"manager", "hostname", "uuid", "device"},
			nil,
		),
		collectError: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, genericSubsystem, "collect_error"),
			"Indicates collection error, 0=no error, 1=error",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
	}, nil
}

// Update updates cgroup metrics on given channel.
func (c *cgroupCollector) Update(ch chan<- prometheus.Metric, metrics []cgMetric) error {
	// Fetch metrics
	metrics = c.doUpdate(metrics)

	// First send num jobs on the current host
	ch <- prometheus.MustNewConstMetric(c.numCgs, prometheus.GaugeValue, float64(len(metrics)), c.cgroupManager.manager, c.hostname)

	// Send metrics of each cgroup
	for _, m := range metrics {
		if m.err {
			ch <- prometheus.MustNewConstMetric(c.collectError, prometheus.GaugeValue, float64(1), c.cgroupManager.manager, c.hostname, m.uuid)
		}

		// CPU stats
		ch <- prometheus.MustNewConstMetric(c.cgCPUUser, prometheus.CounterValue, m.cpuUser, c.cgroupManager.manager, c.hostname, m.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgCPUSystem, prometheus.CounterValue, m.cpuSystem, c.cgroupManager.manager, c.hostname, m.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgCPUs, prometheus.GaugeValue, float64(m.cpus), c.cgroupManager.manager, c.hostname, m.uuid)

		// Memory stats
		ch <- prometheus.MustNewConstMetric(c.cgMemoryRSS, prometheus.GaugeValue, m.memoryRSS, c.cgroupManager.manager, c.hostname, m.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryCache, prometheus.GaugeValue, m.memoryCache, c.cgroupManager.manager, c.hostname, m.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryUsed, prometheus.GaugeValue, m.memoryUsed, c.cgroupManager.manager, c.hostname, m.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryTotal, prometheus.GaugeValue, m.memoryTotal, c.cgroupManager.manager, c.hostname, m.uuid)
		ch <- prometheus.MustNewConstMetric(c.cgMemoryFailCount, prometheus.GaugeValue, m.memoryFailCount, c.cgroupManager.manager, c.hostname, m.uuid)

		// Memory swap stats
		if c.opts.collectSwapMemStats {
			ch <- prometheus.MustNewConstMetric(c.cgMemswUsed, prometheus.GaugeValue, m.memswUsed, c.cgroupManager.manager, c.hostname, m.uuid)
			ch <- prometheus.MustNewConstMetric(c.cgMemswTotal, prometheus.GaugeValue, m.memswTotal, c.cgroupManager.manager, c.hostname, m.uuid)
			ch <- prometheus.MustNewConstMetric(c.cgMemswFailCount, prometheus.GaugeValue, m.memswFailCount, c.cgroupManager.manager, c.hostname, m.uuid)
		}

		// PSI stats
		if c.opts.collectPSIStats {
			ch <- prometheus.MustNewConstMetric(c.cgCPUPressure, prometheus.GaugeValue, m.cpuPressure, c.cgroupManager.manager, c.hostname, m.uuid)
			ch <- prometheus.MustNewConstMetric(c.cgMemoryPressure, prometheus.GaugeValue, m.memoryPressure, c.cgroupManager.manager, c.hostname, m.uuid)
		}

		// RDMA stats
		for device, handles := range m.rdmaHCAHandles {
			if handles > 0 {
				ch <- prometheus.MustNewConstMetric(c.cgRDMAHCAHandles, prometheus.GaugeValue, handles, c.cgroupManager.manager, c.hostname, m.uuid, device)
			}
		}

		for device, objects := range m.rdmaHCAHandles {
			if objects > 0 {
				ch <- prometheus.MustNewConstMetric(c.cgRDMAHCAObjects, prometheus.GaugeValue, objects, c.cgroupManager.manager, c.hostname, m.uuid, device)
			}
		}
	}

	return nil
}

// Stop releases any system resources held by collector.
func (c *cgroupCollector) Stop(_ context.Context) error {
	return nil
}

// doUpdate gets metrics of current active cgroups.
func (c *cgroupCollector) doUpdate(metrics []cgMetric) []cgMetric {
	// Start wait group for go routines
	wg := &sync.WaitGroup{}
	wg.Add(len(metrics))

	// No need for any lock primitives here as we read/write
	// a different element of slice in each go routine
	for i := range len(metrics) {
		go func(idx int) {
			defer wg.Done()

			c.update(&metrics[idx])
		}(i)
	}

	// Wait for all go routines
	wg.Wait()

	return metrics
}

// update get metrics of a given cgroup path.
func (c *cgroupCollector) update(m *cgMetric) {
	if c.cgroupManager.mode == cgroups.Unified {
		c.statsV2(m)
	} else {
		c.statsV1(m)
	}
}

// parseCPUSet parses cpuset.cpus file to return a list of CPUs in the cgroup.
func (c *cgroupCollector) parseCPUSet(cpuset string) ([]string, error) {
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
func (c *cgroupCollector) getCPUs(path string) ([]string, error) {
	var cpusPath string
	if c.cgroupManager.mode == cgroups.Unified {
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

// statsV1 fetches metrics from cgroups v1.
func (c *cgroupCollector) statsV1(metric *cgMetric) {
	path := metric.path

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

// statsV2 fetches metrics from cgroups v2.
func (c *cgroupCollector) statsV2(metric *cgMetric) {
	path := metric.path

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
		cgroup1.NewPids(*cgroupfsPath),
		cgroup1.NewBlkio(*cgroupfsPath),
		cgroup1.NewCpuset(*cgroupfsPath),
	}

	return s, nil
}

// cgroupController is a container for cgroup controllers in v1.
type cgroupController struct {
	id     uint64 // Hierarchy unique ID
	idx    uint64 // Cgroup SubSys index
	name   string // Controller name
	active bool   // Will be set to true if controller is set and active
}

// parseCgroupSubSysIds returns cgroup controllers for cgroups v1.
func parseCgroupSubSysIds() ([]cgroupController, error) {
	var cgroupControllers []cgroupController

	// Read /proc/cgroups file
	file, err := os.Open(procFilePath("cgroups"))
	if err != nil {
		return nil, err
	}

	defer file.Close()

	fscanner := bufio.NewScanner(file)

	var idx uint64 = 0

	fscanner.Scan() // ignore first entry

	for fscanner.Scan() {
		line := fscanner.Text()
		fields := strings.Fields(line)

		/* We care only for the controllers that we want */
		if idx >= cgroupSubSysCount {
			/* Maybe some cgroups are not upstream? */
			return cgroupControllers, fmt.Errorf(
				"cgroup default subsystem '%s' is indexed at idx=%d higher than CGROUP_SUBSYS_COUNT=%d",
				fields[0],
				idx,
				cgroupSubSysCount,
			)
		}

		if id, err := strconv.ParseUint(fields[1], 10, 32); err == nil {
			cgroupControllers = append(cgroupControllers, cgroupController{
				id:     id,
				idx:    idx,
				name:   fields[0],
				active: true,
			})
		}

		idx++
	}

	return cgroupControllers, nil
}
