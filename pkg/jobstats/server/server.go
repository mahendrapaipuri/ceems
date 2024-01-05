package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats/db"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
)

// Server config struct
type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	DBConfig         db.Config
	AdminUsers       []string
}

// Job Stats Server struct
type JobstatsServer struct {
	logger      log.Logger
	server      *http.Server
	webConfig   *web.FlagConfig
	dbConfig    db.Config
	adminUsers  []string
	Accounts    func(string, string, log.Logger) ([]base.Account, error)
	Jobs        func(Query, log.Logger) ([]base.BatchJob, error)
	HealthCheck func(log.Logger) bool
}

// Query builder struct
type Query struct {
	builder strings.Builder
	params  []string
}

// Add query to builder
func (q *Query) query(s string) {
	q.builder.WriteString(s)
}

// Add parameter and its placeholder
func (q *Query) param(val []string) {
	q.builder.WriteString(fmt.Sprintf("(%s)", strings.Join(strings.Split(strings.Repeat("?", len(val)), ""), ",")))
	q.params = append(q.params, val...)
}

// Add multiple parameters and its placeholder
// func (q *Query) multiParam(val []string) {
// 	q.builder.WriteString(fmt.Sprintf("(%s)", strings.Join(strings.Split(strings.Repeat("?", len(val)), ""), ",")))
// 	q.params = append(q.params, val...)
// }

// Get current query string and its parameters
func (q *Query) get() (string, []string) {
	return q.builder.String(), q.params
}

