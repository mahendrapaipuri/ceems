//go:build cgo
// +build cgo

// Package frontend implements the frontend server of the load balancer
package frontend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	ceems_api_base "github.com/mahendrapaipuri/ceems/pkg/api/base"
	ceems_api_cli "github.com/mahendrapaipuri/ceems/pkg/api/cli"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
)

// Custom errors.
var (
	ErrUnknownClusterID = errors.New("unknown cluster ID")
)

// RetryContextKey is the key used to set context value for retry.
type RetryContextKey struct{}

// ReqParamsContextKey is the key used to set context value for request parameters.
type ReqParamsContextKey struct{}

// ReqParams is the context value.
type ReqParams struct {
	clusterID   string
	uuids       []string
	time        int64
	queryPeriod time.Duration
}

// LoadBalancer is the interface to implement.
type LoadBalancer interface {
	Serve(w http.ResponseWriter, r *http.Request)
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// Config makes a server config from CLI args.
type Config struct {
	Logger           *slog.Logger
	LBType           base.LBType
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	APIServer        ceems_api_cli.CEEMSAPIServerConfig
	Manager          serverpool.Manager
}

// loadBalancer struct.
type loadBalancer struct {
	logger    *slog.Logger
	lbType    base.LBType
	manager   serverpool.Manager
	server    *http.Server
	webConfig *web.FlagConfig
	amw       *authenticationMiddleware
}

// New returns a new instance of load balancer.
func New(c *Config) (LoadBalancer, error) {
	// Setup new auth middleware
	amw, err := newAuthMiddleware(c)
	if err != nil {
		return nil, fmt.Errorf("failed to setup auth middleware: %w", err)
	}

	// Instantiate LB struct
	lb := &loadBalancer{
		logger: c.Logger,
		lbType: c.LBType,
		server: &http.Server{
			Addr:              c.Address,
			ReadHeaderTimeout: 2 * time.Second, // slowloris attack: https://app.deepsource.com/directory/analyzers/go/issues/GO-S2112
		},
		webConfig: &web.FlagConfig{
			WebListenAddresses: &[]string{c.Address},
			WebSystemdSocket:   &c.WebSystemdSocket,
			WebConfigFile:      &c.WebConfigFile,
		},
		manager: c.Manager,
		amw:     amw,
	}

	// Setup a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Validate LB
	if err := lb.validate(ctx); err != nil {
		return nil, fmt.Errorf("failed to valiate load balancer frontend: %w", err)
	}

	// Setup error handlers for backend servers
	lb.errorHandlers()

	return lb, nil
}

// errorHandlers sets up error handlers for backend servers.
func (lb *loadBalancer) errorHandlers() {
	// Iterate over all backend servers
	for _, backends := range lb.manager.Backends() {
		for _, backend := range backends {
			backend.ReverseProxy().ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
				lb.logger.Error("Failed to handle the request", "host", backend.URL().Host, "err", err)
				backend.SetAlive(false)

				// If already retried the request, return error
				if !allowRetry(request) {
					lb.logger.Info("Max retry attempts reached, terminating", "address", request.RemoteAddr, "path", request.URL.Path)
					http.Error(writer, "Service not available", http.StatusServiceUnavailable)

					return
				}

				// Retry request and set context value so that we dont retry for second time
				lb.logger.Info("Attempting retry", "address", request.RemoteAddr, "path", request.URL.Path)
				lb.Serve(
					writer,
					request.WithContext(
						context.WithValue(request.Context(), RetryContextKey{}, true),
					),
				)
			}
		}
	}
}

