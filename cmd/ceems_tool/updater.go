package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"text/template"
)

// Embed the updater directory.
//
//go:embed updater
var updaterFS embed.FS

type TSDBQuery struct {
	Query, Series string
}

var tsdbQueries = map[string]map[string]TSDBQuery{
	"avg_cpu_usage": {
		"global": TSDBQuery{
			Series: "uuid:ceems_cpu_usage:ratio_irate",
			Query:  `avg_over_time(avg by (uuid) (%s{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])`,
		},
	},
	"avg_cpu_mem_usage": {
		"global": TSDBQuery{
			Series: "uuid:ceems_cpu_memory_usage:ratio",
			Query:  `avg_over_time(avg by (uuid) (%s{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])`,
		},
	},
	"total_cpu_energy_usage_kwh": {
		"total": TSDBQuery{
			Series: "uuid:ceems_host_power_watts:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 3.6e9`,
		},
	},
	"total_cpu_emissions_gms": {
		"rte_total": TSDBQuery{
			Series: "uuid:ceems_host_emissions_g_s:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}",provider="rte"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3`,
		},
		"emaps_total": TSDBQuery{
			Series: "uuid:ceems_host_emissions_g_s:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}",provider="emaps"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3`,
		},
		"owid_total": TSDBQuery{
			Series: "uuid:ceems_host_emissions_g_s:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}",provider="owid"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3`,
		},
	},
	"avg_gpu_usage": {
		"global": TSDBQuery{
			Series: "uuid:ceems_gpu_usage:ratio",
			Query:  `avg_over_time(avg by (uuid) (%s{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])`,
		},
	},
	"avg_gpu_mem_usage": {
		"global": TSDBQuery{
			Series: "uuid:ceems_gpu_memory_usage:ratio",
			Query:  `avg_over_time(avg by (uuid) (%s{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:])`,
		},
	},
	"total_gpu_energy_usage_kwh": {
		"total": TSDBQuery{
			Series: "uuid:ceems_gpu_power_watts:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 3.6e9`,
		},
	},
	"total_gpu_emissions_gms": {
		"rte_total": TSDBQuery{
			Series: "uuid:ceems_gpu_emissions_g_s:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}",provider="rte"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3`,
		},
		"emaps_total": TSDBQuery{
			Series: "uuid:ceems_gpu_emissions_g_s:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}",provider="emaps"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3`,
		},
		"owid_total": TSDBQuery{
			Series: "uuid:ceems_gpu_emissions_g_s:pue",
			Query:  `sum_over_time(sum by (uuid) (%s{uuid=~"{{.UUIDs}}",provider="owid"} > 0 < inf)[{{.Range}}:{{.ScrapeInterval}}]) * {{.ScrapeIntervalMilli}} / 1e3`,
		},
	},
}

// tsdbQueriesTemplateData contains data to be used inside templates.
type tsdbQueriesTemplateData struct {
	Queries     map[string]map[string]TSDBQuery
	FoundSeries []string
	Providers   []string
}

// GenerateTSDBUpdaterConfig generates TSDB updater config.
func GenerateTSDBUpdaterConfig(ctx context.Context, serverURL *url.URL, start string, end string, roundTripper http.RoundTripper) error {
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

	// Get all required series from queries
	var allSeries []string

	for _, queries := range tsdbQueries {
		for _, query := range queries {
			if !slices.Contains(allSeries, query.Series) {
				allSeries = append(allSeries, query.Series)
			}
		}
	}

	// Run query to get matching series.
	labelset, _, err := api.Series(ctx, allSeries, stime, etime) // Ignoring warnings for now.
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching labelsets:", err)

		return err
	}

	var foundSeries []string
	for _, label := range labelset {
		if s := string(label["__name__"]); !slices.Contains(foundSeries, s) {
			foundSeries = append(foundSeries, s)
		}
	}

	// Get all providers of emission factors
	providers, _, err := api.LabelValues(ctx, "provider", []string{"uuid:ceems_host_emissions_g_s:pue"}, stime, etime) // Ignoring warnings for now.
	if err != nil {
		fmt.Fprintln(os.Stderr, "error fetching emission factor providers:", err)

		return err
	}

	// Append _total suffix to all providers
	var providerNames []string
	for _, provider := range providers {
		providerNames = append(providerNames, string(provider)+"_total")
	}

	// Template data
	tmplData := tsdbQueriesTemplateData{
		Queries:     tsdbQueries,
		FoundSeries: foundSeries,
		Providers:   providerNames,
	}

	// Custom functions
	funcMap := template.FuncMap{
		"SliceContains": func(s []string, e string) bool {
			return slices.Contains(s, e)
		},
		"StringContains": func(s string, sub string) bool {
			return strings.Contains(s, sub)
		},
	}

	// Make a new template
	tmpl, err := template.New("rules").Delims("[[", "]]").Funcs(funcMap).ParseFS(updaterFS, "updater/*.yml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing rules template:", err)

		return err
	}

	// Render the CPU rules template
	buf := &bytes.Buffer{}
	if err := tmpl.ExecuteTemplate(buf, "queries.yml", tmplData); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Queries for TSDB updater:")
	fmt.Fprintln(os.Stderr, buf.String())

	return nil
}
