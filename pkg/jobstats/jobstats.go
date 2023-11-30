package jobstats

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/utils"
)

var (
	dateFormat = "2006-01-02T15:04:05"
	checksMap  = map[string]interface{}{
		"slurm": slurmChecks,
	}
	statsMap = map[string]interface{}{
		"slurm": getSlurmJobs,
	}
)

func NewJobStats(
	logger log.Logger,
	batchScheduler string,
	jobstatDBPath string,
	jobstatDBTable string,
	retentionPeriod int,
	jobsLastTimeStampFile string,
	vacuumLastTimeStampFile string,
) *jobStats {
	return &jobStats{
		logger:                  logger,
		batchScheduler:          batchScheduler,
		jobstatDBPath:           jobstatDBPath,
		jobstatDBTable:          jobstatDBTable,
		retentionPeriod:         retentionPeriod,
		jobsLastTimeStampFile:   jobsLastTimeStampFile,
		vacuumLastTimeStampFile: vacuumLastTimeStampFile,
	}
}

// Do preliminary checks
func (j *jobStats) checks() {
	if checksFunc, ok := checksMap[j.batchScheduler]; ok {
		checksFunc.(func(log.Logger))(j.logger)
	}
}

// Open DB connection and return connection poiner
func (j *jobStats) openDBConnection() (*sql.DB, error) {
	dbConn, err := sql.Open("sqlite", j.jobstatDBPath)
	if err != nil {
		return nil, err
	}
	return dbConn, nil
}

// Set all necessary PRAGMA statement on DB
func (j *jobStats) setPragmaDirectives(db *sql.DB) {
	// Ref: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	// Ref: https://gitlab.com/gnufred/logslate/-/blob/8eda5cedc9a28da3793dcf73480d618c95cc322c/playground/sqlite3.go
	// Ref: https://github.com/mattn/go-sqlite3/issues/1145#issuecomment-1519012055
	_, err := db.Exec("PRAGMA synchronous = OFF")
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to set synchronous to OFF", "err", err)
	}
	_, err = db.Exec("PRAGMA journal_mode = MEMORY")
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to set journal_mode to MEMORY", "err", err)
	}
}

// Check if DB is locked
func (j *jobStats) checkDBLock(db *sql.DB) error {
	_, err := db.Exec("BEGIN EXCLUSIVE TRANSACTION;")
	if err != nil {
		return err
	}
	_, err = db.Exec("COMMIT;")
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to commit exclusive transcation", "err", err)
		return err
	}
	return nil
}

// Prepare DB and create table
func (j *jobStats) prepareDB() (*sql.DB, error) {
	if _, err := os.Stat(j.jobstatDBPath); err == nil {
		// Open the created SQLite File
		db, err := j.openDBConnection()
		if err != nil {
			level.Error(j.logger).Log("msg", "Failed to open DB file", "err", err)
			return nil, err
		}
		return db, nil
	}

	// If file does not exist, create SQLite file
	file, err := os.Create(j.jobstatDBPath)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to create DB file", "err", err)
		return nil, err
	}
	file.Close()

	// Open the created SQLite File
	db, err := j.openDBConnection()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to open DB file", "err", err)
		return nil, err
	}

	// Create Table
	err = j.createTable(db)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to create DB table", "err", err)
		return nil, err
	}
	return db, nil
}

