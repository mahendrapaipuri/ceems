package collector

import (
	"fmt"
	"net/http"
	_ "net/http/pprof" // #nosec
	"os"
	"os/user"
	"runtime"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

// CEEMSExporter represents the `ceems_exporter` cli.
type CEEMSExporter struct {
	appName string
	App     kingpin.Application
}

// CEEMSExporterAppName is kingpin app name.
const CEEMSExporterAppName = "ceems_exporter"

// CEEMSExporterApp is kingpin CLI app.
var CEEMSExporterApp = *kingpin.New(
	CEEMSExporterAppName,
	"Prometheus Exporter to export compute (job, VM, pod) resource usage metrics.",
)

// Current hostname.
var hostname string

// Empty hostname flag (Used only for testing).
var emptyHostnameLabel *bool

// NewCEEMSExporter returns a new CEEMSExporter instance.
func NewCEEMSExporter() (*CEEMSExporter, error) {
	return &CEEMSExporter{
		appName: CEEMSExporterAppName,
		App:     CEEMSExporterApp,
	}, nil
}

// Create a new handler for exporting metrics.
func (b *CEEMSExporter) newHandler(includeExporterMetrics bool, maxRequests int, logger log.Logger) *handler {
	h := &handler{
		exporterMetricsRegistry: prometheus.NewRegistry(),
		includeExporterMetrics:  includeExporterMetrics,
		maxRequests:             maxRequests,
		logger:                  logger,
	}
	if h.includeExporterMetrics {
		h.exporterMetricsRegistry.MustRegister(
			promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}),
			promcollectors.NewGoCollector(),
		)
	}

	if innerHandler, err := h.innerHandler(); err != nil {
		panic(fmt.Errorf("couldn't create metrics handler: %w", err))
	} else {
		h.unfilteredHandler = innerHandler
	}

	return h
}

// Main is the entry point of the `ceems_exporter` command.
func (b *CEEMSExporter) Main() error {
	var (
		metricsPath = b.App.Flag(
			"web.telemetry-path",
			"Path under which to expose metrics.",
		).Default("/metrics").String()
		disableExporterMetrics = b.App.Flag(
			"web.disable-exporter-metrics",
			"Exclude metrics about the exporter itself (promhttp_*, process_*, go_*).",
		).Bool()
		maxRequests = b.App.Flag(
			"web.max-requests",
			"Maximum number of parallel scrape requests. Use 0 to disable.",
		).Default("40").Int()
		disableDefaultCollectors = b.App.Flag(
			"collector.disable-defaults",
			"Set all collectors to disabled by default.",
		).Default("false").Bool()
		maxProcs = b.App.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
		toolkitFlags = kingpinflag.AddFlags(&b.App, ":9010")
	)

	// This is hidden flag only used for e2e testing
	emptyHostnameLabel = b.App.Flag(
		"collector.empty-hostname-label",
		"Use empty hostname in labels. Only for testing. (default is disabled)",
	).Hidden().Default("false").Bool()

	promlogConfig := &promlog.Config{}
	flag.AddFlags(&b.App, promlogConfig)
	b.App.Version(version.Print(b.appName))
	b.App.UsageWriter(os.Stdout)
	b.App.HelpFlag.Short('h')

	_, err := b.App.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("failed to parse CLI flags: %w", err)
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	if *disableDefaultCollectors {
		DisableDefaultCollectors()
	}

	level.Info(logger).Log("msg", "Starting "+b.appName, "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", internal_runtime.Uname())
	level.Info(logger).Log("fd_limits", internal_runtime.FdLimits())

	if user, err := user.Current(); err == nil && user.Uid == "0" {
		level.Warn(logger).
			Log("msg", "Batch Job Metrics Exporter is running as root user. This exporter can be run as unprivileged user, root is not required.")
	}

	// Get hostname
	if !*emptyHostnameLabel {
		hostname, err = os.Hostname()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get hostname", "err", err)
		}
	}

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	http.Handle(*metricsPath, b.newHandler(!*disableExporterMetrics, *maxRequests, logger))

	if *metricsPath != "/" {
		landingConfig := web.LandingConfig{
			Name:        b.App.Name,
			Description: b.App.Help,
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address: *metricsPath,
					Text:    "Metrics",
				},
			},
		}

		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			return fmt.Errorf("failed to create landing page: %w", err)
		}

		http.Handle("/", landingPage)
	}

	server := &http.Server{
		ReadHeaderTimeout: 2 * time.Second, // slowloris attack: https://app.deepsource.com/directory/analyzers/go/issues/GO-S2112
	}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}
