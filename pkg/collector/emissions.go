//go:build !noemissions
// +build !noemissions

package collector

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/emissions"
)

const emissionsCollectorSubsystem = "emissions"

type emissionsCollector struct {
	logger                   log.Logger
	ctx                      context.Context
	emissionFactorProviders  emissions.FactorProviders
	emissionFactorMetricDesc *prometheus.Desc
	prevReadTime             int64
	prevEmissionFactors      map[string]float64
}

var (
	emissionsLock     = sync.RWMutex{}
	countryCodeAlpha3 string
	countryCodeAlpha2 = BatchJobExporterApp.Flag(
		"collector.emissions.country.code",
		"ISO 3166-1 alpha-2 Country code.",
	).Default("FR").String()
	countryCodesMap    = emissions.CountryCodes.IsoCode
	newFactorProviders = emissions.NewFactorProviders
)

func init() {
	RegisterCollector(emissionsCollectorSubsystem, defaultDisabled, NewEmissionsCollector)
}

// Get ISO3 code from ISO2 country code
func convertISO2ToISO3(countryCodeISO2 string) string {
	for _, country := range countryCodesMap {
		if country.Alpha2Code == *countryCodeAlpha2 {
			return country.Alpha3Code
		}
	}
	return ""
}

// NewEmissionsCollector returns a new Collector exposing emission factor metrics.
func NewEmissionsCollector(logger log.Logger) (Collector, error) {
	// Ensure country code is in upper case
	*countryCodeAlpha2 = strings.ToUpper(*countryCodeAlpha2)

	// Set up context values
	contextValues := emissions.ContextValues{
		CountryCodeAlpha2: *countryCodeAlpha2,
		CountryCodeAlpha3: convertISO2ToISO3(*countryCodeAlpha2),
	}

	// Add contextValues to current context
	ctx := context.WithValue(context.Background(), emissions.ContextKey{}, contextValues)

	// Create metric description
	emissionsMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, emissionsCollectorSubsystem, "gCo2_kWh"),
		"Current emission factor in CO2eq grams per kWh", 
		[]string{"provider", "provider_name", "country"}, nil,
	)

	// Create a new instance of EmissionCollector
	emissionFactorProviders, err := newFactorProviders(ctx, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create new EmissionCollector", "err", err)
		return nil, err
	}

	return &emissionsCollector{
		logger:                   logger,
		ctx:                      ctx,
		emissionFactorProviders:  *emissionFactorProviders,
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
		if payload.Factor > -1 {
			ch <- prometheus.MustNewConstMetric(c.emissionFactorMetricDesc, prometheus.GaugeValue, float64(payload.Factor), provider, payload.Name, *countryCodeAlpha2)
		}
	}
	return nil
}
