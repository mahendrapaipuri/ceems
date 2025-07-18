// Package serverpool implements the interface that manages pool of backend servers of
// load balancer app
package serverpool

import (
	"errors"
	"log/slog"

	"github.com/ceems-dev/ceems/pkg/lb/backend"
	"github.com/ceems-dev/ceems/pkg/lb/base"
)

// Custom errors.
var (
	ErrInvalidStrategy = errors.New("invalid strategy")
)

// Manager is the interface every strategy must implement.
type Manager interface {
	Backends() map[string][]backend.Server
	Target(id string) backend.Server
	Add(id string, b backend.Server)
	Size(id string) int
}

// New returns a new instance of server pool manager.
func New(strategy base.LBStrategy, logger *slog.Logger) (Manager, error) {
	switch strategy {
	case base.RoundRobin:
		return &roundRobin{
			backends: make(map[string][]backend.Server, 0),
			current:  0,
			logger:   logger,
		}, nil
	case base.LeastConnection:
		return &leastConn{
			backends: make(map[string][]backend.Server, 0),
			logger:   logger,
		}, nil
	default:
		return nil, ErrInvalidStrategy
	}
}
