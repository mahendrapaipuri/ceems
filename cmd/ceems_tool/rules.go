package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/emissions"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Embed the rules directory.
//
//go:embed rules
var rulesFS embed.FS

var (
	seriesNames = []string{
		"ceems_compute_unit_cpu_user_seconds_total",
		"ceems_compute_unit_memory_used_bytes",
		"ceems_rapl_package_joules_total",
		"ceems_rapl_dram_joules_total",
		"ceems_ipmi_dcmi_current_watts",
		"ceems_redfish_current_watts",
		"ceems_cray_pm_counters_power_watts",
		"ceems_emissions_gCo2_kWh",
		"DCGM_FI_DEV_POWER_USAGE_INSTANT",
		"amd_gpu_power",
		"gpu_power_usage", // AMD metrics device exporter
		"ceems_compute_unit_gpu_index_flag",
		"ceems_compute_unit_gpu_sm_count",
	}

	nvidiaProfSeriesNames = []string{
		"DCGM_FI_PROF_SM_ACTIVE",
		"DCGM_FI_PROF_SM_OCCUPANCY",
		"DCGM_FI_PROF_GR_ENGINE_ACTIVE",
		"DCGM_FI_PROF_PIPE_TENSOR_ACTIVE",
		"DCGM_FI_PROF_PIPE_FP64_ACTIVE",
		"DCGM_FI_PROF_PIPE_FP32_ACTIVE",
		"DCGM_FI_PROF_PIPE_FP16_ACTIVE",
		"DCGM_FI_PROF_DRAM_ACTIVE",
		"DCGM_FI_PROF_NVLINK_TX_BYTES",
		"DCGM_FI_PROF_NVLINK_RX_BYTES",
		"DCGM_FI_PROF_PCIE_TX_BYTES",
		"DCGM_FI_PROF_PCIE_RX_BYTES",
	}
)

// Config represents Prometheus config.
type Config struct {
	Global struct {
		ScrapeInterval     model.Duration `yaml:"scrape_interval"`
		EvaluationInterval model.Duration `yaml:"evaluation_interval"`
	} `yaml:"global"`
}

type gpuTemplateData struct {
	templateFile     string
	powerSeries      model.LabelValue
	powerScaler      int64
	powerInHostPower bool
	job              model.LabelValue
	nvProfSeries     model.LabelValues
}

type EmissionFactor struct {
	Provider string
	Value    float64
}

// rulesTemplateData contains data to be used inside templates.
type rulesTemplateData struct {
	GPU                *gpuTemplateData
	TemplateFile       string
	HostPowerSeries    string
	RAPLAvailable      bool
	Job                model.LabelValue
	PUE                float64
	EmissionFactor     EmissionFactor
	Providers          model.LabelValues
	Chassis            model.LabelValue
	CountryCode        string
	RateInterval       string
	EvaluationInterval string
}

func (t *rulesTemplateData) GPUPowerInHostPower() bool {
	if t.GPU == nil {
		return false
	}

	return t.GPU.powerInHostPower
}

func (t *rulesTemplateData) GPUPowerSeries() model.LabelValue {
	if t.GPU == nil {
		return ""
	}

	return t.GPU.powerSeries
}

func (t *rulesTemplateData) GPUPowerScaler() int64 {
	if t.GPU == nil {
		return 1
	}

	return t.GPU.powerScaler
}

func (t *rulesTemplateData) GPUJob() model.LabelValue {
	if t.GPU == nil {
		return ""
	}

	return t.GPU.job
}

func (t *rulesTemplateData) NVProfSeries() model.LabelValues {
	if t.GPU == nil {
		return nil
	}

	return t.GPU.nvProfSeries
}

