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
)

func queryServer(address string) error {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/api/%s/health", address, base.APIVersion), nil)
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
	os.WriteFile(configPath, []byte(configFile), 0600)
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
	if config, err = createDirs(config); err != nil {
		t.Errorf("failed to create data directories")
	}

	// Check data dir exists
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("Data directory does not exist")
	}
	if _, err := os.Stat(backupDataDir); err != nil {
		t.Errorf("Backup data directory does not exist")
	}

	// Check if paths are absolute
	if !filepath.IsAbs(config.Server.Data.Path) || !filepath.IsAbs(config.Server.Data.BackupPath) {
		t.Errorf("Data paths are not absolute")
	}
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

	if _, err := common.MakeConfig[CEEMSAPIAppConfig](configFilePath); err == nil {
		t.Errorf("Expected config parsing error")
	}
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

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--config.file=%s", configFilePath))
	os.Args = append(os.Args, "--log.level=debug")
	a, _ := NewCEEMSServer()

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := 0; i < 10; i++ {
		if err := queryServer("localhost:9020"); err == nil {
			break
		} else {
			fmt.Printf("err %s", err)
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Errorf("Could not start stats server after %d attempts", i)
		}
	}

	// Send INT signal and wait a second to clean up server and DB
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	time.Sleep(1 * time.Second)
}
