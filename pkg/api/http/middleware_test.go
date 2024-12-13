//go:build cgo
// +build cgo

package http

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mockAdminUsers(_ context.Context, _ *sql.DB, _ *slog.Logger) []string {
	return []string{"adm1"}
}

func setupMiddleware() http.Handler {
	// Create an instance of middleware
	amw := authenticationMiddleware{
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		whitelistedURLs: regexp.MustCompile("/api/v1/(swagger|debug|health|demo)(.*)"),
		adminUsers:      mockAdminUsers,
	}

	// create a handler to use as "next" which will verify the request
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// create the handler to test, using our custom "next" handler
	return amw.Middleware(nextHandler)
}

func TestMiddlewareSuccess(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)
	req.Header.Set(grafanaUserHeader, "usr1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	// Should pass test
	assert.Equal(t, 200, res.StatusCode)

	// Check headers added to req
	assert.Equal(t, "usr1", req.Header.Get(loggedUserHeader))
	assert.Equal(t, "usr1", req.Header.Get(dashboardUserHeader))
}

func TestMiddlewareFailure(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units", nil)

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	// Should pass test
	assert.Equal(t, 401, res.StatusCode)
}

func TestMiddlewareAdminSuccess(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units/admin", nil)
	req.Header.Set(grafanaUserHeader, "adm1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	// Should pass test
	assert.Equal(t, 200, res.StatusCode)

	// Check headers added to req
	assert.Equal(t, "adm1", req.Header.Get(adminUserHeader))
}

func TestMiddlewareAdminFailure(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units/admin", nil)
	req.Header.Set(grafanaUserHeader, "usr1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	// Should pass test
	assert.Equal(t, 403, res.StatusCode)
}

func TestMiddlewareAdminFailurePresetHeader(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest(http.MethodGet, "/api/v1/units/admin", nil)
	req.Header.Set(grafanaUserHeader, "usr1")
	req.Header.Set(adminUserHeader, "usr1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	// Should pass test
	assert.Equal(t, 403, res.StatusCode)

	// Should not contain adminHeader
	assert.Equal(t, "", req.Header.Get(adminUserHeader))
}
