package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func queryServer(address string) error {
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/api/%s/health", address, base.APIVersion), nil)
	req.Header.Set("X-Grafana-User", "usr1")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := resp.Body.Close(); err != nil {
		return err
	}

	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}

	return nil
}

func makeConfigFile(configFile string, tmpDir string) string {
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0o600)

	return configPath
}

func TestCEEMSConfigNestedDataDirs(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data1", "data2", "data3")
	backupDataDir := filepath.Join(tmpDir, "data1", "data2", "data3", "bakcup")

	config := &CEEMSAPIAppConfig{
		CEEMSAPIServerConfig{
			Data: db.DataConfig{
				Path:       dataDir,
				BackupPath: backupDataDir,
			},
		},
	}

	// Setup data directories
	var err error
	config, err = createDirs(config)
	require.NoError(t, err, "failed to create data directories")

	// Check data dirs exists
	assert.DirExists(t, dataDir, "data directory does not exist")
	assert.DirExists(t, backupDataDir, "backup data directory does not exist")

	// Check if paths are absolute
	assert.True(t, filepath.IsAbs(config.Server.Data.Path), "data path is not absolute")
	assert.True(t, filepath.IsAbs(config.Server.Data.BackupPath), "backup path is not absolute")
}

func TestCEEMSConfigMalformedData(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Make config file
	configFileTmpl := `
---
ceems_api_server:
  data:
    path: %s
    retention_period: 3l`

	configFile := fmt.Sprintf(configFileTmpl, dataDir)
	configFilePath := makeConfigFile(configFile, tmpDir)

	_, err := common.MakeConfig[CEEMSAPIAppConfig](configFilePath)
	assert.Error(t, err)
}

func TestCEEMSServerMain(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Make config file
	configFileTmpl := `
---
ceems_api_server:
  data:
    path: %[1]s
    backup_path: %[1]s`

	configFile := fmt.Sprintf(configFileTmpl, dataDir)
	configFilePath := makeConfigFile(configFile, tmpDir)

	// Create sample DB file
	os.MkdirAll(dataDir, os.ModePerm)

	f, err := os.Create(filepath.Join(dataDir, base.CEEMSDBName))
	if err != nil {
		require.NoError(t, err)
	}

	f.Close()

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, "--config.file="+configFilePath)
	os.Args = append(os.Args, "--log.level=debug")
	a, _ := NewCEEMSServer()

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := range 10 {
		if err := queryServer("localhost:9020"); err == nil {
			break
		}

		time.Sleep(500 * time.Millisecond)

		if i == 9 {
			require.Errorf(t, fmt.Errorf("Could not start stats server after %d attempts", i), "failed to start server")
		}
	}

	// Send INT signal and wait a second to clean up server and DB
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	time.Sleep(1 * time.Second)
}
