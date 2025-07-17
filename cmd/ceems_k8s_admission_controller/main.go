package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/base"
	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/http"
	internal_runtime "github.com/ceems-dev/ceems/internal/runtime"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
)

var (
	app = kingpin.New(
		base.AppName,
		"CEEMS Kubernetes Admission Controller.",
	)
	webListenAddresses = app.Flag(
		"web.listen-address",
		"Addresses on which admission controller listens to requests.",
	).Default(":9000").Strings()
	webConfigFile = app.Flag(
		"web.config.file",
		"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
	).Default("").String()
	maxProcs = app.Flag(
		"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
	).Envar("GOMAXPROCS").Default("1").Int()
	enableDebugServer = app.Flag(
		"web.debug-server",
		"Enable /debug/pprof profiling (default: disabled).",
	).Default("false").Bool()
)

func main() {
	// Setup logger config
	promslogConfig := &promslog.Config{}
	flag.AddFlags(app, promslogConfig)
	app.Version(version.Print(app.Name))
	app.UsageWriter(os.Stdout)
	app.HelpFlag.Short('h')

	_, err := app.Parse(os.Args[1:])
	if err != nil {
		panic(err)
	}

	// Set logger here after properly configuring promlog
	logger := promslog.New(promslogConfig)

	logger.Info("Starting "+base.AppName, "version", version.Info())
	logger.Info(
		"Operational information", "build_context", version.BuildContext(),
		"host_details", internal_runtime.Uname(), "fd_limits", internal_runtime.FdLimits(),
	)

	runtime.GOMAXPROCS(*maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// If webConfigFile is set, get absolute path
	var webConfigFilePath string
	if *webConfigFile != "" {
		webConfigFilePath, err = filepath.Abs(*webConfigFile)
		if err != nil {
			logger.Error("Failed to get absolute path of web config file", "err", err)

			os.Exit(1)
		}
	}

	// Make a new config based
	config := &base.Config{
		Logger: logger,
		Web: base.WebConfig{
			Addresses:         *webListenAddresses,
			WebSystemdSocket:  false,
			WebConfigFile:     webConfigFilePath,
			EnableDebugServer: *enableDebugServer,
		},
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create a new controller instance
	server, err := http.NewAdmissionControllerServer(config)
	if err != nil {
		panic(err)
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below.
	go func() {
		if err := server.Start(); err != nil {
			logger.Error("Failed to start server", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	logger.Info("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Failed to gracefully shutdown server", "err", err)
	}

	logger.Info("Server exiting")
	logger.Info("See you next time!!")
}
