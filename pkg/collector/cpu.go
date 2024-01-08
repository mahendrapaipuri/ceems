//go:build !nocpu
// +build !nocpu

package collector

import (
	"fmt"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

type cpuCollector struct {
	fs            procfs.FS
	cpu           *prometheus.Desc
	logger        log.Logger
	cpuStats      procfs.CPUStat
	cpuStatsMutex sync.Mutex
	hostname      string
}

// Idle jump back limit in seconds.
const jumpBackSeconds = 3.0

const cpuCollectorSubsystem = "cpu"

var (
	jumpBackDebugMessage = fmt.Sprintf("CPU Idle counter jumped backwards more than %f seconds, possible hotplug event, resetting CPU stats", jumpBackSeconds)
)

func init() {
	RegisterCollector(cpuCollectorSubsystem, defaultEnabled, NewCPUCollector)
}

// NewCPUCollector returns a new Collector exposing kernel/system statistics.
func NewCPUCollector(logger log.Logger) (Collector, error) {
	fs, err := procfs.NewFS(*procfsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open procfs: %w", err)
	}

	return &cpuCollector{
		fs: fs,
		cpu: prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, cpuCollectorSubsystem, "seconds_total"),
			"Seconds the CPUs spent in each mode.",
			[]string{"hostname", "mode"}, nil,
		),
		logger:   logger,
		hostname: hostname,
		cpuStats: procfs.CPUStat{},
	}, nil
}

// Update reads /proc/stat through procfs and exports CPU-related metrics.
func (c *cpuCollector) Update(ch chan<- prometheus.Metric) error {
	stats, err := c.fs.Stat()
	if err != nil {
		return err
	}

	c.updateCPUStats(stats.CPUTotal)

	// Acquire a lock to read the stats.
	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.User, c.hostname, "user")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.Nice, c.hostname, "nice")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.System, c.hostname, "system")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.Idle, c.hostname, "idle")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.Iowait, c.hostname, "iowait")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.IRQ, c.hostname, "irq")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.SoftIRQ, c.hostname, "softirq")
	ch <- prometheus.MustNewConstMetric(c.cpu, prometheus.CounterValue, c.cpuStats.Steal, c.hostname, "steal")
	return nil
}

// updateCPUStats updates the internal cache of CPU stats.
func (c *cpuCollector) updateCPUStats(newStats procfs.CPUStat) {

	// Acquire a lock to update the stats.
	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()

	cpuStats := c.cpuStats

	// If idle jumps backwards by more than X seconds, assume we had a hotplug event and reset the stats for this CPU.
	if (cpuStats.Idle - newStats.Idle) >= jumpBackSeconds {
		level.Debug(c.logger).Log("msg", jumpBackDebugMessage, "old_value", c.cpuStats.Idle, "new_value", newStats.Idle)
		cpuStats = procfs.CPUStat{}
	}

	if newStats.Idle >= cpuStats.Idle {
		cpuStats.Idle = newStats.Idle
	} else {
		level.Debug(c.logger).Log("msg", "CPU Idle counter jumped backwards", "old_value", c.cpuStats.Idle, "new_value", newStats.Idle)
	}

	if newStats.User >= cpuStats.User {
		cpuStats.User = newStats.User
	} else {
		level.Debug(c.logger).Log("msg", "CPU User counter jumped backwards", "old_value", c.cpuStats.User, "new_value", newStats.User)
	}

	if newStats.Nice >= cpuStats.Nice {
		cpuStats.Nice = newStats.Nice
	} else {
		level.Debug(c.logger).Log("msg", "CPU Nice counter jumped backwards", "old_value", c.cpuStats.Nice, "new_value", newStats.Nice)
	}

	if newStats.System >= cpuStats.System {
		cpuStats.System = newStats.System
	} else {
		level.Debug(c.logger).Log("msg", "CPU System counter jumped backwards", "old_value", c.cpuStats.System, "new_value", newStats.System)
	}

	if newStats.Iowait >= cpuStats.Iowait {
		cpuStats.Iowait = newStats.Iowait
	} else {
		level.Debug(c.logger).Log("msg", "CPU Iowait counter jumped backwards", "old_value", c.cpuStats.Iowait, "new_value", newStats.Iowait)
	}

	if newStats.IRQ >= cpuStats.IRQ {
		cpuStats.IRQ = newStats.IRQ
	} else {
		level.Debug(c.logger).Log("msg", "CPU IRQ counter jumped backwards", "old_value", c.cpuStats.IRQ, "new_value", newStats.IRQ)
	}

	if newStats.SoftIRQ >= cpuStats.SoftIRQ {
		cpuStats.SoftIRQ = newStats.SoftIRQ
	} else {
		level.Debug(c.logger).Log("msg", "CPU SoftIRQ counter jumped backwards", "old_value", c.cpuStats.SoftIRQ, "new_value", newStats.SoftIRQ)
	}

	if newStats.Steal >= cpuStats.Steal {
		cpuStats.Steal = newStats.Steal
	} else {
		level.Debug(c.logger).Log("msg", "CPU Steal counter jumped backwards", "old_value", c.cpuStats.Steal, "new_value", newStats.Steal)
	}

	if newStats.Guest >= cpuStats.Guest {
		cpuStats.Guest = newStats.Guest
	} else {
		level.Debug(c.logger).Log("msg", "CPU Guest counter jumped backwards", "old_value", c.cpuStats.Guest, "new_value", newStats.Guest)
	}

	if newStats.GuestNice >= cpuStats.GuestNice {
		cpuStats.GuestNice = newStats.GuestNice
	} else {
		level.Debug(c.logger).Log("msg", "CPU GuestNice counter jumped backwards", "old_value", c.cpuStats.GuestNice, "new_value", newStats.GuestNice)
	}

	c.cpuStats = cpuStats
}
