//go:build cgo
// +build cgo

// Package http implements the HTTP server handlers for different resource endpoints
package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/httprate"
	"github.com/gorilla/mux"
	"github.com/jellydator/ttlcache/v3"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/db"
	"github.com/mahendrapaipuri/ceems/pkg/api/http/docs"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/sqlite3"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/exporter-toolkit/web"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// API Resources names.
const (
	unitsResourceName      = "units"
	usageResourceName      = "usage"
	adminUsersResourceName = "admin_users"
	usersResourceName      = "users"
	projectsResourceName   = "projects"
	clustersResourceName   = "clusters"
	statsResourceName      = "stats"
)

// Usage modes.
const (
	currentUsage = "current"
	globalUsage  = "global"
)

// WebConfig makes HTTP web config from CLI args.
type WebConfig struct {
	Addresses        []string
	WebSystemdSocket bool
	WebConfigFile    string
	RoutePrefix      string                  `yaml:"route_prefix"`
	MaxQueryPeriod   model.Duration          `yaml:"max_query"`
	RequestsLimit    int                     `yaml:"requests_limit"`
	URL              string                  `yaml:"url"`
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	*c = WebConfig{
		RoutePrefix: "/",
	}

	type plain WebConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Set HTTPClientConfig in Web to empty struct as we do not and should not need
	// CEEMS API server's client config on the server. The client config is only used
	// in LB
	//
	// If we are using the same config file for both API server and LB,
	// secrets will be available in the client config and to reduce attack surface we
	// remove them all here by setting it to empty struct
	c.HTTPClientConfig = config.HTTPClientConfig{}

	return nil
}

// Config makes a server config.
type Config struct {
	Logger *slog.Logger
	Web    WebConfig
	DB     db.Config
}

type queriers struct {
	unit    func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.Unit, error)
	usage   func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.Usage, error)
	user    func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.User, error)
	project func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.Project, error)
	cluster func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.Cluster, error)
	stat    func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.Stat, error)
	key     func(context.Context, *sql.DB, Query, *slog.Logger) ([]models.Key, error)
}

// CEEMSServer struct implements HTTP server for stats.
type CEEMSServer struct {
	logger         *slog.Logger
	server         *http.Server
	webConfig      *web.FlagConfig
	db             *sql.DB
	dbConfig       db.Config
	maxQueryPeriod time.Duration
	queriers       queriers
	usageCache     *ttlcache.Cache[uint64, []models.Usage] // Cache that stores usage query results
	healthCheck    func(*sql.DB, *slog.Logger) bool
}

// Response defines the response model of CEEMSAPIServer.
type Response[T any] struct {
	Status    string    `json:"status"`
	Data      []T       `json:"data"`
	ErrorType errorType `json:"errorType,omitempty"`
	Error     string    `json:"error,omitempty"`
	Warnings  []string  `json:"warnings,omitempty"`
}

var (
	aggUsageQueries    = make(map[string]string, len(base.UsageDBTableColNames))
	cacheTTL           = 15 * time.Minute
	defaultQueryWindow = 24 * time.Hour // One day
)

const (
	// Query to get quick stats like active projects, groups, jobs, etc.
	statsQuery = `cluster_id,resource_manager,COUNT(*) AS num_units,COUNT(CASE WHEN ended_at_ts > 0 THEN 1 END) as num_inactive_units,COUNT(CASE WHEN ended_at_ts = 0 THEN 1 END) as num_active_units,COUNT(DISTINCT project) AS num_projects,COUNT(DISTINCT username) AS num_users`
)

// Make summary DB col names by using aggregate SQL functions.
func init() {
	// Use SQL aggregate functions in query
	// For metrics involving total and averages, we use templates to build query
	// after fetching all the keys in each metric map
	for _, col := range base.UsageDBTableColNames {
		switch {
		case strings.HasPrefix(col, "num"):
			if col == "num_units" {
				aggUsageQueries[col] = "COUNT(u.id) AS num_units"
			} else {
				aggUsageQueries[col] = "SUM(u.num_updates) AS num_updates"
			}
		case strings.HasPrefix(col, "total"):
			aggUsageQueries[col] = `{{- $mn := .MetricName -}}json_object({{range $i, $r := .MetricKeys}}{{if $i}},{{end}}'{{$r.Name}}',(SUM({{$mn}}_{{$r.Name}}.value)){{end}}) AS {{.MetricName}}||{{range $i, $r := .MetricKeys}}{{if $i}}|{{end}}json_each({{$mn}},'$.{{$r.Name}}') AS {{$mn}}_{{$r.Name}}{{end}}`
		case strings.HasPrefix(col, "avg"):
			aggUsageQueries[col] = `{{- $mn := .MetricName -}}{{- $mw := .MetricWeight -}}{{- $tmn := .TimesMetricName -}}json_object({{range $i, $r := .MetricKeys}}{{if $i}},{{end}}'{{$r.Name}}',(SUM({{$mn}}_{{$r.Name}}.value*{{$tmn}}_{{$mw}}.value) / SUM({{$tmn}}_{{$mw}}.value)){{end}}) AS {{.MetricName}}||{{range $i, $r := .MetricKeys}}{{if $i}}|{{end}}json_each({{$mn}},'$.{{$r.Name}}') AS {{$mn}}_{{$r.Name}}{{end}}|json_each({{$tmn}},'$.{{$mw}}') AS {{$tmn}}_{{$mw}}`
		default:
			aggUsageQueries[col] = col
		}
	}
}

// Ping DB for connection test.
func getDBStatus(dbConn *sql.DB, logger *slog.Logger) bool {
	if err := dbConn.Ping(); err != nil {
		logger.Error("DB Ping failed", "err", err)

		return false
	}

	return true
}

