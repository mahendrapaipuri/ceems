package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setCLIArgs() {
	os.Args = append(os.Args, "--resource.manager.slurm")
	os.Args = append(os.Args, "--slurm.sacct.path=../fixtures/sacct")
	os.Args = append(os.Args, "--log.level=error")
}

func queryServer(address string) error {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://%s/api/health", address), nil)
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

func TestBatchStatsServerMainNestedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data1", "data2", "data3")
	backupDataDir := filepath.Join(tmpDir, "data1", "data2", "data3", "bakcup")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, fmt.Sprintf("--storage.data.backup.path=%s", backupDataDir))
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	go func() {
		a.Main()
	}()

	// Query exporter
	for i := 0; i < 10; i++ {
		if err := queryServer("localhost:9020"); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if i == 9 {
			t.Errorf("Could not start stats server after %d attempts", i)
		}
	}

	// Check data dir exists
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("Data directory does not exist")
	}
	if _, err := os.Stat(backupDataDir); err != nil {
		t.Errorf("Backup data directory does not exist")
	}
}

func TestBatchStatsServerMainRetentionMalformedDuration(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--storage.data.retention.period=3l")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected CLI arg parsing error")
	}
}

func TestBatchStatsServerMainUpdateIntMalformedDuration(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--storage.data.update.interval=3l")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected CLI arg parsing error")
	}
}

func TestBatchStatsServerMainBackupIntMalformedDuration(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--storage.data.backup.interval=3l")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected CLI arg parsing error")
	}
}

func TestBatchStatsServerJobCutoffIntMalformedDuration(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--storage.data.job.duration.cutoff=3l")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected CLI arg parsing error")
	}
}

func TestBatchStatsServerMalformedLastUpdateTime(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--storage.data.update.from=12-10-2008")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected CLI arg parsing error")
	}
}

func TestBatchStatsServerTSDBCLIArgs(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--tsdb.data.clean")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected TSDB web url CLI arg error")
	}
}

func TestBatchStatsServerGrafanaCLIArgs(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")

	// Remove test related args
	os.Args = append([]string{os.Args[0]}, fmt.Sprintf("--storage.data.path=%s", dataDir))
	os.Args = append(os.Args, "--web.admin-users.sync.from.grafana")
	setCLIArgs()
	a, _ := NewCEEMSServer()

	// Start Main
	if err := a.Main(); err == nil {
		t.Errorf("Expected Grafana web url CLI arg error")
	}
}