// CreatePromRecordingRules generates CEEMS specific recording rules for Prometheus.
func CreatePromRecordingRules(
	ctx context.Context,
	serverURL *url.URL,
	start string,
	end string,
	pueValue float64,
	emissionFactorValue float64,
	countryCode string,
	evalInterval time.Duration,
	outDir string,
	disableProviders bool,
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

	// Get Prom's config
	config, err := config(ctx, api)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching config:", err)

		return err
	}

	// Get scrape intervals
	jobScrapeIntervals, err := scrapeIntervals(ctx, api)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching scrape intervals:", err)

		return err
	}

	// Use default evaluation interval when not provided
	if evalInterval == 0 {
		evalInterval = time.Duration(config.Global.EvaluationInterval)
	}

	// Get available emission factor providers
	var emissionFactor EmissionFactor

	var providers model.LabelValues

	if emissionFactorValue == 0 {
		// If no emission factor value has been passed, attempt to get from time series or
		// static OWID data
		providers, err = efProviders(ctx, api, stime, etime, countryCode, disableProviders)
		if err != nil {
			if owid, err := emissions.NewOWIDProvider(slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
				if owidData, err := owid.Update(); err == nil {
					emissionFactor = EmissionFactor{Provider: "owid", Value: owidData[countryCode].Factor}

					fmt.Fprintln(os.Stderr, "static emission factor", emissionFactor.Value, "g/kWh from OWID data will be used")
				}
			}
		}
	} else {
		emissionFactor = EmissionFactor{Provider: "custom", Value: emissionFactorValue}
	}

	// Get necessary job meta data
	activeJobs, jobSeries, gpuJobMap, err := jobSeriesMetaData(ctx, api, stime, etime, append(seriesNames, nvidiaProfSeriesNames...))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching series label values:", err)

		return err
	}

	// Assert prof series into model.Values
	var nvProfSeries model.LabelValues
	for _, s := range nvidiaProfSeriesNames {
		nvProfSeries = append(nvProfSeries, model.LabelValue(s))
	}

	// Create a new template and output director
	tmpl, err := newTemplate(outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating template and/or output directory:", err)

		return err
	}

	// Loop over all the active jobs and generate templates
	for _, job := range activeJobs {
		// Get correct template file
		var tmplFile string

		var hostPowerSeries string

		switch {
		case slices.Contains(jobSeries[job], "ceems_cray_pm_counters_power_watts"):
			tmplFile = "cpu-cray.rules"
			hostPowerSeries = "ceems_cray_pm_counters_power_watts"
		case slices.Contains(jobSeries[job], "ceems_redfish_current_watts"):
			tmplFile = "cpu-ipmi-redfish.rules"
			hostPowerSeries = "ceems_redfish_current_watts"
		case slices.Contains(jobSeries[job], "ceems_ipmi_dcmi_current_watts"):
			tmplFile = "cpu-ipmi-redfish.rules"
			hostPowerSeries = "ceems_ipmi_dcmi_current_watts"
		case slices.Contains(jobSeries[job], "ceems_rapl_package_joules_total"):
			tmplFile = "cpu-rapl.rules"
			hostPowerSeries = "ceems_rapl_package_joules_total"
		default:
			continue
		}

		fmt.Fprintln(os.Stderr, "generating recording rules for job", job, "in file", job+".rules")

		// For redfish power usage counter, get all the possible chassis
		var targetChassis model.LabelValue

		var hostPowerLabel string

		if hostPowerSeries == "ceems_redfish_current_watts" {
			matcher := fmt.Sprintf(`ceems_redfish_current_watts{job="%s"}`, job)

			chassis, _, err := api.LabelValues(ctx, "chassis", []string{matcher}, stime, etime) // Ignoring warnings for now.
			if err != nil {
				fmt.Fprintln(os.Stderr, "job:", job, "error fetching redfish chassis values:", err)

				return err
			}

			// If there are more than 1 chassis, emit log for operators to tell them to
			// choose appropriate chassis to get CPU power usage
			if len(chassis) > 1 {
				fmt.Fprintln(os.Stderr, "Multiple chassis found for ceems_redfish_current_watts for job", job)
				fmt.Fprintln(os.Stderr, "Choose the chassis that reports host power usage")

				for ichas, chas := range chassis {
					msg := fmt.Sprintf("[%d]: %s", ichas, chas)
					fmt.Fprintln(os.Stderr, msg)
				}

				// Read input from user
				var input string

				fmt.Fprintln(os.Stderr, "Enter number between 0 and", len(chassis)-1)

				if _, err = fmt.Scanln(&input); err != nil {
					fmt.Fprintln(os.Stderr, "failed to scan user input:", err)

					return err
				}

				// Convert user response to int
				idx, err := strconv.Atoi(input)
				if err != nil {
					fmt.Fprintln(os.Stderr, "invalid user input:", err)

					return err
				}

				// Check whether user input is valid
				if idx >= len(chassis) {
					fmt.Fprintln(os.Stderr, "user input out of range. Must be between 0 and", len(chassis)-1)

					return errors.New("user input out of range")
				}

				targetChassis = chassis[idx]
			} else if len(chassis) == 1 {
				targetChassis = chassis[0]
			} else {
				fmt.Fprintln(os.Stderr, "no chassis found for ceems_redfish_current_watts for job", job)

				return errors.New("no chassis found for ceems_redfish_current_watts")
			}

			// If targetChassis is found, set up label
			if targetChassis != "" {
				hostPowerLabel = fmt.Sprintf(",chassis=\"%s\"", targetChassis)
			}
		}

		// Check if GPUs are present on the hosts and get GPU related template data
		gpu := gpuData(ctx, api, stime, etime, hostPowerSeries, hostPowerLabel, job, nvProfSeries, gpuJobMap, jobSeries)

		// Use a rate interval that is atleast 4 times of scrape interval
		rateInterval := 4 * time.Duration(config.Global.ScrapeInterval)
		if val, ok := jobScrapeIntervals[string(job)]; ok {
			rateInterval = 4 * val
		}

		// Template data
		tmplData := &rulesTemplateData{
			GPU:                gpu,
			TemplateFile:       tmplFile,
			HostPowerSeries:    hostPowerSeries,
			RAPLAvailable:      slices.Contains(jobSeries[job], "ceems_rapl_package_joules_total") && slices.Contains(jobSeries[job], "ceems_rapl_dram_joules_total"),
			Job:                job,
			Chassis:            targetChassis,
			PUE:                pueValue,
			EmissionFactor:     emissionFactor,
			Providers:          providers,
			CountryCode:        countryCode,
			RateInterval:       rateInterval.String(),
			EvaluationInterval: evalInterval.String(),
		}

		// Render templates
		if err := renderRules(tmpl, tmplData, outDir); err != nil {
			fmt.Fprintln(os.Stderr, "job:", job, "error executing rules template:", err)

			continue
		}
	}

	return nil
}

