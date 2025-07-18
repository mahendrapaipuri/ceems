package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec
	"time"

	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/admit"
	"github.com/ceems-dev/ceems/cmd/ceems_k8s_admission_controller/base"
	"github.com/gorilla/mux"
	"github.com/prometheus/exporter-toolkit/web"
)

const (
	mutatePath   = "/ceems-admission-controller/mutate"
	validatePath = "/ceems-admission-controller/validate"
)

// AdmissionControllerServer struct implements HTTP server for k8s admission controller.
type AdmissionControllerServer struct {
	logger    *slog.Logger
	server    *http.Server
	webConfig *web.FlagConfig
}

// NewAdmissionControllerServer creates new AdmissionControllerServer struct instance.
func NewAdmissionControllerServer(c *base.Config) (*AdmissionControllerServer, error) {
	router := mux.NewRouter()
	server := &AdmissionControllerServer{
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
	}

	// If EnableDebugServer is true add debug endpoints
	if c.Web.EnableDebugServer {
		// pprof debug end points. Expose them only on localhost
		router.PathPrefix("/debug/").Handler(http.DefaultServeMux).Methods(http.MethodGet).Host("localhost")
	}

	// Instances hooks
	objsValidation := admit.NewValidationHook()
	objsMutation := admit.NewMutationHook()

	// Create a new handler
	handler, err := newAdmissionHandler(c.Logger)
	if err != nil {
		c.Logger.Error("Failed to create new admission handler", "err", err)

		return nil, fmt.Errorf("failed to make new admission handler: %w", err)
	}

	// Handle metrics path
	router.Path("/health").HandlerFunc(server.health()).Methods(http.MethodGet)
	router.Path(validatePath).HandlerFunc(handler.Serve(objsValidation)).Methods(http.MethodPost)
	router.Path(mutatePath).HandlerFunc(handler.Serve(objsMutation)).Methods(http.MethodPost)

	return server, nil
}

// Start launches admission controller HTTP server.
func (s *AdmissionControllerServer) Start() error {
	s.logger.Info("Starting " + base.AppName)

	if err := web.ListenAndServe(s.server, s.webConfig, s.logger); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("Failed to Listen and Serve HTTP server", "err", err)

		return err
	}

	return nil
}

// Shutdown stops admission controller HTTP server.
func (s *AdmissionControllerServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Stopping " + base.AppName)

	// First shutdown HTTP server to avoid accepting any incoming
	// connections
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Failed to stop exporter's HTTP server")

		return err
	}

	return nil
}

// health reports the health status of server.
func (s *AdmissionControllerServer) health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("CEEMS k8s Admission Controller is healthy"))
	}
}
