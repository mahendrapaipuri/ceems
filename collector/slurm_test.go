//go:build !noslurm
// +build !noslurm

package collector

import (
	"reflect"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
)

var expectedSlurmMetrics = make(map[string]CgroupMetric)

func TestSlurmJobMetrics(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse([]string{"--path.cgroupfs", "fixtures/sys/fs/cgroup"}); err != nil {
		t.Fatal(err)
	}
	c := slurmCollector{cgroupV2: true, logger: log.NewNopLogger()}
	metrics, err := c.getJobsMetrics()
	expectedSlurmMetrics["/system.slice/slurmstepd.scope/job_1009248"] = CgroupMetric{
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
		uid:             -1,
		jobid:           "1009248",
		batch:           "slurm",
		err:             false}
	if err != nil {
		t.Fatalf("Cannot retrieve data from getJobsMetrics function: %v ", err)
	}
	if !reflect.DeepEqual(metrics, expectedSlurmMetrics) {
		t.Fatalf("Expected metrics data is %+v: \nGot %+v", expectedSlurmMetrics, metrics)
	}
}
