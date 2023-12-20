// Main entrypoint for batchjob_stats

package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/log/level"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"

	batchjob_runtime "github.com/mahendrapaipuri/batchjob_monitoring/internal/runtime"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"
)

var (
	jobsTimestampFile = "lastjobsupdatetime"
)

func main() {
	var (
		webListenAddresses = jobstats.JobstatsApp.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9020").String()
		webConfigFile = jobstats.JobstatsApp.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Default("").String()
		batchScheduler = jobstats.JobstatsApp.Flag(
			"batch.scheduler",
			"Name of batch scheduler (eg slurm, lsf, pbs). Currently only slurm is supported.",
		).Default("slurm").String()
		dataPath = jobstats.JobstatsApp.Flag(
			"data.path",
			"Absolute path to a directory where job data is stored. SQLite DB that contains jobs stats will be saved to this directory.",
		).Default("/var/lib/jobstats").String()
		retentionPeriod = jobstats.JobstatsApp.Flag(
			"data.retention.period",
			"Period in days for which job stats data will be retained.",
		).Default("365").Int()
		jobstatDBFile = jobstats.JobstatsApp.Flag(
			"db.name",
			"Name of the SQLite DB file that contains job stats.",
		).Default("jobstats.db").String()
		jobstatDBTable = jobstats.JobstatsApp.Flag(
			"db.table.name",
			"Name of the table in SQLite DB file that contains job stats.",
		).Default("jobs").String()
		lastUpdateTime = jobstats.JobstatsApp.Flag(
			"db.last.update.time",
			"Last time the DB was updated. Job stats from this time will be added for new DB.",
		).Default(time.Now().Format("2006-01-02")).String()
		updateInterval = jobstats.JobstatsApp.Flag(
			"db.update.interval",
			"Time period in seconds at which DB will be updated with job stats.",
		).Default("1800").Int()
		maxProcs = jobstats.JobstatsApp.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)

	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }()
	if runtime.GOOS == "linux" {
		systemdSocket = jobstats.JobstatsApp.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Bool()
	}

	promlogConfig := &promlog.Config{}
	flag.AddFlags(jobstats.JobstatsApp, promlogConfig)
	jobstats.JobstatsApp.Version(version.Print(jobstats.JobstatsAppName))
	jobstats.JobstatsApp.UsageWriter(os.Stdout)
	jobstats.JobstatsApp.HelpFlag.Short('h')
	_, err := jobstats.JobstatsApp.Parse(os.Args[1:])
	if err != nil {
		panic(fmt.Sprintf("Failed to parse %s command", jobstats.JobstatsAppName))
	}
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Running %s", jobstats.JobstatsAppName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", batchjob_runtime.Uname())
	level.Info(logger).Log("fd_limits", batchjob_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	absDataPath, err := filepath.Abs(*dataPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to get absolute path for --data.path=%s. Error: %s", *dataPath, err))
	}
	jobstatDBPath := filepath.Join(absDataPath, *jobstatDBFile)
	jobsLastTimeStampFile := filepath.Join(absDataPath, jobsTimestampFile)

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := &jobstats.Config{
		Logger:           logger,
		Address:          *webListenAddresses,
		WebSystemdSocket: *systemdSocket,
		WebConfigFile:    *webConfigFile,
		JobstatDBFile:    jobstatDBPath,
		JobstatDBTable:   *jobstatDBTable,
	}

	server, cleanup, err := jobstats.NewJobstatsServer(config)
	defer cleanup()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create jobstats server", "err", err)
		return
	}

	jobCollector, err := jobstats.NewJobStatsDB(
		logger,
		*batchScheduler,
		jobstatDBPath,
		*jobstatDBTable,
		*retentionPeriod,
		*lastUpdateTime,
		jobsLastTimeStampFile,
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create jobstats DB", "err", err)
		return
	}

	// Initialize a wait group
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		// Start a ticker
		ticker := time.NewTicker(time.Second * time.Duration(*updateInterval))
		defer ticker.Stop()

	loop:
		for {
			level.Info(logger).Log("msg", "Updating JobStats DB")
			err := jobCollector.Collect()
			if err != nil {
				level.Error(logger).Log("msg", "Failed to get job stats", "err", err)
			}

			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				level.Info(logger).Log("msg", "Received Interrupt. Stopping DB update")
				err := jobCollector.Stop()
				if err != nil {
					level.Error(logger).Log("msg", "Failed to close DB connection", "err", err)
				}
				wg.Done()
				break loop
			}
		}
	}()

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	wg.Add(1)
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
	if err := server.Shutdown(ctx, &wg); err != nil {
		level.Error(logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	// Wait for all go routines to finish
	wg.Wait()

	level.Info(logger).Log("msg", "Server exiting")
	level.Info(logger).Log("msg", "See you next time!!")
}
