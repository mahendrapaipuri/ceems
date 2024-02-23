package serverpool

import (
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

func TestUnAvailableBackends(t *testing.T) {
	d := time.Duration(0 * time.Second)

	// Start manager
	manager, err := NewManager("least-connection")
	if err != nil {
		t.Fatal(err)
	}

	// Make dummy backend servers
	backendURLs := make([]*url.URL, 2)
	backends := make([]backend.TSDBServer, 2)
	for i := 0; i < 2; i++ {
		backendURL, err := url.Parse("http://localhost:1234")
		if err != nil {
			t.Fatal(err)
		}
		backendURLs[i] = backendURL

		rp := httputil.NewSingleHostReverseProxy(backendURL)
		backend := backend.NewTSDBServer(backendURL, false, rp)
		backends[i] = backend
		manager.Add(backend)
	}

	// Check manager size
	if manager.Size() != 2 {
		t.Errorf("expected 2 backend TSDB servers, got %d", manager.Size())
	}

	// Set backend1 to dead
	backends[0].SetAlive(false)

	// Get target and it should be backend2
	if target := manager.Target(d); target.URL() != backendURLs[1] {
		t.Errorf("expected backend2, got %s", target.URL().String())
	}

	// Set backend2 to dead as well
	backends[1].SetAlive(false)

	// Get target and it should be nil
	if target := manager.Target(d); target != nil {
		t.Errorf("expected nil, got %s", target)
	}
}

func TestLeastConnectionLB(t *testing.T) {
	d := time.Duration(0 * time.Second)

	// Start manager
	manager, err := NewManager("least-connection")
	if err != nil {
		t.Fatal(err)
	}

	// Make dummy backend servers
	backendURLs := make([]*url.URL, 2)
	backends := make([]backend.TSDBServer, 2)
	for i := 0; i < 2; i++ {
		dummyServer := httptest.NewServer(h)
		defer dummyServer.Close()
		backendURL, err := url.Parse(dummyServer.URL)
		if err != nil {
			t.Fatal(err)
		}
		backendURLs[i] = backendURL

		rp := httputil.NewSingleHostReverseProxy(backendURL)
		backend := backend.NewTSDBServer(backendURL, false, rp)
		backends[i] = backend
		manager.Add(backend)
	}

	// Check manager size
	if manager.Size() != 2 {
		t.Errorf("expected 2 backend TSDB servers, got %d", manager.Size())
	}

	// Start wait group
	var wg sync.WaitGroup
	wg.Add(1)

	// Check if we get non nil target
	var target backend.TSDBServer
	if target = manager.Target(d); target == nil {
		t.Errorf("expected a target, got nil")
	}

	// Serve a long request
	go func() {
		defer wg.Done()
		target.Serve(w, req)
	}()

	// Let the server serve request
	time.Sleep(1 * time.Second)

	// Check new target is not nil
	if newTarget := manager.Target(d); newTarget == nil {
		t.Errorf("expected a new target, got nil")
	} else {
		if connTwo := newTarget.ActiveConnections(); connTwo != 0 {
			t.Errorf("expected 0 connections for two, got %d", connTwo)
		}

		// New target must not be old one
		if target == newTarget {
			t.Errorf("expected target and newTarget to be different")
		}
	}

	// Wait for go routines
	wg.Wait()
}
