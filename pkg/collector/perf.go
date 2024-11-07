//go:build !noperf
// +build !noperf

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"

	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/mahendrapaipuri/perf-utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const perfCollectorSubsystem = "perf"

// CLI opts.
var (
	perfHwProfilersFlag = CEEMSExporterApp.Flag(
		"collector.perf.hardware-events",
		"Enables collection of perf hardware events (default: disabled)",
	).Default("false").Bool()
	perfHwProfilers = CEEMSExporterApp.Flag(
		"collector.perf.hardware-profilers",
		"perf hardware profilers to collect",
	).Strings()
	perfSwProfilersFlag = CEEMSExporterApp.Flag(
		"collector.perf.software-events",
		"Enables collection of perf software events (default: disabled)",
	).Default("false").Bool()
	perfSwProfilers = CEEMSExporterApp.Flag(
		"collector.perf.software-profilers",
		"perf software profilers to collect",
	).Strings()
	perfCacheProfilersFlag = CEEMSExporterApp.Flag(
		"collector.perf.hardware-cache-events",
		"Enables collection of perf harware cache events (default: disabled)",
	).Default("false").Bool()
	perfCacheProfilers = CEEMSExporterApp.Flag(
		"collector.perf.cache-profilers",
		"perf cache profilers to collect",
	).Strings()
	perfProfilersEnvVars = CEEMSExporterApp.Flag(
		"collector.perf.env-var",
		"Enable profiling only on the processes having any of these environment variables set. If empty, all processes will be profiled.",
	).Strings()
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

// Security context names.
const (
	perfProcFilterCtx     = "perf_proc_filter"
	perfOpenProfilersCtx  = "perf_open_profilers"
	perfCloseProfilersCtx = "perf_close_profilers"
)

// perfProcFilterSecurityCtxData contains the input/output data for
// filterProc function to execute inside security context.
type perfProcFilterSecurityCtxData struct {
	targetEnvVars []string
	cgroups       []cgroup
	ignoreProc    func(string) bool
}

// perfProfilerSecurityCtxData contains the input/output data for
// opening/closing profilers inside security context.
type perfProfilerSecurityCtxData struct {
	logger                    *slog.Logger
	cgroups                   []cgroup
	activePIDs                []int
	perfHwProfilers           map[int]*perf.HardwareProfiler
	perfSwProfilers           map[int]*perf.SoftwareProfiler
	perfCacheProfilers        map[int]*perf.CacheProfiler
	perfHwProfilerTypes       perf.HardwareProfilerType
	perfSwProfilerTypes       perf.SoftwareProfilerType
	perfCacheProfilerTypes    perf.CacheProfilerType
	perfHwProfilersEnabled    bool
	perfSwProfilersEnabled    bool
	perfCacheProfilersEnabled bool
}

type perfOpts struct {
	perfHwProfilersEnabled    bool
	perfSwProfilersEnabled    bool
	perfCacheProfilersEnabled bool
	perfHwProfilers           []string
	perfSwProfilers           []string
	perfCacheProfilers        []string
	targetEnvVars             []string
}

// perfCollector is a Collector that uses the perf subsystem to collect
// metrics. It uses perf_event_open an ioctls for profiling. Due to the fact
// that the perf subsystem is highly dependent on kernel configuration and
// settings not all profiler values may be exposed on the target system at any
// given time.
type perfCollector struct {
	logger                  *slog.Logger
	hostname                string
	cgroupManager           *cgroupManager
	fs                      procfs.FS
	opts                    perfOpts
	securityContexts        map[string]*security.SecurityContext
	perfHwProfilers         map[int]*perf.HardwareProfiler
	perfSwProfilers         map[int]*perf.SoftwareProfiler
	perfCacheProfilers      map[int]*perf.CacheProfiler
	perfHwProfilerTypes     perf.HardwareProfilerType
	perfSwProfilerTypes     perf.SoftwareProfilerType
	perfCacheProfilerTypes  perf.CacheProfilerType
	desc                    map[string]*prometheus.Desc
	lastRawHwCounters       map[int]map[string]perf.ProfileValue
	lastRawCacheCounters    map[int]map[string]perf.ProfileValue
	lastCgroupHwCounters    map[string]map[string]float64
	lastCgroupCacheCounters map[string]map[string]float64
}

// NewPerfCollector returns a new perf based collector, it creates a profiler
// per compute unit.
func NewPerfCollector(logger *slog.Logger, cgManager *cgroupManager) (*perfCollector, error) {
	// Make perfOpts
	opts := perfOpts{
		perfHwProfilersEnabled:    *perfHwProfilersFlag,
		perfSwProfilersEnabled:    *perfSwProfilersFlag,
		perfCacheProfilersEnabled: *perfCacheProfilersFlag,
		perfHwProfilers:           *perfHwProfilers,
		perfSwProfilers:           *perfSwProfilers,
		perfCacheProfilers:        *perfCacheProfilers,
		targetEnvVars:             *perfProfilersEnvVars,
	}

	// Instantiate a new Proc FS
	fs, err := procfs.NewFS(*procfsPath)
	if err != nil {
		logger.Error("Unable to open procfs", "path", *procfsPath, "err", err)

		return nil, err
	}

	// Check if perf_event_open is supported on current kernel.
	// Checking for the existence of a /proc/sys/kernel/perf_event_paranoid
	// file, which is the canonical method for determining if a
	// kernel supports it or not.
	//
	// Moreover, Debian and Ubuntu distributions patched the paranoid
	// parameter to either 3 or 4 (which does not exist in kernel).
	// This parameter signifies that unprivileged user CANNOT use
	// perf_event_open even with CAP_PERFMON capabilities. In this
	// only root or CAP_SYS_ADMIN can open perf_event_open. So, we need
	// to ensure that paranoid parameter is not more than 2.
	//
	// Even with paranoid set to -1, we still need CAP_PERFMON to be
	// able to open perf events for ANY process on the host.
	if paranoid, err := fs.SysctlInts("kernel.perf_event_paranoid"); err == nil {
		if len(paranoid) == 1 && paranoid[0] > 2 {
			return nil, fmt.Errorf(
				"perf_event_open syscall is not possible with perf_event_paranoid=%d. Set it to value 2",
				paranoid[0],
			)
		}
	} else {
		return nil, fmt.Errorf("error opening /proc/sys/kernel/perf_event_paranoid file: %w", err)
	}

	collector := &perfCollector{
		logger:                  logger,
		fs:                      fs,
		hostname:                hostname,
		cgroupManager:           cgManager,
		opts:                    opts,
		perfHwProfilers:         make(map[int]*perf.HardwareProfiler),
		perfSwProfilers:         make(map[int]*perf.SoftwareProfiler),
		perfCacheProfilers:      make(map[int]*perf.CacheProfiler),
		lastRawHwCounters:       make(map[int]map[string]perf.ProfileValue),
		lastRawCacheCounters:    make(map[int]map[string]perf.ProfileValue),
		lastCgroupHwCounters:    make(map[string]map[string]float64),
		lastCgroupCacheCounters: make(map[string]map[string]float64),
	}

	// Configure perf profilers
	collector.perfHwProfilerTypes = perf.AllHardwareProfilers
	if collector.opts.perfHwProfilersEnabled && len(collector.opts.perfHwProfilers) > 0 {
		for _, hf := range collector.opts.perfHwProfilers {
			if v, ok := perfHardwareProfilerMap[hf]; ok {
				collector.perfHwProfilerTypes |= v
			}
		}
	}

	collector.perfSwProfilerTypes = perf.AllSoftwareProfilers
	if collector.opts.perfSwProfilersEnabled && len(collector.opts.perfSwProfilers) > 0 {
		for _, sf := range collector.opts.perfSwProfilers {
			if v, ok := perfSoftwareProfilerMap[sf]; ok {
				collector.perfSwProfilerTypes |= v
			}
		}
	}

	collector.perfCacheProfilerTypes = perf.AllCacheProfilers
	if collector.opts.perfCacheProfilersEnabled && len(collector.opts.perfCacheProfilers) > 0 {
		for _, cf := range collector.opts.perfCacheProfilers {
			if v, ok := perfCacheProfilerMap[cf]; ok {
				collector.perfCacheProfilerTypes |= v
			}
		}
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

	// Setup necessary capabilities. cap_perfmon is necessary to open perf events.
	capabilities := []string{"cap_perfmon"}
	reqCaps := setupCollectorCaps(logger, perfCollectorSubsystem, capabilities)

	// Setup new security context(s)
	// Security context for openining profilers
	collector.securityContexts = make(map[string]*security.SecurityContext)

	collector.securityContexts[perfOpenProfilersCtx], err = security.NewSecurityContext(
		perfOpenProfilersCtx,
		reqCaps,
		openProfilers,
		logger,
	)
	if err != nil {
		logger.Error("Failed to create a security context for opening perf profiler(s)", "err", err)

		return nil, err
	}

	// Security context for closing profilers
	collector.securityContexts[perfCloseProfilersCtx], err = security.NewSecurityContext(
		perfCloseProfilersCtx,
		reqCaps,
		closeProfilers,
		logger,
	)
	if err != nil {
		logger.Error("Failed to create a security context for closing perf profiler(s)", "err", err)

		return nil, err
	}

	// If we need to inspect env vars of processes, we will need cap_sys_ptrace and
	// cap_dac_read_search caps
	if len(collector.opts.targetEnvVars) > 0 {
		capabilities = []string{"cap_sys_ptrace", "cap_dac_read_search"}
		auxCaps := setupCollectorCaps(logger, perfCollectorSubsystem, capabilities)

		collector.securityContexts[perfProcFilterCtx], err = security.NewSecurityContext(
			perfProcFilterCtx,
			auxCaps,
			filterPerfProcs,
			logger,
		)
		if err != nil {
			logger.Error("Failed to create a security context for perf process filter", "err", err)

			return nil, err
		}
	}

	return collector, nil
}

// Update implements the Collector interface and will collect metrics per compute unit.
// cgroupIDUUIDMap provides a map to cgroupID to compute unit UUID. If the map is empty, it means
// cgroup ID and compute unit UUID is identical.
func (c *perfCollector) Update(ch chan<- prometheus.Metric, cgroups []cgroup) error {
	var err error

	// Filter processes in cgroups based on target env vars
	if len(c.opts.targetEnvVars) > 0 {
		cgroups, err = c.filterProcs(cgroups)
		if err != nil {
			return fmt.Errorf("failed to discover processes: %w", err)
		}
	}

	// Ensure cgroups is non empty
	if len(cgroups) == 0 {
		return nil
	}

	// Get a list of active cgroup IDs
	activeCgroupIDs := make([]string, len(cgroups))
	for icgrp := range cgroups {
		activeCgroupIDs[icgrp] = cgroups[icgrp].uuid
	}

	// Start new profilers for new processes
	activePIDs := c.newProfilers(cgroups)

	// Remove all profilers that have already finished
	// Ignore all errors
	if err := c.closeProfilers(activePIDs); err != nil {
		c.logger.Error("failed to close profilers counters", "err", err)
	}

	// Evict older entries in state maps
	c.updateStateMaps(activePIDs, activeCgroupIDs)

	// Update metrics in go routines for each cgroup
	// NOTE: Update concurrently and manage state variables better?
	for _, cgroup := range cgroups {
		uuid := cgroup.uuid

		if err := c.updateHardwareCounters(uuid, cgroup.procs, ch); err != nil {
			c.logger.Error("failed to update hardware counters", "uuid", uuid, "err", err)
		}

		if err := c.updateSoftwareCounters(uuid, cgroup.procs, ch); err != nil {
			c.logger.Error("failed to update software counters", "uuid", uuid, "err", err)
		}

		if err := c.updateCacheCounters(uuid, cgroup.procs, ch); err != nil {
			c.logger.Error("failed to update cache counters", "uuid", uuid, "err", err)
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *perfCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "sub_collector", perfCollectorSubsystem)

	// Close all profilers
	if err := c.closeProfilers([]int{}); err != nil {
		c.logger.Error("failed to close profilers counters", "err", err)
	}

	return nil
}

// updateStateMaps evicts inactive entries in state maps.
func (c *perfCollector) updateStateMaps(activePIDs []int, activeCgroupIDs []string) {
	if c.opts.perfHwProfilersEnabled {
		// Evict entries that are not in activePIDs
		for pid := range c.lastRawHwCounters {
			if !slices.Contains(activePIDs, pid) {
				delete(c.lastRawHwCounters, pid)
			}
		}

		// Evict entries that are not in activeCgroupIDs
		for cgroupID := range c.lastCgroupHwCounters {
			if !slices.Contains(activeCgroupIDs, cgroupID) {
				delete(c.lastCgroupHwCounters, cgroupID)
			}
		}

		// Allocate new pids
		for _, pid := range activePIDs {
			if c.lastRawHwCounters[pid] == nil {
				c.lastRawHwCounters[pid] = make(map[string]perf.ProfileValue)
			}
		}

		// Allocate new cgroupIDs
		for _, cgroupID := range activeCgroupIDs {
			if c.lastCgroupHwCounters[cgroupID] == nil {
				c.lastCgroupHwCounters[cgroupID] = make(map[string]float64)
			}
		}
	}

	if c.opts.perfCacheProfilersEnabled {
		// Evict entries that are not in activePIDs
		for pid := range c.lastRawCacheCounters {
			if !slices.Contains(activePIDs, pid) {
				delete(c.lastRawCacheCounters, pid)
			}
		}

		// Evict entries that are not in activeCgroupIDs
		for cgroupID := range c.lastCgroupCacheCounters {
			if !slices.Contains(activeCgroupIDs, cgroupID) {
				delete(c.lastCgroupCacheCounters, cgroupID)
			}
		}

		// Allocate new pids
		for _, pid := range activePIDs {
			if c.lastRawCacheCounters[pid] == nil {
				c.lastRawCacheCounters[pid] = make(map[string]perf.ProfileValue)
			}
		}

		// Allocate new cgroupIDs
		for _, cgroupID := range activeCgroupIDs {
			if c.lastCgroupCacheCounters[cgroupID] == nil {
				c.lastCgroupCacheCounters[cgroupID] = make(map[string]float64)
			}
		}
	}
}

func (c *perfCollector) updateHwCounter(pid int, metricName string, profileValue perf.ProfileValue) float64 {
	scaledValue := scaleCounter(c.lastRawHwCounters[pid][metricName], profileValue)
	c.lastRawHwCounters[pid][metricName] = profileValue

	return scaledValue
}

// aggHardwareCounters aggregates process hardware counters of a given cgroup.
func (c *perfCollector) aggHardwareCounters(hwProfiles map[int]*perf.HardwareProfile, cgroupHwPerfCounters map[string]float64) map[string]float64 {
	for pid, hwProfile := range hwProfiles {
		if hwProfile.CPUCycles != nil {
			metricName := "cpucycles_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.CPUCycles)
		}

		if hwProfile.Instructions != nil {
			metricName := "instructions_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.Instructions)
		}

		if hwProfile.BranchInstr != nil {
			metricName := "branch_instructions_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.BranchInstr)
		}

		if hwProfile.BranchMisses != nil {
			metricName := "branch_misses_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.BranchMisses)
		}

		if hwProfile.CacheRefs != nil {
			metricName := "cache_refs_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.CacheRefs)
		}

		if hwProfile.CacheMisses != nil {
			metricName := "cache_misses_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.CacheMisses)
		}

		if hwProfile.RefCPUCycles != nil {
			metricName := "ref_cpucycles_total"
			cgroupHwPerfCounters[metricName] += c.updateHwCounter(pid, metricName, *hwProfile.RefCPUCycles)
		}
	}

	return cgroupHwPerfCounters
}

// updateHardwareCounters collects hardware counters for the given cgroup.
func (c *perfCollector) updateHardwareCounters(
	cgroupID string,
	procs []procfs.Proc,
	ch chan<- prometheus.Metric,
) error {
	if !c.opts.perfHwProfilersEnabled {
		return nil
	}

	hwProfiles := make(map[int]*perf.HardwareProfile, len(procs))

	var pid int

	var errs error

	for _, proc := range procs {
		pid = proc.PID

		if hwProfiler, ok := c.perfHwProfilers[pid]; ok {
			hwProfile := &perf.HardwareProfile{}
			if err := (*hwProfiler).Profile(hwProfile); err != nil {
				errs = errors.Join(errs, fmt.Errorf("%w: %d", err, pid))

				continue
			}

			hwProfiles[pid] = hwProfile
		}
	}

	// Aggregate perf counters
	c.lastCgroupHwCounters[cgroupID] = c.aggHardwareCounters(hwProfiles, c.lastCgroupHwCounters[cgroupID])

	for counter, value := range c.lastCgroupHwCounters[cgroupID] {
		if value > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.desc[counter],
				prometheus.CounterValue, value,
				c.cgroupManager.manager, c.hostname, cgroupID,
			)
		}
	}

	return errs
}

// aggSoftwareCounters aggregates process software counters of a given cgroup.
func (c *perfCollector) aggSoftwareCounters(swProfiles map[int]*perf.SoftwareProfile) map[string]float64 {
	cgroupSwPerfCounters := make(map[string]float64)

	for _, swProfile := range swProfiles {
		if swProfile.PageFaults != nil {
			metricName := "page_faults_total"
			profileValue := *swProfile.PageFaults
			cgroupSwPerfCounters[metricName] += float64(profileValue.Value)
		}

		if swProfile.ContextSwitches != nil {
			metricName := "context_switches_total"
			profileValue := *swProfile.ContextSwitches
			cgroupSwPerfCounters[metricName] += float64(profileValue.Value)
		}

		if swProfile.CPUMigrations != nil {
			metricName := "cpu_migrations_total"
			profileValue := *swProfile.CPUMigrations
			cgroupSwPerfCounters[metricName] += float64(profileValue.Value)
		}

		if swProfile.MinorPageFaults != nil {
			metricName := "minor_faults_total"
			profileValue := *swProfile.MinorPageFaults
			cgroupSwPerfCounters[metricName] += float64(profileValue.Value)
		}

		if swProfile.MajorPageFaults != nil {
			metricName := "major_faults_total"
			profileValue := *swProfile.MajorPageFaults
			cgroupSwPerfCounters[metricName] += float64(profileValue.Value)
		}
	}

	return cgroupSwPerfCounters
}

// updateSoftwareCounters collects software counters for the given cgroup.
func (c *perfCollector) updateSoftwareCounters(
	cgroupID string,
	procs []procfs.Proc,
	ch chan<- prometheus.Metric,
) error {
	if !c.opts.perfSwProfilersEnabled {
		return nil
	}

	swProfiles := make(map[int]*perf.SoftwareProfile, len(procs))

	activePIDs := make([]int, len(procs))

	var pid int

	var errs error

	for iproc, proc := range procs {
		pid = proc.PID

		activePIDs[iproc] = pid

		if swProfiler, ok := c.perfSwProfilers[pid]; ok {
			swProfile := &perf.SoftwareProfile{}
			if err := (*swProfiler).Profile(swProfile); err != nil {
				errs = errors.Join(errs, fmt.Errorf("%w: %d", err, pid))

				continue
			}

			swProfiles[pid] = swProfile
		}
	}

	// Aggregate perf counters
	cgroupSwPerfCounters := c.aggSoftwareCounters(swProfiles)

	for counter, value := range cgroupSwPerfCounters {
		if value > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.desc[counter],
				prometheus.CounterValue, value,
				c.cgroupManager.manager, c.hostname, cgroupID,
			)
		}
	}

	return errs
}

