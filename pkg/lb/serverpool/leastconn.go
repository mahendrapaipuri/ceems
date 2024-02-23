package serverpool

import (
	"math"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// leastConn implements the least connection load balancer strategy
type leastConn struct {
	backends []backend.TSDBServer
}

// Target returns the backend server to send the request if it is alive
func (s *leastConn) Target(d time.Duration) backend.TSDBServer {
	var targetBackend backend.TSDBServer
	var activeConnections = math.MaxInt32
	for _, backend := range s.backends {
		if !backend.IsAlive() {
			continue
		}

		backendActiveConnections := backend.ActiveConnections()
		if activeConnections > backendActiveConnections {
			targetBackend = backend
			activeConnections = backendActiveConnections
		}
	}
	return targetBackend
}

func (s *leastConn) Add(b backend.TSDBServer) {
	s.backends = append(s.backends, b)
}

func (s *leastConn) Size() int {
	return len(s.backends)
}

func (s *leastConn) Backends() []backend.TSDBServer {
	return s.backends
}
