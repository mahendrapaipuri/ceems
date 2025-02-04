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
	"DCGM_FI_DEV_POWER_USAGE",
	"amd_gpu_power",
}

// RelabelConfig contains the Prometheus relabel config.
type RelabelConfig struct {
	SourceLabels []string `yaml:"source_labels,omitempty"`
	TargetLabel  string   `yaml:"target_label,omitempty"`
	Regex        string   `yaml:"regex,omitempty"`
	Replacement  string   `yaml:"replacement,omitempty"`
	Action       string   `yaml:"action"`
}

// ScrapeConfig represents Prometheus scrape config.
type ScrapeConfig struct {
	Job            model.LabelValue `yaml:"job"`
	RelabelConfigs []RelabelConfig  `yaml:"relabel_configs"`
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
	roundTripper http.RoundTripper,
) error {
	// Make a new API client
	api, err := newAPI(serverURL, roundTripper, nil)
	if err != nil {
		return err
	}

	// Get necessary job meta data
	activeJobs, jobSeries, _, err := jobSeriesMetaData(ctx, api, gpuSeries)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching series label values:", err)

		return err
	}

	// Initialise scrape configs
	var scrapeConfigs []ScrapeConfig

	// Loop over all activeJobs to generate metric_relabel config
	for _, job := range activeJobs {
		var relabelConfigs []RelabelConfig

		switch {
		case slices.Contains(jobSeries[job], model.LabelValue(gpuSeries[0])):
			// GPU UUID and MIG Instance ID
			relabelConfigs = []RelabelConfig{
				{
					SourceLabels: []string{"UUID"},
					TargetLabel:  "gpuuuid",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "UUID",
					Action: "labeldrop",
				},
				{
					SourceLabels: []string{"GPU_I_ID"},
					TargetLabel:  "gpuiid",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "GPU_I_ID",
					Action: "labeldrop",
				},
			}
		case slices.Contains(jobSeries[job], model.LabelValue(gpuSeries[1])):
			// Seems like AMD SMI exporter using a different label name for each
			// metric series to identify GPU index
			relabelConfigs = []RelabelConfig{
				{
					SourceLabels: []string{"gpu_power"},
					TargetLabel:  "index",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "gpu_power",
					Action: "labeldrop",
				},
				{
					SourceLabels: []string{"gpu_use_percent"},
					TargetLabel:  "index",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "gpu_use_percent",
					Action: "labeldrop",
				},
				{
					SourceLabels: []string{"gpu_memory_use_percent"},
					TargetLabel:  "index",
					Regex:        "(.*)",
					Replacement:  "$1",
					Action:       "replace",
				},
				{
					Regex:  "gpu_memory_use_percent",
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
