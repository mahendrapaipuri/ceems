package http

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
)

// Headers
const (
	grafanaUserHeader   = "X-Grafana-User"
	dashboardUserHeader = "X-Dashboard-User"
	loggedUserHeader    = "X-Logged-User"
	adminUserHeader     = "X-Admin-User"
)

// Define our struct
type authenticationMiddleware struct {
	logger           log.Logger
	adminUsers       []string
	grafana          *grafana.Grafana
	lastAdminsUpdate time.Time
}

// Update admin users from Grafana teams
func (amw *authenticationMiddleware) updateAdminUsers() {
	if amw.lastAdminsUpdate.IsZero() || time.Since(amw.lastAdminsUpdate) > time.Duration(time.Hour) {
		adminUsers, err := amw.grafana.TeamMembers(base.GrafanaAdminTeamID)
		if err != nil {
			level.Warn(amw.logger).Log("msg", "Failed to sync admin users from Grafana Teams", "err", err)
			return
		}

		// Get list of all usernames and add them to admin users
		amw.adminUsers = append(amw.adminUsers, adminUsers...)
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

		// If requested URI is health or root "/" pass through
		if strings.HasSuffix(r.URL.Path, "health") || r.URL.Path == "/" {
			goto end
		}

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
			errorResponse(w, &apiError{errorUnauthorized, errNoUser}, amw.logger, nil)
			return
		}
		level.Info(amw.logger).Log("loggedUser", loggedUser, "url", r.URL)

		// Set logged user header
		r.Header.Set(loggedUserHeader, loggedUser)

		// If current user is in list of admin users, get "actual" user from
		// X-Dashboard-User header. For normal users, this header will be exactly same
		// as their username.
		// For admin users who can look at dashboard of "any" user this will be the
		// username of the "impersonated" user and we take it into account
		if slices.Contains(amw.adminUsers, loggedUser) {
			// Set X-Admin-User header
			r.Header.Set(adminUserHeader, loggedUser)

			if dashboardUser := r.Header.Get(dashboardUserHeader); dashboardUser != "" {
				level.Info(amw.logger).Log(
					"msg", "Admin user accessing dashboards", "loggedUser", loggedUser,
					"dashboardUser", dashboardUser, "url", r.URL,
				)
			} else {
				r.Header.Set(dashboardUserHeader, loggedUser)
			}
		} else {
			// Check if requested URI is not admin endpoints
			if strings.HasSuffix(r.URL.Path, "admin") {
				level.Error(amw.logger).Log("msg", "Unprivileged user accessing admin endpoint", "user", loggedUser, "url", r.URL)

				// Write an error and stop the handler chain
				errorResponse(w, &apiError{errorUnauthorized, errNoPrivs}, amw.logger, nil)
				return
			}
			r.Header.Set(dashboardUserHeader, loggedUser)
		}

	end:
		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, r)
	})
}
