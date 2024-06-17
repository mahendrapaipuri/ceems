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

var (
	rbIDs = []string{"rb0", "rb1"}
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
	backendURLs := make(map[string][]*url.URL, len(rbIDs))
	backends := make(map[string][]backend.TSDBServer, len(rbIDs))

	// Make backends
	for i, p := range periods {
		for _, id := range rbIDs {
			dummyServer := dummyTSDBServer(p)
			defer dummyServer.Close()
			backendURL, err := url.Parse(dummyServer.URL)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, len(periods))
				backends[id] = make([]backend.TSDBServer, len(periods))
			}

			backendURLs[id][i] = backendURL

			rp := httputil.NewSingleHostReverseProxy(backendURL)
			backend := backend.NewTSDBServer(backendURL, rp, log.NewNopLogger())
			manager.Add(id, backend)
			backends[id][i] = backend
		}
	}

	// Manager size should be three
	for _, id := range rbIDs {
		if manager.Size(id) != len(periods) {
			t.Errorf("expected %d backend TSDB servers, got %d for %s", len(periods), manager.Size(id), id)
		}
	}

	// Start wait group
	var wg sync.WaitGroup

	// Target should be backend[0]
	for _, id := range rbIDs {
		target := manager.Target(id, time.Duration(10*time.Hour))
		if target.URL() != backendURLs[id][0] {
			t.Errorf("expected a backend1, got %s for %s", target.URL().String(), id)
		}
	}

	// Backend[1] is serving a long request
	for _, id := range rbIDs {
		wg.Add(1)
		go func(i string) {
			defer wg.Done()
			backends[i][1].Serve(w, req)
		}(id)
	}

	// Let the server serve request
	time.Sleep(1 * time.Second)

	// This request should be proxied to backend[2] as backend[1] is busy with request
	for _, id := range rbIDs {
		target := manager.Target(id, time.Duration(100*24*time.Hour))
		if target.URL().String() != backendURLs[id][2].String() {
			t.Errorf("expected backend3, got %s for %s", target.URL().String(), id)
		}
	}

	// Check if backend[1] has one active connection
	for _, id := range rbIDs {
		connTwo := backends[id][1].ActiveConnections()
		if connTwo != 1 {
			t.Errorf("expected 1 connections for backends[1], got %d for %s", connTwo, id)
		}
	}

	// Just to check for any race conditions
	for _, id := range rbIDs {
		wg.Add(1)
		go func(iid string) {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				manager.Target(iid, time.Duration(10*time.Hour))
			}
		}(id)
	}

	// Wait for all go routines
	wg.Wait()

	// Make all backends offline
	for _, id := range rbIDs {
		for _, backend := range backends[id] {
			backend.SetAlive(false)
		}
	}

	// Should return nil target
	for _, id := range rbIDs {
		if target := manager.Target(id, time.Duration(100*24*time.Hour)); target != nil {
			t.Errorf("expected nil target, got %s", target.URL().String())
		}
	}
}
