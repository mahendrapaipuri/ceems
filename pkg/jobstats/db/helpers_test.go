package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	_ "github.com/mattn/go-sqlite3"
)

func TestJobStatsDBPreparation(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBTable := "jobstats"
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	s := &storageConfig{
		dbPath: jobstatDBPath,
	}
	j := jobStatsDB{
		logger:  log.NewNopLogger(),
		storage: s,
	}

	// Test setupDB function
	_, _, err := setupDB(jobstatDBPath, jobstatDBTable, j.logger)
	if err != nil {
		t.Errorf("Failed to prepare DB due to %s", err)
	}
	if _, err := os.Stat(jobstatDBPath); err != nil {
		t.Errorf("Expected DB file not created at %s.", jobstatDBPath)
	}

	// Call setupDB again. This should return with db conn
	_, _, err = setupDB(jobstatDBPath, jobstatDBTable, j.logger)
	if err != nil {
		t.Errorf("Failed to return DB connection on already prepared DB due to %s", err)
	}

	// Check DB file exists
	if _, err := os.Stat(jobstatDBPath); err != nil {
		t.Errorf("DB file not created")
	}
}
