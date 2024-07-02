package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

// Default port Prometheus listens on.
const portNum string = ":9090"

// QueryHandler handles queries
func QueryHandler(w http.ResponseWriter, r *http.Request) {
	var response tsdb.Response
	var query string
	switch r.Method {
	case "GET":
		query = r.URL.Query()["query"][0]
	case "POST":
		// Call ParseForm() to parse the raw query and update r.PostForm and r.Form.
		if err := r.ParseForm(); err != nil {
			http.Error(w, "ParseForm error", http.StatusInternalServerError)
			return
		}
		query = r.FormValue("query")
	default:
		http.Error(w, "Only GET and POST are allowed", http.StatusForbidden)
		return
	}

	log.Println("Query", query)

	if slices.Contains(
		[]string{
			"avg_cpu_usage", "avg_cpu_mem_usage", "avg_gpu_usage",
			"avg_gpu_mem_usage", "total_cpu_energy_usage_kwh", "total_gpu_energy_usage_kwh",
			"total_cpu_emissions_gms", "total_gpu_emissions_gms",
		}, query) {
		response = tsdb.Response{
			Status: "success",
			Data: map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1479763",
						},
						"value": []interface{}{
							12345, "14.79",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1481508",
						},
						"value": []interface{}{
							12345, "14.58",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "147975",
						},
						"value": []interface{}{
							12345, "14.79",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "11508",
						},
						"value": []interface{}{
							12345, "11.50",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "81510",
						},
						"value": []interface{}{
							12345, "81.51",
						},
					},
				},
			},
		}
	} else if slices.Contains(
		[]string{
			"total_io_read_stats_bytes", "total_io_write_stats_bytes",
		}, query) {
		response = tsdb.Response{
			Status: "success",
			Data: map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1479763",
						},
						"value": []interface{}{
							12345, "1479763",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1481508",
						},
						"value": []interface{}{
							12345, "1481508",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "147975",
						},
						"value": []interface{}{
							12345, "147975",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "11508",
						},
						"value": []interface{}{
							12345, "11508",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "81510",
						},
						"value": []interface{}{
							12345, "81510",
						},
					},
				},
			},
		}
	} else if slices.Contains(
		[]string{
			"total_io_read_stats_requests", "total_io_write_stats_requests",
		}, query) {
		response = tsdb.Response{
			Status: "success",
			Data: map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1479763",
						},
						"value": []interface{}{
							12345, "14797630",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1481508",
						},
						"value": []interface{}{
							12345, "14815080",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "147975",
						},
						"value": []interface{}{
							12345, "1479750",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "11508",
						},
						"value": []interface{}{
							12345, "115080",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "81510",
						},
						"value": []interface{}{
							12345, "815100",
						},
					},
				},
			},
		}
	} else if slices.Contains(
		[]string{
			"total_ingress_stats_bytes", "total_outgress_stats_bytes",
		}, query) {
		response = tsdb.Response{
			Status: "success",
			Data: map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1479763",
						},
						"value": []interface{}{
							12345, "147976300",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1481508",
						},
						"value": []interface{}{
							12345, "148150800",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "147975",
						},
						"value": []interface{}{
							12345, "14797500",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "11508",
						},
						"value": []interface{}{
							12345, "1150800",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "81510",
						},
						"value": []interface{}{
							12345, "8151000",
						},
					},
				},
			},
		}
	} else if slices.Contains(
		[]string{
			"total_ingress_stats_packets", "total_outgress_stats_packets",
		}, query) {
		response = tsdb.Response{
			Status: "success",
			Data: map[string]interface{}{
				"resultType": "vector",
				"result": []interface{}{
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1479763",
						},
						"value": []interface{}{
							12345, "1479763000",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "1481508",
						},
						"value": []interface{}{
							12345, "1481508000",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "147975",
						},
						"value": []interface{}{
							12345, "147975000",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "11508",
						},
						"value": []interface{}{
							12345, "11508000",
						},
					},
					map[string]interface{}{
						"metric": map[string]string{
							"uuid": "81510",
						},
						"value": []interface{}{
							12345, "81510000",
						},
					},
				},
			},
		}
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

// ConfigHandler handles Promtheus config
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	response := tsdb.Response{
		Status: "success",
		Data: map[string]string{
			"yaml": "global:\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  evaluation_interval: 10s\n  external_labels:\n    environment: prometheus-demo\nalerting:\n  alertmanagers:\n  - follow_redirects: true\n    enable_http2: true\n    scheme: http\n    timeout: 10s\n    api_version: v2\n    static_configs:\n    - targets:\n      - demo.do.prometheus.io:9093\nrule_files:\n- /etc/prometheus/rules/*.rules\nscrape_configs:\n- job_name: prometheus\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:9090\n- job_name: random\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/random.yml\n    refresh_interval: 5m\n- job_name: caddy\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - localhost:2019\n- job_name: grafana\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  static_configs:\n  - targets:\n    - demo.do.prometheus.io:3000\n- job_name: node\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/node.yml\n    refresh_interval: 5m\n- job_name: alertmanager\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/alertmanager.yml\n    refresh_interval: 5m\n- job_name: cadvisor\n  honor_timestamps: true\n  track_timestamps_staleness: true\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /metrics\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  file_sd_configs:\n  - files:\n    - /etc/prometheus/file_sd/cadvisor.yml\n    refresh_interval: 5m\n- job_name: blackbox\n  honor_timestamps: true\n  track_timestamps_staleness: false\n  params:\n    module:\n    - http_2xx\n  scrape_interval: 15s\n  scrape_timeout: 10s\n  scrape_protocols:\n  - OpenMetricsText1.0.0\n  - OpenMetricsText0.0.1\n  - PrometheusText0.0.4\n  metrics_path: /probe\n  scheme: http\n  enable_compression: true\n  follow_redirects: true\n  enable_http2: true\n  relabel_configs:\n  - source_labels: [__address__]\n    separator: ;\n    regex: (.*)\n    target_label: __param_target\n    replacement: $1\n    action: replace\n  - source_labels: [__param_target]\n    separator: ;\n    regex: (.*)\n    target_label: instance\n    replacement: $1\n    action: replace\n  - separator: ;\n    regex: (.*)\n    target_label: __address__\n    replacement: 127.0.0.1:9115\n    action: replace\n  static_configs:\n  - targets:\n    - http://localhost:9100\n",
		},
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		w.Write([]byte("KO"))
	}
}

func main() {
	log.Println("Starting fake prometheus server")

	// Registering our handler functions, and creating paths.
	http.HandleFunc("/api/v1/query", QueryHandler)
	http.HandleFunc("/api/v1/status/config", ConfigHandler)

	log.Println("Started on port", portNum)
	fmt.Println("To close connection CTRL+C :-)")

	// Spinning up the server.
	err := http.ListenAndServe(portNum, nil)
	if err != nil {
		log.Fatal(err)
	}
}
