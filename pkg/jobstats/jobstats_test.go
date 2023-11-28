package jobstats

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	_ "modernc.org/sqlite"
)

func TestJobStatsDBPreparation(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	j := jobStats{logger: log.NewNopLogger(), batchScheduler: "slurm", jobstatDBPath: jobstatDBPath}
	dbConn, err := j.prepareDB()
	if err != nil {
		t.Errorf("Failed to prepare DB due to %s", err)
	}
	if _, err := os.Stat(jobstatDBPath); err != nil {
		t.Errorf("Expected DB file not created at %s.", jobstatDBPath)
	}
	_, err = dbConn.Query("SELECT * FROM jobs;")
	if err != nil {
		t.Errorf("Failed to create table in DB file at %s.", jobstatDBPath)
	}
}
