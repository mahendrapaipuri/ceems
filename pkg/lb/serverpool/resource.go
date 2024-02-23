package serverpool

import (
	"math"
	"slices"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/lb/backend"
)

// resourceBased implements resource based load balancer strategy. The resource is
// the retention period of each TSDB.
//
// Based on the request's "from" timestamp and backend TSDB retention period, load
// balancer will make a decision on which backend TSDB server to use. If a request
// can be served by multiple backend TSDB servers, the one with least retention period
// will be chosen as it is assumed as "hot" TSDB with maximum performance
type resourceBased struct {
	backends []backend.TSDBServer
}

// Target returns the backend server to send the request if it is alive
func (s *resourceBased) Target(d time.Duration) backend.TSDBServer {
	// Get a list of eligible TSDB servers based on retention period and
	// start time of TSDB query
	var targetBackend backend.TSDBServer
	var targetBackends []backend.TSDBServer
	var retentionPeriods []time.Duration
	for i := 0; i < s.Size(); i++ {
		if !s.backends[i].IsAlive() {
			continue
		}

		// If query duration is less than backend TSDB's retention period, it is
		// target backend as it can serve the query
		if d < s.backends[i].RetentionPeriod() {
			targetBackends = append(targetBackends, s.backends[i])
			retentionPeriods = append(retentionPeriods, s.backends[i].RetentionPeriod())
		}
	}

	// If no eligible servers found return
	if len(targetBackends) == 0 {
		return targetBackend
	}

	// Get the minimum retention period from all eligible backends
	minRetentionPeriod := slices.Min(retentionPeriods)

	// If multiple eligible servers has same retention period as minimum retention
	// period, return the one that has least connections
	var activeConnections = math.MaxInt32
	for i := 0; i < len(targetBackends); i++ {
		if !targetBackends[i].IsAlive() {
			continue
		}

		if retentionPeriods[i] == minRetentionPeriod {
			backendActiveConnections := targetBackends[i].ActiveConnections()
			if activeConnections > backendActiveConnections {
				targetBackend = targetBackends[i]
				activeConnections = backendActiveConnections
			}
		}
	}
	return targetBackend
}

// List all backend servers in pool
func (s *resourceBased) Backends() []backend.TSDBServer {
	return s.backends
}

// Add a backend server to pool
func (s *resourceBased) Add(b backend.TSDBServer) {
	s.backends = append(s.backends, b)
}

// Total number of backend servers in pool
func (s *resourceBased) Size() int {
	return len(s.backends)
}
