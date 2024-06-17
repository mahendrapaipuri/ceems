package serverpool

import (
	"fmt"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

var (
	rrIDs = []string{"rr0", "rr1"}
)

func TestRoundRobinIteration(t *testing.T) {
	d := time.Duration(0 * time.Second)
	manager, err := NewManager("round-robin", log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Make dummy backend servers
	backendURLs := make(map[string][]*url.URL, 3)
	backends := make(map[string][]backend.TSDBServer, 3)
	for i := 0; i < 3; i++ {
		for j, id := range rrIDs {
			backendURL, err := url.Parse(fmt.Sprintf("http://localhost:%d", 3333*(i+1)+j))
			if err != nil {
				t.Fatal(err)
			}

			if _, ok := backendURLs[id]; !ok {
				backendURLs[id] = make([]*url.URL, 3)
				backends[id] = make([]backend.TSDBServer, 3)
			}

			backendURLs[id][i] = backendURL

			rp := httputil.NewSingleHostReverseProxy(backendURL)
			backend := backend.NewTSDBServer(backendURL, rp, log.NewNopLogger())
			backends[id][i] = backend
			manager.Add(id, backend)
		}
	}

	// Start wait group
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			manager.Target(rrIDs[0], d)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 2; i++ {
			manager.Target(rrIDs[1], d)
		}
	}()

	// Wait for all go routines
	wg.Wait()

	// This should be backends[0] as next round is multiple of 3 for rrID[0]
	// and backends[2] for rrIDs[1]
	for i, id := range rrIDs {
		if backends[id][i].URL().String() != manager.Target(id, d).URL().String() {
			t.Errorf("expected %s, got %s", backends[id][i].URL().String(), manager.Target(id, d).URL().String())
		}
	}

	// For unknown ID expect nil
	if manager.Target("unknown", d) != nil {
		t.Errorf("expected nil, got %v", manager.Target("unknown", d))
	}
}
