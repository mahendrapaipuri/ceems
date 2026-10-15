package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
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

	"github.com/ceems-dev/ceems/pkg/emissions"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// Embed the rules directory.
//
//go:embed rules
var rulesFS embed.FS

const (
	ipmiPowerMetric    = "ceems_ipmi_dcmi_power_current_watts"
	redfishPowerMetric = "ceems_redfish_power_current_watts"
	crayPowerMetric    = "ceems_cray_pm_counters_power_watts"
	hwmonPowerMetric   = "ceems_hwmon_power_current_watts"
)

const (
	dcgmInstantPowerMetric       = "DCGM_FI_DEV_POWER_USAGE_INSTANT"
	dcgmPowerMetric              = "DCGM_FI_DEV_POWER_USAGE"
	amdSMIPowerMetric            = "amd_gpu_power"
	amdDevExporterPkgPowerMetric = "gpu_package_power"
)

const (
	emissionsMetric = "ceems_emissions_gCo2_kWh"
)

var (
	seriesNames = []string{
		"ceems_compute_unit_cpu_user_seconds_total",
		"ceems_compute_unit_memory_used_bytes",
		"ceems_rapl_package_joules_total",
		"ceems_rapl_dram_joules_total",
		ipmiPowerMetric,
		redfishPowerMetric,
		crayPowerMetric,
		hwmonPowerMetric,
		emissionsMetric,
		dcgmPowerMetric,
		dcgmInstantPowerMetric,
		amdSMIPowerMetric,
		amdDevExporterPkgPowerMetric, // AMD metrics device exporter
		"ceems_compute_unit_gpu_index_flag",
		"ceems_compute_unit_gpu_sm_count",
		"ceems_ebpf_read_bytes_total",
		"ceems_ebpf_write_bytes_total",
		"ceems_ebpf_ingress_bytes_total",
		"ceems_ebpf_egress_bytes_total",
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

	amdDevProfSeriesNames = []string{
		"gpu_prof_sm_active",
		"gpu_prof_occupancy_elapsed",
		"gpu_prof_occupancy_per_active_cu",
		"gpu_prof_tensor_active_percent",
		"gpu_prof_occupancy_percent",
		"gpu_prof_total_16_ops",
		"gpu_prof_total_32_ops",
		"gpu_prof_total_64_ops",
		"gpu_prof_write_size",
		"gpu_prof_fetch_size",
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
	templateFile  string
	powerMetric   string
	metricPrefix  string
	job           model.LabelValue
	nvProfSeries  model.LabelValues
	amdProfSeries model.LabelValues
}

type EmissionFactor struct {
	Provider string
	Value    float64
}

// rulesTemplateData contains data to be used inside templates.
type rulesTemplateData struct {
	GPU                *gpuTemplateData
	TemplateFile       string
	HostPowerQuery     string
	HostPowerSeries    string
	RAPLAvailable      bool
	IOAvailable        bool
	NetAvailable       bool
	Job                model.LabelValue
	PUE                float64
	EmissionFactor     EmissionFactor
	Providers          model.LabelValues
	CountryCode        string
	Instance           string
	RateInterval       string
	EvaluationInterval string
}

func (t *rulesTemplateData) GPUMetricPrefix() string {
	if t.GPU == nil {
		return ""
	}

	return t.GPU.metricPrefix
}

func (t *rulesTemplateData) GPUPowerMetric() string {
	if t.GPU == nil {
		return ""
	}

	return t.GPU.powerMetric
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

func (t *rulesTemplateData) AMDProfSeries() model.LabelValues {
	if t.GPU == nil {
		return nil
	}

	return t.GPU.amdProfSeries
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

	var emissionCollectorInstance string

	if emissionFactorValue == 0 {
		// If no emission factor value has been passed, attempt to get from time series or
		// static OWID data
		providers, err = efProviders(ctx, api, stime, etime, countryCode, disableProviders)
		if err != nil {
			owid, err := emissions.NewOWIDProvider(slog.New(slog.DiscardHandler))
			if err == nil {
				owidData, err := owid.Update()
				if err == nil {
					emissionFactor = EmissionFactor{Provider: "owid", Value: owidData[countryCode].Factor}

					fmt.Fprintln(os.Stderr, "static emission factor", emissionFactor.Value, "g/kWh from OWID data will be used")
				}
			}
		}

		// Get instance of emission collector
		emissionCollectorInstance = efInstance(ctx, api, stime, etime)
	} else {
		emissionFactor = EmissionFactor{Provider: "custom", Value: emissionFactorValue}
	}

	// Get necessary job meta data
	series := append(seriesNames, append(nvidiaProfSeriesNames, amdDevProfSeriesNames...)...)

	activeJobs, jobSeries, gpuJobMap, err := jobSeriesMetaData(ctx, api, stime, etime, series)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching series label values:", err)

		return err
	}

	// Assert prof series into model.Values
	var nvProfSeries, amdProfSeries model.LabelValues
	for _, s := range nvidiaProfSeriesNames {
		nvProfSeries = append(nvProfSeries, model.LabelValue(s))
	}

	for _, s := range amdDevProfSeriesNames {
		amdProfSeries = append(amdProfSeries, model.LabelValue(s))
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
		case slices.Contains(jobSeries[job], crayPowerMetric):
			tmplFile = "cpu-cray.rules"
			hostPowerSeries = crayPowerMetric
		case slices.Contains(jobSeries[job], redfishPowerMetric):
			tmplFile = "cpu-ipmi-redfish-hwmon.rules"
			hostPowerSeries = redfishPowerMetric
		case slices.Contains(jobSeries[job], ipmiPowerMetric):
			tmplFile = "cpu-ipmi-redfish-hwmon.rules"
			hostPowerSeries = ipmiPowerMetric
		case slices.Contains(jobSeries[job], hwmonPowerMetric):
			tmplFile = "cpu-ipmi-redfish-hwmon.rules"
			hostPowerSeries = hwmonPowerMetric
		case slices.Contains(jobSeries[job], "ceems_rapl_package_joules_total"):
			tmplFile = "cpu-rapl.rules"
			hostPowerSeries = "ceems_rapl_package_joules_total"
		default:
			continue
		}

		fmt.Fprintln(os.Stderr, "generating recording rules for job", job, "in file", job+".rules")

		// For redfish power usage counter, get all the possible chassis
		var hostPowerLabelName, hostPowerLabel string

		switch hostPowerSeries {
		case redfishPowerMetric:
			hostPowerLabelName = "chassis"

			targetChassis, err := findTargetLabel(ctx, api, redfishPowerMetric, hostPowerLabelName, job, stime, etime)
			if err != nil {
				fmt.Fprintln(os.Stderr, "job:", job, "error fetching redfish target chassis values:", err)

				return err
			}

			// If targetChassis is found, set up label
			if targetChassis != nil {
				if len(targetChassis) > 1 {
					hostPowerLabel = fmt.Sprintf(",%s=~\"%s\"", hostPowerLabelName, strings.Join(targetChassis, "|"))
				} else {
					hostPowerLabel = fmt.Sprintf(",%s=\"%s\"", hostPowerLabelName, targetChassis[0])
				}
			}
		case hwmonPowerMetric:
			hostPowerLabelName = "chip"

			targetChips, err := findTargetLabel(ctx, api, hwmonPowerMetric, hostPowerLabelName, job, stime, etime)
			if err != nil {
				fmt.Fprintln(os.Stderr, "job:", job, "error fetching hwmon target chip values:", err)

				return err
			}

			// If targetChassis is found, set up label
			if targetChips != nil {
				if len(targetChips) > 1 {
					hostPowerLabel = fmt.Sprintf(",%s=~\"%s\"", hostPowerLabelName, strings.Join(targetChips, "|"))
				} else {
					hostPowerLabel = fmt.Sprintf(",%s=\"%s\"", hostPowerLabelName, targetChips[0])
				}
			}

			// Overwrite chip to sensor as one chip can have multiple sensors and we need to sum over all of them
			hostPowerLabelName = "sensor"
		}

		// Host power query
		var hostPowerQuery string

		if hostPowerLabel != "" {
			hostPowerQuery = fmt.Sprintf(`sum without (%s) (%s{job="%s"%s})`, hostPowerLabelName, hostPowerSeries, job, hostPowerLabel)
		} else {
			hostPowerQuery = fmt.Sprintf(`%s{job="%s"%s}`, hostPowerSeries, job, hostPowerLabel)
		}

		var gpu *gpuTemplateData

		// Check if GPUs are present on the hosts and get GPU related template data if there
		// is a GPU job corresponding to current job
		if gpuJob, ok := gpuJobMap[job]; ok {
			gpu, hostPowerQuery = gpuData(ctx, api, stime, etime, hostPowerQuery, job, gpuJob, nvProfSeries, amdProfSeries, jobSeries)
		}

		// Use a rate interval that is atleast 4 times of scrape interval
		rateInterval := 4 * time.Duration(config.Global.ScrapeInterval)
		if val, ok := jobScrapeIntervals[string(job)]; ok {
			rateInterval = 4 * val
		}

		// Template data
		tmplData := &rulesTemplateData{
			GPU:                gpu,
			TemplateFile:       tmplFile,
			HostPowerQuery:     hostPowerQuery,
			HostPowerSeries:    hostPowerSeries,
			RAPLAvailable:      slices.Contains(jobSeries[job], "ceems_rapl_package_joules_total") && slices.Contains(jobSeries[job], "ceems_rapl_dram_joules_total"),
			IOAvailable:        slices.Contains(jobSeries[job], "ceems_ebpf_read_bytes_total") || slices.Contains(jobSeries[job], "ceems_ebpf_write_bytes_total"),
			NetAvailable:       slices.Contains(jobSeries[job], "ceems_ebpf_ingress_bytes_total") || slices.Contains(jobSeries[job], "ceems_ebpf_egress_bytes_total"),
			Job:                job,
			PUE:                pueValue,
			EmissionFactor:     emissionFactor,
			Providers:          providers,
			CountryCode:        countryCode,
			Instance:           emissionCollectorInstance,
			RateInterval:       rateInterval.String(),
			EvaluationInterval: evalInterval.String(),
		}

		// Render templates
		err := renderRules(tmpl, tmplData, outDir)
		if err != nil {
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

// efInstance returns first found instance of emission factor metric. In case when operators run emissions collector on
// multiple instances, it comes handy.
func efInstance(ctx context.Context, api v1.API, start time.Time, end time.Time) string {
	// First fetch all the available series of emissions
	series, _, _, err := api.Series(ctx, []string{emissionsMetric}, start, end)
	if err != nil || len(series) == 0 {
		return ""
	}

	var instances []string
	for _, s := range series {
		instances = append(instances, string(s["instance"]))
	}

	// Remove duplicates and if more than one instance is found, it means multiple
	// instances of emissions collectors are running which is not ideal. Emit a warning
	instances = slices.Compact(instances)
	slices.Sort(instances)

	if len(instances) > 1 {
		fmt.Fprintln(os.Stderr, `WARNING: multiple instances of emissions collectors are detected. The generated recording rules might not work in this case. Ensure there is only one single instance of emissions collector running in the cluster.`)
	}

	return instances[0]
}

// efProviders returns a slice of available emission factor providers.
func efProviders(ctx context.Context, api v1.API, start time.Time, end time.Time, countryCode string, disableProviders bool) (model.LabelValues, error) {
	// Run query to get label values.
	matcher := fmt.Sprintf(`%s{country_code="%s"}`, emissionsMetric, countryCode)

	providers, _, _, err := api.LabelValues(ctx, "provider", []string{matcher}, start, end) // Ignoring warnings for now.
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
	// We might not have exact series names so make them regex matchable
	seriesMatches := make([]string, len(series))

	for is, s := range series {
		seriesMatches[is] = fmt.Sprintf(`{__name__=~"(.*)%s(.*)"}`, s)
	}

	// Run query to get matching series.
	foundSeries, _, _, err := api.Series(ctx, seriesMatches, start, end) // Ignoring warnings for now.
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

		// A special case for AMD device metrics exporter where metric label can have
		// variable prefix
		if strings.Contains(string(s["__name__"]), amdDevExporterPkgPowerMetric) {
			if !slices.Contains(seriesJobs[amdDevExporterPkgPowerMetric], s["job"]) {
				seriesJobs[amdDevExporterPkgPowerMetric] = append(seriesJobs[amdDevExporterPkgPowerMetric], s["job"])
			}
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
		// Look for NVIDIA GPU associations. First check for instant power metric and if
		// it is not enabled, check for regular power metrics which is averaged
		if gpuJobs, ok := seriesJobs[dcgmInstantPowerMetric]; ok {
			for _, gpuJob := range gpuJobs {
				// If job instances between CEEMS job and GPU job matches, we mark it as an association
				if foundInstances := intersection(jobInstances[gpuJob], jobInstances[cpuJob]); len(foundInstances) > 0 {
					gpuJobsMap[cpuJob] = gpuJob
				}
			}
		} else {
			for _, gpuJob := range seriesJobs[dcgmPowerMetric] {
				// If job instances between CEEMS job and GPU job matches, we mark it as an association
				if foundInstances := intersection(jobInstances[gpuJob], jobInstances[cpuJob]); len(foundInstances) > 0 {
					gpuJobsMap[cpuJob] = gpuJob
				}
			}
		}

		// Look for AMD GPU associations with AMD SMI exporter
		for _, gpuJob := range seriesJobs[amdSMIPowerMetric] {
			// If job instances between CEEMS job and GPU job matches, we mark it as an association
			if foundInstances := intersection(jobInstances[gpuJob], jobInstances[cpuJob]); len(foundInstances) > 0 {
				gpuJobsMap[cpuJob] = gpuJob
			}
		}

		// Look for AMD GPU associations with AMD device metrics exporter
		for _, gpuJob := range seriesJobs[amdDevExporterPkgPowerMetric] {
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
		"IsSubString": func(s, sub string) bool {
			return strings.Contains(s, sub)
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
	err = os.MkdirAll(outDir, 0o700)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating output directory:", err)

		return nil, err
	}

	return tmpl, nil
}

// findTargetLabel returns the target label when multiple labels found on metric.
func findTargetLabel(ctx context.Context, api v1.API, metricName string, labelName string, job model.LabelValue, stime time.Time, etime time.Time) ([]string, error) {
	matcher := fmt.Sprintf(`%s{job="%s"}`, metricName, job)

	labels, _, _, err := api.LabelValues(ctx, labelName, []string{matcher}, stime, etime) // Ignoring warnings for now.
	if err != nil {
		fmt.Fprintln(os.Stderr, "job:", job, "error fetching", labelName, "values:", err)

		return nil, err
	}

	var targetLabels []string

	// If there are more than 1 chassis, emit log for operators to tell them to
	// choose appropriate chassis to get CPU power usage
	if len(labels) > 1 {
		fmt.Fprintln(os.Stderr, "Multiple", labelName, "found for", metricName, "for job", job)
		fmt.Fprintln(os.Stderr, "Choose the", labelName, "that reports host power usage")

		for ichas, chas := range labels {
			msg := fmt.Sprintf("[%d]: %s", ichas, chas)
			fmt.Fprintln(os.Stderr, msg)
		}

		// Read input from user
		var inputs string

		fmt.Fprintln(os.Stderr, "Enter number(s) between 0 and", len(labels)-1)
		fmt.Fprintln(os.Stderr, "Multiple labels can be selected by using comma separated list of numbers, e.g., 0,1")

		_, err = fmt.Scanln(&inputs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to scan user input:", err)

			return nil, err
		}

		for input := range strings.SplitSeq(inputs, ",") {
			// Convert user response to int
			idx, err := strconv.Atoi(input)
			if err != nil {
				fmt.Fprintln(os.Stderr, "invalid user input:", err)

				return nil, err
			}

			// Check whether user input is valid
			if idx >= len(labels) {
				fmt.Fprintln(os.Stderr, "user input out of range. Must be between 0 and", len(labels)-1)

				return nil, errors.New("user input out of range")
			}

			targetLabels = append(targetLabels, string(labels[idx]))
		}
	} else if len(labels) == 1 {
		targetLabels = []string{string(labels[0])}
	} else {
		fmt.Fprintln(os.Stderr, "no", labelName, "found for", metricName, "for job", job)

		return nil, fmt.Errorf("no %s found for %s", labelName, redfishPowerMetric)
	}

	return targetLabels, nil
}

// gpuData returns the template related data for GPUs.
func gpuData(
	ctx context.Context,
	api v1.API,
	stime time.Time,
	etime time.Time,
	hostPowerQuery string,
	job model.LabelValue,
	gpuJob model.LabelValue,
	nvProfSeries model.LabelValues,
	amdProfSeries model.LabelValues,
	jobSeries map[model.LabelValue]model.LabelValues,
) (*gpuTemplateData, string) {
	var hostPowerOnlyQuery string

	// Instantiate GPU template data
	gpu := &gpuTemplateData{
		job: gpuJob,
	}

	// Based on GPU type get Get GPU power series name and template file name
	//
	// Get labels of unique devices on each node. In case of NVIDIA GPU partitions, power consumption
	// metric will be duplicated for each partition and so we should only take into account
	// the power consumption of physical devices.
	//
	// However, in the case of AMD GPUs, power usage is reported only for first partition
	// and rest of partitions will have power usage reported as zero.
	//
	// Also, we noticed that in the case of AMD GPUs, device metrics exporter when deployed
	// using GPU operator, it reported gpu_power_usage as zero for all GPUs where as
	// gpu_package_power reported correct GPU power usage. However, when device metrics
	// exporter is installed via system package manager such as apt, gpu_power_usage
	// was reporting correct power usage.
	// AMD device exporter allows to add a prefix to metric names and we should figure
	// out that prefix as well for the recording rules.
	switch {
	case slices.Contains(jobSeries[gpu.job], amdSMIPowerMetric):
		gpu.templateFile = "gpu-amd-smi.rules"

		// Host power query assuming GPU power is in host power
		// We dont know if AMD SMI exporter duplicates power consumption for all partitions or reports
		// usage only for first partiition and rest as zero like in AMD device metrics exporter. We assume
		// the behaviour is same as the AMD device exporter for the moment.
		hostPowerOnlyQuery = fmt.Sprintf(
			`(%s - on (hostname) group_left () sum by (hostname) (label_replace(sum by (hostname) (%s{job="%s"}) / 1e6, "hostname", "$1", "instance","([^:]+):\\d+")))`,
			hostPowerQuery, amdSMIPowerMetric, gpu.job,
		)
	case slices.Contains(jobSeries[gpu.job], dcgmInstantPowerMetric):
		gpu.templateFile = "gpu-nvidia.rules"
		gpu.powerMetric = dcgmInstantPowerMetric

		// Host power query assuming GPU power is in host power
		hostPowerOnlyQuery = fmt.Sprintf(
			`(%s - on (hostname) group_left () sum by (hostname) (avg by (hostname,device) (label_replace(%s{job="%s"}, "hostname", "$1", "Hostname","(.*)"))))`,
			hostPowerQuery, dcgmInstantPowerMetric, gpu.job,
		)

		// For NVIDIA GPUs check if prof metrics are available
		gpu.nvProfSeries = intersection(jobSeries[gpu.job], nvProfSeries)
	case slices.Contains(jobSeries[gpu.job], dcgmPowerMetric):
		gpu.templateFile = "gpu-nvidia.rules"
		gpu.powerMetric = dcgmPowerMetric

		// Host power query assuming GPU power is in host power
		hostPowerOnlyQuery = fmt.Sprintf(
			`(%s - on (hostname) group_left () sum by (hostname) (avg by (hostname,device) (label_replace(%s{job="%s"}, "hostname", "$1", "Hostname","(.*)"))))`,
			hostPowerQuery, dcgmPowerMetric, gpu.job,
		)

		// For NVIDIA GPUs check if prof metrics are available
		gpu.nvProfSeries = intersection(jobSeries[gpu.job], nvProfSeries)
	default:
		gpu.templateFile = "gpu-amd-device-metrics.rules"

		// Default case is that we are using AMD device metrics exporter. In this case, first we need
		// to figure out metric prefix if there is any
		for _, metric := range jobSeries[gpu.job] {
			if strings.Contains(string(metric), amdDevExporterPkgPowerMetric) {
				if p := strings.Split(string(metric), amdDevExporterPkgPowerMetric); len(p) == 2 {
					gpu.metricPrefix = p[0]

					break
				}
			}
		}

		// Host power query assuming GPU power is in host power
		hostPowerOnlyQuery = fmt.Sprintf(
			`(%s - on (hostname) group_left () sum by (hostname) (sum by (hostname,serial_number) (%s%s{job="%s"})))`,
			hostPowerQuery, gpu.metricPrefix, amdDevExporterPkgPowerMetric, gpu.job,
		)

		// Prof series names with prefix
		var amdProfSeriesPrefix model.LabelValues
		for _, n := range amdProfSeries {
			amdProfSeriesPrefix = append(amdProfSeriesPrefix, model.LabelValue(gpu.metricPrefix+string(n)))
		}

		// For AMD GPUs check if prof metrics are available
		gpu.amdProfSeries = intersection(jobSeries[gpu.job], amdProfSeriesPrefix)
	}

	// If host power series is cray, we dont need to check if GPU power is in host power
	// Cray exposes all components separately
	if strings.Contains(hostPowerQuery, crayPowerMetric) {
		return gpu, hostPowerQuery
	}

	// Check if host power includes GPU power or not
	query := fmt.Sprintf(`avg_over_time(%s[%s:])`, hostPowerOnlyQuery, etime.Sub(stime).Truncate(time.Minute).String())

	// Make query against Prometheus
	result, _, _, err := api.Query(ctx, query, etime)
	if err == nil {
		// If average value is more than 0, that means Host power includes GPU power
		if val, ok := result.(model.Vector); ok && len(val) > 0 {
			if val[0].Value > 0 {
				return gpu, hostPowerOnlyQuery
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "failed to verify if host power reported by", hostPowerQuery, "for job", job, "includes GPU power. Please make manual check and modify rule appropriately. Error is:", err)
	}

	return gpu, hostPowerQuery
}

// renderRules generates recording rules by rendering template files.
func renderRules(tmpl *template.Template, tmplData *rulesTemplateData, outDir string) error {
	// Render the CPU rules template
	buf := &bytes.Buffer{}

	err := tmpl.ExecuteTemplate(buf, tmplData.TemplateFile, tmplData)
	if err != nil {
		return err
	}

	// Write to CPU recording rules to file
	path := filepath.Join(outDir, fmt.Sprintf("%s.rules", tmplData.Job))

	err = os.WriteFile(path, buf.Bytes(), 0o600)
	if err != nil {
		return err
	}

	// If there is GPU related template data, we need to render recording rules for GPU
	if tmplData.GPU != nil {
		fmt.Fprintln(os.Stderr, "generating recording rules for GPU for job", tmplData.GPU.job, "in file", tmplData.GPU.job+"-gpu.rules")

		buf := &bytes.Buffer{}

		err := tmpl.ExecuteTemplate(buf, tmplData.GPU.templateFile, tmplData)
		if err != nil {
			return err
		}

		// Write to CPU recording rules to file
		path := filepath.Join(outDir, fmt.Sprintf("%s-gpu.rules", tmplData.GPU.job))

		err = os.WriteFile(path, buf.Bytes(), 0o600)
		if err != nil {
			return err
		}
	}

	return nil
}
