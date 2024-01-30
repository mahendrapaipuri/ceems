package tsdb

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// metric type
type metric map[int64]float64

// CPU metrics
type CPUMetrics struct {
	AvgCPUUsage         metric
	AvgCPUMemUsage      metric
	TotalCPUEnergyUsage metric
	TotalCPUEmissions   metric
}

// GPU metrics
type GPUMetrics struct {
	AvgGPUUsage         metric
	AvgGPUMemUsage      metric
	TotalGPUEnergyUsage metric
	TotalGPUEmissions   metric
}

// TSDB response
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
	globalConfig       map[interface{}]interface{}
	scrapeInterval     time.Duration
	rateInterval       time.Duration
	lastConfigUpdate   time.Time
	available          bool
}

const (
	// Default scrape interval. Return this if we cannot fetch config
	defaultScrapeInterval = time.Duration(time.Minute)
)

// Return a new instance of TSDB struct
func NewTSDB(webURL string, skipTLSVerify bool) (*TSDB, error) {
	// If webURL is empty return empty struct with available set to false
	if webURL == "" {
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
	if skipTLSVerify {
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
		available:          true,
	}, nil
}

// Stringer receiver for tsdbConfig
func (t *TSDB) String() string {
	return fmt.Sprintf("TSDB{URL: %s, available: %t}", t.URL.Redacted(), t.available)
}

// Return true if TSDB is available
func (t *TSDB) Available() bool {
	return t.available
}

// Check if TSDB is reachable
func (t *TSDB) Ping() error {
	// Create a new GET request to reach out to TSDB
	req, err := http.NewRequest(http.MethodGet, t.URL.String(), nil)
	if err != nil {
		return err
	}

	// Check if TSDB is reachable
	if _, err = t.Client.Do(req); err != nil {
		return err
	}
	return nil
}

// TSDB config setter
func (t *TSDB) Config() error {
	// Create a new GET request to reach out to TSDB
	req, err := http.NewRequest(http.MethodGet, t.ConfigEndpoint.String(), nil)
	if err != nil {
		return err
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/json")

	// Check if TSDB is reachable
	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Unpack into data
	var data Response
	if err = json.Unmarshal(body, &data); err != nil {
		return err
	}

	// if Data field is nil return err
	if data.Data == nil {
		return fmt.Errorf("TSDB returned no config")
	}

	// Parse full config data and then extract only global config
	var fullConfig map[interface{}]interface{}
	configData := data.Data.(map[string]interface{})
	if v, exists := configData["yaml"]; exists && v.(string) != "" {
		if err = yaml.Unmarshal([]byte(v.(string)), &fullConfig); err != nil {
			return err
		}

		// Set global config
		if v, exists := fullConfig["global"]; exists {
			t.globalConfig = v.(map[interface{}]interface{})
		}
	}
	return nil
}

// Scrape interval setter
func (t *TSDB) ScrapeInterval() {
	// Check if lastConfigUpdate time is more than 3 hrs
	if time.Since(t.lastConfigUpdate) < time.Duration(3*time.Hour) {
		return
	}

	// Get config
	if err := t.Config(); err != nil {
		t.scrapeInterval = defaultScrapeInterval
		return
	}
	t.lastConfigUpdate = time.Now()

	// Parse duration string
	if v, exists := t.globalConfig["scrape_interval"]; exists {
		scrapeInterval, err := model.ParseDuration(v.(string))
		if err != nil {
			t.scrapeInterval = defaultScrapeInterval
		}
		t.scrapeInterval = time.Duration(scrapeInterval)
	} else {
		t.scrapeInterval = defaultScrapeInterval
	}
}

// Rate interval setter
func (t *TSDB) RateInterval() {
	// Get scrape interval
	t.ScrapeInterval()

	// Grafana recommends atleast 4 times of scrape interval to estimate rate
	t.rateInterval = 4 * t.scrapeInterval
}

// Return formatted query string after replacing placeholders
func (t *TSDB) queryString(query string, jobs string, maxDuration time.Duration) string {
	return fmt.Sprintf(strings.TrimLeft(query, "\n"), jobs, t.rateInterval, maxDuration, t.scrapeInterval, t.scrapeInterval.Milliseconds())
}

// Get CPU metrics of jobs
func (t *TSDB) CPUMetrics(queryTime time.Time, maxDuration time.Duration, jobs string) (CPUMetrics, error) {
	var cpuMetrics CPUMetrics
	var err error
	var errs error

	// Get scrape and rate intervals
	t.RateInterval()

	// Avg CPU usage query
	cpuUsageQuery := t.queryString(avgCpuUsageQuery, jobs, maxDuration)
	if cpuMetrics.AvgCPUUsage, err = t.Query(cpuUsageQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query avg CPU usage: %s", err), errs)
	}

	// Avg CPU mem query
	cpuMemQuery := t.queryString(avgCpuMemUsageQuery, jobs, maxDuration)
	if cpuMetrics.AvgCPUMemUsage, err = t.Query(cpuMemQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query avg CPU mem usage: %s", err), errs)
	}

	// Avg CPU usage query
	cpuEnergyQuery := t.queryString(totalCpuEnergyUsageQuery, jobs, maxDuration)
	if cpuMetrics.TotalCPUEnergyUsage, err = t.Query(cpuEnergyQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query total CPU energy usage: %s", err), errs)
	}

	// Avg CPU usage query
	cpuEmissionsQuery := t.queryString(totalCpuEmissionsUsageQuery, jobs, maxDuration)
	if cpuMetrics.TotalCPUEmissions, err = t.Query(cpuEmissionsQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query total CPU emissions: %s", err), errs)
	}
	return cpuMetrics, errs
}

// Get GPU metrics of jobs
func (t *TSDB) GPUMetrics(queryTime time.Time, maxDuration time.Duration, jobs string) (GPUMetrics, error) {
	var gpuMetrics GPUMetrics
	var err error
	var errs error

	// Get rate interval
	t.RateInterval()

	// Avg GPU usage query
	gpuUsageQuery := t.queryString(avgGpuUsageQuery, jobs, maxDuration)
	if gpuMetrics.AvgGPUUsage, err = t.Query(gpuUsageQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query avg GPU usage: %s", err), errs)
	}

	// Avg GPU mem query
	gpuMemQuery := t.queryString(avgGpuMemUsageQuery, jobs, maxDuration)
	if gpuMetrics.AvgGPUMemUsage, err = t.Query(gpuMemQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query avg GPU mem usage: %s", err), errs)
	}

	// Avg GPU usage query
	gpuEnergyQuery := t.queryString(totalGpuEnergyUsageQuery, jobs, maxDuration)
	if gpuMetrics.TotalGPUEnergyUsage, err = t.Query(gpuEnergyQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query total GPU energy usage: %s", err), errs)
	}

	// Avg GPU usage query
	gpuEmissionsQuery := t.queryString(totalGpuEmissionsUsageQuery, jobs, maxDuration)
	if gpuMetrics.TotalGPUEmissions, err = t.Query(gpuEmissionsQuery, queryTime); err != nil {
		errs = errors.Join(fmt.Errorf("failed to query total GPU emissions: %s", err), errs)
	}
	return gpuMetrics, errs
}

// Get average CPU utilisation of jobs
func (t *TSDB) Query(query string, queryTime time.Time) (map[int64]float64, error) {
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

	// Unpack into data
	var data Response
	if err = json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Check if Data exists on response
	if data.Data == nil {
		return nil, fmt.Errorf("TSDB response returned no data")
	}

	// Parse data
	var queriedValues = make(map[int64]float64)
	queryData := data.Data.(map[string]interface{})
	if results, exists := queryData["result"]; exists {
		for _, result := range results.([]interface{}) {
			// Check if metric exists on result. If it does, check for jobid and value
			var jobid, value string
			if metric, exists := result.(map[string]interface{})["metric"]; exists {
				if jid, exists := metric.(map[string]interface{})["jobid"]; exists {
					jobid = jid.(string)
				}
				if val, exists := result.(map[string]interface{})["value"]; exists {
					if len(val.([]interface{})) > 1 {
						value = val.([]interface{})[1].(string)
					}
				}
			}

			// Cast jobid and value into proper types
			jobidInt, err := strconv.ParseInt(jobid, 10, 64)
			if err != nil {
				continue
			}
			valueFloat, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			queriedValues[jobidInt] = valueFloat
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
	if _, err = t.Client.Do(req); err != nil {
		return err
	}
	return nil
}
