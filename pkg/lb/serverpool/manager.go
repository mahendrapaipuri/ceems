// Package serverpool implements the interface that manages pool of backend servers of
// load balancer app
package serverpool

import (
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// Manager is the interface every strategy must implement
type Manager interface {
	Backends() map[string][]backend.TSDBServer
	Target(string, time.Duration) backend.TSDBServer
	Add(string, backend.TSDBServer)
	Size(string) int
}

// NewManager returns a new instance of server pool manager
func NewManager(strategy string, logger log.Logger) (Manager, error) {
	switch strategy {
	case "round-robin":
		return &roundRobin{
			backends: make(map[string][]backend.TSDBServer, 0),
			current:  0,
			logger:   logger,
		}, nil
	case "least-connection":
		return &leastConn{
			backends: make(map[string][]backend.TSDBServer, 0),
			logger:   logger,
		}, nil
	case "resource-based":
		return &resourceBased{
			backends: make(map[string][]backend.TSDBServer, 0),
			logger:   logger,
		}, nil
	default:
		return nil, fmt.Errorf("invalid strategy")
	}
}
