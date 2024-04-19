// Package base defines base variables that will be used in lb package
package base

import "github.com/alecthomas/kingpin/v2"

// CEEMSLoadBalancerAppName is kingpin app name
const CEEMSLoadBalancerAppName = "ceems_lb"

// CEEMSLoadBalancerApp is kingpin CLI app
var CEEMSLoadBalancerApp = *kingpin.New(
	CEEMSLoadBalancerAppName,
	"Prometheus load balancer to query from different instances.",
)

// CEEMSAPI defines CEEMS API server struct
type CEEMSAPI struct {
	WebURL        string `yaml:"url"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify"`
	DBPath        string `yaml:"db_path"`
}

// Grafana defines Grafana server struct
type Grafana struct {
	WebURL        string `yaml:"url"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify"`
	AdminTeamID   string `yaml:"admin_team_id"`
}

// Backend defines backend server
type Backend struct {
	URL string `yaml:"url"`
}
