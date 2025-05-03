//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"testing"

	"github.com/containerd/cgroups/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCgroupManagerV2(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name        string
		manager     manager
		mountPoints []string
		numCgroups  int
	}{
		{
			name:    "slurm",
			manager: slurm,
			mountPoints: []string{
				"testdata/sys/fs/cgroup/system.slice/slurmstepd.scope",
				"testdata/sys/fs/cgroup/system.slice/node0_slurmstepd.scope",
				"testdata/sys/fs/cgroup/system.slice/node1_slurmstepd.scope",
			},
			numCgroups: 9,
		},
		{
			name:        "libvirt",
			manager:     libvirt,
			mountPoints: []string{"testdata/sys/fs/cgroup/machine.slice"},
			numCgroups:  4,
		},
		{
			name:    "k8s",
			manager: k8s,
			mountPoints: []string{
				"testdata/sys/fs/cgroup/kubepods",
			},
			numCgroups: 10,
		},
	}

	noLogger := noOpLogger

	for _, test := range tests {
		manager, err := NewCgroupManager(test.manager, noLogger)
		require.NoError(t, err)

		assert.ElementsMatch(t, test.mountPoints, manager.mountPoints)
		assert.NotNil(t, manager.isChild)
		assert.NotNil(t, manager.ignoreProc)

		cgroups, err := manager.discover()
		require.NoError(t, err)
		assert.Len(t, cgroups, test.numCgroups)
	}
}

func TestNewCgroupManagerV1(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v1",
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name        string
		manager     manager
		mountPoints []string
		numCgroups  int
	}{
		{
			name:    "slurm",
			manager: slurm,
			mountPoints: []string{
				"testdata/sys/fs/cgroup/cpu,cpuacct/slurm",
				"testdata/sys/fs/cgroup/cpu,cpuacct/slurm_host0",
				"testdata/sys/fs/cgroup/cpu,cpuacct/slurm_host1",
			},
			numCgroups: 9,
		},
		{
			name:        "libvirt",
			manager:     libvirt,
			mountPoints: []string{"testdata/sys/fs/cgroup/cpu,cpuacct/machine.slice"},
			numCgroups:  4,
		},
		{
			name:    "k8s",
			manager: k8s,
			mountPoints: []string{
				"testdata/sys/fs/cgroup/cpu,cpuacct/kubepods",
			},
			numCgroups: 10,
		},
	}

	noLogger := noOpLogger

	for _, test := range tests {
		manager, err := NewCgroupManager(test.manager, noLogger)
		require.NoError(t, err)

		assert.ElementsMatch(t, test.mountPoints, manager.mountPoints)
		assert.NotNil(t, manager.isChild)
		assert.NotNil(t, manager.ignoreProc)

		cgroups, err := manager.discover()
		require.NoError(t, err)
		assert.Len(t, cgroups, test.numCgroups)
	}

	// Check error for unknown resource manager
	_, err = NewCgroupManager(-1, noOpLogger)
	assert.Error(t, err)
}

func TestNewCgroupCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	// cgroup Manager
	cgManager := &cgroupManager{
		id:          libvirt,
		mode:        cgroups.Unified,
		mountPoints: []string{"testdata/sys/fs/cgroup/machine.slice"},
		idRegex:     libvirtCgroupPathRegex,
		isChild:     func(s string) bool { return false },
		ignoreProc:  func(s string) bool { return false },
	}

	// opts
	opts := cgroupOpts{
		collectSwapMemStats: true,
		collectPSIStats:     true,
		collectBlockIOStats: true,
	}

	collector, err := NewCgroupCollector(noOpLogger, cgManager, opts)
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

	cgroups, err := cgManager.discover()
	require.NoError(t, err)

	err = collector.Update(metrics, cgroups)
	require.NoError(t, err)

	err = collector.Stop(context.Background())
	require.NoError(t, err)
}

