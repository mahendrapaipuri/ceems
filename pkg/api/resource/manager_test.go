package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// mockResourceManager struct
type mockResourceManager struct {
	logger log.Logger
}

// NewMockResourceManager returns a new defaultResourceManager that returns empty compute units
func NewMockResourceManager(cluster models.Cluster, logger log.Logger) (Fetcher, error) {
	level.Info(logger).Log("msg", "Default resource manager activated")
	return &mockResourceManager{
		logger: logger,
	}, nil
}

// Return empty units response
func (d *mockResourceManager) FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return []models.ClusterUnits{
		{
			Cluster: models.Cluster{ID: "mock"},
			Units: []models.Unit{
				{
					UUID: "10000",
				},
			},
		},
	}, nil
}

// Return empty projects response
func (d *mockResourceManager) FetchUsersProjects(
	currentTime time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return []models.ClusterUsers{
			{
				Cluster: models.Cluster{ID: "mock"},
				Users: []models.User{
					{
						Name: "foo",
					},
				},
			},
		}, []models.ClusterProjects{
			{
				Cluster: models.Cluster{ID: "mock"},
				Projects: []models.Project{
					{
						Name: "fooprj",
					},
				},
			},
		}, nil
}

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
	case "mock_instance":
		configFileTmpl = `
---
clusters:
  - id: default
    manager: mock
    cli:
      path: %[1]s
    web:
      url: %[2]s`
	case "empty_instance":
		configFileTmpl = `
---
# %[1]s %[2]s
clusters: []`
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
		// Missing s in clusters
		configFileTmpl = `
---
# %[1]s %[2]s
cluster:
  - id: default`
	case "malformed_2":
		// Missing manager name
		configFileTmpl = `
---
# %[1]s
clusters:
  - id: default
    web:
      url: %[2]s`
	case "malformed_3":
		// Duplicated IDs
		configFileTmpl = `
---
# %[1]s
clusters:
  - id: default
    web:
      url: %[2]s
  - id: default
    web:
      url: %[2]s`
	case "malformed_4":
		// invalid ID
		configFileTmpl = `
---
# %[1]s
clusters:
  - id: defau!$lt
    manager: slurm
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

func TestInvalidIDManagerConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_4", "")

	cfg, err := managerConfig()
	if err != nil {
		t.Errorf("failed to create manager config: %s", err)
	}
	if _, err = checkConfig([]string{"slurm"}, cfg); err == nil {
		t.Errorf("expected error due to invalid ID in config. Got none")
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

func TestNewManager(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "mock_instance", "")

	// Register mock manager
	RegisterManager("mock", NewMockResourceManager)

	// Create new manager
	manager, err := NewManager(log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to create new manager: %s", err)
	}

	// Fetch units
	units, err := manager.FetchUnits(time.Now(), time.Now())
	if err != nil {
		t.Errorf("failed to fetch units: %s", err)
	}
	if len(units[0].Units) != 1 {
		t.Errorf("expected only 1 unit got %d", len(units[0].Units))
	}

	// Fetch users and projects
	users, projects, err := manager.FetchUsersProjects(time.Now())
	if err != nil {
		t.Errorf("failed to fetch users and projects: %s", err)
	}
	// Index 0 seems to be default manager
	if len(users[0].Users) != 1 || len(projects[0].Projects) != 1 {
		t.Errorf("expected 1 user and 1 project, got %d, %d", len(users[0].Users), len(projects[0].Projects))
	}
}

func TestNewManagerWithNoClusters(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "empty_instance", "")

	// Register mock manager
	RegisterManager("mock", NewMockResourceManager)

	// Create new manager
	manager, err := NewManager(log.NewNopLogger())
	if err != nil {
		t.Errorf("failed to create new manager: %s", err)
	}

	// Fetch units
	units, err := manager.FetchUnits(time.Now(), time.Now())
	if err != nil {
		t.Errorf("failed to fetch units: %s", err)
	}
	if len(units[0].Units) != 0 {
		t.Errorf("expected only 0 units got %d", len(units[0].Units))
	}

	// Fetch users and projects
	users, projects, err := manager.FetchUsersProjects(time.Now())
	if err != nil {
		t.Errorf("failed to fetch users and projects: %s", err)
	}
	// Index 0 seems to be default manager
	if len(users[0].Users) != 0 || len(projects[0].Projects) != 0 {
		t.Errorf("expected 0 users and 0 projects, got %d, %d", len(users[0].Users), len(projects[0].Projects))
	}
}
