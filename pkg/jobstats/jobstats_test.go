package jobstats

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	_ "modernc.org/sqlite"
)

func populateDBWithMockData(db *sql.DB, j *jobStats) {
	jobs := []BatchJob{{Jobid: "10000"}, {Jobid: "10001"}}
	tx, _ := db.Begin()
	stmt, _ := j.getSQLPrepareStatement(tx)
	j.insertJobsInDB(stmt, jobs)
	tx.Commit()
}

func TestJobStatsDBPreparation(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBTable := "jobstats"
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	j := jobStats{
		logger:         log.NewNopLogger(),
		batchScheduler: "slurm",
		jobstatDBPath:  jobstatDBPath,
		jobstatDBTable: jobstatDBTable,
	}
	db, err := j.prepareDB()
	if err != nil {
		t.Errorf("Failed to prepare DB due to %s", err)
	}
	if _, err := os.Stat(jobstatDBPath); err != nil {
		t.Errorf("Expected DB file not created at %s.", jobstatDBPath)
	}
	populateDBWithMockData(db, &j)
	var numRows int = 0
	rows, _ := db.Query(fmt.Sprintf("SELECT * FROM %s;", jobstatDBTable))
	for rows.Next() {
		numRows += 1
	}
	if numRows != 2 {
		t.Errorf("DB data insertion failed. Expected rows 2 , Got %d.", numRows)
	}
}

func TestJobStatsDBLock(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBTable := "jobstats"
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	j := jobStats{
		logger:         log.NewNopLogger(),
		batchScheduler: "slurm",
		jobstatDBPath:  jobstatDBPath,
		jobstatDBTable: jobstatDBTable,
	}
	db, err := j.prepareDB()
	if err != nil {
		t.Errorf("Failed to prepare DB")
	}
	_, err = db.Exec("BEGIN EXCLUSIVE;")
	if err != nil {
		t.Errorf("Failed to lock DB due to %s", err)
	}
	err = j.GetJobStats()
	if err == nil {
		t.Errorf("Failed to skip data insertion when DB is locked")
	}
	db.Exec("COMMIT;")
}

func TestJobStatsDBVacuum(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBTable := "jobstats"
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	j := jobStats{
		logger:         log.NewNopLogger(),
		batchScheduler: "slurm",
		jobstatDBPath:  jobstatDBPath,
		jobstatDBTable: jobstatDBTable,
	}
	db, err := j.prepareDB()
	if err != nil {
		t.Errorf("Failed to prepare DB")
	}

	// Populate DB with data
	populateDBWithMockData(db, &j)

	// Run vacuum
	err = j.vacuumDB(db)
	if err != nil {
		t.Errorf("Failed to vacuum DB")
	}
}

func TestJobStatsDeleteOldJobs(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBTable := "jobstats"
	jobId := "1111"
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	j := jobStats{
		logger:          log.NewNopLogger(),
		batchScheduler:  "slurm",
		jobstatDBPath:   jobstatDBPath,
		jobstatDBTable:  jobstatDBTable,
		retentionPeriod: 1,
	}
	db, err := j.prepareDB()
	if err != nil {
		t.Errorf("Failed to prepare DB")
	}

	// Add new row that should be deleted
	jobs := []BatchJob{
		{
			Jobid: jobId,
			Submit: time.Now().
				Add(time.Duration(-j.retentionPeriod*24*2) * time.Hour).
				Format(dateFormat),
		},
	}
	tx, _ := db.Begin()
	stmt, err := j.getSQLPrepareStatement(tx)
	if err != nil {
		t.Errorf("Failed to prepare SQL statements")
	}
	j.insertJobsInDB(stmt, jobs)

	// Now clean up DB for old jobs
	err = j.deleteOldJobs(tx)
	if err != nil {
		t.Errorf("Failed to delete old entries in DB")
	}
	tx.Commit()

	// Query for deleted job
	result, err := db.Prepare(
		fmt.Sprintf("SELECT COUNT(Jobid) FROM %s WHERE Jobid = ?;", jobstatDBTable),
	)
	if err != nil {
		t.Errorf("Failed to prepare SQL statement")
	}
	var numRows string
	err = result.QueryRow(jobId).Scan(&numRows)
	if err != nil {
		t.Errorf("Failed to get query result due to %s.", err)
	}
	if numRows != "0" {
		t.Errorf("Deleting old jobs failed. Expected 0 rows. Returned %s", numRows)
	}
}
