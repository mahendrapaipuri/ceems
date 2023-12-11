// Main entrypoint for batchjob_stats_server

package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

func main() {
	var (
		webListenAddresses = jobstats.JobstatServerApp.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9020").String()
		webConfigFile = jobstats.JobstatServerApp.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Default("").String()
		jobstatDBFile = jobstats.JobstatServerApp.Flag(
			"path.db",
			"Absolute path to the SQLite DB file that contains jobs stats.",
		).Default("/var/lib/jobstats/jobstats.db").String()
		jobstatDBTable = jobstats.JobstatServerApp.Flag(
			"db.table.name",
			"Name of the table in SQLite DB file that contains jobs stats.",
		).Default("jobs").String()
		maxProcs = jobstats.JobstatServerApp.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)
	systemdSocket := func() *bool { b := false; return &b }() // Socket activation only available on Linux
	if runtime.GOOS == "linux" {
		systemdSocket = jobstats.JobstatServerApp.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Bool()
	}

	promlogConfig := &promlog.Config{}
	flag.AddFlags(jobstats.JobstatServerApp, promlogConfig)
	jobstats.JobstatServerApp.Version(version.Print(jobstats.JobstatServerAppName))
	jobstats.JobstatServerApp.UsageWriter(os.Stdout)
	jobstats.JobstatServerApp.HelpFlag.Short('h')
	_, err := jobstats.JobstatServerApp.Parse(os.Args[1:])
	if err != nil {
		panic(fmt.Sprintf("Failed to parse %s command", jobstats.JobstatDBAppName))
	}
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Running %s", jobstats.JobstatServerAppName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := &jobstats.Config{
		Logger:           logger,
		Address:          *webListenAddresses,
		WebSystemdSocket: *systemdSocket,
		WebConfigFile:    *webConfigFile,
		JobstatDBFile:    *jobstatDBFile,
		JobstatDBTable:   *jobstatDBTable,
	}

	server, cleanup, err := jobstats.NewJobstatsServer(config)
	defer cleanup()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create batchjob_stats_server", "err", err)
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
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
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		level.Error(logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	level.Info(logger).Log("msg", "Server exiting")
}
