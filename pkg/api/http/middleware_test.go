package http

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
)

func mockAdminUsers(db *sql.DB, logger log.Logger) []string {
	return []string{"adm1"}
}

func setupMiddleware() http.Handler {
	// Create an instance of middleware
	amw := authenticationMiddleware{
		logger:          log.NewNopLogger(),
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
	req := httptest.NewRequest("GET", "/api/v1/units", nil)
	req.Header.Set(grafanaUserHeader, "usr1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)
	resp := w.Result()
	defer resp.Body.Close()

	// Should pass test
	assert.Equal(t, resp.StatusCode, 200)

	// Check headers added to req
	assert.Equal(t, req.Header.Get(loggedUserHeader), "usr1")
	assert.Equal(t, req.Header.Get(dashboardUserHeader), "usr1")
}

func TestMiddlewareFailure(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest("GET", "/api/v1/units", nil)

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	// Should pass test
	assert.Equal(t, w.Result().StatusCode, 401)
}

func TestMiddlewareAdminSuccess(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest("GET", "/api/v1/units/admin", nil)
	req.Header.Set(grafanaUserHeader, "adm1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	// Should pass test
	assert.Equal(t, w.Result().StatusCode, 200)

	// Check headers added to req
	assert.Equal(t, req.Header.Get(adminUserHeader), "adm1")
}

func TestMiddlewareAdminFailure(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest("GET", "/api/v1/units/admin", nil)
	req.Header.Set(grafanaUserHeader, "usr1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	// Should pass test
	assert.Equal(t, w.Result().StatusCode, 403)
}

func TestMiddlewareAdminFailurePresetHeader(t *testing.T) {
	// Setup middleware handler
	handlerToTest := setupMiddleware()

	// create a mock request to use
	req := httptest.NewRequest("GET", "/api/v1/units/admin", nil)
	req.Header.Set(grafanaUserHeader, "usr1")
	req.Header.Set(adminUserHeader, "usr1")

	// call the handler using a mock response recorder (we'll not use that anyway)
	w := httptest.NewRecorder()
	handlerToTest.ServeHTTP(w, req)

	// Should pass test
	assert.Equal(t, w.Result().StatusCode, 403)

	// Should not contain adminHeader
	assert.Equal(t, req.Header.Get(adminUserHeader), "")
}
