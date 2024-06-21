package http

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/go-kit/log"
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
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 got %d", w.Result().StatusCode)
	}
	// Check headers added to req
	if req.Header.Get(loggedUserHeader) != "usr1" {
		t.Errorf("expected %s header to be adm1, got %s", loggedUserHeader, req.Header.Get(loggedUserHeader))
	}
	if req.Header.Get(dashboardUserHeader) != "usr1" {
		t.Errorf("expected %s header to be adm1, got %s", dashboardUserHeader, req.Header.Get(dashboardUserHeader))
	}
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
	if w.Result().StatusCode != 401 {
		t.Errorf("expected 401 got %d", w.Result().StatusCode)
	}
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
	if w.Result().StatusCode != 200 {
		t.Errorf("expected 200 got %d", w.Result().StatusCode)
	}
	// Check headers added to req
	if req.Header.Get(adminUserHeader) != "adm1" {
		t.Errorf("expected %s header to be adm1, got %s", adminUserHeader, req.Header.Get(adminUserHeader))
	}
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
	if w.Result().StatusCode != 403 {
		t.Errorf("expected 403 got %d", w.Result().StatusCode)
	}
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
	if w.Result().StatusCode != 403 {
		t.Errorf("expected 403 got %d", w.Result().StatusCode)
	}
	// Should not contain adminHeader
	if req.Header.Get(adminUserHeader) != "" {
		t.Errorf("expected no %s header got %s", adminUserHeader, req.Header.Get(adminUserHeader))
	}
}
