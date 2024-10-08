package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var binary, _ = filepath.Abs("../../bin/ceems_api_server")

const (
	address = "localhost:19020"
)

func TestBatchjobStatsExecutable(t *testing.T) {
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("ceems_api_server binary not available, try to run `make build` first: %s", err)
	}

	tmpDir := t.TempDir()
	tmpSacctPath := tmpDir + "/sacct"

	sacctPath, err := filepath.Abs("../../pkg/api/testdata/sacct")
	if err != nil {
		t.Error(err)
	}

	err = os.Link(sacctPath, tmpSacctPath)
	if err != nil {
		t.Error(err)
	}

	usagestats := exec.Command(
		binary,
		"--web.listen-address", address,
		"--no-security.drop-privileges",
	)
	if err := runCommandAndTests(usagestats); err != nil {
		t.Error(err)
	}
}

func runCommandAndTests(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Sleep for a while and kill process
	time.Sleep(1 * time.Second)

	cmd.Process.Kill()

	return nil
}