// scrapeIntervals returns scrape interval for each Prom job.
func scrapeIntervals(ctx context.Context, api v1.API) (map[string]time.Duration, error) {
	// Run query to get jobs and their scrape intervals.
	targets, err := api.Targets(ctx)
	if err != nil {
		return nil, err
	}

	// Get all the job scrape intervals
	scrapeIntervals := make(map[string]time.Duration)

	for _, target := range targets.Active {
		scrapeInt, err := time.ParseDuration(target.DiscoveredLabels["__scrape_interval__"])
		if err != nil {
			fmt.Fprintln(os.Stderr, "target:", target, "error parsing scrape duration value:", err)

			continue
		}

		scrapeIntervals[target.DiscoveredLabels["job"]] = scrapeInt
	}

	return scrapeIntervals, nil
}

// efProviders returns a slice of available emission factor providers.
func efProviders(ctx context.Context, api v1.API, start time.Time, end time.Time, countryCode string, disableProviders bool) (model.LabelValues, error) {
	// Run query to get label values.
	matcher := fmt.Sprintf(`ceems_emissions_gCo2_kWh{country_code="%s"}`, countryCode)

	providers, _, err := api.LabelValues(ctx, "provider", []string{matcher}, start, end) // Ignoring warnings for now.
	if err != nil {
		return nil, err
	}

	// If no providers are found, exit
	if len(providers) == 0 || disableProviders {
		return nil, fmt.Errorf("no providers found for country code: %s", countryCode)
	}

	return providers, nil
}

