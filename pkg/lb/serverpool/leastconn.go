package serverpool

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// leastConn implements the least connection load balancer strategy.
type leastConn struct {
	backends map[string][]backend.TSDBServer
	logger   *slog.Logger
}

// Target returns the backend server to send the request if it is alive.
func (s *leastConn) Target(id string, d time.Duration) backend.TSDBServer {
	// If the ID is unknown return
	if _, ok := s.backends[id]; !ok {
		s.logger.Error("Round Robin strategy", "err", fmt.Errorf("unknown backend ID: %s", id))

		return nil
	}

	var targetBackend backend.TSDBServer

	activeConnections := math.MaxInt32

	for _, backend := range s.backends[id] {
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
		s.logger.Debug("Least connection strategy", "selected_backend", targetBackend.String())

		return targetBackend
	}

	return nil
}

func (s *leastConn) Add(id string, b backend.TSDBServer) {
	s.backends[id] = append(s.backends[id], b)
}

func (s *leastConn) Size(id string) int {
	return len(s.backends[id])
}

func (s *leastConn) Backends() map[string][]backend.TSDBServer {
	return s.backends
}
