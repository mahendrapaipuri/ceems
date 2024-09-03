//go:build !perf
// +build !perf

package collector

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hodgesds/perf-utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const perfCollectorSubsystem = "perf"

// slurm related regexes.
var (
	slurmCgroupIDRegex    = regexp.MustCompile("^.*/(?:.+?)job_([0-9]+)(?:.*$)")
	slurmIgnoreProcsRegex = regexp.MustCompile("slurmstepd:(.*)|sleep ([0-9]+)|/bin/bash (.*)/slurm_script")
)

var (
	perfHardwareProfilerMap = map[string]perf.HardwareProfilerType{
		"CpuCycles":    perf.CpuCyclesProfiler,
		"CpuInstr":     perf.CpuInstrProfiler,
		"CacheRef":     perf.CacheRefProfiler,
		"CacheMisses":  perf.CacheMissesProfiler,
		"BranchInstr":  perf.BranchInstrProfiler,
		"BranchMisses": perf.BranchMissesProfiler,
		"RefCpuCycles": perf.RefCpuCyclesProfiler,
	}

	perfSoftwareProfilerMap = map[string]perf.SoftwareProfilerType{
		"PageFault":     perf.PageFaultProfiler,
		"ContextSwitch": perf.ContextSwitchProfiler,
		"CpuMigration":  perf.CpuMigrationProfiler,
		"MinorFault":    perf.MinorFaultProfiler,
		"MajorFault":    perf.MajorFaultProfiler,
	}

	perfCacheProfilerMap = map[string]perf.CacheProfilerType{
		"L1DataReadHit":    perf.L1DataReadHitProfiler,
		"L1DataReadMiss":   perf.L1DataReadMissProfiler,
		"L1DataWriteHit":   perf.L1DataWriteHitProfiler,
		"L1InstrReadMiss":  perf.L1InstrReadMissProfiler,
		"LLReadHit":        perf.LLReadHitProfiler,
		"LLReadMiss":       perf.LLReadMissProfiler,
		"LLWriteHit":       perf.LLWriteHitProfiler,
		"LLWriteMiss":      perf.LLWriteMissProfiler,
		"InstrTLBReadHit":  perf.InstrTLBReadHitProfiler,
		"InstrTLBReadMiss": perf.InstrTLBReadMissProfiler,
		"BPUReadHit":       perf.BPUReadHitProfiler,
		"BPUReadMiss":      perf.BPUReadMissProfiler,
	}
)

// perfCollector is a Collector that uses the perf subsystem to collect
// metrics. It uses perf_event_open an ioctls for profiling. Due to the fact
// that the perf subsystem is highly dependent on kernel configuration and
// settings not all profiler values may be exposed on the target system at any
// given time.
type perfCollector struct {
	fs procfs.FS

	envVar string

	perfHwProfilersEnabled    bool
	perfSwProfilersEnabled    bool
	perfCacheProfilersEnabled bool

	perfHwProfilers    sync.Map
	perfSwProfilers    sync.Map
	perfCacheProfilers sync.Map

	perfHwProfilerTypes    perf.HardwareProfilerType
	perfSwProfilerTypes    perf.SoftwareProfilerType
	perfCacheProfilerTypes perf.CacheProfilerType

	cgroupIDRegex      *regexp.Regexp // Regex to extract cgroup ID from process
	filterProcCmdRegex *regexp.Regexp // Processes with command line matching this regex will be ignored

	desc map[string]*prometheus.Desc

	hostname string
	manager  string

	logger log.Logger
}

func init() {
	RegisterCollector(perfCollectorSubsystem, defaultDisabled, NewPerfCollector)
}

// CLI options.
var (
	perfProfilersEnvVars = CEEMSExporterApp.Flag(
		"collector.perf.env-var",
		"Processes having this environment variable set will be profiled. If empty, all the relevant processes will be profiled.",
	).String()
	perfHwProfilersFlag = CEEMSExporterApp.Flag(
		"collector.perf.enable-hardware-profilers",
		"Enables perf hardware profilers (default: enabled)",
	).Default("true").Bool()
	perfHwProfilers = CEEMSExporterApp.Flag(
		"collector.perf.hardware-profilers",
		"perf hardware profilers to collect",
	).Strings()
	perfSwProfilersFlag = CEEMSExporterApp.Flag(
		"collector.perf.enable-software-profilers",
		"Enables perf software profilers (default: enabled)",
	).Default("true").Bool()
	perfSwProfilers = CEEMSExporterApp.Flag(
		"collector.perf.software-profilers",
		"perf software profilers to collect",
	).Strings()
	perfCacheProfilersFlag = CEEMSExporterApp.Flag(
		"collector.perf.enable-cache-profilers",
		"Enables perf cache profilers (default: disabled)",
	).Default("false").Bool()
	perfCacheProfilers = CEEMSExporterApp.Flag(
		"collector.perf.cache-profilers",
		"perf cache profilers to collect",
	).Strings()
)

