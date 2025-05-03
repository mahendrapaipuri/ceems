//go:build !nocpu
// +build !nocpu

package collector

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestCPUCollector(s procfs.CPUStat) *cpuCollector {
	dupStat := s

	return &cpuCollector{
		logger:   noOpLogger,
		cpuStats: dupStat,
	}
}

func TestCPUCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc", "--collector.empty-hostname-label",
	})
	require.NoError(t, err)

	collector, err := NewCPUCollector(noOpLogger)
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

func TestCPU(t *testing.T) {
	firstCPUStat := procfs.CPUStat{
		User:      100.0,
		Nice:      100.0,
		System:    100.0,
		Idle:      100.0,
		Iowait:    100.0,
		IRQ:       100.0,
		SoftIRQ:   100.0,
		Steal:     100.0,
		Guest:     100.0,
		GuestNice: 100.0,
	}

	c := makeTestCPUCollector(firstCPUStat)
	want := procfs.CPUStat{
		User:      101.0,
		Nice:      101.0,
		System:    101.0,
		Idle:      101.0,
		Iowait:    101.0,
		IRQ:       101.0,
		SoftIRQ:   101.0,
		Steal:     101.0,
		Guest:     101.0,
		GuestNice: 101.0,
	}
	c.updateCPUStats(want)
	got := c.cpuStats
	assert.Equal(t, want, got)

	c = makeTestCPUCollector(firstCPUStat)
	jumpBack := procfs.CPUStat{
		User:      99.9,
		Nice:      99.9,
		System:    99.9,
		Idle:      99.9,
		Iowait:    99.9,
		IRQ:       99.9,
		SoftIRQ:   99.9,
		Steal:     99.9,
		Guest:     99.9,
		GuestNice: 99.9,
	}
	c.updateCPUStats(jumpBack)
	got = c.cpuStats
	assert.NotEqual(t, jumpBack, got)

	c = makeTestCPUCollector(firstCPUStat)
	resetIdle := procfs.CPUStat{
		User:      102.0,
		Nice:      102.0,
		System:    102.0,
		Idle:      1.0,
		Iowait:    102.0,
		IRQ:       102.0,
		SoftIRQ:   102.0,
		Steal:     102.0,
		Guest:     102.0,
		GuestNice: 102.0,
	}
	c.updateCPUStats(resetIdle)
	got = c.cpuStats
	assert.Equal(t, resetIdle, got)
}
