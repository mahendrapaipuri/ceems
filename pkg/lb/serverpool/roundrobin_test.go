package serverpool

import (
	"fmt"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

func TestRoundRobinIteration(t *testing.T) {
	d := time.Duration(0 * time.Second)
	manager, err := NewManager("round-robin")
	if err != nil {
		t.Fatal(err)
	}

	// Make dummy backend servers
	backendURLs := make([]*url.URL, 3)
	backends := make([]backend.TSDBServer, 3)
	for i := 0; i < 2; i++ {
		backendURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", 3333+i))
		if err != nil {
			t.Fatal(err)
		}
		backendURLs[i] = backendURL

		rp := httputil.NewSingleHostReverseProxy(backendURL)
		backend := backend.NewTSDBServer(backendURL, rp)
		backends[i] = backend
		manager.Add(backend)
	}

	// Start wait group
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			manager.Target(d)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 2; i++ {
			manager.Target(d)
		}
	}()

	// Wait for all go routines
	wg.Wait()

	// This should be backends[0] as next round is multiple of 3
	if backends[0].URL().String() != manager.Target(d).URL().String() {
		t.Errorf("expected , got %v", manager.Target(d))
	}
}
