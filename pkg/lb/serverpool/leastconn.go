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
	backends map[string][]backend.Server
	logger   *slog.Logger
}

// Target returns the backend server to send the request if it is alive.
func (s *leastConn) Target(id string, _ time.Duration) backend.Server {
	// If the ID is unknown return
	if _, ok := s.backends[id]; !ok {
		s.logger.Error("Round Robin strategy", "err", fmt.Errorf("unknown backend ID: %s", id))

		return nil
	}

	var targetBackend backend.Server

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
		s.logger.Debug("Least connection strategy", "cluster_id", id, "selected_backend", targetBackend.String())

		return targetBackend
	}

	return nil
}

func (s *leastConn) Add(id string, b backend.Server) {
	s.logger.Debug("Backend added", "strategy", "least-connection", "cluster_id", id, "backend", b.String())

	s.backends[id] = append(s.backends[id], b)
}

func (s *leastConn) Size(id string) int {
	return len(s.backends[id])
}

func (s *leastConn) Backends() map[string][]backend.Server {
	return s.backends
}
