// Package base defines base variables that will be used in lb package
package base

import (
	"github.com/alecthomas/kingpin/v2"
)

// CEEMSLoadBalancerAppName is kingpin app name.
const CEEMSLoadBalancerAppName = "ceems_lb"

// CEEMSLoadBalancerApp is kingpin CLI app.
var CEEMSLoadBalancerApp = *kingpin.New(
	CEEMSLoadBalancerAppName,
	"CEEMS load balancer for TSDB and Pyroscope servers with access control support.",
)

// LBType is type of load balancer server.
type LBType int

// LB types enum.
const (
	_ LBType = iota
	PromLB
	PyroLB
)

func (l LBType) String() string {
	switch l {
	case PromLB:
		return "tsdb"
	case PyroLB:
		return "pyroscope"
	}

	return "undefined"
}

// LBStrategy is strategy of load balancer server.
type LBStrategy int

// LB types enum.
const (
	_ LBStrategy = iota
	RoundRobin
	LeastConnection
)

func (l LBStrategy) String() string {
	switch l {
	case RoundRobin:
		return "round-robin"
	case LeastConnection:
		return "least-connection"
	}

	return "undefined"
}

// LBStrategyMap defines the map to translate the value in config to enum.
var LBStrategyMap = map[string]LBStrategy{
	"round-robin":      RoundRobin,
	"least-connection": LeastConnection,
}
