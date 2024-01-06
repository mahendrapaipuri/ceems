package collector

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/user"
	"runtime"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	batchjob_runtime "github.com/mahendrapaipuri/batchjob_monitoring/internal/runtime"
	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

// BatchJobExporter represents the `batchjob_exporter` cli.
type BatchJobExporter struct {
	appName string
	App     kingpin.Application
}

// Name of batchjob_exporter kingpin app
const BatchJobExporterAppName = "batchjob_exporter"

// `batchjob_exporter` CLI app
var BatchJobExporterApp = *kingpin.New(
	BatchJobExporterAppName,
	"Prometheus Exporter to export batch job metrics.",
)

// Current hostname
var hostname string

// Empty hostname flag (Used only for testing)
var emptyHostnameLabel *bool

// Create a new BatchJobExporter struct
func NewBatchJobExporter() (*BatchJobExporter, error) {
	return &BatchJobExporter{
		appName: BatchJobExporterAppName,
		App:     BatchJobExporterApp,
	}, nil
}

// Create a new handler for exporting metrics
func (b *BatchJobExporter) newHandler(includeExporterMetrics bool, maxRequests int, logger log.Logger) *handler {
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
		panic(fmt.Sprintf("Couldn't create metrics handler: %s", err))
	} else {
		h.unfilteredHandler = innerHandler
	}
	return h
}

// Main is the entry point of the `batchjob_exporter` command
func (b *BatchJobExporter) Main() {
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
		"collector.empty.hostname.label",
		"Use empty hostname in labels. Only for testing. (default is disabled)",
	).Hidden().Default("false").Bool()

	promlogConfig := &promlog.Config{}
	flag.AddFlags(&b.App, promlogConfig)
	b.App.Version(version.Print(b.appName))
	b.App.UsageWriter(os.Stdout)
	b.App.HelpFlag.Short('h')
	_, err := b.App.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("Failed to parse CLI flags. Error: %s", err)
		os.Exit(1)
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	if *disableDefaultCollectors {
		DisableDefaultCollectors()
	}
	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", b.appName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", batchjob_runtime.Uname())
	level.Info(logger).Log("fd_limits", batchjob_runtime.FdLimits())

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
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
}
