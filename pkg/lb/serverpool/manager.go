// Package serverpool implements the interface that manages pool of backend servers of
// load balancer app
package serverpool

import (
	"errors"
	"log/slog"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// Custom errors.
var (
	ErrInvalidStrategy = errors.New("invalid strategy")
)

// Manager is the interface every strategy must implement.
type Manager interface {
	Backends() map[string][]backend.Server
	Target(id string, d time.Duration) backend.Server
	Add(id string, b backend.Server)
	Size(id string) int
}

// New returns a new instance of server pool manager.
func New(strategy string, logger *slog.Logger) (Manager, error) {
	switch strategy {
	case "round-robin":
		return &roundRobin{
			backends: make(map[string][]backend.Server, 0),
			current:  0,
			logger:   logger,
		}, nil
	case "least-connection":
		return &leastConn{
			backends: make(map[string][]backend.Server, 0),
			logger:   logger,
		}, nil
	case "resource-based":
		return &resourceBased{
			backends: make(map[string][]backend.Server, 0),
			logger:   logger,
		}, nil
	default:
		return nil, ErrInvalidStrategy
	}
}
