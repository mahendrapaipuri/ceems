package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
    updater: tsdb
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
# %[1]s %[2]s
updater:
  - id: default
    updater: tsdb`
	case "malformed_2":
		// Missing updater name
		configFileTmpl = `
---
# %[1]s %[2]s
updaters:
  - id: default`
	case "malformed_3":
		// Duplicated IDs
		configFileTmpl = `
---
# %[1]s %[2]s
updaters:
  - id: default
  - id: default`
	case "malformed_4":
		// Unknown updater
		configFileTmpl = `
---
# %[1]s %[2]s
updaters:
  - id: default
    updater: unknown`
	case "malformed_5":
		// invalid ID updater
		configFileTmpl = `
---
# %[1]s %[2]s
updaters:
  - id: defau%lt
    updater: tsdb`
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
	require.NoError(t, err)
	assert.Len(t, cfg.Instances, 0)
}

func TestMissingUpdaterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_2", "http://localhost:9090")

	cfg, err := updaterConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"tsdb"}, cfg)
	require.Error(t, err, "missing updater name")
}

func TestUnknownUpdaterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_4", "http://localhost:9090")

	cfg, err := updaterConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"tsdb"}, cfg)
	assert.Error(t, err, "unknown updater name")
}

func TestInvalidIDUpdaterConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_5", "http://localhost:9090")

	cfg, err := updaterConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"tsdb"}, cfg)
	assert.Error(t, err, "invalid ID")
}

func TestDuplicatedIDsConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "malformed_3", "http://localhost:9090")

	cfg, err := updaterConfig()
	require.NoError(t, err)

	_, err = checkConfig([]string{"tsdb"}, cfg)
	assert.Error(t, err, "duplicated IDs")
}

func TestOneInstanceConfig(t *testing.T) {
	// Make mock config
	base.ConfigFilePath = mockConfig(t.TempDir(), "one_instance", "")

	cfg, err := updaterConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Instances, 1)

	_, err = checkConfig([]string{"tsdb"}, cfg)
	assert.NoError(t, err)
}
