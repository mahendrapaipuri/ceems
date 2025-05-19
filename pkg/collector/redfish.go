//go:build !noredfish
// +build !noredfish

package collector

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
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
		Proto            string                  `yaml:"protocol"`
		Hostname         string                  `yaml:"hostname"`
		Port             int                     `yaml:"port"`
		Username         string                  `yaml:"username"`
		Password         string                  `yaml:"password"`
		SessionToken     bool                    `yaml:"use_session_token"`
		Timeout          int64                   `yaml:"timeout"`
		ExternalURL      string                  `yaml:"external_url"`
		HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
		url              *url.URL
		// Deprecated: InSecure exists for historical compatibility
		// and should not be used. This must be configured under
		// `tls_config.insecure_skip_verify` from now on.
		InSecure bool `yaml:"insecure_skip_verify"`
	} `yaml:"redfish_web_config"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *redfishConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	*c = redfishConfig{}
	c.Web.SessionToken = true
	c.Web.HTTPClientConfig = config.DefaultHTTPClientConfig

	type plain redfishConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	var err error

	// If BMC Hostname is not provided, attempt to discover it using OpenIPMI interface
	if c.Web.Hostname == "" {
		// Make a new IPMI client
		client, err := ipmi.NewIPMIClient(0, slog.New(slog.DiscardHandler))
		if err != nil {
			return fmt.Errorf("failed to create IPMI client to get BMC address: %w", err)
		}
		defer client.Close()

		// Attempt to get new IP address
		bmcIP, err := client.LanIP(time.Second)
		if err != nil {
			return fmt.Errorf("failed to get BMC LAN IP: %w", err)
		}

		// Attempt to get BMC hostname from IP
		if hostname, err := net.LookupAddr(*bmcIP); err == nil {
			c.Web.Hostname = hostname[0]
		} else {
			c.Web.Hostname = *bmcIP
		}
	}

	// If cfg.Web.Hostname has {hostname} placeholder, replace it with current host name
	c.Web.Hostname = strings.ReplaceAll(c.Web.Hostname, hostnamePlaceholder, hostname)

	// Build Redfish URL
	c.Web.url, err = url.Parse(fmt.Sprintf("%s://%s:%d", c.Web.Proto, c.Web.Hostname, c.Web.Port))
	if err != nil {
		return fmt.Errorf("invalid redfish web config: %w", err)
	}

	// Add redfish target URL in the header for proxy web config
	c.Web.HTTPClientConfig.HTTPHeaders = &config.Headers{
		Headers: map[string]config.Header{
			redfishURLHeaderName: {
				Values: []string{c.Web.url.String()},
			},
		},
	}

	// If InSecure is set to true
	if c.Web.InSecure {
		c.Web.HTTPClientConfig.TLSConfig = config.TLSConfig{
			InsecureSkipVerify: c.Web.InSecure,
		}
	}

	return nil
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
).Envar("CEEMS_EXPORTER_REDFISH_COLL_CONFIG_FILE").Default("").String()

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

	logger.Debug("Redfish URL", "url", cfg.Web.url.String())

	// Get the URL that client will talk to
	// If external URL is provided, always prefer it over the raw BMC hostname and port
	var endpoint string
	if cfg.Web.ExternalURL != "" {
		endpoint = cfg.Web.ExternalURL
	} else {
		endpoint = cfg.Web.url.String()
	}

	// Make a HTTP client from client config
	httpClient, err := config.NewClientFromConfig(cfg.Web.HTTPClientConfig, "redfish")
	if err != nil {
		logger.Error("Failed to create a HTTP client for Redfish", "err", err)

		return nil, fmt.Errorf("failed to create a HTTP client for Redfish: %w", err)
	}

	// Set a timeout here to not to block redfish collector whole
	// exporter. If no timeout is provided use default value of 5 seconds
	// Good ref: https://stackoverflow.com/a/72358623
	if cfg.Web.Timeout <= 0 {
		cfg.Web.Timeout = 5000
	}

	httpClient.Timeout = time.Duration(cfg.Web.Timeout * int64(time.Millisecond))

	// Override username and password from env vars when found
	if os.Getenv("REDFISH_WEB_USERNAME") != "" {
		cfg.Web.Username = os.Getenv("REDFISH_WEB_USERNAME")
	}

	if os.Getenv("REDFISH_WEB_PASSWORD") != "" {
		cfg.Web.Password = os.Getenv("REDFISH_WEB_PASSWORD")
	}

	// Create a redfish client
	config := gofish.ClientConfig{
		Endpoint:         endpoint,
		Username:         cfg.Web.Username,
		Password:         cfg.Web.Password,
		Insecure:         cfg.Web.HTTPClientConfig.TLSConfig.InsecureSkipVerify,
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
		if err != nil || power == nil {
			c.logger.Error(
				"Failed to get power statistics from Redfish. Using last cached values",
				"err", err,
			)

			power = c.cachedPower[chassisID]

			// If there is an error, reset client
			if err != nil {
				// Attempt to log out and create new client
				c.logout()

				// When this happens this scrape is lost and it will return cached values
				// but the next scrape should be good as we created new client
				if err := c.connect(); err != nil {
					c.logger.Error("Failed to create new redfish client", "err", err)
				}
			}
		} else {
			c.cachedPower[chassisID] = power
		}

		// Even if cached Power is nil bail
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
