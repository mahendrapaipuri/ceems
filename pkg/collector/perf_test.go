//go:build !noperf
// +build !noperf

package collector

import (
	"context"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/mahendrapaipuri/perf-utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--collector.perf.hardware-events",
		"--collector.perf.software-events",
		"--collector.perf.hardware-cache-events",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager := &cgroupManager{
		mode:        cgroups.Unified,
		mountPoints: []string{"testdata/sys/fs/cgroup/system.slice/slurmstepd.scope"},
		idRegex:     slurmCgroupV2PathRegex,
		ignoreProc: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	collector, err := NewPerfCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), cgManager)
	require.NoError(t, err)

	// Setup background goroutine to capture metrics.
	metrics := make(chan prometheus.Metric)
	defer close(metrics)

	go func() {
		i := 0
		for range metrics {
			i++
		}
	}()

	err = collector.Update(metrics, nil, "")
	require.NoError(t, err)

	err = collector.Stop(context.Background())
	require.NoError(t, err)
}

func TestDiscoverProcess(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--collector.cgroups.force-version", "v2",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// perf opts
	opts := perfOpts{
		perfHwProfilersEnabled:    true,
		perfSwProfilersEnabled:    true,
		perfCacheProfilersEnabled: true,
		targetEnvVars:             []string{"ENABLE_PROFILING"},
	}

	collector := perfCollector{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		cgroupManager:    cgManager,
		opts:             opts,
		securityContexts: make(map[string]*security.SecurityContext),
	}

	// Create dummy security context
	collector.securityContexts[perfProcFilterCtx], err = security.NewSecurityContext(
		perfProcFilterCtx,
		nil,
		filterPerfProcs,
		collector.logger,
	)
	require.NoError(t, err)

	collector.fs, err = procfs.NewFS("testdata/proc")
	require.NoError(t, err)

	// Discover cgroups
	cgroups, err := cgManager.discover()
	require.NoError(t, err)

	// Filter processes
	cgroups, err = collector.filterProcs(cgroups)
	require.NoError(t, err)

	// expected
	expectedCgroupIDs := []string{
		"1009248", "1009249",
		"2009248", "2009249",
		"3009248", "3009249",
	}
	expectedCgroupProcs := map[string][]int{
		"1009248": {46231, 46281},
		"1009249": {46235, 46236},
		"2009248": {56231, 56281},
		"2009249": {56235, 56236},
		"3009248": {66231, 66281},
		"3009249": {66235, 66236},
	}

	// Get cgroup IDs
	var cgroupIDs []string

	cgroupProcs := make(map[string][]int)

	for _, cgroup := range cgroups {
		if !slices.Contains(cgroupIDs, cgroup.id) {
			cgroupIDs = append(cgroupIDs, cgroup.id)
		}

		for _, proc := range cgroup.procs {
			cgroupProcs[cgroup.id] = append(cgroupProcs[cgroup.id], proc.PID)
		}
	}

	assert.ElementsMatch(t, cgroupIDs, expectedCgroupIDs)

	for _, cgroupID := range cgroupIDs {
		assert.ElementsMatch(t, cgroupProcs[cgroupID], expectedCgroupProcs[cgroupID], "cgroup %s", cgroupID)
	}
}

func TestNewProfilers(t *testing.T) {
	var err error

	var ok bool

	_, err = CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--collector.perf.hardware-events",
		"--collector.perf.software-events",
		"--collector.perf.hardware-cache-events",
		"--collector.cgroups.force-version", "v1",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	collector, err := NewPerfCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), cgManager)
	require.NoError(t, err)

	// Use fake cgroupID for current process
	cgroups := []cgroup{
		{
			id: "1234",
			procs: []procfs.Proc{
				{PID: os.Getpid()},
			},
		},
	}

	// Setup background goroutine to capture metrics.
	metrics := make(chan prometheus.Metric)
	defer close(metrics)

	go func() {
		i := 0
		for range metrics {
			i++
		}
	}()

	// make new profilers
	pids := collector.newProfilers(cgroups)
	assert.ElementsMatch(t, pids, []int{os.Getpid()})

	// update counters
	err = collector.updateHardwareCounters("1234", []procfs.Proc{{PID: os.Getpid()}}, metrics)
	require.NoError(t, err)

	err = collector.updateSoftwareCounters("1234", []procfs.Proc{{PID: os.Getpid()}}, metrics)
	require.NoError(t, err)

	err = collector.updateCacheCounters("1234", []procfs.Proc{{PID: os.Getpid()}}, metrics)
	require.NoError(t, err)

	// close and stop profilers
	collector.closeProfilers([]int{})

	// check the map should not contain the proc
	_, ok = collector.perfHwProfilers[os.Getpid()]
	assert.False(t, ok)

	_, ok = collector.perfSwProfilers[os.Getpid()]
	assert.False(t, ok)

	_, ok = collector.perfCacheProfilers[os.Getpid()]
	assert.False(t, ok)
}