var dbConn *sql.DB

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
		dbConfig:    c.DBConfig,
		adminUsers:  c.AdminUsers,
		Accounts:    fetchAccounts,
		Jobs:        fetchJobs,
		HealthCheck: getDBStatus,
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
			</body>
			</html>`))
	})

	// Allow only GET methods
	router.HandleFunc("/api/health", server.health).Methods("GET")
	router.HandleFunc("/api/accounts", server.accounts).Methods("GET")
	router.HandleFunc("/api/jobs", server.jobs).Methods("GET")

	// pprof debug end points
	router.PathPrefix("/debug/").Handler(http.DefaultServeMux)

	// Open DB connection
	var err error
	dbConn, err = sql.Open("sqlite3", c.DBConfig.JobstatsDBPath)
	if err != nil {
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
func (s *JobstatsServer) Shutdown(ctx context.Context, wg *sync.WaitGroup) error {
	if err := s.server.Shutdown(ctx); err != nil {
		level.Error(s.logger).Log("msg", "Failed to shutdown HTTP server", "err", err)
		return err
	}
	wg.Done()
	return nil
}

// Get current user from the header
func (s *JobstatsServer) getUser(r *http.Request) (string, string) {
	// Check if username header is available
	loggedUser := r.Header.Get("X-Grafana-User")
	if loggedUser == "" {
		level.Warn(s.logger).Log("msg", "Header X-Grafana-User not found")
		return loggedUser, loggedUser
	}

	// If current user is in list of admin users, get "actual" user from
	// X-Dashboard-User header. For normal users, this header will be exactly same
	// as their username.
	// For admin users who can look at dashboard of "any" user this will be the
	// username of the "impersonated" user and we take it into account
	// Besides, X-Dashboard-User can have a special user "all" that will return jobs
	// of all users
	if slices.Contains(s.adminUsers, loggedUser) {
		if dashboardUser := r.Header.Get("X-Dashboard-User"); dashboardUser != "" {
			level.Info(s.logger).Log(
				"msg", "Admin user accessing dashboards", "loggedUser", loggedUser, "dashboardUser", dashboardUser,
			)
			return loggedUser, dashboardUser
		}
	}
	return loggedUser, loggedUser
}

// Set response headers
func (s *JobstatsServer) setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// GET /api/accounts
// Get all accounts of a user
func (s *JobstatsServer) accounts(w http.ResponseWriter, r *http.Request) {
	var response base.AccountsResponse
	s.setHeaders(w)

	// Get current user from header
	loggedUser, dashboardUser := s.getUser(r)
	// If no user found, return empty response
	if loggedUser == "" {
		w.WriteHeader(http.StatusUnauthorized)
		response = base.AccountsResponse{
			Response: base.Response{
				Status:    "error",
				ErrorType: "User Error",
				Error:     "No user identified",
			},
			Data: []base.Account{},
		}
		err := json.NewEncoder(w).Encode(&response)
		if err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
		return
	}

	// Get all user accounts
	accounts, err := s.Accounts(s.dbConfig.JobstatsDBTable, dashboardUser, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch accounts", "user", dashboardUser, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		response = base.AccountsResponse{
			Response: base.Response{
				Status:    "error",
				ErrorType: "Internal server error",
				Error:     "Failed to fetch user accounts",
			},
			Data: []base.Account{},
		}
		err = json.NewEncoder(w).Encode(&response)
		if err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	response = base.AccountsResponse{
		Response: base.Response{
			Status: "success",
		},
		Data: accounts,
	}
	err = json.NewEncoder(w).Encode(&response)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// Return error response for jobs with setting errorString and errorType in response
func (s *JobstatsServer) jobsErrorResponse(errorString string, errorType string, w http.ResponseWriter) {
	response := base.JobsResponse{
		Response: base.Response{
			Status:    "error",
			ErrorType: errorString,
			Error:     errorType,
		},
		Data: []base.BatchJob{},
	}
	err := json.NewEncoder(w).Encode(&response)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /api/jobs
// Get jobs of a user based on query params
func (s *JobstatsServer) jobs(w http.ResponseWriter, r *http.Request) {
	var fromTime, toTime time.Time
	var response base.JobsResponse
	s.setHeaders(w)

	// Initialise utility vars
	checkQueryWindow := true                             // Check query window size
	defaultQueryWindow := time.Duration(168) * time.Hour // One week

	// Get current logged user and dashboard user from headers
	loggedUser, dashboardUser := s.getUser(r)
	// If no user found, return empty response
	if loggedUser == "" {
		w.WriteHeader(http.StatusUnauthorized)
		s.jobsErrorResponse("User Error", "No user identified", w)
		return
	}

	// Initialise query builder
	q := Query{}
	q.query(fmt.Sprintf("SELECT * FROM %s", s.dbConfig.JobstatsDBTable))

	// Add dummy condition at the beginning
	q.query(" WHERE id > 0")

	// Check if logged user is in admin users and if we are quering for "all" jobs
	// If that is not the case, add user condition in query
	if !slices.Contains(s.adminUsers, loggedUser) || dashboardUser != "all" {
		q.query(" AND Usr IN ")
		q.param([]string{dashboardUser})
	}

	// Get account query parameters if any
	if accounts := r.URL.Query()["account"]; len(accounts) > 0 {
		q.query(" AND Account IN ")
		q.param(accounts)
	}

	// Check if jobuuid present in query params and add them
	// If any of jobuuid or jobid query params are present
	// do not check query window as we are fetching a specific job(s)
	if jobuuids := r.URL.Query()["jobuuid"]; len(jobuuids) > 0 {
		q.query(" AND Jobuuid IN ")
		q.param(jobuuids)
		checkQueryWindow = false
	}

	// Similarly check for jobid
	if jobids := r.URL.Query()["jobid"]; len(jobids) > 0 {
		q.query(" AND Jobid IN ")
		q.param(jobids)
		checkQueryWindow = false
	}

	// If we dont have to specific query window skip next section of code as it becomes
	// irrelevant
	if !checkQueryWindow {
		goto queryJobs
	}

	// Get to and from query parameters and do checks on them
	if f := r.URL.Query().Get("from"); f == "" {
		// If from is not present in query params, use a default query window of 1 week
		fromTime = time.Now().Add(-defaultQueryWindow)
	} else {
		// Return error response if from is not a timestamp
		if ts, err := strconv.Atoi(f); err != nil {
			level.Error(s.logger).Log("msg", "Failed to parse from timestamp", "from", f, "err", err)
			w.WriteHeader(http.StatusBadRequest)
			s.jobsErrorResponse("Internal server error", "Malformed 'from' timestamp", w)
			return
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
			w.WriteHeader(http.StatusBadRequest)
			s.jobsErrorResponse("Internal server error", "Malformed 'to' timestamp", w)
			return
		} else {
			toTime = time.Unix(int64(ts), 0)
		}
	}

	// If difference between from and to is more than 1 months, return with empty
	// response. This is to prevent users from making "big" requests that can "potentially"
	// choke server and end up in OOM errors
	if toTime.Sub(fromTime) > time.Duration(30*24)*time.Hour {
		level.Error(s.logger).Log(
			"msg", "Exceeded maximum query time window of 3 months",
			"from", fromTime.Format(time.DateTime), "to", toTime.Format(time.DateTime),
			"queryWindow", toTime.Sub(fromTime).String(),
		)
		w.WriteHeader(http.StatusBadRequest)
		s.jobsErrorResponse("Internal server error", "Maximum query window exceeded", w)
		return
	}

	// Add from and to to query only when checkQueryWindow is true
	q.query(" AND Start BETWEEN ")
	q.param([]string{fromTime.Format(time.DateTime)})
	q.query(" AND ")
	q.param([]string{toTime.Format(time.DateTime)})

queryJobs:

	// Get all user jobs in the given time window
	jobs, err := s.Jobs(q, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch jobs", "loggedUser", loggedUser, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		s.jobsErrorResponse("Internal server error", "Failed to fetch user jobs", w)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	response = base.JobsResponse{
		Response: base.Response{
			Status: "success",
		},
		Data: jobs,
	}
	err = json.NewEncoder(w).Encode(&response)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// Check status of server
func (s *JobstatsServer) health(w http.ResponseWriter, r *http.Request) {
	if !s.HealthCheck(s.logger) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("KO"))
	} else {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// Get user accounts using SQL query
func fetchAccounts(dbTable string, user string, logger log.Logger) ([]base.Account, error) {
	// Prepare statement
	stmt, err := dbConn.Prepare(fmt.Sprintf("SELECT DISTINCT Account FROM %s WHERE Usr = ?", dbTable))
	if err != nil {
		level.Error(logger).Log("msg", "Failed to prepare SQL statement for accounts query", "user", user, "err", err)
		return nil, err
	}

	defer stmt.Close()
	rows, err := stmt.Query(user)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute SQL statement for accounts query", "user", user, "err", err)
		return nil, err
	}

	// Loop through rows, using Scan to assign column data to struct fields.
	var accounts []base.Account
	var accountNames []string
	for rows.Next() {
		var account string
		if err := rows.Scan(&account); err != nil {
			level.Error(logger).Log("msg", "Could not scan row for accounts query", "user", user, "err", err)
		}
		accounts = append(accounts, base.Account{ID: account})
		accountNames = append(accountNames, account)
	}
	level.Debug(logger).Log("msg", "Accounts found for user", "user", user, "accounts", strings.Join(accountNames, ","))
	return accounts, nil
}

// Get user jobs within a given time window
func fetchJobs(
	query Query,
	logger log.Logger,
) ([]base.BatchJob, error) {
	var numJobs int
	// Get query string and params
	queryString, queryParams := query.get()

	// Prepare SQL statements
	countStmt, err := dbConn.Prepare(strings.Replace(queryString, "*", "COUNT(*)", 1))
	if err != nil {
		level.Error(logger).Log(
			"msg", "Failed to prepare count SQL statement for jobs", "query", queryString,
			"queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}
	defer countStmt.Close()

	queryStmt, err := dbConn.Prepare(strings.Replace(queryString, "*", strings.Join(base.BatchJobFieldNames, ","), 1))
	if err != nil {
		level.Error(logger).Log(
			"msg", "Failed to prepare query SQL statement for jobs", "query", queryString,
			"queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}
	defer queryStmt.Close()

	// queryParams has to be an inteface. Do casting here
	qParams := make([]interface{}, len(queryParams))
	for i, v := range queryParams {
		qParams[i] = v
	}

	// First make a query to get number of rows that will be returned by query
	_ = countStmt.QueryRow(qParams...).Scan(&numJobs)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute count SQL statement for jobs query",
			"query", queryString, "queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}

	rows, err := queryStmt.Query(qParams...)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute query SQL statement for jobs query",
			"query", queryString, "queryParams", strings.Join(queryParams, ","), "err", err,
		)
		return nil, err
	}

	// Loop through rows, using Scan to assign column data to struct fields.
	var jobs = make([]base.BatchJob, numJobs)
	rowIdx := 0
	for rows.Next() {
		var jobid, jobuuid,
			partition, qos, account,
			group, gid, user, uid,
			submit, start, end,
			submitTs, startTs, endTs,
			elapsed, exitcode, state,
			nnodes, ncpus, nodelist,
			nodelistExp, jobName, workDir string
		if err := rows.Scan(
			&jobid,
			&jobuuid,
			&partition,
			&qos,
			&account,
			&group,
			&gid,
			&user,
			&uid,
			&submit,
			&start,
			&end,
			&submitTs,
			&startTs,
			&endTs,
			&elapsed,
			&exitcode,
			&state,
			&nnodes,
			&ncpus,
			&nodelist,
			&nodelistExp,
			&jobName,
			&workDir,
		); err != nil {
			level.Error(logger).Log("msg", "Could not scan row for accounts query", "user", user, "err", err)
		}
		jobs[rowIdx] = base.BatchJob{
			Jobid:       jobid,
			Jobuuid:     jobuuid,
			Partition:   partition,
			QoS:         qos,
			Account:     account,
			Grp:         group,
			Gid:         gid,
			Usr:         user,
			Uid:         uid,
			Submit:      submit,
			Start:       start,
			End:         end,
			SubmitTS:    submitTs,
			StartTS:     startTs,
			EndTS:       endTs,
			Elapsed:     elapsed,
			Exitcode:    exitcode,
			State:       state,
			Nnodes:      nnodes,
			Ncpus:       ncpus,
			Nodelist:    nodelist,
			NodelistExp: nodelistExp,
			JobName:     jobName,
			WorkDir:     workDir,
		}
		rowIdx++
	}
	level.Debug(logger).Log(
		"msg", "Jobs found for user", "numjobs", len(jobs), "query", queryString,
		"queryParams", strings.Join(queryParams, ","),
	)
	return jobs, nil
}

// Ping DB for connection test
func getDBStatus(logger log.Logger) bool {
	err := dbConn.Ping()
	if err != nil {
		level.Error(logger).Log("msg", "DB Ping failed", "err", err)
		return false
	}
	return true
}
