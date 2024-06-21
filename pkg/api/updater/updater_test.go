package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
)

func mockConfig(tmpDir string, cfg string, serverURL string) string {
	var configFileTmpl string
	switch cfg {
	case "one_instance":
		configFileTmpl = `
---
updaters:
  - id: default
    updater: tsdb
    web: 
      url: %[1]s
    extra_config:
      cutoff_duration: %[2]s
      queries:
        avg_cpu_usage: foo
        avg_cpu_mem_usage: foo`
	case "two_instances":
		configFileTmpl = `
---
updaters:
  - id: default-0
    updater: tsdb
    web:
      url: %[1]s
    extra_config:
      cutoff_duration: %[2]s
      queries:
        avg_cpu_usage: foo
        avg_cpu_mem_usage: foo
  - id: default-1
    web:
      url: %[1]s
    extra_config:
      cutoff_duration: %[2]s
      queries:
        avg_cpu_usage: foo
        avg_cpu_mem_usage: foo`
	case "malformed_1":
		// Missing s in tsbd_instances
		configFileTmpl = `
---
updater:
  - id: default
    updater: tsdb`
	case "malformed_2":
		// Missing updater name
		configFileTmpl = `
---
updaters:
  - id: default`
	case "malformed_3":
		// Duplicated IDs
		configFileTmpl = `
---
updaters:
  - id: default
  - id: default`
	case "malformed_4":
		// Unknown updater
		configFileTmpl = `
---
updaters:
  - id: default
    updater: unknown`
	}

	configFile := fmt.Sprintf(configFileTmpl, serverURL, "2m")
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0600)
	return configPath
}

func TestMalformedConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_1", "http://localhost:9090")

	cfg, err := updaterConfig()
	if err != nil {
		t.Errorf("failed to created updater config: %s", err)
	}
	if len(cfg.Instances) != 0 {
		t.Errorf("expected no updater instances, got %#v", cfg.Instances)
	}
}

func TestMissingUpdaterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_2", "http://localhost:9090")

	cfg, err := updaterConfig()
	if err != nil {
		t.Errorf("failed to created updater config: %s", err)
	}
	if _, err = checkConfig([]string{"tsdb"}, cfg); err == nil {
		t.Errorf("expected error due to missing updater name in config. Got none")
	}
}

func TestUnknownUpdaterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_4", "http://localhost:9090")

	cfg, err := updaterConfig()
	if err != nil {
		t.Errorf("failed to created updater config: %s", err)
	}
	if _, err = checkConfig([]string{"tsdb"}, cfg); err == nil {
		t.Errorf("expected error due to unknown updater name in config. Got none")
	}
}

func TestDuplicatedIDsConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_3", "http://localhost:9090")

	cfg, err := updaterConfig()
	if err != nil {
		t.Errorf("failed to created updater config: %s", err)
	}
	if _, err = checkConfig([]string{"tsdb"}, cfg); err == nil {
		t.Errorf("expected error due to duplicated IDs in config. Got none")
	}
}

func TestOneInstanceConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "one_instance", "")

	cfg, err := updaterConfig()
	if err != nil {
		t.Errorf("Failed to create updater config: %s", err)
	}
	if len(cfg.Instances) != 1 {
		t.Errorf("expected one instance, got %#v", cfg.Instances)
	}

	if _, err = checkConfig([]string{"tsdb"}, cfg); err != nil {
		t.Errorf("config failed preflight checks to %s", err)
	}
}
