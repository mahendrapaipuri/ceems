package serverpool

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

var (
	lcIDs = []string{"lc0", "lc1"}
)

func TestUnAvailableBackends(t *testing.T) {
	d := time.Duration(0 * time.Second)

	// Start manager
	manager, err := NewManager("least-connection", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Make dummy backend servers
	backendURLs := make(map[string][]*url.URL, 2)
	backends := make(map[string][]backend.TSDBServer, 2)
	for i := 0; i < 2; i++ {
		for j, id := range lcIDs {
			backendURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", 3333*(i+1)+j))
			if err != nil {
				t.Fatal(err)
			}

			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, 2)
				backends[id] = make([]backend.TSDBServer, 2)
			}

			backendURLs[id][i] = backendURL

			rp := httputil.NewSingleHostReverseProxy(backendURL)
			backend := backend.NewTSDBServer(backendURL, rp, log.NewNopLogger())
			backends[id][i] = backend
			manager.Add(id, backend)
		}
	}

	// Check manager size
	for _, id := range lcIDs {
		if manager.Size(id) != 2 {
			t.Errorf("expected 2 backend TSDB servers, got %d", manager.Size(id))
		}
	}

	// Set one backend to dead
	backends[lcIDs[0]][1].SetAlive(false)
	backends[lcIDs[1]][0].SetAlive(false)

	// Get target and it should be backend2
	for i, id := range lcIDs {
		if target := manager.Target(id, d); target.URL() != backendURLs[id][i] {
			t.Errorf("expected %s, got %s", backendURLs[id][i], target.URL().String())
		}
	}

	// Set other backend to dead as well
	backends[lcIDs[0]][0].SetAlive(false)
	backends[lcIDs[1]][1].SetAlive(false)

	// Get target and it should be nil
	for _, id := range lcIDs {
		if target := manager.Target(id, d); target != nil {
			t.Errorf("expected nil, got %s", target)
		}
	}
}

func TestLeastConnectionLB(t *testing.T) {
	d := time.Duration(0 * time.Second)

	// Start manager
	manager, err := NewManager("least-connection", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	backendURLs := make(map[string][]*url.URL, 2)
	backends := make(map[string][]backend.TSDBServer, 2)
	for i := 0; i < 2; i++ {
		for _, id := range lcIDs {
			dummyServer := httptest.NewServer(h)
			defer dummyServer.Close()
			backendURL, err := url.Parse(dummyServer.URL)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, 2)
				backends[id] = make([]backend.TSDBServer, 2)
			}
			backendURLs[id][i] = backendURL

			rp := httputil.NewSingleHostReverseProxy(backendURL)
			backend := backend.NewTSDBServer(backendURL, rp, log.NewNopLogger())
			backends[id][i] = backend
			manager.Add(id, backend)
		}
	}

	// Check manager size
	for _, id := range lcIDs {
		if manager.Size(id) != 2 {
			t.Errorf("expected 2 backend TSDB servers, got %d", manager.Size(id))
		}
	}

	// Start wait group
	var wg sync.WaitGroup
	wg.Add(len(lcIDs))

	// Check if we get non nil target
	var target = make(map[string]backend.TSDBServer)
	for _, id := range lcIDs {
		if target[id] = manager.Target(id, d); target[id] == nil {
			t.Errorf("expected a target, got nil for %s", id)
		}
	}

	// Serve a long request
	for _, id := range lcIDs {
		go func(i string) {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			if target := manager.Target(i, d); target != nil {
				target.Serve(w, r)
			}
		}(id)
	}

	// Let the server serve request
	time.Sleep(1 * time.Second)

	// Check new target is not nil
	for _, id := range lcIDs {
		if newTarget := manager.Target(id, d); newTarget == nil {
			t.Errorf("expected a new target, got nil for %s", id)
		} else {
			if connTwo := newTarget.ActiveConnections(); connTwo != 0 {
				t.Errorf("expected 0 connections for two, got %d for %s", connTwo, id)
			}

			// New target must not be old one
			if target[id] == newTarget {
				t.Errorf("expected target and newTarget to be different for %s", id)
			}
		}
	}

	// For unknown ID expect nil
	if manager.Target("unknown", d) != nil {
		t.Errorf("expected nil, got %v", manager.Target("unknown", d))
	}

	// Wait for go routines
	wg.Wait()
}
