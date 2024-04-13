package cli

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Grafana defines Grafana server struct
type Grafana struct {
	URL           string `yaml:"url"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify"`
	AdminTeamID   string `yaml:"admin_team_id"`
}

// Backend defines backend server
type Backend struct {
	URL string `yaml:"url"`
}

// Config defines the backend servers config
type Config struct {
	Backends   []Backend `yaml:"backends"`
	Strategy   string    `yaml:"strategy"`
	DBPath     string    `yaml:"db_path"`
	AdminUsers []string  `yaml:"admin_users"`
	Grafana    Grafana   `yaml:"grafana"`
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