func TestAggProfiles(t *testing.T) {
	// // Return address of constant
	// i := func(i uint64) *uint64 { return &i }
	var err error

	_, err = CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--collector.perf.hardware-events",
		"--collector.perf.software-events",
		"--collector.perf.hardware-cache-events",
		"--collector.cgroups.force-version", "v1",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	collector, err := NewPerfCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), cgManager)
	require.NoError(t, err)

	// Initialise state counters
	collector.lastRawHwCounters[46231] = make(map[string]perf.ProfileValue)
	collector.lastRawHwCounters[46281] = make(map[string]perf.ProfileValue)
	collector.lastRawCacheCounters[46231] = make(map[string]perf.ProfileValue)
	collector.lastRawCacheCounters[46281] = make(map[string]perf.ProfileValue)

	// First update
	// hardware counters
	hwProfiles := map[int]*perf.HardwareProfile{
		46231: {
			CPUCycles: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			Instructions: &perf.ProfileValue{
				Value:       50,
				TimeEnabled: 10,
				TimeRunning: 5,
			},
			BranchInstr: &perf.ProfileValue{
				Value:       10,
				TimeEnabled: 10,
				TimeRunning: 1,
			},
			BranchMisses: &perf.ProfileValue{
				Value:       90,
				TimeEnabled: 10,
				TimeRunning: 9,
			},
			CacheRefs: &perf.ProfileValue{
				Value:       50,
				TimeEnabled: 10,
				TimeRunning: 5,
			},
			CacheMisses: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			RefCPUCycles: &perf.ProfileValue{
				Value:       20,
				TimeEnabled: 10,
				TimeRunning: 2,
			},
		},
		46281: {
			CPUCycles: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			Instructions: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			BranchInstr: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			BranchMisses: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			CacheRefs: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			CacheMisses: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			RefCPUCycles: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
		},
	}
	expectedRawLastValues := map[int]map[string]perf.ProfileValue{
		46231: {
			"cpucycles_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			"branch_instructions_total": perf.ProfileValue{
				Value:       10,
				TimeEnabled: 10,
				TimeRunning: 1,
			},
			"instructions_total": perf.ProfileValue{
				Value:       50,
				TimeEnabled: 10,
				TimeRunning: 5,
			},
			"branch_misses_total": perf.ProfileValue{
				Value:       90,
				TimeEnabled: 10,
				TimeRunning: 9,
			},
			"cache_refs_total": perf.ProfileValue{
				Value:       50,
				TimeEnabled: 10,
				TimeRunning: 5,
			},
			"cache_misses_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			"ref_cpucycles_total": perf.ProfileValue{
				Value:       20,
				TimeEnabled: 10,
				TimeRunning: 2,
			},
		},
		46281: {
			"cpucycles_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"branch_instructions_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"instructions_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"branch_misses_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"cache_refs_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"cache_misses_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"ref_cpucycles_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
		},
	}
	expectedAggHwValues := map[string]float64{
		"cpucycles_total":           200,
		"branch_instructions_total": 200,
		"instructions_total":        200,
		"branch_misses_total":       200,
		"cache_refs_total":          200,
		"cache_misses_total":        200,
		"ref_cpucycles_total":       200,
	}

	cgroupCounters := collector.aggHardwareCounters(hwProfiles, make(map[string]float64))
	assert.EqualValues(t, expectedRawLastValues, collector.lastRawHwCounters)
	assert.EqualValues(t, expectedAggHwValues, cgroupCounters)

	cacheProfiles := map[int]*perf.CacheProfile{
		46231: {
			L1DataReadHit: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			L1DataReadMiss: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			L1DataWriteHit: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			L1InstrReadMiss: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			InstrTLBReadHit: &perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
		},
		46281: {
			L1DataReadHit: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			L1DataReadMiss: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			L1DataWriteHit: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			L1InstrReadMiss: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			InstrTLBReadHit: &perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
		},
	}
	expectedRawLastValues = map[int]map[string]perf.ProfileValue{
		46231: {
			"cache_l1d_read_hits_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			"cache_l1d_read_misses_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			"cache_l1d_write_hits_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			"cache_l1_instr_read_misses_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
			"cache_tlb_instr_read_hits_total": perf.ProfileValue{
				Value:       40,
				TimeEnabled: 10,
				TimeRunning: 4,
			},
		},
		46281: {
			"cache_l1d_read_hits_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"cache_l1d_read_misses_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"cache_l1d_write_hits_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"cache_l1_instr_read_misses_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
			"cache_tlb_instr_read_hits_total": perf.ProfileValue{
				Value:       60,
				TimeEnabled: 10,
				TimeRunning: 6,
			},
		},
	}
	expectedAggCacheValues := map[string]float64{
		"cache_l1d_read_hits_total":        200,
		"cache_l1d_read_misses_total":      200,
		"cache_l1d_write_hits_total":       200,
		"cache_l1_instr_read_misses_total": 200,
		"cache_tlb_instr_read_hits_total":  200,
	}

	cgroupCounters = collector.aggCacheCounters(cacheProfiles, make(map[string]float64))
	assert.EqualValues(t, expectedRawLastValues, collector.lastRawCacheCounters)
	assert.EqualValues(t, expectedAggCacheValues, cgroupCounters)

	// Second update
	hwProfiles = map[int]*perf.HardwareProfile{
		46231: {
			CPUCycles: &perf.ProfileValue{
				Value:       45,
				TimeEnabled: 20,
				TimeRunning: 5,
			},
		},
		46281: {CPUCycles: &perf.ProfileValue{
			Value:       80,
			TimeEnabled: 20,
			TimeRunning: 10,
		}},
	}

	cgroupCounters = collector.aggHardwareCounters(hwProfiles, expectedAggHwValues)
	assert.InDelta(t, 300, cgroupCounters["cpucycles_total"], 0)

	cacheProfiles = map[int]*perf.CacheProfile{
		46231: {L1DataReadHit: &perf.ProfileValue{
			Value:       45,
			TimeEnabled: 20,
			TimeRunning: 5,
		}},
		46281: {L1DataReadHit: &perf.ProfileValue{
			Value:       80,
			TimeEnabled: 20,
			TimeRunning: 10,
		}},
	}

	cgroupCounters = collector.aggCacheCounters(cacheProfiles, expectedAggCacheValues)
	assert.InDelta(t, 300, cgroupCounters["cache_l1d_read_hits_total"], 0)
}

