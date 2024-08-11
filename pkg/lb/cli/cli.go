// Package cli implements the CLI app of load balancer
package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/common"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	ceems_api "github.com/mahendrapaipuri/ceems/pkg/api/cli"
	ceems_http "github.com/mahendrapaipuri/ceems/pkg/api/http"
	ceems_api_models "github.com/mahendrapaipuri/ceems/pkg/api/models"
	tsdb "github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/lb/frontend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

// Custom errors.
var (
	ErrMissingIDs  = errors.New("missing ID for backend(s)")
	ErrMissingURLs = errors.New("missing TSDB URL(s) for backend(s)")
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

		if len(backend.URLs) == 0 {
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
func (c *CEEMSLBAppConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
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
	Backends []base.Backend `yaml:"backends"`
	Strategy string         `yaml:"strategy"`
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
	var (
		webListenAddresses = lb.App.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9030").String()
		webConfigFile = lb.App.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Envar("CEEMS_LB_WEB_CONFIG_FILE").Default("").String()
		configFile = lb.App.Flag(
			"config.file",
			"Configuration file path.",
		).Envar("CEEMS_LB_CONFIG_FILE").Default("").String()
		maxProcs = lb.App.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)

	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }() //nolint:nlreturn
	if runtime.GOOS == "linux" {
		systemdSocket = lb.App.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Bool()
	}

	promlogConfig := &promlog.Config{}
	flag.AddFlags(&lb.App, promlogConfig)
	lb.App.Version(version.Print(lb.appName))
	lb.App.UsageWriter(os.Stdout)
	lb.App.HelpFlag.Short('h')

	_, err := lb.App.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("failed to parse CLI flags: %w", err)
	}

	// Make LB config
	config, err := common.MakeConfig[CEEMSLBAppConfig](*configFile)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", "Starting "+lb.appName, "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", internal_runtime.Uname())
	level.Info(logger).Log("fd_limits", internal_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create a pool of backend TSDB servers
	manager, err := serverpool.New(config.LB.Strategy, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create backend TSDB server pool", "err", err)

		return err
	}

	// Create frontend config
	frontendConfig := &frontend.Config{
		Logger:           logger,
		Address:          *webListenAddresses,
		WebSystemdSocket: *systemdSocket,
		WebConfigFile:    *webConfigFile,
		APIServer:        config.Server,
		Manager:          manager,
	}

	// Create frontend instance
	loadBalancer, err := frontend.New(frontendConfig)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create load balancer frontend", "err", err)

		return err
	}

	// Add backend TSDB servers to serverPool
	for _, backend := range config.LB.Backends {
		for _, backendURL := range backend.URLs {
			webURL, err := url.Parse(backendURL)
			if err != nil {
				// If we dont unwrap original error, the URL string will be printed to log which
				// might contain sensitive passwords
				level.Error(logger).Log("msg", "Could not parse backend TSDB server URL", "err", errors.Unwrap(err))

				continue
			}

			rp := httputil.NewSingleHostReverseProxy(webURL)
			backendServer := tsdb.New(webURL, rp, logger)
			rp.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
				level.Error(logger).Log("msg", "Failed to handle the request", "host", webURL.Host, "err", err)
				backendServer.SetAlive(false)

				// If already retried the request, return error
				if !frontend.AllowRetry(request) {
					level.Info(logger).
						Log("msg", "Max retry attempts reached, terminating", "address", request.RemoteAddr, "path", request.URL.Path)
					http.Error(writer, "Service not available", http.StatusServiceUnavailable)

					return
				}

				// Retry request and set context value so that we dont retry for second time
				level.Info(logger).
					Log("msg", "Attempting retry", "address", request.RemoteAddr, "path", request.URL.Path)
				loadBalancer.Serve(
					writer,
					request.WithContext(
						context.WithValue(request.Context(), frontend.RetryContextKey{}, true),
					),
				)
			}

			manager.Add(backend.ID, backendServer)
		}
	}

	// Validate configured cluster IDs against the ones in CEEMS DB
	if err := loadBalancer.ValidateClusterIDs(ctx); err != nil {
		return err
	}

	// Declare wait group and tickers
	var wg sync.WaitGroup

	// Spawn a go routine to do health checks of backend TSDB servers
	wg.Add(1)

	go func() {
		defer wg.Done()
		frontend.Monitor(ctx, manager, logger)
	}()

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := loadBalancer.Start(); err != nil {
			level.Error(logger).Log("msg", "Failed to start load balancer", "err", err)
		}
	}()

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Wait for all DB go routines to finish
	wg.Wait()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	level.Info(logger).Log("msg", "Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := loadBalancer.Shutdown(shutDownCtx); err != nil {
		level.Error(logger).Log("msg", "Failed to gracefully shutdown server", "err", err)
	}

	level.Info(logger).Log("msg", "Load balancer exiting")
	level.Info(logger).Log("msg", "See you next time!!")

	return nil
}
