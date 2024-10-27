//go:build !noperf
// +build !noperf

package collector

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/go-kit/log"
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
		ignoreProc: func(p string) bool {
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

	err = collector.Update(metrics, nil)
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
	cgManager, err := NewCgroupManager("slurm", log.NewNopLogger())
	require.NoError(t, err)

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
	expectedCgroupIDs := []string{"1009248", "1009249"}
	expectedCgroupProcs := map[string][]int{
		"1009248": {46231, 46281},
		"1009249": {46235, 46236},
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
	cgManager, err := NewCgroupManager("slurm", log.NewNopLogger())
	require.NoError(t, err)

	collector, err := NewPerfCollector(log.NewNopLogger(), cgManager)
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