// NewPerfCollector returns a new perf based collector, it creates a profiler
// per compute unit.
func NewPerfCollector(logger log.Logger) (Collector, error) {
	collector := &perfCollector{
		logger:                    logger,
		hostname:                  hostname,
		envVar:                    *perfProfilersEnvVars,
		perfHwProfilersEnabled:    *perfHwProfilersFlag,
		perfSwProfilersEnabled:    *perfSwProfilersFlag,
		perfCacheProfilersEnabled: *perfCacheProfilersFlag,
	}

	// Configure perf profilers
	collector.perfHwProfilerTypes = perf.AllHardwareProfilers
	if collector.perfHwProfilersEnabled && len(*perfHwProfilers) > 0 {
		for _, hf := range *perfHwProfilers {
			if v, ok := perfHardwareProfilerMap[hf]; ok {
				collector.perfHwProfilerTypes |= v
			}
		}
	}

	collector.perfSwProfilerTypes = perf.AllSoftwareProfilers
	if collector.perfSwProfilersEnabled && len(*perfSwProfilers) > 0 {
		for _, sf := range *perfSwProfilers {
			if v, ok := perfSoftwareProfilerMap[sf]; ok {
				collector.perfSwProfilerTypes |= v
			}
		}
	}

	collector.perfCacheProfilerTypes = perf.AllCacheProfilers
	if collector.perfCacheProfilersEnabled && len(*perfCacheProfilers) > 0 {
		for _, cf := range *perfCacheProfilers {
			if v, ok := perfCacheProfilerMap[cf]; ok {
				collector.perfCacheProfilerTypes |= v
			}
		}
	}

	if *collectorState[slurmCollectorSubsystem] {
		collector.manager = "slurm"
		collector.cgroupIDRegex = slurmCgroupIDRegex
		collector.filterProcCmdRegex = slurmIgnoreProcsRegex
	}

	var err error

	// Instantiate a new Proc FS
	collector.fs, err = procfs.NewFS(*procfsPath)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	collector.desc = map[string]*prometheus.Desc{
		"cpucycles_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cpucycles_total",
			),
			"Number of CPU cycles (frequency scaled)",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"instructions_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"instructions_total",
			),
			"Number of CPU instructions",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"branch_instructions_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"branch_instructions_total",
			),
			"Number of CPU branch instructions",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"branch_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"branch_misses_total",
			),
			"Number of CPU branch misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_refs_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_refs_total",
			),
			"Number of cache references (non frequency scaled)",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_misses_total",
			),
			"Number of cache misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"ref_cpucycles_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"ref_cpucycles_total",
			),
			"Number of CPU cycles",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"page_faults_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"page_faults_total",
			),
			"Number of page faults",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"context_switches_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"context_switches_total",
			),
			"Number of context switches",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cpu_migrations_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cpu_migrations_total",
			),
			"Number of CPU process migrations",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"minor_faults_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"minor_faults_total",
			),
			"Number of minor page faults",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"major_faults_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"major_faults_total",
			),
			"Number of major page faults",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_l1d_read_hits_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_l1d_read_hits_total",
			),
			"Number L1 data cache read hits",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_l1d_read_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_l1d_read_misses_total",
			),
			"Number L1 data cache read misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_l1d_write_hits_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_l1d_write_hits_total",
			),
			"Number L1 data cache write hits",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_l1_instr_read_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_l1_instr_read_misses_total",
			),
			"Number instruction L1 instruction read misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_tlb_instr_read_hits_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_tlb_instr_read_hits_total",
			),
			"Number instruction TLB read hits",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_tlb_instr_read_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_tlb_instr_read_misses_total",
			),
			"Number instruction TLB read misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_ll_read_hits_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_ll_read_hits_total",
			),
			"Number last level read hits",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_ll_read_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_ll_read_misses_total",
			),
			"Number last level read misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_ll_write_hits_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_ll_write_hits_total",
			),
			"Number last level write hits",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_ll_write_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_ll_write_misses_total",
			),
			"Number last level write misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_bpu_read_hits_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_bpu_read_hits_total",
			),
			"Number BPU read hits",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
		"cache_bpu_read_misses_total": prometheus.NewDesc(
			prometheus.BuildFQName(
				Namespace,
				perfCollectorSubsystem,
				"cache_bpu_read_misses_total",
			),
			"Number BPU read misses",
			[]string{"manager", "hostname", "uuid"},
			nil,
		),
	}

	return collector, nil
}