// validate validates the cluster IDs by checking them against DB.
func (lb *loadBalancer) validate(ctx context.Context) error {
	// Fetch all cluster IDs set in config file
	for id := range lb.manager.Backends() {
		lb.amw.clusterIDs = append(lb.amw.clusterIDs, id)
	}

	// If neither CEEMD DB or API server is configured, return
	// This means LB is used without any access control configured
	if lb.amw.ceems.db == nil && lb.amw.ceems.clustersEndpoint() == nil {
		return nil
	}

	// If DB file is accessible, check directly from DB
	var clusters []models.Cluster

	if lb.amw.ceems.db != nil {
		//nolint:gosec
		rows, err := lb.amw.ceems.db.QueryContext(
			ctx, "SELECT DISTINCT cluster_id, resource_manager FROM "+ceems_api_base.UnitsDBTableName,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		var cluster models.Cluster
		for rows.Next() {
			if err := rows.Scan(&cluster.ID, &cluster.Manager); err != nil {
				continue
			}

			clusters = append(clusters, cluster)
		}

		// Ref: http://go-database-sql.org/errors.html
		// Get all the errors during iteration
		if err := rows.Err(); err != nil {
			lb.logger.Error("Errors during scanning rows", "err", err)
		}

		goto validate
	}

	if lb.amw.ceems.clustersEndpoint() != nil {
		// Create a new GET request
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, lb.amw.ceems.clustersEndpoint().String(), nil)
		if err != nil {
			return err
		}

		// Add necessary headers
		// Use service account as user which will be in list of admin users.
		req.Header.Add(grafanaUserHeader, ceems_api_base.CEEMSServiceAccount)

		// Make request
		clusters, err = ceemsAPIRequest[models.Cluster](req, lb.amw.ceems.client)
		if err != nil {
			return err
		}
	}

validate:
	// Gather all IDs in clusters
	var actualClusterIDs []string

	for _, cluster := range clusters {
		actualClusterIDs = append(actualClusterIDs, cluster.ID)
	}

	// Check if ID is in actualClusterIDs
	for _, id := range lb.amw.clusterIDs {
		if !slices.Contains(actualClusterIDs, id) {
			return fmt.Errorf(
				"%w: %s. Cluster IDs in CEEMS DB are %s",
				ErrUnknownClusterID, id, strings.Join(actualClusterIDs, ","),
			)
		}
	}

	return nil
}

// Start server.
func (lb *loadBalancer) Start(_ context.Context) error {
	// Apply middleware
	lb.server.Handler = lb.amw.Middleware(http.HandlerFunc(lb.Serve))
	lb.logger.Info("Starting "+base.CEEMSLoadBalancerAppName, "listening", lb.server.Addr)

	// Listen for requests
	if err := web.ListenAndServe(lb.server, lb.webConfig, lb.logger); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		lb.logger.Error("Failed to Listen and Serve HTTP server", "err", err)

		return err
	}

	return nil
}

// Shutdown server.
func (lb *loadBalancer) Shutdown(ctx context.Context) error {
	// Close DB connection only if DB file is provided
	if lb.amw.ceems.db != nil {
		if err := lb.amw.ceems.db.Close(); err != nil {
			lb.logger.Error("Failed to close DB connection", "err", err)

			return err
		}
	}

	// Shutdown the server
	if err := lb.server.Shutdown(ctx); err != nil {
		lb.logger.Error("Failed to shutdown HTTP server", "err", err)

		return err
	}

	return nil
}

// Serve serves the request using a backend TSDB server from the pool.
func (lb *loadBalancer) Serve(w http.ResponseWriter, r *http.Request) {
	// Health check
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("CEEMS LB Server is healthy"))

		return
	}

	// Retrieve query params from context
	queryParams := r.Context().Value(ReqParamsContextKey{})

	// Check if queryParams is nil which could happen in edge cases
	if queryParams == nil {
		http.Error(w, "Query parameters not found", http.StatusBadRequest)

		return
	}

	// Middleware ensures that query parameters are always set in request's context
	var queryPeriod time.Duration

	var id string

	if v, ok := queryParams.(*ReqParams); ok {
		queryPeriod = v.queryPeriod
		id = v.clusterID
	} else {
		http.Error(w, "Invalid query parameters", http.StatusBadRequest)

		return
	}

	// Choose target based on query Period
	if target := lb.manager.Target(id, queryPeriod); target != nil {
		target.Serve(w, r)

		return
	}

	http.Error(w, "Service not available", http.StatusServiceUnavailable)
}