func (c *perfCollector) updateCacheCounter(pid int, metricName string, profileValue perf.ProfileValue) float64 {
	scaledValue := scaleCounter(c.lastRawCacheCounters[pid][metricName], profileValue)
	c.lastRawCacheCounters[pid][metricName] = profileValue

	return scaledValue
}

// aggCacheCounters aggregates process cache counters of a given cgroup.
func (c *perfCollector) aggCacheCounters(cacheProfiles map[int]*perf.CacheProfile, cgroupCachePerfCounters map[string]float64) map[string]float64 {
	for pid, cacheProfile := range cacheProfiles {
		if cacheProfile.L1DataReadHit != nil {
			metricName := "cache_l1d_read_hits_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.L1DataReadHit)
		}

		if cacheProfile.L1DataReadMiss != nil {
			metricName := "cache_l1d_read_misses_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.L1DataReadMiss)
		}

		if cacheProfile.L1DataWriteHit != nil {
			metricName := "cache_l1d_write_hits_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.L1DataWriteHit)
		}

		if cacheProfile.L1InstrReadMiss != nil {
			metricName := "cache_l1_instr_read_misses_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.L1InstrReadMiss)
		}

		if cacheProfile.InstrTLBReadHit != nil {
			metricName := "cache_tlb_instr_read_hits_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.InstrTLBReadHit)
		}

		if cacheProfile.InstrTLBReadMiss != nil {
			metricName := "cache_tlb_instr_read_misses_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.InstrTLBReadMiss)
		}

		if cacheProfile.LastLevelReadHit != nil {
			metricName := "cache_ll_read_hits_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.LastLevelReadHit)
		}

		if cacheProfile.LastLevelReadMiss != nil {
			metricName := "cache_ll_read_misses_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.LastLevelReadMiss)
		}

		if cacheProfile.LastLevelWriteHit != nil {
			metricName := "cache_ll_write_hits_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.LastLevelWriteHit)
		}

		if cacheProfile.LastLevelWriteMiss != nil {
			metricName := "cache_ll_write_misses_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.LastLevelWriteMiss)
		}

		if cacheProfile.BPUReadHit != nil {
			metricName := "cache_bpu_read_hits_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.BPUReadHit)
		}

		if cacheProfile.BPUReadMiss != nil {
			metricName := "cache_bpu_read_misses_total"
			cgroupCachePerfCounters[metricName] += c.updateCacheCounter(pid, metricName, *cacheProfile.BPUReadMiss)
		}
	}

	return cgroupCachePerfCounters
}

