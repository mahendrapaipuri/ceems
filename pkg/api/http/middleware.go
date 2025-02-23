//go:build cgo
// +build cgo

package http

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
)

// Define our struct.
type authenticationMiddleware struct {
	logger          *slog.Logger
	whitelistedURLs *regexp.Regexp
	db              *sql.DB
	headers         []string
	adminUsers      func(context.Context, *sql.DB) ([]string, error)
}

// newAuthenticationMiddleware returns a new instance authenticationMiddleware.
func newAuthenticationMiddleware(routePrefix string, headers []string, db *sql.DB, logger *slog.Logger) (*authenticationMiddleware, error) {
	// Playground: https://regex101.com/r/lkmsWz/3
	urlsRegex, err := regexp.Compile(
		fmt.Sprintf("^(?:(%s)?)/(swagger|debug|health|demo)(.*)", strings.TrimSuffix(routePrefix, "/")),
	)
	if err != nil {
		return nil, err
	}

	// Check if atleast one header name is configured.
	if len(headers) == 0 {
		return nil, errors.New("no header names configured to fetch username")
	}

	return &authenticationMiddleware{
		logger:          logger,
		whitelistedURLs: urlsRegex,
		db:              db,
		headers:         headers,
		adminUsers:      AdminUserNames,
	}, nil
}

// Middleware function, which will be called for each request.
func (amw *authenticationMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var loggedUser string

		var admUsers []string

		var q url.Values

		var err error

		// Dont apply Middleware for preflight requests
		if r.Method == http.MethodOptions {
			goto end
		}

		// If requested URI is one of the following, skip checking for user header
		//  - /
		//  - /health endpoint
		//  - /demo/* endpoint
		//  - /swagger/* endpoints
		//  - /debug/* endpoints
		//
		// NOTE that we only skip checking X-Grafana-User header. In prod when
		// basic auth is enabled, all these end points are under auth and hence an
		// unautorised user cannot access these end points
		if r.URL.Path == "/" || amw.whitelistedURLs.MatchString(r.URL.Path) {
			goto end
		}

		// Remove any X-Ceems-Admin-User header or X-Ceems-Logged-User if passed
		r.Header.Del(base.AdminUserHeader)
		r.Header.Del(base.LoggedUserHeader)

		// Check if username header is available
		if loggedUser = amw.getAuthenticatedUserFromHeader(r); loggedUser == "" {
			amw.logger.Error("User Header not found. Denying authentication")

			// Write an error and stop the handler chain
			errorResponse[any](w, &apiError{errorUnauthorized, errNoUser}, amw.logger, nil)

			return
		}

		amw.logger.Info("middleware", "logged_user", loggedUser, "url", r.URL)

		// Set logged user header
		r.Header.Set(base.LoggedUserHeader, loggedUser)

		// Set user in URL query as well as we will use it as key for caching
		q = r.URL.Query()
		q.Add("logged_user", loggedUser)
		r.URL.RawQuery = q.Encode()

		// Fetch admin users from DB
		admUsers, err = amw.adminUsers(r.Context(), amw.db)
		if err != nil {
			amw.logger.Error("Failed to fetch admin users", "logged_user", loggedUser, "url", r.URL, "err", err)
		}

		// If current user is not in the list of admin users, do access control of resource.
		if slices.Contains(admUsers, loggedUser) {
			// Set X-Ceems-Admin-User header
			r.Header.Set(base.AdminUserHeader, loggedUser)
		} else {
			// Check if requested URI is not admin endpoints
			if strings.HasSuffix(r.URL.Path, "admin") {
				amw.logger.Error("Unprivileged user accessing admin resource", "logger_user", loggedUser, "url", r.URL)

				// Write an error and stop the handler chain
				errorResponse[any](w, &apiError{errorForbidden, errNoPrivs}, amw.logger, nil)

				return
			}
		}

	end:
		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, r)
	})
}

// getAuthenticatedUserFromHeader looks different headers for the presence of value
// and returns the first found value.
func (amw *authenticationMiddleware) getAuthenticatedUserFromHeader(r *http.Request) string {
	// Check for the presence of header and return the first value found
	for _, header := range amw.headers {
		if value := r.Header.Get(header); value != "" {
			return value
		}
	}

	return ""
}
