// Package tsdb implements TSDB client
package tsdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
)

// Custom errors.
var (
	ErrMissingData         = errors.New("missing data in TSDB response")
	ErrMissingConfig       = errors.New("global config not found in TSDB config")
	ErrFailedTypeAssertion = errors.New("failed type assertion")
)

// Metric defines TSDB metrics.
type Metric map[string]float64

// Response is the TSDB response model.
type Response struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType string      `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

// TSDB struct.
type TSDB struct {
	URL                *url.URL
	Client             *http.Client
	Logger             log.Logger
	scrapeInterval     time.Duration
	evaluationInterval time.Duration
	lastUpdate         time.Time
	available          bool
}

const (
	// Default intervals. Return them if we cannot fetch config.
	defaultScrapeInterval     = time.Minute
	defaultEvaluationInterval = time.Minute
)

// New returns a new instance of TSDB.
func New(webURL string, config config_util.HTTPClientConfig, logger log.Logger) (*TSDB, error) {
	// If webURL is empty return empty struct with available set to false
	if webURL == "" {
		level.Warn(logger).Log("msg", "TSDB web URL not found")

		return &TSDB{
			available: false,
		}, nil
	}

	var tsdbClient *http.Client

	var tsdbURL *url.URL

	var err error
	// Unwrap original error to avoid leaking sensitive passwords in output
	tsdbURL, err = url.Parse(webURL)
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	// Make a HTTP client for TSDB from client config
	if tsdbClient, err = config_util.NewClientFromConfig(config, "tsdb"); err != nil {
		return nil, err
	}

	return &TSDB{
		URL:       tsdbURL,
		Client:    tsdbClient,
		Logger:    logger,
		available: true,
	}, nil
}

// Delete endpoint.
func (t *TSDB) deleteEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/admin/tsdb/delete_series")
}

// Query endpoint.
func (t *TSDB) queryEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/query")
}

// // Query range endpoint
// func (t *TSDB) queryRangeEndpoint() *url.URL {
// 	return t.URL.JoinPath("/api/v1/query_range")
// }

// Config endpoint.
func (t *TSDB) configEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/status/config")
}

// CLI flags endpoint.
func (t *TSDB) flagsEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/status/flags")
}

// String implements stringer method for TSDB.
func (t *TSDB) String() string {
	return fmt.Sprintf("TSDB{URL: %s, available: %t}", t.URL.Redacted(), t.available)
}

// Available returns true if TSDB is alive.
func (t *TSDB) Available() bool {
	return t.available
}

// Ping attempts to ping TSDB.
func (t *TSDB) Ping() error {
	var d net.Dialer
	// Check if TSDB is reachable
	conn, err := d.Dial("tcp", t.URL.Host)
	if err != nil {
		return err
	}

	defer conn.Close()

	return nil
}

// Config returns TSDB config.
func (t *TSDB) Config(ctx context.Context) (map[interface{}]interface{}, error) {
	// Make a API request to TSDB
	data, err := Request(ctx, t.configEndpoint().String(), t.Client)
	if err != nil {
		return nil, err
	}

	// Parse full config data and then extract only global config
	var fullConfig map[interface{}]interface{}

	var configData map[string]interface{}

	var ok bool

	if configData, ok = data.(map[string]interface{}); !ok {
		return nil, ErrFailedTypeAssertion
	}

	if v, exists := configData["yaml"]; exists {
		if value, ok := v.(string); !ok {
			return nil, ErrFailedTypeAssertion
		} else if err = yaml.Unmarshal([]byte(value), &fullConfig); err != nil {
			return nil, err
		}
	}

	return fullConfig, nil
}

// GlobalConfig returns global config section of TSDB.
func (t *TSDB) GlobalConfig(ctx context.Context) (map[string]interface{}, error) {
	// Get config
	fullConfig, err := t.Config(ctx)
	if err != nil {
		return nil, err
	}

	// Extract global config
	if v, exists := fullConfig["global"]; exists {
		if c, ok := v.(map[string]interface{}); ok {
			return c, nil
		}

		return nil, ErrFailedTypeAssertion
	}

	return nil, ErrMissingConfig
}

// Flags returns CLI flags of TSDB.
func (t *TSDB) Flags(ctx context.Context) (map[string]interface{}, error) {
	// Make a API request to TSDB
	data, err := Request(ctx, t.flagsEndpoint().String(), t.Client)
	if err != nil {
		return nil, err
	}

	var flagsData map[string]interface{}

	var ok bool
	if flagsData, ok = data.(map[string]interface{}); !ok {
		return nil, ErrFailedTypeAssertion
	}

	return flagsData, nil
}

// Intervals returns scrape and evaluation intervals of TSDB.
func (t *TSDB) Intervals(ctx context.Context) map[string]time.Duration {
	// Check if lastUpdate time is more than 3 hrs
	if time.Since(t.lastUpdate) < 3*time.Hour {
		return map[string]time.Duration{
			"scrape_interval":     t.scrapeInterval,
			"evaluation_interval": t.evaluationInterval,
		}
	}

	// Set scrapeInterval and evaluationInterval cache values to
	// default and we will override it if found from config
	t.lastUpdate = time.Now()
	t.scrapeInterval = defaultScrapeInterval
	t.evaluationInterval = defaultEvaluationInterval

	// Get config
	var globalConfig map[string]interface{}

	var err error
	if globalConfig, err = t.GlobalConfig(ctx); err != nil {
		return map[string]time.Duration{
			"scrape_interval":     defaultScrapeInterval,
			"evaluation_interval": defaultEvaluationInterval,
		}
	}

	// Parse duration string
	intervals := map[string]time.Duration{
		"scrape_interval":     defaultScrapeInterval,
		"evaluation_interval": defaultEvaluationInterval,
	}

	if v, exists := globalConfig["scrape_interval"]; exists {
		if scrapeInterval, err := model.ParseDuration(v.(string)); err == nil {
			t.scrapeInterval = time.Duration(scrapeInterval)
			intervals["scrape_interval"] = time.Duration(scrapeInterval)
		}
	}

	if v, exists := globalConfig["evaluation_interval"]; exists {
		if evaluationInterval, err := model.ParseDuration(v.(string)); err == nil {
			t.evaluationInterval = time.Duration(evaluationInterval)
			intervals["evaluation_interval"] = time.Duration(evaluationInterval)
		}
	}

	return intervals
}

// RateInterval returns rate interval of TSDB.
func (t *TSDB) RateInterval(ctx context.Context) time.Duration {
	// Grafana recommends atleast 4 times of scrape interval to estimate rate
	return 4 * t.Intervals(ctx)["scrape_interval"]
}

// Query makes a TSDB query.
func (t *TSDB) Query(ctx context.Context, query string, queryTime time.Time) (Metric, error) {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"query": []string{query},
		"time":  []string{queryTime.UTC().Format(time.RFC3339Nano)},
	}

	// Create a new POST request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.queryEndpoint().String(), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unpack into data
	var data Response
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Check if Status is error
	if data.Status == "error" {
		return nil, fmt.Errorf("error response from TSDB: %v", data)
	}

	// Check if Data exists on response
	if data.Data == nil {
		return nil, fmt.Errorf("TSDB response returned no data: %v", data)
	}

	// Check response code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query returned status: %d", resp.StatusCode)
	}

	// Parse data
	queriedValues := make(Metric)

	queryData, ok := data.Data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed type assertion of data: %v", data.Data)
	}

	// Check if results is not nil before converting it to slice of interfaces
	if r, exists := queryData["result"]; exists && r != nil {
		var results, values []interface{}

		var result, metric map[string]interface{}

		var ok bool
		if results, ok = r.([]interface{}); !ok {
			return nil, fmt.Errorf("failed type assertion of result: %v", r)
		}

		for _, res := range results {
			// Check if metric exists on result. If it does, check for uuid and value
			var uuid, value string

			if result, ok = res.(map[string]interface{}); !ok {
				continue
			}

			if m, exists := result["metric"]; exists {
				if metric, ok = m.(map[string]interface{}); !ok {
					continue
				}

				if id, exists := metric["uuid"]; exists {
					if v, ok := id.(string); ok {
						uuid = v
					}
				}

				if val, exists := result["value"]; exists {
					if values, ok = val.([]interface{}); ok {
						if len(values) > 1 {
							if v, ok := values[1].(string); ok {
								value = v
							}
						}
					}
				}
			}

			// Cast value into float64
			valueFloat, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}

			queriedValues[uuid] = valueFloat
		}
	}

	return queriedValues, nil
}

// Delete time series with given labels.
func (t *TSDB) Delete(ctx context.Context, startTime time.Time, endTime time.Time, matchers []string) error {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"match[]": matchers,
		"start":   []string{startTime.UTC().Format(time.RFC3339Nano)},
		"end":     []string{endTime.UTC().Format(time.RFC3339Nano)},
	}

	// Create a new POST request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.deleteEndpoint().String(), strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make request and check status code which is supposed to be 204
	if resp, err := t.Client.Do(req); err != nil {
		return err
	} else if resp.StatusCode != http.StatusNoContent {
		defer resp.Body.Close()

		return fmt.Errorf("expected 204 after deletion of time series received %d", resp.StatusCode)
	}

	return nil
}
