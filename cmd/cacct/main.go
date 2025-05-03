package main

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	http_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"gopkg.in/yaml.v3"
)

// Locations where config file must be found.
var (
	configPaths = []string{
		"/etc/ceems",
	}

	// CLI app.
	cacctApp = kingpin.New(
		filepath.Base(os.Args[0]), "Energy/Emissions/Performance/Usage data for all jobs fetched from CEEMS database.",
	).UsageWriter(os.Stdout)

	// mock user and config paths for test.
	mockCurrentUser, mockConfigPath string

	fieldMap = map[string]*field{
		"jobid": {
			tag:   "uuid",
			name:  "jobID",
			help:  "Job ID",
			title: "Job ID",
			minW:  3,
			maxW:  7,
		},
		"name": {
			tag:   "name",
			name:  "Name",
			help:  "Name of the job",
			title: "Name",
			minW:  5,
			maxW:  8,
		},
		"account": {
			tag:   "project",
			name:  "Account",
			help:  "Account name",
			title: "Account",
			minW:  5,
			maxW:  8,
		},
		"group": {
			tag:   "groupname",
			name:  "group",
			help:  "Group name",
			title: "Group",
			minW:  5,
			maxW:  5,
		},
		"user": {
			tag:   "username",
			name:  "user",
			help:  "User name",
			title: "User",
			minW:  5,
			maxW:  5,
		},
		"createdat": {
			tag:   "created_at",
			name:  "createdAt",
			help:  "Job creation time",
			title: "Created",
			minW:  5,
			maxW:  8,
		},
		"startedat": {
			tag:   "started_at",
			name:  "startedAt",
			help:  "Job start time",
			title: "Started",
			minW:  5,
			maxW:  12,
		},
		"endedat": {
			tag:   "ended_at",
			name:  "endedAt",
			help:  "Job end time",
			title: "Ended",
			minW:  5,
			maxW:  12,
		},
		"elapsed": {
			tag:   "elapsed",
			name:  "elapsed",
			help:  "Job elapsed time",
			title: "Elapsed",
			minW:  5,
			maxW:  12,
		},
		"state": {
			tag:   "state",
			name:  "state",
			help:  "Job state",
			title: "State",
			minW:  5,
			maxW:  5,
		},
		"cpuusage": {
			tag:   "avg_cpu_usage",
			name:  "cpuUsage",
			help:  "Average CPU usage over the duration of the job",
			title: "CPU Usage(%)",
			minW:  5,
			maxW:  6,
		},
		"cpumemoryusage": {
			tag:   "avg_cpu_mem_usage",
			name:  "cpuMemoryUsage",
			help:  "Average CPU memory usage over the duration of the job",
			title: "CPU Mem. Usage(%)",
			minW:  5,
			maxW:  6,
		},
		"hostenergy": {
			tag:   "total_cpu_energy_usage_kwh",
			name:  "hostEnergy",
			help:  "Total energy usage by the host duration of the job",
			title: "Host Energy(kWh)",
			minW:  5,
			maxW:  8,
		},
		"hostemissions": {
			tag:   "total_cpu_emissions_gms",
			name:  "hostEmissions",
			help:  "Total eq. emissions due to host energy usage duration of the job",
			title: "Host Emissions(gms)",
			minW:  5,
			maxW:  12,
		},
		"gpuusage": {
			tag:   "avg_gpu_usage",
			name:  "gpuUsage",
			help:  "Average GPU(s) usage over the duration of the job",
			title: "GPU Usage(%)",
			minW:  5,
			maxW:  6,
		},
		"gpumemoryusage": {
			tag:   "avg_gpu_mem_usage",
			name:  "gpuMemoryUsage",
			help:  "Average GPU(s) memory usage over the duration of the job",
			title: "GPU Mem. Usage(%)",
			minW:  5,
			maxW:  6,
		},
		"gpuenergy": {
			tag:   "total_gpu_energy_usage_kwh",
			name:  "gpuEnergy",
			help:  "Total energy usage by the GPU(s) duration of the job",
			title: "GPU Energy(kWh)",
			minW:  5,
			maxW:  8,
		},
		"gpuemissions": {
			tag:   "total_gpu_emissions_gms",
			name:  "gpuEmissions",
			help:  "Total eq. emissions due to GPU(s) energy usage duration of the job",
			title: "GPU Emissions(gms)",
			minW:  5,
			maxW:  12,
		},
	}

	allFields = []string{
		"jobid",
		"name",
		"account",
		"group",
		"user",
		"createdat",
		"startedat",
		"endedat",
		"elapsed",
		"state",
		"cpuusage",
		"cpumemoryusage",
		"hostenergy",
		"hostemissions",
		"gpuusage",
		"gpumemoryusage",
		"gpuenergy",
		"gpuemissions",
	}

	defaultFields = []string{
		"jobid",
		"account",
		"elapsed",
		"cpuusage",
		"cpumemoryusage",
		"hostenergy",
		"hostemissions",
		"gpuusage",
		"gpumemoryusage",
		"gpuenergy",
		"gpuemissions",
	}
)

