// Package serverpool implements the interface that manages pool of backend servers of
// load balancer app
package serverpool

import (
	"fmt"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// Manager is the interface every strategy must implement
type Manager interface {
	Backends() []backend.TSDBServer
	Target(time.Duration) backend.TSDBServer
	Add(backend.TSDBServer)
	Size() int
}

// NewManager returns a new instance of server pool manager
func NewManager(strategy string) (Manager, error) {
	switch strategy {
	case "round-robin":
		return &roundRobin{
			backends: make([]backend.TSDBServer, 0),
			current:  0,
		}, nil
	case "least-connection":
		return &leastConn{
			backends: make([]backend.TSDBServer, 0),
		}, nil
	case "resource-based":
		return &resourceBased{
			backends: make([]backend.TSDBServer, 0),
		}, nil
	default:
		return nil, fmt.Errorf("invalid strategy")
	}
}
