//go:build cgo
// +build cgo

package http

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockAdminUsers(_ context.Context, _ *sql.DB) ([]string, error) {
	return []string{"adm1"}, nil
}

func setupMiddleware() (http.Handler, error) {
	amw, err := newAuthenticationMiddleware("/api/v1", []string{base.GrafanaUserHeader}, nil, noOpLogger)
	if err != nil {
		return nil, err
	}

	// Set mock instance of admin users
	amw.adminUsers = mockAdminUsers

	// create a handler to use as "next" which will verify the request
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// create the handler to test, using our custom "next" handler
	return amw.Middleware(nextHandler), nil
}

func TestNewMiddleware(t *testing.T) {
	// Valid route prefix
	_, err := newAuthenticationMiddleware("/api/v1", []string{base.GrafanaUserHeader}, nil, noOpLogger)
	require.NoError(t, err)

	// Invald route prefix
	_, err = newAuthenticationMiddleware("/(?!\\/)", []string{base.GrafanaUserHeader}, nil, noOpLogger)
	require.Error(t, err)

	// No user headers
	_, err = newAuthenticationMiddleware("/api/v1", nil, nil, noOpLogger)
	require.Error(t, err)
}

func TestMiddleware(t *testing.T) {
	// Define tests
	tests := []struct {
		name     string
		method   string
		endpoint string
		code     int
		header   bool
		admin    bool
	}{
		{
			name:     "user accessing normal resource",
			method:   http.MethodGet,
			endpoint: "/api/v1/units",
			code:     200,
			header:   true,
			admin:    false,
		},
		{
			name:     "user accessing normal resource without header",
			method:   http.MethodGet,
			endpoint: "/api/v1/units",
			code:     401,
			header:   false,
			admin:    false,
		},
		{
			name:     "user accessing admin resource",
			method:   http.MethodGet,
			endpoint: "/api/v1/units/admin",
			code:     http.StatusForbidden,
			header:   true,
			admin:    false,
		},
		{
			name:     "user accessing swagger resource",
			method:   http.MethodGet,
			endpoint: "/swagger/index.html",
			code:     200,
			header:   false,
			admin:    false,
		},
		{
			name:     "user accessing debug resource",
			method:   http.MethodGet,
			endpoint: "/debug/pprof",
			code:     200,
			header:   false,
			admin:    false,
		},
		{
			name:     "user accessing health resource",
			method:   http.MethodGet,
			endpoint: "/health",
			code:     200,
			header:   false,
			admin:    false,
		},
		{
			name:     "user making preflight request",
			method:   http.MethodOptions,
			endpoint: "/api/v1/units",
			code:     200,
			header:   false,
			admin:    false,
		},
		{
			name:     "admin user accessing normal resource",
			method:   http.MethodGet,
			endpoint: "/api/v1/units",
			code:     200,
			header:   true,
			admin:    true,
		},
		{
			name:     "admin user accessing normal resource without user header",
			method:   http.MethodGet,
			endpoint: "/api/v1/units",
			code:     401,
			header:   false,
			admin:    true,
		},
		{
			name:     "admin user accessing admin resource",
			method:   http.MethodGet,
			endpoint: "/api/v1/units/admin",
			code:     200,
			header:   true,
			admin:    true,
		},
		{
			name:     "admin user making preflight request",
			method:   http.MethodOptions,
			endpoint: "/api/v1/units",
			code:     200,
			header:   false,
			admin:    true,
		},
	}

	// Setup middleware
	handlerToTest, err := setupMiddleware()
	require.NoError(t, err)

	for _, test := range tests {
		// create a mock request to use
		req := httptest.NewRequest(test.method, test.endpoint, nil)

		userName := "usr1"
		if test.admin {
			userName = "adm1"
		}

		if test.header {
			req.Header.Set(base.GrafanaUserHeader, userName)
		}

		// call the handler using a mock response recorder (we'll not use that anyway)
		w := httptest.NewRecorder()
		handlerToTest.ServeHTTP(w, req)

		res := w.Result()
		defer res.Body.Close()

		// Check status
		assert.Equal(t, test.code, res.StatusCode, test.name)

		if test.header {
			assert.Equal(t, userName, req.Header.Get(base.LoggedUserHeader))
		}
	}
}
