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

func dummyTSDBServer(retention string, rmID string) *httptest.Server {
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
			w.Write([]byte(rmID))
		}
	}))
	return server
}

func TestNewFrontendSingleGroup(t *testing.T) {
	rmID := "default"

	// Backends
	dummyServer1 := dummyTSDBServer("30d", rmID)
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
	manager.Add(rmID, backend1)

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
			code:     400,
			response: false,
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
					&QueryParams{queryPeriod: period, id: rmID},
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
			if strings.TrimSpace(responseRecorder.Body.String()) != rmID {
				t.Errorf("%s: expected dummy-response, got %s", test.name, responseRecorder.Body)
			}
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)
	request := httptest.NewRequest("GET", "/test", nil)
	newReq := request.WithContext(
		context.WithValue(
			request.Context(), QueryParamsContextKey{},
			&QueryParams{id: "default"},
		),
	)
	responseRecorder := httptest.NewRecorder()

	http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

	if responseRecorder.Code != 503 {
		t.Errorf("expected status 503, got %d", responseRecorder.Code)
	}
}

func TestNewFrontendTwoGroups(t *testing.T) {
	// Backends for group 1
	dummyServer1 := dummyTSDBServer("30d", "rm-0")
	defer dummyServer1.Close()
	backend1URL, err := url.Parse(dummyServer1.URL)
	if err != nil {
		t.Fatal(err)
	}

	rp1 := httputil.NewSingleHostReverseProxy(backend1URL)
	backend1 := backend.NewTSDBServer(backend1URL, rp1, log.NewNopLogger())

	// Backends for group 2
	dummyServer2 := dummyTSDBServer("30d", "rm-1")
	defer dummyServer2.Close()
	backend2URL, err := url.Parse(dummyServer2.URL)
	if err != nil {
		t.Fatal(err)
	}

	rp2 := httputil.NewSingleHostReverseProxy(backend2URL)
	backend2 := backend.NewTSDBServer(backend2URL, rp2, log.NewNopLogger())

	// Start manager
	manager, err := serverpool.NewManager("resource-based", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	manager.Add("rm-0", backend1)
	manager.Add("rm-1", backend2)

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
		rmID     string
		code     int
		response bool
	}{
		{
			name:     "query for rm-0 with params in ctx",
			start:    time.Now().UTC().Unix(),
			rmID:     "rm-0",
			code:     200,
			response: true,
		},
		{
			name:     "query for rm-1 with params in ctx",
			start:    time.Now().UTC().Unix(),
			rmID:     "rm-1",
			code:     200,
			response: true,
		},
		{
			name:     "query with no rmID params in ctx",
			start:    time.Now().UTC().Unix(),
			code:     503,
			response: false,
		},
		{
			name:     "query with params in ctx and start more than retention period",
			start:    time.Now().UTC().Add(-time.Duration(31 * 24 * time.Hour)).Unix(),
			rmID:     "rm-0",
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
					&QueryParams{queryPeriod: period, id: test.rmID},
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
			if strings.TrimSpace(responseRecorder.Body.String()) != test.rmID {
				t.Errorf("%s: expected dummy-response, got %s", test.name, responseRecorder.Body)
			}
		}
	}

	// Take backend offline, we should expect 503
	backend1.SetAlive(false)
	request := httptest.NewRequest("GET", "/test", nil)
	newReq := request.WithContext(
		context.WithValue(
			request.Context(), QueryParamsContextKey{},
			&QueryParams{id: "rm-0"},
		),
	)
	responseRecorder := httptest.NewRecorder()

	http.HandlerFunc(lb.Serve).ServeHTTP(responseRecorder, newReq)

	if responseRecorder.Code != 503 {
		t.Errorf("expected status 503, got %d", responseRecorder.Code)
	}
}
