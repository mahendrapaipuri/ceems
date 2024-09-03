package collector

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	_ "net/http/pprof" // #nosec
	"sort"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
)

// WebConfig makes HTTP web config from CLI args.
type WebConfig struct {
	Addresses              []string
	WebSystemdSocket       bool
	WebConfigFile          string
	MetricsPath            string
	MaxRequests            int
	IncludeExporterMetrics bool
	EnableDebugServer      bool
	LandingConfig          *web.LandingConfig
}

// Config makes a server config.
type Config struct {
	Logger log.Logger
	Web    WebConfig
}

// CEEMSExporterServer struct implements HTTP server for exporter.
type CEEMSExporterServer struct {
	logger    log.Logger
	server    *http.Server
	webConfig *web.FlagConfig
	collector *CEEMSCollector
	handler   *metricsHandler
}

// metricsHandler wraps an metrics http.Handler. Create instances with
// newHandler.
type metricsHandler struct {
	handler http.Handler
	// exporterMetricsRegistry is a separate registry for the metrics about
	// the exporter itself.
	metricsRegistry         *prometheus.Registry
	exporterMetricsRegistry *prometheus.Registry
	includeExporterMetrics  bool
	maxRequests             int
}

// ServeHTTP implements http.Handler.
func (h *metricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// NewCEEMSExporterServer creates new CEEMSExporterServer struct instance.
func NewCEEMSExporterServer(c *Config) (*CEEMSExporterServer, error) {
	var err error

	router := mux.NewRouter()
	server := &CEEMSExporterServer{
		logger: c.Logger,
		server: &http.Server{
			Addr:              c.Web.Addresses[0],
			Handler:           router,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			ReadHeaderTimeout: 2 * time.Second, // slowloris attack: https://app.deepsource.com/directory/analyzers/go/issues/GO-S2112
		},
		webConfig: &web.FlagConfig{
			WebListenAddresses: &c.Web.Addresses,
			WebSystemdSocket:   &c.Web.WebSystemdSocket,
			WebConfigFile:      &c.Web.WebConfigFile,
		},
		handler: &metricsHandler{
			metricsRegistry:         prometheus.NewRegistry(),
			exporterMetricsRegistry: prometheus.NewRegistry(),
			includeExporterMetrics:  c.Web.IncludeExporterMetrics,
			maxRequests:             c.Web.MaxRequests,
		},
	}

	// Register exporter metrics when requested
	if c.Web.IncludeExporterMetrics {
		server.handler.exporterMetricsRegistry.MustRegister(
			promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}),
			promcollectors.NewGoCollector(),
		)
	}

	// Create a new instance of collector
	server.collector, err = NewCEEMSCollector(c.Logger)
	if err != nil {
		return nil, fmt.Errorf("couldn't create collector: %w", err)
	}

	// Log all enabled collectors
	level.Info(c.Logger).Log("msg", "Enabled collectors")

	collectors := []string{}
	for n := range server.collector.Collectors {
		collectors = append(collectors, n)
	}

	sort.Strings(collectors)

	for _, coll := range collectors {
		level.Info(c.Logger).Log("collector", coll)
	}

	// Register metrics collector with Prometheus
	server.handler.metricsRegistry.MustRegister(version.NewCollector(CEEMSExporterAppName))

	if err := server.handler.metricsRegistry.Register(server.collector); err != nil {
		return nil, fmt.Errorf("couldn't register compute resource collector: %w", err)
	}

	// Landing page
	if c.Web.MetricsPath != "/" {
		landingPage, err := web.NewLandingPage(*c.Web.LandingConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create landing page: %w", err)
		}

		router.Handle("/", landingPage)
	}

	// Handle metrics path
	router.Handle(c.Web.MetricsPath, server.metricsHandler())

	// If EnableDebugServer is true add debug endpoints
	if c.Web.EnableDebugServer {
		// pprof debug end points. Expose them only on localhost
		router.PathPrefix("/debug/").Handler(http.DefaultServeMux).Methods(http.MethodGet).Host("localhost")
	}

	return server, nil
}

// Start launches CEEMS exporter HTTP server.
func (s *CEEMSExporterServer) Start() error {
	level.Info(s.logger).Log("msg", "Starting "+CEEMSExporterAppName)

	if err := web.ListenAndServe(s.server, s.webConfig, s.logger); err != nil && !errors.Is(err, http.ErrServerClosed) {
		level.Error(s.logger).Log("msg", "Failed to Listen and Serve HTTP server", "err", err)

		return err
	}

	return nil
}

// Shutdown stops CEEMS exporter HTTP server.
func (s *CEEMSExporterServer) Shutdown(ctx context.Context) error {
	level.Info(s.logger).Log("msg", "Stopping "+CEEMSExporterAppName)

	var errs error

	// First shutdown HTTP server to avoid accepting any incoming
	// connections
	// Do not return error here as we SHOULD ENSURE to close collectors
	// that might release any system resources
	if err := s.server.Shutdown(ctx); err != nil {
		level.Error(s.logger).Log("msg", "Failed to stop exporter's HTTP server")

		errs = errors.Join(errs, err)
	}

	// Now close all collectors that release any system resources
	if err := s.collector.Close(ctx); err != nil {
		level.Error(s.logger).Log("msg", "Failed to stop collector(s)")

		return errors.Join(errs, err)
	}

	return nil
}

// metricsHandler creates a new handler for exporting metrics.
func (s *CEEMSExporterServer) metricsHandler() http.Handler {
	var handler http.Handler
	if s.handler.includeExporterMetrics {
		handler = promhttp.HandlerFor(
			prometheus.Gatherers{s.handler.exporterMetricsRegistry, s.handler.metricsRegistry},
			promhttp.HandlerOpts{
				ErrorLog:            stdlog.New(log.NewStdlibAdapter(level.Error(s.logger)), "", 0),
				ErrorHandling:       promhttp.ContinueOnError,
				MaxRequestsInFlight: s.handler.maxRequests,
				Registry:            s.handler.exporterMetricsRegistry,
			},
		)
		// Note that we have to use h.exporterMetricsRegistry here to
		// use the same promhttp metrics for all expositions.
		handler = promhttp.InstrumentMetricHandler(
			s.handler.exporterMetricsRegistry, handler,
		)
	} else {
		handler = promhttp.HandlerFor(
			s.handler.metricsRegistry,
			promhttp.HandlerOpts{
				ErrorLog:            stdlog.New(log.NewStdlibAdapter(level.Error(s.logger)), "", 0),
				ErrorHandling:       promhttp.ContinueOnError,
				MaxRequestsInFlight: s.handler.maxRequests,
			},
		)
	}

	return handler
}