func TestPIDEviction(t *testing.T) {
	var err error

	_, err = CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
		"--path.cgroupfs", "testdata/sys/fs/cgroup",
		"--collector.perf.hardware-events",
		"--collector.perf.software-events",
		"--collector.perf.hardware-cache-events",
		"--collector.cgroups.force-version", "v1",
	})
	require.NoError(t, err)

	// cgroup manager
	cgManager, err := NewCgroupManager("slurm", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	collector, err := NewPerfCollector(slog.New(slog.NewTextHandler(io.Discard, nil)), cgManager)
	require.NoError(t, err)

	// Use fake processes
	activePIDs := []int{
		12341, 12342, 12343,
	}

	// Setup background goroutine to capture metrics.
	metrics := make(chan prometheus.Metric)
	defer close(metrics)

	go func() {
		i := 0
		for range metrics {
			i++
		}
	}()

	// update counters
	collector.updateStateMaps(activePIDs, []string{"1234"})

	// check if last raw and scaled counters are populated
	for _, pid := range activePIDs {
		assert.NotNil(t, collector.lastRawHwCounters[pid])
		assert.NotNil(t, collector.lastRawCacheCounters[pid])
	}

	assert.NotNil(t, collector.lastCgroupHwCounters["1234"])
	assert.NotNil(t, collector.lastCgroupCacheCounters["1234"])

	// Updated procs
	activePIDs = []int{
		12342, 12343, 12344,
	}

	// update counters
	collector.updateStateMaps(activePIDs, []string{"1235"})

	// check if last raw and scaled counters are populated
	for _, pid := range activePIDs {
		assert.NotNil(t, collector.lastRawHwCounters[pid])
		assert.NotNil(t, collector.lastRawCacheCounters[pid])
	}

	assert.NotNil(t, collector.lastCgroupHwCounters["1235"])
	assert.NotNil(t, collector.lastCgroupCacheCounters["1235"])

	// Check if we evicted finished process
	assert.Nil(t, collector.lastRawHwCounters[12341])
	assert.Nil(t, collector.lastRawCacheCounters[12341])
	assert.Nil(t, collector.lastCgroupHwCounters["1234"])
	assert.Nil(t, collector.lastCgroupCacheCounters["1234"])
}
