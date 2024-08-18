//go:build !noslurm
// +build !noslurm

package collector

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedSlurmMetrics CgroupMetric

func mockGPUDevices() map[int]Device {
	devs := make(map[int]Device, 4)

	for i := 0; i <= 4; i++ {
		idxString := strconv.Itoa(i)
		devs[i] = Device{index: idxString, uuid: fmt.Sprintf("GPU-%d", i)}
	}

	return devs
}

func TestNewSlurmCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.create-unique-jobids",
			"--collector.slurm.job-props-path", "testdata/slurmjobprops",
			"--collector.slurm.gpu-job-map-path", "testdata/gpujobmap",
		},
	)
	require.NoError(t, err)

	_, err = NewSlurmCollector(log.NewNopLogger())
	assert.NoError(t, err)
}

func TestCgroupsV2SlurmJobMetrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.create-unique-jobids",
			"--collector.slurm.job-props-path", "testdata/slurmjobprops",
			"--collector.slurm.gpu-job-map-path", "testdata/gpujobmap",
		},
	)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v2",
		gpuDevs:          mockGPUDevices(),
		cgroupsRootPath:  *cgroupfsPath,
		hostMemTotal:     float64(123456),
		slurmCgroupsPath: *cgroupfsPath + "/system.slice/slurmstepd.scope",
		logger:           log.NewNopLogger(),
	}
	metrics, err := c.getJobsMetrics()
	expectedSlurmMetrics = CgroupMetric{
		name:            "/system.slice/slurmstepd.scope/job_1009249",
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
		jobuser:         "testusr2",
		jobaccount:      "testacc2",
		jobid:           "1009249",
		jobuuid:         "018ce2fe-b3f9-632a-7507-0e01c2687de5",
		jobgpuordinals:  []string{"0"},
		err:             false,
	}

	require.NoError(t, err)
	assert.Equal(t, expectedSlurmMetrics, metrics["1009249"])
}

func TestCgroupsV2SlurmJobMetricsWithProcFs(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.create-unique-jobids",
			"--path.procfs", "testdata/proc",
		},
	)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v2",
		cgroupsRootPath:  *cgroupfsPath,
		gpuDevs:          mockGPUDevices(),
		hostMemTotal:     float64(123456),
		slurmCgroupsPath: *cgroupfsPath + "/system.slice/slurmstepd.scope",
		logger:           log.NewNopLogger(),
	}
	metrics, err := c.getJobsMetrics()
	expectedSlurmMetrics = CgroupMetric{
		name:            "/system.slice/slurmstepd.scope/job_1009248",
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
		rdmaHCAHandles:  make(map[string]float64),
		rdmaHCAObjects:  make(map[string]float64),
		jobuser:         "testusr",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5",
		jobgpuordinals:  []string{"2", "3"},
		err:             false,
	}

	require.NoError(t, err)
	assert.Equal(t, expectedSlurmMetrics, metrics["1009248"])
}

func TestCgroupsV2SlurmJobMetricsNoJobProps(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--collector.slurm.create-unique-jobids",
		},
	)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v2",
		cgroupsRootPath:  *cgroupfsPath,
		gpuDevs:          mockGPUDevices(),
		slurmCgroupsPath: *cgroupfsPath + "/system.slice/slurmstepd.scope",
		logger:           log.NewNopLogger(),
	}
	metrics, err := c.getJobsMetrics()
	expectedSlurmMetrics = CgroupMetric{
		name:            "/system.slice/slurmstepd.scope/job_1009248",
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
		memswTotal:      1.8446744073709552e+19,
		memswFailCount:  0,
		memoryPressure:  0,
		rdmaHCAHandles:  make(map[string]float64),
		rdmaHCAObjects:  make(map[string]float64),
		jobuser:         "",
		jobaccount:      "",
		jobid:           "1009248",
		jobuuid:         "a0523e93-a037-c2b1-8b34-410c9996399c",
		err:             false,
	}

	require.NoError(t, err)
	assert.Equal(t, expectedSlurmMetrics, metrics["1009248"])
}

func TestCgroupsV1SlurmJobMetrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
			"--collector.slurm.create-unique-jobids",
			"--collector.slurm.job-props-path", "testdata/slurmjobprops",
		},
	)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v1",
		logger:           log.NewNopLogger(),
		gpuDevs:          mockGPUDevices(),
		cgroupsRootPath:  *cgroupfsPath + "/cpuacct",
		slurmCgroupsPath: *cgroupfsPath + "/cpuacct/slurm",
	}
	metrics, err := c.getJobsMetrics()
	expectedSlurmMetrics = CgroupMetric{
		name:            "/slurm/uid_1000/job_1009248",
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
		jobuser:         "testusr",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5",
		jobgpuordinals:  []string{"2", "3"},
		err:             false,
	}

	require.NoError(t, err)
	assert.Equal(t, expectedSlurmMetrics, metrics["1009248"])
}
