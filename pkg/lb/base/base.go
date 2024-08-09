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
	"Prometheus load balancer to query from different instances.",
)

// Backend defines backend server.
type Backend struct {
	ID   string   `yaml:"id"`
	URLs []string `yaml:"tsdb_urls"`
}
