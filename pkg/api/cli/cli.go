package cli

import (
	"context"
	"fmt"
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
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/db"
	"github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

// CEEMSServer represents the `ceems_server` cli.
type CEEMSServer struct {
	appName string
	App     kingpin.Application
}

// Create a new CEEMSServer struct
func NewCEEMSServer() (*CEEMSServer, error) {
	return &CEEMSServer{
		appName: base.CEEMSServerAppName,
		App:     base.CEEMSServerApp,
	}, nil
}

// Main is the entry point of the `ceems_server` command
func (b *CEEMSServer) Main() error {
	var (
		webListenAddresses = b.App.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9020").String()
		webConfigFile = b.App.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Default("").String()
		maxQueryString = b.App.Flag(
			"web.max.query.period",
			"Maximum allowable query period to get usage statistics. Units Supported: y, w, d, h, m, s, ms.",
		).Default("1w").String()
		adminUsers = b.App.Flag(
			"web.admin-users",
			"Comma separated list of admin users (example: \"admin1,admin2\").",
		).Default("").String()

		// Storage related CLI args
		dataPath = b.App.Flag(
			"storage.data.path",
			"Base path for data storage.",
		).Default("data").String()
		retentionPeriodString = b.App.Flag(
			"storage.data.retention.period",
			"How long to retain data. Units Supported: y, w, d, h, m, s, ms.",
		).Default("30d").String()
		lastUpdateTime = b.App.Flag(
			"storage.data.update.from",
			"Job data from this day will be gathered. Format Supported: YYYY-MM-DD.",
		).Default(time.Now().Format("2006-01-02")).String()
		updateIntervalString = b.App.Flag(
			"storage.data.update.interval",
			"Job data will be updated at this interval. Units Supported: y, w, d, h, m, s, ms.",
		).Default("15m").String()
		dataBackupPath = b.App.Flag(
			"storage.data.backup.path",
			"Base path for backup data storage. Ideally this should on a separate storage device to achieve fault tolerance."+
				" Default is empty, no backups are created.",
		).Default("").String()
		backupIntervalString = b.App.Flag(
			"storage.data.backup.interval",
			"Job data DB will be backed up at this interval. Minimum interval is 1 day. Units Supported: y, w, d, h, m, s, ms.",
		).Default("1d").String()

		// Grafana related CLI args
		grafanaWebURL = b.App.Flag(
			"grafana.web.url",
			"Grafana URL for fetching admin users list from a service account.",
		).Default("").String()
		grafanaWebSkipTLSVerify = b.App.Flag(
			"grafana.web.skip-tls-verify",
			"Whether to skip TLS verification when using self signed certificates (default is false).",
		).Default("false").Bool()
		grafanaAdminTeamID = b.App.Flag(
			"grafana.teams.admin.id",
			"Grafana admins team ID. An API token must be set via GRAFANA_API_TOKEN environment variable.",
		).Default("").String()

		// Testing related hidden CLI args
		skipDeleteOldUnits = b.App.Flag(
			"storage.data.skip.delete.old.units",
			"Skip deleting old compute units. Used only in testing. (default is false)",
		).Hidden().Default("false").Bool()
		disableChecks = b.App.Flag(
			"test.disable.checks",
			"Disable sanity checks. Used only in testing. (default is false)",
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
		return fmt.Errorf("failed to parse CLI flags: %s", err)
	}

	// Parse retentionPeriod and updateInterval
	retentionPeriod, err := model.ParseDuration(*retentionPeriodString)
	if err != nil {
		return fmt.Errorf("failed to parse --storage.data.retention.period flag: %s", err)
	}
	updateInterval, err := model.ParseDuration(*updateIntervalString)
	if err != nil {
		return fmt.Errorf("failed to parse --storage.data.update.interval flag: %s", err)
	}
	backupInterval, err := model.ParseDuration(*backupIntervalString)
	if err != nil {
		return fmt.Errorf("failed to parse --storage.data.backup.interval flag: %s", err)
	}
	maxQuery, err := model.ParseDuration(*maxQueryString)
	if err != nil {
		return fmt.Errorf("failed to parse --web.max.query.period flag: %s", err)
	}

	// Parse lastUpdateTime to check if it is in correct format
	if _, err = time.Parse("2006-01-02", *lastUpdateTime); err != nil {
		return fmt.Errorf("failed to parse --storage.data.update.from flag: %s", err)
	}

	// If dataPath/dataBackupPath is empty, use current directory
	if *dataPath == "" {
		*dataPath = "data"
	}

	// Get absolute Data path
	var absDataBackupPath string
	absDataPath, err := filepath.Abs(*dataPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for --storage.data.path=%s: %s", *dataPath, err)
	}
	if *dataBackupPath != "" {
		if absDataBackupPath, err = filepath.Abs(*dataBackupPath); err != nil {
			return fmt.Errorf("failed to get absolute path for --storage.data.backup.path=%s: %s", *dataBackupPath, err)
		}
	}

	// Check if absDataPath/absDataBackupPath exists and create one if it does not
	if _, err := os.Stat(absDataPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absDataPath, 0750); err != nil {
			return fmt.Errorf("failed to create data directory: %s", err)
		}
	}
	if absDataBackupPath != "" {
		if _, err := os.Stat(absDataBackupPath); os.IsNotExist(err) {
			if err := os.MkdirAll(absDataBackupPath, 0750); err != nil {
				return fmt.Errorf("failed to create backup data directory: %s", err)
			}
		}
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", b.appName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", internal_runtime.Uname())
	level.Info(logger).Log("fd_limits", internal_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Make a new Grafana instance
	base.GrafanaWebURL = *grafanaWebURL
	base.GrafanaWebSkipTLSVerify = *grafanaWebSkipTLSVerify
	base.GrafanaAdminTeamID = *grafanaAdminTeamID
	grafana, err := grafana.NewGrafana(*grafanaWebURL, *grafanaWebSkipTLSVerify, logger)
	if err != nil {
		return fmt.Errorf("failed to create Grafana client: %s", err)
	}
	if grafana.Available() {
		if err := grafana.Ping(); err != nil {
			//lint:ignore ST1005 Grafana is a noun and need to capitalize!
			return fmt.Errorf("Grafana at %s is unreachable: %s", grafana.URL.Redacted(), err)
		}
	}

	// Get slice of admin users
	var adminUsersList []string
	for _, user := range strings.Split(*adminUsers, ",") {
		if u := strings.TrimSpace(user); u != "" {
			adminUsersList = append(adminUsersList, u)
		}
	}

	// Emit a log line that backup interval of less than 1 day is not possible
	if time.Duration(backupInterval) < time.Duration(24*time.Hour) && !*disableChecks {
		level.Warn(logger).
			Log("msg", "Back up interval of less than 1 day is not supported. Setting back up interval to 1 day.", "arg", *backupIntervalString)
		backupInterval, _ = model.ParseDuration("1d")
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make DB config
	dbConfig := &db.Config{
		Logger:               logger,
		DataPath:             absDataPath,
		DataBackupPath:       absDataBackupPath,
		RetentionPeriod:      time.Duration(retentionPeriod),
		SkipDeleteOldUnits:   *skipDeleteOldUnits,
		LastUpdateTimeString: *lastUpdateTime,
		ResourceManager:      resource.NewManager,
		Updater:              updater.NewUnitUpdater,
	}

	// Make server config
	serverConfig := &http.Config{
		Logger:           logger,
		Address:          *webListenAddresses,
		WebSystemdSocket: *systemdSocket,
		WebConfigFile:    *webConfigFile,
		DBConfig:         *dbConfig,
		MaxQueryPeriod:   time.Duration(maxQuery),
		AdminUsers:       adminUsersList,
		Grafana:          grafana,
	}

	// Create server instance
	apiServer, cleanup, err := http.NewCEEMSServer(serverConfig)
	defer cleanup()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create ceems_server server", "err", err)
		return err
	}

	// Create DB instance
	collector, err := db.NewStatsDB(dbConfig)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create ceems_server DB", "err", err)
		return err
	}

	// Declare wait group and tickers
	var wg sync.WaitGroup
	var dbUpdateTicker, dbBackupTicker *time.Ticker

	// Initialize tickers. We will stop the ticker immediately after signal has received
	dbUpdateTicker = time.NewTicker(time.Duration(updateInterval))

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			// This will ensure that we will run the method as soon as go routine
			// starts instead of waiting for ticker to tick
			level.Info(logger).Log("msg", "Updating CEEMS DB")
			if err := collector.Collect(); err != nil {
				level.Error(logger).Log("msg", "Failed to fetch compute units", "err", err)
			}

			select {
			case <-dbUpdateTicker.C:
				continue
			case <-ctx.Done():
				level.Info(logger).Log("msg", "Received Interrupt. Stopping DB update")
				return
			}
		}
	}()

	// Start backup go routine only backup path is provided in CLI
	if *dataBackupPath != "" {
		// Initialise ticker and increase waitgroup counter
		dbBackupTicker = time.NewTicker(time.Duration(backupInterval))
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-dbBackupTicker.C:
					// Dont run backup as soon as go routine is spawned. In prod, it
					// can take very long depending on the size of DB and so wait until
					// first tick to run it
					level.Info(logger).Log("msg", "Backing up CEEMS DB")
					if err := collector.Backup(); err != nil {
						level.Error(logger).Log("msg", "Failed to backup DB", "err", err)
					}
				case <-ctx.Done():
					level.Info(logger).Log("msg", "Received Interrupt. Stopping DB backup")
					return
				}
			}
		}()
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := apiServer.Start(); err != nil {
			level.Error(logger).Log("msg", "Failed to start server", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Stop tickers
	dbUpdateTicker.Stop()
	if *dataBackupPath != "" {
		dbBackupTicker.Stop()
	}

	// Wait for all DB go routines to finish
	wg.Wait()

	// Close DB only after all DB go routines are done
	if err := collector.Stop(); err != nil {
		level.Error(logger).Log("msg", "Failed to close DB connection", "err", err)
	}

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	level.Info(logger).Log("msg", "Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(ctx); err != nil {
		level.Error(logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	level.Info(logger).Log("msg", "Server exiting")
	level.Info(logger).Log("msg", "See you next time!!")
	return nil
}
