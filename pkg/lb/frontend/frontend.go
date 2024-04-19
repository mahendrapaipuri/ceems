// Package frontend implements the frontend server of the load balancer
package frontend

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
)

// RetryContextKey is the key used to set context value for retry
type RetryContextKey struct{}

// QueryParamsContextKey is the key used to set context value for query params
type QueryParamsContextKey struct{}

// QueryParams is the context value
type QueryParams struct {
	uuids       []string
	queryPeriod time.Duration
}

// LoadBalancer is the interface to implement
type LoadBalancer interface {
	Serve(http.ResponseWriter, *http.Request)
	Start() error
	Shutdown(context.Context) error
}

// Config makes a server config from CLI args
type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	CEEMSAPI         base.CEEMSAPI
	AdminUsers       []string
	Manager          serverpool.Manager
	Grafana          *grafana.Grafana
	GrafanaTeamID    string
}

// loadBalancer struct
type loadBalancer struct {
	logger    log.Logger
	manager   serverpool.Manager
	server    *http.Server
	webConfig *web.FlagConfig
	amw       authenticationMiddleware
	db        *sql.DB
}

// NewLoadBalancer returns a new instance of load balancer
func NewLoadBalancer(c *Config) (LoadBalancer, error) {
	var db *sql.DB
	var ceemsClient *http.Client
	var ceemsURL *url.URL
	var err error

	// Check if DB path exists and get pointer to DB connection
	if c.CEEMSAPI.DBPath != "" {
		if db, err = sql.Open("sqlite3", c.CEEMSAPI.DBPath); err != nil {
			return nil, err
		}
	}

	// Check if URL for CEEMS API exists
	if c.CEEMSAPI.URL == "" {
		goto outside
	}

	// Unwrap original error to avoid leaking sensitive passwords in output
	ceemsURL, err = url.Parse(c.CEEMSAPI.URL)
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	// If skip verify is set to true for TSDB add it to client
	if c.CEEMSAPI.SkipTLSVerify {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		ceemsClient = &http.Client{Transport: tr, Timeout: time.Duration(30 * time.Second)}
	} else {
		ceemsClient = &http.Client{Timeout: time.Duration(30 * time.Second)}
	}

outside:
	return &loadBalancer{
		logger: c.Logger,
		server: &http.Server{
			Addr: c.Address,
		},
		webConfig: &web.FlagConfig{
			WebListenAddresses: &[]string{c.Address},
			WebSystemdSocket:   &c.WebSystemdSocket,
			WebConfigFile:      &c.WebConfigFile,
		},
		manager: c.Manager,
		db:      db,
		amw: authenticationMiddleware{
			logger:     c.Logger,
			adminUsers: c.AdminUsers,
			grafana:    c.Grafana,
			ceems: ceems{
				db:     db,
				url:    ceemsURL,
				client: ceemsClient,
			},
			grafanaTeamID: c.GrafanaTeamID,
		},
	}, nil
}

// Start server
func (lb *loadBalancer) Start() error {
	lb.server.Handler = lb.amw.Middleware(http.HandlerFunc(lb.Serve))
	level.Info(lb.logger).Log("msg", fmt.Sprintf("Starting %s", base.CEEMSLoadBalancerAppName))
	if err := web.ListenAndServe(lb.server, lb.webConfig, lb.logger); err != nil && err != http.ErrServerClosed {
		level.Error(lb.logger).Log("msg", "Failed to Listen and Serve HTTP server", "err", err)
		return err
	}
	return nil
}

// Shutdown server
func (lb *loadBalancer) Shutdown(ctx context.Context) error {
	// Close DB connection only if DB file is provided
	if lb.db != nil {
		if err := lb.db.Close(); err != nil {
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

// Serve serves the request using a backend TSDB server from the pool
func (lb *loadBalancer) Serve(w http.ResponseWriter, r *http.Request) {
	var queryPeriod time.Duration

	// Retrieve query params from context
	queryParams := r.Context().Value(QueryParamsContextKey{})

	// Check if queryParams is nil which could happen in edge cases
	if queryParams == nil {
		queryPeriod = time.Duration(0 * time.Second)
	} else {
		queryPeriod = queryParams.(*QueryParams).queryPeriod
	}

	// Choose target based on query Period
	target := lb.manager.Target(queryPeriod)
	if target != nil {
		target.Serve(w, r)
		return
	}
	http.Error(w, "Service not available", http.StatusServiceUnavailable)
}
