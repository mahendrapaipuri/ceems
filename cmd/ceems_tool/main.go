package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"gopkg.in/yaml.v3"
)

const (
	checkHealth = "/health"

	successExitCode = 0
	failureExitCode = 1
)

func main() {
	var (
		httpRoundTripper = api.DefaultRoundTripper
		apiServerURL     *url.URL
		lbServerURL      *url.URL

		promServerURL      *url.URL
		httpConfigFilePath string
		outDir             string

		start string
		end   string

		evalInterval        time.Duration
		pueValue            float64
		emissionFactorValue float64
		countryCode         string
		disableProviders    bool

		webConfigBasicAuth   bool
		webConfigTLS         bool
		webConfigTLSHosts    []string
		webConfigTLSValidity time.Duration
	)

	app := kingpin.New(filepath.Base(os.Args[0]), "Tooling for the CEEMS.").UsageWriter(os.Stdout)
	app.Version(version.Print("ceems_tool"))
	app.HelpFlag.Short('h')

	checkCmd := app.Command("check", "Check the CEEMS resources for validity.")

	checkAPIServerHealthCmd := checkCmd.Command("api-healthy", "Check if the CEEMS API server is healthy.")
	checkAPIServerHealthCmd.Flag(
		"http.config.file", "HTTP client configuration file for ceems_tool to connect to CEEMS API server.",
	).PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	checkAPIServerHealthCmd.Flag(
		"url", "The URL for the CEEMS API server.",
	).Default("http://localhost:9020").URLVar(&apiServerURL)

	checkLBServerHealthCmd := checkCmd.Command("lb-healthy", "Check if the CEEMS LB server is healthy.")
	checkLBServerHealthCmd.Flag(
		"http.config.file", "HTTP client configuration file for ceems_tool to connect to CEEMS LB server.",
	).PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	checkLBServerHealthCmd.Flag(
		"url", "The URL for the CEEMS TSDB LB server.",
	).Default("http://localhost:9030").URLVar(&lbServerURL)

	checkWebConfigCmd := checkCmd.Command("web-config", "Check if the web config files are valid or not.")
	webConfigFiles := checkWebConfigCmd.Arg(
		"web-config-files",
		"The config files to check.",
	).Required().ExistingFiles()

	configCmd := app.Command("config", "Configuration files related commands.")

	webConfigCmd := configCmd.Command("create-web-config", "Create web config file for CEEMS components.")
	webConfigCmd.Flag(
		"basic-auth", "Create web config file with basic auth (default: enabled).",
	).Default("true").BoolVar(&webConfigBasicAuth)
	webConfigCmd.Flag(
		"tls", "Create web config file with self signed TLS certificates (default: disabled).",
	).Default("false").BoolVar(&webConfigTLS)
	webConfigCmd.Flag(
		"tls.host", "Hostnames and/or IPs to generate a certificate for.",
	).StringsVar(&webConfigTLSHosts)
	webConfigCmd.Flag(
		"tls.validity", "Validity for TLS certificates. Default is 1 year.",
	).Default("8760h").DurationVar(&webConfigTLSValidity)
	webConfigCmd.Flag(
		"output-dir", "Output directory to place config files.",
	).Default("config").StringVar(&outDir)

	tsdbCmd := app.Command("tsdb", "TSDB related commands.")

	tsdbRecRulesCmd := tsdbCmd.Command("create-recording-rules", "Create Prometheus recording rules.")
	tsdbRecRulesCmd.Flag(
		"http.config.file", "HTTP client configuration file for ceems_tool to connect to Prometheus server.",
	).PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	tsdbRecRulesCmd.Flag(
		"url", "The URL for the Prometheus server.",
	).Default("http://localhost:9090").URLVar(&promServerURL)
	tsdbRecRulesCmd.Flag(
		"start", "The time to start querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. Default is 3 hours ago.",
	).StringVar(&start)
	tsdbRecRulesCmd.Flag(
		"end", "The time to end querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. Default is current time.",
	).StringVar(&end)
	tsdbRecRulesCmd.Flag(
		"pue", "Power Usage Effectiveness (PUE) value to use in power estimation rules.",
	).Default("1").Float64Var(&pueValue)
	tsdbRecRulesCmd.Flag(
		"emission-factor", "Static emission factor in gCO2/kWh value to use in equivalent emission estimation rules.",
	).Default("0").Float64Var(&emissionFactorValue)
	tsdbRecRulesCmd.Flag(
		"country-code", "ISO-2 code of the country to use in emissions estimation rules.",
	).StringVar(&countryCode)
	tsdbRecRulesCmd.Flag(
		"eval-interval", "Evaluation interval for the rules. If not set, default will be used.",
	).Default("0s").DurationVar(&evalInterval)
	tsdbRecRulesCmd.Flag(
		"output-dir", "Output directory to place rules files.",
	).Default("rules").StringVar(&outDir)
	tsdbRecRulesCmd.Flag(
		"disable-providers", "Disable providers (only for e2e testing).",
	).Hidden().Default("false").BoolVar(&disableProviders)

	tsdbRelabelConfigCmd := tsdbCmd.Command("create-relabel-configs", "Create Prometheus relabel configs.")
	tsdbRelabelConfigCmd.Flag(
		"http.config.file", "HTTP client configuration file for ceems_tool to connect to Prometheus server.",
	).PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	tsdbRelabelConfigCmd.Flag(
		"url", "The URL for the Prometheus server.",
	).Default("http://localhost:9090").URLVar(&promServerURL)
	tsdbRelabelConfigCmd.Flag(
		"start", "The time to start querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. Default is 3 hours ago.",
	).StringVar(&start)
	tsdbRelabelConfigCmd.Flag(
		"end", "The time to end querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. Default is current time.",
	).StringVar(&end)

	tsdbUpdaterConfigCmd := tsdbCmd.Command("create-ceems-tsdb-updater-queries", "Create CEEMS API TSDB updater queries.")
	tsdbUpdaterConfigCmd.Flag(
		"http.config.file", "HTTP client configuration file for ceems_tool to connect to Prometheus server.",
	).PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	tsdbUpdaterConfigCmd.Flag(
		"url", "The URL for the Prometheus server.",
	).Default("http://localhost:9090").URLVar(&promServerURL)
	tsdbUpdaterConfigCmd.Flag(
		"start", "The time to start querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. Default is 3 hours ago.",
	).StringVar(&start)
	tsdbUpdaterConfigCmd.Flag(
		"end", "The time to end querying for metrics. Must be a RFC3339 formatted date or Unix timestamp. Default is current time.",
	).StringVar(&end)

	parsedCmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	if httpConfigFilePath != "" {
		if promServerURL != nil && promServerURL.User.Username() != "" {
			kingpin.Fatalf("Cannot set base auth in the server URL and use a http.config.file at the same time")
		}

		var err error

		httpConfig, _, err := promconfig.LoadHTTPConfigFile(httpConfigFilePath)
		if err != nil {
			kingpin.Fatalf("Failed to load HTTP config file: %v", err)
		}

		httpRoundTripper, err = promconfig.NewRoundTripperFromConfig(*httpConfig, "ceems_tool", promconfig.WithUserAgent("ceems_tool/"+version.Version))
		if err != nil {
			kingpin.Fatalf("Failed to create a new HTTP round tripper: %v", err)
		}
	}

	switch parsedCmd {
	case checkAPIServerHealthCmd.FullCommand():
		os.Exit(checkErr(CheckServerStatus(apiServerURL, checkHealth, httpRoundTripper)))

	case checkLBServerHealthCmd.FullCommand():
		os.Exit(checkErr(CheckServerStatus(lbServerURL, checkHealth, httpRoundTripper)))

	case checkWebConfigCmd.FullCommand():
		os.Exit(CheckWebConfig(*webConfigFiles...))

	case webConfigCmd.FullCommand():
		if webConfigTLS && len(webConfigTLSHosts) == 0 {
			kingpin.Fatalf("--tls.host must be provided.")
		}

		os.Exit(checkErr(GenerateWebConfig(webConfigBasicAuth, webConfigTLS, webConfigTLSHosts, webConfigTLSValidity, outDir)))

	case tsdbUpdaterConfigCmd.FullCommand():
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		os.Exit(checkErr(GenerateTSDBUpdaterConfig(ctx, promServerURL, start, end, httpRoundTripper)))

	case tsdbRecRulesCmd.FullCommand():
		// Both country code and emission factor cannot be used together
		if countryCode != "" && emissionFactorValue > 0 {
			kingpin.Fatalf("--country-code and --emission-factor cannot be used together. Set atmost one.")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		os.Exit(checkErr(CreatePromRecordingRules(ctx, promServerURL, start, end, pueValue, emissionFactorValue, countryCode, evalInterval, outDir, disableProviders, httpRoundTripper)))

	case tsdbRelabelConfigCmd.FullCommand():
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		os.Exit(checkErr(CreatePromRelabelConfig(ctx, promServerURL, start, end, httpRoundTripper)))
	}
}

// CheckServerStatus checks the server status by making a request to check endpoint.
func CheckServerStatus(serverURL *url.URL, checkEndpoint string, roundTripper http.RoundTripper) error {
	if serverURL.Scheme == "" {
		serverURL.Scheme = "http"
	}

	config := api.Config{
		Address:      serverURL.String() + checkEndpoint,
		RoundTripper: roundTripper,
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)

		return err
	}

	request, err := http.NewRequest(http.MethodGet, config.Address, nil) //nolint:noctx
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	response, dataBytes, err := c.Do(ctx, request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("check failed: URL=%s, status=%d", serverURL, response.StatusCode)
	}

	fmt.Fprintln(os.Stderr, "  SUCCESS: ", string(dataBytes))

	return nil
}

