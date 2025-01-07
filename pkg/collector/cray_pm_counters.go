//go:build !nocraypmc
// +build !nocraypmc

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
)

const crayPMCCollectorSubsystem = "cray_pm_counters"

// Currently supported PM counters domains.
var (
	pmcDomainRegex = regexp.MustCompile("((?:cpu|memory|accel)[0-9]*?)*?_*?(energy|power|temp)_*?(cap)*?")
)

type crayPMCCollector struct {
	fs                   sysfs.FS
	logger               *slog.Logger
	hostname             string
	joulesMetricDesc     *prometheus.Desc
	wattsMetricDesc      *prometheus.Desc
	wattsLimitMetricDesc *prometheus.Desc
	tempMetricDesc       *prometheus.Desc
}

func init() {
	RegisterCollector(crayPMCCollectorSubsystem, defaultDisabled, NewCrayPMCCollector)
}

// NewCrayPMCCollector returns a new Collector exposing Cray's `pm_counters` metrics.
func NewCrayPMCCollector(logger *slog.Logger) (Collector, error) {
	fs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		return nil, err
	}

	joulesMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, crayPMCCollectorSubsystem, "energy_joules"),
		"Current energy value in joules",
		[]string{"hostname", "domain"}, nil,
	)

	wattsMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, crayPMCCollectorSubsystem, "power_watts"),
		"Current power value in watts",
		[]string{"hostname", "domain"}, nil,
	)

	wattsLimitMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, crayPMCCollectorSubsystem, "power_limit_watts"),
		"Current power limit value in watts",
		[]string{"hostname", "domain"}, nil,
	)

	tempMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, crayPMCCollectorSubsystem, "temp_celsius"),
		"Current temperature value in celsius",
		[]string{"hostname", "domain"}, nil,
	)

	collector := crayPMCCollector{
		fs:                   fs,
		logger:               logger,
		hostname:             hostname,
		joulesMetricDesc:     joulesMetricDesc,
		wattsMetricDesc:      wattsMetricDesc,
		wattsLimitMetricDesc: wattsLimitMetricDesc,
		tempMetricDesc:       tempMetricDesc,
	}

	return &collector, nil
}

// Update implements Collector and exposes `pm_counters` metrics.
func (c *crayPMCCollector) Update(ch chan<- prometheus.Metric) error {
	domains, err := GetCrayPMCDomains(c.fs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("Platform doesn't have cray/pm_counters files present", "err", err)

			return ErrNoData
		}

		if errors.Is(err, os.ErrPermission) {
			c.logger.Debug("Can't access /sys/cray/pm_counters files", "err", err)

			return ErrNoData
		}

		return fmt.Errorf("failed to fetch /sys/cray/pm_counters stats: %w", err)
	}

	// Update metrics
	for _, domain := range domains {
		if val, err := domain.GetEnergyJoules(); err == nil && val > 0 {
			ch <- prometheus.MustNewConstMetric(c.joulesMetricDesc, prometheus.GaugeValue, float64(val), c.hostname, domain.Name)
		}

		if val, err := domain.GetPowerWatts(); err == nil && val > 0 {
			ch <- prometheus.MustNewConstMetric(c.wattsMetricDesc, prometheus.GaugeValue, float64(val), c.hostname, domain.Name)
		}

		if val, err := domain.GetPowerLimitWatts(); err == nil && val > 0 {
			ch <- prometheus.MustNewConstMetric(c.wattsLimitMetricDesc, prometheus.GaugeValue, float64(val), c.hostname, domain.Name)
		}

		if val, err := domain.GetTempCelsius(); err == nil && val > 0 {
			ch <- prometheus.MustNewConstMetric(c.tempMetricDesc, prometheus.GaugeValue, float64(val), c.hostname, domain.Name)
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *crayPMCCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", crayPMCCollectorSubsystem)

	return nil
}

// PMCDomain stores the information for one Cray's domain PM counter.
type PMCDomain struct {
	Name string // name of PM counter domain zone from filename
	Path string // filesystem path of PM counters
}

// GetEnergyJoules returns the current joule value from the domain counter.
func (pd PMCDomain) GetEnergyJoules() (uint64, error) {
	return parsePMCounterValueFromFile(pd.Path + "energy")
}

// GetPowerWatts returns the current watt value from the domain counter.
func (pd PMCDomain) GetPowerWatts() (uint64, error) {
	return parsePMCounterValueFromFile(pd.Path + "power")
}

// GetPowerLimitWatts returns the current power limit watt value from the domain counter.
func (pd PMCDomain) GetPowerLimitWatts() (uint64, error) {
	return parsePMCounterValueFromFile(pd.Path + "power_cap")
}

// GetTempCelsius returns the current node temperature in C value from the domain counter.
func (pd PMCDomain) GetTempCelsius() (uint64, error) {
	return parsePMCounterValueFromFile(pd.Path + "temp")
}

// GetCrayPMCDomains returns a slice of Cray's `pm_counters` domains.
// - https://cray-hpe.github.io/docs-csm/en-10/operations/power_management/user_access_to_compute_node_power_data/
func GetCrayPMCDomains(fs sysfs.FS) ([]PMCDomain, error) {
	pmcDir := sysFilePath("cray/pm_counters")

	files, err := os.ReadDir(pmcDir)
	if err != nil {
		return nil, fmt.Errorf("unable to read /sys/cray/pm_counters: %w", err)
	}

	var domains []PMCDomain

	var domainNames []string

	// Loop through directory files searching for filename that matches domain regex.
	for _, f := range files {
		name := f.Name()
		for _, match := range pmcDomainRegex.FindAllStringSubmatch(name, -1) {
			if len(match) > 1 {
				domainName := match[1]
				pathSuffix := match[1] + "_"

				if domainName == "" {
					domainName = "node"
					pathSuffix = ""
				}

				if !slices.Contains(domainNames, domainName) {
					domain := PMCDomain{
						Name: domainName,
						Path: fmt.Sprintf("%s/%s", pmcDir, pathSuffix),
					}

					domains = append(domains, domain)
					domainNames = append(domainNames, domainName)
				}
			}
		}
	}

	return domains, nil
}

func parsePMCounterValueFromFile(p string) (uint64, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return 0, err
	}

	// Values are of format "100 W 1734094386462336 us" or "100 J 1734094386462336 us"
	// So split the string and take first part
	return strconv.ParseUint(strings.Split(strings.TrimSpace(string(data)), " ")[0], 10, 64)
}
