package tsdb

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWithNoURL(t *testing.T) {
	tsdb, err := New("", config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.False(t, tsdb.Available())
}

func TestNewWithURL(t *testing.T) {
	// Start test server
	expected := "dummy data"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expected))
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	// Check if Ping is working
	assert.NoError(t, tsdb.Ping())
}

func TestTSDBConfigSuccess(t *testing.T) {
	// Start test server
	expected := Response[any]{
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Check if Ping is working
	assert.True(t, tsdb.Available())

	// Check global config
	var globalConfig map[string]interface{}
	globalConfig, err = tsdb.GlobalConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "15s", globalConfig["scrape_interval"].(string)) //nolint:forcetypeassert

	// Reduce cache TTL to test concurrent update
	tsdb.settingsCacheTTL = time.Millisecond

	time.Sleep(2 * time.Millisecond)

	expectedSettings := Settings{
		ScrapeInterval:     15 * time.Second,
		EvaluationInterval: 10 * time.Second,
		RateInterval:       60 * time.Second,
		QueryLookbackDelta: defaultLookbackDelta,
		QueryTimeout:       defaultQueryTimeout,
		QueryMaxSamples:    defaultQueryMaxSamples,
	}

	wg := sync.WaitGroup{}

	// Get settings from concurrent go routines
	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			settings := tsdb.Settings(ctx)
			assert.Equal(t, expectedSettings, *settings)
		}()
	}

	wg.Wait()
}

func TestTSDBFlagsSuccess(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"alertmanager.notification-queue-capacity":  10000,
			"alertmanager.timeout":                      "",
			"auto-gomemlimit.ratio":                     0.9,
			"config.file":                               "/etc/prometheus/prometheus.yml",
			"enable-feature":                            "promql-at-modifier,promql-negative-offset",
			"log.format":                                "logfmt",
			"log.level":                                 "info",
			"query.lookback-delta":                      "5m",
			"query.max-concurrency":                     20,
			"query.max-samples":                         50000000,
			"query.timeout":                             "2m",
			"rules.alert.for-grace-period":              "10m",
			"rules.alert.for-outage-tolerance":          "1h",
			"rules.alert.resend-delay":                  "1m",
			"rules.max-concurrent-evals":                4,
			"scrape.adjust-timestamps":                  true,
			"scrape.discovery-reload-interval":          "5s",
			"scrape.timestamp-tolerance":                "2ms",
			"storage.agent.no-lockfile":                 false,
			"storage.agent.path":                        "data-agent/",
			"storage.agent.retention.max-time":          "0s",
			"storage.agent.retention.min-time":          "0s",
			"storage.agent.wal-compression":             true,
			"storage.agent.wal-compression-type":        "snappy",
			"storage.agent.wal-segment-size":            "0B",
			"storage.agent.wal-truncate-frequency":      "0s",
			"storage.remote.flush-deadline":             "1m",
			"storage.remote.read-concurrent-limit":      10,
			"storage.remote.read-max-bytes-in-frame":    1048576,
			"storage.remote.read-sample-limit":          50000000,
			"storage.tsdb.allow-overlapping-blocks":     true,
			"storage.tsdb.head-chunks-write-queue-size": "0",
			"storage.tsdb.max-block-chunk-segment-size": "0B",
			"storage.tsdb.max-block-duration":           "3d2h24m",
			"storage.tsdb.min-block-duration":           "2h",
			"storage.tsdb.no-lockfile":                  false,
			"storage.tsdb.path":                         "/var/lib/prometheus",
			"storage.tsdb.retention":                    "0s",
			"storage.tsdb.retention.size":               "0B",
			"storage.tsdb.retention.time":               "31d",
			"storage.tsdb.samples-per-chunk":            120,
			"storage.tsdb.wal-compression":              true,
			"storage.tsdb.wal-compression-type":         "snappy",
			"storage.tsdb.wal-segment-size":             "0B",
			"web.config.file":                           "/etc/prometheus/web.yml",
			"web.console.libraries":                     "/etc/prometheus/console_libraries",
			"web.console.templates":                     "/etc/prometheus/consoles",
			"web.cors.origin":                           ".*",
			"web.enable-admin-api":                      false,
			"web.enable-lifecycle":                      false,
			"web.enable-remote-write-receiver":          false,
			"web.external-url":                          "http://demo.do.prometheus.io:9090",
			"web.listen-address":                        "0.0.0.0:9090",
			"web.max-connections":                       512,
			"web.page-title":                            "Prometheus Time Series Collection and Processing Server",
			"web.read-timeout":                          "5m",
			"web.route-prefix":                          "/",
			"web.user-assets":                           "",
			"write-documentation":                       false,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	// Check if Ping is working
	assert.True(t, tsdb.Available())

	// Check flags
	var flags map[string]interface{}
	flags, err = tsdb.Flags(ctx)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:9090", flags["web.listen-address"].(string)) //nolint:forcetypeassert
	assert.InEpsilon(t, 512, flags["web.max-connections"].(float64), 0)   //nolint:forcetypeassert
	assert.False(t, flags["write-documentation"].(bool))                  //nolint:forcetypeassert
}

