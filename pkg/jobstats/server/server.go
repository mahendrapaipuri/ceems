package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/db"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/grafana"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
)

// Server config struct
type Config struct {
	Logger             log.Logger
	Address            string
	WebSystemdSocket   bool
	WebConfigFile      string
	DBConfig           db.Config
	MaxQueryPeriod     time.Duration
	AdminUsers         []string
	Grafana            *grafana.Grafana
	GrafanaAdminTeamID string
}

// Job Stats Server struct
type JobstatsServer struct {
	logger         log.Logger
	server         *http.Server
	webConfig      *web.FlagConfig
	db             *sql.DB
	dbConfig       db.Config
	maxQueryPeriod time.Duration
	Querier        func(*sql.DB, Query, string, log.Logger) (interface{}, error)
	HealthCheck    func(*sql.DB, log.Logger) bool
}

// Common API response model
type Response struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

var (
	aggJobDBCols       = make([]string, len(base.UsageDBColNames)+1)
	defaultQueryWindow = time.Duration(2 * time.Hour) // Two hours
)

// Make accountSummary DB col names by using aggregate SQL functions
func init() {
	// Add primary field manually as it is ignored in UsageDBColNames
	aggJobDBCols[0] = "id"

	// Use SQL aggregate functions in query
	for i := 0; i < len(base.UsageDBColNames); i++ {
		col := base.UsageDBColNames[i]
		if strings.HasPrefix(col, "avg") {
			aggJobDBCols[i+1] = fmt.Sprintf("AVG(%[1]s) AS %[1]s", col)
		} else if strings.HasPrefix(col, "total") {
			aggJobDBCols[i+1] = fmt.Sprintf("SUM(%[1]s) AS %[1]s", col)
		} else if strings.HasPrefix(col, "num") {
			aggJobDBCols[i+1] = "COUNT(id) AS num_jobs"
		} else {
			aggJobDBCols[i+1] = col
		}
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

// Create new Jobstats server
func NewJobstatsServer(c *Config) (*JobstatsServer, func(), error) {
	router := mux.NewRouter()
	server := &JobstatsServer{
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
			<head><title>Batch Job Stats API Server</title></head>
			<body>
			<h1>Job Stats</h1>
			<p><a href="./api/jobs">Jobs</a></p>
			<p><a href="./api/accounts">Accounts</a></p>
			<p><a href="./api/usage/current">Current Usage</a></p>
			<p><a href="./api/usage/global">Total Usage</a></p>
			</body>
			</html>`))
	})

	// Allow only GET methods
	router.HandleFunc("/api/health", server.health).Methods("GET")
	router.HandleFunc("/api/accounts", server.accounts).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s", base.JobsEndpoint), server.jobs).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/admin", base.JobsEndpoint), server.jobsAdmin).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/{query:(?:current|global)}", base.UsageEndpoint), server.usage).
		Methods("GET")
	router.HandleFunc(fmt.Sprintf("/api/%s/{query:(?:current|global)}/admin", base.UsageEndpoint), server.usageAdmin).
		Methods("GET")

	// pprof debug end points
	router.PathPrefix("/debug/").Handler(http.DefaultServeMux)

	// Add a middleware that verifies headers and pass them in requests
	// The middleware will fetch admin users from Grafana periodically to update list
	amw := authenticationMiddleware{
		logger:             c.Logger,
		adminUsers:         c.AdminUsers,
		grafana:            c.Grafana,
		grafanaAdminTeamID: c.GrafanaAdminTeamID,
	}
	router.Use(amw.Middleware)

	// Open DB connection
	var err error
	if server.db, err = sql.Open("sqlite3", c.DBConfig.JobstatsDBPath); err != nil {
		return nil, func() {}, err
	}
	return server, func() {}, nil
}

// Start server
func (s *JobstatsServer) Start() error {
	level.Info(s.logger).Log("msg", "Starting batchjob_stats_server")
	if err := web.ListenAndServe(s.server, s.webConfig, s.logger); err != nil && err != http.ErrServerClosed {
		level.Error(s.logger).Log("msg", "Failed to Listen and Server HTTP server", "err", err)
		return err
	}
	return nil
}

// Shutdown server
func (s *JobstatsServer) Shutdown(ctx context.Context) error {
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
func (s *JobstatsServer) getUser(r *http.Request) (string, string) {
	return r.Header.Get(loggedUserHeader), r.Header.Get(dashboardUserHeader)
}

// Set response headers
func (s *JobstatsServer) setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// Check status of server
func (s *JobstatsServer) health(w http.ResponseWriter, r *http.Request) {
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

// Fetch account, partition and QoS query params and add them to query
func (s *JobstatsServer) getCommonQueryParams(q *Query, URLValues url.Values) Query {
	// Get account query parameters if any
	if accounts := URLValues["account"]; len(accounts) > 0 {
		q.query(" AND account IN ")
		q.param(accounts)
	}

	// Get partition query parameters if any
	if partitions := URLValues["partition"]; len(partitions) > 0 {
		q.query(" AND partition IN ")
		q.param(partitions)
	}

	// Get qos query parameters if any
	if qoss := URLValues["qos"]; len(qoss) > 0 {
		q.query(" AND qos IN ")
		q.param(qoss)
	}
	return *q
}

// Get from and to time stamps from query vars and cast them into proper format
func (s *JobstatsServer) getQueryWindow(r *http.Request) (map[string]string, error) {
	var fromTime, toTime time.Time
	// Get to and from query parameters and do checks on them
	if f := r.URL.Query().Get("from"); f == "" {
		// If from is not present in query params, use a default query window of 1 week
		fromTime = time.Now().Add(-defaultQueryWindow)
	} else {
		// Return error response if from is not a timestamp
		if ts, err := strconv.Atoi(f); err != nil {
			level.Error(s.logger).Log("msg", "Failed to parse from timestamp", "from", f, "err", err)
			return nil, fmt.Errorf("Malformed 'from' timestamp")
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
			return nil, fmt.Errorf("Malformed 'to' timestamp")
		} else {
			toTime = time.Unix(int64(ts), 0)
		}
	}

	// If difference between from and to is more than max query period, return with empty
	// response. This is to prevent users from making "big" requests that can "potentially"
	// choke server and end up in OOM errors
	if toTime.Sub(fromTime) > s.maxQueryPeriod {
		level.Error(s.logger).Log(
			"msg", "Exceeded maximum query time window",
			"maxQueryWindow", s.maxQueryPeriod,
			"from", fromTime.Format(time.DateTime), "to", toTime.Format(time.DateTime),
			"queryWindow", toTime.Sub(fromTime).String(),
		)
		return nil, fmt.Errorf("Maximum query window exceeded")
	}
	return map[string]string{
		"from": fromTime.Format(base.DatetimeLayout),
		"to":   toTime.Format(base.DatetimeLayout),
	}, nil
}

// Get jobs of users
func (s *JobstatsServer) jobsQuerier(queriedUsers []string, loggedUser string, w http.ResponseWriter, r *http.Request) {
	var queryWindowTS map[string]string
	var err error

	// Set headers
	s.setHeaders(w)

	// Initialise utility vars
	checkQueryWindow := true // Check query window size

	// Initialise query builder
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(base.JobsDBColNames, ","), base.JobsDBTableName))

	// Query for only unignored jobs
	q.query(" WHERE ignore = 0 ")

	// Add condition to query only for current dashboardUser
	if len(queriedUsers) > 0 {
		q.query(" AND usr IN ")
		q.param(queriedUsers)
	}

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Check if jobuuid present in query params and add them
	// If any of jobuuid or jobid query params are present
	// do not check query window as we are fetching a specific job(s)
	if jobuuids := r.URL.Query()["jobuuid"]; len(jobuuids) > 0 {
		q.query(" AND jobuuid IN ")
		q.param(jobuuids)
		checkQueryWindow = false
	}

	// Similarly check for jobid
	if jobids := r.URL.Query()["jobid"]; len(jobids) > 0 {
		q.query(" AND jobid IN ")
		q.param(jobids)
		checkQueryWindow = false
	}

	// If we dont have to specific query window skip next section of code as it becomes
	// irrelevant
	if !checkQueryWindow {
		goto queryJobs
	}

	// Get query window time stamps
	queryWindowTS, err = s.getQueryWindow(r)
	if err != nil {
		errorResponse(w, &apiError{errorBadData, err}, s.logger, nil)
		return
	}

	// Add from and to to query only when checkQueryWindow is true
	q.query(" AND end BETWEEN ")
	q.param([]string{queryWindowTS["from"]})
	q.query(" AND ")
	q.param([]string{queryWindowTS["to"]})

queryJobs:

	// Get all user jobs in the given time window
	jobs, err := s.Querier(s.db, q, base.JobsEndpoint, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch jobs", "loggedUser", loggedUser, "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	response := Response{
		Status: "success",
		Data:   jobs,
	}
	if err = json.NewEncoder(w).Encode(&response); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/jobs/admin
// Get any job of any user
func (s *JobstatsServer) jobsAdmin(w http.ResponseWriter, r *http.Request) {
	// Get current logged user and dashboard user from headers
	loggedUser, _ := s.getUser(r)

	// Query for jobs and write response
	s.jobsQuerier(r.URL.Query()["user"], loggedUser, w, r)
}

// GET /api/jobs
// Get job of dashboard user
func (s *JobstatsServer) jobs(w http.ResponseWriter, r *http.Request) {
	// Get current logged user and dashboard user from headers
	loggedUser, dashboardUser := s.getUser(r)

	// Query for jobs and write response
	s.jobsQuerier([]string{dashboardUser}, loggedUser, w, r)
}

// GET /api/accounts
// Get accounts list of user
func (s *JobstatsServer) accounts(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Make wuery
	q := Query{}
	q.query(fmt.Sprintf("SELECT DISTINCT account FROM %s", base.JobsDBTableName))
	q.query(" WHERE usr IN ")
	q.param([]string{dashboardUser})

	// Make query and check for accounts returned in usage
	accounts, err := s.Querier(s.db, q, "accounts", s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch accounts", "user", dashboardUser, "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	accountsResponse := Response{
		Status: "success",
		Data:   accounts.([]base.Account),
	}
	if err = json.NewEncoder(w).Encode(&accountsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// Make sub query for fetching accounts of users
func (s *JobstatsServer) accountsSubQuery(users []string) Query {
	// Make a sub query that will fetch accounts of users
	qSub := Query{}
	qSub.query(fmt.Sprintf("SELECT DISTINCT account FROM %s", base.UsageDBTableName))

	// Add conditions to sub query
	if len(users) > 0 {
		qSub.query(" WHERE usr IN ")
		qSub.param(users)
	}
	return qSub
}

// GET /api/usage/current
// Get current usage statistics
func (s *JobstatsServer) currentUsage(users []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for accounts
	qSub := s.accountsSubQuery(users)

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(aggJobDBCols, ","), base.JobsDBTableName))

	// First select all accounts that user is part of using subquery
	q.query(" WHERE account IN ")
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
	q.query(" AND end BETWEEN ")
	q.param([]string{queryWindowTS["from"]})
	q.query(" AND ")
	q.param([]string{queryWindowTS["to"]})

	// Finally add GROUP BY clause
	groupByCols := []string{"account"}
	if groupby := r.URL.Query()["groupby"]; len(groupby) > 0 {
		groupByCols = append(groupByCols, groupby...)
	}
	q.query(fmt.Sprintf(" GROUP BY %s", strings.Join(groupByCols, ",")))

	// Make query and check for returned number of rows
	usage, err := s.Querier(s.db, q, base.UsageResourceName, s.logger)
	if err != nil {
		level.Error(s.logger).
			Log("msg", "Failed to fetch current usage statistics", "users", strings.Join(users, ","), "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	accountsResponse := Response{
		Status: "success",
		Data:   usage.([]base.Usage),
	}
	if err = json.NewEncoder(w).Encode(&accountsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/usage/global
// Get global usage statistics
func (s *JobstatsServer) globalUsage(users []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for accounts
	qSub := s.accountsSubQuery(users)

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(base.UsageDBColNames, ","), base.UsageDBTableName))

	// First select all accounts that user is part of using subquery
	q.query(" WHERE account IN ")
	q.subQuery(qSub)

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Make query and check for returned number of rows
	usage, err := s.Querier(s.db, q, base.UsageResourceName, s.logger)
	if err != nil {
		level.Error(s.logger).
			Log("msg", "Failed to fetch global usage statistics", "users", strings.Join(users, ","), "err", err)
		errorResponse(w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	accountsResponse := Response{
		Status: "success",
		Data:   usage.([]base.Usage),
	}
	if err = json.NewEncoder(w).Encode(&accountsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/usage
// Get current/global usage statistics
func (s *JobstatsServer) usage(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Get path parameter type
	var queryType string
	var exists bool
	if queryType, exists = mux.Vars(r)["query"]; !exists {
		errorResponse(w, &apiError{errorBadData, invalidRequestError}, s.logger, nil)
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
func (s *JobstatsServer) usageAdmin(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get path parameter type
	var queryType string
	var exists bool
	if queryType, exists = mux.Vars(r)["query"]; !exists {
		errorResponse(w, &apiError{errorBadData, invalidRequestError}, s.logger, nil)
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