// Custom errors.
var (
	errNoPerm   = errors.New("forbidden response from API server")
	errInternal = errors.New("internal server error")
)

// field is a container for each field metadata in the table.
type field struct {
	tag   string
	name  string
	help  string
	title string
	keys  []string
	minW  int
	maxW  int
}

// titles return header titles for the table.
func (f field) titles() []interface{} {
	if len(f.keys) <= 1 {
		return []interface{}{f.title}
	}

	t := make([]interface{}, len(f.keys))
	for i := range len(f.keys) {
		t[i] = f.title
	}

	return t
}

// subtitles return header subtitles for the table.
func (f field) subtitles() []interface{} {
	if len(f.keys) <= 1 {
		return []interface{}{""}
	}

	t := make([]interface{}, len(f.keys))
	for i, k := range f.keys {
		t[i] = k
	}

	return t
}

// // Default TSDB queries.
// var (
// 	defaultQueries = map[string]string{
// 		"cpu_usage":         `uuid:ceems_cpu_usage:ratio_irate{uuid=~"%s"}`,
// 		"cpu_mem_usage":     `uuid:ceems_cpu_memory_usage:ratio{uuid=~"%s"}`,
// 		"host_power_usage":  `uuid:ceems_host_power_watts:pue{uuid=~"%s"}`,
// 		"host_emissions":    `uuid:ceems_host_emissions_g_s:pue{uuid=~"%s"}`,
// 		"avg_gpu_usage":     `uuid:ceems_gpu_usage:ratio{uuid=~"%s"}`,
// 		"avg_gpu_mem_usage": `uuid:ceems_gpu_memory_usage:ratio{uuid=~"%s"}`,
// 		"gpu_power_usage":   `uuid:ceems_gpu_power_watts:pue{uuid=~"%s"}`,
// 		"gpu_emissions":     `uuid:ceems_gpu_emissions_g_s:pue{uuid=~"%s"}`,
// 		"io_read_bytes":     `irate(ceems_ebpf_read_bytes_total{uuid=~"%s"}[1m])`,
// 		"io_write_bytes":    `irate(ceems_ebpf_write_bytes_total{uuid=~"%s"}[1m])`,
// 	}
// )

