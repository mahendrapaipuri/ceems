// Package http implements the HTTP server handlers for different resource endpoints
package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/db"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
)

// API Resources names
const (
	unitsResourceName    = "units"
	usageResourceName    = "usage"
	projectsResourceName = "projects"
)

// Config makes a server config from CLI args
type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	DBConfig         db.Config
	MaxQueryPeriod   time.Duration
	AdminUsers       []string
	Grafana          *grafana.Grafana
}

// CEEMSServer struct implements HTTP server for stats
type CEEMSServer struct {
	logger         log.Logger
	server         *http.Server
	webConfig      *web.FlagConfig
	db             *sql.DB
	dbConfig       db.Config
	maxQueryPeriod time.Duration
	Querier        func(*sql.DB, Query, string, log.Logger) (interface{}, error)
	HealthCheck    func(*sql.DB, log.Logger) bool
}

// Response defines the response model of CEEMSServer
type Response struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

var (
	aggUsageDBCols     = make([]string, len(base.UsageDBTableColNames))
	defaultQueryWindow = time.Duration(2 * time.Hour) // Two hours
)

// Make summary DB col names by using aggregate SQL functions
func init() {
	// Add primary field manually as it is ignored in UsageDBColNames
	aggUsageDBCols[0] = "id"

	// Use SQL aggregate functions in query
	j := 0
	for i := 0; i < len(base.UsageDBTableColNames); i++ {
		col := base.UsageDBTableColNames[i]
		// Ignore last_updated_at col
		if slices.Contains([]string{"last_updated_at"}, col) {
			continue
		}

		if strings.HasPrefix(col, "avg") {
			aggUsageDBCols[j+1] = fmt.Sprintf("AVG(%[1]s) AS %[1]s", col)
		} else if strings.HasPrefix(col, "total") {
			aggUsageDBCols[j+1] = fmt.Sprintf("SUM(%[1]s) AS %[1]s", col)
		} else if strings.HasPrefix(col, "num") {
			aggUsageDBCols[j+1] = "COUNT(id) AS num_units"
		} else {
			aggUsageDBCols[j+1] = col
		}
		j++
	}
}

// Ping DB for connection test
func getDBStatus(dbConn *sql.DB, logger log.Logger) bool {
	if err := dbConn.Ping(); err != nil {
		level.Error(logger).Log("msg", "DB Ping failed", "err", err)
		return false
	}
	return true
}

