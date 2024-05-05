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
	"github.com/mahendrapaipuri/ceems/pkg/api/http/docs"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/exporter-toolkit/web"
	httpSwagger "github.com/swaggo/http-swagger/v2"
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
type Response[T any] struct {
	Status    string    `json:"status"`
	Data      []T       `json:"data,omitempty"`
	ErrorType errorType `json:"errorType,omitempty"`
	Error     string    `json:"error,omitempty"`
	Warnings  []string  `json:"warnings,omitempty"`
}

var (
	aggUsageDBCols     = make(map[string]string, len(base.UsageDBTableColNames))
	defaultQueryWindow = time.Duration(2 * time.Hour) // Two hours
)

// Make summary DB col names by using aggregate SQL functions
func init() {
	// Add primary field manually as it is ignored in UsageDBColNames
	aggUsageDBCols["id"] = "id"

	// Use SQL aggregate functions in query
	for _, col := range base.UsageDBTableColNames {
		if strings.HasPrefix(col, "avg") {
			aggUsageDBCols[col] = fmt.Sprintf("SUM(%[1]s * %[2]s) / SUM(%[2]s) AS %[1]s", col, db.Weights[col])
		} else if strings.HasPrefix(col, "total") {
			aggUsageDBCols[col] = fmt.Sprintf("SUM(%[1]s) AS %[1]s", col)
		} else if strings.HasPrefix(col, "num") {
			aggUsageDBCols[col] = "COUNT(id) AS num_units"
		} else {
			aggUsageDBCols[col] = col
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

	// Create a sub router with apiVersion as PathPrefix
	subRouter := router.PathPrefix(fmt.Sprintf("/api/%s/", base.APIVersion)).Subrouter()

	// Allow only GET methods
	subRouter.HandleFunc("/health", server.health).Methods(http.MethodGet)
	subRouter.HandleFunc("/projects", server.projects).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s", unitsResourceName), server.units).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/admin", unitsResourceName), server.unitsAdmin).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/{mode:(?:current|global)}", usageResourceName), server.usage).
		Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/{mode:(?:current|global)}/admin", usageResourceName), server.usageAdmin).
		Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/verify", unitsResourceName), server.verifyUnitsOwnership).
		Methods(http.MethodGet)

	// A demo end point that returns mocked data for units and/or usage tables
	subRouter.HandleFunc("/{resource:(?:units|usage)}/demo", server.demo).Methods(http.MethodGet)

	// pprof debug end points
	router.PathPrefix("/debug/").Handler(http.DefaultServeMux)

	router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("doc.json"), // The url pointing to API definition
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	)).Methods(http.MethodGet)

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

// Start launches CEEMS HTTP server godoc
//
//	@title						CEEMS API
//	@version					1.0
//	@description				CEEMS REST API server.
//	@termsOfService				http://swagger.io/terms/
//
//	@contact.name				Mahendra Paipuri
//	@contact.url				https://github.com/mahendrapaipuri/ceems/issues
//	@contact.email				mahendra.paipuri@gmail.com
//
//	@license.name				BSD-3-Clause license
//	@license.url				https://opensource.org/license/bsd-3-clause
//
//	@securityDefinitions.basic	BasicAuth
func (s *CEEMSServer) Start() error {
	// Set swagger info
	docs.SwaggerInfo.BasePath = fmt.Sprintf("/api/%s", base.APIVersion)
	docs.SwaggerInfo.Schemes = []string{"http", "https"}
	docs.SwaggerInfo.Host = s.server.Addr

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

// health godoc
//
//	@Summary		Health status
//	@Description	get health status of API server
//	@Tags			health
//	@Produce		plain
//	@Success		200	{string}	OK
//	@Failure		503	{string}	KO
//	@Router			/health [get]
//
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

// Fetch queried fields
func (s *CEEMSServer) getQueriedFields(URLValues url.Values, validFieldNames []string) []string {
	// Get fields query parameters if any
	var queriedFields []string
	if fields := URLValues["field"]; len(fields) > 0 {
		// Check if fields are valid field names
		for _, f := range fields {
			if slices.Contains(validFieldNames, f) {
				queriedFields = append(queriedFields, f)
			}
		}
	} else {
		queriedFields = validFieldNames
	}
	return queriedFields
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
	queriedFields := s.getQueriedFields(r.URL.Query(), base.UnitsDBTableColNames)

	// Initialise query builder
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(queriedFields, ","), base.UnitsDBTableName))

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
		errorResponse[any](w, &apiError{errorBadData, err}, s.logger, nil)
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
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	response := Response[models.Unit]{
		Status: "success",
		Data:   units.([]models.Unit),
	}
	if err = json.NewEncoder(w).Encode(&response); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// unitsAdmin    godoc
