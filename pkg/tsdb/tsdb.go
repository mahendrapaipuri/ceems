// Package tsdb implements TSDB client
package tsdb

import (
	"crypto/tls"
	"encoding/json"
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
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// Metric defines TSDB metrics
type Metric map[string]float64

// Response is the TSDB response model
type Response struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType string      `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

// TSDB struct
type TSDB struct {
	URL                *url.URL
	Client             *http.Client
	DeleteEndpoint     *url.URL
	QueryEndpoint      *url.URL
	QueryRangeEndpoint *url.URL
	ConfigEndpoint     *url.URL
	Logger             log.Logger
	scrapeInterval     time.Duration
	lastConfigUpdate   time.Time
	available          bool
}

const (
	// Default scrape interval. Return this if we cannot fetch config
	defaultScrapeInterval = time.Duration(time.Minute)
)

// NewTSDB returns a new instance of TSDB
func NewTSDB(webURL string, webSkipTLSVerify bool, logger log.Logger) (*TSDB, error) {
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
	tsdbURL, err = url.Parse(webURL)
	if err != nil {
		return nil, err
	}

	// If skip verify is set to true for TSDB add it to client
	if webSkipTLSVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		tsdbClient = &http.Client{Transport: tr, Timeout: time.Duration(30 * time.Second)}
	} else {
		tsdbClient = &http.Client{Timeout: time.Duration(30 * time.Second)}
	}
	return &TSDB{
		URL:                tsdbURL,
		Client:             tsdbClient,
		DeleteEndpoint:     tsdbURL.JoinPath("/api/v1/admin/tsdb/delete_series"),
		QueryEndpoint:      tsdbURL.JoinPath("/api/v1/query"),
		QueryRangeEndpoint: tsdbURL.JoinPath("/api/v1/query_range"),
		ConfigEndpoint:     tsdbURL.JoinPath("/api/v1/status/config"),
		Logger:             logger,
		available:          true,
	}, nil
}

// String implements stringer method for TSDB
func (t *TSDB) String() string {
	return fmt.Sprintf("TSDB{URL: %s, available: %t}", t.URL.Redacted(), t.available)
}

// Available returns true if TSDB is alive
func (t *TSDB) Available() bool {
	return t.available
}

// Ping attempts to ping TSDB
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

// Config returns TSDB config
func (t *TSDB) Config() (map[interface{}]interface{}, error) {
	// Make a API request to TSDB
	data, err := Request(t.ConfigEndpoint.String(), t.Client)
	if err != nil {
		return nil, err
	}

	// Parse full config data and then extract only global config
	var fullConfig map[interface{}]interface{}
	configData := data.(map[string]interface{})
	if v, exists := configData["yaml"]; exists && v.(string) != "" {
		if err = yaml.Unmarshal([]byte(v.(string)), &fullConfig); err != nil {
			return nil, err
		}
	}
	return fullConfig, nil
}

// GlobalConfig returns global config section of TSDB
func (t *TSDB) GlobalConfig() (map[interface{}]interface{}, error) {
	// Get config
	fullConfig, err := t.Config()
	if err != nil {
		return nil, err
	}

	// Extract global config
	if v, exists := fullConfig["global"]; exists {
		return v.(map[interface{}]interface{}), nil
	}
	return nil, fmt.Errorf("global config not found in TSDB config")
}

// ScrapeInterval returns scrape interval of TSDB
func (t *TSDB) ScrapeInterval() time.Duration {
	// Check if lastConfigUpdate time is more than 3 hrs
	if time.Since(t.lastConfigUpdate) < time.Duration(3*time.Hour) {
		return t.scrapeInterval
	}

	// Set scrapeInterval cache value to default and we will override it if found
	// from config
	t.lastConfigUpdate = time.Now()
	t.scrapeInterval = defaultScrapeInterval

	// Get config
	var globalConfig map[interface{}]interface{}
	var err error
	if globalConfig, err = t.GlobalConfig(); err != nil {
		return defaultScrapeInterval
	}

	// Parse duration string
	if v, exists := globalConfig["scrape_interval"]; exists {
		scrapeInterval, err := model.ParseDuration(v.(string))
		if err != nil {
			return defaultScrapeInterval
		}
		t.scrapeInterval = time.Duration(scrapeInterval)
		return time.Duration(scrapeInterval)
	}
	return defaultScrapeInterval
}

// RateInterval returns rate interval of TSDB
func (t *TSDB) RateInterval() time.Duration {
	// Grafana recommends atleast 4 times of scrape interval to estimate rate
	return 4 * t.ScrapeInterval()
}

// Query makes a TSDB query
func (t *TSDB) Query(query string, queryTime time.Time) (Metric, error) {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"query": []string{query},
		"time":  []string{queryTime.UTC().Format(time.RFC3339Nano)},
	}

	// Create a new POST request
	req, err := http.NewRequest(http.MethodPost, t.QueryEndpoint.String(), strings.NewReader(values.Encode()))
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

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check response code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("query returned status: %d", resp.StatusCode)
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

	// Parse data
	var queriedValues = make(Metric)
	queryData := data.Data.(map[string]interface{})
	if results, exists := queryData["result"]; exists {
		for _, result := range results.([]interface{}) {
			// Check if metric exists on result. If it does, check for uuid and value
			var uuid, value string
			if metric, exists := result.(map[string]interface{})["metric"]; exists {
				if id, exists := metric.(map[string]interface{})["uuid"]; exists {
					uuid = id.(string)
				}
				if val, exists := result.(map[string]interface{})["value"]; exists {
					if len(val.([]interface{})) > 1 {
						value = val.([]interface{})[1].(string)
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

// Delete time series with given labels
func (t *TSDB) Delete(startTime time.Time, endTime time.Time, matcher string) error {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"match[]": []string{matcher},
		"start":   []string{startTime.UTC().Format(time.RFC3339Nano)},
		"end":     []string{endTime.UTC().Format(time.RFC3339Nano)},
	}

	// Create a new POST request
	req, err := http.NewRequest(http.MethodPost, t.DeleteEndpoint.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	if resp, err := t.Client.Do(req); err != nil {
		return err
	} else {
		// Check status code which is supposed to be 204
		if resp.StatusCode != 204 {
			return fmt.Errorf("expected 204 after deletion of time series received %d", resp.StatusCode)
		}
	}
	return nil
}
