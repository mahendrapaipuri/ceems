//go:build !noemissions
// +build !noemissions

package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/emissions"
	"github.com/prometheus/client_golang/prometheus"
)

const emissionsCollectorSubsystem = "emissions"

// CLI opts.
var (
	emissionProviders = CEEMSExporterApp.Flag(
		"collector.emissions.provider",
		`Exports emission factors from these providers (default: all).
Supported providers:
	- "owid": Our World In Data (https://ourworldindata.org/grapher/carbon-intensity-electricity?tab=table)
	- "emaps": Electricity Maps (https://app.electricitymaps.com/)
	- "rte": RTE eCO2 Mix (Only for France) (https://www.rte-france.com/en/eco2mix/co2-emissions)
	- "wt": Watt Time (https://docs.watttime.org/#tag/Introduction)`,
	).Enums("owid", "emaps", "rte", "wt")
)

type emissionsCollector struct {
	logger                   *slog.Logger
	emissionFactorProviders  *emissions.FactorProviders
	emissionFactorMetricDesc *prometheus.Desc
	prevReadTime             int64
	prevEmissionFactors      map[string]float64
}

func init() {
	RegisterCollector(emissionsCollectorSubsystem, defaultDisabled, NewEmissionsCollector)
}

// NewEmissionsCollector returns a new Collector exposing emission factor metrics.
func NewEmissionsCollector(logger *slog.Logger) (Collector, error) {
	// Create metric description
	emissionsMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, emissionsCollectorSubsystem, "gCo2_kWh"),
		"Current emission factor in CO2eq grams per kWh",
		[]string{"provider", "provider_name", "country_code", "country"}, nil,
	)

	// Create a new instance of EmissionCollector
	emissionFactorProviders, err := emissions.NewFactorProviders(logger, *emissionProviders)
	if err != nil {
		logger.Error("Failed to create new EmissionCollector", "err", err)

		return nil, err
	}

	return &emissionsCollector{
		logger:                   logger,
		emissionFactorProviders:  emissionFactorProviders,
		emissionFactorMetricDesc: emissionsMetricDesc,
		prevReadTime:             time.Now().Unix(),
		prevEmissionFactors:      make(map[string]float64),
	}, nil
}

// Update implements Collector and exposes emission factor.
func (c *emissionsCollector) Update(ch chan<- prometheus.Metric) error {
	currentEmissionFactors := c.emissionFactorProviders.Collect()
	// Returned value negative == emissions factor is not avail
	for provider, payload := range currentEmissionFactors {
		if payload.Factor != nil {
			for code, factor := range payload.Factor {
				if factor.Factor > 0 {
					ch <- prometheus.MustNewConstMetric(c.emissionFactorMetricDesc, prometheus.GaugeValue, float64(factor.Factor), provider, payload.Name, code, factor.Name)
				}
			}
		}
	}

	return nil
}

// Stops collector and releases system resources.
func (c *emissionsCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", emissionsCollectorSubsystem)

	// Stop all providers to release any system resources that are being used
	if err := c.emissionFactorProviders.Stop(); err != nil {
		c.logger.Error("Failed to stop emission factor providers", "err", err)

		return err
	}

	return nil
}
