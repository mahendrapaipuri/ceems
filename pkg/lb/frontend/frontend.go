// Package frontend implements the frontend server of the load balancer
package frontend

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
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
	DBPath           string
	AdminUsers       []string
	Manager          serverpool.Manager
	Grafana          *grafana.Grafana
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
	var err error
	if c.DBPath != "" {
		if db, err = sql.Open("sqlite3", c.DBPath); err != nil {
			return nil, err
		}
	}
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
			db:         db,
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
	var body []byte
	var err error

	// Make a new request and add newReader to that request body
	newReq := r.Clone(r.Context())

	// If request has no body go to proxy directly
	if r.Body == nil {
		goto proxy
	}

	// If failed to read body, skip verification and go to request proxy
	body, err = io.ReadAll(r.Body)
	if err != nil {
		level.Error(lb.logger).Log("msg", "Failed to read request body", "err", err)
		goto proxy
	}

	// clone body to existing request and new request
	r.Body = io.NopCloser(bytes.NewReader(body))
	newReq.Body = io.NopCloser(bytes.NewReader(body))

	// Get form values
	if err := newReq.ParseForm(); err != nil {
		level.Error(lb.logger).Log("msg", "Could not parse request body", "err", err)
		goto proxy
	}

	// If not userUnits, forbid request
	// if !lb.userUnits(newReq) {
	// 	http.Error(w, "Bad request", http.StatusBadRequest)
	// 	return
	// }

	// Get query period and target server will depend on this
	if startTime, err := parseTimeParam(newReq, "start", time.Now().UTC()); err != nil {
		level.Error(lb.logger).Log("msg", "Could not parse start query param", "err", err)
		queryPeriod = time.Duration(0 * time.Second)
	} else {
		queryPeriod = time.Now().UTC().Sub(startTime)
	}

proxy:
	// Choose target based on query Period
	target := lb.manager.Target(queryPeriod)
	if target != nil {
		target.Serve(w, r)
		return
	}
	http.Error(w, "Service not available", http.StatusServiceUnavailable)
}
