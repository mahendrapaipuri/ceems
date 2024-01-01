package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"slices"
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
	Jobs        func(string, string, []string, []string, []string, string, string, log.Logger) ([]base.BatchJob, error)
	HealthCheck func(log.Logger) bool
}

var (
	dbConn     *sql.DB
	dateFormat = "2006-01-02T15:04:05"
)

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
func (s *JobstatsServer) getUser(r *http.Request) string {
	// Check if username header is available
	user := r.Header.Get("X-Grafana-User")
	if user == "" {
		level.Warn(s.logger).Log("msg", "Header X-Grafana-User not found")
		return user
	}

	// If current user is in list of admin users, get "actual" user from
	// X-Dashboard-User header. For normal users, this header will be exactly same
	// as their username.
	// For admin users who can look at dashboard of "any" user this will be the
	// username of the "impersonated" user and we take it into account
	if slices.Contains(s.adminUsers, user) {
		if dashboardUser := r.Header.Get("X-Dashboard-User"); dashboardUser != "" {
			level.Info(s.logger).Log(
				"msg", "Admin user accessing dashboards", "currentUser", user, "impersonatedUser", dashboardUser,
			)
			user = dashboardUser
		}
	}
	return user
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
	w.WriteHeader(http.StatusOK)

	// Get current user from header
	user := s.getUser(r)
	// If no user found, return empty response
	if user == "" {
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
	accounts, err := s.Accounts(s.dbConfig.JobstatsDBTable, user, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch accounts", "user", user, "err", err)
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
	var response base.JobsResponse
	s.setHeaders(w)
	w.WriteHeader(http.StatusOK)

	// Get current user from header
	user := s.getUser(r)
	// If no user found, return empty response
	if user == "" {
		s.jobsErrorResponse("User Error", "No user identified", w)
		return
	}

	// Get query parameters
	accounts := r.URL.Query()["account"]
	// Add accounts from var-account query parameter as well
	// This is to take into account query string made by grafana when getting
	// from variables
	accounts = append(accounts, r.URL.Query()["var-account"]...)

	// Check if jobuuid or var-jobuuid is present in query params
	jobuuids := r.URL.Query()["jobuuid"]
	jobuuids = append(jobuuids, r.URL.Query()["var-jobuuid"]...)

	// Similarly check for jobid or var-jobid
	jobids := r.URL.Query()["jobid"]
	jobids = append(jobids, r.URL.Query()["var-jobid"]...)

	var from, to string
	var checkQueryWindow = true
	// If no from provided, use 1 week from now as from
	if from = r.URL.Query().Get("from"); from == "" {
		// If query is made for particular jobs and from is not present, use the
		// default retention period as from as we need to look in whole DB
		if len(jobuuids) > 0 || len(jobids) > 0 {
			from = time.Now().Add(-s.dbConfig.RetentionPeriod).Format(dateFormat)
			checkQueryWindow = false
		} else {
			from = time.Now().Add(-time.Duration(168) * time.Hour).Format(dateFormat)
		}
	}
	if to = r.URL.Query().Get("to"); to == "" {
		to = time.Now().Format(dateFormat)
	}

	// If jobuuids or jobids is present, turn off checkQueryWindow check
	if len(jobuuids) > 0 || len(jobids) > 0 {
		checkQueryWindow = false
	}

	// Convert "from" and "to" to time.Time and return error response if they
	// cannot be parsed
	fromTime, err := time.Parse(dateFormat, from)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to parse from datestring", "from", from, "err", err)
		s.jobsErrorResponse("Internal server error", "Malformed 'from' time string", w)
		return
	}
	toTime, err := time.Parse(dateFormat, to)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to parse from datestring", "to", to, "err", err)
		s.jobsErrorResponse("Internal server error", "Malformed 'to' time string", w)
		return
	}

	// If difference between from and to is more than 3 months, return with empty
	// response. This is to prevent users from making "big" requests that can "potentially"
	// choke server and end up in OOM errors
	if toTime.Sub(fromTime) > time.Duration(3*30*24)*time.Hour && checkQueryWindow {
		level.Error(s.logger).Log(
			"msg", "Exceeded maximum query time window of 3 months", "from", from, "to", to,
			"queryWindow", toTime.Sub(fromTime).String(),
		)
		s.jobsErrorResponse("Internal server error", "Maximum query window exceeded", w)
		return
	}

	// Get all user jobs in the given time window
	jobs, err := s.Jobs(s.dbConfig.JobstatsDBTable, user, accounts, jobids, jobuuids, from, to, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to fetch jobs", "user", user, "err", err)
		s.jobsErrorResponse("Internal server error", "Failed to fetch user jobs", w)
		return
	}

	// Write response
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
	dbTable string,
	user string,
	accounts []string,
	jobids []string,
	jobuuids []string,
	from string,
	to string,
	logger log.Logger,
) ([]base.BatchJob, error) {
	var numJobs int

	// Requested jobuuids for logging
	queryAccounts := strings.Join(accounts, ",")
	queryJobuuids := strings.Join(jobuuids, ",")
	queryJobids := strings.Join(jobids, ",")

	// Placeholders
	accountPlaceholder := strings.Join(strings.Split(strings.Repeat("?", len(accounts)), ""), ",")
	jobuuidPlaceholder := strings.Join(strings.Split(strings.Repeat("?", len(jobuuids)), ""), ",")
	jobidPlaceholder := strings.Join(strings.Split(strings.Repeat("?", len(jobids)), ""), ",")

	// Make correct query strings based on queried jobuuid and jobid
	var queryStmtString string
	var countStmtString string
	if len(jobuuids) > 0 && len(jobids) > 0 {
		countStmtString = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s) AND (Jobuuid IN (%s) OR Jobid IN (%s))",
			dbTable,
			accountPlaceholder,
			jobuuidPlaceholder,
			jobidPlaceholder,
		)
		queryStmtString = fmt.Sprintf("SELECT %s FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s) AND (Jobuuid IN (%s) OR Jobid IN (%s))",
			strings.Join(base.BatchJobFieldNames, ","),
			dbTable,
			accountPlaceholder,
			jobuuidPlaceholder,
			jobidPlaceholder,
		)
	} else if len(jobuuids) > 0 && len(jobids) == 0 {
		countStmtString = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s) AND Jobuuid IN (%s)",
			dbTable,
			accountPlaceholder,
			jobuuidPlaceholder,
		)
		queryStmtString = fmt.Sprintf("SELECT %s FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s) AND Jobuuid IN (%s)",
			strings.Join(base.BatchJobFieldNames, ","),
			dbTable,
			accountPlaceholder,
			jobuuidPlaceholder,
		)
	} else if len(jobuuids) == 0 && len(jobids) > 0 {
		countStmtString = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s) AND Jobid IN (%s)",
			dbTable,
			accountPlaceholder,
			jobidPlaceholder,
		)
		queryStmtString = fmt.Sprintf("SELECT %s FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s) AND Jobid IN (%s)",
			strings.Join(base.BatchJobFieldNames, ","),
			dbTable,
			accountPlaceholder,
			jobidPlaceholder,
		)
	} else {
		countStmtString = fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s)",
			dbTable,
			accountPlaceholder,
		)
		queryStmtString = fmt.Sprintf("SELECT %s FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s)",
			strings.Join(base.BatchJobFieldNames, ","),
			dbTable,
			accountPlaceholder,
		)
	}

	// Prepare SQL statements
	countStmt, err := dbConn.Prepare(countStmtString)
	if err != nil {
		level.Error(logger).Log(
			"msg", "Failed to prepare count SQL statement for jobs", "user", user,
			"accounts", queryAccounts, "jobuuids", queryJobuuids, "jobids", queryJobids,
			"from", from, "to", to, "err", err,
		)
		return nil, err
	}
	defer countStmt.Close()

	queryStmt, err := dbConn.Prepare(queryStmtString)
	if err != nil {
		level.Error(logger).Log(
			"msg", "Failed to prepare query SQL statement for jobs", "user", user,
			"accounts", queryAccounts, "jobuuids", queryJobuuids, "jobids", queryJobids,
			"from", from, "to", to, "err", err,
		)
		return nil, err
	}
	defer queryStmt.Close()

	// Prepare query args
	var args = []any{user, from, to}
	for _, account := range accounts {
		args = append(args, account)
	}
	for _, jobuuid := range jobuuids {
		args = append(args, jobuuid)
	}
	for _, jobid := range jobids {
		args = append(args, jobid)
	}

	// First make a query to get number of rows that will be returned by query
	_ = countStmt.QueryRow(args...).Scan(&numJobs)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute count SQL statement for jobs query",
			"user", user, "accounts", queryAccounts, "jobuuids", queryJobuuids, "jobids", queryJobids,
			"from", from, "to", to, "err", err,
		)
		return nil, err
	}

	rows, err := queryStmt.Query(args...)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute query SQL statement for jobs query",
			"user", user, "accounts", queryAccounts, "jobuuids", queryJobuuids, "jobids", queryJobids,
			"from", from, "to", to, "err", err,
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
			submit, start, end, elapsed,
			exitcode, state,
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
		"msg", "Jobs found for user", "user", user,
		"numjobs", len(jobs), "accounts", queryAccounts, "jobuuids", queryJobuuids, "jobids", queryJobids,
		"from", from, "to", to,
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