// Update implements the Collector interface and will collect metrics per compute unit.
func (c *perfCollector) Update(ch chan<- prometheus.Metric) error {
	// Discover new processes
	cgroupIDProcMap, err := c.discoverProcess()
	if err != nil {
		return err
	}

	// Start new profilers for new processes
	activePIDs := c.newProfilers(cgroupIDProcMap)

	// Remove all profilers that have already finished
	c.closeProfilers(activePIDs)

	for cgroupID, procs := range cgroupIDProcMap {
		if err := c.updateHardwareCounters(cgroupID, procs, ch); err != nil {
			level.Error(c.logger).Log("msg", "failed to update hardware counters", "cgroup", cgroupID, "err", err)
		}

		if err := c.updateSoftwareCounters(cgroupID, procs, ch); err != nil {
			level.Error(c.logger).Log("msg", "failed to update software counters", "cgroup", cgroupID, "err", err)
		}

		if err := c.updateCacheCounters(cgroupID, procs, ch); err != nil {
			level.Error(c.logger).Log("msg", "failed to update cache counters", "cgroup", cgroupID, "err", err)
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *perfCollector) Stop(_ context.Context) error {
	level.Debug(c.logger).Log("msg", "Stopping", "collector", perfCollectorSubsystem)

	// Close all profilers
	c.closeProfilers([]int{})

	return nil
}

// updateHardwareCounters collects hardware counters for the given cgroup.
func (c *perfCollector) updateHardwareCounters(cgroupID string, procs []procfs.Proc, ch chan<- prometheus.Metric) error {
	if !c.perfHwProfilersEnabled {
		return nil
	}

	cgroupHwPerfCounters := make(map[string]float64)

	var pid int

	var errs error

	for _, proc := range procs {
		pid = proc.PID

		if profiler, ok := c.perfHwProfilers.Load(pid); ok {
			hwProfile := &perf.HardwareProfile{}
			if hwProfiler, ok := profiler.(*perf.HardwareProfiler); ok {
				if err := (*hwProfiler).Profile(hwProfile); err != nil {
					errs = errors.Join(errs, fmt.Errorf("%w: %d", err, pid))

					continue
				}
			}

			if hwProfile.CPUCycles != nil {
				cgroupHwPerfCounters["cpucycles_total"] += float64(*hwProfile.CPUCycles)
			}

			if hwProfile.Instructions != nil {
				cgroupHwPerfCounters["instructions_total"] += float64(*hwProfile.Instructions)
			}

			if hwProfile.BranchInstr != nil {
				cgroupHwPerfCounters["branch_instructions_total"] += float64(*hwProfile.BranchInstr)
			}

			if hwProfile.BranchMisses != nil {
				cgroupHwPerfCounters["branch_misses_total"] += float64(*hwProfile.BranchMisses)
			}

			if hwProfile.CacheRefs != nil {
				cgroupHwPerfCounters["cache_refs_total"] += float64(*hwProfile.CacheRefs)
			}

			if hwProfile.CacheMisses != nil {
				cgroupHwPerfCounters["cache_misses_total"] += float64(*hwProfile.CacheMisses)
			}

			if hwProfile.RefCPUCycles != nil {
				cgroupHwPerfCounters["ref_cpucycles_total"] += float64(*hwProfile.RefCPUCycles)
			}
		}
	}

	for counter, value := range cgroupHwPerfCounters {
		if value > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.desc[counter],
				prometheus.CounterValue, value,
				c.manager, c.hostname, cgroupID,
			)
		}
	}

	return errs
}

