//go:build !noslurm
// +build !noslurm

package collector

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/go-kit/log"
)

var expectedSlurmMetrics CgroupMetric

func TestCgroupsV2SlurmJobMetrics(t *testing.T) {
	if _, err := BatchJobExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.create.unique.jobids",
			"--collector.slurm.job.props.path", "fixtures/slurmjobprops",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{
		cgroups:          "v2",
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
		memoryRSS:       4.098592768e+09,
		memoryCache:     0,
		memoryUsed:      4.111491072e+09,
		memoryTotal:     4.294967296e+09,
		memoryFailCount: 0,
		memswUsed:       0,
		memswTotal:      -1,
		memswFailCount:  0,
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "ac28caf5-ce6c-35f6-73fb-47d9d43f7780",
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false}
	if err != nil {
		t.Fatalf("Cannot retrieve data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics)
	}
}

func TestCgroupsV2WithProcFsSlurmJobMetrics(t *testing.T) {
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
		memoryRSS:       4.098592768e+09,
		memoryCache:     0,
		memoryUsed:      4.111491072e+09,
		memoryTotal:     4.294967296e+09,
		memoryFailCount: 0,
		memswUsed:       0,
		memswTotal:      -1,
		memswFailCount:  0,
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "ac28caf5-ce6c-35f6-73fb-47d9d43f7780",
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false}
	if err != nil {
		t.Fatalf("Cannot retrieve data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics)
	}
}

func TestCgroupsV1SlurmJobMetrics(t *testing.T) {
	if _, err := BatchJobExporterApp.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.create.unique.jobids",
			"--collector.slurm.job.props.path", "fixtures/slurmjobprops",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{
		cgroups:          "v1",
		logger:           log.NewNopLogger(),
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
		memoryRSS:       1.0407936e+07,
		memoryCache:     2.1086208e+07,
		memoryUsed:      4.0194048e+07,
		memoryTotal:     2.01362030592e+11,
		memoryFailCount: 0,
		memswUsed:       4.032512e+07,
		memswTotal:      9.223372036854772e+18,
		memswFailCount:  0,
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "ac28caf5-ce6c-35f6-73fb-47d9d43f7780",
		step:            "",
		task:            "",
		batch:           "slurm",
		err:             false}
	if err != nil {
		t.Fatalf("Cannot retrieve data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics["1009248"], expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics)
	}
}