// Create a table for storing job stats
func (j *jobStats) createTable(db *sql.DB) error {
	allFields := utils.GetStructFieldName(BatchJob{})
	fieldLines := []string{}
	for _, field := range allFields {
		fieldLines = append(fieldLines, fmt.Sprintf("		\"%s\" TEXT,", field))
	}
	fieldLines[len(fieldLines)-1] = strings.Split(fieldLines[len(fieldLines)-1], ",")[0]
	createBatchJobStatSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		"id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,		
		%s
	  );`, j.jobstatDBTable, strings.Join(fieldLines, "\n"))

	// Prepare SQL DB creation Statement
	level.Info(j.logger).Log("msg", "Creating SQLite DB table for storing job stats")
	statement, err := db.Prepare(createBatchJobStatSQL)
	if err != nil {
		level.Error(j.logger).
			Log("msg", "Failed to prepare SQL statement duirng DB creation", "err", err)
		return err
	}

	// Execute SQL DB creation Statements
	_, err = statement.Exec()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to execute SQL command creation statement", "err", err)
		return err
	}

	// Prepare SQL DB index creation Statement
	level.Info(j.logger).Log("msg", "Creating DB index with Usr,Account,Start columns")
	createIndexSQL := fmt.Sprintf(`CREATE INDEX i1 ON %s (Usr,Account,Start);`, j.jobstatDBTable)
	statement, err = db.Prepare(createIndexSQL)
	if err != nil {
		level.Error(j.logger).
			Log("msg", "Failed to prepare SQL statement for index creation", "err", err)
		return err
	}

	// Execute SQL DB index creation Statements
	_, err = statement.Exec()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to execute SQL command for index creation statement", "err", err)
		return err
	}
	level.Info(j.logger).Log("msg", "SQLite DB table for jobstats successfully created")
	return nil
}

// Make and return prepare statement for inserting entries
func (j *jobStats) getSQLPrepareStatement(tx *sql.Tx) (*sql.Stmt, error) {
	allFields := utils.GetStructFieldName(BatchJob{})
	fieldNamesString := strings.Join(allFields[:], ", ")
	placeholderSlice := make([]string, len(allFields))
	for i := range placeholderSlice {
		placeholderSlice[i] = "?"
	}
	placeholderString := strings.Join(placeholderSlice, ", ")
	insertSQLPlaceholder := fmt.Sprintf(
		"INSERT INTO %s(%s) VALUES (%s)",
		j.jobstatDBTable,
		fieldNamesString,
		placeholderString,
	)
	stmt, err := tx.Prepare(insertSQLPlaceholder)
	if err != nil {
		return nil, err
	}
	return stmt, nil
}

// Get start and end time to query for jobs
func (j *jobStats) getStartEndTimes() (time.Time, time.Time) {
	var startTime time.Time
	var foundStartTime bool = false
	if _, err := os.Stat(j.jobsLastTimeStampFile); err == nil {
		timestamp, err := os.ReadFile(j.jobsLastTimeStampFile)
		if err != nil {
			level.Error(j.logger).Log("msg", "Failed to read jobs lasttimestamp file", "err", err)
		} else {
			startTime, err = time.Parse(dateFormat, string(timestamp))
			if err != nil {
				level.Error(j.logger).Log("msg", "Failed to parse jobs lasttimestamp file", "err", err)
			} else {
				foundStartTime = true
			}
		}
	}
	if !foundStartTime {
		// If not lasttimestamp file found, get data from rententionPeriod number of days
		startTime = time.Now().Add(time.Duration(-j.retentionPeriod*24) * time.Hour)
	}
	endTime := time.Now()
	return startTime, endTime
}

// Write endTime to a file
func (j *jobStats) writeLastTimeStampFile(lastTimeStampFile string, endTime time.Time) {
	lastTimeStamp := []byte(endTime.Format(dateFormat))
	err := os.WriteFile(lastTimeStampFile, lastTimeStamp, 0644)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to write lasttimestamp file", "err", err)
	}
}

// Get job stats and insert them into DB
func (j *jobStats) GetJobStats() error {
	// First do basic checks
	j.checks()
	// Get start and end times for retrieving jobs
	startTime, endTime := j.getStartEndTimes()
	var jobs []BatchJob
	var err error
	if statsFunc, ok := statsMap[j.batchScheduler]; ok {
		jobs, err = statsFunc.(func(time.Time, time.Time, log.Logger) ([]BatchJob, error))(
			startTime,
			endTime,
			j.logger,
		)
		if err != nil {
			return err
		}
	}

	// Prepare DB and return db object
	db, err := j.prepareDB()
	if err != nil {
		level.Error(j.logger).Log("msg", "Preparation of DB failed", "err", err)
		return err
	}

	// Vacuum DB every Monday after 02h:00 (Sunday after midnight)
	err = j.vacuumDB(db)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to vacuum DB", "err", err)
	}

	// Check if DB is already locked.
	// If locked, return with noop
	err = j.checkDBLock(db)
	if err != nil {
		level.Error(j.logger).Log("msg", "DB is locked. Jobs WILL NOT BE inserted.", "err", err)
		return err
	}

	// Begin transcation
	tx, err := db.Begin()
	if err != nil {
		level.Error(j.logger).Log("msg", "Could not start transcation", "err", err)
		return err
	}

	// Set pragma statements
	j.setPragmaDirectives(db)
	stmt, err := j.getSQLPrepareStatement(tx)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to prepare insert job statement in DB", "err", err)
		return err
	}

	// Insert data into DB
	level.Info(j.logger).Log("msg", "Inserting jobs into DB")
	j.insertJobsInDB(stmt, jobs)
	level.Info(j.logger).Log("msg", "Finished inserting jobs into DB")

	// Delete older entries
	level.Info(j.logger).Log("msg", "Deleting old jobs")
	err = j.deleteOldJobs(tx)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to delete old job entries", "err", err)
		return err
	}
	level.Info(j.logger).Log("msg", "Finished deleting old jobs in DB")

	// Commit changes
	err = tx.Commit()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to commit DB transcation", "err", err)
		return err
	}

	// Write endTime to file to keep track upon successful insertion of data
	j.writeLastTimeStampFile(j.jobsLastTimeStampFile, endTime)

	// Defer Closing the database
	defer stmt.Close()
	defer db.Close()
	return nil
}

// Vacuum DB to reduce fragementation and size
func (j *jobStats) vacuumDB(db *sql.DB) error {
	weekday := time.Now().Weekday().String()
	hours, _, _ := time.Now().Clock()
	var nextVacuumTime time.Time
	// Check if lasttimestamp for vacuum exists
	if _, err := os.Stat(j.vacuumLastTimeStampFile); err == nil {
		timestamp, err := os.ReadFile(j.vacuumLastTimeStampFile)
		if err != nil {
			level.Error(j.logger).Log("msg", "Failed to read vacuum lasttimestamp file", "err", err)
			nextVacuumTime = time.Now()
		} else {
			lastVacuumTime, err := time.Parse(dateFormat, string(timestamp))
			if err != nil {
				level.Error(j.logger).Log("msg", "Failed to parse vacuum lasttimestamp file", "err", err)
			}
			nextVacuumTime = lastVacuumTime.Add(time.Duration(168) * time.Hour)
		}
	} else {
		nextVacuumTime = time.Now()
	}

	// Check if we are on Monday at 02hr and **after** nextVacuumTime
	if weekday != "Monday" && hours != 2 && time.Now().Compare(nextVacuumTime) == -1 {
		return nil
	}

	// Start vacuuming
	level.Info(j.logger).Log("msg", "Starting to vacuum DB")
	_, err := db.Exec("VACUUM;")
	if err != nil {
		return err
	}
	level.Info(j.logger).Log("msg", "DB vacuum successfully finished")

	// Write endTime to file to keep track upon successful insertion of data
	j.writeLastTimeStampFile(j.vacuumLastTimeStampFile, time.Now())
	return nil
}

// Delete old entries in DB
func (j *jobStats) deleteOldJobs(tx *sql.Tx) error {
	deleteSQLCmd := fmt.Sprintf(
		"DELETE FROM %s WHERE Start <= date('now', '-%d day')",
		j.jobstatDBTable,
		j.retentionPeriod,
	)
	_, err := tx.Exec(deleteSQLCmd)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to delete old jobs", "err", err)
		return err
	}
	return nil
}

// Insert job stat into DB
func (j *jobStats) insertJobsInDB(statement *sql.Stmt, jobStats []BatchJob) {
	for _, jobStat := range jobStats {
		// Empty job
		if jobStat == (BatchJob{}) {
			continue
		}
		level.Debug(j.logger).Log("msg", "Inserting job", "jobid", jobStat.Jobid)
		_, err := statement.Exec(
			jobStat.Jobid, jobStat.Jobuuid,
			jobStat.Partition, jobStat.Account, jobStat.Grp, jobStat.Gid,
			jobStat.Usr, jobStat.Uid, jobStat.Submit, jobStat.Start,
			jobStat.End, jobStat.Elapsed, jobStat.Exitcode,
			jobStat.State, jobStat.Nnodes, jobStat.Nodelist,
			jobStat.NodelistExp,
		)
		if err != nil {
			level.Error(j.logger).
				Log("msg", "Failed to insert job in DB", "jobid", jobStat.Jobid, "err", err)
		}
	}
}