// NewCEEMSServer creates new CEEMSServer struct instance
func NewCEEMSServer(c *Config) (*CEEMSServer, func(), error) {
	router := mux.NewRouter()
	server := &CEEMSServer{
		logger: c.Logger,
		server: &http.Server{
			Addr:         c.Address,
			Handler:      router,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		webConfig: &web.FlagConfig{
			WebListenAddresses: &[]string{c.Address},
			WebSystemdSocket:   &c.WebSystemdSocket,
			WebConfigFile:      &c.WebConfigFile,
		},
		dbConfig:       c.DBConfig,
		maxQueryPeriod: c.MaxQueryPeriod,
		Querier:        querier,
		HealthCheck:    getDBStatus,
	}

	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html>
			<head><title>Usage Stats API Server</title></head>
			<body>
			<h1>Compute Stats</h1>
			<p><a href="./api/units">Compute Units</a></p>
			<p><a href="./api/projects">Projects</a></p>
			<p><a href="./api/usage/current">Current Usage</a></p>
			<p><a href="./api/usage/global">Total Usage</a></p>
			</body>
			</html>`))
	})

	// Allow only GET methods
	router.HandleFunc("/api/health", server.health).Methods("GET")
	router.HandleFunc("/api/projects", server.projects).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s", unitsResourceName), server.units).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/admin", unitsResourceName), server.unitsAdmin).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/{query:(?:current|global)}", usageResourceName), server.usage).
		Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/{query:(?:current|global)}/admin", usageResourceName), server.usageAdmin).
		Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/verify", unitsResourceName), server.verifyUnitsOwnership).Methods("GET")

	// pprof debug end points
	router.PathPrefix("/debug/").Handler(http.DefaultServeMux)

	// Add a middleware that verifies headers and pass them in requests
	// The middleware will fetch admin users from Grafana periodically to update list
	amw := authenticationMiddleware{
		logger:     c.Logger,
		adminUsers: c.AdminUsers,
		grafana:    c.Grafana,
	}
	router.Use(amw.Middleware)

	// Open DB connection
	var err error
	if server.db, err = sql.Open("sqlite3", filepath.Join(c.DBConfig.DataPath, fmt.Sprintf("%s.db", base.CEEMSServerAppName))); err != nil {
		return nil, func() {}, err
	}
	return server, func() {}, nil
}

// Start server
func (s *CEEMSServer) Start() error {
	level.Info(s.logger).Log("msg", fmt.Sprintf("Starting %s", base.CEEMSServerAppName))
	if err := web.ListenAndServe(s.server, s.webConfig, s.logger); err != nil && err != http.ErrServerClosed {
		level.Error(s.logger).Log("msg", "Failed to Listen and Server HTTP server", "err", err)
		return err
	}
	return nil
}

// Shutdown server
func (s *CEEMSServer) Shutdown(ctx context.Context) error {
	// Close DB connection
	if err := s.db.Close(); err != nil {
		level.Error(s.logger).Log("msg", "Failed to close DB connection", "err", err)
		return err
	}

	// Shutdown the server
	if err := s.server.Shutdown(ctx); err != nil {
		level.Error(s.logger).Log("msg", "Failed to shutdown HTTP server", "err", err)
		return err
	}
	return nil
}

// Get current user from the header
func (s *CEEMSServer) getUser(r *http.Request) (string, string) {
	return r.Header.Get(loggedUserHeader), r.Header.Get(dashboardUserHeader)
}

// Set response headers
func (s *CEEMSServer) setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// Check status of server
func (s *CEEMSServer) health(w http.ResponseWriter, r *http.Request) {
	if !s.HealthCheck(s.db, s.logger) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("KO"))
	} else {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// Fetch project, partition and QoS query params and add them to query
func (s *CEEMSServer) getCommonQueryParams(q *Query, URLValues url.Values) Query {
	// Get project query parameters if any
	if projects := URLValues["project"]; len(projects) > 0 {
		q.query(" AND project IN ")
		q.param(projects)
	}

	// Check if running query param is included
	// Running units will have ended_at_ts as 0 and we use this in query to
	// fetch these units
	if _, ok := URLValues["running"]; ok {
		q.query(" OR ended_at_ts IN ")
		q.param([]string{"0"})
	}
	return *q
}

// Get from and to time stamps from query vars and cast them into proper format
func (s *CEEMSServer) getQueryWindow(r *http.Request) (map[string]string, error) {
	var fromTime, toTime time.Time
	// Get to and from query parameters and do checks on them
	if f := r.URL.Query().Get("from"); f == "" {
		// If from is not present in query params, use a default query window of 1 week
		fromTime = time.Now().Add(-defaultQueryWindow)
	} else {
		// Return error response if from is not a timestamp
		if ts, err := strconv.Atoi(f); err != nil {
			level.Error(s.logger).Log("msg", "Failed to parse from timestamp", "from", f, "err", err)
			return nil, fmt.Errorf("malformed 'from' timestamp")
		} else {
			fromTime = time.Unix(int64(ts), 0)
		}
	}
	if t := r.URL.Query().Get("to"); t == "" {
		// Use current time as default to
		toTime = time.Now()
	} else {
		// Return error response if to is not a timestamp
		if ts, err := strconv.Atoi(t); err != nil {
			level.Error(s.logger).Log("msg", "Failed to parse to timestamp", "to", t, "err", err)
			return nil, fmt.Errorf("malformed 'to' timestamp")
		} else {
			toTime = time.Unix(int64(ts), 0)
		}
	}

	// If difference between from and to is more than max query period, return with empty
	// response. This is to prevent users from making "big" requests that can "potentially"
	// choke server and end up in OOM errors
	if s.maxQueryPeriod > time.Duration(0*time.Second) && toTime.Sub(fromTime) > s.maxQueryPeriod {
		level.Error(s.logger).Log(
			"msg", "Exceeded maximum query time window",
			"maxQueryWindow", s.maxQueryPeriod,
			"from", fromTime.Format(time.DateTime), "to", toTime.Format(time.DateTime),
			"queryWindow", toTime.Sub(fromTime).String(),
		)
		return nil, fmt.Errorf("maximum query window exceeded")
	}
	return map[string]string{
		"from": fromTime.Format(base.DatetimeLayout),
		"to":   toTime.Format(base.DatetimeLayout),
	}, nil
}

// Get units of users
func (s *CEEMSServer) unitsQuerier(
	queriedUsers []string,
	w http.ResponseWriter,
	r *http.Request,
) {
	var queryWindowTS map[string]string
	var err error

	// Get current logged user and dashboard user from headers
	loggedUser, _ := s.getUser(r)

	// Set headers
	s.setHeaders(w)

	// Initialise utility vars
	checkQueryWindow := true // Check query window size

	// Get fields query parameters if any
	var reqFields string
	if fields := r.URL.Query()["field"]; len(fields) > 0 {
		// Check if fields are valid field names
		var validFields []string
		for _, f := range fields {
			if slices.Contains(base.UnitsDBTableColNames, f) {
				validFields = append(validFields, f)
			}
		}
		reqFields = strings.Join(validFields, ",")
	} else {
		reqFields = strings.Join(base.UnitsDBTableColNames, ",")
	}

	// Initialise query builder
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", reqFields, base.UnitsDBTableName))

	// Query for only unignored units
	q.query(" WHERE ignore = 0 ")

	// Add condition to query only for current dashboardUser
	if len(queriedUsers) > 0 {
		q.query(" AND usr IN ")
		q.param(queriedUsers)
	}

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Check if uuid present in query params and add them
	// If any of uuid query params are present
	// do not check query window as we are fetching a specific unit(s)
	if uuids := r.URL.Query()["uuid"]; len(uuids) > 0 {
		q.query(" AND uuid IN ")
		q.param(uuids)
		checkQueryWindow = false
	}

	// If we dont have to specific query window skip next section of code as it becomes
	// irrelevant
	if !checkQueryWindow {
		goto queryUnits
	}

	// Get query window time stamps
	queryWindowTS, err = s.getQueryWindow(r)
	if err != nil {
		errorResponse(w, &apiError{errorBadData, err}, s.logger, nil)
		return
	}

	// Add from and to to query only when checkQueryWindow is true
	q.query(" AND ended_at BETWEEN ")
	q.param([]string{queryWindowTS["from"]})
	q.query(" AND ")
	q.param([]string{queryWindowTS["to"]})

queryUnits:

	// Get all user units in the given time window
	units, err := s.Querier(s.db, q, unitsResourceName, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch units", "loggedUser", loggedUser, "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	response := Response{
		Status: "success",
		Data:   units.([]models.Unit),
	}
	if err = json.NewEncoder(w).Encode(&response); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/units/admin
// Get any unit of any user
func (s *CEEMSServer) unitsAdmin(w http.ResponseWriter, r *http.Request) {
	// Query for units and write response
	s.unitsQuerier(r.URL.Query()["user"], w, r)
}

// GET /api/units
// Get unit of dashboard user
func (s *CEEMSServer) units(w http.ResponseWriter, r *http.Request) {
	// Get current logged user and dashboard user from headers
	_, dashboardUser := s.getUser(r)

	// Query for units and write response
	s.unitsQuerier([]string{dashboardUser}, w, r)
}

// GET /api/units/verify
// Verify the user ownership for queried units
func (s *CEEMSServer) verifyUnitsOwnership(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get current logged user and dashboard user from headers
	_, dashboardUser := s.getUser(r)

	// Get list of queried uuids
	uuids := r.URL.Query()["uuid"]

	// Check if user is owner of the queries uuids
	if VerifyOwnership(dashboardUser, uuids, s.db, s.logger) {
		w.WriteHeader(http.StatusOK)
		response := Response{
			Status: "success",
			Data: map[string]interface{}{
				"user":   dashboardUser,
				"uuids":  uuids,
				"verfiy": true,
			},
		}
		if err := json.NewEncoder(w).Encode(&response); err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
	} else {
		errorResponse(w, &apiError{errorUnauthorized, fmt.Errorf("user do not have permissions on uuids")}, s.logger, nil)
	}
}

// GET /api/projects
// Get projects list of user
func (s *CEEMSServer) projects(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Make wuery
	q := Query{}
	q.query(fmt.Sprintf("SELECT DISTINCT project FROM %s", base.UnitsDBTableName))
	q.query(" WHERE usr IN ")
	q.param([]string{dashboardUser})

	// Make query and check for projects returned in usage
	projects, err := s.Querier(s.db, q, "projects", s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch projects", "user", dashboardUser, "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	projectsResponse := Response{
		Status: "success",
		Data:   projects.([]models.Project),
	}
	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// Make sub query for fetching projects of users
func (s *CEEMSServer) projectsSubQuery(users []string) Query {
	// Make a sub query that will fetch projects of users
	qSub := Query{}
	qSub.query(fmt.Sprintf("SELECT DISTINCT project FROM %s", base.UsageDBTableName))

	// Add conditions to sub query
	if len(users) > 0 {
		qSub.query(" WHERE usr IN ")
		qSub.param(users)
	}
	return qSub
}

// GET /api/usage/current
// Get current usage statistics
func (s *CEEMSServer) currentUsage(users []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for projects
	qSub := s.projectsSubQuery(users)

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(aggUsageDBCols, ","), base.UnitsDBTableName))

	// First select all projects that user is part of using subquery
	q.query(" WHERE project IN ")
	q.subQuery(qSub)

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Get query window time stamps
	queryWindowTS, err := s.getQueryWindow(r)
	if err != nil {
		errorResponse(w, &apiError{errorBadData, err}, s.logger, nil)
		return
	}

	// Add from and to to query only when checkQueryWindow is true
	q.query(" AND ended_at BETWEEN ")
	q.param([]string{queryWindowTS["from"]})
	q.query(" AND ")
	q.param([]string{queryWindowTS["to"]})

	// Finally add GROUP BY clause
	if groupby := r.URL.Query()["groupby"]; len(groupby) > 0 {
		q.query(fmt.Sprintf(" GROUP BY %s", strings.Join(groupby, ",")))
	}

	// Make query and check for returned number of rows
	usage, err := s.Querier(s.db, q, usageResourceName, s.logger)
	if err != nil {
		level.Error(s.logger).
			Log("msg", "Failed to fetch current usage statistics", "users", strings.Join(users, ","), "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	projectsResponse := Response{
		Status: "success",
		Data:   usage.([]models.Usage),
	}
	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/usage/global
// Get global usage statistics
func (s *CEEMSServer) globalUsage(users []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for projects
	qSub := s.projectsSubQuery(users)

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(base.UsageDBTableColNames, ","), base.UsageDBTableName))

	// First select all projects that user is part of using subquery
	q.query(" WHERE project IN ")
	q.subQuery(qSub)

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Make query and check for returned number of rows
	usage, err := s.Querier(s.db, q, usageResourceName, s.logger)
	if err != nil {
		level.Error(s.logger).
			Log("msg", "Failed to fetch global usage statistics", "users", strings.Join(users, ","), "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	projectsResponse := Response{
		Status: "success",
		Data:   usage.([]models.Usage),
	}
	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/usage
// Get current/global usage statistics
func (s *CEEMSServer) usage(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Get path parameter type
	var queryType string
	var exists bool
	if queryType, exists = mux.Vars(r)["query"]; !exists {
		errorResponse(w, &apiError{errorBadData, errInvalidRequest}, s.logger, nil)
		return
	}

	// handle current usage query
	if queryType == "current" {
		s.currentUsage([]string{dashboardUser}, w, r)
	}

	// handle global usage query
	if queryType == "global" {
		s.globalUsage([]string{dashboardUser}, w, r)
	}
}

// GET /api/usage/admin
// Get current/global usage statistics of any user
func (s *CEEMSServer) usageAdmin(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get path parameter type
	var queryType string
	var exists bool
	if queryType, exists = mux.Vars(r)["query"]; !exists {
		errorResponse(w, &apiError{errorBadData, errInvalidRequest}, s.logger, nil)
		return
	}

	// handle current usage query
	if queryType == "current" {
		s.currentUsage(r.URL.Query()["user"], w, r)
	}

	// handle global usage query
	if queryType == "global" {
		s.globalUsage(r.URL.Query()["user"], w, r)
	}
}
