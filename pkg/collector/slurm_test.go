//go:build !noslurm
// +build !noslurm

package collector

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/go-kit/log"
)

var expectedSlurmMetrics CgroupMetric

func mockGPUDevices() map[int]Device {
	var devs = make(map[int]Device, 4)
	for i := 0; i <= 4; i++ {
		idxString := strconv.Itoa(i)
		devs[i] = Device{index: idxString, uuid: fmt.Sprintf("GPU-%d", i)}
	}
	return devs
}

func TestCgroupsV2SlurmJobMetrics(t *testing.T) {
	if _, err := BatchJobExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.create.unique.jobids",
			"--collector.slurm.job.props.path", "fixtures/slurmjobprops",
			"--collector.slurm.nvidia.gpu.job.map.path", "fixtures/gpujobmap",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{
		cgroups:          "v2",
		gpuDevs:          mockGPUDevices(),
		cgroupsRootPath:  *cgroupfsPath,
		slurmCgroupsPath: fmt.Sprintf("%s/system.slice/slurmstepd.scope", *cgroupfsPath),
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
		memswTotal:      1.8446744073709552e+19, // cgroupv2 just returns math.MaxUint64
		memswFailCount:  0,
		memoryPressure:  0,
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "ac28caf5-ce6c-35f6-73fb-47d9d43f7780",
		jobGpuOrdinals:  []string{"2", "3"},
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false,
	}
	if err != nil {
		t.Fatalf("Cannot fetch data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics["1009248"])
	}
}

func TestCgroupsV2SlurmJobMetricsWithProcFs(t *testing.T) {
	if _, err := BatchJobExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.create.unique.jobids",
			"--path.procfs", "fixtures/proc",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{
		cgroups:          "v2",
		cgroupsRootPath:  *cgroupfsPath,
		gpuDevs:          mockGPUDevices(),
		slurmCgroupsPath: fmt.Sprintf("%s/system.slice/slurmstepd.scope", *cgroupfsPath),
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
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "ac28caf5-ce6c-35f6-73fb-47d9d43f7780",
		jobGpuOrdinals:  []string{"2", "3"},
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false,
	}
	if err != nil {
		t.Fatalf("Cannot fetch data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics["1009248"])
	}
}

func TestCgroupsV2SlurmJobMetricsNoJobProps(t *testing.T) {
	if _, err := BatchJobExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.create.unique.jobids",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{
		cgroups:          "v2",
		cgroupsRootPath:  *cgroupfsPath,
		gpuDevs:          mockGPUDevices(),
		slurmCgroupsPath: fmt.Sprintf("%s/system.slice/slurmstepd.scope", *cgroupfsPath),
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
		userslice:       false,
		jobuid:          "",
		jobaccount:      "",
		jobid:           "1009248",
		jobuuid:         "a0523e93-a037-c2b1-8b34-410c9996399c",
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false,
	}
	if err != nil {
		t.Fatalf("Cannot fetch data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics["1009248"])
	}
}

func TestCgroupsV1SlurmJobMetrics(t *testing.T) {
	if _, err := BatchJobExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--path.procfs", "fixtures/proc",
			"--collector.slurm.create.unique.jobids",
			"--collector.slurm.job.props.path", "fixtures/slurmjobprops",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{
		cgroups:          "v1",
		logger:           log.NewNopLogger(),
		gpuDevs:          mockGPUDevices(),
		cgroupsRootPath:  fmt.Sprintf("%s/cpuacct", *cgroupfsPath),
		slurmCgroupsPath: fmt.Sprintf("%s/cpuacct/slurm", *cgroupfsPath),
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
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "ac28caf5-ce6c-35f6-73fb-47d9d43f7780",
		jobGpuOrdinals:  []string{"2", "3"},
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false,
	}
	if err != nil {
		t.Fatalf("Cannot fetch data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics["1009248"])
	}
}
