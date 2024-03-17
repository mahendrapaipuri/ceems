package serverpool

import (
	"math"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// leastConn implements the least connection load balancer strategy
type leastConn struct {
	backends []backend.TSDBServer
	logger   log.Logger
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
	if targetBackend != nil {
		level.Debug(s.logger).Log("msg", "Least connection strategy", "selected_backend", targetBackend.String())
		return targetBackend
	}
	return nil
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
