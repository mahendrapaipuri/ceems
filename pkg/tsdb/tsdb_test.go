package tsdb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
)

func TestNewTSDBWithNoURL(t *testing.T) {
	tsdb, err := NewTSDB("", true, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB instance")
	}
	if tsdb.Available() {
		t.Errorf("Expected TSDB to not available")
	}
}

func TestNewTSDBWithURL(t *testing.T) {
	// Start test server
	expected := "dummy data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expected))
	}))
	defer server.Close()

	tsdb, err := NewTSDB(server.URL, true, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB instance")
	}
	if !tsdb.Available() {
		t.Errorf("Expected TSDB to available")
	}

	// Check if Ping is working
	if err := tsdb.Ping(); err != nil {
		t.Errorf("Could not ping TSDB")
	}
}

func TestTSDBConfigSuccess(t *testing.T) {
	// Start test server
	expected := Response{
		Status: "success",
		Data: map[string]string{
			"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  evaluation_interval: 10s\n  external_labels:\n    environment: prometheus-demo\nalerting:\n  alertmanagers:\n  - follow_redirects: true\n    enable_http2: true\n    scheme: http\n    timeout: 10s\n    api_version: v2\n    static_configs:\n    - targets:\n      - demo.do.prometheus.io:9093\nrule_files:\n- /etc/prometheus/rules/*.rules\nscrape_configs:\n- job_name: prometheus\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:9090\n- job_name: random\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/random.yml\n    refresh_interval: 5m\n- job_name: caddy\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - localhost:2019\n- job_name: grafana\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:3000\n- job_name: node\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/node.yml\n    refresh_interval: 5m\n- job_name: alertmanager\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/alertmanager.yml\n    refresh_interval: 5m\n- job_name: cadvisor\n  honor_timestamps: true\n  track_timestamps_staleness: true\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/cadvisor.yml\n    refresh_interval: 5m\n- job_name: blackbox\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  params:\n    module:\n    - http_2xx\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /probe\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  relabel_configs:\n  - source_labels: [__address__]\n    separator: ;\n    regex: (.*)\n    target_label: __param_target\n    replacement: $1\n    action: replace\n  - source_labels: [__param_target]\n    separator: ;\n    regex: (.*)\n    target_label: instance\n    replacement: $1\n    action: replace\n  - separator: ;\n    regex: (.*)\n    target_label: __address__\n    replacement: 127.0.0.1:9115\n    action: replace\n  static_configs:\n  - targets:\n    - http://localhost:9100\n",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := NewTSDB(server.URL, true, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB instance")
	}

	// Check if Ping is working
	if !tsdb.Available() {
		t.Errorf("Expected TSDB to available")
	}

	// Check global config
	var globalConfig map[interface{}]interface{}
	if globalConfig, err = tsdb.GlobalConfig(); err != nil {
		t.Errorf("Could not get TSDB config: %s", err)
	}
	if v, exists := globalConfig["scrape_interval"]; !exists {
		t.Errorf("scrape_interval expected to exist in config")
	} else {
		if v.(string) != "15s" {
			t.Errorf("expected scrape_interval 15s got %s", v.(string))
		}
	}

	// Check scrape interval
	scrapeInterval := tsdb.Intervals()["scrape_interval"]
	if scrapeInterval != time.Duration(15*time.Second) {
		t.Errorf("expected scrape_interval 15s got %s", scrapeInterval)
	}

	// Check evaluation interval
	evaluationInterval := tsdb.Intervals()["evaluation_interval"]
	if evaluationInterval != time.Duration(10*time.Second) {
		t.Errorf("expected evaluation_interval 10s got %s", evaluationInterval)
	}

	// Check rate interval
	rateInterval := tsdb.RateInterval()
	if rateInterval != time.Duration(60*time.Second) {
		t.Errorf("expected rate_interval 60s got %s", rateInterval)
	}
}

func TestTSDBConfigFail(t *testing.T) {
	// Start test server
	expected := Response{
		Status: "error",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := NewTSDB(server.URL, true, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB instance")
	}
	if !tsdb.Available() {
		t.Errorf("Expected TSDB to available")
	}

	// Check if config is working
	if _, err := tsdb.Config(); err == nil {
		t.Errorf("Expected TSDB config error")
	}

	scrapeInterval := tsdb.Intervals()["scrape_interval"]
	if scrapeInterval != time.Duration(defaultScrapeInterval) {
		t.Errorf("Expected default scrape interval, got %s", scrapeInterval)
	}

	// Check evaluation interval
	evaluationInterval := tsdb.Intervals()["evaluation_interval"]
	if evaluationInterval != time.Duration(defaultEvaluationInterval) {
		t.Errorf("expected default scrape interval got %s", evaluationInterval)
	}

	rateInterval := tsdb.RateInterval()
	if rateInterval != time.Duration(defaultScrapeInterval)*4 {
		t.Errorf("Expected default rate interval, got %s", rateInterval)
	}
}

func TestTSDBQuerySuccess(t *testing.T) {
	// Start test server
	expected := Response{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "vector",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": "1",
					},
					"value": []interface{}{
						12345, "1.1",
					},
				},
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": "2",
					},
					"value": []interface{}{
						12345, "2.2",
					},
				},
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := NewTSDB(server.URL, true, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB instance")
	}
	if !tsdb.Available() {
		t.Errorf("Expected TSDB to available")
	}

	if m, err := tsdb.Query("", time.Now()); err != nil {
		t.Errorf("Expected TSDB query to return value")
	} else {
		if !reflect.DeepEqual(m, Metric{"1": 1.1, "2": 2.2}) {
			t.Errorf("Expected {1: 1.1, 2: 2.2}, got %v", m)
		}
	}
}

func TestTSDBQueryFail(t *testing.T) {
	// Start test server
	expected := Response{
		Status: "error",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := NewTSDB(server.URL, true, log.NewNopLogger())
	if err != nil {
		t.Errorf("Failed to create TSDB instance")
	}
	if !tsdb.Available() {
		t.Errorf("Expected TSDB to available")
	}

	if _, err := tsdb.Query("", time.Now()); err == nil {
		t.Errorf("Expected TSDB query to return error")
	}
}
