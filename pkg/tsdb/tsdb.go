// Package tsdb implements TSDB client
package tsdb

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

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

var settingsLock = sync.RWMutex{}

// Metric defines TSDB metrics.
type Metric map[string]float64

// RangeMetric defines TSDB range metrics.
type RangeMetric map[string][]interface{}

// Series is TSDB series.
type Series struct {
	Name     string `json:"__name__"`
	Job      string `json:"job"`
	Instance string `json:"instance"`
}

// Response is the TSDB response model.
type Response[T any] struct {
	Status    string   `json:"status"`
	Data      T        `json:"data,omitempty"`
	ErrorType string   `json:"errorType,omitempty"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

type Settings struct {
	ScrapeInterval     time.Duration
	EvaluationInterval time.Duration
	RateInterval       time.Duration
	QueryLookbackDelta time.Duration
	QueryTimeout       time.Duration
	QueryMaxSamples    uint64
	RetentionPeriod    time.Duration
}

// TSDB struct.
type TSDB struct {
	URL              *url.URL
	Client           *http.Client
	Logger           *slog.Logger
	settingsCache    *Settings
	settingsCacheTTL time.Duration
	lastUpdate       time.Time
	available        bool
}

const (
	// Default intervals. Return them if we cannot fetch config.
	defaultScrapeInterval     = time.Minute
	defaultEvaluationInterval = time.Minute
	defaultLookbackDelta      = 5 * time.Minute
	defaultQueryTimeout       = 2 * time.Minute
	defaultQueryMaxSamples    = 50000000
)

const errorStatus = "error"

var defaultSettings = Settings{
	ScrapeInterval:     defaultScrapeInterval,
	EvaluationInterval: defaultEvaluationInterval,
	RateInterval:       4 * defaultScrapeInterval,
	QueryLookbackDelta: defaultLookbackDelta,
	QueryTimeout:       defaultQueryTimeout,
	QueryMaxSamples:    uint64(defaultQueryMaxSamples),
}

// New returns a new instance of TSDB.
func New(webURL string, config config_util.HTTPClientConfig, logger *slog.Logger) (*TSDB, error) {
	// If webURL is empty return empty struct with available set to false
	if webURL == "" {
		logger.Warn("TSDB web URL not found")

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
		URL:              tsdbURL,
		Client:           tsdbClient,
		Logger:           logger,
		settingsCache:    &defaultSettings,
		settingsCacheTTL: 6 * time.Hour, // Update TSDB settings for every 6 hours
		available:        true,
	}, nil
}

// Series endpoint.
func (t *TSDB) seriesEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/series")
}

// Labels endpoint.
func (t *TSDB) labelsEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/labels")
}

// Delete endpoint.
func (t *TSDB) deleteEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/admin/tsdb/delete_series")
}

// Query endpoint.
func (t *TSDB) queryEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/query")
}

// Query range endpoint.
func (t *TSDB) queryRangeEndpoint() *url.URL {
	return t.URL.JoinPath("/api/v1/query_range")
}

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

// Settings returns selected TSDB config settings.
func (t *TSDB) Settings(ctx context.Context) *Settings {
	// This must be protected from concurrent access
	settingsLock.Lock()
	defer settingsLock.Unlock()

	// Check if lastUpdate time is more than 3 hrs
	if time.Since(t.lastUpdate) < t.settingsCacheTTL {
		return t.settingsCache
	}

	// Update settings and lastUpdate time
	if settings, err := t.fetchSettings(ctx); err == nil {
		t.lastUpdate = time.Now()
		t.settingsCache = settings
	}

	return t.settingsCache
}

// fetchSettings returns selected TSDB config parameters.
func (t *TSDB) fetchSettings(ctx context.Context) (*Settings, error) {
	// Get global config
	globalConfig, err := t.GlobalConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Get flags
	flags, err := t.Flags(ctx)
	if err != nil {
		return nil, err
	}

	// Make a default settings struct
	settings := defaultSettings

	// Get scrape and evaluation intervals
	if v, exists := globalConfig["scrape_interval"]; exists {
		if scrapeInterval, err := model.ParseDuration(v.(string)); err == nil {
			settings.ScrapeInterval = time.Duration(scrapeInterval)
		}
	}

	if v, exists := globalConfig["evaluation_interval"]; exists {
		if evaluationInterval, err := model.ParseDuration(v.(string)); err == nil {
			settings.EvaluationInterval = time.Duration(evaluationInterval)
		}
	}

	// Get query timeout and max samples from flags
	if v, exists := flags["query.max-samples"]; exists {
		if s, ok := v.(float64); ok {
			settings.QueryMaxSamples = uint64(s)
		}
	}

	if v, exists := flags["query.timeout"]; exists {
		if queryTimeout, err := model.ParseDuration(v.(string)); err == nil {
			settings.QueryTimeout = time.Duration(queryTimeout)
		}
	}

	if v, exists := flags["query.lookback-delta"]; exists {
		if queryTimeout, err := model.ParseDuration(v.(string)); err == nil {
			settings.QueryTimeout = time.Duration(queryTimeout)
		}
	}

	// Set rate interval as 4 times scrape interval (Grafana recommendation)
	settings.RateInterval = 4 * settings.ScrapeInterval

	return &settings, nil
}

// Config returns full TSDB config.
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

func GetBytes(key interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	err := enc.Encode(key)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Series makes a TSDB query to get series.
func (t *TSDB) Series(ctx context.Context, matchers []string, start time.Time, end time.Time) ([]Series, error) {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"match[]": matchers,
	}

	// If times are not zero, set them in query params
	if !start.IsZero() {
		values.Add("start", start.UTC().Format(time.RFC3339Nano))
	}

	if !end.IsZero() {
		values.Add("end", end.UTC().Format(time.RFC3339Nano))
	}

	// Create a new POST request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		t.seriesEndpoint().String(),
		strings.NewReader(values.Encode()),
	)
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
	var data Response[[]Series]
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Check if Status is error
	if data.Status == errorStatus {
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

	return data.Data, nil
}

// Labels makes a TSDB query to get list of labels.
func (t *TSDB) Labels(ctx context.Context, matchers []string, start time.Time, end time.Time) ([]string, error) {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{}

	// If matchers are provided add to query
	for _, matcher := range matchers {
		values.Add("match[]", matcher)
	}

	// If times are not zero, add to query params
	if !start.IsZero() {
		values.Add("start", start.UTC().Format(time.RFC3339Nano))
	}

	if !end.IsZero() {
		values.Add("end", end.UTC().Format(time.RFC3339Nano))
	}

	// Create a new POST request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		t.labelsEndpoint().String(),
		strings.NewReader(values.Encode()),
	)
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
	var data Response[[]string]
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Check if Status is error
	if data.Status == errorStatus {
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

	return data.Data, nil
}

// Query makes a TSDB query.
func (t *TSDB) Query(ctx context.Context, query string, queryTime time.Time) (Metric, error) {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"query": []string{query},
		"time":  []string{queryTime.UTC().Format(time.RFC3339Nano)},
	}

	// Get current scrape interval to use as lookback_delta
	// This query parameter is undocumented on Prometheus. If we use
	// default value of 5m, we tend to have metrics 5m **after** compute
	// unit has finished which gives over estimation of energy
	if scrapeInterval := t.Settings(ctx).ScrapeInterval; scrapeInterval > 0 {
		values.Add("lookback_delta", scrapeInterval.String())
	}

	// Create a new POST request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		t.queryEndpoint().String(),
		strings.NewReader(values.Encode()),
	)
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
	var data Response[any]
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Check if Status is error
	if data.Status == errorStatus {
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
		return nil, fmt.Errorf("%w on data: %v", ErrFailedTypeAssertion, data.Data)
	}

	// Check if results is not nil before converting it to slice of interfaces
	if r, exists := queryData["result"]; exists && r != nil {
		var results, values []interface{}

		var result, metric map[string]interface{}

		var ok bool
		if results, ok = r.([]interface{}); !ok {
			return nil, fmt.Errorf("%w on result: %v", ErrFailedTypeAssertion, r)
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

// RangeQuery makes a TSDB range query.
func (t *TSDB) RangeQuery(
	ctx context.Context,
	query string,
	startTime time.Time,
	endTime time.Time,
	step string,
) (RangeMetric, error) {
	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"query": []string{query},
		"start": []string{startTime.UTC().Format(time.RFC3339Nano)},
		"end":   []string{endTime.UTC().Format(time.RFC3339Nano)},
		"step":  []string{step},
	}

	// Get current scrape interval to use as lookback_delta
	// This query parameter is undocumented on Prometheus. If we use
	// default value of 5m, we tend to have metrics 5m **after** compute
	// unit has finished which gives over estimation of energy
	if scrapeInterval := t.Settings(ctx).ScrapeInterval; scrapeInterval > 0 {
		values.Add("lookback_delta", scrapeInterval.String())
	}

	// Create a new POST request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		t.queryRangeEndpoint().String(),
		strings.NewReader(values.Encode()),
	)
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
	var data Response[any]
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Check if Status is error
	if data.Status == errorStatus {
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

	queryData, ok := data.Data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%w on data: %v", ErrFailedTypeAssertion, data.Data)
	}

	// Parse data
	queriedRangeValues := make(RangeMetric)

	// Check if results is not nil before converting it to slice of interfaces
	if r, exists := queryData["result"]; exists && r != nil {
		var results, values []interface{}

		var result, metric map[string]interface{}

		var name interface{}

		var n string

		var ok bool
		if results, ok = r.([]interface{}); !ok {
			return nil, fmt.Errorf("%w on result: %v", ErrFailedTypeAssertion, r)
		}

		for _, res := range results {
			if result, ok = res.(map[string]interface{}); !ok {
				continue
			}

			if m, exists := result["metric"]; exists {
				if metric, ok = m.(map[string]interface{}); !ok {
					continue
				}

				if name, ok = metric["__name__"]; !ok {
					continue
				}

				if n, ok = name.(string); !ok {
					continue
				}

				if val, exists := result["values"]; exists {
					if values, ok = val.([]interface{}); ok {
						queriedRangeValues[n] = values
					}
				}
			}
		}
	}

	return queriedRangeValues, nil
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
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		t.deleteEndpoint().String(),
		strings.NewReader(values.Encode()),
	)
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
