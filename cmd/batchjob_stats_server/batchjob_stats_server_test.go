package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	binary, _ = filepath.Abs("bin/batchjob_stats_server")
)

func TestBatchjobStatsServerExecutable(t *testing.T) {
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("batchjob_stats_server binary not available, try to run `make build` first: %s", err)
	}
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	file, err := os.Create(dbPath)
	if err != nil {
		t.Errorf("Failed to create DB file: %s", err)
	}
	file.Close()

	jobstats := exec.Command(binary, "--path.db", dbPath)
	if err := runCommandAndTests(jobstats); err != nil {
		t.Error(err)
	}
}

func runCommandAndTests(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %s", err)
	}
	return nil
}
