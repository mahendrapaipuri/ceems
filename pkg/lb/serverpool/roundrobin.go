package serverpool

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// roundRobin implements round robin load balancer strategy.
type roundRobin struct {
	backends map[string][]backend.Server
	mux      sync.RWMutex
	current  int
	logger   *slog.Logger
}

// Rotate returns the backend server to be used for next request.
func (s *roundRobin) Rotate(id string) backend.Server {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.current = (s.current + 1) % s.Size(id)

	return s.backends[id][s.current]
}

// Target returns the backend server to send the request if it is alive.
func (s *roundRobin) Target(id string) backend.Server {
	// If the ID is unknown return
	if _, ok := s.backends[id]; !ok {
		s.logger.Error("Round Robin strategy", "err", fmt.Errorf("unknown backend ID: %s", id))

		return nil
	}

	for range s.Size(id) {
		nextPeer := s.Rotate(id)
		if nextPeer.IsAlive() {
			s.logger.Debug("Round Robin strategy", "cluster_id", id, "selected_backend", nextPeer.String())

			return nextPeer
		}
	}

	return nil
}

// List all backend servers in pool.
func (s *roundRobin) Backends() map[string][]backend.Server {
	return s.backends
}

// Add a backend server to pool.
func (s *roundRobin) Add(id string, b backend.Server) {
	s.logger.Debug("Backend added", "strategy", "roundrobin", "cluster_id", id, "backend", b.String())

	s.backends[id] = append(s.backends[id], b)
}

// Total number of backend servers in pool.
func (s *roundRobin) Size(id string) int {
	return len(s.backends[id])
}
