package jobstats

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	_ "github.com/mattn/go-sqlite3"
)

var (
	dateFormat = "2006-01-02T15:04:05"
	// Ref: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	// Ref: https://gitlab.com/gnufred/logslate/-/blob/8eda5cedc9a28da3793dcf73480d618c95cc322c/playground/sqlite3.go
	// Ref: https://github.com/mattn/go-sqlite3/issues/1145#issuecomment-1519012055
	pragmaStatements = []string{
		"PRAGMA synchronous = OFF",
		"PRAGMA journal_mode = MEMORY",
	}
	indexStatements = []string{
		`CREATE INDEX IF NOT EXISTS i1 ON %s (Usr,Account,Start);`,
		`CREATE INDEX IF NOT EXISTS i2 ON %s (Usr,Jobuuid);`,
	}
	allFields = GetStructFieldName(BatchJob{})
)

// Write timestamp to a file
func writeTimeStampToFile(filePath string, timeStamp time.Time, logger log.Logger) {
	timeStampString := timeStamp.Format(dateFormat)
	timeStampByte := []byte(timeStampString)
	err := os.WriteFile(filePath, timeStampByte, 0644)
	if err != nil {
		level.Error(logger).
			Log("msg", "Failed to write timestamp to file", "time", timeStampString, "file", filePath, "err", err)
	}
}

