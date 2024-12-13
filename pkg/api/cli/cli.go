//go:build cgo
// +build cgo

// Package cli implements the CLI of the CEEMS API server app
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/internal/common"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	ceems_db "github.com/mahendrapaipuri/ceems/pkg/api/db"
	ceems_http "github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// Custom errors.
var (
	ErrBackupInt = errors.New("back up interval of less than 1 day is not supported")
)

// CEEMSAPIAppConfig contains the configuration of CEEMS API server.
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
			Data: ceems_db.DataConfig{
				Path:            "data",
				RetentionPeriod: model.Duration(30 * 24 * time.Hour),
				UpdateInterval:  model.Duration(15 * time.Minute),
				BackupInterval:  model.Duration(24 * time.Hour),
				LastUpdateTime:  todayMidnight,
			},
			Admin: ceems_db.AdminConfig{
				Grafana: common.GrafanaWebConfig{
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
			Web: ceems_http.WebConfig{
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

// CEEMSAPIServerConfig contains the configuration of CEEMS API server.
type CEEMSAPIServerConfig struct {
	Data  ceems_db.DataConfig  `yaml:"data"`
	Admin ceems_db.AdminConfig `yaml:"admin"`
	Web   ceems_http.WebConfig `yaml:"web"`
}

// CEEMSServer represents the `ceems_server` cli.
type CEEMSServer struct {
	appName string
	App     kingpin.Application
}

// NewCEEMSServer creates a new CEEMSServer instance.
func NewCEEMSServer() (*CEEMSServer, error) {
	return &CEEMSServer{
		appName: base.CEEMSServerAppName,
		App:     base.CEEMSServerApp,
	}, nil
}

// Main is the entry point of the `ceems_server` command.
func (b *CEEMSServer) Main() error {
	var (
		webListenAddresses = b.App.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9020").Strings()
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

	promslogConfig := &promslog.Config{}
	flag.AddFlags(&b.App, promslogConfig)
	b.App.Version(version.Print(b.appName))
	b.App.UsageWriter(os.Stdout)
	b.App.HelpFlag.Short('h')

	_, err := b.App.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("failed to parse CLI flags: %w", err)
	}

	// Get absolute path for web config file if provided
	var webConfigFilePath string
	if *webConfigFile != "" {
		webConfigFilePath, err = filepath.Abs(*webConfigFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path of the web config file: %w", err)
		}
	}

	// Get absolute config file path global variable that will be used in resource manager
	// and updater packages
	base.ConfigFilePath, err = filepath.Abs(*configFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of the config file: %w", err)
	}

	// Make config from file
	config, err := common.MakeConfig[CEEMSAPIAppConfig](base.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}
	// Set directory for reading files
	config.SetDirectory(filepath.Dir(base.ConfigFilePath))
	// This is used only in tests
	config.Server.Data.SkipDeleteOldUnits = *skipDeleteOldUnits

	// Return error if backup interval of less than 1 day is used
	if time.Duration(config.Server.Data.BackupInterval) < 24*time.Hour && !*disableChecks {
		return ErrBackupInt
	}

	// Setup data directories
	if config, err = createDirs(config); err != nil {
		return err
	}

	// Set logger here after properly configuring promlog
	logger := promslog.New(promslogConfig)

	logger.Info("Starting "+b.appName, "version", version.Info())
	logger.Info(
		"Operational information", "build_context", version.BuildContext(),
		"host_details", internal_runtime.Uname(), "fd_limits", internal_runtime.FdLimits(),
	)

	runtime.GOMAXPROCS(*maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	if user, err := user.Current(); err == nil && user.Uid == "0" {
		logger.Info("CEEMS API server is running as root user. Privileges will be dropped and process will be run as unprivileged user")
	}

	// Make security related config
	// CEEMS API server should not need any privileges except executing SLURM sacct command.
	//
	// In future we should add SLURM API support as well which should avoid any privilege
	// requirements for CEEMS API server.
	//
	// Until then the required privileges should not be more than cap_setuid and cap_setgid.
	//
	// So we start with that assumption and during the resource manager instantitation, if
	// we are using sacct method, we keep the privilege or if the runtime config uses
	// future SLURM API support, we drop those privileges as well.
	//
	// So, we keep the privileges only as a insurance and once we confirm with resource manager
	// we decide to either keep them or drop them.
	var allCaps []cap.Value

	for _, name := range []string{"cap_setuid", "cap_setgid"} {
		value, err := cap.FromName(name)
		if err != nil {
			logger.Error("Error parsing capability %s: %w", name, err)

			continue
		}

		allCaps = append(allCaps, value)
	}

	if *dropPrivs {
		// We should STRONGLY advise in docs that CEEMS API server should not be started as root
		// as that will end up dropping the privileges and running it as nobody user which can
		// be strange as CEEMS API server writes data to DB.
		securityCfg := &security.Config{
			RunAsUser:      "nobody",
			Caps:           allCaps,
			ReadPaths:      []string{webConfigFilePath, base.ConfigFilePath},
			ReadWritePaths: []string{config.Server.Data.Path, config.Server.Data.BackupPath},
		}

		// Drop all unnecessary privileges
		if err := security.DropPrivileges(securityCfg); err != nil {
			return err
		}
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make DB config.
	dbConfig := &ceems_db.Config{
		Logger:          logger,
		Data:            config.Server.Data,
		Admin:           config.Server.Admin,
		ResourceManager: resource.New,
		Updater:         updater.New,
	}

	// Make server config.
	serverConfig := &ceems_http.Config{
		Logger: logger,
		Web: ceems_http.WebConfig{
			Addresses:        *webListenAddresses,
			WebSystemdSocket: *systemdSocket,
			WebConfigFile:    webConfigFilePath,
			RoutePrefix:      config.Server.Web.RoutePrefix,
			RequestsLimit:    config.Server.Web.RequestsLimit,
			MaxQueryPeriod:   config.Server.Web.MaxQueryPeriod,
		},
		DB: *dbConfig,
	}

	// Create server instance.
	apiServer, cleanup, err := ceems_http.New(serverConfig)
	defer cleanup()

	if err != nil {
		logger.Error("Failed to create ceems_server server", "err", err)

		return err
	}

	// Create DB instance.
	collector, err := ceems_db.New(dbConfig)
	if err != nil {
		logger.Error("Failed to create ceems_server DB", "err", err)

		return err
	}

	// Declare wait group and tickers.
	var wg sync.WaitGroup

	var dbUpdateTicker, dbBackupTicker *time.Ticker

	// Initialize tickers. We will stop the ticker immediately after signal has received.
	dbUpdateTicker = time.NewTicker(time.Duration(config.Server.Data.UpdateInterval))

	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			// This will ensure that we will run the method as soon as go routine
			// starts instead of waiting for ticker to tick.
			logger.Info("Updating CEEMS DB")

			if err := collector.Collect(ctx); err != nil {
				logger.Error("Failed to fetch data", "err", err)
			}

			select {
			case <-dbUpdateTicker.C:
				continue
			case <-ctx.Done():
				logger.Info("Received Interrupt. Stopping DB update")

				return
			}
		}
	}()

	// Start backup go routine only backup path is provided in CLI.
	if config.Server.Data.BackupPath != "" {
		// Initialise ticker and increase waitgroup counter.
		dbBackupTicker = time.NewTicker(time.Duration(config.Server.Data.BackupInterval))

		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-dbBackupTicker.C:
					// Dont run backup as soon as go routine is spawned. In prod, it
					// can take very long depending on the size of DB and so wait until
					// first tick to run it.
					logger.Info("Backing up CEEMS DB")

					if err := collector.Backup(ctx); err != nil {
						logger.Error("Failed to backup DB", "err", err)
					}
				case <-ctx.Done():
					logger.Info("Received Interrupt. Stopping DB backup")

					return
				}
			}
		}()
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below.
	go func() {
		if err := apiServer.Start(); err != nil {
			logger.Error("Failed to start server", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Stop tickers.
	dbUpdateTicker.Stop()

	if config.Server.Data.BackupPath != "" {
		dbBackupTicker.Stop()
	}

	// Wait for all DB go routines to finish.
	wg.Wait()

	// Close DB only after all DB go routines are done.
	if err := collector.Stop(); err != nil {
		logger.Error("Failed to close DB connection", "err", err)
	}

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	logger.Info("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(ctx); err != nil {
		logger.Error("Failed to gracefully shutdown server", "err", err)
	}

	logger.Info("Server exiting")
	logger.Info("See you next time!!")

	return nil
}

// createDirs makes data directories and set paths to absolute in config.
func createDirs(config *CEEMSAPIAppConfig) (*CEEMSAPIAppConfig, error) {
	var err error
	// Get absolute Data path
	config.Server.Data.Path, err = filepath.Abs(config.Server.Data.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for data.path=%s: %w", config.Server.Data.Path, err)
	}

	if config.Server.Data.BackupPath != "" {
		if config.Server.Data.BackupPath, err = filepath.Abs(config.Server.Data.BackupPath); err != nil {
			return nil, fmt.Errorf(
				"failed to get absolute path for data.backup_path=%s: %w",
				config.Server.Data.BackupPath,
				err,
			)
		}
	}

	// Check if config.Data.Path/config.Data.BackupPath exists and create one if it does not.
	if _, err := os.Stat(config.Server.Data.Path); os.IsNotExist(err) {
		if err := os.MkdirAll(config.Server.Data.Path, 0o750); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
	}

	if config.Server.Data.BackupPath != "" {
		if _, err := os.Stat(config.Server.Data.BackupPath); os.IsNotExist(err) {
			if err := os.MkdirAll(config.Server.Data.BackupPath, 0o750); err != nil {
				return nil, fmt.Errorf("failed to create backup data directory: %w", err)
			}
		}
	}

	return config, nil
}
