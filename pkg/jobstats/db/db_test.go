package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/schedulers"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/tsdb"
	_ "github.com/mattn/go-sqlite3"
)

type mockScheduler struct{}

var mockJobs = []base.JobStats{{Jobid: 10000}, {Jobid: 10001}}

func newMockScheduler(logger log.Logger) (*schedulers.BatchScheduler, error) {
	return &schedulers.BatchScheduler{Scheduler: &mockScheduler{}}, nil
}

// GetJobs implements collection jobs between start and end times
func (m *mockScheduler) Fetch(start time.Time, end time.Time) ([]base.JobStats, error) {
	return mockJobs, nil
}

func prepareMockConfig(tmpDir string) *Config {
	dataDir := filepath.Join(tmpDir, "data")
	dataBackupDir := filepath.Join(tmpDir, "data-backup")
	jobstatDBPath := filepath.Join(dataDir, "jobstats.db")
	lastJobsUpdateTimeFile := filepath.Join(dataDir, "update")

	// Create data directory
	if err := os.Mkdir(dataDir, 0750); err != nil {
		fmt.Printf("Failed to create data directory: %s", err)
	}
	if err := os.Mkdir(dataBackupDir, 0750); err != nil {
		fmt.Printf("Failed to create data directory: %s", err)
	}

	// Create an empty file for sacct
	sacctFile, err := os.Create(filepath.Join(tmpDir, "sacct"))
	if err != nil {
		fmt.Println(err)
	}
	sacctFile.Close()
	return &Config{
		Logger:                  log.NewNopLogger(),
		JobstatsDBPath:          jobstatDBPath,
		JobstatsDBBackupPath:    dataBackupDir,
		LastUpdateTimeStampFile: lastJobsUpdateTimeFile,
		LastUpdateTimeString:    "2023-12-20",
		RetentionPeriod:         7,
		BatchScheduler:          newMockScheduler,
		TSDB:                    &tsdb.TSDB{},
	}
}

func populateDBWithMockData(j *jobStatsDB) {
	tx, _ := j.db.Begin()
	stmt, err := j.prepareInsertStatement(tx)
	if err != nil {
		fmt.Println(err)
	}
	j.insertJobs(stmt, mockJobs)
	tx.Commit()
}

