package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	binary, _ = filepath.Abs("batchjob_stats")
)

func TestBatchjobStatExecutable(t *testing.T) {
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("batchjob_stats binary not available, try to run `make build` first: %s", err)
	}
	tmpDir := t.TempDir()

	jobstats := exec.Command(binary, "--data.path", tmpDir)
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