// New creates new CEEMSServer struct instance.
func New(c *Config) (*CEEMSServer, func(), error) {
	var err error

	router := mux.NewRouter()
	server := &CEEMSServer{
		logger: c.Logger,
		server: &http.Server{
			Addr:              c.Web.Addresses[0],
			Handler:           router,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			ReadHeaderTimeout: 2 * time.Second, // slowloris attack: https://app.deepsource.com/directory/analyzers/go/issues/GO-S2112
		},
		webConfig: &web.FlagConfig{
			WebListenAddresses: &c.Web.Addresses,
			WebSystemdSocket:   &c.Web.WebSystemdSocket,
			WebConfigFile:      &c.Web.WebConfigFile,
		},
		dbConfig:       c.DB,
		maxQueryPeriod: time.Duration(c.Web.MaxQueryPeriod),
		queriers: queriers{
			unit:    Querier[models.Unit],
			usage:   Querier[models.Usage],
			user:    Querier[models.User],
			project: Querier[models.Project],
			cluster: Querier[models.Cluster],
			stat:    Querier[models.Stat],
			key:     Querier[models.Key],
		},
		healthCheck: getDBStatus,
	}

	// Get route prefix based on external URL path
	var routePrefix string
	if c.Web.RoutePrefix != "/" {
		routePrefix = fmt.Sprintf("%s/api/%s/", c.Web.RoutePrefix, base.APIVersion)
	} else {
		routePrefix = fmt.Sprintf("/api/%s/", base.APIVersion)
	}

	c.Logger.Debug("CEEMS API server running on prefix", "prefix", routePrefix)

	// Create a sub router with apiVersion as PathPrefix
	subRouter := router.PathPrefix(routePrefix).Subrouter()

	// If the prefix is missing for the root path, prepend it.
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, routePrefix, http.StatusFound)
	})

	subRouter.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html>
			<head><title>CEEMS API Server</title></head>
			<body>
			<h1>Compute Stats</h1>
			<p><a href="swagger/index.html">Swagger API</a></p>
			</body>
			</html>`))
	})

	// Allow only GET methods
	subRouter.HandleFunc("/health", server.health).Methods(http.MethodGet)
	subRouter.HandleFunc("/"+usersResourceName, server.users).Methods(http.MethodGet)
	subRouter.HandleFunc("/"+projectsResourceName, server.projects).Methods(http.MethodGet)
	subRouter.HandleFunc("/"+unitsResourceName, server.units).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/{mode:(?:current|global)}", usageResourceName), server.usage).
		Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/verify", unitsResourceName), server.verifyUnitsOwnership).
		Methods(http.MethodGet)

	// Admin end points
	subRouter.HandleFunc(fmt.Sprintf("/%s/admin", usersResourceName), server.usersAdmin).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/admin", projectsResourceName), server.projectsAdmin).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/admin", clustersResourceName), server.clustersAdmin).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/admin", unitsResourceName), server.unitsAdmin).Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/{mode:(?:current|global)}/admin", usageResourceName), server.usageAdmin).
		Methods(http.MethodGet)
	subRouter.HandleFunc(fmt.Sprintf("/%s/{mode:(?:current|global)}/admin", statsResourceName), server.statsAdmin).
		Methods(http.MethodGet)

	// A demo end point that returns mocked data for units and/or usage tables
	subRouter.HandleFunc("/demo/{resource:(?:units|usage)}", server.demo).Methods(http.MethodGet)

	// pprof debug end points. Expose them only on localhost
	router.PathPrefix("/debug/").Handler(http.DefaultServeMux).Host("localhost")

	subRouter.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("doc.json"), // The url pointing to API definition
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DomID("swagger-ui"),
	)).Methods(http.MethodGet)

	// Open DB connection
	dsn := fmt.Sprintf(
		"file:%s?%s",
		filepath.Join(c.DB.Data.Path, base.CEEMSDBName),
		"_mutex=no&mode=ro&_busy_timeout=5000",
	)
	if server.db, err = sql.Open(sqlite3.DriverName, dsn); err != nil {
		return nil, func() {}, fmt.Errorf("failed to open DB: %w", err)
	}

	// Rate limit requests by RealIP
	if c.Web.RequestsLimit > 0 {
		c.Logger.Debug("Rate limiting settings", "reqs_per_minute", c.Web.RequestsLimit)
		router.Use(httprate.LimitByRealIP(c.Web.RequestsLimit, time.Minute))
	}

	// Add a middleware that verifies headers and pass them in requests
	// The middleware will fetch admin users from Grafana periodically to update list
	amw := authenticationMiddleware{
		logger:          c.Logger,
		routerPrefix:    routePrefix,
		whitelistedURLs: regexp.MustCompile(routePrefix + "(swagger|health|demo)(.*)"),
		db:              server.db,
		adminUsers:      adminUsers,
	}
	router.Use(amw.Middleware)

	// Instantiate new cache for storing current usage query results with TTL of 15 min
	server.usageCache = ttlcache.New(
		ttlcache.WithTTL[uint64, []models.Usage](cacheTTL),
	)
	// starts automatic expired item deletion
	go server.usageCache.Start()

	return server, func() {}, nil
}

// Start launches CEEMS HTTP server godoc
//
//	@title			CEEMS API
//	@version		1.0
//	@description	OpenAPI specification (OAS) for the CEEMS REST API.
//	@description
//	@description	See the Interactive Docs to try CEEMS API methods without writing code, and get
//	@description	the complete schema of resources exposed by the API.
//	@description
//	@description	If basic auth is enabled, all the endpoints require authentication.
//	@description
//	@description	All the endpoints, except `health`, `swagger`, `debug` and `demo`,
//	@description	must send a user-agent header.
//	@description
//	@description				Timestamps must be specified in milliseconds, unless otherwise specified.
//
//	@contact.name				Mahendra Paipuri
//	@contact.url				https://github.com/mahendrapaipuri/ceems/issues
//	@contact.email				mahendra.paipuri@gmail.com
//
//	@license.name				GPL-3.0 license
//	@license.url				https://www.gnu.org/licenses/gpl-3.0.en.html
//
//	@securityDefinitions.basic	BasicAuth
//
//	@externalDocs.url			https://mahendrapaipuri.github.io/ceems/
func (s *CEEMSServer) Start() error {
	// Set swagger info
	docs.SwaggerInfo.BasePath = "/api/" + base.APIVersion
	docs.SwaggerInfo.Schemes = []string{"http", "https"}
	docs.SwaggerInfo.Host = s.server.Addr

	s.logger.Info("Starting " + base.CEEMSServerAppName)

	if err := web.ListenAndServe(s.server, s.webConfig, s.logger); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.logger.Error("Failed to Listen and Serve HTTP server", "err", err)

		return err
	}

	return nil
}

// Shutdown server.
func (s *CEEMSServer) Shutdown(ctx context.Context) error {
	// Close DB connection
	if err := s.db.Close(); err != nil {
		s.logger.Error("Failed to close DB connection", "err", err)

		return err
	}

	// Shutdown the server
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Failed to shutdown HTTP server", "err", err)

		return err
	}

	return nil
}

// Get current user from the header.
func (s *CEEMSServer) getUser(r *http.Request) (string, string) {
	return r.Header.Get(loggedUserHeader), r.Header.Get(dashboardUserHeader)
}

// setHeaders sets common response headers.
func (s *CEEMSServer) setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// setWriteDeadline sets write deadline to the request.
func (s *CEEMSServer) setWriteDeadline(deadline time.Duration, w http.ResponseWriter) {
	// Response controller
	rc := http.NewResponseController(w) //nolint:bodyclose

	// Set write deadline to this request
	if err := rc.SetWriteDeadline(time.Now().Add(deadline)); err != nil {
		s.logger.Error("Failed to set write deadline", "err", err)
	}
}

// health godoc
//
//	@Summary		Health status
//	@Description	This endpoint returns the health status of the server.
//	@Description
//	@Description	A healthy server returns 200 response code and any other
//	@Description	responses should be treated as unhealthy server.
//	@Tags			health
//	@Produce		plain
//	@Success		200	{string}	OK
//	@Failure		503	{string}	KO
//	@Router			/health [get]
//
// Check status of server.
func (s *CEEMSServer) health(w http.ResponseWriter, r *http.Request) {
	if !s.healthCheck(s.db, s.logger) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("KO"))
	} else {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// getCommonQueryParams fetches project and running query parameters and add them to query.
func (s *CEEMSServer) getCommonQueryParams(q *Query, urlValues url.Values) Query {
	// Get project query parameters if any
	if projects := urlValues["project"]; len(projects) > 0 {
		q.query(" AND project IN ")
		q.param(projects)
	}

	// Get cluster_id query parameters if any
	if clusterIDs := urlValues["cluster_id"]; len(clusterIDs) > 0 {
		q.query(" AND cluster_id IN ")
		q.param(clusterIDs)
	}

	return *q
}

// getQueriedFields returns a slice of queried fields.
func (s *CEEMSServer) getQueriedFields(urlValues url.Values, validFieldNames []string) []string {
	// Get fields query parameters if any
	var queriedFields []string

	if fields := urlValues["field"]; len(fields) > 0 {
		// Check if fields are valid field names
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if slices.Contains(validFieldNames, f) {
				queriedFields = append(queriedFields, f)
			}
		}
	} else {
		queriedFields = validFieldNames
	}

	return queriedFields
}

// timeLocation returns `time.Location` based on location name.
func (s *CEEMSServer) timeLocation(l string) *time.Location {
	if l == "" {
		return s.dbConfig.Data.Timezone.Location
	} else {
		if loc, err := time.LoadLocation(l); err != nil {
			return s.dbConfig.Data.Timezone.Location
		} else {
			return loc
		}
	}
}

// getQueryWindow returns `from` and `to` time stamps from query vars and
// cast them into proper format.
func (s *CEEMSServer) getQueryWindow(r *http.Request) (map[string]string, error) {
	q := r.URL.Query()

	var fromTime, toTime time.Time
	// Get to and from query parameters and do checks on them
	if f := q.Get("from"); f == "" {
		// If from is not present in query params, use a default query window of 1 week
		fromTime = time.Now().Add(-defaultQueryWindow).In(s.dbConfig.Data.Timezone.Location)
	} else {
		// Return error response if from is not a timestamp
		if ts, err := strconv.ParseInt(f, 10, 64); err != nil {
			s.logger.Error("Failed to parse from timestamp", "from", f, "err", err)

			return nil, fmt.Errorf("query parameter 'from': %w", ErrMalformedTimeStamp)
		} else {
			fromTime = time.Unix(ts, 0).In(s.dbConfig.Data.Timezone.Location)
		}
	}

	if t := q.Get("to"); t == "" {
		// Use current time as default to
		toTime = time.Now().In(s.dbConfig.Data.Timezone.Location)
	} else {
		// Return error response if to is not a timestamp
		if ts, err := strconv.ParseInt(t, 10, 64); err != nil {
			s.logger.Error("Failed to parse to timestamp", "to", t, "err", err)

			return nil, fmt.Errorf("query parameter 'to': %w", ErrMalformedTimeStamp)
		} else {
			toTime = time.Unix(ts, 0).In(s.dbConfig.Data.Timezone.Location)
		}
	}

	// If difference between from and to is more than max query period, return with empty
	// response. This is to prevent users from making "big" requests that can "potentially"
	// choke server and end up in OOM errors
	if s.maxQueryPeriod > 0*time.Second && toTime.Sub(fromTime) > s.maxQueryPeriod {
		s.logger.Error(
			"Exceeded maximum query time window",
			"max_query_window", s.maxQueryPeriod,
			"from", fromTime.Format(time.DateTime), "to", toTime.Format(time.DateTime),
			"query_window", toTime.Sub(fromTime).String(),
		)

		return nil, ErrMaxQueryWindow
	}

	return map[string]string{
		"from": fromTime.Format(base.DatetimeLayout),
		"to":   toTime.Format(base.DatetimeLayout),
	}, nil
}

// roundQueryWindow rounds `to` and `from` query parameters to nearest multiple of
// `cacheTTL`.
func (s *CEEMSServer) roundQueryWindow(r *http.Request) error {
	cacheTTLSeconds := int64(cacheTTL.Seconds())
	q := r.URL.Query()

	// Get to and from query parameters and do checks on them
	if f := q.Get("from"); f == "" {
		q.Set(
			"from",
			strconv.FormatInt(
				common.Round(
					time.Now().Add(-defaultQueryWindow).In(s.dbConfig.Data.Timezone.Location).Unix(),
					cacheTTLSeconds,
				), 10,
			),
		)
	} else {
		// Return error response if from is not a timestamp
		if ts, err := strconv.ParseInt(f, 10, 64); err != nil {
			s.logger.Error("Failed to parse from timestamp", "from", f, "err", err)

			return fmt.Errorf("query parameter 'from': %w", ErrMalformedTimeStamp)
		} else {
			q.Set("from", strconv.FormatInt(common.Round(ts, cacheTTLSeconds), 10))
		}
	}

	if t := q.Get("to"); t == "" {
		q.Set(
			"to",
			strconv.FormatInt(
				common.Round(
					time.Now().In(s.dbConfig.Data.Timezone.Location).Unix(),
					cacheTTLSeconds,
				), 10,
			),
		)
	} else {
		// Return error response if from is not a timestamp
		if ts, err := strconv.ParseInt(t, 10, 64); err != nil {
			s.logger.Error("Failed to parse from timestamp", "to", t, "err", err)

			return fmt.Errorf("query parameter 'to': %w", ErrMalformedTimeStamp)
		} else {
			q.Set("to", strconv.FormatInt(common.Round(ts, cacheTTLSeconds), 10))
		}
	}

	r.URL.RawQuery = q.Encode()

	return nil
}

// inTargetTimeLocation converts the string representations of times in units to target
// time location.
func (s *CEEMSServer) inTargetTimeLocation(tz string, units []models.Unit) []models.Unit {
	// If no time zone is provided, we present times stored in DB without any changes
	if tz == "" {
		return units
	}

	// Location in which we need times to be presented
	targetLoc := s.timeLocation(tz)

	// If target location is same as source, return
	if s.dbConfig.Data.Timezone.Location.String() == targetLoc.String() {
		return units
	}

	for i := range units {
		units[i].CreatedAt = convertTimeLocation(s.dbConfig.Data.Timezone.Location, targetLoc, units[i].CreatedAt)
		units[i].StartedAt = convertTimeLocation(s.dbConfig.Data.Timezone.Location, targetLoc, units[i].StartedAt)
		units[i].EndedAt = convertTimeLocation(s.dbConfig.Data.Timezone.Location, targetLoc, units[i].EndedAt)
	}

	return units
}

// unitsQuerier queries for compute units and write response.
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

	// Set write deadline
	s.setWriteDeadline(5*time.Minute, w)

	// Initialise utility vars
	checkQueryWindow := true // Check query window size

	// Get fields query parameters if any
	queriedFields := s.getQueriedFields(r.URL.Query(), base.UnitsDBTableColNames)
	if len(queriedFields) == 0 {
		s.logger.Error("Invalid query fields", "loggedUser", loggedUser, "err", errInvalidQueryField)
		errorResponse[any](w, &apiError{errorBadData, errInvalidQueryField}, s.logger, nil)

		return
	}

	// Initialise query builder
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(queriedFields, ","), base.UnitsDBTableName))

	// Query for only unignored units
	q.query(" WHERE ignore = 0 ")

	// Add condition to query only for current dashboardUser
	if len(queriedUsers) > 0 {
		q.query(" AND username IN ")
		q.param(queriedUsers)
	}

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Check if running query param is included
	// Running units will have ended_at_ts as 0 and we use this in query to
	// fetch these units
	if _, ok := r.URL.Query()["running"]; ok {
		q.query(" OR ended_at_ts IN ")
		q.param([]string{"0"})
	}

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
	// Sort by uuid
	q.query(" ORDER BY cluster_id ASC, uuid ASC ")

	// Get all user units in the given time window
	units, err := s.queriers.unit(r.Context(), s.db, q, s.logger)
	if units == nil && err != nil {
		s.logger.Error("Failed to fetch units", "loggedUser", loggedUser, "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Convert times to time zone provided in the query
	units = s.inTargetTimeLocation(r.URL.Query().Get("timezone"), units)

	// Write response
	w.WriteHeader(http.StatusOK)

	response := Response[models.Unit]{
		Status: "success",
		Data:   units,
	}
	if err != nil {
		response.Warnings = append(response.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&response); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// unitsAdmin    godoc
//
//	@Summary		Admin endpoint for fetching compute units.
//	@Description	This admin endpoint will fetch compute units of _any_ user, compute unit and/or project. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The user who is making the request must be in the list of admin users
//	@Description	configured for the server.
//	@Description
//	@Description	If multiple query parameters are passed, for instance, `?uuid=<uuid>&user=<user>`,
//	@Description	the intersection of query parameters are used to fetch compute units rather than
//	@Description	the union. That means if the compute unit's `uuid` does not belong to the queried
//	@Description	user, null response will be returned.
//	@Description
//	@Description	In order to return the running compute units as well, use the query parameter `running`.
//	@Description
//	@Description	If `to` query parameter is not provided, current time will be used. If `from`
//	@Description	query parameter is not used, a default query window of 24 hours will be used.
//	@Description	It means if `to` is provided, `from` will be calculated as `to` - 24hrs. If query
//	@Description	parameter `timezone` is provided, the unit's created, start and end time strings
//	@Description	will be presented in that time zone.
//	@Description
//	@Description	To limit the number of fields in the response, use `field` query parameter. By default, all
//	@Description	fields will be included in the response if they are _non-empty_.
//	@Security		BasicAuth
//	@Tags			units
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Param			uuid			query		[]string	false	"Unit UUID"		collectionFormat(multi)
//	@Param			project			query		[]string	false	"Project"		collectionFormat(multi)
//	@Param			user			query		[]string	false	"User name"		collectionFormat(multi)
//	@Param			running			query		bool		false	"Whether to fetch running units"
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			timezone		query		string		false	"Time zone in IANA format"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Unit]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/units/admin [get]
//
// GET /units/admin
// Get any unit of any user.
func (s *CEEMSServer) unitsAdmin(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "units admin endpoint", s.logger)

	// Query for units and write response
	s.unitsQuerier(r.URL.Query()["user"], w, r)
}

// units         godoc
//
//	@Summary		User endpoint for fetching compute units
//	@Description	This user endpoint will fetch compute units of the current user. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	If multiple query parameters are passed, for instance, `?uuid=<uuid>&project=<project>`,
//	@Description	the intersection of query parameters are used to fetch compute units rather than
//	@Description	the union. That means if the compute unit's `uuid` does not belong to the queried
//	@Description	project, null response will be returned.
//	@Description
//	@Description	In order to return the running compute units as well, use the query parameter `running`.
//	@Description
//	@Description	If `to` query parameter is not provided, current time will be used. If `from`
//	@Description	query parameter is not used, a default query window of 24 hours will be used.
//	@Description	It means if `to` is provided, `from` will be calculated as `to` - 24hrs. If query
//	@Description	parameter `timezone` is provided, the unit's created, start and end time strings
//	@Description	will be presented in that time zone.
//	@Description
//	@Description	To limit the number of fields in the response, use `field` query parameter. By default, all
//	@Description	fields will be included in the response if they are _non-empty_.
//	@Security		BasicAuth
//	@Tags			units
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Param			uuid			query		[]string	false	"Unit UUID"		collectionFormat(multi)
//	@Param			project			query		[]string	false	"Project"		collectionFormat(multi)
//	@Param			running			query		bool		false	"Whether to fetch running units"
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			timezone		query		string		false	"Time zone in IANA format"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Unit]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/units [get]
//
// GET /units
// Get unit of dashboard user.
func (s *CEEMSServer) units(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "units endpoint", s.logger)

	// Get current logged user and dashboard user from headers
	_, dashboardUser := s.getUser(r)

	// Query for units and write response
	s.unitsQuerier([]string{dashboardUser}, w, r)
}

// verifyUnitsOwnership         godoc
//
//	@Summary		Verify unit ownership
//	@Description	This endpoint will check if the current user is the owner of the
//	@Description	queried UUIDs. The current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	A response of 200 means that the current user is the owner of the queried UUIDs.
//	@Description	Any other response code should be treated as the current user not being the owner
//	@Description	of the queried units.
//	@Description
//	@Description	The ownership check passes if any of the following conditions are `true`:
//	@Description	- If the current user is the _direct_ owner of the compute unit.
//	@Description	- If the current user belongs to the same account/project/namespace as
//	@Description	the compute unit. This means the users belonging to the same project can
//	@Description	access each others compute units.
//	@Description
//	@Description	The above checks must pass for **all** the queried units.
//	@Description	If the check does not pass for at least one queried unit, a response 403 will be
//	@Description	returned.
//	@Description
//	@Description	Any 500 response codes should be treated as failed check as well.
//	@Security		BasicAuth
//	@Tags			units
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			uuid			query		[]string	false	"Unit UUID"		collectionFormat(multi)
//	@Param			cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Param			time			query		[]string	false	"Timestamps"	collectionFormat(multi)
//	@Success		200				{object}	Response[any]
//	@Failure		401				{object}	Response[any]
//	@Failure		403				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/units/verify [get]
//
// GET /units/verify
// Verify the user ownership for queried units.
func (s *CEEMSServer) verifyUnitsOwnership(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "verify endpoint", s.logger)

	// Set headers
	s.setHeaders(w)

	// Get current logged user and dashboard user from headers
	_, dashboardUser := s.getUser(r)

	// Get cluster ID
	clusterID := r.URL.Query()["cluster_id"]

	// Get list of queried uuids
	uuids := r.URL.Query()["uuid"]
	if len(uuids) == 0 {
		errorResponse[any](w, &apiError{errorBadData, errMissingUUIDs}, s.logger, nil)

		return
	}

	// Get start time of queried uuids
	var starts []int64

	for _, s := range r.URL.Query()["time"] {
		if is, err := strconv.ParseInt(s, 10, 64); err == nil {
			starts = append(starts, is)
		}
	}

	// Check if user is owner of the queries uuids
	if VerifyOwnership(r.Context(), dashboardUser, clusterID, uuids, starts, s.db, s.logger) {
		w.WriteHeader(http.StatusOK)

		response := Response[string]{
			Status: "success",
		}
		if err := json.NewEncoder(w).Encode(&response); err != nil {
			s.logger.Error("Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
	} else {
		errorResponse[any](w, &apiError{errorForbidden, errNoAuth}, s.logger, nil)
	}
}

// clusters         godoc
//
//	@Summary		List clusters
//	@Description	This endpoint will list all the cluster IDs in the CEEMS DB. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	This will list all the cluster IDs in the DB. This is primarily
//	@Description	used to verify the CEEMS load balancer's backend IDs that should match
//	@Description	with cluster IDs.
//	@Description
//	@Security	BasicAuth
//	@Tags		clusters
//	@Produce	json
//	@Param		X-Grafana-User	header		string	true	"Current user name"
//	@Success	200				{object}	Response[models.Cluster]
//	@Failure	401				{object}	Response[any]
//	@Failure	500				{object}	Response[any]
//	@Router		/clusters/admin [get]
//
// GET /clusters/admin
// Get clusters list in the DB.
func (s *CEEMSServer) clustersAdmin(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "clusters admin endpoint", s.logger)

	// Set headers
	s.setHeaders(w)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Make query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT DISTINCT cluster_id, resource_manager FROM %s ORDER BY cluster_id ASC",
			base.UnitsDBTableName,
		),
	)

	// Make query and get list of cluster ids
	clusterIDs, err := s.queriers.cluster(r.Context(), s.db, q, s.logger)
	if clusterIDs == nil && err != nil {
		s.logger.Error("Failed to fetch cluster IDs", "user", dashboardUser, "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)

	clusterIDsResponse := Response[models.Cluster]{
		Status: "success",
		Data:   clusterIDs,
	}
	if err != nil {
		clusterIDsResponse.Warnings = append(clusterIDsResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&clusterIDsResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// Get user details.
func (s *CEEMSServer) usersQuerier(users []string, w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Make query
	q := Query{}
	q.query("SELECT * FROM " + base.UsersDBTableName)
	// If no user is queried, return all users. This can happen only for admin
	// end points
	if len(users) == 0 {
		q.query(" WHERE name LIKE '%' ")
	} else {
		q.query(" WHERE name IN ")
		q.param(users)
	}

	// Get cluster_id query parameters if any
	if clusterIDs := r.URL.Query()["cluster_id"]; len(clusterIDs) > 0 {
		q.query(" AND cluster_id IN ")
		q.param(clusterIDs)
	}

	// Sort by cluster_id and name
	q.query(" ORDER BY cluster_id ASC, name ASC ")

	// Make query and check for users returned in usage
	userModels, err := s.queriers.user(r.Context(), s.db, q, s.logger)
	if userModels == nil && err != nil {
		s.logger.Error("Failed to fetch user details", "users", strings.Join(users, ","), "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)

	usersResponse := Response[models.User]{
		Status: "success",
		Data:   userModels,
	}
	if err != nil {
		usersResponse.Warnings = append(usersResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&usersResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// users         godoc
//
//	@Summary		Show user details
//	@Description	This endpoint will show details of the current user. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The details include list of projects that user is currently a part of.
//	@Description
//	@Security	BasicAuth
//	@Tags		users
//	@Produce	json
//	@Param		X-Grafana-User	header		string		true	"Current user name"
//	@Param		cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Success	200				{object}	Response[models.User]
//	@Failure	401				{object}	Response[any]
//	@Failure	500				{object}	Response[any]
//	@Router		/users [get]
//
// GET /users
// Get users details.
func (s *CEEMSServer) users(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "users endpoint", s.logger)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Query for users and write response
	s.usersQuerier([]string{dashboardUser}, w, r)
}

// usersAdmin         godoc
//
//	@Summary		Admin endpoint for fetching user details of _any_ user.
//	@Description	This endpoint will show details of the queried user(s). The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The user who is making the request must be in the list of admin users
//	@Description	configured for the server.
//	@Description
//	@Description	When the query parameter `user` is empty, all users will be returned
//	@Description	in the response.
//	@Description
//	@Description	The details include list of projects that user is currently a part of.
//	@Description
//	@Security	BasicAuth
//	@Tags		users
//	@Produce	json
//	@Param		X-Grafana-User	header		string		true	"Current user name"
//	@Param		user			query		[]string	false	"User name"		collectionFormat(multi)
//	@Param		cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Success	200				{object}	Response[models.User]
//	@Failure	401				{object}	Response[any]
//	@Failure	500				{object}	Response[any]
//	@Router		/users/admin [get]
//
// GET /users/admin
// Get users details.
func (s *CEEMSServer) usersAdmin(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "users admin endpoint", s.logger)

	// Query for users and write response
	s.usersQuerier(r.URL.Query()["user"], w, r)
}

// Get project details.
func (s *CEEMSServer) projectsQuerier(users []string, w http.ResponseWriter, r *http.Request) {
	// Set headers
	s.setHeaders(w)

	// Get sub query for projects
	qSub := projectsSubQuery(users)

	// Make query
	q := Query{}
	q.query("SELECT * FROM " + base.ProjectsDBTableName)

	// First select all projects that user is part of using subquery
	q.query(" WHERE name IN ")
	q.subQuery(qSub)

	// Get project query parameters if any
	if projects := r.URL.Query()["project"]; len(projects) > 0 {
		q.query(" AND name IN ")
		q.param(projects)
	}

	// Get cluster_id query parameters if any
	if clusterIDs := r.URL.Query()["cluster_id"]; len(clusterIDs) > 0 {
		q.query(" AND cluster_id IN ")
		q.param(clusterIDs)
	}

	// Sort by cluster_id and name
	q.query(" ORDER BY cluster_id ASC, name ASC ")

	// Make query
	projectModels, err := s.queriers.project(r.Context(), s.db, q, s.logger)
	if projectModels == nil && err != nil {
		s.logger.Error(
			"Failed to fetch project details",
			"users", strings.Join(users, ","), "err", err,
		)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)

	projectsResponse := Response[models.Project]{
		Status: "success",
		Data:   projectModels,
	}
	if err != nil {
		projectsResponse.Warnings = append(projectsResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// projects         godoc
//
//	@Summary		Show project details
//	@Description	This endpoint will show details of the queried project of current user. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The details include list of users in that project. If current user
//	@Description	attempts to query a project that they are not part of, empty response
//	@Description	will be returned
//	@Description
//	@Security	BasicAuth
//	@Tags		projects
//	@Produce	json
//	@Param		X-Grafana-User	header		string		true	"Current user name"
//	@Param		project			query		[]string	false	"Project"		collectionFormat(multi)
//	@Param		cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Success	200				{object}	Response[models.Project]
//	@Failure	401				{object}	Response[any]
//	@Failure	500				{object}	Response[any]
//	@Router		/projects [get]
//
// GET /projects
// Get project details.
func (s *CEEMSServer) projects(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "projects endpoint", s.logger)

	// Get current user from header
	_, dashboardUser := s.getUser(r)

	// Make query and write response
	s.projectsQuerier([]string{dashboardUser}, w, r)
}

// projectsAdmin         godoc
//
//	@Summary		Admin endpoint to fetch project details
//	@Description	This endpoint will show details of the queried project. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The user who is making the request must be in the list of admin users
//	@Description	configured for the server.
//	@Description
//	@Description	The details include list of users in that project. If current user
//	@Description	attempts to query a project that they are not part of, empty response
//	@Description	will be returned
//	@Description
//	@Security	BasicAuth
//	@Tags		projects
//	@Produce	json
//	@Param		X-Grafana-User	header		string		true	"Current user name"
//	@Param		project			query		[]string	false	"Project"		collectionFormat(multi)
//	@Param		cluster_id		query		[]string	false	"Cluster ID"	collectionFormat(multi)
//	@Success	200				{object}	Response[models.Project]
//	@Failure	401				{object}	Response[any]
//	@Failure	500				{object}	Response[any]
//	@Router		/projects/admin [get]
//
// GET /projects/admin
// Get project details.
func (s *CEEMSServer) projectsAdmin(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "projects admin endpoint", s.logger)

	// Make query and write response
	s.projectsQuerier(nil, w, r)
}

// aggQueryBuilder builds the aggregate queries for current usage.
func (s *CEEMSServer) aggQueryBuilder(
	r *http.Request,
	metric string,
	queryWindow map[string]string,
) string {
	// Query to return all unqiue json keys
	q := Query{}
	q.query(fmt.Sprintf("SELECT DISTINCT json_each.key AS name FROM %s, json_each(%s)", base.UnitsDBTableName, metric))

	// Ignore null values
	q.query(" WHERE json_each.key IS NOT NULL ")

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Get keys only within the requested period
	q.query(" AND last_updated_at BETWEEN ")
	q.param([]string{queryWindow["from"]})
	q.query(" AND ")
	q.param([]string{queryWindow["to"]})

	// Make query and get keys
	keys, err := s.queriers.key(r.Context(), s.db, q, s.logger)
	if keys == nil && err != nil {
		s.logger.Error("Failed to fetch metric keys", "metric", metric, "err", err)

		return ""
	}

	// Template data
	data := map[string]interface{}{
		"MetricName":      metric,
		"TimesMetricName": "total_time_seconds",
		"MetricWeight":    db.Weights[metric],
		"MetricKeys":      keys,
	}

	// Execute template and get query
	tmpl := template.Must(template.New(metric).Parse(aggUsageQueries[metric]))
	query := &bytes.Buffer{}

	if err := tmpl.Execute(query, data); err != nil {
		s.logger.Error("Failed to execute query template", "metric", metric, "err", err)

		return ""
	}

	return query.String()
}

// GET /usage/current
// Get current usage statistics.
func (s *CEEMSServer) currentUsage(users []string, fields []string, w http.ResponseWriter, r *http.Request) {
	var usage []models.Usage

	var groupby []string

	var targetTable string

	var queryWindowTS map[string]string

	queryParts := make([]string, len(fields))

	var queries, virtualTables []string

	var wg sync.WaitGroup

	var mu sync.RWMutex

	var q Query

	var err, qErrs error

	// Round `to` and `from` query parameters to cacheTTL
	if err := s.roundQueryWindow(r); err != nil {
		errorResponse[any](w, &apiError{errorBadData, err}, s.logger, nil)

		return
	}

	// Get query window time stamps
	queryWindowTS, err = s.getQueryWindow(r)
	if err != nil {
		errorResponse[any](w, &apiError{errorBadData, err}, s.logger, nil)

		return
	}

	// Attempt to retrieve from cache if present
	// Use URL as cache key
	// Add Expires header when cached value is being returned
	cacheKey := common.GenerateKey(r.URL.String())
	if present := s.usageCache.Has(cacheKey); present {
		cacheValue := s.usageCache.Get(cacheKey)
		usage = cacheValue.Value()
		w.Header().Set("Expires", cacheValue.ExpiresAt().Format(time.RFC1123))

		goto writer
	}

	// Set write deadline
	s.setWriteDeadline(5*time.Minute, w)

	// Start a wait group
	wg = sync.WaitGroup{}

	// Get aggUsageCols based on queried fields
	for iField, field := range fields {
		if strings.HasPrefix(field, "avg") || strings.HasPrefix(field, "total") {
			wg.Add(1)

			go func(i int, f string) {
				defer wg.Done()

				if query := s.aggQueryBuilder(r, f, queryWindowTS); query != "" {
					queryParts[i] = query
				} else {
					mu.Lock()
					qErrs = errors.Join(fmt.Errorf("failed to build query for %s", field), qErrs)
					mu.Unlock()
				}
			}(iField, field)
		} else {
			queryParts[iField] = aggUsageQueries[field]
		}
	}

	// Wait for all go routines
	wg.Wait()

	// Each templated query will have two parts delimited by "||".
	// First part is SELECT query that forms JSON object from aggregated metrics
	// Second part is the CTE that makes a virtual table by iterating using json_each
	// Second part is delimited by "|" so that we can get individual virtual tables
	// and remove any duplicates and join them using LEFT JOIN
	// Ignore any empty parts
	for _, query := range queryParts {
		parts := strings.Split(query, "||")
		queries = append(queries, parts[0])

		if len(parts) == 1 || (len(parts) > 1 && parts[1] == "") {
			continue
		}

		for _, p := range strings.Split(parts[1], "|") {
			if p == "" || slices.Contains(virtualTables, p) {
				continue
			}

			virtualTables = append(virtualTables, p)
		}
	}

	if _, ok := r.URL.Query()["experimental"]; ok {
		targetTable = base.DailyUsageDBTableName

		for iQuery, query := range queries {
			if strings.Contains(query, "COUNT") {
				queries[iQuery] = "SUM(u.num_units) AS num_units"
			}
		}
	} else {
		targetTable = base.UnitsDBTableName
	}

	// Make query
	q = Query{}
	q.query(
		fmt.Sprintf(
			"SELECT %s FROM (%s AS u LEFT JOIN %s)",
			strings.Join(queries, ","),
			targetTable,
			strings.Join(virtualTables, " LEFT JOIN "),
		),
	)

	// First select all projects that user is part of using subquery
	q.query(" WHERE project IN ")
	q.subQuery(projectsSubQuery(users)) // Get sub query for projects

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Add from and to to query only when checkQueryWindow is true
	q.query(" AND last_updated_at BETWEEN ")
	q.param([]string{queryWindowTS["from"]})
	q.query(" AND ")
	q.param([]string{queryWindowTS["to"]})

	// Get only units that have finished. We do not present this
	// query parameter for end users. **Only used in testing**
	if _, ok := r.URL.Query()["__terminated"]; ok {
		q.query(" AND ended_at_ts > 0 ")
	}

	// Finally add GROUP BY clause. Always group by username,project
	groupby = []string{"username", "project"}

	for _, q := range r.URL.Query()["groupby"] {
		if q != "" {
			groupby = append(groupby, q)
		}
	}
	// Remove duplicates values
	slices.Sort(groupby)
	groupby = slices.Compact(groupby)
	q.query(" GROUP BY " + strings.Join(groupby, ","))

	// Sort by cluster_id, username and project
	q.query(" ORDER BY cluster_id ASC, username ASC, project ASC ")

	// Make query and check for returned number of rows
	usage, err = s.queriers.usage(r.Context(), s.db, q, s.logger)
	if usage == nil && err != nil {
		s.logger.Error("Failed to fetch current usage statistics", "users", strings.Join(users, ","), "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Push to cache
	if len(usage) > 0 {
		s.usageCache.Set(cacheKey, usage, ttlcache.DefaultTTL)
	}

writer:
	// Write response
	w.WriteHeader(http.StatusOK)

	usageResponse := Response[models.Usage]{
		Status: "success",
		Data:   usage,
	}
	if qErrs != nil {
		usageResponse.Warnings = append(usageResponse.Warnings, qErrs.Error())
	}

	if err != nil {
		usageResponse.Warnings = append(usageResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&usageResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /usage/global
// Get global usage statistics.
func (s *CEEMSServer) globalUsage(users []string, queriedFields []string, w http.ResponseWriter, r *http.Request) {
	// Get sub query for projects
	qSub := projectsSubQuery(users)

	// Make query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", strings.Join(queriedFields, ","), base.UsageDBTableName))

	// First select all projects that user is part of using subquery
	q.query(" WHERE project IN ")
	q.subQuery(qSub)

	// Add common query parameters
	q = s.getCommonQueryParams(&q, r.URL.Query())

	// Sort by cluster_id, username and project
	q.query(" ORDER BY cluster_id ASC, username ASC, project ASC ")

	// Make query and check for returned number of rows
	usage, err := s.queriers.usage(r.Context(), s.db, q, s.logger)
	if usage == nil && err != nil {
		s.logger.Error("Failed to fetch global usage statistics", "users", strings.Join(users, ","), "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)

	usageResponse := Response[models.Usage]{
		Status: "success",
		Data:   usage,
	}
	if err != nil {
		usageResponse.Warnings = append(usageResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&usageResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// usage         godoc
//
//	@Summary		Usage statistics
//	@Description	This endpoint will return the usage statistics current user. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	A path parameter `mode` is required to return the kind of usage statistics.
//	@Description	Currently, two modes of statistics are supported:
//	@Description	- `current`: In this mode the usage between two time periods is returned
//	@Description	based on `from` and `to` query parameters.
//	@Description	- `global`: In this mode the _total_ usage statistics are returned. For
//	@Description	instance, if the retention period of the DB is set to 2 years, usage
//	@Description	statistics of last 2 years will be returned.
//	@Description
//	@Description	The statistics can be limited to certain projects by passing `project` query,
//	@Description	parameter.
//	@Description
//	@Description	If `to` query parameter is not provided, current time will be used. If `from`
//	@Description	query parameter is not used, a default query window of 24 hours will be used.
//	@Description	It means if `to` is provided, `from` will be calculated as `to` - 24hrs.
//	@Description
//	@Description	To limit the number of fields in the response, use `field` query parameter. By default, all
//	@Description	fields will be included in the response if they are _non-empty_.
//	@Description
//	@Description	The `current` usage mode can be slow query depending the requested
//	@Description	window interval. This is mostly due to the fact that the CEEMS DB
//	@Description	uses custom JSON types to store metric data and usage statistics
//	@Description	needs to aggregate metrics over these JSON types using custom aggregate
//	@Description	functions which can be slow.
//	@Description
//	@Description	Therefore the query results are cached for 15 min to avoid load on server.
//	@Description	URL string is used as the cache key. Thus, the query parameters
//	@Description	`from` and `to` are rounded to the nearest timestamp that are
//	@Description	multiple of 900 sec (15 min). The first query will make a DB query and
//	@Description	cache results and subsequent queries, for a given user and same URL
//	@Description	query parameters, will return the same cached result until the cache
//	@Description	is invalidated after 15 min.
//	@Security		BasicAuth
//	@Tags			usage
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			mode			path		string		true	"Whether to get usage stats within a period or global"	Enums(current, global)
//	@Param			cluster_id		query		[]string	false	"cluster ID"											collectionFormat(multi)
//	@Param			project			query		[]string	false	"Project"												collectionFormat(multi)
//	@Param			from			query		string		false	"From timestamp"
//	@Param			to				query		string		false	"To timestamp"
//	@Param			field			query		[]string	false	"Fields to return in response"	collectionFormat(multi)
//	@Success		200				{object}	Response[models.Usage]
//	@Failure		401				{object}	Response[any]
//	@Failure		500				{object}	Response[any]
//	@Router			/usage/{mode} [get]
//
// GET /usage/{mode}
// Get current/global usage statistics.
func (s *CEEMSServer) usage(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "usage endpoint", s.logger)

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
	if len(queriedFields) == 0 {
		s.logger.Error("Invalid query fields", "loggedUser", dashboardUser)
		errorResponse[any](w, &apiError{errorBadData, errInvalidQueryField}, s.logger, nil)

		return
	}

	// handle current usage query
	if mode == currentUsage {
		s.currentUsage([]string{dashboardUser}, queriedFields, w, r)
	}

	// handle global usage query
	if mode == globalUsage {
		s.globalUsage([]string{dashboardUser}, queriedFields, w, r)
	}
}

// usage         godoc
//
//	@Summary		Admin Usage statistics
//	@Description	This admin endpoint will return the usage statistics of _queried_ user. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The user who is making the request must be in the list of admin users
//	@Description	configured for the server.
//	@Description
//	@Description	A path parameter `mode` is required to return the kind of usage statistics.
//	@Description	Currently, two modes of statistics are supported:
//	@Description	- `current`: In this mode the usage between two time periods is returned
//	@Description	based on `from` and `to` query parameters.
//	@Description	- `global`: In this mode the _total_ usage statistics are returned. For
//	@Description	instance, if the retention period of the DB is set to 2 years, usage
//	@Description	statistics of last 2 years will be returned.
//	@Description
//	@Description	The statistics can be limited to certain projects by passing `project` query,
//	@Description	parameter.
//	@Description
//	@Description	If `to` query parameter is not provided, current time will be used. If `from`
//	@Description	query parameter is not used, a default query window of 24 hours will be used.
//	@Description	It means if `to` is provided, `from` will be calculated as `to` - 24hrs.
//	@Description
//	@Description	To limit the number of fields in the response, use `field` query parameter. By default, all
//	@Description	fields will be included in the response if they are _non-empty_.
//	@Description
//	@Description	The `current` usage mode can be slow query depending the requested
//	@Description	window interval. This is mostly due to the fact that the CEEMS DB
//	@Description	uses custom JSON types to store metric data and usage statistics
//	@Description	needs to aggregate metrics over these JSON types using custom aggregate
//	@Description	functions which can be slow.
//	@Description
//	@Description	Therefore the query results are cached for 15 min to avoid load on server.
//	@Description	URL string is used as the cache key. Thus, the query parameters
//	@Description	`from` and `to` are rounded to the nearest timestamp that are
//	@Description	multiple of 900 sec (15 min). The first query will make a DB query and
//	@Description	cache results and subsequent queries, for a given user and same URL
//	@Description	query parameters, will return the same cached result until the cache
//	@Description	is invalidated after 15 min.
//	@Security		BasicAuth
//	@Tags			usage
//	@Produce		json
//	@Param			X-Grafana-User	header		string		true	"Current user name"
//	@Param			mode			path		string		true	"Whether to get usage stats within a period or global"	Enums(current, global)
//	@Param			cluster_id		query		[]string	false	"cluster ID"											collectionFormat(multi)
//	@Param			project			query		[]string	false	"Project"
//	@Param			user			query		[]string	false	"Username"	collectionFormat(multi)
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
// Get current/global usage statistics of any user.
func (s *CEEMSServer) usageAdmin(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "usage admin endpoint", s.logger)

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
	if len(queriedFields) == 0 {
		s.logger.Error("Invalid query fields", "loggedUser", dashboardUser)
		errorResponse[any](w, &apiError{errorBadData, errInvalidQueryField}, s.logger, nil)

		return
	}

	// handle current usage query
	if mode == currentUsage {
		s.currentUsage(r.URL.Query()["user"], queriedFields, w, r)
	}

	// handle global usage query
	if mode == globalUsage {
		s.globalUsage(r.URL.Query()["user"], queriedFields, w, r)
	}
}

// GET /stats/current
// Get current quick stats.
func (s *CEEMSServer) currentStats(users []string, w http.ResponseWriter, r *http.Request) {
	var stats []models.Stat

	var queryWindowTS map[string]string

	var q, qSub Query

	var err error

	// Set write deadline
	s.setWriteDeadline(1*time.Minute, w)

	// Make query
	q = Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s WHERE 1=1", statsQuery, base.UnitsDBTableName))

	// Get query window time stamps
	queryWindowTS, err = s.getQueryWindow(r)
	if err != nil {
		errorResponse[any](w, &apiError{errorBadData, err}, s.logger, nil)

		return
	}

	// Add from and to to query in a sub query so that we can check for both running
	// and terminates units
	qSub = Query{}
	qSub.query("ended_at BETWEEN ")
	qSub.param([]string{queryWindowTS["from"]})
	qSub.query(" AND ")
	qSub.param([]string{queryWindowTS["to"]})
	qSub.query(" OR ended_at_ts IN ")
	qSub.param([]string{"0"})

	// Add sub query to main query
	q.query(" AND ")
	q.subQuery(qSub)

	// Get cluster_id query parameters if any
	if clusterIDs := r.URL.Query()["cluster_id"]; len(clusterIDs) > 0 {
		q.query(" AND cluster_id IN ")
		q.param(clusterIDs)
	}

	// Finally add GROUP BY clause. Always group by cluster_id
	q.query(" GROUP BY cluster_id")

	// Sort by cluster_id, username and project
	q.query(" ORDER BY cluster_id ASC")

	// Make query and check for returned number of rows
	stats, err = s.queriers.stat(r.Context(), s.db, q, s.logger)
	if stats == nil && err != nil {
		s.logger.Error("Failed to fetch current quick stats", "users", strings.Join(users, ","), "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)

	projectsResponse := Response[models.Stat]{
		Status: "success",
		Data:   stats,
	}
	if err != nil {
		projectsResponse.Warnings = append(projectsResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// GET /stats/global
// Get global usage statistics.
func (s *CEEMSServer) globalStats(users []string, w http.ResponseWriter, r *http.Request) {
	var stats []models.Stat

	var q Query

	var err error

	// Set write deadline
	s.setWriteDeadline(1*time.Minute, w)

	// Make query
	q = Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s WHERE 1=1", statsQuery, base.UnitsDBTableName))

	// Get cluster_id query parameters if any
	if clusterIDs := r.URL.Query()["cluster_id"]; len(clusterIDs) > 0 {
		q.query(" AND cluster_id IN ")
		q.param(clusterIDs)
	}

	// Finally add GROUP BY clause. Always group by cluster_id
	q.query(" GROUP BY cluster_id")

	// Sort by cluster_id, username and project
	q.query(" ORDER BY cluster_id ASC")

	// Make query and check for returned number of rows
	stats, err = s.queriers.stat(r.Context(), s.db, q, s.logger)
	if stats == nil && err != nil {
		s.logger.Error("Failed to fetch global quick stats", "users", strings.Join(users, ","), "err", err)
		errorResponse[any](w, &apiError{errorInternal, err}, s.logger, nil)

		return
	}

	// Write response
	w.WriteHeader(http.StatusOK)

	projectsResponse := Response[models.Stat]{
		Status: "success",
		Data:   stats,
	}
	if err != nil {
		projectsResponse.Warnings = append(projectsResponse.Warnings, err.Error())
	}

	if err = json.NewEncoder(w).Encode(&projectsResponse); err != nil {
		s.logger.Error("Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}

// usage         godoc
//
//	@Summary		Admin Stats
//	@Description	This admin endpoint will return the quick stats of _queried_ cluster. The
//	@Description	current user is always identified by the header `X-Grafana-User` in
//	@Description	the request.
//	@Description
//	@Description	The user who is making the request must be in the list of admin users
//	@Description	configured for the server.
//	@Description
//	@Description	A path parameter `mode` is required to return the kind of usage statistics.
//	@Description	Currently, two modes of statistics are supported:
//	@Description	- `current`: In this mode the usage between two time periods is returned
//	@Description	based on `from` and `to` query parameters.
//	@Description	- `global`: In this mode the _total_ usage statistics are returned. For
//	@Description	instance, if the retention period of the DB is set to 2 years, usage
//	@Description	statistics of last 2 years will be returned.
//	@Description
//	@Description	The statistics include current number of active users, projects, jobs, _etc_.
//	@Description
//	@Description	If `to` query parameter is not provided, current time will be used. If `from`
//	@Description	query parameter is not used, a default query window of 24 hours will be used.
//	@Description	It means if `to` is provided, `from` will be calculated as `to` - 24hrs.
//	@Description
//	@Security	BasicAuth
//	@Tags		stats
//	@Produce	json
//	@Param		X-Grafana-User	header		string		true	"Current user name"
//	@Param		mode			path		string		true	"Whether to get quick stats within a period or global"	Enums(current, global)
//	@Param		cluster_id		query		[]string	false	"cluster ID"											collectionFormat(multi)
//	@Param		from			query		string		false	"From timestamp"
//	@Param		to				query		string		false	"To timestamp"
//	@Success	200				{object}	Response[models.Stat]
//	@Failure	401				{object}	Response[any]
//	@Failure	403				{object}	Response[any]
//	@Failure	500				{object}	Response[any]
//	@Router		/stats/{mode}/admin [get]
//
// GET /stats/{mode}/admin
// Get current/global stats.
func (s *CEEMSServer) statsAdmin(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "stats admin endpoint", s.logger)

	// Set headers
	s.setHeaders(w)

	// Get path parameter type
	var mode string

	var exists bool
	if mode, exists = mux.Vars(r)["mode"]; !exists {
		errorResponse[any](w, &apiError{errorBadData, errInvalidRequest}, s.logger, nil)

		return
	}

	// handle current usage query
	if mode == currentUsage {
		s.currentStats(r.URL.Query()["user"], w, r)
	}

	// handle global usage query
	if mode == globalUsage {
		s.globalStats(r.URL.Query()["user"], w, r)
	}
}

// demo         godoc
//
//	@Summary		Demo Units/Usage endpoints
//	@Description	This endpoint returns sample response for units and usage models. This
//	@Description	endpoint do not require the setting of `X-Grafana-User` header as it
//	@Description	only returns mock data for each request. This can be used to introspect
//	@Description	the response models for different resources.
//	@Description
//	@Description	The endpoint requires a path parameter `resource` which takes either:
//	@Description	- `units` which returns a mock units response
//	@Description	- `usage` which returns a mock usage response.
//	@Description
//	@Description	The mock data is generated randomly for each request and there is
//	@Description	no guarantee that the data has logical sense.
//	@Tags			demo
//	@Produce		json
//	@Param			resource	path		string	true	"Whether to return mock units or usage data"	Enums(units, usage)
//	@Success		200			{object}	Response[models.Unit]
//	@Success		200			{object}	Response[models.Usage]
//	@Failure		500			{object}	Response[any]
//	@Router			/demo/{resource} [get]
//
// GET /demo/{units,usage}
// Return mocked data for different models.
func (s *CEEMSServer) demo(w http.ResponseWriter, r *http.Request) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "demo endpoint", s.logger)

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
			s.logger.Error("Failed to encode response", "err", err)
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
			s.logger.Error("Failed to encode response", "err", err)
			w.Write([]byte("KO"))
		}
	}
}

// convertTimeLocation converts time from source location to target location.
func convertTimeLocation(sourceLoc *time.Location, targetLoc *time.Location, val string) string {
	if t, err := time.ParseInLocation(base.DatetimezoneLayout, val, sourceLoc); err == nil {
		return t.In(targetLoc).Format(base.DatetimezoneLayout)
	}

	return val
}