// Config contains the cacct configuration settings.
type Config struct {
	API struct {
		Web            WebConfig `yaml:"web"`
		ClusterID      string    `yaml:"cluster_id"`
		UserHeaderName string    `yaml:"user_header_name"`
	} `yaml:"ceems_api_server"`
	TSDB struct {
		Web     WebConfig         `yaml:"web"`
		Queries map[string]string `yaml:"queries"`
	} `yaml:"tsdb"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	*c = Config{}
	c.API.UserHeaderName = "X-Grafana-User"
	// c.TSDB.Queries = defaultQueries

	type plain Config

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return nil
}

// WebConfig contains HTTP related config.
type WebConfig struct {
	URL              string                       `yaml:"url"`
	HTTPClientConfig http_config.HTTPClientConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (w *WebConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	*w = WebConfig{}
	w.HTTPClientConfig = http_config.DefaultHTTPClientConfig

	type plain WebConfig

	if err := unmarshal((*plain)(w)); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := w.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return nil
}

// Response defines the response model of CEEMSAPIServer.
type Response[T any] struct {
	Status   string   `json:"status"`
	Data     []T      `json:"data"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

func main() {
	var (
		tsData, helpFormat, longFormat    bool
		htmlOut, csvOut, mdOut            bool
		tsDataOut                         string
		accountsFlag, jobsFlag, usersFlag string
		formatFlag                        string
		startTime, endTime                string
	)

	cacctApp.Version(version.Print("caact"))
	cacctApp.HelpFlag.Short('h')

	// CLI flags
	cacctApp.Flag(
		"account", "Comma separated list of account to select jobs to display. By default, all accounts are selected.",
	).StringVar(&accountsFlag)
	cacctApp.Flag(
		"starttime", "Select jobs eligible after this time. Valid format is YYYY-MM-DD[THH:MM[:SS]] (default: 00:00:00 of the current day).",
	).Default(time.Now().Format("2006-01-02") + "T00:00:00").StringVar(&startTime)
	cacctApp.Flag(
		"endtime", "Select jobs eligible before this time. Valid format is YYYY-MM-DD[THH:MM[:SS]] (default: now).",
	).Default(time.Now().Format("2006-01-02T15:04:05")).StringVar(&endTime)
	cacctApp.Flag(
		"job", "Comma separated list of jobs to display information. Default is all jobs in the period.",
	).StringVar(&jobsFlag)
	cacctApp.Flag(
		"user", "Comma separated list of user names to select jobs to display. By default, the running user is used.",
	).StringVar(&usersFlag)
	cacctApp.Flag(
		"format", "Comma separated list of fields (Use --helpformat for list of available fields).",
	).StringVar(&formatFlag)
	cacctApp.Flag(
		"helpformat", "List of available fields.",
	).Default("false").BoolVar(&helpFormat)
	cacctApp.Flag(
		"long", fmt.Sprintf("Equivalent to specifying --format=\"%s\".", strings.Join(allFields, ",")),
	).Default("false").BoolVar(&longFormat)
	cacctApp.Flag(
		"ts", "Time series data of jobs are saved in CSV format (default: false).",
	).BoolVar(&tsData)
	cacctApp.Flag(
		"ts.out-dir", "Directory to save time series data.",
	).Default("out").StringVar(&tsDataOut)
	cacctApp.Flag(
		"csv", "Produce CSV output (default: false).",
	).Default("false").BoolVar(&csvOut)
	cacctApp.Flag(
		"html", "Produce HTML output (default: false).",
	).Default("false").BoolVar(&htmlOut)
	cacctApp.Flag(
		"markdown", "Produce markdown output (default: false).",
	).Default("false").BoolVar(&mdOut)

	if _, err := cacctApp.Parse(os.Args[1:]); err != nil {
		kingpin.Fatalf("failed to parse CLI flags: %v", err)
	}

	// If helpformat, print available fields and return
	if helpFormat {
		// First collect keys and sort them
		keys := sortedKeys(fieldMap)

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Field", "Description"})

		for _, k := range keys {
			t.AppendRow(table.Row{fieldMap[k].name, fieldMap[k].help})
		}

		t.Render()

		os.Exit(0)
	}

	// Convert flags to slices
	accounts := splitString(accountsFlag, ",")
	jobs := splitString(jobsFlag, ",")
	userNames := splitString(usersFlag, ",")

	// Get format fields
	formatFields := splitString(formatFlag, ",")
	if len(formatFields) == 0 {
		formatFields = defaultFields
	}

	// If long format is asked, use all fields
	if longFormat {
		formatFields = allFields
	}

	var fields []string
	for _, f := range formatFields {
		fields = append(fields, fieldMap[strings.ToLower(f)].tag)
	}

	// Always add started and ended ts fields as we will need them for TSDB data retrieval
	fields = append(fields, []string{"started_at_ts", "ended_at_ts"}...)

	// Ensure --job flag is passed when asking for metric data
	// This is to avoid fetching metrics of too many jobs when only
	// period is set
	if tsData && len(jobs) == 0 {
		kingpin.Fatalf("explicit job IDs must be passed using --job when --ts is enabled")
	}

	// Convert start and end times to time.Time
	var start, end time.Time

	var err error
	if start, err = parseTime(startTime); err != nil {
		kingpin.Fatalf("failed to parse --starttime flag: %v", err)
	}

	if end, err = parseTime(endTime); err != nil {
		kingpin.Fatalf("failed to parse --endtime flag: %v", err)
	}

	// Get current user and add user's config dir to slice of config
	// dirs.
	// If current user is root and mockCurrentUser and/or mockConfigPath
	// are set, we override the actual with mock ones. Only used in testing
	// and it should not affect production cases.
	currentUser, err := getCurrentUser(mockCurrentUser, mockConfigPath)
	if err != nil {
		os.Exit(checkErr(fmt.Errorf("failed to get current user: %w", err)))
	}

	// Check if currentUser is only user in userNames and if so, set userNames to nil
	if len(userNames) == 1 && userNames[0] == currentUser {
		userNames = nil
	}

	// Get stats
	units, usages, err := stats(currentUser, start, end, accounts, jobs, userNames, fields, tsData, tsDataOut)
	if err != nil {
		os.Exit(checkErr(err))
	}

	// Print stats as table
	t := newTable(currentUser, userNames, units, usages)

	// Based on request rendering format
	switch {
	case htmlOut:
		t.RenderHTML()
	case csvOut:
		t.RenderCSV()
	case mdOut:
		t.RenderMarkdown()
	default:
		t.Render()
	}
}

// newTable returns a new table with data.
func newTable(currentUser string, users []string, units []models.Unit, usages []models.Usage) table.Writer {
	// Make a new writer
	t := table.NewWriter()

	// Row config
	rowConfig := table.RowConfig{AutoMerge: true}

	// Table style
	style := table.Style{
		Name:    "CustomStyleLight",
		Box:     table.StyleBoxLight,
		Color:   table.ColorOptionsDefault,
		HTML:    table.DefaultHTMLOptions,
		Options: table.OptionsDefault,
		Size:    table.SizeOptionsDefault,
		Title:   table.TitleOptionsDefault,
		Format: table.FormatOptions{
			Footer: text.FormatDefault,
			Header: text.FormatUpper,
			Row:    text.FormatDefault,
		},
	}

	// Configure table
	var columnConfigs []table.ColumnConfig
	for _, field := range fieldMap {
		columnConfigs = append(columnConfigs, table.ColumnConfig{
			Name:     field.title,
			WidthMin: field.minW,
			WidthMax: field.maxW,
		})
	}

	t.SuppressEmptyColumns()
	t.SuppressTrailingSpaces()
	t.SetStyle(style)
	t.SetOutputMirror(os.Stdout)
	t.SetColumnConfigs(columnConfigs)

	// Collect metric map's keys for each metric
	for _, unit := range units {
		updateField(unit.AveCPUUsage.Keys(), fieldMap["cpuusage"])
		updateField(unit.AveCPUMemUsage.Keys(), fieldMap["cpumemoryusage"])
		updateField(unit.TotalCPUEnergyUsage.Keys(), fieldMap["hostenergy"])
		updateField(unit.TotalCPUEmissions.Keys(), fieldMap["hostemissions"])
		updateField(unit.AveGPUUsage.Keys(), fieldMap["gpuusage"])
		updateField(unit.AveGPUMemUsage.Keys(), fieldMap["gpumemoryusage"])
		updateField(unit.TotalGPUEnergyUsage.Keys(), fieldMap["gpuenergy"])
		updateField(unit.TotalGPUEmissions.Keys(), fieldMap["gpuemissions"])
	}

	// Setup headers
	headers := table.Row{}
	subHeaders := table.Row{}

	for _, h := range allFields {
		headers = append(headers, fieldMap[h].titles()...)
		subHeaders = append(subHeaders, fieldMap[h].subtitles()...)
	}

	t.AppendHeader(headers, rowConfig)
	t.AppendHeader(subHeaders)

	// Append rows
	rows := make([]table.Row, len(units))

	for iunit, unit := range units {
		row := table.Row{
			unit.UUID, unit.Name, unit.Project, unit.Group, unit.User, unit.CreatedAt,
			unit.StartedAt, unit.EndedAt, unit.Elapsed, unit.State,
		}
		row = append(row, unit.AveCPUUsage.Values("%.2f")...)
		row = append(row, unit.AveCPUMemUsage.Values("%.2f")...)
		row = append(row, unit.TotalCPUEnergyUsage.Values("%f")...)
		row = append(row, unit.TotalCPUEmissions.Values("%f")...)
		row = append(row, unit.AveGPUUsage.Values("%.2f")...)
		row = append(row, unit.AveGPUMemUsage.Values("%.2f")...)
		row = append(row, unit.TotalGPUEnergyUsage.Values("%f")...)
		row = append(row, unit.TotalGPUEmissions.Values("%f")...)
		rows[iunit] = row
	}

	t.AppendRows(rows)

	// Append summary row
	t.AppendSeparator()

	summaryRow := table.Row{"Summary"}
	for range headers {
		summaryRow = append(summaryRow, "")
	}

	t.AppendRow(summaryRow, rowConfig)
	t.AppendSeparator()

	for _, usage := range usages {
		if usage.User == currentUser || slices.Contains(users, usage.User) {
			// Check if elapsed time in non zero
			var totalElapsedTime string
			if usage.TotalTime["walltime"] > 0 {
				totalElapsedTime = common.Timespan(time.Duration(usage.TotalTime["walltime"]) * time.Second).Format("15:04:05")
			}

			// Usage row
			row := table.Row{
				usage.NumUnits, "", usage.Project, usage.Group, usage.User, "", "", "", totalElapsedTime, "",
			}
			row = append(row, usage.AveCPUUsage.Values("%.2f")...)
			row = append(row, usage.AveCPUMemUsage.Values("%.2f")...)
			row = append(row, usage.TotalCPUEnergyUsage.Values("%f")...)
			row = append(row, usage.TotalCPUEmissions.Values("%f")...)
			row = append(row, usage.AveGPUUsage.Values("%.2f")...)
			row = append(row, usage.AveGPUMemUsage.Values("%.2f")...)
			row = append(row, usage.TotalGPUEnergyUsage.Values("%f")...)
			row = append(row, usage.TotalGPUEmissions.Values("%f")...)

			// Append row to table
			t.AppendFooter(row)
		}
	}

	return t
}

// updateField updates field keys with the ones found struct field keys.
func updateField(structKeys []string, f *field) {
	for _, k := range structKeys {
		if !slices.Contains(f.keys, k) {
			f.keys = append(f.keys, k)
		}
	}
}

// readConfig returns config struct from first found config file.
func readConfig() (*Config, error) {
	var config Config

	// Look for config.yml or config.yaml or cacct.yml or cacct.yaml files
	for _, configPath := range configPaths {
		for _, file := range []string{"config.yml", "config.yaml", "cacct.yml", "cacct.yaml"} {
			configFile := filepath.Join(configPath, file)
			if _, err := os.Stat(configFile); err == nil {
				// Read config file
				cfg, err := os.ReadFile(configFile)
				if err != nil {
					return nil, err
				}

				if err = yaml.Unmarshal(cfg, &config); err != nil {
					return nil, err
				}

				return &config, nil
			}
		}
	}

	return nil, errors.New("config file not found")
}

// getCurrentUser returns the actual user executing the cacct. If --current-user
// CLI flag is passed, that user will be returned as current user.
func getCurrentUser(mockUserName string, mockConfigPath string) (string, error) {
	// Get current user is who is executing cacct
	var currentUser string

	if u, err := user.Current(); err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	} else {
		// Check if mockUserName is set. This will be always empty string
		// for production builds as we do not compile flags for production
		// builds
		if mockUserName != "" {
			currentUser = mockUserName

			// If mockConfigPath is set as well, add to configPaths
			if mockConfigPath != "" {
				configPaths = append(configPaths, mockConfigPath)
			}
		} else {
			currentUser = u.Name
		}
	}

	// Add user HOME to configPaths
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config file: %w", err)
	}

	configPaths = append(configPaths, filepath.Join(userConfigDir, "ceems"))

	return currentUser, nil
}

func parseTime(s string) (time.Time, error) {
	// First attempt is to parse as YYYY-MM-DDTHH:MM:SS
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t.In(time.Local), nil
	}

	// Second attempt is to parse as YYYY-MM-DDTHH:MM
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.In(time.Local), nil
	}

	// Third attempt is to parse as YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.In(time.Local), nil
	}

	// If nothing works, return error
	return time.Time{}, errors.New("invalid time format")
}

func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, len(m))
	i := 0

	for k := range m {
		keys[i] = k
		i++
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	return keys
}

func splitString(s, d string) []string { //nolint:unparam
	var parts []string

	for _, p := range strings.Split(s, d) {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return parts
}

func checkErr(err error) int {
	if err != nil {
		switch {
		case errors.Is(err, errNoPerm):
			fmt.Fprintln(os.Stderr, "forbidden. It is likely that the user is attempting to view statistics of others")
		case errors.Is(err, errInternal):
			fmt.Fprintln(os.Stderr, "server did not return any data due to unknown error")
		}

		return 1
	}

	return 0
}
