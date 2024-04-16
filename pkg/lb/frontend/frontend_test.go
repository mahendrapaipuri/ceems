package frontend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

func dummyTSDBServer(retention string) *httptest.Server {
	// Start test server
	expected := tsdb.Response{
		Status: "success",
		Data: map[string]string{
			"storageRetention": retention,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "runtimeinfo") {
			if err := json.NewEncoder(w).Encode(&expected); err != nil {
				w.Write([]byte("KO"))
			}
		} else {
			w.Write([]byte("dummy-response"))
		}
	}))
	return server
}

func TestNewFrontend(t *testing.T) {
	// Backends
	dummyServer1 := dummyTSDBServer("30d")
	defer dummyServer1.Close()
	backend1URL, err := url.Parse(dummyServer1.URL)
	if err != nil {
		t.Fatal(err)
	}

	rp1 := httputil.NewSingleHostReverseProxy(backend1URL)
	backend1 := backend.NewTSDBServer(backend1URL, rp1, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.NewManager("resource-based", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	manager.Add(backend1)

	// make minimal config
	config := &Config{
		Logger:  log.NewNopLogger(),
		Manager: manager,
	}

	// New load balancer
	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Errorf("failed to create load balancer: %s", err)
	}

	tests := []struct {
		name     string
		start    int64
		code     int
		response bool
	}{
		{
			name:     "query with params in ctx",
			start:    time.Now().UTC().Unix(),
			code:     200,
			response: true,
		},
		{
			name:     "query with no params in ctx",
			code:     200,
			response: true,
		},
		{
			name:     "query with params in ctx and start more than retention period",
			start:    time.Now().UTC().Add(-time.Duration(31 * 24 * time.Hour)).Unix(),
			code:     503,
			response: false,
		},
	}

	for _, test := range tests {
		request := httptest.NewRequest("GET", "/test", nil)

		// Add uuids and start to context
		var newReq *http.Request
		if test.start > 0 {
			period := time.Duration((time.Now().UTC().Unix() - test.start)) * time.Second
			newReq = request.WithContext(
				context.WithValue(
					request.Context(), QueryParamsContextKey{},
					&QueryParams{queryPeriod: period},
				),
			)
		} else {
			newReq = request
		}

		responseRecorder := httptest.NewRecorder()

		http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

		if responseRecorder.Code != test.code {
			t.Errorf("%s: expected status %d, got %d", test.name, test.code, responseRecorder.Code)
		}
		if test.response {
			if strings.TrimSpace(responseRecorder.Body.String()) != "dummy-response" {
				t.Errorf("%s: expected dummy-response, got %s", test.name, responseRecorder.Body)
			}
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)
	request := httptest.NewRequest("GET", "/test", nil)
	responseRecorder := httptest.NewRecorder()

	http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != 503 {
		t.Errorf("expected status 503, got %d", responseRecorder.Code)
	}
}
