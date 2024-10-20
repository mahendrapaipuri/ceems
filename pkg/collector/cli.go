package collector

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
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
// var emptyHostnameLabel *bool
// This is hidden flag only used for e2e testing.
var emptyHostnameLabel = CEEMSExporterApp.Flag(
	"collector.empty-hostname-label",
	"Use empty hostname in labels. Only for testing. (default is disabled)",
).Hidden().Default("false").Bool()

// NewCEEMSExporter returns a new CEEMSExporter instance.
func NewCEEMSExporter() (*CEEMSExporter, error) {
	return &CEEMSExporter{
		appName: CEEMSExporterAppName,
		App:     CEEMSExporterApp,
	}, nil
}

// Main is the entry point of the `ceems_exporter` command.
func (b *CEEMSExporter) Main() error {
	var (
		webListenAddresses = b.App.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9010").Strings()
		webConfigFile = b.App.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Envar("CEEMS_EXPORTER_WEB_CONFIG_FILE").Default("").String()
		metricsPath = b.App.Flag(
			"web.telemetry-path",
			"Path under which to expose metrics.",
		).Default("/metrics").String()
		targetsPath = b.App.Flag(
			"web.targets-path",
			"Path under which to expose Grafana Alloy targets.",
		).Default("/alloy-targets").String()
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
		enableDebugServer = b.App.Flag(
			"web.debug-server",
			"Enable debug server (default: disabled).",
		).Default("false").Bool()

		// test CLI flags hidden
		dropPrivs = b.App.Flag(
			"security.drop-privileges",
			"Drop privileges and run as nobody when exporter is started as root.",
		).Default("true").Hidden().Bool()
	)

	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }() //nolint:nlreturn
	if runtime.GOOS == "linux" {
		systemdSocket = b.App.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Bool()
	}

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

	// Get hostname
	if !*emptyHostnameLabel {
		hostname, err = os.Hostname()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get hostname", "err", err)
		}
	}

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create a new instance of collector
	collector, err := NewCEEMSCollector(logger)
	if err != nil {
		return err
	}

	// Create a new instance of Alloy targets discoverer
	discoverer, err := NewAlloyTargetDiscoverer(log.With(logger, "discoverer", "alloy_targets"))
	if err != nil {
		return err
	}

	if user, err := user.Current(); err == nil && user.Uid == "0" {
		level.Info(logger).
			Log("msg", "CEEMS Exporter is running as root user. Privileges will be dropped and process will be run as unprivileged user")
	}

	// Make security related config
	// If the exporter is started as root, we pick up necessary privileges and
	// change user to nobody.
	// Why nobody? Because we are sure that this user exists on all distros and
	// we do not/should not create users as it can have unwanted side-effects.
	// We should be minimally intrusive but at the same time should provide maximum
	// security
	if *dropPrivs {
		securityCfg := &security.Config{
			RunAsUser: "nobody",
			Caps:      allCollectorCaps,
			ReadPaths: []string{*webConfigFile},
		}

		// Drop all unnecessary privileges
		if err := security.DropPrivileges(securityCfg); err != nil {
			return err
		}
	}

	// Create web server config
	config := &Config{
		Logger:     logger,
		Collector:  collector,
		Discoverer: discoverer,
		Web: WebConfig{
			Addresses:              *webListenAddresses,
			WebSystemdSocket:       *systemdSocket,
			WebConfigFile:          *webConfigFile,
			MetricsPath:            *metricsPath,
			TargetsPath:            *targetsPath,
			MaxRequests:            *maxRequests,
			IncludeExporterMetrics: !*disableExporterMetrics,
			EnableDebugServer:      *enableDebugServer,
			LandingConfig: &web.LandingConfig{
				Name:        b.App.Name,
				Description: b.App.Help,
				Version:     version.Info(),
				Links: []web.LandingLinks{
					{
						Address: *metricsPath,
						Text:    "Metrics",
					},
					{
						Address: *targetsPath,
						Text:    "Grafana Alloy Targets",
					},
				},
			},
		},
	}

	// Create a new exporter server instance
	server, err := NewCEEMSExporterServer(config)
	if err != nil {
		return err
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below.
	go func() {
		if err := server.Start(); err != nil {
			level.Error(logger).Log("msg", "Failed to start server", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	level.Info(logger).Log("msg", "Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		level.Error(logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	level.Info(logger).Log("msg", "Server exiting")
	level.Info(logger).Log("msg", "See you next time!!")

	return nil
}
