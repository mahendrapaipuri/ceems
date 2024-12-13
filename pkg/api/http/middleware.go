//go:build cgo
// +build cgo

package http

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Headers.
const (
	grafanaUserHeader   = "X-Grafana-User"
	dashboardUserHeader = "X-Dashboard-User"
	loggedUserHeader    = "X-Logged-User"
	adminUserHeader     = "X-Admin-User"
	ceemsUserHeader     = "X-Ceems-User" // Special header that will be included in requests from CEEMS LB
)

// Debug end point regex match.
var (
	debugEndpoints = regexp.MustCompile("/debug/(.*)")
)

// Define our struct.
type authenticationMiddleware struct {
	logger          *slog.Logger
	routerPrefix    string
	whitelistedURLs *regexp.Regexp
	db              *sql.DB
	adminUsers      func(context.Context, *sql.DB, *slog.Logger) []string
}

// Middleware function, which will be called for each request.
func (amw *authenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loggedUser string

		var admUsers []string

		var q url.Values

		// If requested URI is one of the following, skip checking for user header
		//  - Root document
		//  - /health endpoint
		//  - /demo/* endpoint
		//  - /swagger/* endpoints
		//  - /debug/* endpoints
		//
		// NOTE that we only skip checking X-Grafana-User header. In prod when
		// basic auth is enabled, all these end points are under auth and hence an
		// unautorised user cannot access these end points
		if r.URL.Path == "/" ||
			r.URL.Path == amw.routerPrefix ||
			amw.whitelistedURLs.MatchString(r.URL.Path) ||
			debugEndpoints.MatchString(r.URL.Path) {
			goto end
		}

		// If request has "special" CEEMS header, pass through. It must be
		// coming from other CEEMS components
		if _, ok := r.Header[ceemsUserHeader]; ok {
			goto end
		}

		// Remove any X-Admin-User header or X-Logged-User if passed
		r.Header.Del(adminUserHeader)
		r.Header.Del(loggedUserHeader)

		// Check if username header is available
		loggedUser = r.Header.Get(grafanaUserHeader)
		if loggedUser == "" {
			amw.logger.Error("Grafana user Header not found. Denying authentication")

			// Write an error and stop the handler chain
			errorResponse[any](w, &apiError{errorUnauthorized, errNoUser}, amw.logger, nil)

			return
		}

		amw.logger.Info("middleware", "loggedUser", loggedUser, "url", r.URL)

		// Set logged user header
		r.Header.Set(loggedUserHeader, loggedUser)

		// Set user in URL query as well as we will use it as key for caching
		q = r.URL.Query()
		q.Add("logged_user", loggedUser)
		r.URL.RawQuery = q.Encode()

		// Fetch admin users from DB
		admUsers = amw.adminUsers(r.Context(), amw.db, amw.logger)

		// If current user is in list of admin users, get "actual" user from
		// X-Dashboard-User header. For normal users, this header will be exactly same
		// as their username.
		// For admin users who can look at dashboard of "any" user this will be the
		// username of the "impersonated" user and we take it into account
		if slices.Contains(admUsers, loggedUser) {
			// Set X-Admin-User header
			r.Header.Set(adminUserHeader, loggedUser)

			if dashboardUser := r.Header.Get(dashboardUserHeader); dashboardUser != "" {
				amw.logger.Info(
					"Admin user accessing dashboards", "loggedUser", loggedUser,
					"dashboardUser", dashboardUser, "url", r.URL,
				)
			} else {
				r.Header.Set(dashboardUserHeader, loggedUser)
			}
		} else {
			// Check if requested URI is not admin endpoints
			if strings.HasSuffix(r.URL.Path, "admin") {
				amw.logger.Error("Unprivileged user accessing admin endpoint", "user", loggedUser, "url", r.URL)

				// Write an error and stop the handler chain
				errorResponse[any](w, &apiError{errorForbidden, errNoPrivs}, amw.logger, nil)

				return
			}

			r.Header.Set(dashboardUserHeader, loggedUser)
		}

	end:
		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, r)
	})
}
