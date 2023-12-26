package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats/helper"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
)

// Server config struct
type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	JobstatDBFile    string
	JobstatDBTable   string
}

// Job Stats Server struct
type JobstatsServer struct {
	logger      log.Logger
	server      *http.Server
	webConfig   *web.FlagConfig
	Accounts    func(string, log.Logger) ([]base.Account, error)
	Jobs        func(string, []string, string, string, log.Logger) ([]base.BatchJob, error)
	HealthCheck func(log.Logger) bool
}

var (
	dbConn         *sql.DB
	jobstatDBFile  string
	jobstatDBTable string
	dateFormat     = "2006-01-02T15:04:05"
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
		Accounts:    fetchAccounts,
		Jobs:        fetchJobs,
		HealthCheck: getDBStatus,
	}

	jobstatDBFile = c.JobstatDBFile
	jobstatDBTable = c.JobstatDBTable

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
	dbConn, err = sql.Open("sqlite3", jobstatDBFile)
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
	accounts, err := s.Accounts(user, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to retrieve accounts", "user", user, "err", err)
		response = base.AccountsResponse{
			Response: base.Response{
				Status:    "error",
				ErrorType: "Data error",
				Error:     "Failed to retrieve user accounts",
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
		response = base.JobsResponse{
			Response: base.Response{
				Status:    "error",
				ErrorType: "User Error",
				Error:     "No user identified",
			},
			Data: []base.BatchJob{},
		}
		err := json.NewEncoder(w).Encode(&response)
		if err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
		return
	}

	// Get query parameters
	accounts := r.URL.Query()["account"]
	// Add accounts from var-account query parameter as well
	// This is to take into account query string made by grafana when getting
	// from variables
	accounts = append(accounts, r.URL.Query()["var-account"]...)

	var from, to string
	// If no from provided, use 1 week from now as from
	if from = r.URL.Query().Get("from"); from == "" {
		from = time.Now().Add(-time.Duration(168) * time.Hour).Format(dateFormat)
	}
	if to = r.URL.Query().Get("to"); to == "" {
		to = time.Now().Format(dateFormat)
	}

	// Get all user jobs in the given time window
	jobs, err := s.Jobs(user, accounts, from, to, s.logger)
	if err != nil {
		level.Error(s.logger).Log("msg", "Failed to retrieve jobs", "user", user, "err", err)
		response = base.JobsResponse{
			Response: base.Response{
				Status:    "error",
				ErrorType: "Data error",
				Error:     "Failed to retrieve user jobs",
			},
			Data: []base.BatchJob{},
		}
		err = json.NewEncoder(w).Encode(&response)
		if err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
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
func fetchAccounts(user string, logger log.Logger) ([]base.Account, error) {
	// Prepare statement
	stmt, err := dbConn.Prepare(fmt.Sprintf("SELECT DISTINCT Account FROM %s WHERE Usr = ?", jobstatDBTable))
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

// Get user jobs using SQL query
func fetchJobs(user string, accounts []string, from string, to string, logger log.Logger) ([]base.BatchJob, error) {
	allFields := helper.GetStructFieldName(base.BatchJob{})

	// Prepare SQL statement
	stmt, err := dbConn.Prepare(
		fmt.Sprintf("SELECT %s FROM %s WHERE Usr = ? AND Start BETWEEN ? AND ? AND Account IN (%s)",
			strings.Join(allFields, ","),
			jobstatDBTable,
			strings.Join(strings.Split(strings.Repeat("?", len(accounts)), ""), ","),
		),
	)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to prepare SQL statement for jobs query", "user", user, "err", err)
		return nil, err
	}

	defer stmt.Close()

	// Prepare query args
	var args = []any{user, from, to}
	for _, account := range accounts {
		args = append(args, account)
	}

	rows, err := stmt.Query(args...)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute SQL statement for jobs query", "user", user, "err", err)
		return nil, err
	}

	// Loop through rows, using Scan to assign column data to struct fields.
	var jobs []base.BatchJob
	for rows.Next() {
		var jobid, jobuuid,
			partition, account,
			group, gid, user, uid,
			submit, start, end, elapsed,
			exitcode, state,
			nnodes, nodelist, nodelistExp string
		if err := rows.Scan(
			&jobid,
			&jobuuid,
			&partition,
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
			&nodelist,
			&nodelistExp,
		); err != nil {
			level.Error(logger).Log("msg", "Could not scan row for accounts query", "user", user, "err", err)
		}
		jobs = append(jobs,
			base.BatchJob{
				Jobid:       jobid,
				Jobuuid:     jobuuid,
				Partition:   partition,
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
				Nodelist:    nodelist,
				NodelistExp: nodelistExp,
			},
		)
	}
	level.Debug(logger).Log(
		"msg", "Jobs found for user", "user", user,
		"numjobs", len(jobs), "accounts", strings.Join(accounts, ","), "from", from, "to", to,
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
