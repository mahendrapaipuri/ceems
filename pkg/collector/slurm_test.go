//go:build !noslurm
// +build !noslurm

package collector

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
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
			"--collector.slurm.gpu-job-map-path", "testdata/gpujobmap",
			"--collector.slurm.swap-memory-metrics",
			"--collector.slurm.psi-metrics",
			"--collector.slurm.nvidia-smi-path", "testdata/nvidia-smi",
			"--collector.slurm.force-cgroups-version", "v2",
		},
	)
	require.NoError(t, err)

	collector, err := NewSlurmCollector(log.NewNopLogger())
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

func TestCgroupsV2SlurmJobMetrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
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
		jobsCache:        make(map[string]jobProps),
	}

	expectedSlurmMetrics = CgroupMetric{
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
		jobuuid:         "1009249",
		jobgpuordinals:  []string{"0"},
		err:             false,
	}

	metrics, err := c.getJobsMetrics()
	require.NoError(t, err)

	var gotMetric CgroupMetric

	for _, metric := range metrics {
		if metric.jobuuid == expectedSlurmMetrics.jobuuid {
			gotMetric = metric
		}
	}

	assert.Equal(t, expectedSlurmMetrics, gotMetric)
}

func TestCgroupsV2SlurmJobMetricsWithProcFs(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
		},
	)
	require.NoError(t, err)

	procFS, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v2",
		cgroupsRootPath:  *cgroupfsPath,
		gpuDevs:          mockGPUDevices(),
		hostMemTotal:     float64(123456),
		slurmCgroupsPath: *cgroupfsPath + "/system.slice/slurmstepd.scope",
		logger:           log.NewNopLogger(),
		jobsCache:        make(map[string]jobProps),
		procFS:           procFS,
	}

	expectedSlurmMetrics = CgroupMetric{
		path:            "/system.slice/slurmstepd.scope/job_1009248",
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
		jobuuid:         "1009248",
		jobgpuordinals:  []string{"2", "3"},
		err:             false,
	}

	metrics, err := c.getJobsMetrics()
	require.NoError(t, err)

	var gotMetric CgroupMetric

	for _, metric := range metrics {
		if metric.jobuuid == expectedSlurmMetrics.jobuuid {
			gotMetric = metric
		}
	}

	assert.Equal(t, expectedSlurmMetrics, gotMetric)
}

func TestCgroupsV2SlurmJobMetricsNoJobProps(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
		},
	)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v2",
		cgroupsRootPath:  *cgroupfsPath,
		gpuDevs:          mockGPUDevices(),
		slurmCgroupsPath: *cgroupfsPath + "/system.slice/slurmstepd.scope",
		logger:           log.NewNopLogger(),
		jobsCache:        make(map[string]jobProps),
	}

	expectedSlurmMetrics = CgroupMetric{
		path:            "/system.slice/slurmstepd.scope/job_1009248",
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
		jobuuid:         "1009248",
		err:             false,
	}

	metrics, err := c.getJobsMetrics()
	require.NoError(t, err)

	var gotMetric CgroupMetric

	for _, metric := range metrics {
		if metric.jobuuid == expectedSlurmMetrics.jobuuid {
			gotMetric = metric
		}
	}

	assert.Equal(t, expectedSlurmMetrics, gotMetric)
}

func TestCgroupsV1SlurmJobMetrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "testdata/sys/fs/cgroup",
			"--path.procfs", "testdata/proc",
		},
	)
	require.NoError(t, err)

	procFS, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	c := slurmCollector{
		cgroups:          "v1",
		logger:           log.NewNopLogger(),
		gpuDevs:          mockGPUDevices(),
		cgroupsRootPath:  *cgroupfsPath + "/cpuacct",
		slurmCgroupsPath: *cgroupfsPath + "/cpuacct/slurm",
		jobsCache:        make(map[string]jobProps),
		procFS:           procFS,
	}

	expectedSlurmMetrics = CgroupMetric{
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
		jobuuid:         "1009248",
		jobgpuordinals:  []string{"2", "3"},
		err:             false,
	}

	metrics, err := c.getJobsMetrics()
	require.NoError(t, err)

	var gotMetric CgroupMetric

	for _, metric := range metrics {
		if metric.jobuuid == expectedSlurmMetrics.jobuuid {
			gotMetric = metric
		}
	}

	assert.Equal(t, expectedSlurmMetrics, gotMetric)
}

func TestJobPropsCaching(t *testing.T) {
	path := t.TempDir()

	cgroupsPath := path + "/cgroups"
	err := os.Mkdir(cgroupsPath, 0o750)
	require.NoError(t, err)

	gpuMapFilePath := path + "/gpu-map"
	err = os.Mkdir(gpuMapFilePath, 0o750)
	require.NoError(t, err)

	_, err = CEEMSExporterApp.Parse(
		[]string{
			"--path.cgroupfs", cgroupsPath,
			"--collector.slurm.gpu-job-map-path", gpuMapFilePath,
		},
	)
	require.NoError(t, err)

	mockGPUDevs := mockGPUDevices()
	c := slurmCollector{
		cgroups:          "v1",
		logger:           log.NewNopLogger(),
		gpuDevs:          mockGPUDevs,
		cgroupsRootPath:  *cgroupfsPath + "/cpuacct",
		slurmCgroupsPath: *cgroupfsPath + "/cpuacct/slurm",
		jobsCache:        make(map[string]jobProps),
	}

	// Add cgroups
	for i := range 20 {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Binds GPUs to first n jobs
	for igpu := range mockGPUDevs {
		err = os.WriteFile(fmt.Sprintf("%s/%d", gpuMapFilePath, igpu), []byte(strconv.FormatInt(int64(igpu), 10)), 0o600)
		require.NoError(t, err)
	}

	// Now call get metrics which should populate jobsCache
	_, err = c.getJobsMetrics()
	require.NoError(t, err)

	// Check if jobsCache has 20 jobs and GPU ordinals are correct
	assert.Len(t, c.jobsCache, 20)

	for igpu := range mockGPUDevs {
		gpuIDString := strconv.FormatInt(int64(igpu), 10)
		assert.Equal(t, []string{gpuIDString}, c.jobsCache[gpuIDString].gpuOrdinals)
	}

	// Remove first 10 jobs and add new 10 more jobs
	for i := range 10 {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.RemoveAll(dir)
		require.NoError(t, err)
	}

	for i := 19; i < 25; i++ {
		dir := fmt.Sprintf("%s/cpuacct/slurm/job_%d", cgroupsPath, i)

		err = os.MkdirAll(dir, 0o750)
		require.NoError(t, err)
	}

	// Now call again get metrics which should populate jobsCache
	_, err = c.getJobsMetrics()
	require.NoError(t, err)

	// Check if jobsCache has only 15 jobs and GPU ordinals are empty
	assert.Len(t, c.jobsCache, 15)

	for _, p := range c.jobsCache {
		assert.Empty(t, p.gpuOrdinals)
	}
}
