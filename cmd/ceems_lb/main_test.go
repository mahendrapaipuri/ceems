package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var (
	binary, _ = filepath.Abs("../../bin/ceems_lb")
)

const (
	address = "localhost:19030"
)

func TestCEEMSLBExecutable(t *testing.T) {
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("ceems_lb binary not available, try to run `make build` first: %s", err)
	}

	tmpDir := t.TempDir()
	tmpConfigPath := tmpDir + "/config.yaml"

	configPath, err := filepath.Abs("../../build/config/ceems_lb/config.yml")
	if err != nil {
		t.Error(err)
	}
	err = os.Link(configPath, tmpConfigPath)
	if err != nil {
		t.Error(err)
	}

	lb := exec.Command(
		binary, "--path.data", tmpDir,
		"--config.path", tmpConfigPath,
		"--web.listen-address", address,
	)
	if err := runCommandAndTests(lb); err != nil {
		t.Error(err)
	}
}

func runCommandAndTests(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %s", err)
	}

	// Sleep for a while and kill process
	time.Sleep(1 * time.Second)

	cmd.Process.Kill()
	return nil
}
