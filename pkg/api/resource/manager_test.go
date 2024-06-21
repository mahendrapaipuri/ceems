package resource

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
clusters:
  - id: default
    manager: slurm
    cli:
      path: %[1]s
    web:
      url: %[2]s`
	case "two_instances":
		configFileTmpl = `
---
clusters:
  - id: slurm-0
    manager: slurm
    cli:
      path: %[1]s
    web:
      url: %[2]s
  - id: slurm-1
    manager: slurm
    cli:
      path: %[1]s
    web:
      url: %[2]s`
	case "mixed_instances":
		configFileTmpl = `
---
clusters:
  - id: slurm-0
    manager: slurm
    cli:
      path: %[1]s
    web:
      url: %[2]s
  - id: slurm-1
    manager: slurm
    cli:
      path: %[1]s
    web:
      url: %[2]s
  - id: openstack-0
    manager: openstack
    cli:
      path: %[1]s
    web:
      url: %[2]s`
	case "unknown_manager":
		configFileTmpl = `
---
clusters:
  - id: manager-0
    manager: unknown
    cli:
      path: %[1]s
    web:
      url: %[2]s
  - id: slurm-1
    manager: slurm
    cli:
      path: %[1]s
    web:
      url: %[2]s`
	case "malformed_1":
		// Missing s in tsbd_instances
		configFileTmpl = `
---
resource_manager:
  - id: default`
	case "malformed_2":
		// Missing manager name
		configFileTmpl = `
---
clusters:
  - id: default
    web:
      url: %[2]s`
	case "malformed_3":
		// Duplicated IDs
		configFileTmpl = `
---
clusters:
  - id: default
    web:
      url: %[2]s
  - id: default
    web:
      url: %[2]s`
	}

	configFile := fmt.Sprintf(configFileTmpl, tmpDir, serverURL)
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0600)
	return configPath
}

func TestMalformedConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_1", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("failed to create manager config: %s", err)
	}
	if len(cfg.Clusters) != 0 {
		t.Errorf("expected no clusters, got %#v", cfg.Clusters)
	}
}

func TestMissingManagerConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_2", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("failed to create manager config: %s", err)
	}
	if _, err = checkConfig([]string{"slurm"}, cfg); err == nil {
		t.Errorf("expected error due to missing manager name in config. Got none")
	}
}

func TestUnknownManagerConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "unknown_manager", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("failed to create manager config: %s", err)
	}
	if _, err = checkConfig([]string{"slurm"}, cfg); err == nil {
		t.Errorf("expected error due to unknown manager name in config. Got none")
	}
}

func TestDuplicatedIDsConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_3", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("failed to create manager config: %s", err)
	}
	if _, err = checkConfig([]string{"slurm"}, cfg); err == nil {
		t.Errorf("expected error due to duplicated IDs in config. Got none")
	}
}

func TestOneClusterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "one_instance", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("Failed to create manager config: %s", err)
	}
	if len(cfg.Clusters) != 1 {
		t.Errorf("expected one cluster, got %#v", cfg.Clusters)
	}

	if _, err = checkConfig([]string{"slurm"}, cfg); err != nil {
		t.Errorf("config failed preflight checks to %s", err)
	}
}

func TestMixedClusterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "mixed_instances", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("Failed to create manager config: %s", err)
	}
	if len(cfg.Clusters) != 3 {
		t.Errorf("expected mixed clusters, got %#v", cfg.Clusters)
	}

	if _, err = checkConfig([]string{"slurm", "openstack"}, cfg); err != nil {
		t.Errorf("config failed preflight checks to %s", err)
	}
}
