package frontend

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
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

// CEEMS API server struct
type ceems struct {
	db     *sql.DB
	webURL *url.URL
	client *http.Client
}

func (c *ceems) verifyEndpoint() *url.URL {
	if c.webURL != nil {
		return c.webURL.JoinPath("/api/units/verify")
	}
	return nil
}

// Define our struct
type authenticationMiddleware struct {
	logger           log.Logger
	ceems            ceems
	adminUsers       []string
	grafana          *grafana.Grafana
	grafanaTeamID    string
	lastAdminsUpdate time.Time
}

// Check UUIDs in query belong to user or not
func (amw *authenticationMiddleware) isUserUnit(user string, uuids []string) bool {
	// Always prefer checking with DB connection directly if it is available
	// As DB query is way more faster than HTTP API request
	if amw.ceems.db != nil {
		return http_api.VerifyOwnership(user, uuids, amw.ceems.db, amw.logger)
	}

	// If CEEMS URL is available make a API request
	// Any errors in making HTTP request will fail the query. This can happen due
	// to deployment issues and by failing queries we make operators to look into
	// what is happening
	if amw.ceems.verifyEndpoint() != nil {
		// Create a new POST request
		req, err := http.NewRequest(http.MethodGet, amw.ceems.verifyEndpoint().String(), nil)
		if err != nil {
			level.Debug(amw.logger).
				Log("msg", "Failed to create new request for unit ownership verification", "user", user, "queried_uuids", strings.Join(uuids, ","), "err", err)
			return false
		}

		// Add uuids to request
		req.URL.RawQuery = url.Values{"uuid": uuids}.Encode()

		// Add necessary headers
		req.Header.Add(grafanaUserHeader, user)

		// Make request
		// If request failed, forbid the query. It can happen when CEEMS API server
		// goes offline and we should wait for it to come back online
		if resp, err := amw.ceems.client.Do(req); err != nil {
			level.Debug(amw.logger).
				Log("msg", "Failed to make request for unit ownership verification", "user", user, "queried_uuids", strings.Join(uuids, ","), "err", err)
			return false
		} else {
			// Any status code other than 200 should be treated as check failure
			if resp.StatusCode != 200 {
				level.Debug(amw.logger).Log("msg", "Unauthorised query", "user", user, "queried_uuids", strings.Join(uuids, ","), "status_code", resp.StatusCode)
				return false
			}
		}
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
