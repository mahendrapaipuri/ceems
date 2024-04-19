package cli

import (
	"fmt"
	"os"

	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"gopkg.in/yaml.v2"
)

// Config defines the backend servers config
type Config struct {
	Backends   []base.Backend `yaml:"backends"`
	Strategy   string         `yaml:"strategy"`
	AdminUsers []string       `yaml:"admin_users"`
	CEEMSAPI   base.CEEMSAPI  `yaml:"ceems_api"`
	Grafana    base.Grafana   `yaml:"grafana"`
}

func getLBConfig(filePath string) (*Config, error) {
	var config Config
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		return nil, err
	}
	if len(config.Backends) == 0 {
		return nil, fmt.Errorf("backend hosts expected, none provided")
	}
	return &config, nil
}
