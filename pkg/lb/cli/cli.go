//go:build cgo
// +build cgo

// Package cli implements the CLI app of load balancer
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/ceems-dev/ceems/internal/common"
	internal_runtime "github.com/ceems-dev/ceems/internal/runtime"
	"github.com/ceems-dev/ceems/internal/security"
	ceems_api_base "github.com/ceems-dev/ceems/pkg/api/base"
	ceems_api "github.com/ceems-dev/ceems/pkg/api/cli"
	ceems_http "github.com/ceems-dev/ceems/pkg/api/http"
	ceems_api_models "github.com/ceems-dev/ceems/pkg/api/models"
	lb_backend "github.com/ceems-dev/ceems/pkg/lb/backend"
	"github.com/ceems-dev/ceems/pkg/lb/base"
	"github.com/ceems-dev/ceems/pkg/lb/frontend"
	"github.com/ceems-dev/ceems/pkg/lb/serverpool"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
)

// Custom errors.
var (
	ErrMissingIDs  = errors.New("missing ID for backend(s)")
	ErrMissingURLs = errors.New("missing TSDB and Pyroscope URL(s) for backend(s)")
)

// CEEMSLBAppConfig contains the configuration of CEEMS load balancer app.
type CEEMSLBAppConfig struct {
	LB       CEEMSLBConfig                  `yaml:"ceems_lb"`
	Server   ceems_api.CEEMSAPIServerConfig `yaml:"ceems_api_server"`
	Clusters []ceems_api_models.Cluster     `yaml:"clusters"`
}

// SetDirectory joins any relative file paths with dir.
func (c *CEEMSLBAppConfig) SetDirectory(dir string) {
	c.Server.Web.HTTPClientConfig.SetDirectory(dir)
}

