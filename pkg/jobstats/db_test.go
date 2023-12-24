package jobstats

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	_ "github.com/mattn/go-sqlite3"
)

func prepareMockConfig(tmpDir string) (string, string, string, string) {
	dataDir := filepath.Join(tmpDir, "data")
	jobstatDBTable := "jobstats"
	jobstatDBPath := filepath.Join(dataDir, "jobstats.db")
	lastJobsUpdateTimeFile := filepath.Join(dataDir, "update")

	// Create an empty file for sacct
	sacctFile, err := os.Create(filepath.Join(tmpDir, "sacct"))
	if err != nil {
		fmt.Println(err)
	}
	sacctFile.Close()

	*sacctPath = filepath.Join(tmpDir, "sacct")
	return dataDir, jobstatDBPath, jobstatDBTable, lastJobsUpdateTimeFile
}

func populateDBWithMockData(db *sql.DB, j *jobStatsDB) {
	jobs := []BatchJob{{Jobid: "10000"}, {Jobid: "10001"}}
	tx, _ := db.Begin()
	stmt, _ := j.prepareInsertStatement(tx, len(jobs))
	j.insertJobsInDB(stmt, jobs)
	tx.Commit()
}

func TestJobStatsDBPreparation(t *testing.T) {
	tmpDir := t.TempDir()
	jobstatDBTable := "jobstats"
	jobstatDBPath := filepath.Join(tmpDir, "jobstats.db")
	j := jobStatsDB{
		logger:         log.NewNopLogger(),
		batchScheduler: "slurm",
		jobstatDBPath:  jobstatDBPath,
		jobstatDBTable: jobstatDBTable,
	}

	// Test setupDB function
	db, err := setupDB(jobstatDBPath, jobstatDBTable, logger)
	if err != nil {
		t.Errorf("Failed to prepare DB due to %s", err)
	}
	if _, err := os.Stat(jobstatDBPath); err != nil {
		t.Errorf("Expected DB file not created at %s.", jobstatDBPath)
	}

	// Call setupDB again. This should return with db conn
	_, err = setupDB(jobstatDBPath, jobstatDBTable, logger)
	if err != nil {
		t.Errorf("Failed to return DB connection on already prepared DB due to %s", err)
	}

	// Populate DB with mock data
	populateDBWithMockData(db, &j)
	var numRows int = 0

	// Run query
	rows, _ := db.Query(fmt.Sprintf("SELECT * FROM %s;", jobstatDBTable))
	for rows.Next() {
		numRows += 1
	}
	if numRows != 2 {
		t.Errorf("DB data insertion failed. Expected rows 2 , Got %d.", numRows)
	}
}

func TestNewJobStatsDB(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir, jobstatDBPath, jobstatDBTable, lastJobsUpdateTimeFile := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	_, err := NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-20",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Check if data directory created
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("Data directory not created")
	}

	// Check if last update time file has been written
	if _, err := os.Stat(lastJobsUpdateTimeFile); err != nil {
		t.Errorf("Last update time file not created")
	}

	// Check content of last update time file
	if timeString, _ := os.ReadFile(lastJobsUpdateTimeFile); string(timeString) != "2023-12-20T00:00:00" {
		t.Errorf("Last update time string test failed. Expected %s got %s", "2023-12-20T00:00:00", string(timeString))
	}

	// Check DB file exists
	if _, err := os.Stat(jobstatDBPath); err != nil {
		t.Errorf("DB file not created")
	}

	// Make again a new jobstats DB with new lastUpdateTime
	_, err = NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-21",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Check content of last update time file. It should not change
	if timeString, _ := os.ReadFile(lastJobsUpdateTimeFile); string(timeString) != "2023-12-20T00:00:00" {
		t.Errorf("Last update time string test failed. Expected %s got %s", "2023-12-20T00:00:00", string(timeString))
	}

	// Remove read permissions on lastupdatetimefile
	err = os.Chmod(lastJobsUpdateTimeFile, 0200)
	if err != nil {
		t.Fatal(err)
	}

	// Make again a new jobstats DB with new lastUpdateTime
	_, err = NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-21",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Add back read permissions on lastupdatetimefile
	err = os.Chmod(lastJobsUpdateTimeFile, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Check content of last update time file. It should change
	if timeString, err := os.ReadFile(lastJobsUpdateTimeFile); string(timeString) != "2023-12-21T00:00:00" {
		t.Errorf(
			"Last update time string test failed. Expected %s got %s %s",
			"2023-12-21T00:00:00",
			string(timeString),
			err,
		)
	}

	// Remove last update time file
	err = os.Remove(lastJobsUpdateTimeFile)
	if err != nil {
		t.Fatal(err)
	}

	// Make again a new jobstats DB with new lastUpdateTime
	_, err = NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-22",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Check content of last update time file. It should change
	if timeString, err := os.ReadFile(lastJobsUpdateTimeFile); string(timeString) != "2023-12-22T00:00:00" {
		t.Errorf(
			"Last update time string test failed. Expected %s got %s %s",
			"2023-12-22T00:00:00",
			string(timeString),
			err,
		)
	}
}

func TestJobStatsDBLock(t *testing.T) {
	tmpDir := t.TempDir()
	_, jobstatDBPath, jobstatDBTable, lastJobsUpdateTimeFile := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-20",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Beging exclusive transcation to lock DB
	_, err = j.db.Exec("BEGIN EXCLUSIVE;")
	if err != nil {
		t.Errorf("Failed to lock DB due to %s", err)
	}

	// Try to insert data. It should fail
	err = j.Collect()
	if err == nil {
		t.Errorf("Failed to skip data insertion when DB is locked")
	}
	j.db.Exec("COMMIT;")

	// Close DB
	j.Stop()
}

func TestJobStatsDBVacuum(t *testing.T) {
	tmpDir := t.TempDir()
	_, jobstatDBPath, jobstatDBTable, lastJobsUpdateTimeFile := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-20",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Populate DB with data
	populateDBWithMockData(j.db, j)

	// Run vacuum
	err = j.vacuumDB()
	if err != nil {
		t.Errorf("Failed to vacuum DB")
	}
}

func TestJobStatsDeleteOldJobs(t *testing.T) {
	tmpDir := t.TempDir()
	jobId := "1111"
	_, jobstatDBPath, jobstatDBTable, lastJobsUpdateTimeFile := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(
		log.NewNopLogger(),
		"slurm",
		jobstatDBPath,
		jobstatDBTable,
		7,
		"2023-12-20",
		lastJobsUpdateTimeFile,
	)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
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
	tx, _ := j.db.Begin()
	stmt, err := j.prepareInsertStatement(tx, len(jobs))
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
	result, err := j.db.Prepare(
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
