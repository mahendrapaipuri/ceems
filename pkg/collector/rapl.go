// Taken from node_exporter/collector/rapl_linux.go

//go:build !norapl
// +build !norapl

package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
)

const raplCollectorSubsystem = "rapl"

type raplCollector struct {
	fs               sysfs.FS
	logger           log.Logger
	hostname         string
	securityContexts map[string]*security.SecurityContext
	joulesMetricDesc *prometheus.Desc
}

// Security context names.
const (
	raplReadEnergyCounter = "rapl_read_energy_counters"
)

type raplCountersSecurityCtxData struct {
	zones    []sysfs.RaplZone
	counters map[sysfs.RaplZone]uint64
}

func init() {
	RegisterCollector(raplCollectorSubsystem, defaultEnabled, NewRaplCollector)
}

var raplZoneLabel = CEEMSExporterApp.Flag(
	"collector.rapl.enable-zone-label",
	"Enables RAPL zone labels (default: disabled)",
).Default("false").Bool()

// NewRaplCollector returns a new Collector exposing RAPL metrics.
func NewRaplCollector(logger log.Logger) (Collector, error) {
	fs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		return nil, err
	}

	// Get kernel version
	securityContexts := make(map[string]*security.SecurityContext)

	if currentKernelVer, err := KernelVersion(); err == nil {
		// Startin from kernel 5.10, RAPL counters are read only by root.
		// So we need CAP_DAC_READ_SEARCH capability to read them.
		if currentKernelVer >= KernelStringToNumeric("5.10") {
			// Setup necessary capabilities. cap_perfmon is necessary to open perf events.
			capabilities := []string{"cap_dac_read_search"}
			reqCaps := setupCollectorCaps(logger, raplCollectorSubsystem, capabilities)

			securityContexts[raplReadEnergyCounter], err = security.NewSecurityContext(raplReadEnergyCounter, reqCaps, readCounters, logger)
			if err != nil {
				level.Error(logger).Log("msg", "Failed to create a security context for reading rapl counters", "err", err)

				return nil, err
			}
		}
	}

	joulesMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, raplCollectorSubsystem, "joules_total"),
		"Current RAPL value in joules",
		[]string{"hostname", "index", "path", "rapl_zone"}, nil,
	)

	collector := raplCollector{
		fs:               fs,
		logger:           logger,
		hostname:         hostname,
		securityContexts: securityContexts,
		joulesMetricDesc: joulesMetricDesc,
	}

	return &collector, nil
}

// Update implements Collector and exposes RAPL related metrics.
func (c *raplCollector) Update(ch chan<- prometheus.Metric) error {
	// nil zones are fine when platform doesn't have powercap files present.
	zones, err := sysfs.GetRaplZones(c.fs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			level.Debug(c.logger).
				Log("msg", "Platform doesn't have powercap files present", "err", err)

			return ErrNoData
		}

		if errors.Is(err, os.ErrPermission) {
			level.Debug(c.logger).Log("msg", "Can't access powercap files", "err", err)

			return ErrNoData
		}

		return fmt.Errorf("failed to fetch rapl stats: %w", err)
	}

	// Data for security context
	dataPtr := &raplCountersSecurityCtxData{
		zones:    zones,
		counters: make(map[sysfs.RaplZone]uint64),
	}

	if len(c.securityContexts) > 0 {
		// Start new profilers within security context
		if securityCtx, ok := c.securityContexts[raplReadEnergyCounter]; ok {
			if err := securityCtx.Exec(dataPtr); err != nil {
				return err
			}
		}
	} else {
		if err := readCounters(dataPtr); err != nil {
			return err
		}
	}

	// If counters map is empty return no data
	if len(dataPtr.counters) == 0 {
		return ErrNoData
	}

	for rz, microJoules := range dataPtr.counters {
		joules := float64(microJoules) / 1000000.0

		if *raplZoneLabel {
			ch <- c.joulesMetricWithZoneLabel(rz, joules)
		} else {
			ch <- c.joulesMetric(rz, joules)
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *raplCollector) Stop(_ context.Context) error {
	level.Debug(c.logger).Log("msg", "Stopping", "collector", raplCollectorSubsystem)

	return nil
}

func (c *raplCollector) joulesMetric(z sysfs.RaplZone, v float64) prometheus.Metric {
	index := strconv.Itoa(z.Index)
	descriptor := prometheus.NewDesc(
		prometheus.BuildFQName(
			Namespace,
			raplCollectorSubsystem,
			SanitizeMetricName(z.Name)+"_joules_total",
		),
		fmt.Sprintf("Current RAPL %s value in joules", z.Name),
		[]string{"hostname", "index", "path"}, nil,
	)

	return prometheus.MustNewConstMetric(
		descriptor,
		prometheus.CounterValue,
		v,
		c.hostname,
		index,
		z.Path,
	)
}

func (c *raplCollector) joulesMetricWithZoneLabel(z sysfs.RaplZone, v float64) prometheus.Metric {
	index := strconv.Itoa(z.Index)

	return prometheus.MustNewConstMetric(
		c.joulesMetricDesc,
		prometheus.CounterValue,
		v,
		c.hostname,
		index,
		z.Path,
		z.Name,
	)
}

// readCounters reads the RAPL counters of different zones inside a security context.
func readCounters(data interface{}) error {
	// Assert data
	var d *raplCountersSecurityCtxData

	var ok bool
	if d, ok = data.(*raplCountersSecurityCtxData); !ok {
		return security.ErrSecurityCtxDataAssertion
	}

	for _, rz := range d.zones {
		microJoules, err := rz.GetEnergyMicrojoules()
		if err != nil {
			continue
		}

		d.counters[rz] = microJoules
	}

	return nil
}