// Validate valides the CEEMS LB config to check if backend servers have IDs set.
func (c *CEEMSLBAppConfig) Validate() error {
	// Fetch all cluster IDs
	var clusterIDs []string

	for _, cluster := range c.Clusters {
		if cluster.ID != "" {
			clusterIDs = append(clusterIDs, cluster.ID)
		}
	}

	// Preflight checks for backends
	for _, backend := range c.LB.Backends {
		if backend.ID == "" {
			return ErrMissingIDs
		}

		if len(backend.TSDBs) == 0 && len(backend.Pyros) == 0 {
			return ErrMissingURLs
		}

		// Clusters config is not always present. Validate only when it is available
		if len(clusterIDs) > 0 && !slices.Contains(clusterIDs, backend.ID) {
			return fmt.Errorf(
				"unknown ID %s found in backends. IDs in ceems_lb.backends and in clusters config should match",
				backend.ID,
			)
		}
	}

	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *CEEMSLBAppConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Set a default config
	*c = CEEMSLBAppConfig{
		CEEMSLBConfig{
			Strategy: "round-robin",
		},
		ceems_api.CEEMSAPIServerConfig{
			Web: ceems_http.WebConfig{
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
		},
		[]ceems_api_models.Cluster{},
	}

	type plain CEEMSLBAppConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Validate backend servers config
	if err := c.Validate(); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.Server.Web.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return nil
}

// CEEMSLBConfig contains the CEEMS load balancer config.
type CEEMSLBConfig struct {
	Backends []lb_backend.Backend `yaml:"backends"`
	Strategy string               `yaml:"strategy"`
}

// CEEMSLoadBalancer represents the `ceems_lb` cli.
type CEEMSLoadBalancer struct {
	appName string
	App     kingpin.Application
}

// NewCEEMSLoadBalancer returns a new CEEMSLoadBalancer instance.
func NewCEEMSLoadBalancer() (*CEEMSLoadBalancer, error) {
	return &CEEMSLoadBalancer{
		appName: base.CEEMSLoadBalancerAppName,
		App:     base.CEEMSLoadBalancerApp,
	}, nil
}

// Main is the entry point of the `ceems_lb` command.
func (lb *CEEMSLoadBalancer) Main() error {
	// CLI vars.
	var (
		configFile, webConfigFile, runAsUser                               string
		configExpandEnvVars, disableCapAwareness, dropPrivs, systemdSocket bool
		webListenAddresses                                                 []string
		maxProcs                                                           int
	)

	// Get default run as user
	defaultRunAsUser, err := security.GetDefaultRunAsUser()
	if err != nil {
		return err
	}

	lb.App.Flag(
		"config.file",
		"Configuration file path.",
	).Envar("CEEMS_LB_CONFIG_FILE").Default("").StringVar(&configFile)
	lb.App.Flag(
		"config.file.expand-env-vars",
		"Any environment variables that are referenced in config file will be expanded. To escape $ use $$ (default: false).",
	).Default("false").BoolVar(&configExpandEnvVars)

	lb.App.Flag(
		"web.listen-address",
		"Addresses on which to expose load balancer(s). When both TSDB and Pyroscope LBs are configured, it must be "+
			"repeated to provide two addresses: one for TSDB LB and one for Pyroscope LB. In that case TSDB LB will listen on "+
			"first address and Pyroscope LB on second address",
	).Default(":9030", ":9040").StringsVar(&webListenAddresses)
	lb.App.Flag(
		"web.config.file",
		"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
	).Envar("CEEMS_LB_WEB_CONFIG_FILE").Default("").StringVar(&webConfigFile)
	lb.App.Flag(
		"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
	).Envar("GOMAXPROCS").Default("1").IntVar(&maxProcs)

	lb.App.Flag(
		"security.run-as-user",
		"LB server will be run under this user. Accepts either a username or uid. If current user is unprivileged, same user "+
			"will be used. When LB server is started as root, by default user will be changed to nobody. To be able to change the user necessary "+
			"capabilities (CAP_SETUID, CAP_SETGID) must exist on the process.",
	).Default(defaultRunAsUser).StringVar(&runAsUser)

	// Hidden test flags
	lb.App.Flag(
		"security.disable-cap-awareness",
		"Disable capability awareness and run as privileged process (default: false).",
	).Default("false").Hidden().BoolVar(&disableCapAwareness)
	lb.App.Flag(
		"security.drop-privileges",
		"Drop privileges and run as nobody when exporter is started as root.",
	).Default("true").Hidden().BoolVar(&dropPrivs)

	// Socket activation only available on Linux
	if runtime.GOOS == "linux" {
		lb.App.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Default("false").BoolVar(&systemdSocket)
	}

	promslogConfig := &promslog.Config{}
	flag.AddFlags(&lb.App, promslogConfig)
	lb.App.Version(version.Print(lb.appName))
	lb.App.UsageWriter(os.Stdout)
	lb.App.HelpFlag.Short('h')

	_, err = lb.App.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("failed to parse CLI flags: %w", err)
	}

	// Get absolute path for web config file if provided
	var webConfigFilePath string
	if webConfigFile != "" {
		webConfigFilePath, err = filepath.Abs(webConfigFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path of the web config file: %w", err)
		}
	}

	// Get absolute config file path global variable that will be used in resource manager
	// and updater packages
	configFilePath, err := filepath.Abs(configFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of the config file: %w", err)
	}

	// Make LB config
	config, err := common.MakeConfig[CEEMSLBAppConfig](configFilePath, configExpandEnvVars)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set logger here after properly configuring promlog
	logger := promslog.New(promslogConfig)

	logger.Info("Starting "+lb.appName, "version", version.Info())
	logger.Info(
		"Operational information", "build_context", version.BuildContext(),
		"host_details", internal_runtime.Uname(), "fd_limits", internal_runtime.FdLimits(),
	)

	runtime.GOMAXPROCS(maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// We should STRONGLY advise in docs that CEEMS LB should not be started as root.
	securityCfg := &security.Config{
		RunAsUser: runAsUser,
		Caps:      nil,
		ReadPaths: []string{webConfigFilePath, configFilePath},
	}

	// Check if DB path and file exists in config and add them to ReadPaths
	if _, err := os.Stat(config.Server.Data.Path); err == nil {
		securityCfg.ReadPaths = append(securityCfg.ReadPaths, config.Server.Data.Path)

		// Now check if DB file exists
		dbFile := filepath.Join(config.Server.Data.Path, ceems_api_base.CEEMSDBName)
		if _, err := os.Stat(dbFile); err == nil {
			securityCfg.ReadPaths = append(securityCfg.ReadPaths, dbFile)
		}
	}

	// Start a new manager
	securityManager, err := security.NewManager(securityCfg, logger)
	if err != nil {
		logger.Error("Failed to create a new security manager", "err", err)

		return err
	}

	// Drop all unnecessary privileges
	if dropPrivs {
		if err := securityManager.DropPrivileges(disableCapAwareness); err != nil {
			logger.Error("Failed to drop privileges", "err", err)

			return err
		}
	}

	// Get list of LB types based on provided config
	lbTypes := backendTypes(config)

	var lbNames []string

	for _, t := range lbTypes {
		lbNames = append(lbNames, t.String())
	}

	logger.Info("Load balancers: " + strings.Join(lbNames, ", "))

	// Ensure that enough web listen addresses are provided
	webListenAddrs := webListenAddresses
	if len(lbTypes) > len(webListenAddrs) {
		logger.Error("Missing web listen addresses", "num_lbs", len(lbTypes), "num_addrs", len(webListenAddrs))

		return fmt.Errorf("insufficient --web.listen-address. Expected %d got %d", len(lbTypes), len(webListenAddrs))
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make manager and LB maps
	managers := make(map[base.LBType]serverpool.Manager, 2)
	lbs := make(map[base.LBType]frontend.LoadBalancer, 2)

	for i, lbType := range lbTypes {
		// Create a pool of backend servers
		managers[lbType], err = serverpool.New(base.LBStrategyMap[config.LB.Strategy], logger.With("backend_type", lbType))
		if err != nil {
			logger.Error("Failed to create backend server poo", "backend_type", lbType, "err", err)

			return err
		}

		// Add backend servers to serverPool
		for _, backend := range config.LB.Backends {
			for _, serverCfg := range backendURLs(lbType, backend) {
				// Set directory for reading files
				serverCfg.Web.SetDirectory(filepath.Dir(webConfigFilePath))

				backendServer, err := lb_backend.New(lbType, serverCfg, logger.With("backend_type", lbType))
				if err != nil {
					logger.Error("Could not set up backend server", "backend_type", lbType, "err", errors.Unwrap(err))

					continue
				}

				managers[lbType].Add(backend.ID, backendServer)
			}
		}

		// Create frontend config for load balancer
		frontendConfig := &frontend.Config{
			Logger:           logger.With("backend_type", lbType),
			LBType:           lbType,
			Address:          webListenAddrs[i],
			WebSystemdSocket: systemdSocket,
			WebConfigFile:    webConfigFilePath,
			APIServer:        config.Server,
			Manager:          managers[lbType],
		}

		// Create frontend instance for load balancer
		lbs[lbType], err = frontend.New(frontendConfig)
		if err != nil {
			logger.Error("Failed to create load balancer frontend", "backend_type", lbType, "err", err)

			return err
		}
	}

	// Declare wait group and tickers
	var wg sync.WaitGroup

	for _, lbType := range lbTypes {
		// Spawn a go routine to do health checks of backend TSDB servers
		wg.Add(1)

		go func() {
			defer wg.Done()
			monitor(ctx, managers[lbType], logger.With("backend_type", lbType))
		}()

		// Initializing the server in a goroutine so that
		// it won't block the graceful shutdown handling below
		go func() {
			if err := lbs[lbType].Start(ctx); err != nil {
				logger.Error("Failed to start load balancer", "backend_type", lbType, "err", err)
			}
		}()
	}

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Wait for all DB go routines to finish
	wg.Wait()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	logger.Info("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, lbType := range lbTypes {
		if err := lbs[lbType].Shutdown(shutDownCtx); err != nil {
			logger.Error("Failed to gracefully shutdown LB server", "backend_type", lbType, "err", err)
		}
	}

	// Restore file permissions by removing any ACLs added
	if err := securityManager.DeleteACLEntries(); err != nil {
		logger.Error("Failed to remove ACL entries", "err", err)
	}

	logger.Info("Load balancer(s) exiting")
	logger.Info("See you next time!!")

	return nil
}

// backendTypes returns LB backend types in the current config.
func backendTypes(config *CEEMSLBAppConfig) []base.LBType {
	var types []base.LBType
	for _, backend := range config.LB.Backends {
		if len(backend.TSDBs) > 0 && !slices.Contains(types, base.PromLB) {
			types = append(types, base.PromLB)
		}

		if len(backend.Pyros) > 0 && !slices.Contains(types, base.PyroLB) {
			types = append(types, base.PyroLB)
		}
	}

	return types
}

// backendURLs returns slice of backend URLs based on backend type `t`.
func backendURLs(t base.LBType, backend lb_backend.Backend) []*lb_backend.ServerConfig {
	switch t {
	case base.PromLB:
		return backend.TSDBs
	case base.PyroLB:
		return backend.Pyros
	}

	return nil
}

// monitor checks the backend servers health.
func monitor(ctx context.Context, manager serverpool.Manager, logger *slog.Logger) {
	t := time.NewTicker(time.Second * 20)

	logger.Info("Starting health checker")

	for {
		// This will ensure that we will run the method as soon as go routine
		// starts instead of waiting for ticker to tick
		go healthCheck(ctx, manager, logger)

		select {
		case <-t.C:
			continue
		case <-ctx.Done():
			logger.Info("Received Interrupt. Stopping health checker")

			return
		}
	}
}

// healthCheck monitors the status of all backend servers.
func healthCheck(ctx context.Context, manager serverpool.Manager, logger *slog.Logger) {
	aliveChannel := make(chan bool, 1)

	for id, backends := range manager.Backends() {
		for _, backend := range backends {
			requestCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()

			go isAlive(requestCtx, aliveChannel, backend.URL(), logger)

			select {
			case <-ctx.Done():
				logger.Info("Gracefully shutting down health check")

				return
			case alive := <-aliveChannel:
				backend.SetAlive(alive)

				if !alive {
					logger.Error("Health check", "id", id, "backend", backend.String(), "status", "down")
				} else {
					logger.Debug("Health check", "id", id, "backend", backend.String(), "status", "up")
				}
			}
		}
	}
}

// isAlive returns the status of backend server with a channel.
func isAlive(ctx context.Context, aliveChannel chan bool, u *url.URL, logger *slog.Logger) {
	var d net.Dialer

	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		logger.Debug("Backend unreachable", "backend", u.Redacted(), "err", err)
		aliveChannel <- false

		return
	}

	_ = conn.Close()
	aliveChannel <- true
}
