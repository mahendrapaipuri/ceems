package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec
	"time"

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
	TargetsPath            string
	MaxRequests            int
	IncludeExporterMetrics bool
	EnableDebugServer      bool
	LandingConfig          *web.LandingConfig
}

// Config makes a server config.
type Config struct {
	Logger     *slog.Logger
	Collector  *CEEMSCollector
	Discoverer Discoverer
	Web        WebConfig
}

// CEEMSExporterServer struct implements HTTP server for exporter.
type CEEMSExporterServer struct {
	logger         *slog.Logger
	server         *http.Server
	webConfig      *web.FlagConfig
	collector      *CEEMSCollector
	discoverer     Discoverer
	metricsHandler *metricsHandler
	targetsHandler *targetsHandler
}

// metricsHandler wraps an metrics http.Handler.
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

// targetsHandler wraps an Alloy targets http.Handler.
type targetsHandler struct {
	handler     http.Handler
	maxRequests int
}

// ServeHTTP implements http.Handler.
func (h *targetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// NewCEEMSExporterServer creates new CEEMSExporterServer struct instance.
func NewCEEMSExporterServer(c *Config) (*CEEMSExporterServer, error) {
	router := mux.NewRouter()
	server := &CEEMSExporterServer{
		logger:     c.Logger,
		collector:  c.Collector,
		discoverer: c.Discoverer,
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
		metricsHandler: &metricsHandler{
			metricsRegistry:         prometheus.NewRegistry(),
			exporterMetricsRegistry: prometheus.NewRegistry(),
			includeExporterMetrics:  c.Web.IncludeExporterMetrics,
			maxRequests:             c.Web.MaxRequests,
		},
		targetsHandler: &targetsHandler{
			maxRequests: c.Web.MaxRequests,
		},
	}

	// Register exporter metrics when requested
	if c.Web.IncludeExporterMetrics {
		server.metricsHandler.exporterMetricsRegistry.MustRegister(
			promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}),
			promcollectors.NewGoCollector(),
		)
	}

	// Register metrics collector with Prometheus
	server.metricsHandler.metricsRegistry.MustRegister(version.NewCollector(CEEMSExporterAppName))

	if err := server.metricsHandler.metricsRegistry.Register(server.collector); err != nil {
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
	router.Handle(c.Web.MetricsPath, server.newMetricsHandler())

	// Handle targets path
	if c.Discoverer != nil && c.Discoverer.Enabled() {
		router.Handle(c.Web.TargetsPath, server.newTargetsHandler())
	}

	// Health endpoint
	router.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("CEEMS Exporter is healthy"))
	})

	// If EnableDebugServer is true add debug endpoints
	if c.Web.EnableDebugServer {
		// pprof debug end points. Expose them only on localhost
		router.PathPrefix("/debug/").Handler(http.DefaultServeMux).Methods(http.MethodGet).Host("localhost")
	}

	return server, nil
}

// Start launches CEEMS exporter HTTP server.
func (s *CEEMSExporterServer) Start() error {
	s.logger.Info("Starting " + CEEMSExporterAppName)

	if err := web.ListenAndServe(s.server, s.webConfig, s.logger); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("Failed to Listen and Serve HTTP server", "err", err)

		return err
	}

	return nil
}

// Shutdown stops CEEMS exporter HTTP server.
func (s *CEEMSExporterServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Stopping " + CEEMSExporterAppName)

	var errs error

	// First shutdown HTTP server to avoid accepting any incoming
	// connections
	// Do not return error here as we SHOULD ENSURE to close collectors
	// that might release any system resources
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Failed to stop exporter's HTTP server")

		errs = errors.Join(errs, err)
	}

	// Now close all collectors that release any system resources
	if err := s.collector.Close(ctx); err != nil {
		s.logger.Error("Failed to stop collector(s)")

		return errors.Join(errs, err)
	}

	return nil
}

// newMetricsHandler creates a new handler for exporting metrics.
func (s *CEEMSExporterServer) newMetricsHandler() http.Handler {
	var handler http.Handler
	if s.metricsHandler.includeExporterMetrics {
		handler = promhttp.HandlerFor(
			prometheus.Gatherers{s.metricsHandler.exporterMetricsRegistry, s.metricsHandler.metricsRegistry},
			promhttp.HandlerOpts{
				ErrorLog:            slog.NewLogLogger(s.logger.Handler(), slog.LevelError),
				ErrorHandling:       promhttp.ContinueOnError,
				MaxRequestsInFlight: s.metricsHandler.maxRequests,
				Registry:            s.metricsHandler.exporterMetricsRegistry,
			},
		)
		// Note that we have to use h.exporterMetricsRegistry here to
		// use the same promhttp metrics for all expositions.
		handler = promhttp.InstrumentMetricHandler(
			s.metricsHandler.exporterMetricsRegistry, handler,
		)
	} else {
		handler = promhttp.HandlerFor(
			s.metricsHandler.metricsRegistry,
			promhttp.HandlerOpts{
				ErrorLog:            slog.NewLogLogger(s.logger.Handler(), slog.LevelError),
				ErrorHandling:       promhttp.ContinueOnError,
				MaxRequestsInFlight: s.metricsHandler.maxRequests,
			},
		)
	}

	return handler
}

// newTargetsHandler creates a new handler for exporting Grafana Alloy targets.
func (s *CEEMSExporterServer) newTargetsHandler() http.Handler {
	return TargetsHandlerFor(
		s.discoverer,
		promhttp.HandlerOpts{
			ErrorLog:            slog.NewLogLogger(s.logger.Handler(), slog.LevelError),
			ErrorHandling:       promhttp.ContinueOnError,
			MaxRequestsInFlight: s.targetsHandler.maxRequests,
		},
	)
}