// Create a table for storing job stats
func createTable(dbTableName string, db *sql.DB, logger log.Logger) error {
	fieldLines := []string{}
	for _, field := range allFields {
		fieldLines = append(fieldLines, fmt.Sprintf("		\"%s\" TEXT,", field))
	}
	fieldLines[len(fieldLines)-1] = strings.Split(fieldLines[len(fieldLines)-1], ",")[0]
	createBatchJobStatSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		"id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,		
		%s
	  );`, dbTableName, strings.Join(fieldLines, "\n"))

	// Prepare SQL DB creation Statement
	level.Info(logger).Log("msg", "Creating SQLite DB table for storing job stats")
	statement, err := db.Prepare(createBatchJobStatSQL)
	if err != nil {
		level.Error(logger).
			Log("msg", "Failed to prepare SQL statement duirng DB creation", "err", err)
		return err
	}

	// Execute SQL DB creation Statements
	_, err = statement.Exec()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to execute SQL command creation statement", "err", err)
		return err
	}

	// Prepare SQL DB index creation Statement
	for _, stmt := range indexStatements {
		level.Info(logger).Log("msg", "Creating DB index with Usr,Account,Start columns")
		createIndexSQL := fmt.Sprintf(stmt, dbTableName)
		statement, err = db.Prepare(createIndexSQL)
		if err != nil {
			level.Error(logger).
				Log("msg", "Failed to prepare SQL statement for index creation", "err", err)
			return err
		}

		// Execute SQL DB index creation Statements
		_, err = statement.Exec()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to execute SQL command for index creation statement", "err", err)
			return err
		}
	}
	level.Info(logger).Log("msg", "SQLite DB table for jobstats successfully created")
	return nil
}

// Open DB connection and return connection poiner
func openDBConnection(dbFilePath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbFilePath)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Setup DB and create table
func setupDB(dbFilePath string, dbTableName string, logger log.Logger) (*sql.DB, error) {
	if _, err := os.Stat(dbFilePath); err == nil {
		// Open the created SQLite File
		db, err := openDBConnection(dbFilePath)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to open DB file", "err", err)
			return nil, err
		}
		return db, nil
	}

	// If file does not exist, create SQLite file
	file, err := os.Create(dbFilePath)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create DB file", "err", err)
		return nil, err
	}
	file.Close()

	// Open the created SQLite File
	db, err := openDBConnection(dbFilePath)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to open DB file", "err", err)
		return nil, err
	}

	// Create Table
	err = createTable(dbTableName, db, logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create DB table", "err", err)
		return nil, err
	}
	return db, nil
}

// Make new JobStatsDB struct
func NewJobStatsDB(
	logger log.Logger,
	jobstatDBPath string,
	jobstatDBTable string,
	retentionPeriod int,
	lastJobsUpdateTimeString string,
	lastJobsUpdateTimeFile string,
	newBatchScheduler func(log.Logger) (*BatchScheduler, error),
) (*jobStatsDB, error) {
	// Emit debug logs
	level.Debug(logger).Log(
		"msg", "DB config:",
		"db.path", jobstatDBPath,
		"db.table", jobstatDBTable,
		"data.retention.period", retentionPeriod,
		"data.last.update.time", lastJobsUpdateTimeString,
		"data.lasttimestamp.file", lastJobsUpdateTimeFile,
	)

	// Check if data path exists and attempt to create if it does not exist
	var lastJobsUpdateTime time.Time
	var err error
	dataPath := filepath.Dir(jobstatDBPath)
	if _, err := os.Stat(dataPath); err != nil {
		level.Info(logger).Log("msg", "Data path directory does not exist. Creating...", "path", dataPath)
		if err := os.Mkdir(dataPath, 0750); err != nil {
			level.Error(logger).Log("msg", "Could not create data path directory", "path", dataPath, "err", err)
			return nil, err
		}
		goto updatetime
	} else {
		// If data directory exists, try to read lastJobsUpdateTimeFile
		if _, err := os.Stat(lastJobsUpdateTimeFile); err == nil {
			lastUpdateTimeString, err := os.ReadFile(lastJobsUpdateTimeFile)
			if err != nil {
				level.Error(logger).Log("msg", "Failed to read lastjobsupdatetime file", "err", err)
				goto updatetime
			} else {
				// Trim any spaces and new lines
				lastJobsUpdateTime, err = time.Parse(dateFormat, strings.TrimSuffix(strings.TrimSpace(string(lastUpdateTimeString)), "\n"))
				if err != nil {
					level.Error(logger).Log("msg", "Failed to parse time string in lastjobsupdatetime file", "time", lastUpdateTimeString, "err", err)
					goto updatetime
				}
			}
			goto setup
		} else {
			goto updatetime
		}
	}

updatetime:
	lastJobsUpdateTime, err = time.Parse("2006-01-02", lastJobsUpdateTimeString)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to parse time string", "time", lastJobsUpdateTimeString, "err", err)
		return nil, err
	}

	// Write to file for persistence in case of restarts
	writeTimeStampToFile(lastJobsUpdateTimeFile, lastJobsUpdateTime, logger)

setup:
	// Setup scheduler struct that retrieves job data
	scheduler, err := newBatchScheduler(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Batch scheduler setup failed", "err", err)
		return nil, err
	}

	// Setup DB
	db, err := setupDB(jobstatDBPath, jobstatDBTable, logger)
	if err != nil {
		level.Error(logger).Log("msg", "DB setup failed", "err", err)
		return nil, err
	}
	return &jobStatsDB{
		logger:                 logger,
		db:                     db,
		scheduler:              scheduler,
		jobstatDBPath:          jobstatDBPath,
		jobstatDBTable:         jobstatDBTable,
		retentionPeriod:        retentionPeriod,
		lastJobsUpdateTime:     lastJobsUpdateTime,
		lastDBVacuumTime:       time.Now(),
		lastJobsUpdateTimeFile: lastJobsUpdateTimeFile,
	}, nil
}

// Set all necessary PRAGMA statement on DB
func (j *jobStatsDB) setPragmaDirectives() {
	for _, stmt := range pragmaStatements {
		_, err := j.db.Exec(stmt)
		if err != nil {
			level.Error(j.logger).Log("msg", "Failed to execute pragma statement", "statement", stmt, "err", err)
		}
	}
}

// Check if DB is locked
func (j *jobStatsDB) checkDBLock() error {
	_, err := j.db.Exec("BEGIN EXCLUSIVE TRANSACTION;")
	if err != nil {
		return err
	}
	_, err = j.db.Exec("COMMIT;")
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to commit exclusive transcation", "err", err)
		return err
	}
	return nil
}

// Make and return prepare statement for inserting entries
func (j *jobStatsDB) prepareInsertStatement(tx *sql.Tx, numJobs int) (*sql.Stmt, error) {
	placeHolderString := fmt.Sprintf(
		"(%s)",
		strings.Join(strings.Split(strings.Repeat("?", len(allFields)), ""), ","),
	)
	fieldNamesString := strings.Join(allFields, ",")
	insertStatement := fmt.Sprintf(
		"INSERT INTO %s(%s) VALUES %s",
		j.jobstatDBTable,
		fieldNamesString,
		placeHolderString,
	)
	stmt, err := tx.Prepare(insertStatement)
	if err != nil {
		return nil, err
	}
	return stmt, nil
}

// Collect job stats
func (j *jobStatsDB) Collect() error {
	var currentTime = time.Now()

	// If duration is less than 1 day do single update
	if currentTime.Sub(j.lastJobsUpdateTime) < time.Duration(24*time.Hour) {
		return j.getJobStats(j.lastJobsUpdateTime, currentTime)
	}
	level.Info(j.logger).
		Log("msg", "DB update duration is more than 1 day. Doing incremental update. This may take a while...")

	// If duration is more than 1 day, do update for each day
	var nextUpdateTime time.Time
	for {
		nextUpdateTime = j.lastJobsUpdateTime.Add(24 * time.Hour)
		if nextUpdateTime.Compare(currentTime) == -1 {
			level.Debug(j.logger).
				Log("msg", "Incremental DB update step", "from", j.lastJobsUpdateTime, "to", nextUpdateTime)
			if err := j.getJobStats(j.lastJobsUpdateTime, nextUpdateTime); err != nil {
				level.Error(j.logger).
					Log("msg", "Failed incremental update", "from", j.lastJobsUpdateTime, "to", nextUpdateTime, "err", err)
				return err
			}
		} else {
			level.Debug(j.logger).Log("msg", "Final incremental DB update step", "from", j.lastJobsUpdateTime, "to", currentTime)
			return j.getJobStats(j.lastJobsUpdateTime, currentTime)
		}

		// Sleep for couple of seconds before making next update
		// This is to avoid let DB breath a bit before serving next request
		time.Sleep(2 * time.Second)
	}
}

// Get job stats and insert them into DB
func (j *jobStatsDB) getJobStats(startTime, endTime time.Time) error {
	// Retrieve jobs from unerlying batch scheduler
	jobs, err := j.scheduler.GetJobs(startTime, endTime)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to retrieve jobs from batch scheduler", "err", err)
		return err
	}

	// Vacuum DB every Monday after 02h:00 (Sunday after midnight)
	err = j.vacuumDB()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to vacuum DB", "err", err)
	}

	// Check if DB is already locked.
	// If locked, return with noop
	err = j.checkDBLock()
	if err != nil {
		level.Error(j.logger).Log("msg", "DB is locked. Jobs WILL NOT BE inserted.", "err", err)
		return err
	}

	// Begin transcation
	tx, err := j.db.Begin()
	if err != nil {
		level.Error(j.logger).Log("msg", "Could not start transcation", "err", err)
		return err
	}

	// Set pragma statements
	j.setPragmaDirectives()
	stmt, err := j.prepareInsertStatement(tx, len(jobs))
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to prepare insert job statement in DB", "err", err)
		return err
	}

	// Insert data into DB
	level.Debug(j.logger).Log("msg", "Inserting jobs into DB")
	j.insertJobsInDB(stmt, jobs)
	level.Debug(j.logger).Log("msg", "Finished inserting jobs into DB")

	// Delete older entries
	level.Debug(j.logger).Log("msg", "Deleting old jobs")
	err = j.deleteOldJobs(tx)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to delete old job entries", "err", err)
		return err
	}
	level.Debug(j.logger).Log("msg", "Finished deleting old jobs in DB")

	// Commit changes
	err = tx.Commit()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to commit DB transcation", "err", err)
		return err
	}

	// Write endTime to file to keep track upon successful insertion of data
	j.lastJobsUpdateTime = endTime
	writeTimeStampToFile(j.lastJobsUpdateTimeFile, j.lastJobsUpdateTime, j.logger)

	// Defer Closing the database
	defer stmt.Close()
	return nil
}

// Close DB connection
func (j *jobStatsDB) Stop() error {
	return j.db.Close()
}

// Vacuum DB to reduce fragementation and size
func (j *jobStatsDB) vacuumDB() error {
	weekday := time.Now().Weekday().String()
	hours, _, _ := time.Now().Clock()

	// Next vacuum time is 7 days after last vacuum
	nextVacuumTime := j.lastDBVacuumTime.Add(time.Duration(168) * time.Hour)

	// Check if we are on Monday at 02hr and **after** nextVacuumTime
	if weekday != "Monday" || hours != 02 || time.Now().Compare(nextVacuumTime) == -1 {
		return nil
	}

	// Start vacuuming
	level.Info(j.logger).Log("msg", "Starting to vacuum DB")
	_, err := j.db.Exec("VACUUM;")
	if err != nil {
		return err
	}
	level.Info(j.logger).Log("msg", "DB vacuum successfully finished")
	j.lastDBVacuumTime = time.Now()
	return nil
}

// Delete old entries in DB
func (j *jobStatsDB) deleteOldJobs(tx *sql.Tx) error {
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
func (j *jobStatsDB) insertJobsInDB(statement *sql.Stmt, jobStats []BatchJob) {
	for _, jobStat := range jobStats {
		// Empty job
		if jobStat == (BatchJob{}) {
			continue
		}
		// level.Debug(j.logger).Log("msg", "Inserting job", "jobid", jobStat.Jobid)
		_, err := statement.Exec(
			jobStat.Jobid,
			jobStat.Jobuuid,
			jobStat.Partition,
			jobStat.Account,
			jobStat.Grp,
			jobStat.Gid,
			jobStat.Usr,
			jobStat.Uid,
			jobStat.Submit,
			jobStat.Start,
			jobStat.End,
			jobStat.Elapsed,
			jobStat.Exitcode,
			jobStat.State,
			jobStat.Nnodes,
			jobStat.Nodelist,
			jobStat.NodelistExp,
		)
		if err != nil {
			level.Error(j.logger).
				Log("msg", "Failed to insert job in DB", "jobid", jobStat.Jobid, "err", err)
		}
	}
}
