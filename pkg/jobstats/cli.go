package jobstats

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	batchjob_runtime "github.com/mahendrapaipuri/batchjob_monitoring/internal/runtime"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

// Name of batchjob_stats_server kingpin app
const BatchJobStatsAppName = "batchjob_stats_server"

// `batchjob_stats_server` CLI app
var BatchJobStatsServerApp = *kingpin.New(
	BatchJobStatsAppName,
	"API server data source for batch job statistics of users.",
)

// Create a new BatchJobStats struct
func NewBatchJobStatsServer() (*BatchJobStatsServer, error) {
	promlogConfig := &promlog.Config{}
	return &BatchJobStatsServer{
		promlogConfig: *promlogConfig,
		appName:       BatchJobStatsAppName,
		App:           BatchJobStatsServerApp,
	}, nil
}

// Main is the entry point of the `batchjob_exporter` command
func (b *BatchJobStatsServer) Main() {
	var (
		webListenAddresses = b.App.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9020").String()
		webConfigFile = b.App.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Default("").String()
		dataPath = b.App.Flag(
			"data.path",
			"Absolute path to a directory where job data is stored. SQLite DB that contains jobs stats will be saved to this directory.",
		).Default("/var/lib/jobstats").String()
		retentionPeriod = b.App.Flag(
			"data.retention.period",
			"Period in days for which job stats data will be retained.",
		).Default("365").Int()
		jobstatDBFile = b.App.Flag(
			"db.name",
			"Name of the SQLite DB file that contains job stats.",
		).Default("jobstats.db").String()
		jobstatDBTable = b.App.Flag(
			"db.table.name",
			"Name of the table in SQLite DB file that contains job stats.",
		).Default("jobs").String()
		lastUpdateTime = b.App.Flag(
			"db.last.update.time",
			"Last time the DB was updated. Job stats from this time will be added for new DB.",
		).Default(time.Now().Format("2006-01-02")).String()
		updateInterval = b.App.Flag(
			"db.update.interval",
			"Time period in seconds at which DB will be updated with job stats.",
		).Default("1800").Int()
		maxProcs = b.App.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)

	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }()
	if runtime.GOOS == "linux" {
		systemdSocket = b.App.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Bool()
	}

	flag.AddFlags(&b.App, &b.promlogConfig)
	b.App.Version(version.Print(b.appName))
	b.App.UsageWriter(os.Stdout)
	b.App.HelpFlag.Short('h')
	_, err := b.App.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("Failed to parse CLI flags. Error: %s", err)
		os.Exit(1)
	}

	// Set logger here after properly configuring promlog
	b.logger = promlog.New(&b.promlogConfig)

	level.Info(b.logger).Log("msg", fmt.Sprintf("Starting %s", b.appName), "version", version.Info())
	level.Info(b.logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(b.logger).Log("fd_limits", batchjob_runtime.Uname())
	level.Info(b.logger).Log("fd_limits", batchjob_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(b.logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	absDataPath, err := filepath.Abs(*dataPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to get absolute path for --data.path=%s. Error: %s", *dataPath, err))
	}
	jobstatDBPath := filepath.Join(absDataPath, *jobstatDBFile)
	jobsLastTimeStampFile := filepath.Join(absDataPath, "lastjobsupdatetime")

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config := &Config{
		Logger:           b.logger,
		Address:          *webListenAddresses,
		WebSystemdSocket: *systemdSocket,
		WebConfigFile:    *webConfigFile,
		JobstatDBFile:    jobstatDBPath,
		JobstatDBTable:   *jobstatDBTable,
	}

	server, cleanup, err := NewJobstatsServer(config)
	defer cleanup()
	if err != nil {
		level.Error(b.logger).Log("msg", "Failed to create jobstats server", "err", err)
		return
	}

	jobCollector, err := NewJobStatsDB(
		b.logger,
		jobstatDBPath,
		*jobstatDBTable,
		*retentionPeriod,
		*lastUpdateTime,
		jobsLastTimeStampFile,
		NewBatchScheduler,
	)
	if err != nil {
		level.Error(b.logger).Log("msg", "Failed to create jobstats DB", "err", err)
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
			level.Info(b.logger).Log("msg", "Updating JobStats DB")
			err := jobCollector.Collect()
			if err != nil {
				level.Error(b.logger).Log("msg", "Failed to get job stats", "err", err)
			}

			select {
			case <-ticker.C:
				continue
			case <-ctx.Done():
				level.Info(b.logger).Log("msg", "Received Interrupt. Stopping DB update")
				err := jobCollector.Stop()
				if err != nil {
					level.Error(b.logger).Log("msg", "Failed to close DB connection", "err", err)
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
			level.Error(b.logger).Log("msg", "Failed to start server", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	level.Info(b.logger).Log("msg", "Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx, &wg); err != nil {
		level.Error(b.logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	// Wait for all go routines to finish
	wg.Wait()

	level.Info(b.logger).Log("msg", "Server exiting")
	level.Info(b.logger).Log("msg", "See you next time!!")
}
