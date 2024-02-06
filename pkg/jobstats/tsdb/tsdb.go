package tsdb

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/schedulers"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// metric type
type metric map[int64]float64

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
	logger             log.Logger
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

// TSDB related CLI args
var (
	tsdbWebUrl = base.BatchJobStatsServerApp.Flag(
		"tsdb.web.url",
		"TSDB URL (Prometheus/Victoria Metrics). If basic auth is enabled consider providing this URL using environment variable TSDB_WEBURL.",
	).Default(os.Getenv("TSDB_WEBURL")).String()
	tsdbWebSkipTLSVerify = base.BatchJobStatsServerApp.Flag(
		"tsdb.web.skip-tls-verify",
		"Whether to skip TLS verification when using self signed certificates (default is false).",
	).Default("false").Bool()
	metricLock = sync.RWMutex{}
)

// Register TSDB updater
func init() {
	schedulers.RegisterUpdater("tsdb", false, NewTSDBUpdater)
}

// Factory to create a new updater interface
func NewTSDBUpdater(logger log.Logger) (schedulers.Updater, error) {
	tsdb, err := NewTSDB(logger)
	if err != nil {
		return nil, err
	}
	return tsdb, nil
}

// Return a new instance of TSDB struct
func NewTSDB(logger log.Logger) (*TSDB, error) {
	// If webURL is empty return empty struct with available set to false
	if *tsdbWebUrl == "" {
		level.Warn(logger).Log("msg", "TSDB web URL not found")
		return &TSDB{
			available: false,
		}, nil
	}

	var tsdbClient *http.Client
	var tsdbURL *url.URL
	var err error
	tsdbURL, err = url.Parse(*tsdbWebUrl)
	if err != nil {
		return nil, err
	}

	// If skip verify is set to true for TSDB add it to client
	if *tsdbWebSkipTLSVerify {
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
		logger:             logger,
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
	return fmt.Sprintf(
		strings.TrimLeft(query, "\n"),
		jobs,
		t.rateInterval,
		maxDuration,
		t.scrapeInterval,
		t.scrapeInterval.Milliseconds(),
	)
}

// Get aggregate metrics of all jobs
func (t *TSDB) FetchAggMetrics(queryTime time.Time, maxDuration time.Duration, jobs string) map[string]metric {
	var aggMetrics = make(map[string]metric, len(aggMetricQueries))
	var err error

	// Get scrape and rate intervals
	t.RateInterval()

	// Start a wait group
	var wg sync.WaitGroup
	wg.Add(len(aggMetricQueries))

	// Loop over aggMetricQueries map and make queries
	for name, query := range aggMetricQueries {
		go func(n string, q string) {
			var aggMetric metric
			tsdbQuery := t.queryString(q, jobs, maxDuration)
			if aggMetric, err = t.Query(tsdbQuery, queryTime); err != nil {
				level.Error(t.logger).Log("msg", "Failed to fetch metrics from TSDB", "metric", n, "err", err)
			} else {
				metricLock.Lock()
				aggMetrics[n] = aggMetric
				metricLock.Unlock()
			}
			wg.Done()
		}(name, query)
	}
	// Wait for all go routines
	wg.Wait()
	return aggMetrics
}

// Fetch job metrics from TSDB and update JobStat struct for each job
func (t *TSDB) Update(queryTime time.Time, jobs []base.Job) []base.Job {
	// Check if TSDB is available
	if !t.Available() {
		return jobs
	}
	var minStartTime = queryTime.UnixMilli()
	var allJobIds = make([]string, len(jobs))

	// Loop over all jobs and find earliest start time of a job
	for i := 0; i < len(jobs); i++ {
		allJobIds[i] = fmt.Sprintf("%d", jobs[i].Jobid)
		if jobs[i].StartTS > 0 && minStartTime > jobs[i].StartTS {
			minStartTime = jobs[i].StartTS
		}
	}
	allJobIdsExp := strings.Join(allJobIds, "|")

	// Get max window from minStartTime to queryTime
	maxDuration := time.Duration((queryTime.UnixMilli() - minStartTime) * int64(time.Millisecond)).Truncate(time.Minute)

	// Get all aggregate metrics
	aggMetrics := t.FetchAggMetrics(queryTime, maxDuration, allJobIdsExp)

	// Update all jobs
	// NOTE: We can improve this by using reflect package by naming queries
	// after field names. That will allow us to dynamically look up struct
	// field using query name and set the properties.
	for _, job := range jobs {
		// Update with CPU metrics
		if metric, mExists := aggMetrics["cpuUsage"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.AveCPUUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuMemUsage"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.AveCPUMemUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuEnergyUsage"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.TotalCPUEnergyUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuEmissions"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.TotalCPUEmissions = value
			}
		}

		// Update with GPU metrics
		if metric, mExists := aggMetrics["gpuUsage"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.AveGPUUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuMemUsage"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.AveGPUMemUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuEnergyUsage"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.TotalGPUEnergyUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuEmissions"]; mExists {
			if value, exists := metric[job.Jobid]; exists {
				job.TotalGPUEmissions = value
			}
		}
	}
	return jobs
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

	// Check if Status is error
	if data.Status == "error" {
		return nil, fmt.Errorf("error response from TSDB: %v", data)
	}

	// Check if Data exists on response
	if data.Data == nil {
		return nil, fmt.Errorf("TSDB response returned no data: %v", data)
	}

	// Parse data
	var queriedValues = make(map[int64]float64)
	queryData := data.Data.(map[string]interface{})
	if results, exists := queryData["result"]; exists {
		for _, result := range results.([]interface{}) {
			// Check if metric exists on result. If it does, check for jobid and value
			var jobid, value string
			if metric, exists := result.(map[string]interface{})["metric"]; exists {
				if jid, exists := metric.(map[string]interface{})["batchjobid"]; exists {
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
