//go:build !noredfish
// +build !noredfish

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/ipmi"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

const redfishCollectorSubsystem = "redfish"

// Header names.
const (
	redfishURLHeaderName = "X-Redfish-Url"
)

const hostnamePlaceholder = "{hostname}"

type redfishConfig struct {
	Web struct {
		Proto        string `yaml:"protocol"`
		Hostname     string `yaml:"hostname"`
		Port         int    `yaml:"port"`
		URL          *url.URL
		ExternalURL  string `yaml:"external_url"`
		Username     string `yaml:"username"`
		Password     string `yaml:"password"`
		InSecure     bool   `yaml:"insecure_skip_verify"`
		SessionToken bool   `yaml:"use_session_token"`
	} `yaml:"redfish_web_config"`
}

type redfishCollector struct {
	logger      *slog.Logger
	hostname    string
	config      *gofish.ClientConfig
	client      *gofish.APIClient
	chassis     []*redfish.Chassis
	cachedPower map[string]*redfish.Power
	metricDesc  map[string]*prometheus.Desc
}

var redfishConfigFile = CEEMSExporterApp.Flag(
	"collector.redfish.web-config",
	"Path to Redfish web configuration file.",
).Default("").String()

func init() {
	RegisterCollector(redfishCollectorSubsystem, defaultDisabled, NewRedfishCollector)
}

// NewRedfishCollector returns a new Collector to fetch power usage from redfish API.
func NewRedfishCollector(logger *slog.Logger) (Collector, error) {
	// Initialize metricDesc map
	metricDesc := map[string]*prometheus.Desc{
		"current": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "current_watts"),
			"Current Power consumption in watts", []string{"hostname", "chassis"}, nil,
		),
		"min": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "min_watts"),
			"Minimum Power consumption in watts", []string{"hostname", "chassis"}, nil,
		),
		"max": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "max_watts"),
			"Maximum Power consumption in watts", []string{"hostname", "chassis"}, nil,
		),
		"avg": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, redfishCollectorSubsystem, "avg_watts"),
			"Average Power consumption in watts", []string{"hostname", "chassis"}, nil,
		),
	}

	// Get absolute config file path
	configFilePath, err := filepath.Abs(*redfishConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of the config file: %w", err)
	}

	// Make config from file
	cfg, err := common.MakeConfig[redfishConfig](configFilePath)
	if err != nil {
		logger.Error("Failed to parse Redfish config file", "err", err)

		return nil, fmt.Errorf("failed to parse Redfish config file: %w", err)
	}

	// If BMC Hostname is not provided, attempt to discover it using OpenIPMI interface
	if cfg.Web.Hostname == "" {
		// Make a new IPMI client
		if client, err := ipmi.NewIPMIClient(0, logger.With("subsystem", "ipmi_client")); err == nil {
			// Attempt to get new IP address
			if bmcIP, err := client.LanIP(time.Second); err == nil {
				// Attempt to get BMC hostname from IP
				if hostname, err := net.LookupAddr(*bmcIP); err == nil {
					cfg.Web.Hostname = hostname[0]
				} else {
					cfg.Web.Hostname = *bmcIP
				}
			}
			defer client.Close()
		}
	}

	// If cfg.Web.Hostname has {hostname} placeholder, replace it with current host name
	cfg.Web.Hostname = strings.Replace(cfg.Web.Hostname, hostnamePlaceholder, hostname, -1)

	// Build Redfish URL
	cfg.Web.URL, err = url.Parse(fmt.Sprintf("%s://%s:%d", cfg.Web.Proto, cfg.Web.Hostname, cfg.Web.Port))
	if err != nil {
		logger.Error("Failed to build Redfish URL", "err", err)

		return nil, fmt.Errorf("invalid redfish web config: %w", err)
	}

	logger.Debug("Redfish URL", "url", cfg.Web.URL.String())

	// Make a new HTTP client config
	clientConfig := config.HTTPClientConfig{
		TLSConfig: config.TLSConfig{
			InsecureSkipVerify: cfg.Web.InSecure,
		},
		HTTPHeaders: &config.Headers{
			Headers: map[string]config.Header{
				redfishURLHeaderName: {
					Values: []string{cfg.Web.URL.String()},
				},
			},
		},
	}

	// Get the URL that client will talk to
	// If external URL is provided, always prefer it over the raw BMC hostname and port
	var endpoint string
	if cfg.Web.ExternalURL != "" {
		endpoint = cfg.Web.ExternalURL
	} else {
		endpoint = cfg.Web.URL.String()
	}

	// Make a HTTP client from client config
	httpClient, err := config.NewClientFromConfig(clientConfig, "redfish")
	if err != nil {
		logger.Error("Failed to create a HTTP client for Redfish", "err", err)

		return nil, fmt.Errorf("failed to create a HTTP client for Redfish: %w", err)
	}

	// Create a redfish client
	config := gofish.ClientConfig{
		Endpoint:         endpoint,
		Username:         cfg.Web.Username,
		Password:         cfg.Web.Password,
		Insecure:         cfg.Web.InSecure,
		BasicAuth:        !cfg.Web.SessionToken,
		HTTPClient:       httpClient,
		ReuseConnections: true,
	}

	collector := redfishCollector{
		logger:      logger,
		hostname:    hostname,
		config:      &config,
		cachedPower: make(map[string]*redfish.Power),
		metricDesc:  metricDesc,
	}

	// Connect to Redfish server
	if err := collector.connect(); err != nil {
		logger.Error("Failed to connect to Redfish server", "err", err)

		return nil, err
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

	// Delete sesssion and close all idle connections before exiting
	c.logout()

	return nil
}

// connect establishes a connection with Redfish server and sets the client.
func (c *redfishCollector) connect() error {
	var err error

	// Connect to redfish API server
	c.client, err = gofish.Connect(*c.config)
	if err != nil {
		return fmt.Errorf("failed to create a Redfish client: %w", err)
	}

	// Get all available chassis
	c.chassis, err = c.client.Service.Chassis()
	if err != nil {
		return fmt.Errorf("failed to fetch available chassis from Redfish: %w", err)
	}

	return nil
}

// logout logs out from active session and set client to nil so that new client can be
// started.
func (c *redfishCollector) logout() {
	// Attempt to log out before creating new client
	c.client.Logout()

	// Set client to nil
	c.client = nil
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

			// Attempt to log out and create new client
			c.logout()

			if err := c.connect(); err != nil {
				c.logger.Error("Failed to create new redfish client", "err", err)
			}
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
