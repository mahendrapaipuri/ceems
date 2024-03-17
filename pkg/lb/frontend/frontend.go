// Package frontend implements the frontend server of the load balancer
package frontend

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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
	Manager          serverpool.Manager
}

// loadBalancer struct
type loadBalancer struct {
	logger    log.Logger
	manager   serverpool.Manager
	server    *http.Server
	webConfig *web.FlagConfig
	db        *sql.DB
}

// Header containing the username
const (
	userHeaderName = "X-Grafana-User"
)

var (
	// Regex that will match job's UUIDs
	// Dont use greedy matching to avoid capturing gpuuuid label
	// Use strict UUID allowable character set. They can be only letters, digits and hypen (-)
	// Playground: https://goplay.tools/snippet/kq_r_1SOgnG
	regexpUUID = regexp.MustCompile("(?:.+?)[^gpu]uuid=[~]{0,1}\"(?P<uuid>[a-zA-Z0-9-|]+)\"(?:.*)")
)

// NewLoadBalancer returns a new instance of load balancer
func NewLoadBalancer(c *Config) (LoadBalancer, error) {
	// Open DB connection
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
	}, nil
}

// Start server
func (lb *loadBalancer) Start() error {
	level.Info(lb.logger).Log("msg", fmt.Sprintf("Starting %s", base.CEEMSLoadBalancerAppName))
	lb.server.Handler = http.HandlerFunc(lb.Serve)
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

// Check UUIDs in query belong to user or not. This check is less invasive.
// Any error will mark the check as pass and request will be proxied to backend
func (lb *loadBalancer) userUnits(r *http.Request) bool {
	// If there is no active DB conn, return
	if lb.db == nil {
		return true
	}

	// Get user name
	user := r.Header.Get(userHeaderName)

	// If there is no user header return true
	if user == "" {
		return true
	}

	// Check if user is quering their own job by looking to DB
	var uuids []string
	if val := r.FormValue("query"); val != "" {
		matches := regexpUUID.FindAllStringSubmatch(val, -1)
		for _, match := range matches {
			if len(match) > 1 {
				for _, uuid := range strings.Split(match[1], "|") {
					// Ignore empty strings
					if strings.TrimSpace(uuid) != "" && !slices.Contains(uuids, uuid) {
						uuids = append(uuids, uuid)
					}
				}
			}
		}
	} else {
		return true
	}

	// If uuids is empty return
	if len(uuids) == 0 {
		return true
	}
	level.Debug(lb.logger).
		Log("msg", "UUIDs in query", "user", user, "queried_uuids", strings.Join(uuids, ","))

	// First get a list of projects that user is part of
	rows, err := lb.db.Query("SELECT DISTINCT project FROM usage WHERE usr = ?", user)
	if err != nil {
		level.Warn(lb.logger).
			Log("msg", "Failed to get user projects. Allowing query", "user", user, 
			"queried_uuids", strings.Join(uuids, ","), "err", err,
		)
		return true
	}

	// Scan project rows
	var projects []string
	var project string
	for rows.Next() {
		if err := rows.Scan(&project); err != nil {
			continue
		}
		projects = append(projects, project)
	}

	// If no projects found, return. This should not be the case and if it happens
	// something is wrong. Spit a warning log
	if len(projects) == 0 {
		level.Warn(lb.logger).
			Log("msg", "No user projects found. Allowing query", "user", user,
				"queried_uuids", strings.Join(uuids, ","), "err", err,
			)
		return true
	}

	// Make a query and query args. Query args must be converted to slice of interfaces
	// and it is sql driver's responsibility to cast them to proper types
	query := fmt.Sprintf(
		"SELECT uuid FROM units WHERE project IN (%s) AND uuid IN (%s)",
		strings.Join(strings.Split(strings.Repeat("?", len(projects)), ""), ","),
		strings.Join(strings.Split(strings.Repeat("?", len(uuids)), ""), ","),
	)
	queryData := islice(append(projects, uuids...))

	// Make query. If query fails for any reason, we allow request to avoid false negatives
	rows, err = lb.db.Query(query, queryData...)
	if err != nil {
		level.Warn(lb.logger).
			Log("msg", "Failed to query the uuid check. Allowing query", "user", user,
				"user_projects", strings.Join(projects, ","), "queried_uuids", strings.Join(uuids, ","),
				"err", err,
			)
		return true
	}
	defer rows.Close()

	// Get number of rows returned by query
	uuidCount := 0
	for rows.Next() {
		uuidCount++
	}

	// If returned number of UUIDs is not same as queried UUIDs, user is attempting
	// to query for jobs of other user
	if uuidCount != len(uuids) {
		level.Debug(lb.logger).
			Log("msg", "Forbiding query", "user", user, "user_projects", strings.Join(projects, ","), 
			"queried_uuids", len(uuids), "found_uuids", uuidCount,
			)
		return false
	}
	return true
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
	if !lb.userUnits(newReq) {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

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
