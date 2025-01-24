package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	promconfig "github.com/prometheus/common/config"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/version"
)

const (
	checkHealth = "/health"
)

func main() {
	var (
		httpRoundTripper   = api.DefaultRoundTripper
		apiServerURL       *url.URL
		lbServerURL        *url.URL
		promServerURL      *url.URL
		httpConfigFilePath string
		rulesOutDir        string
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

	tsdbCmd := app.Command("tsdb", "TSDB related commands.")

	tsdbRecRulesCmd := tsdbCmd.Command("recording-rules", "Create Prometheus recording rules.")
	tsdbRecRulesCmd.Flag(
		"http.config.file", "HTTP client configuration file for ceems_tool to connect to Prometheus server.",
	).PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	tsdbRecRulesCmd.Flag(
		"url", "The URL for the Prometheus server.",
	).Default("http://localhost:9090").URLVar(&promServerURL)
	tsdbRecRulesCmd.Flag(
		"output-dir", "Output directory to place rules files.",
	).Default("rules").StringVar(&rulesOutDir)

	parsedCmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	if httpConfigFilePath != "" {
		if apiServerURL != nil && apiServerURL.User.Username() != "" {
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

	case tsdbRecRulesCmd.FullCommand():
		os.Exit(checkErr(CreatePromRecordingRules(lbServerURL, httpRoundTripper)))
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

	request, err := http.NewRequest(http.MethodGet, config.Address, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, dataBytes, err := c.Do(ctx, request)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("check failed: URL=%s, status=%d", serverURL, response.StatusCode)
	}

	fmt.Fprintln(os.Stderr, "  SUCCESS: ", string(dataBytes))
	return nil
}

func checkErr(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