func TestCgroupsV2Metrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v2",
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name     string
		manager  manager
		expected cgMetric
	}{
		{
			name:    "slurm",
			manager: slurm,
			expected: cgMetric{
				cgroup:          cgroup{uuid: "1009249", path: cgroupPath{rel: "/system.slice/slurmstepd.scope/job_1009249"}},
				cpuUser:         60375.292848,
				cpuSystem:       115.777502,
				cpuTotal:        60491.070351,
				cpus:            2000,
				cpuPressure:     0,
				memoryRSS:       4.098592768e+09,
				memoryCache:     0,
				memoryUsed:      4.111491072e+09,
				memoryTotal:     4.294967296e+09,
				memoryFailCount: 0,
				memswUsed:       0,
				memswTotal:      1234,
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
			},
		},
		{
			name:    "libvirt",
			manager: libvirt,
			expected: cgMetric{
				cgroup:          cgroup{uuid: "instance-00000002", path: cgroupPath{rel: "/machine.slice/machine-qemu\\x2d1\\x2dinstance\\x2d00000002.scope"}},
				cpuUser:         60375.292848,
				cpuSystem:       115.777502,
				cpuTotal:        60491.070351,
				cpus:            1000,
				cpuPressure:     0,
				memoryRSS:       4.098592768e+09,
				memoryCache:     0,
				memoryUsed:      4.111491072e+09,
				memoryTotal:     4.294967296e+09,
				memoryFailCount: 0,
				memswUsed:       0,
				memswTotal:      1234,
				memswFailCount:  0,
				memoryPressure:  0,
				rdmaHCAHandles:  map[string]float64{},
				rdmaHCAObjects:  map[string]float64{},
				blkioReadBytes: map[string]float64{
					"": 3.0206976e+07,
				},
				blkioWriteBytes: map[string]float64{
					"": 1.00337664e+09,
				},
				blkioReadReqs: map[string]float64{
					"": 1141,
				},
				blkioWriteReqs: map[string]float64{
					"": 14997,
				},
				blkioPressure: 0.433924,
				err:           false,
			},
		},
		{
			name:    "k8s",
			manager: k8s,
			expected: cgMetric{
				cgroup:          cgroup{uuid: "6d06282c-0377-4527-9a0f-9968bc9c4102", path: cgroupPath{rel: "/kubepods/burstable/pod6d06282c-0377-4527-9a0f-9968bc9c4102"}},
				cpuUser:         1.434007,
				cpuSystem:       1.561373,
				cpuTotal:        2.995381,
				cpus:            78,
				cpuPressure:     0.544266,
				memoryRSS:       1.1849728e+07,
				memoryCache:     335872,
				memoryUsed:      1.30048e+07,
				memoryTotal:     1.7825792e+08,
				memoryFailCount: 0,
				memswUsed:       36864,
				memswTotal:      1234,
				memswFailCount:  0,
				memoryPressure:  0,
				rdmaHCAHandles:  map[string]float64{},
				rdmaHCAObjects:  map[string]float64{},
				blkioReadBytes: map[string]float64{
					"": 274432,
				},
				blkioWriteBytes: map[string]float64{
					"": 36864,
				},
				blkioReadReqs: map[string]float64{
					"": 7,
				},
				blkioWriteReqs: map[string]float64{
					"": 9,
				},
				blkioPressure: 0.16605,
				err:           false,
			},
		},
	}

	noLogger := noOpLogger

	for _, test := range tests {
		manager, err := NewCgroupManager(test.manager, noLogger)
		require.NoError(t, err)

		// opts
		opts := cgroupOpts{
			collectSwapMemStats: true,
			collectPSIStats:     true,
		}

		c := cgroupCollector{
			cgroupManager: manager,
			opts:          opts,
			hostMemInfo:   map[string]float64{"MemTotal_bytes": float64(123456), "SwapTotal_bytes": float64(1234)},
			logger:        noLogger,
		}

		metrics := c.update([]cgroup{test.expected.cgroup})
		assert.Equal(t, test.expected, metrics[0], test.name)
	}
}

func TestCgroupsV1Metrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.cgroups.force-version", "v1",
		},
	)
	require.NoError(t, err)

	tests := []struct {
		name     string
		manager  manager
		expected cgMetric
	}{
		{
			name:    "slurm",
			manager: slurm,
			expected: cgMetric{
				cgroup:          cgroup{uuid: "1009249", path: cgroupPath{rel: "/slurm/uid_1000/job_1009248"}},
				cpuUser:         0.39,
				cpuSystem:       0.45,
				cpuTotal:        1.012410966,
				cpus:            2000,
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
			},
		},
		{
			name:    "libvirt",
			manager: libvirt,
			expected: cgMetric{
				cgroup:          cgroup{uuid: "instance-00000001", path: cgroupPath{rel: "/machine.slice/machine-qemu\\x2d2\\x2dinstance\\x2d00000001.scope"}},
				cpuUser:         0.39,
				cpuSystem:       0.45,
				cpuTotal:        1.012410966,
				cpus:            2000,
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
				blkioReadBytes: map[string]float64{
					"": 3.25280768e+08,
				},
				blkioWriteBytes: map[string]float64{
					"": 3.088384e+07,
				},
				blkioReadReqs: map[string]float64{
					"": 10957,
				},
				blkioWriteReqs: map[string]float64{
					"": 4803,
				},
				err: false,
			},
		},
		{
			name:    "k8s",
			manager: k8s,
			expected: cgMetric{
				cgroup:          cgroup{uuid: "3a61e77f-1538-476b-8231-5af9eed40fdc", path: cgroupPath{rel: "/kubepods/besteffort/pod3a61e77f-1538-476b-8231-5af9eed40fdc"}},
				cpuUser:         1.39,
				cpuSystem:       1.59,
				cpuTotal:        3.686596141,
				cpus:            1,
				cpuPressure:     0,
				memoryRSS:       1.2750848e+07,
				memoryCache:     16384,
				memoryUsed:      2.4354816e+07,
				memoryTotal:     9.223372036854772e+18,
				memoryFailCount: 0,
				memswUsed:       0,
				memswTotal:      0,
				memswFailCount:  0,
				memoryPressure:  0,
				rdmaHCAHandles:  map[string]float64{},
				rdmaHCAObjects:  map[string]float64{},
				blkioReadBytes: map[string]float64{
					"": 0,
				},
				blkioWriteBytes: map[string]float64{
					"": 0,
				},
				blkioReadReqs: map[string]float64{
					"": 0,
				},
				blkioWriteReqs: map[string]float64{
					"": 0,
				},
				blkioPressure: 0,
				err:           false,
			},
		},
	}

	noLogger := noOpLogger

	for _, test := range tests {
		manager, err := NewCgroupManager(test.manager, noLogger)
		require.NoError(t, err)

		// opts
		opts := cgroupOpts{
			collectSwapMemStats: true,
			collectPSIStats:     true,
		}

		c := cgroupCollector{
			cgroupManager: manager,
			opts:          opts,
			hostMemInfo:   map[string]float64{"MemTotal_bytes": float64(123456), "SwapTotal_bytes": float64(1234)},
			logger:        noLogger,
		}

		metrics := c.update([]cgroup{test.expected.cgroup})
		assert.Equal(t, test.expected, metrics[0], test.name)
	}
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