func TestTSDBConfigFail(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "error",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if config is working
	_, err = tsdb.Config(ctx)
	require.Error(t, err)

	// Expected settings with default values
	expectedSettings := Settings{
		ScrapeInterval:     defaultScrapeInterval,
		EvaluationInterval: defaultEvaluationInterval,
		RateInterval:       defaultScrapeInterval * 4,
		QueryLookbackDelta: defaultLookbackDelta,
		QueryTimeout:       defaultQueryTimeout,
		QueryMaxSamples:    defaultQueryMaxSamples,
	}

	settings := tsdb.Settings(ctx)
	assert.Equal(t, expectedSettings, *settings)
}

func TestTSDBSeriesSuccess(t *testing.T) {
	// Start test server
	expected := Response[[]Series]{
		Status: "success",
		Data: []Series{
			{
				Name:     "up",
				Job:      "prom",
				Instance: "localhost:9090",
			},
			{
				Name:     "up",
				Job:      "node",
				Instance: "localhost:9091",
			},
			{
				Name:     "process_start_time_seconds",
				Job:      "prom",
				Instance: "localhost:9090",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.Series(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, expected.Data, m)
}

func TestTSDBSeriesFail(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "error",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.Series(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	assert.Error(t, err)
}

func TestTSDBLabelsSuccess(t *testing.T) {
	// Start test server
	expected := Response[[]string]{
		Status: "success",
		Data: []string{
			"job", "instance", "__name__",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.Labels(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, expected.Data, m)
}

func TestTSDBLabelsFail(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "error",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.Labels(context.Background(), []string{"up", "process_start_time_seconds"}, time.Time{}, time.Time{})
	assert.Error(t, err)
}

func TestTSDBQuerySuccess(t *testing.T) {
	// Start test server
	expected := Response[any]{
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

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.Query(context.Background(), "", time.Now())
	require.NoError(t, err)
	assert.Equal(t, Metric{"1": 1.1, "2": 2.2}, m)
}

func TestTSDBQueryFail(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "error",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.Query(context.Background(), "", time.Now())
	assert.Error(t, err)
}

func TestTSDBQueryRangeSuccess(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "matrix",
			"result": []interface{}{
				map[string]interface{}{
					"metric": map[string]string{
						"__name__": "up",
						"instance": "localhost:9090",
					},
					"values": []interface{}{
						[]interface{}{1727367964.929, "1"},
						[]interface{}{1727368964.929, "1"},
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

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	m, err := tsdb.RangeQuery(context.Background(), "", time.Now(), time.Now(), "300")
	require.NoError(t, err)
	assert.Equal(
		t,
		RangeMetric{
			"up": []interface{}{[]interface{}{1.727367964929e+09, "1"}, []interface{}{1.727368964929e+09, "1"}},
		},
		m,
	)
}

func TestTSDBQueryRangeFail(t *testing.T) {
	// Start test server
	expected := Response[any]{
		Status: "error",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	_, err = tsdb.RangeQuery(context.Background(), "", time.Now(), time.Now(), "300")
	assert.Error(t, err)
}

func TestTSDBDeleteSuccess(t *testing.T) {
	// Start test server
	expected := []string{"metric1", "metric2"}

	var got []string

	var err error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err = r.ParseForm(); err != nil {
			w.Write([]byte("KO"))

			return
		}

		got = r.Form["match[]"]

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	err = tsdb.Delete(context.Background(), time.Now(), time.Now(), expected)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, got)
}

func TestTSDBDeleteFail(t *testing.T) {
	// Start test server
	expected := []string{"metric1", "metric2"}

	var err error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err = r.ParseForm(); err != nil {
			w.Write([]byte("KO"))

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tsdb, err := New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	assert.True(t, tsdb.Available())

	err = tsdb.Delete(context.Background(), time.Now(), time.Now(), expected)
	require.Error(t, err)
}
