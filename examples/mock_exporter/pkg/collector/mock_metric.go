package collector

import (
	"math/rand"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/collector"
	"github.com/prometheus/client_golang/prometheus"
)

// A mock collector that exports a random value between 0 and configurable maximum value
const mockCollectorSubsystem = "mock"

// Define mock collector struct
type mockCollector struct {
	logger         log.Logger
	mockMetricDesc *prometheus.Desc
}

// Define vars and flags necessary to configure collector
var (
	maxRandInt = collector.BatchJobExporterApp.Flag(
		"collector.mock.max",
		"Maximum value of the mock metric.",
	).Default("100").Int()
)

// Register mock collector
func init() {
	collector.RegisterCollector(mockCollectorSubsystem, true, NewMockCollector)
}

// NewMockCollector returns a new Collector exposing mock metrics.
func NewMockCollector(logger log.Logger) (collector.Collector, error) {
	// Define mock metric description
	mockMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(collector.Namespace, mockCollectorSubsystem, "mockunit_total"),
		"Current mock metric", []string{}, nil,
	)

	// Create a new mockCollector struct
	collector := mockCollector{
		logger:         logger,
		mockMetricDesc: mockMetricDesc,
	}
	return &collector, nil
}

// Update implements Collector and exposes mock metrics.
func (c *mockCollector) Update(ch chan<- prometheus.Metric) error {
	// Return a random value
	ch <- prometheus.MustNewConstMetric(c.mockMetricDesc, prometheus.CounterValue, float64(rand.Intn(*maxRandInt)))
	return nil
}