//
//	@Summary		Admin endpoint for fetching compute units
//	@Description	get units for admins that can query units of any user
//	@Security		BasicAuth
//	@Tags			units
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			uuid			query		[]string	false	"Unit UUID"	collectionFormat(multi)
//	@Param			project			query		[]string	false	"Project"	collectionFormat(multi)
//	@Param			user			query		[]string	false	"User name"	collectionFormat(multi)
//	@Param			running			query		bool		false	"Whether to fetch running units"
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Unit]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/units/admin [get]
//
// GET /units/admin
// Get any unit of any user
func (s *CEEMSServer) unitsAdmin(w http.ResponseWriter, r *http.Request) {
	// Query for units and write response
	s.unitsQuerier(r.URL.Query()["user"], w, r)
}

// units         godoc
//
//	@Summary		User endpoint for fetching compute units
//	@Description	get units queried by a user
//	@Security		BasicAuth
//	@Tags			units
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			uuid			query		[]string	false	"Unit UUID"	collectionFormat(multi)
//	@Param			project			query		[]string	false	"Project"	collectionFormat(multi)
//	@Param			running			query		bool		false	"Whether to fetch running units"
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Unit]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/units [get]
//
// GET /units
// Get unit of dashboard user
func (s *CEEMSServer) units(w http.ResponseWriter, r *http.Request) {
	// Get current logged user and dashboard user from headers
	_, dashboardUser := s.getUser(r)

	// Query for units and write response
	s.unitsQuerier([]string{dashboardUser}, w, r)
}

// verifyUnitsOwnership         godoc
//
//	@Summary		Verify unit ownership
//	@Description	verify ownership of the unit
//	@Security		BasicAuth
//	@Tags			units
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			uuid			query		[]string	false	"Unit UUID"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Ownership]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/units/verify [get]
//
// GET /units/verify
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
		response := Response[models.Ownership]{
			Status: "success",
			Data: []models.Ownership{
				{
					User:  dashboardUser,
					UUIDS: uuids,
					Owner: true,
				},
			},
		}
		if err := json.NewEncoder(w).Encode(&response); err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
	} else {
		errorResponse[any](w, &apiError{errorForbidden, fmt.Errorf("user do not have permissions on uuids")}, s.logger, nil)
	}
}

// projects         godoc
//
//	@Summary		List projects
//	@Description	get list of projects that user belong to
//	@Security		BasicAuth
//	@Tags			projects
//	@Produce		json
//	@Param			X-Grafana-User	header		string	true	"Current user name"
//	@Success		200				{object}	Response[models.Project]
//	@Failure		401				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/projects [get]
//
// GET /projects
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
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	projectsResponse := Response[models.Project]{
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

// GET /usage/current
// Get current usage statistics
func (s *CEEMSServer) currentUsage(users []string, fields []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for projects
	qSub := s.projectsSubQuery(users)

	// Get aggUsageCols based on queried fields
	var queriedFields []string
	for _, field := range fields {
		// Ignore last_updated_at col
		if slices.Contains([]string{"last_updated_at"}, field) {
			continue
		}
		queriedFields = append(queriedFields, aggUsageDBCols[field])
	}

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(queriedFields, ","), base.UnitsDBTableName))

	// First select all projects that user is part of using subquery
	q.query(" WHERE project IN ")
	q.subQuery(qSub)

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Get query window time stamps
	queryWindowTS, err := s.getQueryWindow(r)
	if err != nil {
		errorResponse[any](w, &apiError{errorBadData, err}, s.logger, nil)
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
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	projectsResponse := Response[models.Usage]{
		Status: "success",
		Data:   usage.([]models.Usage),
	}
	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /usage/global
// Get global usage statistics
func (s *CEEMSServer) globalUsage(users []string, queriedFields []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for projects
	qSub := s.projectsSubQuery(users)

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(queriedFields, ","), base.UsageDBTableName))

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
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)
		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)
	projectsResponse := Response[models.Usage]{
		Status: "success",
		Data:   usage.([]models.Usage),
	}
	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// usage         godoc
