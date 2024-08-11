// Package frontend implements the frontend server of the load balancer
package frontend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	ceems_api_base "github.com/mahendrapaipuri/ceems/pkg/api/base"
	ceems_api_cli "github.com/mahendrapaipuri/ceems/pkg/api/cli"
	ceems_api_http "github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/common/config"
	"github.com/prometheus/exporter-toolkit/web"
)

// Custom errors.
var (
	ErrUnknownClusterID = errors.New("unknown cluster ID")
)

// RetryContextKey is the key used to set context value for retry.
type RetryContextKey struct{}

// QueryParamsContextKey is the key used to set context value for query params.
type QueryParamsContextKey struct{}

// QueryParams is the context value.
type QueryParams struct {
	id          string
	uuids       []string
	queryPeriod time.Duration
}

// LoadBalancer is the interface to implement.
type LoadBalancer interface {
	Serve(w http.ResponseWriter, r *http.Request)
	Start() error
	Shutdown(ctx context.Context) error
	ValidateClusterIDs(ctx context.Context) error
}

// Config makes a server config from CLI args.
type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	APIServer        ceems_api_cli.CEEMSAPIServerConfig
	Manager          serverpool.Manager
}

// loadBalancer struct.
type loadBalancer struct {
	logger    log.Logger
	manager   serverpool.Manager
	server    *http.Server
	webConfig *web.FlagConfig
	amw       authenticationMiddleware
}

// New returns a new instance of load balancer.
func New(c *Config) (LoadBalancer, error) {
	var db *sql.DB

	var ceemsClient *http.Client

	var ceemsWebURL *url.URL

	var err error

	// Check if DB path exists and get pointer to DB connection
	if c.APIServer.Data.Path != "" {
		dbAbsPath, err := filepath.Abs(
			filepath.Join(c.APIServer.Data.Path, ceems_api_base.CEEMSDBName),
		)
		if err != nil {
			return nil, err
		}

		// Set DB pointer only if file exists. Else sql.Open will create an empty
		// file as if exists already
		if _, err := os.Stat(dbAbsPath); err == nil {
			if db, err = sql.Open("sqlite3", dbAbsPath); err != nil {
				return nil, err
			}
		}
	}

	// Check if URL for CEEMS API exists
	if c.APIServer.Web.URL == "" {
		goto outside
	}

	// Unwrap original error to avoid leaking sensitive passwords in output
	ceemsWebURL, err = url.Parse(c.APIServer.Web.URL)
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	// Make a CEEMS API server client from client config
	if ceemsClient, err = config.NewClientFromConfig(c.APIServer.Web.HTTPClientConfig, "ceems_api_server"); err != nil {
		return nil, err
	}

outside:
	return &loadBalancer{
		logger: c.Logger,
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
		amw: authenticationMiddleware{
			logger: c.Logger,
			ceems: ceems{
				db:     db,
				webURL: ceemsWebURL,
				client: ceemsClient,
			},
		},
	}, nil
}

// ValidateClusterIDs validates the cluster IDs by checking them against DB.
func (lb *loadBalancer) ValidateClusterIDs(ctx context.Context) error {
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
			level.Error(lb.logger).Log("msg", "Errors during scanning rows", "err", err)
		}

		goto validate
	}

	if lb.amw.ceems.clustersEndpoint() != nil {
		// Create a new GET request
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, lb.amw.ceems.clustersEndpoint().String(), nil)
		if err != nil {
			return err
		}

		// Add necessary headers. Value of header is not important only its presence
		req.Header.Add(ceemsUserHeader, "admin")

		// Make request
		// If request failed, forbid the query. It can happen when CEEMS API server
		// goes offline and we should wait for it to come back online
		if resp, err := lb.amw.ceems.client.Do(req); err != nil {
			return err
		} else {
			defer resp.Body.Close()

			// Any status code other than 200 should be treated as check failure
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("error response code %d from CEEMS API server", resp.StatusCode)
			}

			// Read response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			// Unpack into data
			var data ceems_api_http.Response[models.Cluster]
			if err = json.Unmarshal(body, &data); err != nil {
				return err
			}

			// Check if Status is error
			if data.Status == "error" {
				return fmt.Errorf("error response from CEEMS API server: %v", data)
			}

			// Check if Data exists on response
			if data.Data == nil {
				return fmt.Errorf("CEEMS API server response returned no data: %v", data)
			}

			clusters = data.Data
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
func (lb *loadBalancer) Start() error {
	// Apply middleware
	lb.server.Handler = lb.amw.Middleware(http.HandlerFunc(lb.Serve))
	level.Info(lb.logger).Log("msg", "Starting "+base.CEEMSLoadBalancerAppName)

	// Listen for requests
	if err := web.ListenAndServe(lb.server, lb.webConfig, lb.logger); err != nil && errors.Is(err, http.ErrServerClosed) {
		level.Error(lb.logger).Log("msg", "Failed to Listen and Serve HTTP server", "err", err)

		return err
	}

	return nil
}

// Shutdown server.
func (lb *loadBalancer) Shutdown(ctx context.Context) error {
	// Close DB connection only if DB file is provided
	if lb.amw.ceems.db != nil {
		if err := lb.amw.ceems.db.Close(); err != nil {
			level.Error(lb.logger).Log("msg", "Failed to close DB connection", "err", err)

			return err
		}
	}

	// Shutdown the server
	if err := lb.server.Shutdown(ctx); err != nil {
		level.Error(lb.logger).Log("msg", "Failed to shutdown HTTP server", "err", err)

		return err
	}

	return nil
}

// Serve serves the request using a backend TSDB server from the pool.
func (lb *loadBalancer) Serve(w http.ResponseWriter, r *http.Request) {
	// Retrieve query params from context
	queryParams := r.Context().Value(QueryParamsContextKey{})

	// Check if queryParams is nil which could happen in edge cases
	if queryParams == nil {
		http.Error(w, "Query parameters not found", http.StatusBadRequest)

		return
	}

	// Middleware ensures that query parameters are always set in request's context
	var queryPeriod time.Duration

	var id string

	if v, ok := queryParams.(*QueryParams); ok {
		queryPeriod = v.queryPeriod
		id = v.id
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