// jobSeriesMetaData returns necessary metadata related to Prom job's series.
func jobSeriesMetaData(ctx context.Context, api v1.API, start time.Time, end time.Time, series []string) (model.LabelValues, map[model.LabelValue]model.LabelValues, map[model.LabelValue]model.LabelValue, error) {
	// Run query to get matching series.
	foundSeries, _, err := api.Series(ctx, series, start, end) // Ignoring warnings for now.
	if err != nil {
		return nil, nil, nil, err
	}

	// Make a map of job to instances
	jobInstances := make(map[model.LabelValue]model.LabelValues)
	jobSeries := make(map[model.LabelValue]model.LabelValues)
	seriesJobs := make(map[model.LabelValue]model.LabelValues)

	var activeJobs model.LabelValues

	for _, s := range foundSeries {
		// If instance is of form host:port, strip port from instance
		instance := model.LabelValue(strings.Split(string(s["instance"]), ":")[0])

		if !slices.Contains(jobInstances[s["job"]], instance) {
			jobInstances[s["job"]] = append(jobInstances[s["job"]], instance)
		}

		if !slices.Contains(jobSeries[s["job"]], s["__name__"]) {
			jobSeries[s["job"]] = append(jobSeries[s["job"]], s["__name__"])
		}

		if !slices.Contains(seriesJobs[s["__name__"]], s["job"]) {
			seriesJobs[s["__name__"]] = append(seriesJobs[s["__name__"]], s["job"])
		}

		if !slices.Contains(activeJobs, s["job"]) {
			activeJobs = append(activeJobs, s["job"])
		}
	}

	// GPU jobs corresponding to CEEMS jobs map
	// Here we find the corresponding GPU job that has same instances as CEEMS job.
	// We need this info when constructing rules for GPU metrics as we need GPU mapper
	// from CEEMS exporter to match with metric from GPU (DCGM/AMD) exporter.
	gpuJobsMap := make(map[model.LabelValue]model.LabelValue)

	for _, cpuJob := range seriesJobs["ceems_compute_unit_gpu_index_flag"] {
		// Look for NVIDIA GPU associations
		for _, gpuJob := range seriesJobs["DCGM_FI_DEV_POWER_USAGE_INSTANT"] {
			// If job instances between CEEMS job and GPU job matches, we mark it as an association
			if foundInstances := intersection(jobInstances[gpuJob], jobInstances[cpuJob]); len(foundInstances) > 0 {
				gpuJobsMap[cpuJob] = gpuJob
			}
		}

		// Look for AMD GPU associations with AMD SMI exporter
		for _, gpuJob := range seriesJobs["amd_gpu_power"] {
			// If job instances between CEEMS job and GPU job matches, we mark it as an association
			if foundInstances := intersection(jobInstances[gpuJob], jobInstances[cpuJob]); len(foundInstances) > 0 {
				gpuJobsMap[cpuJob] = gpuJob
			}
		}

		// Look for AMD GPU associations with AMD device metrics exporter
		for _, gpuJob := range seriesJobs["gpu_power_usage"] {
			// If job instances between CEEMS job and GPU job matches, we mark it as an association
			if foundInstances := intersection(jobInstances[gpuJob], jobInstances[cpuJob]); len(foundInstances) > 0 {
				gpuJobsMap[cpuJob] = gpuJob
			}
		}
	}

	return activeJobs, jobSeries, gpuJobsMap, nil
}

// newTemplate creates a new template and new output directory to store templated files.
func newTemplate(outDir string) (*template.Template, error) {
	// Custom functions
	funcMap := template.FuncMap{
		"ToUpper": strings.ToUpper,
		"ToLower": strings.ToLower,
		"Split": func(s, sep string) []string {
			return strings.Split(s, sep)
		},
		"Contains": func(s model.LabelValues, e string) bool {
			return slices.Contains(s, model.LabelValue(e))
		},
	}

	// Make a new template
	// Testing on playground: https://goplay.tools/snippet/xx5CbUWBR27
	tmpl, err := template.New("rules").Funcs(funcMap).ParseFS(rulesFS, "rules/*.rules")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing rules template:", err)

		return nil, err
	}

	// Make directory to store recording rules files
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "error creating output directory:", err)

		return nil, err
	}

	return tmpl, nil
}