// CheckWebConfig validates web configuration files.
func CheckWebConfig(files ...string) int {
	failed := false

	for _, f := range files {
		if err := web.Validate(f); err != nil {
			fmt.Fprintln(os.Stderr, f, "FAILED:", err)

			failed = true

			continue
		}

		fmt.Fprintln(os.Stderr, f, "SUCCESS")
	}

	if failed {
		return failureExitCode
	}

	return successExitCode
}

// newAPI returns a new API client.
func newAPI(url *url.URL, roundTripper http.RoundTripper, headers map[string]string) (v1.API, error) {
	if url.Scheme == "" {
		url.Scheme = "http"
	}

	config := api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	}

	if len(headers) > 0 {
		config.RoundTripper = promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			for key, value := range headers {
				req.Header.Add(key, value)
			}

			return roundTripper.RoundTrip(req)
		})
	}

	// Create new client.
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	api := v1.NewAPI(client)

	return api, nil
}

// config returns Prom config by making an API request.
func config(ctx context.Context, api v1.API) (*Config, error) {
	// Get Prom config
	c, err := api.Config(ctx)
	if err != nil {
		return nil, err
	}

	// Unmarshall config
	var config Config
	if err := yaml.Unmarshal([]byte(c.YAML), &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// intersection returns intersection of elements between slices.
func intersection[T cmp.Ordered](pS ...[]T) []T {
	hash := make(map[T]*int) // value, counter
	result := make([]T, 0)

	for _, slice := range pS {
		duplicationHash := make(map[T]bool) // duplication checking for individual slice
		for _, value := range slice {
			if _, isDup := duplicationHash[value]; !isDup { // is not duplicated in slice
				if counter := hash[value]; counter != nil { // is found in hash counter map
					if *counter++; *counter >= len(pS) { // is found in every slice
						result = append(result, value)
					}
				} else { // not found in hash counter map
					i := 1
					hash[value] = &i
				}

				duplicationHash[value] = true
			}
		}
	}

	return result
}

func parseTimes(start, end string) (time.Time, time.Time, error) {
	var stime, etime time.Time

	var err error

	if end == "" {
		etime = time.Now().UTC()
	} else {
		etime, err = parseTime(end)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("error parsing end time: %w", err)
		}
	}

	if start == "" {
		stime = time.Now().UTC().Add(-3 * time.Hour)
	} else {
		stime, err = parseTime(start)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("error parsing start time: %w", err)
		}
	}

	if !stime.Before(etime) {
		return time.Time{}, time.Time{}, errors.New("start time is not before end time")
	}

	return stime, etime, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)

		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}

	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}

func checkErr(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	return 0
}