// updateSoftwareCounters collects software counters for the given cgroup.
func (c *perfCollector) updateSoftwareCounters(cgroupID string, procs []procfs.Proc, ch chan<- prometheus.Metric) error {
	if !c.perfSwProfilersEnabled {
		return nil
	}

	cgroupSwPerfCounters := make(map[string]float64)

	var pid int

	var errs error

	for _, proc := range procs {
		pid = proc.PID

		if profiler, ok := c.perfSwProfilers.Load(pid); ok {
			swProfile := &perf.SoftwareProfile{}
			if swProfiler, ok := profiler.(*perf.SoftwareProfiler); ok {
				if err := (*swProfiler).Profile(swProfile); err != nil {
					errs = errors.Join(errs, fmt.Errorf("%w: %d", err, pid))

					continue
				}
			}

			if swProfile.PageFaults != nil {
				cgroupSwPerfCounters["page_faults_total"] += float64(*swProfile.PageFaults)
			}

			if swProfile.ContextSwitches != nil {
				cgroupSwPerfCounters["context_switches_total"] += float64(*swProfile.ContextSwitches)
			}

			if swProfile.CPUMigrations != nil {
				cgroupSwPerfCounters["cpu_migrations_total"] += float64(*swProfile.CPUMigrations)
			}

			if swProfile.MinorPageFaults != nil {
				cgroupSwPerfCounters["minor_faults_total"] += float64(*swProfile.MinorPageFaults)
			}

			if swProfile.MajorPageFaults != nil {
				cgroupSwPerfCounters["major_faults_total"] += float64(*swProfile.MajorPageFaults)
			}
		}
	}

	for counter, value := range cgroupSwPerfCounters {
		if value > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.desc[counter],
				prometheus.CounterValue, value,
				c.manager, c.hostname, cgroupID,
			)
		}
	}

	return errs
}

// updateCacheCounters collects cache counters for the given cgroup.
func (c *perfCollector) updateCacheCounters(cgroupID string, procs []procfs.Proc, ch chan<- prometheus.Metric) error {
	if !c.perfCacheProfilersEnabled {
		return nil
	}

	cgroupCachePerfCounters := make(map[string]float64)

	var pid int

	var errs error

	for _, proc := range procs {
		pid = proc.PID

		if profiler, ok := c.perfCacheProfilers.Load(pid); ok {
			cacheProfile := &perf.CacheProfile{}
			if cacheProfiler, ok := profiler.(*perf.CacheProfiler); ok {
				if err := (*cacheProfiler).Profile(cacheProfile); err != nil {
					errs = errors.Join(errs, fmt.Errorf("%w: %d", err, pid))

					continue
				}
			}

			if cacheProfile.L1DataReadHit != nil {
				cgroupCachePerfCounters["cache_l1d_read_hits_total"] += float64(*cacheProfile.L1DataReadHit)
			}

			if cacheProfile.L1DataReadMiss != nil {
				cgroupCachePerfCounters["cache_l1d_read_misses_total"] += float64(*cacheProfile.L1DataReadMiss)
			}

			if cacheProfile.L1DataWriteHit != nil {
				cgroupCachePerfCounters["cache_l1d_write_hits_total"] += float64(*cacheProfile.L1DataWriteHit)
			}

			if cacheProfile.L1InstrReadMiss != nil {
				cgroupCachePerfCounters["cache_l1_instr_read_misses_total"] += float64(*cacheProfile.L1InstrReadMiss)
			}

			if cacheProfile.InstrTLBReadHit != nil {
				cgroupCachePerfCounters["cache_tlb_instr_read_hits_total"] += float64(*cacheProfile.InstrTLBReadHit)
			}

			if cacheProfile.InstrTLBReadMiss != nil {
				cgroupCachePerfCounters["cache_tlb_instr_read_misses_total"] += float64(*cacheProfile.InstrTLBReadMiss)
			}

			if cacheProfile.LastLevelReadHit != nil {
				cgroupCachePerfCounters["cache_ll_read_hits_total"] += float64(*cacheProfile.LastLevelReadHit)
			}

			if cacheProfile.LastLevelReadMiss != nil {
				cgroupCachePerfCounters["cache_ll_read_misses_total"] += float64(*cacheProfile.LastLevelReadMiss)
			}

			if cacheProfile.LastLevelWriteHit != nil {
				cgroupCachePerfCounters["cache_ll_write_hits_total"] += float64(*cacheProfile.LastLevelWriteHit)
			}

			if cacheProfile.LastLevelWriteMiss != nil {
				cgroupCachePerfCounters["cache_ll_write_misses_total"] += float64(*cacheProfile.LastLevelWriteMiss)
			}

			if cacheProfile.BPUReadHit != nil {
				cgroupCachePerfCounters["cache_bpu_read_hits_total"] += float64(*cacheProfile.BPUReadHit)
			}

			if cacheProfile.BPUReadMiss != nil {
				cgroupCachePerfCounters["cache_bpu_read_misses_total"] += float64(*cacheProfile.BPUReadMiss)
			}
		}
	}

	for counter, value := range cgroupCachePerfCounters {
		if value > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.desc[counter],
				prometheus.CounterValue, value,
				c.manager, c.hostname, cgroupID,
			)
		}
	}

	return errs
}