// updateCacheCounters collects cache counters for the given cgroup.
func (c *perfCollector) updateCacheCounters(cgroupID string, procs []procfs.Proc, ch chan<- prometheus.Metric) error {
	if !c.opts.perfCacheProfilersEnabled {
		return nil
	}

	cacheProfiles := make(map[int]*perf.CacheProfile, len(procs))

	var pid int

	var errs error

	for _, proc := range procs {
		pid = proc.PID

		if cacheProfiler, ok := c.perfCacheProfilers[pid]; ok {
			cacheProfile := &perf.CacheProfile{}
			if err := (*cacheProfiler).Profile(cacheProfile); err != nil {
				errs = errors.Join(errs, fmt.Errorf("%w: %d", err, pid))

				continue
			}

			cacheProfiles[pid] = cacheProfile
		}
	}

	// Aggregate perf counters
	c.lastCgroupCacheCounters[cgroupID] = c.aggCacheCounters(cacheProfiles, c.lastCgroupCacheCounters[cgroupID])

	for counter, value := range c.lastCgroupCacheCounters[cgroupID] {
		if value > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.desc[counter],
				prometheus.CounterValue, value,
				c.cgroupManager.manager, c.hostname, cgroupID,
			)
		}
	}

	return errs
}

// filterProcs filters the processes that need to be profiled by looking at the
// presence of targetEnvVars.
func (c *perfCollector) filterProcs(cgroups []cgroup) ([]cgroup, error) {
	// Setup data pointer
	dataPtr := &perfProcFilterSecurityCtxData{
		cgroups:       cgroups,
		targetEnvVars: c.opts.targetEnvVars,
		ignoreProc:    c.cgroupManager.ignoreProc,
	}

	// Use security context as reading procs env vars is a privileged action
	if securityCtx, ok := c.securityContexts[perfProcFilterCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			return nil, err
		}
	} else {
		return nil, security.ErrNoSecurityCtx
	}

	if len(dataPtr.cgroups) > 0 {
		c.logger.Debug("Discovered cgroups for profiling")
	} else {
		c.logger.Debug("No cgroups found for profiling")
	}

	return dataPtr.cgroups, nil
}

