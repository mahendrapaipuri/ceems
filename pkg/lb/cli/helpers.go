package cli

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Backend defines backend server
type Backend struct {
	URL           string `yaml:"url"`
	SkipTLSVerify bool   `yaml:"skip_tls_verify"`
}

// Config defines the backend servers config
type Config struct {
	Backends []Backend `yaml:"backends"`
	Strategy string    `yaml:"strategy"`
	DBPath   string    `yaml:"db_path"`
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
