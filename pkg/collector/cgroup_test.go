//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCgroupCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Unified,
		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
		idRegex:    slurmCgroupPathRegex,
	}

	// opts
	opts := cgroupOpts{
		collectSwapMemStats: true,
		collectPSIStats:     true,
	}

	collector, err := NewCgroupCollector(log.NewNopLogger(), cgManager, opts)
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

func TestCgroupsV2Metrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Unified,
		mountPoint: "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
		idRegex:    slurmCgroupPathRegex,
	}

	// opts
	opts := cgroupOpts{
		collectSwapMemStats: true,
		collectPSIStats:     true,
	}

	c := cgroupCollector{
		cgroupManager: cgManager,
		opts:          opts,
		hostMemTotal:  float64(123456),
		logger:        log.NewNopLogger(),
	}

	expectedMetrics := cgMetric{
		path:            "/system.slice/slurmstepd.scope/job_1009249",
		cpuUser:         60375.292848,
		cpuSystem:       115.777502,
		cpuTotal:        60491.070351,
		cpus:            2,
		cpuPressure:     0,
		memoryRSS:       4.098592768e+09,
		memoryCache:     0,
		memoryUsed:      4.111491072e+09,
		memoryTotal:     4.294967296e+09,
		memoryFailCount: 0,
		memswUsed:       0,
		memswTotal:      123456,
		memswFailCount:  0,
		memoryPressure:  0,
		rdmaHCAHandles:  map[string]float64{"hfi1_0": 479, "hfi1_1": 1479, "hfi1_2": 2479},
		rdmaHCAObjects:  map[string]float64{"hfi1_0": 340, "hfi1_1": 1340, "hfi1_2": 2340},
		blkioReadBytes:  map[string]float64{},
		blkioWriteBytes: map[string]float64{},
		blkioReadReqs:   map[string]float64{},
		blkioWriteReqs:  map[string]float64{},
		blkioPressure:   0,
		err:             false,
	}

	metric := c.doUpdate([]cgMetric{{path: expectedMetrics.path}})
	assert.Equal(t, expectedMetrics, metric[0])
}

func TestCgroupsV1Metrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		mode:       cgroups.Legacy,
		mountPoint: "testdata/sys/fs/cgroup/cpuacct/slurm",
		idRegex:    slurmCgroupPathRegex,
	}

	// opts
	opts := cgroupOpts{
		collectSwapMemStats: true,
		collectPSIStats:     true,
	}

	c := cgroupCollector{
		logger:        log.NewNopLogger(),
		cgroupManager: cgManager,
		opts:          opts,
		hostMemTotal:  float64(123456),
	}

	expectedMetrics := cgMetric{
		path:            "/slurm/uid_1000/job_1009248",
		cpuUser:         0.39,
		cpuSystem:       0.45,
		cpuTotal:        1.012410966,
		cpus:            0,
		cpuPressure:     0,
		memoryRSS:       1.0407936e+07,
		memoryCache:     2.1086208e+07,
		memoryUsed:      4.0194048e+07,
		memoryTotal:     2.01362030592e+11,
		memoryFailCount: 0,
		memswUsed:       4.032512e+07,
		memswTotal:      9.223372036854772e+18,
		memswFailCount:  0,
		memoryPressure:  0,
		rdmaHCAHandles:  map[string]float64{"hfi1_0": 479, "hfi1_1": 1479, "hfi1_2": 2479},
		rdmaHCAObjects:  map[string]float64{"hfi1_0": 340, "hfi1_1": 1340, "hfi1_2": 2340},
		err:             false,
	}

	metric := c.doUpdate([]cgMetric{{path: expectedMetrics.path}})
	assert.Equal(t, expectedMetrics, metric[0])
}

func TestNewCgroupManagerV2(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	// Slurm case
	manager, err := NewCgroupManager("slurm", log.NewNopLogger())
	require.NoError(t, err)

	assert.Equal(t, "testdata/sys/fs/cgroup/system.slice/slurmstepd.scope", manager.mountPoint)
	assert.NotNil(t, manager.isChild)
	assert.NotNil(t, manager.ignoreProc)

	cgroups, err := manager.discover()
	require.NoError(t, err)
	assert.Len(t, cgroups, 3)

	// libvirt case
	manager, err = NewCgroupManager("libvirt", log.NewNopLogger())
	require.NoError(t, err)

	assert.Equal(t, "testdata/sys/fs/cgroup/machine.slice", manager.mountPoint)
	assert.NotNil(t, manager.isChild)
	assert.NotNil(t, manager.ignoreProc)

	cgroups, err = manager.discover()
	require.NoError(t, err)
	assert.Len(t, cgroups, 4)
}

func TestNewCgroupManagerV1(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v1",
		},
	)
	require.NoError(t, err)

	// Slurm case
	manager, err := NewCgroupManager("slurm", log.NewNopLogger())
	require.NoError(t, err)

	assert.Equal(t, "testdata/sys/fs/cgroup/cpuacct/slurm", manager.mountPoint)
	assert.NotNil(t, manager.isChild)
	assert.NotNil(t, manager.ignoreProc)

	cgroups, err := manager.discover()
	require.NoError(t, err)
	assert.Len(t, cgroups, 3)

	// libvirt case
	manager, err = NewCgroupManager("libvirt", log.NewNopLogger())
	require.NoError(t, err)

	assert.Equal(t, "testdata/sys/fs/cgroup/cpuacct/machine.slice", manager.mountPoint)
	assert.NotNil(t, manager.isChild)
	assert.NotNil(t, manager.ignoreProc)

	cgroups, err = manager.discover()
	require.NoError(t, err)
	assert.Len(t, cgroups, 4)

	// Check error for unknown resource manager
	_, err = NewCgroupManager("unknown", log.NewNopLogger())
	assert.Error(t, err)
}

func TestParseCgroupSubSysIds(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.procfs", "testdata/proc",
		},
	)
	require.NoError(t, err)

	controllers, err := parseCgroupSubSysIds()
	require.NoError(t, err)

	expectedControllers := []cgroupController{
		{
			id:     5,
			idx:    0,
			name:   "cpuset",
			active: true,
		},
		{
			id:     6,
			idx:    1,
			name:   "cpu",
			active: true,
		},
		{
			id:     6,
			idx:    2,
			name:   "cpuacct",
			active: true,
		},
		{
			id:     12,
			idx:    3,
			name:   "blkio",
			active: true,
		},
		{
			id:     7,
			idx:    4,
			name:   "memory",
			active: true,
		},
		{
			id:     11,
			idx:    5,
			name:   "devices",
			active: true,
		},
		{
			id:     2,
			idx:    6,
			name:   "freezer",
			active: true,
		},
		{
			id:     4,
			idx:    7,
			name:   "net_cls",
			active: true,
		},
		{
			id:     3,
			idx:    8,
			name:   "perf_event",
			active: true,
		},
		{
			id:     4,
			idx:    9,
			name:   "net_prio",
			active: true,
		},
		{
			id:     8,
			idx:    10,
			name:   "hugetlb",
			active: true,
		},
		{
			id:     9,
			idx:    11,
			name:   "pids",
			active: true,
		},
		{
			id:     10,
			idx:    12,
			name:   "rdma",
			active: true,
		},
	}

	assert.ElementsMatch(t, expectedControllers, controllers)
}
