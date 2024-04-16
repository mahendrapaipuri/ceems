package frontend

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	http_api "github.com/mahendrapaipuri/ceems/pkg/api/http"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
)

// Headers
const (
	grafanaUserHeader   = "X-Grafana-User"
	dashboardUserHeader = "X-Dashboard-User"
	loggedUserHeader    = "X-Logged-User"
	adminUserHeader     = "X-Admin-User"
)

var (
	// Regex that will match job's UUIDs
	// Dont use greedy matching to avoid capturing gpuuuid label
	// Use strict UUID allowable character set. They can be only letters, digits and hypen (-)
	// Playground: https://goplay.tools/snippet/kq_r_1SOgnG
	regexpUUID = regexp.MustCompile("(?:.+?)[^gpu]uuid=[~]{0,1}\"(?P<uuid>[a-zA-Z0-9-|]+)\"(?:.*)")
)

// Define our struct
type authenticationMiddleware struct {
	logger           log.Logger
	db               *sql.DB
	adminUsers       []string
	grafana          *grafana.Grafana
	grafanaTeamID    string
	lastAdminsUpdate time.Time
}

// Check UUIDs in query belong to user or not. This check is less invasive.
// Any error will mark the check as pass and request will be proxied to backend
func (amw *authenticationMiddleware) isUserUnit(user string, uuids []string) bool {
	// If there is no active DB conn or if uuids is empty, return
	if amw.db == nil || len(uuids) == 0 {
		return true
	}

	level.Debug(amw.logger).
		Log("msg", "UUIDs in query", "user", user, "queried_uuids", strings.Join(uuids, ","))

	// First get a list of projects that user is part of
	rows, err := amw.db.Query("SELECT DISTINCT project FROM usage WHERE usr = ?", user)
	if err != nil {
		level.Warn(amw.logger).
			Log("msg", "Failed to get user projects. Allowing query", "user", user,
				"queried_uuids", strings.Join(uuids, ","), "err", err,
			)
		return false
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
		level.Warn(amw.logger).
			Log("msg", "No user projects found. Query unauthorized", "user", user,
				"queried_uuids", strings.Join(uuids, ","), "err", err,
			)
		return false
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
	rows, err = amw.db.Query(query, queryData...)
	if err != nil {
		level.Warn(amw.logger).
			Log("msg", "Failed to check uuid ownership. Query unauthorized", "user", user, "query", query,
				"user_projects", strings.Join(projects, ","), "queried_uuids", strings.Join(uuids, ","),
				"err", err,
			)
		return false
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
		level.Debug(amw.logger).
			Log("msg", "Unauthorized query", "user", user, "user_projects", strings.Join(projects, ","),
				"queried_uuids", len(uuids), "found_uuids", uuidCount,
			)
		return false
	}
	return true
}

// Update admin users from Grafana teams
func (amw *authenticationMiddleware) updateAdminUsers() {
	if amw.lastAdminsUpdate.IsZero() || time.Since(amw.lastAdminsUpdate) > time.Duration(time.Hour) {
		adminUsers, err := amw.grafana.TeamMembers(amw.grafanaTeamID)
		if err != nil {
			level.Warn(amw.logger).Log("msg", "Failed to sync admin users from Grafana Teams", "err", err)
			return
		}

		// Get list of all usernames and add them to admin users after removing duplicates
		latesAdminUsers := append(amw.adminUsers, adminUsers...)
		slices.Sort(latesAdminUsers) // First sort the slice and then compact
		amw.adminUsers = slices.Compact(latesAdminUsers)
		level.Debug(amw.logger).
			Log("msg", "Admin users updated from Grafana teams API", "admins", strings.Join(amw.adminUsers, ","))

		// Update the lastAdminsUpdate time
		amw.lastAdminsUpdate = time.Now()
	}
}

// Middleware function, which will be called for each request
func (amw *authenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loggedUser string
		var newReq *http.Request
		var queryParams interface{}

		// First update admin users
		if amw.grafana.Available() {
			amw.updateAdminUsers()
		}

		// Remove any X-Admin-User header or X-Logged-User if passed
		r.Header.Del(adminUserHeader)
		r.Header.Del(loggedUserHeader)

		// Check if username header is available
		loggedUser = r.Header.Get(grafanaUserHeader)
		if loggedUser == "" {
			level.Error(amw.logger).
				Log("msg", "Grafana user Header not found. Denying authentication")

			// Write an error and stop the handler chain
			w.WriteHeader(http.StatusUnauthorized)
			response := http_api.Response{
				Status:    "error",
				ErrorType: "unauthorized",
				Error:     "no user header found",
			}
			if err := json.NewEncoder(w).Encode(&response); err != nil {
				level.Error(amw.logger).Log("msg", "Failed to encode response", "err", err)
				w.Write([]byte("KO"))
			}
			return
		}
		level.Debug(amw.logger).Log("loggedUser", loggedUser, "url", r.URL)

		// Set logged user header
		r.Header.Set(loggedUserHeader, loggedUser)

		// Clone request, parse query params and set them in request context
		newReq = parseQueryParams(r, amw.logger)

		// If current user is in list of admin users, dont do any checks
		if slices.Contains(amw.adminUsers, loggedUser) {
			goto end
		}

		// Retrieve query params from context
		queryParams = newReq.Context().Value(QueryParamsContextKey{})

		// If no query params found, pass request
		if queryParams == nil {
			goto end
		}

		// Check if user is querying for his/her own compute units by looking to DB
		if !amw.isUserUnit(loggedUser, queryParams.(*QueryParams).uuids) {
			// Write an error and stop the handler chain
			w.WriteHeader(http.StatusUnauthorized)
			response := http_api.Response{
				Status:    "error",
				ErrorType: "unauthorized",
				Error:     "user do not have permissions to view unit metrics",
			}
			if err := json.NewEncoder(w).Encode(&response); err != nil {
				level.Error(amw.logger).Log("msg", "Failed to encode response", "err", err)
				w.Write([]byte("KO"))
			}
			return
		}

	end:
		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, newReq)
	})
}
