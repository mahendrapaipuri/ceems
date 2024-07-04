package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

// Default port Prometheus listens on.
const portNum string = ":9090"

// Regex to capture query
var (
	queryRegex = regexp.MustCompile("^(.*){")
	regexpUUID = regexp.MustCompile("(?:.+?)[^gpu]uuid=[~]{0,1}\"(?P<uuid>[a-zA-Z0-9-|]+)\"(?:.*)")
)

// hash returns hash of a given string
func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// lenLoop returns number of digits in an integer
func lenLoop(i uint32) int {
	if i == 0 {
		return 1
	}
	count := 0
	for i != 0 {
		i /= 10
		count++
	}
	return count
}

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

	// Extract UUIDs from query
	var uuids []string
	uuidMatches := regexpUUID.FindAllStringSubmatch(query, -1)
	for _, match := range uuidMatches {
		if len(match) > 1 {
			for _, uuid := range strings.Split(match[1], "|") {
				// Ignore empty strings
				if strings.TrimSpace(uuid) != "" && !slices.Contains(uuids, uuid) {
					uuids = append(uuids, uuid)
				}
			}
		}
	}

	// Extract only query without any labels
	matches := queryRegex.FindStringSubmatch(query)
	if len(matches) == 2 {
		query = matches[1]
	}

	// log.Println("Query", query, "UUIDs", uuids)

	var results []interface{}
	if slices.Contains(
		[]string{
			"avg_cpu_usage", "avg_cpu_mem_usage", "avg_gpu_usage",
			"avg_gpu_mem_usage", "total_cpu_energy_usage_kwh", "total_gpu_energy_usage_kwh",
			"total_cpu_emissions_gms", "total_gpu_emissions_gms",
		}, query) {
		// Convert uuid into hash and transform that hash number into float64 between 0 and 100
		for _, uuid := range uuids {
			h := hash(uuid)
			numDigits := lenLoop(h)
			value := float64(h) / math.Pow(10, float64(numDigits)-2)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": uuid,
					},
					"value": []interface{}{
						12345, strconv.FormatFloat(value, 'f', -1, 64),
					},
				})
		}
	} else if slices.Contains(
		[]string{
			"total_io_read_stats_bytes", "total_io_write_stats_bytes",
		}, query) {
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": uuid,
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h), 10),
					},
				})
		}
	} else if slices.Contains(
		[]string{
			"total_io_read_stats_requests", "total_io_write_stats_requests",
		}, query) {
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": uuid,
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h)*10, 10),
					},
				})
		}
	} else if slices.Contains(
		[]string{
			"total_ingress_stats_bytes", "total_outgress_stats_bytes",
		}, query) {
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": uuid,
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h)*100, 10),
					},
				})
		}
	} else if slices.Contains(
		[]string{
			"total_ingress_stats_packets", "total_outgress_stats_packets",
		}, query) {
		for _, uuid := range uuids {
			h := hash(uuid)
			results = append(results,
				map[string]interface{}{
					"metric": map[string]string{
						"uuid": uuid,
					},
					"value": []interface{}{
						12345, strconv.FormatUint(uint64(h)*1000, 10),
					},
				})
		}
	}
	// responseResults := filterResults(uuids, esults)
	response = tsdb.Response{
		Status: "success",
		Data: map[string]interface{}{
			"resultType": "vector",
			"result":     results,
		},
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

	// Start server
	server := &http.Server{
		Addr:              portNum,
		ReadHeaderTimeout: 3 * time.Second,
	}

	// Spinning up the server.
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
