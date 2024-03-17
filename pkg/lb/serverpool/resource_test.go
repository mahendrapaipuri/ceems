package serverpool

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
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
			time.Sleep(3 * time.Second)
		}
	}))
	return server
}

func TestResourceBasedLB(t *testing.T) {
	// Create a manager
	manager, err := NewManager("resource-based", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Retention periods
	periods := []string{"30d", "180d or 100GiB", "180d or 100GiB"}
	backendURLs := make([]*url.URL, len(periods))
	backends := make([]backend.TSDBServer, len(periods))

	// Make backends
	for i, p := range periods {
		dummyServer := dummyTSDBServer(p)
		defer dummyServer.Close()
		backendURL, err := url.Parse(dummyServer.URL)
		if err != nil {
			t.Fatal(err)
		}
		backendURLs[i] = backendURL

		rp := httputil.NewSingleHostReverseProxy(backendURL)
		backend := backend.NewTSDBServer(backendURL, rp, log.NewNopLogger())
		manager.Add(backend)
		backends[i] = backend
	}

	// Manager size should be three
	if manager.Size() != len(periods) {
		t.Errorf("expected %d backend TSDB servers, got %d", len(periods), manager.Size())
	}

	// Start wait group
	var wg sync.WaitGroup

	// Target should be backend[0]
	target := manager.Target(time.Duration(10 * time.Hour))
	if target.URL() != backendURLs[0] {
		t.Errorf("expected a backend1, got %s", target.URL().String())
	}

	// Backend[1] is serving a long request
	wg.Add(1)
	go func() {
		defer wg.Done()
		backends[1].Serve(w, req)
	}()

	// Let the server serve request
	time.Sleep(1 * time.Second)

	// This request should be proxied to backend[2] as backend[1] is busy with request
	target = manager.Target(time.Duration(100 * 24 * time.Hour))
	if target.URL().String() != backendURLs[2].String() {
		t.Errorf("expected backend3, got %s", target.URL().String())
	}

	// Check if backend[1] has one active connection
	connTwo := backends[1].ActiveConnections()
	if connTwo != 1 {
		t.Errorf("expected 1 connections for backends[1], got %d", connTwo)
	}

	// Just to check for any race conditions
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			manager.Target(time.Duration(10 * time.Hour))
		}
	}()

	// Wait for all go routines
	wg.Wait()

	// Make all backends offline
	for _, backend := range backends {
		backend.SetAlive(false)
	}
	// Should return nil target
	if target = manager.Target(time.Duration(100 * 24 * time.Hour)); target != nil {
		t.Errorf("expected nil target, got %s", target.URL().String())
	}
}