func TestNewJobStatsDB(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	_, err := NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Check if last update time file has been written
	if _, err := os.Stat(c.LastUpdateTimeStampFile); err != nil {
		t.Errorf("Last update time file not created")
	}

	// Check content of last update time file
	if timeString, _ := os.ReadFile(c.LastUpdateTimeStampFile); string(timeString) != "2023-12-20T00:00:00" {
		t.Errorf("Last update time string test failed. Expected %s got %s", "2023-12-20T00:00:00", string(timeString))
	}

	// Check DB file exists
	if _, err := os.Stat(c.JobstatsDBPath); err != nil {
		t.Errorf("DB file not created")
	}

	// Make again a new jobstats DB with new lastUpdateTime
	c.LastUpdateTimeString = "2023-12-21"
	_, err = NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Check content of last update time file. It should not change
	if timeString, _ := os.ReadFile(c.LastUpdateTimeStampFile); string(timeString) != "2023-12-20T00:00:00" {
		t.Errorf("Last update time string test failed. Expected %s got %s", "2023-12-20T00:00:00", string(timeString))
	}

	// Remove read permissions on lastupdatetimefile
	err = os.Chmod(c.LastUpdateTimeStampFile, 0200)
	if err != nil {
		t.Fatal(err)
	}

	// Make again a new jobstats DB with new lastUpdateTime
	_, err = NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Add back read permissions on lastupdatetimefile
	err = os.Chmod(c.LastUpdateTimeStampFile, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Check content of last update time file. It should change
	if timeString, err := os.ReadFile(c.LastUpdateTimeStampFile); string(timeString) != "2023-12-21T00:00:00" {
		t.Errorf(
			"Last update time string test failed. Expected %s got %s %s",
			"2023-12-21T00:00:00",
			string(timeString),
			err,
		)
	}

	// Remove last update time file
	err = os.Remove(c.LastUpdateTimeStampFile)
	if err != nil {
		t.Fatal(err)
	}

	// Make again a new jobstats DB with new lastUpdateTime
	c.LastUpdateTimeString = "2023-12-22"
	_, err = NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Check content of last update time file. It should change
	if timeString, err := os.ReadFile(c.LastUpdateTimeStampFile); string(timeString) != "2023-12-22T00:00:00" {
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
	c := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(c)
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
	c := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Populate DB with data
	populateDBWithMockData(j)

	// Run vacuum
	err = j.vacuum()
	if err != nil {
		t.Errorf("Failed to vacuum DB due to %s", err)
	}
}

func TestJobStatsDBBackup(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Populate DB with data
	populateDBWithMockData(j)

	// Run backup
	expectedBackupFile := filepath.Join(c.JobstatsDBBackupPath, "backup.db")
	err = j.backup(expectedBackupFile)
	if err != nil {
		t.Errorf("Failed to backup DB %s", err)
	}

	if _, err := os.Stat(expectedBackupFile); err != nil {
		t.Errorf("Backup DB file not found")
	}

	// Check contents of backed up DB
	var numRows int
	db, _, err := openDBConnection(expectedBackupFile)
	if err != nil {
		t.Errorf("Failed to create DB connection to backup DB: %s", err)
	}
	rows, _ := db.Query(fmt.Sprintf("SELECT * FROM %s;", base.JobStatsDBTable))
	for rows.Next() {
		numRows += 1
	}
	if numRows != 2 {
		t.Errorf("Backup DB check failed. Expected rows 2 , Got %d.", numRows)
	}
}

// func TestJobStatsDBBackupFailRetries(t *testing.T) {
// 	tmpDir := t.TempDir()
// 	c := prepareMockConfig(tmpDir)

// 	// Make new jobstats DB
// 	j, err := NewJobStatsDB(c)
// 	if err != nil {
// 		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
// 	}

// 	// Make backup dir non existent
// 	j.storage.dbBackupPath = "non-existent"

// 	// Populate DB with data
// 	populateDBWithMockData(j.db, j)

// 	// Run backup
// 	for i := 0; i < maxBackupRetries; i++ {
// 		j.createBackup()
// 	}
// 	if j.storage.backupRetries != 0 {
// 		t.Errorf("Failed to reset DB backup retries counter. Expected 0, got %d", j.storage.backupRetries)
// 	}

// 	for i := 0; i < maxBackupRetries-1; i++ {
// 		j.createBackup()
// 	}
// 	if j.storage.backupRetries != 0 {
// 		t.Errorf("Failed to increment DB backup retries counter. Expected %d, got %d", maxBackupRetries-1, j.storage.backupRetries)
// 	}
// }

func TestJobStatsDeleteOldJobs(t *testing.T) {
	tmpDir := t.TempDir()
	jobId := 1111
	c := prepareMockConfig(tmpDir)

	// Make new jobstats DB
	j, err := NewJobStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new jobstatsDB struct due to %s", err)
	}

	// Add new row that should be deleted
	jobs := []base.JobStats{
		{
			Jobid: int64(jobId),
			Submit: time.Now().
				Add(time.Duration(-j.storage.retentionPeriod*24*2) * time.Hour).
				Format(base.DatetimeLayout),
		},
	}
	tx, _ := j.db.Begin()
	stmt, err := j.prepareInsertStatement(tx)
	if err != nil {
		t.Errorf("Failed to prepare SQL statements")
	}
	j.insertJobs(stmt, jobs)

	// Now clean up DB for old jobs
	err = j.deleteOldJobs(tx)
	if err != nil {
		t.Errorf("Failed to delete old entries in DB")
	}
	tx.Commit()

	// Query for deleted job
	result, err := j.db.Prepare(
		fmt.Sprintf("SELECT COUNT(Jobid) FROM %s WHERE Jobid = ?;", base.JobStatsDBTable),
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
