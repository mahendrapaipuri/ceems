//go:build !noredfish
// +build !noredfish

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

const redfishCollectorSubsystem = "redfish"

const realIPHeaderName = "X-Real-IP"

type redfishConfig struct {
	Web struct {
		URL           string `yaml:"url"`
		Username      string `yaml:"username"`
		Password      string `yaml:"password"`
		SkipTLSVerify bool   `yaml:"insecure_skip_verify"`
		SessionToken  bool   `yaml:"use_session_token"`
	} `yaml:"redfish_web_config"`
}

type redfishCollector struct {
	logger      *slog.Logger
	hostname    string
	client      *gofish.APIClient
	chassis     []*redfish.Chassis
	cachedPower map[string]*redfish.Power
	metricDesc  map[string]*prometheus.Desc
}

var (
	redfishConfigFile = CEEMSExporterApp.Flag(
		"collector.redfish.web-config",
		"Path to Redfish web configuration file.",
	).Default("").String()
	ipAddrHeader = CEEMSExporterApp.Flag(
		"collector.redfish.send-real-ip-header",
		"Include X-Real-IP header in requests. (default: false)",
	).Default("false").Bool()
	testIPAddr = CEEMSExporterApp.Flag(
		"collector.redfish.local-ip-address",
		"IP address to include for X-Real-IP header in tests.",
	).Default("").Hidden().String()
)

func init() {
	RegisterCollector(redfishCollectorSubsystem, defaultDisabled, NewRedfishCollector)
}

// NewRedfishCollector returns a new Collector to fetch power usage from redfish API.
func NewRedfishCollector(logger *slog.Logger) (Collector, error) {
	// Get absolute config file path
	configFilePath, err := filepath.Abs(*redfishConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of the config file: %w", err)
	}

	// Get all local IPs when X-Real-IP header must be included
	var realIPHeaderValue config.Header

	if *ipAddrHeader {
		if ipAddrs, err := lookupIPs(); err != nil {
			logger.Warn("Failed to find IP addresses for the current host", "err", err)
		} else {
			if *testIPAddr != "" {
				realIPHeaderValue = config.Header{
					Values: []string{*testIPAddr},
				}
			} else {
				realIPHeaderValue = config.Header{
					Values: ipAddrs,
				}
			}
		}
	}

	// Make config from file
	cfg, err := common.MakeConfig[redfishConfig](configFilePath)
	if err != nil {
		logger.Error("Failed to parse Redfish config file", "err", err)

		return nil, fmt.Errorf("failed to parse Redfish config file: %w", err)
	}

	// Make a new HTTP client config
	clientConfig := config.HTTPClientConfig{
		TLSConfig: config.TLSConfig{
			InsecureSkipVerify: cfg.Web.SkipTLSVerify,
		},
		HTTPHeaders: &config.Headers{
			Headers: map[string]config.Header{
				realIPHeaderName: realIPHeaderValue,
			},
		},
	}

	// Make a HTTP client from client config
	httpClient, err := config.NewClientFromConfig(clientConfig, "redfish")
	if err != nil {
		logger.Error("Failed to create a HTTP client for Redfish", "err", err)

		return nil, fmt.Errorf("failed to create a HTTP client for Redfish: %w", err)
	}

	// Create a redfish client
	config := gofish.ClientConfig{
		Endpoint:         cfg.Web.URL,
		Username:         cfg.Web.Username,
		Password:         cfg.Web.Password,
		Insecure:         cfg.Web.SkipTLSVerify,
		BasicAuth:        !cfg.Web.SessionToken,
		HTTPClient:       httpClient,
		ReuseConnections: true,
	}

	redfishClient, err := gofish.Connect(config)
	if err != nil {
		logger.Error("Failed to create Redfish client", "err", err)

		return nil, fmt.Errorf("failed to create a Redfish client: %w", err)
	}

	// Get all available chassis
	chassis, err := redfishClient.Service.Chassis()
	if err != nil {
		logger.Error("Failed to fetch chassis information from Redfish", "err", err)

		return nil, fmt.Errorf("failed to fetch available chassis from Redfish: %w", err)
	}

	// Initialize metricDesc map
	metricDesc := make(map[string]*prometheus.Desc, 4)

	metricDesc["current"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "current_watts"),
		"Current Power consumption in watts", []string{"hostname", "chassis"}, nil,
	)
	metricDesc["min"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "min_watts"),
		"Minimum Power consumption in watts", []string{"hostname", "chassis"}, nil,
	)
	metricDesc["max"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "max_watts"),
		"Maximum Power consumption in watts", []string{"hostname", "chassis"}, nil,
	)
	metricDesc["avg"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "avg_watts"),
		"Average Power consumption in watts", []string{"hostname", "chassis"}, nil,
	)

	collector := redfishCollector{
		logger:      logger,
		hostname:    hostname,
		chassis:     chassis,
		client:      redfishClient,
		cachedPower: make(map[string]*redfish.Power, len(chassis)),
		metricDesc:  metricDesc,
	}

	return &collector, nil
}

// Update implements Collector and exposes Redfish power related metrics.
func (c *redfishCollector) Update(ch chan<- prometheus.Metric) error {
	// Returned value 0 means Power Measurement is not avail
	for pType, pValues := range c.powerReadings() {
		for chassID, chassPower := range pValues {
			if chassPower > 0 {
				ch <- prometheus.MustNewConstMetric(c.metricDesc[pType], prometheus.GaugeValue, float64(chassPower), c.hostname, chassID)
			}
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *redfishCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", redfishCollectorSubsystem)

	// Close all idle connections before exiting
	c.client.HTTPClient.CloseIdleConnections()

	return nil
}

// Update implements Collector and exposes Redfish power related metrics.
func (c *redfishCollector) powerReadings() map[string]map[string]float64 {
	// Allocate values map
	values := map[string]map[string]float64{
		"current": make(map[string]float64),
		"min":     make(map[string]float64),
		"max":     make(map[string]float64),
		"avg":     make(map[string]float64),
	}

	// Get power consumption from Redfish
	for _, chass := range c.chassis {
		chassisID := SanitizeMetricName(chass.ID)

		power, err := chass.Power()
		if err != nil {
			c.logger.Error(
				"Failed to get power statistics from Redfish. Using last cached values",
				"err", err,
			)

			power = c.cachedPower[chassisID]
		} else {
			c.cachedPower[chassisID] = power
		}

		// Ensure power is not nil
		if power == nil {
			continue
		}

		// Get all power readings from response
		for _, pwc := range power.PowerControl {
			values["current"][chassisID] += float64(pwc.PowerConsumedWatts)
			values["min"][chassisID] += float64(pwc.PowerMetrics.MinConsumedWatts)
			values["max"][chassisID] += float64(pwc.PowerMetrics.MaxConsumedWatts)
			values["avg"][chassisID] += float64(pwc.PowerMetrics.AverageConsumedWatts)
		}
	}

	return values
}