// gpuData returns the template related data for GPUs.
func gpuData(
	ctx context.Context,
	api v1.API,
	stime time.Time,
	etime time.Time,
	hostPowerSeries string,
	hostPowerLabel string,
	job model.LabelValue,
	nvProfSeries model.LabelValues,
	gpuJobMap map[model.LabelValue]model.LabelValue,
	jobSeries map[model.LabelValue]model.LabelValues,
) *gpuTemplateData {
	// If there is no GPUs on the instances of current job, return
	if _, ok := gpuJobMap[job]; !ok {
		return nil
	}

	// Get GPU job name associated with current job
	gpu := &gpuTemplateData{
		job: gpuJobMap[job],
	}

	// Based on GPU type get Get GPU power series name and template file name
	switch {
	case slices.Contains(jobSeries[gpu.job], "DCGM_FI_DEV_POWER_USAGE_INSTANT"):
		gpu.powerSeries = "DCGM_FI_DEV_POWER_USAGE_INSTANT"
		gpu.powerScaler = 1
		gpu.templateFile = "gpu-nvidia.rules"

		// For NVIDIA GPUs check if prof metrics are available
		gpu.nvProfSeries = intersection(jobSeries[gpu.job], nvProfSeries)
	case slices.Contains(jobSeries[gpu.job], "gpu_power_usage"):
		gpu.powerSeries = "gpu_power_usage"
		gpu.powerScaler = 1
		gpu.templateFile = "gpu-amd-device-metrics.rules"
	default:
		gpu.powerSeries = "amd_gpu_power"
		gpu.powerScaler = 1e6
		gpu.templateFile = "gpu-amd-smi.rules"
	}

	// If host power series is cray, we dont need to check if GPU power is in host power
	// Cray exposes all components separately
	if hostPowerSeries == "ceems_cray_pm_counters_power_watts" {
		return gpu
	}

	// Check if host power includes GPU power or not
	query := fmt.Sprintf(
		`avg_over_time((label_replace(%s{job="%s"%s}, "instancehost", "$1", "instance", "([^:]+):\\d+") - on (instancehost) group_left () sum by (instancehost) (label_replace(%s{job="%s"} / %d, "instancehost", "$1", "instance","([^:]+):\\d+")))[%s:])`,
		hostPowerSeries, job, hostPowerLabel, gpu.powerSeries, gpu.job, gpu.powerScaler, etime.Sub(stime).Truncate(time.Minute).String(),
	)

	// Make query against Prometheus
	if result, _, err := api.Query(ctx, query, etime); err == nil {
		// If average value is more than 0, that means Host power includes GPU power
		if val, ok := result.(model.Vector); ok && len(val) > 0 {
			if val[0].Value > 0 {
				gpu.powerInHostPower = true
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "failed to verify if host power reported by", hostPowerSeries, "for job", job, "includes GPU power. Please make manual check and modify rule appropriately. Error is:", err)
	}

	return gpu
}

// renderRules generates recording rules by rendering template files.
func renderRules(tmpl *template.Template, tmplData *rulesTemplateData, outDir string) error {
	// Render the CPU rules template
	buf := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(buf, tmplData.TemplateFile, tmplData); err != nil {
		return err
	}

	// Write to CPU recording rules to file
	path := filepath.Join(outDir, fmt.Sprintf("%s.rules", tmplData.Job))
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return err
	}

	// If there is GPU related template data, we need to render recording rules for GPU
	if tmplData.GPU != nil {
		fmt.Fprintln(os.Stderr, "generating recording rules for GPU for job", tmplData.GPU.job, "in file", tmplData.GPU.job+"-gpu.rules")

		buf := &bytes.Buffer{}
		if err := tmpl.ExecuteTemplate(buf, tmplData.GPU.templateFile, tmplData); err != nil {
			return err
		}

		// Write to CPU recording rules to file
		path := filepath.Join(outDir, fmt.Sprintf("%s-gpu.rules", tmplData.GPU.job))
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			return err
		}
	}

	return nil
}
