// Taken from node_exporter/collector/rapl_linux.go

//go:build !norapl
// +build !norapl

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
)

const raplCollectorSubsystem = "rapl"

type raplCollector struct {
	fs               sysfs.FS
	logger           *slog.Logger
	hostname         string
	securityContexts map[string]*security.SecurityContext
	joulesMetricDesc *prometheus.Desc
	wattsMetricDesc  *prometheus.Desc
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
func NewRaplCollector(logger *slog.Logger) (Collector, error) {
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

			reqCaps, err := setupCollectorCaps(capabilities)
			if err != nil {
				logger.Warn("Failed to parse capability name(s)", "err", err)
			}

			securityContexts[raplReadEnergyCounter], err = security.NewSecurityContext(
				raplReadEnergyCounter,
				reqCaps,
				readCounters,
				logger,
			)
			if err != nil {
				logger.Error("Failed to create a security context for reading rapl counters", "err", err)

				return nil, err
			}
		}
	}

	joulesMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, raplCollectorSubsystem, "joules_total"),
		"Current RAPL value in joules",
		[]string{"hostname", "index", "path", "rapl_zone"}, nil,
	)

	wattsMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, raplCollectorSubsystem, "power_limit_watts_total"),
		"Current RAPL power Limit value in watts",
		[]string{"hostname", "index", "path", "rapl_zone"}, nil,
	)

	collector := raplCollector{
		fs:               fs,
		logger:           logger,
		hostname:         hostname,
		securityContexts: securityContexts,
		joulesMetricDesc: joulesMetricDesc,
		wattsMetricDesc:  wattsMetricDesc,
	}

	return &collector, nil
}

// Update implements Collector and exposes RAPL related metrics.
func (c *raplCollector) Update(ch chan<- prometheus.Metric) error {
	// nil zones are fine when platform doesn't have powercap files present.
	zones, err := sysfs.GetRaplZones(c.fs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Platform doesn't have powercap files present", "err", err)

			return ErrNoData
		}

		if errors.Is(err, os.ErrPermission) {
			c.logger.Debug("Can't access powercap files", "err", err)

			return ErrNoData
		}

		return fmt.Errorf("failed to fetch rapl stats: %w", err)
	}

	// Start wait group
	wg := sync.WaitGroup{}

	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := c.updateLimits(zones, ch); err != nil {
			c.logger.Error("Failed to update RAPL power limits", "err", err)
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := c.updateEnergy(zones, ch); err != nil {
			c.logger.Error("Failed to update RAPL energy counters", "err", err)
		}
	}()

	// Wait for go routines
	wg.Wait()

	return nil
}

// Stop releases system resources used by the collector.
func (c *raplCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", raplCollectorSubsystem)

	return nil
}

func (c *raplCollector) updateLimits(zones []sysfs.RaplZone, ch chan<- prometheus.Metric) error {
	// Get current limits
	powerLimits, err := readPowerLimits(zones)
	if err != nil {
		return err
	}

	// Update metrics
	for rz, microWatts := range powerLimits {
		joules := float64(microWatts) / 1000000.0

		if *raplZoneLabel {
			ch <- c.wattsMetricWithZoneLabel(rz, joules)
		} else {
			ch <- c.wattsMetric(rz, joules)
		}
	}

	return nil
}

func (c *raplCollector) updateEnergy(zones []sysfs.RaplZone, ch chan<- prometheus.Metric) error {
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

func (c *raplCollector) wattsMetric(z sysfs.RaplZone, v float64) prometheus.Metric {
	index := strconv.Itoa(z.Index)
	descriptor := prometheus.NewDesc(
		prometheus.BuildFQName(
			Namespace,
			raplCollectorSubsystem,
			SanitizeMetricName(z.Name)+"_power_limit_watts_total",
		),
		fmt.Sprintf("Current RAPL %s power limit in watts", z.Name),
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

func (c *raplCollector) wattsMetricWithZoneLabel(z sysfs.RaplZone, v float64) prometheus.Metric {
	index := strconv.Itoa(z.Index)

	return prometheus.MustNewConstMetric(
		c.wattsMetricDesc,
		prometheus.CounterValue,
		v,
		c.hostname,
		index,
		z.Path,
		z.Name,
	)
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

// powerLimits gets power limit for each zone. We are interested in long term limit.
// According to powecap docs, only files power_limit_uw and time_window_us are
// guaranteed to exist. So, we should rely only on them
// Ref: https://www.kernel.org/doc/html/next/power/powercap/powercap.html
func readPowerLimits(zones []sysfs.RaplZone) (map[sysfs.RaplZone]uint64, error) {
	powerLimits := make(map[sysfs.RaplZone]uint64)

	for _, rz := range zones {
		var timeWindow uint64

		var longtermConstraint int

		for c := range 2 {
			timeWindowFile := filepath.Join(rz.Path, fmt.Sprintf("constraint_%d_time_window_us", c))
			if _, err := os.Stat(timeWindowFile); err != nil {
				continue
			}

			// Read time window in micro seconds
			if constTimeWindow, err := readUintFromFile(timeWindowFile); err == nil {
				if constTimeWindow > timeWindow {
					timeWindow = constTimeWindow
					longtermConstraint = c
				}
			}
		}

		// Now read power_limit_uw for the selected constraint. Value is in micro watts.
		powerLimitFile := filepath.Join(rz.Path, fmt.Sprintf("constraint_%d_power_limit_uw", longtermConstraint))
		if powerLimit, err := readUintFromFile(powerLimitFile); err == nil {
			powerLimits[rz] = powerLimit
		}
	}

	if len(powerLimits) == 0 {
		return nil, errors.New("no RAPL power limits found")
	}

	return powerLimits, nil
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
