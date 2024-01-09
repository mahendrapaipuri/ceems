package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	batchjob_runtime "github.com/mahendrapaipuri/batchjob_metrics_monitor/internal/runtime"
	"github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/db"
	"github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/schedulers"
	"github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/server"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

// BatchJobStatsServer represents the `batchjob_stats_server` cli.
type BatchJobStatsServer struct {
	appName string
	App     kingpin.Application
}

// Create a new BatchJobStats struct
func NewBatchJobStatsServer() (*BatchJobStatsServer, error) {
	return &BatchJobStatsServer{
		appName: base.BatchJobStatsServerAppName,
		App:     base.BatchJobStatsServerApp,
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
		adminUsers = b.App.Flag(
			"web.admin-users",
			"Comma separated list of admin users (example: \"admin1,admin2\").",
		).Default("").String()
		dataPath = b.App.Flag(
			"storage.data.path",
			"Base path for data storage.",
		).Default("/var/lib/jobstats").String()
		retentionPeriodString = b.App.Flag(
			"storage.data.retention.period",
			"How long to retain job data. Units Supported: y, w, d, h, m, s, ms.",
		).Default("1y").String()
		lastUpdateTime = b.App.Flag(
			"storage.data.update.from",
			"Job data from this day will be gathered. Supported format: YYYY-MM-DD.",
		).Default(time.Now().Format("2006-01-02")).String()
		updateIntervalString = b.App.Flag(
			"storage.data.update.interval",
			"Job data will be updated at this interval. Units Supported: y, w, d, h, m, s, ms.",
		).Default("15m").String()
		jobDurationCutoffString = b.App.Flag(
			"storage.data.job.duration.cutoff",
			"Jobs with wall time less than this period will be ignored. Units Supported: y, w, d, h, m, s, ms.",
		).Default("5m").String()
		tsdbWebUrl = b.App.Flag(
			"tsdb.web.url",
			"TSDB URL (Prometheus/Victoria Metrics). If basic auth is enabled consider providing this URL using environment variable TSDB_WEBURL.",
		).Default(os.Getenv("TSDB_WEBURL")).String()
		tsdbWebSkipTLSVerify = b.App.Flag(
			"tsdb.web.skip-tls-verify",
			"Whether to skip TLS verification when using self signed certificates (default is false).",
		).Default("false").Bool()
		tsdbCleanUp = b.App.Flag(
			"tsdb.data.clean",
			"TSDB will be cleaned by removing time series of ignored jobs based on value set for --storage.data.job.duration.cutoff."+
				" --tsdb.web.url should be provided if this flag is set to true. (default is false)",
		).Default("false").Bool()
		skipDeleteOldJobs = b.App.Flag(
			"storage.data.skip.delete.old.jobs",
			"Skip deleting old jobs. Used only in testing. (default is false)",
		).Hidden().Default("false").Bool()
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

	// Parse retentionPeriod and updateInterval
	retentionPeriod, err := model.ParseDuration(*retentionPeriodString)
	if err != nil {
		fmt.Printf("Failed to parse --storage.data.retention.period flag. Error: %s", err)
		os.Exit(1)
	}
	updateInterval, err := model.ParseDuration(*updateIntervalString)
	if err != nil {
		fmt.Printf("Failed to parse --storage.data.update.interval flag. Error: %s", err)
		os.Exit(1)
	}
	jobDurationCutoff, err := model.ParseDuration(*jobDurationCutoffString)
	if err != nil {
		fmt.Printf("Failed to parse --storage.data.job.duration.cutoff flag. Error: %s", err)
		os.Exit(1)
	}

	// Parse lastUpdateTime to check if it is in correct format
	_, err = time.Parse("2006-01-02", *lastUpdateTime)
	if err != nil {
		fmt.Printf("Failed to parse --storage.data.update.from flag. Error: %s", err)
		os.Exit(1)
	}

	// Check if TSDB delete series flag is turned on, a valid TSDB Web URL is provided
	var client *http.Client
	var tsdbURL *url.URL
	if *tsdbCleanUp {
		tsdbURL, err = url.Parse(*tsdbWebUrl)
		if err != nil {
			fmt.Printf("Failed to parse --tsdb.web.url %s", err)
			os.Exit(1)
		}

		// If skip verify is set to true for TSDB add it to client
		if *tsdbWebSkipTLSVerify {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client = &http.Client{Transport: tr, Timeout: time.Duration(30 * time.Second)}
		} else {
			client = &http.Client{Timeout: time.Duration(30 * time.Second)}
		}

		// Create a new GET request to reach out to TSDB
		req, err := http.NewRequest(http.MethodGet, tsdbURL.String(), nil)
		if err != nil {
			fmt.Printf("Failed to make a new HTTP request for TSDB %s", err)
			os.Exit(1)
		}

		// Check if TSDB is reachable
		_, err = client.Do(req)
		if err != nil {
			fmt.Printf(
				"--tsdb.data.clean is set to true but TSDB at %s is unreachable %s",
				tsdbURL.Redacted(),
				err,
			)
			os.Exit(1)
		}
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", b.appName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", batchjob_runtime.Uname())
	level.Info(logger).Log("fd_limits", batchjob_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	absDataPath, err := filepath.Abs(*dataPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to get absolute path for --storage.data.path=%s. Error: %s", *dataPath, err))
	}
	jobstatDBPath := filepath.Join(absDataPath, "jobstats.db")
	jobsLastTimeStampFile := filepath.Join(absDataPath, "lastjobsupdatetime")

	// Get slice of admin users
	var adminUsersList []string
	for _, user := range strings.Split(*adminUsers, ",") {
		adminUsersList = append(adminUsersList, strings.TrimSpace(user))
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make DB config
	dbConfig := &db.Config{
		Logger:                  logger,
		JobstatsDBPath:          jobstatDBPath,
		JobstatsDBTable:         "jobs",
		JobCutoffPeriod:         time.Duration(jobDurationCutoff),
		RetentionPeriod:         time.Duration(retentionPeriod),
		SkipDeleteOldJobs:       *skipDeleteOldJobs,
		TSDBCleanUp:             *tsdbCleanUp,
		TSDBURL:                 tsdbURL,
		HTTPClient:              client,
		LastUpdateTimeString:    *lastUpdateTime,
		LastUpdateTimeStampFile: jobsLastTimeStampFile,
		BatchScheduler:          schedulers.NewBatchScheduler,
	}

	// Make server config
	serverConfig := &server.Config{
		Logger:           logger,
		Address:          *webListenAddresses,
		WebSystemdSocket: *systemdSocket,
		WebConfigFile:    *webConfigFile,
		DBConfig:         *dbConfig,
		AdminUsers:       adminUsersList,
	}

	// Create server instance
	apiServer, cleanup, err := server.NewJobstatsServer(serverConfig)
	defer cleanup()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create jobstats server", "err", err)
		return
	}

	// Create DB instance
	jobCollector, err := db.NewJobStatsDB(dbConfig)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create jobstats DB", "err", err)
		return
	}

	// Initialize a wait group
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		// Start a ticker
		ticker := time.NewTicker(time.Duration(updateInterval))
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
		if err := apiServer.Start(); err != nil {
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
	if err := apiServer.Shutdown(ctx, &wg); err != nil {
		level.Error(logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	// Wait for all go routines to finish
	wg.Wait()

	level.Info(logger).Log("msg", "Server exiting")
	level.Info(logger).Log("msg", "See you next time!!")
}