// newProfilers open new perf profilers if they are not already in profilers map.
func (c *perfCollector) newProfilers(cgroups []cgroup) []int {
	dataPtr := &perfProfilerSecurityCtxData{
		logger:                    c.logger,
		cgroups:                   cgroups,
		perfHwProfilers:           c.perfHwProfilers,
		perfSwProfilers:           c.perfSwProfilers,
		perfCacheProfilers:        c.perfCacheProfilers,
		perfHwProfilerTypes:       c.perfHwProfilerTypes,
		perfSwProfilerTypes:       c.perfSwProfilerTypes,
		perfCacheProfilerTypes:    c.perfCacheProfilerTypes,
		perfHwProfilersEnabled:    c.opts.perfHwProfilersEnabled,
		perfSwProfilersEnabled:    c.opts.perfSwProfilersEnabled,
		perfCacheProfilersEnabled: c.opts.perfCacheProfilersEnabled,
	}

	// Start new profilers within security context
	if securityCtx, ok := c.securityContexts[perfOpenProfilersCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err == nil {
			return dataPtr.activePIDs
		}
	}

	return nil
}

// closeProfilers stops and closes profilers of PIDs that do not exist anymore.
func (c *perfCollector) closeProfilers(activePIDs []int) error {
	dataPtr := &perfProfilerSecurityCtxData{
		logger:                    c.logger,
		activePIDs:                activePIDs,
		perfHwProfilers:           c.perfHwProfilers,
		perfSwProfilers:           c.perfSwProfilers,
		perfCacheProfilers:        c.perfCacheProfilers,
		perfHwProfilerTypes:       c.perfHwProfilerTypes,
		perfSwProfilerTypes:       c.perfSwProfilerTypes,
		perfCacheProfilerTypes:    c.perfCacheProfilerTypes,
		perfHwProfilersEnabled:    c.opts.perfHwProfilersEnabled,
		perfSwProfilersEnabled:    c.opts.perfSwProfilersEnabled,
		perfCacheProfilersEnabled: c.opts.perfCacheProfilersEnabled,
	}

	// Start new profilers within security context
	if securityCtx, ok := c.securityContexts[perfCloseProfilersCtx]; ok {
		if err := securityCtx.Exec(dataPtr); err != nil {
			return err
		}
	}

	return nil
}

