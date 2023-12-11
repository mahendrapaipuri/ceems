//go:build !noslurm
// +build !noslurm

package collector

import (
	"reflect"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
)

var expectedSlurmMetrics CgroupMetric

func TestCgroupsV2SlurmJobMetrics(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.job.stat.path", "fixtures/slurmjobstat",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{cgroupV2: true, logger: log.NewNopLogger()}
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
		memswTotal:      0,
		memswFailCount:  0,
		userslice:       false,
		jobuid:          "1000",
		jobaccount:      "testacc",
		jobid:           "1009248",
		jobuuid:         "7f6c39b7-2740-fc1f-32c2-8fc28880829c",
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
	if _, err := kingpin.CommandLine.Parse(
		[]string{
			"--path.cgroupfs", "fixtures/sys/fs/cgroup",
			"--collector.slurm.job.stat.path", "fixtures/slurmjobstat",
		},
	); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{cgroupV2: false, logger: log.NewNopLogger()}
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
		jobuuid:         "7f6c39b7-2740-fc1f-32c2-8fc28880829c",
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