//
//	@Summary		Usage statistics
//	@Description	get current/global usage statistics of a current user
//	@Security		BasicAuth
//	@Tags			usage
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			mode			path		string		true	"Whether to get usage stats within a period or global"	Enums(current, global)
//	@Param			user			query		[]string	false	"User name(s)"											collectionFormat(multi)
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Usage]
//	@Failure		401				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/usage/{mode} [get]
//
// GET /usage/{mode}
// Get current/global usage statistics
func (s *CEEMSServer) usage(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Get path parameter type
	var mode string
	var exists bool
	if mode, exists = mux.Vars(r)["mode"]; !exists {
		errorResponse[any](w, &apiError{errorBadData, errInvalidRequest}, s.logger, nil)
		return
	}

	// Get fields query parameters if any
	queriedFields := s.getQueriedFields(r.URL.Query(), base.UsageDBTableColNames)

	// handle current usage query
	if mode == "current" {
		s.currentUsage([]string{dashboardUser}, queriedFields, w, r)
	}

	// handle global usage query
	if mode == "global" {
		s.globalUsage([]string{dashboardUser}, queriedFields, w, r)
	}
}

// usage         godoc
//
//	@Summary		Admin Usage statistics
//	@Description	get current/global usage statistics of a given user and/or project for admins
//	@Security		BasicAuth
//	@Tags			usage
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			mode			path		string		true	"Whether to get usage stats within a period or global"	Enums(current, global)
//	@Param			user			query		[]string	false	"User name(s)"											collectionFormat(multi)
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Usage]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/usage/{mode}/admin [get]
//
// GET /usage/{mode}/admin
// Get current/global usage statistics of any user
func (s *CEEMSServer) usageAdmin(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get path parameter type
	var mode string
	var exists bool
	if mode, exists = mux.Vars(r)["mode"]; !exists {
		errorResponse[any](w, &apiError{errorBadData, errInvalidRequest}, s.logger, nil)
		return
	}

	// Get fields query parameters if any
	queriedFields := s.getQueriedFields(r.URL.Query(), base.UsageDBTableColNames)

	// handle current usage query
	if mode == "current" {
		s.currentUsage(r.URL.Query()["user"], queriedFields, w, r)
	}

	// handle global usage query
	if mode == "global" {
		s.globalUsage(r.URL.Query()["user"], queriedFields, w, r)
	}
}

// usage         godoc
//
//	@Summary		Demo Units/Usage endpoints
//	@Description	get units and/or usage response generated by mock data
//	@Tags			demo
//	@Produce		json
//	@Param			resource	path		string	true	"Whether to return mock units or usage data"	Enums(units, usage)
//	@Success		200			{object}	Response[models.Unit]
//	@Success		200			{object}	Response[models.Usage]
//	@Failure		500			{object}	Response[any]
//	@Router			/{resource}/demo [get]
//
// GET /demo/{units,usage}
// Return mocked data for different models
func (s *CEEMSServer) demo(w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get path parameter type
	var resourceType string
	var exists bool
	if resourceType, exists = mux.Vars(r)["resource"]; !exists {
		errorResponse[any](w, &apiError{errorBadData, errInvalidRequest}, s.logger, nil)
		return
	}

	// handle units mock data
	if resourceType == "units" {
		units := mockUnits()
		// Write response
		w.WriteHeader(http.StatusOK)
		unitsResponse := Response[models.Unit]{
			Status: "success",
			Data:   units,
		}
		if err := json.NewEncoder(w).Encode(&unitsResponse); err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
	}

	// handle usage mock data
	if resourceType == "usage" {
		usage := mockUsage()
		// Write response
		w.WriteHeader(http.StatusOK)
		usageResponse := Response[models.Usage]{
			Status: "success",
			Data:   usage,
		}
		if err := json.NewEncoder(w).Encode(&usageResponse); err != nil {
			level.Error(s.logger).Log("msg", "Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
	}
}