// openProfilers is a convenience function for newProfilers receiver. This function
// will be executed within a security context with necessary capabilities.
func openProfilers(data interface{}) error {
	// Assert data type
	var d *perfProfilerSecurityCtxData

	var ok bool
	if d, ok = data.(*perfProfilerSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	var activePIDs []int

	for _, cgroup := range d.cgroups {
		for _, proc := range cgroup.procs {
			pid := proc.PID

			activePIDs = append(activePIDs, pid)

			cmdLine, err := proc.CmdLine()
			if err != nil {
				cmdLine = []string{err.Error()}
			}

			if d.perfHwProfilersEnabled {
				if _, ok := d.perfHwProfilers[pid]; !ok {
					if hwProfiler, err := newHwProfiler(pid, d.perfHwProfilerTypes); err != nil {
						d.logger.Error("failed to start hardware profiler", "pid", pid, "cmd", strings.Join(cmdLine, " "), "err", err)
					} else {
						d.perfHwProfilers[pid] = hwProfiler
					}
				}
			}

			if d.perfSwProfilersEnabled {
				if _, ok := d.perfSwProfilers[pid]; !ok {
					if swProfiler, err := newSwProfiler(pid, d.perfSwProfilerTypes); err != nil {
						d.logger.Error("failed to start software profiler", "pid", pid, "cmd", strings.Join(cmdLine, " "), "err", err)
					} else {
						d.perfSwProfilers[pid] = swProfiler
					}
				}
			}

			if d.perfCacheProfilersEnabled {
				if _, ok := d.perfCacheProfilers[pid]; !ok {
					if cacheProfiler, err := newCacheProfiler(pid, d.perfCacheProfilerTypes); err != nil {
						d.logger.Error("failed to start cache profiler", "pid", pid, "cmd", strings.Join(cmdLine, " "), "err", err)
					} else {
						d.perfCacheProfilers[pid] = cacheProfiler
					}
				}
			}
		}
	}

	// Read activePIDs into d
	d.activePIDs = activePIDs

	return nil
}

// newHwProfiler opens a new hardware profiler for the given process PID.
func newHwProfiler(pid int, profilerTypes perf.HardwareProfilerType) (*perf.HardwareProfiler, error) {
	hwProf, err := perf.NewHardwareProfiler(
		pid,
		-1,
		profilerTypes,
	)
	if err != nil && !hwProf.HasProfilers() {
		return nil, err
	}

	if err := hwProf.Start(); err != nil {
		return nil, err
	}

	return &hwProf, nil
}

// newSwProfiler opens a new software profiler for the given process PID.
func newSwProfiler(pid int, profilerTypes perf.SoftwareProfilerType) (*perf.SoftwareProfiler, error) {
	swProf, err := perf.NewSoftwareProfiler(
		pid,
		-1,
		profilerTypes,
	)
	if err != nil && !swProf.HasProfilers() {
		return nil, err
	}

	if err := swProf.Start(); err != nil {
		return nil, err
	}

	return &swProf, nil
}

// newCacheProfiler opens a new cache profiler for the given process PID.
func newCacheProfiler(pid int, profilerTypes perf.CacheProfilerType) (*perf.CacheProfiler, error) {
	cacheProf, err := perf.NewCacheProfiler(
		pid,
		-1,
		profilerTypes,
	)
	if err != nil && !cacheProf.HasProfilers() {
		return nil, err
	}

	if err := cacheProf.Start(); err != nil {
		return nil, err
	}

	return &cacheProf, nil
}

// closeProfilers is a convenience function for closeProfilers receiver. This function
// will be executed within a security context with necessary capabilities.
func closeProfilers(data interface{}) error {
	// Assert data is of perfSecurityCtxData
	var d *perfProfilerSecurityCtxData

	var ok bool
	if d, ok = data.(*perfProfilerSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	if d.perfHwProfilersEnabled {
		for pid, hwProfiler := range d.perfHwProfilers {
			if !slices.Contains(d.activePIDs, pid) {
				if err := closeHwProfiler(hwProfiler); err != nil {
					d.logger.Error("failed to shutdown hardware profiler", "err", err)
				}

				// Remove profiler from the map
				delete(d.perfHwProfilers, pid)
			}
		}
	}

	if d.perfSwProfilersEnabled {
		for pid, swProfiler := range d.perfSwProfilers {
			if !slices.Contains(d.activePIDs, pid) {
				if err := closeSwProfiler(swProfiler); err != nil {
					d.logger.Error("failed to shutdown software profiler", "err", err)
				}

				// Remove profiler from the map
				delete(d.perfSwProfilers, pid)
			}
		}
	}

	if d.perfCacheProfilersEnabled {
		for pid, cacheProfiler := range d.perfCacheProfilers {
			if !slices.Contains(d.activePIDs, pid) {
				if err := closeCacheProfiler(cacheProfiler); err != nil {
					d.logger.Error("failed to shutdown cache profiler", "err", err)
				}

				// Remove profiler from the map
				delete(d.perfCacheProfilers, pid)
			}
		}
	}

	return nil
}

// closeHwProfiler stops and closes a hardware profiler.
func closeHwProfiler(profiler *perf.HardwareProfiler) error {
	if err := (*profiler).Stop(); err != nil {
		return err
	}

	if err := (*profiler).Close(); err != nil {
		return err
	}

	return nil
}

// closeSwProfiler stops and closes a software profiler.
func closeSwProfiler(profiler *perf.SoftwareProfiler) error {
	if err := (*profiler).Stop(); err != nil {
		return err
	}

	if err := (*profiler).Close(); err != nil {
		return err
	}

	return nil
}

// closeCacheProfiler stops and closes a cache profiler.
func closeCacheProfiler(profiler *perf.CacheProfiler) error {
	if err := (*profiler).Stop(); err != nil {
		return err
	}

	if err := (*profiler).Close(); err != nil {
		return err
	}

	return nil
}

// filterPerfProcs filters the processes of each cgroup inside data pointer based on
// presence of target env vars.
func filterPerfProcs(data interface{}) error {
	// Assert data is of perfSecurityCtxData
	var d *perfProcFilterSecurityCtxData

	var ok bool
	if d, ok = data.(*perfProcFilterSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	// Read filtered cgroups into d
	d.cgroups = cgroupProcFilterer(d.cgroups, d.targetEnvVars, d.ignoreProc)

	return nil
}

// scaleCounter uses the enabled and running times of counter to extrapolate counter value.
func scaleCounter(lastProfileValue, currentProfileValue perf.ProfileValue) float64 {
	deltaEnabled := currentProfileValue.TimeEnabled - lastProfileValue.TimeEnabled
	deltaRunning := currentProfileValue.TimeRunning - lastProfileValue.TimeRunning

	var deltaCounter uint64

	// Still no sure how to handle the overflows. If deltaCounter is < 0, treat it as
	// a overflow and assume the lastProfileValue.Value is the overflow limit and estimate
	// new deltaCounter
	if currentProfileValue.Value < lastProfileValue.Value {
		deltaCounter = lastProfileValue.Value + currentProfileValue.Value
	} else {
		deltaCounter = currentProfileValue.Value - lastProfileValue.Value
	}

	// Overflow here as well? Not sure if it happens and not sure how to handle it yet!
	if currentProfileValue.TimeRunning > lastProfileValue.TimeRunning {
		return math.Round((float64(deltaEnabled) / float64(deltaRunning)) * float64(deltaCounter))
	}

	return float64(deltaCounter)
}

// perfCollectorEnabled returns true if any of perf profilers are enabled.
func perfCollectorEnabled() bool {
	return *perfHwProfilersFlag || *perfSwProfilersFlag || *perfCacheProfilersFlag
}
