// Package cli implements the CLI app of load balancer
package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	tsdb "github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/lb/frontend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
)

// CEEMSLoadBalancer represents the `ceems_lb` cli.
type CEEMSLoadBalancer struct {
	appName string
	App     kingpin.Application
}

// NewCEEMSLoadBalancer returns a new CEEMSLoadBalancer instance
func NewCEEMSLoadBalancer() (*CEEMSLoadBalancer, error) {
	return &CEEMSLoadBalancer{
		appName: base.CEEMSLoadBalancerAppName,
		App:     base.CEEMSLoadBalancerApp,
	}, nil
}

// Main is the entry point of the `ceems_lb` command
func (lb *CEEMSLoadBalancer) Main() error {
	var (
		webListenAddresses = lb.App.Flag(
			"web.listen-address",
			"Addresses on which to expose metrics and web interface.",
		).Default(":9030").String()
		webConfigFile = lb.App.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Default("").String()
		configFile = lb.App.Flag(
			"config.file",
			"Config file containing backend server web URLs.",
		).Default("").String()
		maxProcs = lb.App.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
	)

	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }()
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
		return fmt.Errorf("failed to parse CLI flags: %s", err)
	}

	// Check if config file arg is present
	if *configFile == "" {
		return fmt.Errorf("--config.file is empty. Provide a valid config file")
	}

	// Get LB config
	config, err := getLBConfig(*configFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %s", err)
	}

	// Set logger here after properly configuring promlog
	logger := promlog.New(promlogConfig)

	level.Info(logger).Log("msg", fmt.Sprintf("Starting %s", lb.appName), "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())
	level.Info(logger).Log("fd_limits", internal_runtime.Uname())
	level.Info(logger).Log("fd_limits", internal_runtime.FdLimits())

	runtime.GOMAXPROCS(*maxProcs)
	level.Debug(logger).Log("msg", "Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create a pool of backend TSDB servers
	manager, err := serverpool.NewManager(config.Strategy)
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
		DBPath:           config.DBPath,
		Manager:          manager,
	}

	// Create frontend instance
	loadBalancer, err := frontend.NewLoadBalancer(frontendConfig)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create load balancer frontend", "err", err)
		return err
	}

	// Add backend TSDB servers to serverPool
	for _, backend := range config.Backends {
		webURL, err := url.Parse(backend.URL)
		if err != nil {
			level.Error(logger).Log("msg", "Could not parse backend TSDB server URL", "url", webURL, "err", err)
			continue
		}

		rp := httputil.NewSingleHostReverseProxy(webURL)
		backendServer := tsdb.NewTSDBServer(webURL, backend.SkipTLSVerify, rp)
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
			level.Info(logger).Log("msg", "Attempting retry", "address", request.RemoteAddr, "path", request.URL.Path)
			loadBalancer.Serve(
				writer,
				request.WithContext(
					context.WithValue(request.Context(), frontend.RetryContextKey{}, true),
				),
			)
		}
		manager.Add(backendServer)
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
