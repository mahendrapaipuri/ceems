package cli

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestGetConfigSuccess(t *testing.T) {
	config := Config{
		Strategy: "round-robin",
		Backends: []Backend{
			{
				URL: "http://localhost:9090",
			},
		},
	}
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		t.Errorf("failed to marshal: %s", err)
	}

	// Write config to file
	configPath := filepath.Join(t.TempDir(), "config.yml")
	err = os.WriteFile(configPath, yamlData, 0644)
	if err != nil {
		t.Fatal("failed to create config file")
	}

	// Read config
	c, err := getLBConfig(configPath)
	if err != nil {
		t.Errorf("failed to read config file: %s", err)
	}
	if len(c.Backends) < 1 {
		t.Errorf("expected 1 backend none found")
	}
}

func TestGetConfigFail(t *testing.T) {
	config := Config{
		Strategy: "round-robin",
	}
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		t.Errorf("failed to marshal: %s", err)
	}

	// Write config to file
	configPath := filepath.Join(t.TempDir(), "config.yml")
	err = os.WriteFile(configPath, yamlData, 0644)
	if err != nil {
		t.Fatal("failed to create config file")
	}

	// Read config
	_, err = getLBConfig(configPath)
	if err == nil {
		t.Errorf("expected error due to no backends")
	}
}
