package serverpool

import (
	"sync"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// roundRobin implements round robin load balancer strategy
type roundRobin struct {
	backends []backend.TSDBServer
	mux      sync.RWMutex
	current  int
}

// Rotate returns the backend server to be used for next request
func (s *roundRobin) Rotate() backend.TSDBServer {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.current = (s.current + 1) % s.Size()
	return s.backends[s.current]
}

// Target returns the backend server to send the request if it is alive
func (s *roundRobin) Target(d time.Duration) backend.TSDBServer {
	for i := 0; i < s.Size(); i++ {
		nextPeer := s.Rotate()
		if nextPeer.IsAlive() {
			return nextPeer
		}
	}
	return nil
}

// List all backend servers in pool
func (s *roundRobin) Backends() []backend.TSDBServer {
	return s.backends
}

// Add a backend server to pool
func (s *roundRobin) Add(b backend.TSDBServer) {
	s.backends = append(s.backends, b)
}

// Total number of backend servers in pool
func (s *roundRobin) Size() int {
	return len(s.backends)
}
