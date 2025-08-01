package resource

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/base"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var noOpLogger = slog.New(slog.DiscardHandler)

// mockResourceManager struct.
type mockResourceManager struct {
	logger *slog.Logger
}

// NewMockResourceManager returns a new defaultResourceManager that returns empty compute units.
func NewMockResourceManager(cluster models.Cluster, logger *slog.Logger) (Fetcher, error) {
	logger.Info("Default resource manager activated")

	return &mockResourceManager{
		logger: logger,
	}, nil
}

// Return empty units response.
func (d *mockResourceManager) FetchUnits(
	_ context.Context,
	start time.Time,
	end time.Time,
) ([]models.ClusterUnits, error) {
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

// Return empty projects response.
func (d *mockResourceManager) FetchUsersProjects(
	_ context.Context,
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

func mockConfig(tmpDir string, cfg string) string {
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

	configFile := fmt.Sprintf(configFileTmpl, tmpDir, "")
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0o600)

	return configPath
}

func TestMalformedConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_1")

	cfg, err := managerConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Clusters)
}

func TestMissingManagerConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_2")

	cfg, err := managerConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"slurm"}, cfg)
	assert.Error(t, err, "missing manager")
}

func TestUnknownManagerConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "unknown_manager")

	cfg, err := managerConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"slurm"}, cfg)
	assert.Error(t, err, "unknown manager")
}

func TestInvalidIDManagerConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_4")

	cfg, err := managerConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"slurm"}, cfg)
	assert.Error(t, err, "invalid ID")
}

func TestDuplicatedIDsConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_3")

	cfg, err := managerConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"slurm"}, cfg)
	assert.Error(t, err, "duplicated IDs")
}

func TestOneClusterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "one_instance")

	cfg, err := managerConfig()
	require.NoError(t, err)

	assert.Len(t, cfg.Clusters, 1)

	_, err = checkConfig([]string{"slurm"}, cfg)
	assert.NoError(t, err)
}

func TestMixedClusterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "mixed_instances")

	cfg, err := managerConfig()
	require.NoError(t, err)

	assert.Len(t, cfg.Clusters, 3)

	_, err = checkConfig([]string{"slurm", "openstack"}, cfg)
	assert.NoError(t, err)
}

func TestNewManager(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "mock_instance")
	ctx := t.Context()

	// Register mock manager
	Register("mock", NewMockResourceManager)

	// Create new manager
	manager, err := New(noOpLogger)
	require.NoError(t, err)

	// Fetch units
	units, err := manager.FetchUnits(ctx, time.Now(), time.Now())
	require.NoError(t, err)
	require.Len(t, units[0].Units, 1)

	// Fetch users and projects
	users, projects, err := manager.FetchUsersProjects(ctx, time.Now())
	require.NoError(t, err)

	// Index 0 seems to be default manager
	assert.Len(t, users[0].Users, 1)
	assert.Len(t, projects[0].Projects, 1)
}

func TestNewManagerWithNoClusters(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "empty_instance")
	ctx := t.Context()

	// Register mock manager
	Register("mock", NewMockResourceManager)

	// Create new manager
	manager, err := New(noOpLogger)
	require.NoError(t, err)

	// Fetch units
	units, err := manager.FetchUnits(ctx, time.Now(), time.Now())
	require.NoError(t, err)
	require.Empty(t, units[0].Units)

	// Fetch users and projects
	users, projects, err := manager.FetchUsersProjects(ctx, time.Now())
	require.NoError(t, err)

	// Index 0 seems to be default manager
	assert.Empty(t, users[0].Users)
	assert.Empty(t, projects[0].Projects)
}
