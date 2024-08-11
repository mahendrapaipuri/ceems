package frontend

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	ceems_api "github.com/mahendrapaipuri/ceems/pkg/api/http"
)

// Headers.
const (
	grafanaUserHeader   = "X-Grafana-User"
	dashboardUserHeader = "X-Dashboard-User"
	loggedUserHeader    = "X-Logged-User"
	adminUserHeader     = "X-Admin-User"
	ceemsUserHeader     = "X-Ceems-User"
)

var (
	// Regex that will match unit's UUIDs
	// Dont use greedy matching to avoid capturing gpuuuid label
	// Use strict UUID allowable character set. They can be only letters, digits and hypen (-)
	// Playground: https://goplay.tools/snippet/kq_r_1SOgnG
	regexpUUID = regexp.MustCompile("(?:.+?)[^gpu]uuid=[~]{0,1}\"(?P<uuid>[a-zA-Z0-9-|]+)\"(?:.*)")

	// Regex that will match unit's ID.
	regexID = regexp.MustCompile("(?:.+?)ceems_id=[~]{0,1}\"(?P<id>[a-zA-Z0-9-|_]+)\"(?:.*)")
)

// ceems is the struct container for CEEMS API server.
type ceems struct {
	db     *sql.DB
	webURL *url.URL
	client *http.Client
}

func (c *ceems) verifyEndpoint() *url.URL {
	if c.webURL != nil {
		return c.webURL.JoinPath("/api/v1/units/verify")
	}

	return nil
}

func (c *ceems) clustersEndpoint() *url.URL {
	if c.webURL != nil {
		return c.webURL.JoinPath("/api/v1/clusters/admin")
	}

	return nil
}

// authenticationMiddleware implements the auth middleware for LB.
type authenticationMiddleware struct {
	logger     log.Logger
	ceems      ceems
	clusterIDs []string
}

// Check UUIDs in query belong to user or not.
func (amw *authenticationMiddleware) isUserUnit(
	ctx context.Context,
	user string,
	clusterIDs []string,
	uuids []string,
) bool {
	// Always prefer checking with DB connection directly if it is available
	// As DB query is way more faster than HTTP API request
	if amw.ceems.db != nil {
		return ceems_api.VerifyOwnership(ctx, user, clusterIDs, uuids, amw.ceems.db, amw.logger)
	}

	// If CEEMS URL is available make a API request
	// Any errors in making HTTP request will fail the query. This can happen due
	// to deployment issues and by failing queries we make operators to look into
	// what is happening
	if amw.ceems.verifyEndpoint() != nil {
		// Create a new POST request
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, amw.ceems.verifyEndpoint().String(), nil)
		if err != nil {
			level.Debug(amw.logger).
				Log("msg", "Failed to create new request for unit ownership verification",
					"user", user, "queried_uuids", strings.Join(uuids, ","), "err", err)

			return false
		}

		// Add uuids to request
		req.URL.RawQuery = url.Values{"uuid": uuids, "cluster_id": clusterIDs}.Encode()

		// Add necessary headers
		req.Header.Add(grafanaUserHeader, user)

		// Make request
		// If request failed, forbid the query. It can happen when CEEMS API server
		// goes offline and we should wait for it to come back online
		if resp, err := amw.ceems.client.Do(req); err != nil {
			level.Debug(amw.logger).
				Log("msg", "Failed to make request for unit ownership verification",
					"user", user, "queried_uuids", strings.Join(uuids, ","), "err", err)

			return false
		} else if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			level.Debug(amw.logger).Log("msg", "Unauthorised query", "user", user,
				"queried_uuids", strings.Join(uuids, ","), "status_code", resp.StatusCode)

			return false
		}
	}

	return true
}

// Middleware function, which will be called for each request.
func (amw *authenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loggedUser string

		var id string

		var uuids []string

		var queryParams interface{}

		// Clone request, parse query params and set them in request context
		// This will ensure we set query params in request's context always
		r = parseQueryParams(r, amw.clusterIDs, amw.logger)

		// Apply middleware only for query and query_range API calls
		if !strings.HasSuffix(r.URL.Path, "query") && !strings.HasSuffix(r.URL.Path, "query_range") {
			goto end
		}

		// If ceems url or db is not configured, pass through. There is nothing
		// to check here
		if amw.ceems.webURL == nil && amw.ceems.db == nil {
			goto end
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

			response := ceems_api.Response[any]{
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

		// Retrieve query params from context
		queryParams = r.Context().Value(QueryParamsContextKey{})

		// If no query params found, return bad request
		if queryParams == nil {
			w.WriteHeader(http.StatusBadRequest)

			response := ceems_api.Response[any]{
				Status:    "error",
				ErrorType: "bad_request",
				Error:     "query could not be parsed",
			}
			if err := json.NewEncoder(w).Encode(&response); err != nil {
				level.Error(amw.logger).Log("msg", "Failed to encode response", "err", err)
				w.Write([]byte("KO"))
			}

			return
		}

		// Check type assertions
		if v, ok := queryParams.(*QueryParams); ok {
			id = v.id
			uuids = v.uuids
		} else {
			response := ceems_api.Response[any]{
				Status:    "error",
				ErrorType: "bad_request",
				Error:     "invalid query",
			}
			if err := json.NewEncoder(w).Encode(&response); err != nil {
				level.Error(amw.logger).Log("msg", "Failed to encode response", "err", err)
				w.Write([]byte("KO"))
			}

			return
		}

		// Check if user is querying for his/her own compute units by looking to DB
		if !amw.isUserUnit(r.Context(), loggedUser, []string{id}, uuids) { //nolint:contextcheck // False positive
			// Write an error and stop the handler chain
			w.WriteHeader(http.StatusForbidden)

			response := ceems_api.Response[any]{
				Status:    "error",
				ErrorType: "forbidden",
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
		next.ServeHTTP(w, r)
	})
}