// discoverProcess discovers all active processes by walking through procsfs and returns
// a map of cgroup ID to procs.
func (c *perfCollector) discoverProcess() (map[string][]procfs.Proc, error) {
	allProcs, err := c.fs.AllProcs()
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to read /proc", "err", err)

		return nil, err
	}

	cgroupIDProcMap := make(map[string][]procfs.Proc)

	var cgroupID string

	for _, proc := range allProcs {
		// if envVar is not empty check if this env vars is present for the process
		// We dont check for the value of env var. Presence of env var is enough to
		// trigger the profiling of that process
		if c.envVar != "" {
			environ, err := proc.Environ()
			if err != nil {
				continue
			}

			for _, env := range environ {
				if strings.HasPrefix(env, c.envVar) {
					goto check_process
				}
			}

			// If env var is not found on process, ignore it
			continue
		}

	check_process:

		// Ignore processes where command line matches the regex
		if c.filterProcCmdRegex != nil {
			procCmdLine, err := proc.CmdLine()
			if err != nil || len(procCmdLine) == 0 {
				continue
			}

			// Ignore process if matches found
			procCmdLineMatches := c.filterProcCmdRegex.FindStringSubmatch(strings.Join(procCmdLine, " "))
			if len(procCmdLineMatches) > 1 {
				continue
			}
		}

		// Get cgroup ID from regex
		if c.cgroupIDRegex != nil {
			cgroups, err := proc.Cgroups()
			if err != nil || len(cgroups) == 0 {
				continue
			}

			for _, cgroup := range cgroups {
				cgroupIDMatches := c.cgroupIDRegex.FindStringSubmatch(cgroup.Path)
				if len(cgroupIDMatches) <= 1 {
					continue
				}

				cgroupID = cgroupIDMatches[1]

				break
			}
		}

		// If no cgroupID found, ignore
		if cgroupID == "" {
			continue
		}

		cgroupIDProcMap[cgroupID] = append(cgroupIDProcMap[cgroupID], proc)
	}

	return cgroupIDProcMap, nil
}

// newProfilers start new perf profilers if they are not already in profilers map.
func (c *perfCollector) newProfilers(cgroupIDProcMap map[string][]procfs.Proc) []int {
	var activePIDs []int

	for _, procs := range cgroupIDProcMap {
		for _, proc := range procs {
			pid := proc.PID

			activePIDs = append(activePIDs, pid)

			cmdLine, err := proc.CmdLine()
			if err != nil {
				cmdLine = []string{err.Error()}
			}

			if c.perfHwProfilersEnabled {
				if _, ok := c.perfHwProfilers.Load(pid); !ok {
					if profiler, err := c.newHwProfiler(pid); err != nil {
						level.Error(c.logger).Log("msg", "failed to start hardware profiler", "pid", pid, "cmd", strings.Join(cmdLine, " "), "err", err)
					} else {
						c.perfHwProfilers.Store(pid, profiler)
					}
				}
			}

			if c.perfSwProfilersEnabled {
				if _, ok := c.perfSwProfilers.Load(pid); !ok {
					if profiler, err := c.newSwProfiler(pid); err != nil {
						level.Error(c.logger).Log("msg", "failed to start software profiler", "pid", pid, "cmd", strings.Join(cmdLine, " "), "err", err)
					} else {
						c.perfSwProfilers.Store(pid, profiler)
					}
				}
			}

			if c.perfCacheProfilersEnabled {
				if _, ok := c.perfCacheProfilers.Load(pid); !ok {
					if profiler, err := c.newCacheProfiler(pid); err != nil {
						level.Error(c.logger).Log("msg", "failed to start cache profiler", "pid", pid, "cmd", strings.Join(cmdLine, " "), "err", err)
					} else {
						c.perfCacheProfilers.Store(pid, profiler)
					}
				}
			}
		}
	}

	return activePIDs
}

