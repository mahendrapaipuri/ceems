//go:build !perf
// +build !perf

package collector

import (
	"context"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{"--path.procfs", "testdata/proc"})
	require.NoError(t, err)

	collector, err := NewPerfCollector(log.NewNopLogger())
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

func TestPerfCollectorWithSlurm(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{"--path.procfs", "testdata/proc", "--collector.slurm"},
	)
	require.NoError(t, err)

	collector, err := NewPerfCollector(log.NewNopLogger())
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
}

func TestDiscoverProcess(t *testing.T) {
	var err error

	collector := perfCollector{
		logger:                    log.NewNopLogger(),
		envVar:                    "ENABLE_PROFILING",
		cgroupIDRegex:             slurmCgroupPathRegex,
		filterProcCmdRegex:        slurmIgnoreProcsRegex,
		perfHwProfilersEnabled:    true,
		perfSwProfilersEnabled:    true,
		perfCacheProfilersEnabled: true,
	}

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

	collector := perfCollector{
		logger:                    log.NewNopLogger(),
		cgroupIDRegex:             slurmCgroupPathRegex,
		filterProcCmdRegex:        slurmIgnoreProcsRegex,
		perfHwProfilersEnabled:    true,
		perfSwProfilersEnabled:    true,
		perfCacheProfilersEnabled: true,
	}

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
	_, ok = collector.perfHwProfilers.Load(os.Getpid())
	assert.False(t, ok)

	_, ok = collector.perfSwProfilers.Load(os.Getpid())
	assert.False(t, ok)

	_, ok = collector.perfCacheProfilers.Load(os.Getpid())
	assert.False(t, ok)
}
