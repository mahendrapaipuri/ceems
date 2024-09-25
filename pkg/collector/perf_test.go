//go:build !perf
// +build !perf

package collector

import (
	"context"
	"os"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/go-kit/log"
	"github.com/hodgesds/perf-utils"
	"github.com/mahendrapaipuri/ceems/internal/security"
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
		mode:       cgroups.Unified,
		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
		idRegex:    slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	collector, err := NewPerfCollector(log.NewNopLogger(), cgManager)
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

	err = collector.Update(metrics)
	require.NoError(t, err)

	err = collector.Stop(context.Background())
	require.NoError(t, err)
}

func TestDiscoverProcess(t *testing.T) {
	var err error

	// cgroup manager
	cgManager := &cgroupManager{
		mode:    cgroups.Unified,
		idRegex: slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	// perf opts
	opts := perfOpts{
		perfHwProfilersEnabled:    true,
		perfSwProfilersEnabled:    true,
		perfCacheProfilersEnabled: true,
		targetEnvVars:             []string{"ENABLE_PROFILING"},
	}

	collector := perfCollector{
		logger:           log.NewNopLogger(),
		cgroupManager:    cgManager,
		opts:             opts,
		securityContexts: make(map[string]*security.SecurityContext),
	}

	// Create dummy security context
	collector.securityContexts[perfDiscovererCtx], err = security.NewSecurityContext(perfDiscovererCtx, nil, discoverer, collector.logger)
	require.NoError(t, err)

	collector.fs, err = procfs.NewFS("testdata/proc")
	require.NoError(t, err)

	// Discover processes
	cgroupIDProcMap, err := collector.discoverProcess()
	require.NoError(t, err)

	// expected
	expectedCgroupIDs := []string{"1320003", "4824887"}
	expectedCgroupProcs := map[string][]int{
		"4824887": {46231, 46281},
		"1320003": {46235, 46236},
	}

	// Get cgroup IDs
	var cgroupIDs []string

	cgroupProcs := make(map[string][]int)

	for cgroupID, procs := range cgroupIDProcMap {
		cgroupIDs = append(cgroupIDs, cgroupID)

		var pids []int
		for _, proc := range procs {
			pids = append(pids, proc.PID)
		}

		cgroupProcs[cgroupID] = pids
	}

	assert.ElementsMatch(t, cgroupIDs, expectedCgroupIDs)

	for _, cgroupID := range cgroupIDs {
		assert.ElementsMatch(t, cgroupProcs[cgroupID], expectedCgroupProcs[cgroupID])
	}
}

func TestNewProfilers(t *testing.T) {
	var err error

	var ok bool

	// cgroup manager
	cgManager := &cgroupManager{
		mode:    cgroups.Legacy,
		idRegex: slurmCgroupPathRegex,
		procFilter: func(p string) bool {
			return slurmIgnoreProcsRegex.MatchString(p)
		},
	}

	// perf opts
	opts := perfOpts{
		perfHwProfilersEnabled:    true,
		perfSwProfilersEnabled:    true,
		perfCacheProfilersEnabled: true,
	}

	collector := perfCollector{
		logger:             log.NewNopLogger(),
		cgroupManager:      cgManager,
		opts:               opts,
		perfHwProfilers:    make(map[int]*perf.HardwareProfiler),
		perfSwProfilers:    make(map[int]*perf.SoftwareProfiler),
		perfCacheProfilers: make(map[int]*perf.CacheProfiler),
		securityContexts:   make(map[string]*security.SecurityContext),
	}

	// Create dummy security context
	collector.securityContexts[perfOpenProfilersCtx], err = security.NewSecurityContext(perfOpenProfilersCtx, nil, openProfilers, collector.logger)
	require.NoError(t, err)

	collector.securityContexts[perfCloseProfilersCtx], err = security.NewSecurityContext(perfCloseProfilersCtx, nil, closeProfilers, collector.logger)
	require.NoError(t, err)

	collector.fs, err = procfs.NewFS("testdata/proc")
	require.NoError(t, err)

	// Use fake cgroupID for current process
	cgroupIDProcMap := map[string][]procfs.Proc{
		"1234": {
			{
				PID: os.Getpid(),
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
	pids := collector.newProfilers(cgroupIDProcMap)
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