// newHwProfiler creates and starts a new hardware profiler for the given process PID.
func (c *perfCollector) newHwProfiler(pid int) (*perf.HardwareProfiler, error) {
	hwProf, err := perf.NewHardwareProfiler(
		pid,
		-1,
		c.perfHwProfilerTypes,
	)
	if err != nil && !hwProf.HasProfilers() {
		return nil, err
	}

	if err := hwProf.Start(); err != nil {
		return nil, err
	}

	return &hwProf, nil
}

// newSwProfiler creates and starts a new software profiler for the given process PID.
func (c *perfCollector) newSwProfiler(pid int) (*perf.SoftwareProfiler, error) {
	swProf, err := perf.NewSoftwareProfiler(
		pid,
		-1,
		c.perfSwProfilerTypes,
	)
	if err != nil && !swProf.HasProfilers() {
		return nil, err
	}

	if err := swProf.Start(); err != nil {
		return nil, err
	}

	return &swProf, nil
}

// newCacheProfiler creates and starts a new cache profiler for the given process PID.
func (c *perfCollector) newCacheProfiler(pid int) (*perf.CacheProfiler, error) {
	cacheProf, err := perf.NewCacheProfiler(
		pid,
		-1,
		c.perfCacheProfilerTypes,
	)
	if err != nil && !cacheProf.HasProfilers() {
		return nil, err
	}

	if err := cacheProf.Start(); err != nil {
		return nil, err
	}

	return &cacheProf, nil
}

// closeProfilers stops and closes profilers of PIDs that do not exist anymore.
func (c *perfCollector) closeProfilers(activePIDs []int) {
	if c.perfHwProfilersEnabled {
		c.perfHwProfilers.Range(func(pid, profiler interface{}) bool {
			if pidInt, ok := pid.(int); ok {
				if !slices.Contains(activePIDs, pidInt) {
					if hwProfiler, ok := profiler.(*perf.HardwareProfiler); ok {
						if err := c.closeHwProfiler(hwProfiler); err != nil {
							level.Error(c.logger).Log("msg", "failed to shutdown hardware profiler", "err", err)
						} else {
							c.perfHwProfilers.Delete(pidInt)
							level.Debug(c.logger).Log("msg", "Removed process from hardware profilers map", "pid", pid)
						}
					}
				}
			}

			return true
		})
	}

	if c.perfSwProfilersEnabled {
		c.perfSwProfilers.Range(func(pid, profiler interface{}) bool {
			if pidInt, ok := pid.(int); ok {
				if !slices.Contains(activePIDs, pidInt) {
					if swProfiler, ok := profiler.(*perf.SoftwareProfiler); ok {
						if err := c.closeSwProfiler(swProfiler); err != nil {
							level.Error(c.logger).Log("msg", "failed to shutdown software profiler", "err", err)
						} else {
							c.perfSwProfilers.Delete(pidInt)
							level.Debug(c.logger).Log("msg", "Removed process from software profilers map", "pid", pid)
						}
					}
				}
			}

			return true
		})
	}

	if c.perfCacheProfilersEnabled {
		c.perfCacheProfilers.Range(func(pid, profiler interface{}) bool {
			if pidInt, ok := pid.(int); ok {
				if !slices.Contains(activePIDs, pidInt) {
					if cacheProfiler, ok := profiler.(*perf.CacheProfiler); ok {
						if err := c.closeCacheProfiler(cacheProfiler); err != nil {
							level.Error(c.logger).Log("msg", "failed to shutdown cache profiler", "err", err)
						} else {
							c.perfCacheProfilers.Delete(pidInt)
							level.Debug(c.logger).Log("msg", "Removed process from cache profilers map", "pid", pid)
						}
					}
				}
			}

			return true
		})
	}
}

// closeHwProfiler stops and closes a hardware profiler.
func (c *perfCollector) closeHwProfiler(profiler *perf.HardwareProfiler) error {
	if err := (*profiler).Stop(); err != nil {
		return err
	}

	if err := (*profiler).Close(); err != nil {
		return err
	}

	return nil
}

// closeSwProfiler stops and closes a software profiler.
func (c *perfCollector) closeSwProfiler(profiler *perf.SoftwareProfiler) error {
	if err := (*profiler).Stop(); err != nil {
		return err
	}

	if err := (*profiler).Close(); err != nil {
		return err
	}

	return nil
}

// closeCacheProfiler stops and closes a cache profiler.
func (c *perfCollector) closeCacheProfiler(profiler *perf.CacheProfiler) error {
	if err := (*profiler).Stop(); err != nil {
		return err
	}

	if err := (*profiler).Close(); err != nil {
		return err
	}

	return nil
}
