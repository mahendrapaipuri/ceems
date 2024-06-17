package common

import (
	config_util "github.com/prometheus/common/config"
)

// GrafanaWebConfig makes HTTP Grafana config
type GrafanaWebConfig struct {
	URL              string                       `yaml:"url"`
	TeamsIDs         []string                     `yaml:"teams_ids"`
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`
}
