package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
)

var gpuSeries = []string{
	"DCGM_FI_DEV_POWER_USAGE_INSTANT",
	"amd_gpu_power",
	"gpu_power_usage",
}

// MetricRelabelConfig contains the Prometheus metric relabel config.
type MetricRelabelConfig struct {
	SourceLabels []string `yaml:"source_labels,omitempty"`
	TargetLabel  string   `yaml:"target_label,omitempty"`
	Regex        string   `yaml:"regex,omitempty"`
	Replacement  string   `yaml:"replacement,omitempty"`
	Action       string   `yaml:"action"`
}

// ScrapeConfig represents Prometheus scrape config.
type ScrapeConfig struct {
	Job            model.LabelValue      `yaml:"job"`
	RelabelConfigs []MetricRelabelConfig `yaml:"metric_relabel_configs"`
}

// PromConfig is container for Prometheus config.
type PromConfig struct {
	ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs"`
}

// CreatePromRelabelConfig generates the necessary relabel config for current Prometheus scrape
// configs.
func CreatePromRelabelConfig(
	ctx context.Context,
	serverURL *url.URL,
	start string,
	end string,
	roundTripper http.RoundTripper,
) error {
	// Parse times
	stime, etime, err := parseTimes(start, end)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing start and/or end time(s):", err)

		return err
	}

	// Make a new API client
	api, err := newAPI(serverURL, roundTripper, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating new API client:", err)

		return err
	}

	// Get necessary job meta data
	activeJobs, jobSeries, _, err := jobSeriesMetaData(ctx, api, stime, etime, gpuSeries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching series label values:", err)

		return err
	}

	// Initialise scrape configs
	var scrapeConfigs []ScrapeConfig

	// Loop over all activeJobs to generate metric_relabel config
	for _, job := range activeJobs {
		var relabelConfigs []MetricRelabelConfig

		switch {
		case slices.Contains(jobSeries[job], model.LabelValue(gpuSeries[0])):
			// GPU UUID and MIG Instance ID
			// Merge with modelName to ensure we match labels only on
			// DCGM exporter series. This way even if we apply relabel
			// config on CEEMS targets, we do not lose gpuuuid and gpuiid
			// labels that we are using
			relabelConfigs = []MetricRelabelConfig{
				{
					SourceLabels: []string{"modelName", "UUID"},
					TargetLabel:  "gpuuuid",
					Regex:        "NVIDIA(.*);(.*)",
					Replacement:  "$2",
					Action:       "replace",
				},
				{
					SourceLabels: []string{"modelName", "GPU_I_ID"},
					TargetLabel:  "gpuiid",
					Regex:        "NVIDIA(.*);(.*)",
					Replacement:  "$2",
					Action:       "replace",
				},
				{
					Regex:  "UUID",
					Action: "labeldrop",
				},
				{
					Regex:  "GPU_I_ID",
					Action: "labeldrop",
				},
			}
		case slices.Contains(jobSeries[job], model.LabelValue(gpuSeries[1])):
			// Seems like AMD SMI exporter using a different label name for each
			// metric series to identify GPU index
			// Having duplicate target_label can be problematic
			// Workaround proposed here: https://stackoverflow.com/questions/70093340/prometheus-multiple-source-label-in-relabel-config
			relabelConfigs = []MetricRelabelConfig{
				{
					SourceLabels: []string{"gpu_power"},
					TargetLabel:  "index",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					SourceLabels: []string{"index", "gpu_use_percent"},
					TargetLabel:  "index",
					Regex:        ";(.+)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					SourceLabels: []string{"index", "gpu_memory_use_percent"},
					TargetLabel:  "index",
					Regex:        ";(.+)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "gpu_power",
					Action: "labeldrop",
				},

				{
					Regex:  "gpu_use_percent",
					Action: "labeldrop",
				},
				{
					Regex:  "gpu_memory_use_percent",
					Action: "labeldrop",
				},
			}
		case slices.Contains(jobSeries[job], model.LabelValue(gpuSeries[2])):
			// Just like DCGM exporter, AMD device metrics exporter
			// exports GPU index as gpu_id and GPU partition ID as
			// gpu_partition_id. We will relabel them to match the
			// CEEMS exporter
			relabelConfigs = []MetricRelabelConfig{
				{
					SourceLabels: []string{"gpu_id"},
					TargetLabel:  "index",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					SourceLabels: []string{"gpu_partition_id"},
					TargetLabel:  "gpuiid",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "gpu_id",
					Action: "labeldrop",
				},
				{
					Regex:  "gpu_partition_id",
					Action: "labeldrop",
				},
			}
		default:
			continue
		}

		scrapeConfigs = append(scrapeConfigs, ScrapeConfig{Job: job, RelabelConfigs: relabelConfigs})
	}

	// Encode to YAML with indent set to 2
	var b bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&b)
	yamlEncoder.SetIndent(2)

	if err := yamlEncoder.Encode(&PromConfig{ScrapeConfigs: scrapeConfigs}); err != nil {
		fmt.Fprintln(os.Stderr, "error encoding scrape_configs", err)

		return err
	}

	fmt.Fprintln(os.Stderr, "Merge the following scrape_configs with the current config.")
	fmt.Fprintln(os.Stderr, b.String())

	return nil
}
