// Package cli implements the CLI of the CEEMS API server app
package cli

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
	"github.com/mahendrapaipuri/ceems/internal/common"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/db"
	"github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

// CEEMSAPIAppConfig contains the configuration of CEEMS API server
type CEEMSAPIAppConfig struct {
	Server CEEMSAPIServerConfig `yaml:"ceems_api_server"`
}

// SetDirectory joins any relative file paths with dir.
func (c *CEEMSAPIAppConfig) SetDirectory(dir string) {
	c.Server.Admin.Grafana.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *CEEMSAPIAppConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	todayMidnight, _ := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	*c = CEEMSAPIAppConfig{
		CEEMSAPIServerConfig{
			Data: db.DataConfig{
				Path:            "data",
				RetentionPeriod: model.Duration(30 * 24 * time.Hour),
				UpdateInterval:  model.Duration(15 * time.Minute),
				BackupInterval:  model.Duration(24 * time.Hour),
				LastUpdateTime:  todayMidnight,
			},
			Admin: db.AdminConfig{
				Grafana: common.GrafanaWebConfig{
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
			Web: http.WebConfig{
				RoutePrefix: "/",
			},
		},
	}

	type plain CEEMSAPIAppConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Set HTTPClientConfig in Web to empty struct as we do not and should not need
	// CEEMS API server's client config on the server. The client config is only used
	// in LB
	//
	// If we are using the same config file for both API server and LB,
	// secrets will be available in the client config and to reduce attack surface we
	// remove them all here by setting it to empty struct
	c.Server.Web.HTTPClientConfig = config.HTTPClientConfig{}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.Server.Admin.Grafana.HTTPClientConfig.Validate()
}

// CEEMSAPIServerConfig contains the configuration of CEEMS API server
type CEEMSAPIServerConfig struct {
	Data  db.DataConfig  `yaml:"data"`
	Admin db.AdminConfig `yaml:"admin"`
	Web   http.WebConfig `yaml:"web"`
}

// CEEMSServer represents the `ceems_server` cli.
type CEEMSServer struct {
	appName string
	App     kingpin.Application
}

// NewCEEMSServer creates a new CEEMSServer instance
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
		).Envar("CEEMS_API_SERVER_WEB_CONFIG_FILE").Default("").String()
		configFile = b.App.Flag(
			"config.file",
			"Path to CEEMS API server configuration file.",
		).Envar("CEEMS_API_SERVER_CONFIG_FILE").Default("").String()

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

	// Get absolute config file path global variable that will be used in resource manager
	// and updater packages
	base.ConfigFilePath, err = filepath.Abs(*configFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of the config file: %s", err)
	}

	// Make config from file
	config, err := common.MakeConfig[CEEMSAPIAppConfig](*configFile)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %s", err)
	}
	// Set directory for reading files
	config.SetDirectory(filepath.Dir(*configFile))
	// This is used only in tests
	config.Server.Data.SkipDeleteOldUnits = *skipDeleteOldUnits

	// Return error if backup interval of less than 1 day is used
	if time.Duration(config.Server.Data.BackupInterval) < time.Duration(24*time.Hour) && !*disableChecks {
		return fmt.Errorf("back up interval of less than 1 day is not supported")
	}

	// Setup data directories
	if config, err = createDirs(config); err != nil {
		return err
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", b.appName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", internal_runtime.Uname())
	level.Info(logger).Log("fd_limits", internal_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make DB config
	dbConfig := &db.Config{
		Logger:          logger,
		Data:            config.Server.Data,
		Admin:           config.Server.Admin,
		ResourceManager: resource.NewManager,
		Updater:         updater.NewUnitUpdater,
	}

	// Make server config
	serverConfig := &http.Config{
		Logger: logger,
		Web: http.WebConfig{
			Address:          *webListenAddresses,
			WebSystemdSocket: *systemdSocket,
			WebConfigFile:    *webConfigFile,
			RoutePrefix:      config.Server.Web.RoutePrefix,
			MaxQueryPeriod:   config.Server.Web.MaxQueryPeriod,
		},
		DB: *dbConfig,
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
	dbUpdateTicker = time.NewTicker(time.Duration(config.Server.Data.UpdateInterval))

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
	if config.Server.Data.BackupPath != "" {
		// Initialise ticker and increase waitgroup counter
		dbBackupTicker = time.NewTicker(time.Duration(config.Server.Data.BackupInterval))
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
	if config.Server.Data.BackupPath != "" {
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

// createDirs makes data directories and set paths to absolute in config
func createDirs(config *CEEMSAPIAppConfig) (*CEEMSAPIAppConfig, error) {
	var err error
	// Get absolute Data path
	config.Server.Data.Path, err = filepath.Abs(config.Server.Data.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for data.path=%s: %s", config.Server.Data.Path, err)
	}
	if config.Server.Data.BackupPath != "" {
		if config.Server.Data.BackupPath, err = filepath.Abs(config.Server.Data.BackupPath); err != nil {
			return nil, fmt.Errorf(
				"failed to get absolute path for data.backup_path=%s: %s",
				config.Server.Data.BackupPath,
				err,
			)
		}
	}

	// Check if config.Data.Path/config.Data.BackupPath exists and create one if it does not
	if _, err := os.Stat(config.Server.Data.Path); os.IsNotExist(err) {
		if err := os.MkdirAll(config.Server.Data.Path, 0750); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %s", err)
		}
	}
	if config.Server.Data.BackupPath != "" {
		if _, err := os.Stat(config.Server.Data.BackupPath); os.IsNotExist(err) {
			if err := os.MkdirAll(config.Server.Data.BackupPath, 0750); err != nil {
				return nil, fmt.Errorf("failed to create backup data directory: %s", err)
			}
		}
	}
	return config, nil
}
