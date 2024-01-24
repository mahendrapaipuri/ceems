package db

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/schedulers"
	"github.com/mattn/go-sqlite3"
	"github.com/rotationalio/ensign/pkg/utils/sqlite"
)

// Unique job labels to identify time series that needed to be deleted
type tsdbLabels struct {
	id      string
	user    string
	account string
}

// TSDB related config
type tsdbConfig struct {
	url                  *url.URL
	cleanUp              bool
	deleteSeriesEndpoint *url.URL
	client               *http.Client
}

// DB config
type Config struct {
	Logger                  log.Logger
	JobstatsDBPath          string
	JobstatsDBBackupPath    string
	JobstatsDBTable         string
	JobCutoffPeriod         time.Duration
	RetentionPeriod         time.Duration
	BackupInterval          time.Duration
	SkipDeleteOldJobs       bool
	TSDBCleanUp             bool
	TSDBURL                 *url.URL
	HTTPClient              *http.Client
	LastUpdateTimeString    string
	LastUpdateTimeStampFile string
	BatchScheduler          func(log.Logger) (*schedulers.BatchScheduler, error)
}

// Storage
type storageConfig struct {
	dbPath             string
	dbBackupPath       string
	dbTable            string
	retentionPeriod    time.Duration
	cutoffPeriod       time.Duration
	backupInterval     time.Duration
	lastUpdateTime     time.Time
	lastBackupTime     time.Time
	lastUpdateTimeFile string
	backupRetries      int
	skipDeleteOldJobs  bool
}

// job stats DB struct
type jobStatsDB struct {
	logger    log.Logger
	db        *sql.DB
	dbConn    *sqlite.Conn
	scheduler *schedulers.BatchScheduler
	tsdb      *tsdbConfig
	storage   *storageConfig
}

const (
	sqlite3Driver    = "ensign_sqlite3"
	sqlite3Main      = "main"
	pagesPerStep     = 5
	stepSleep        = 50 * time.Millisecond
	maxBackupRetries = 5
)

var (
	// Ref: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	// Ref: https://gitlab.com/gnufred/logslate/-/blob/8eda5cedc9a28da3793dcf73480d618c95cc322c/playground/sqlite3.go
	// Ref: https://github.com/mattn/go-sqlite3/issues/1145#issuecomment-1519012055
	// pragmaStatements = []string{
	// 	"PRAGMA synchronous = OFF",
	// 	"PRAGMA journal_mode = MEMORY",
	// }
	defaultOpts = map[string]string{
		"_busy_timeout": "5000",
		"_journal_mode": "MEMORY",
		"_synchronous":  "0",
	}
	indexStatements = []string{
		`CREATE INDEX IF NOT EXISTS i1 ON %s (Usr,Account,Start);`,
		`CREATE INDEX IF NOT EXISTS i2 ON %s (Usr,Jobuuid);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS i3 ON %s (Jobid,Start);`, // To ensure we dont insert duplicated rows
	}
)

// Stringer receiver for storageConfig
func (s *storageConfig) String() string {
	return fmt.Sprintf(
		"storageConfig{dbPath: %s, dbTable: %s, retentionPeriod: %s, cutoffPeriod: %s, "+
			"lastUpdateTime: %s, lastBackupTime: %s, lastUpdateTimeFile: %s}",
		s.dbPath, s.dbTable, s.retentionPeriod, s.cutoffPeriod, s.lastUpdateTime,
		s.lastBackupTime, s.lastUpdateTimeFile,
	)
}

// Stringer receiver for tsdbConfig
func (t *tsdbConfig) String() string {
	return fmt.Sprintf(
		"tsdbConfig{url: %s, cleanUp: %t, deleteSeriesEndpoint: %s}",
		t.url.Redacted(), t.cleanUp, t.deleteSeriesEndpoint.Redacted(),
	)
}

// Make DSN from DB file path and opts map
func makeDSN(filePath string, opts map[string]string) string {
	dsn := fmt.Sprintf("file:%s", filePath)
	optsSlice := []string{}
	for opt, val := range opts {
		optsSlice = append(optsSlice, fmt.Sprintf("%s=%s", opt, val))
	}
	optString := strings.Join(optsSlice, "&")
	return fmt.Sprintf("%s?%s", dsn, optString)
}

// Write timestamp to a file
func writeTimeStampToFile(filePath string, timeStamp time.Time, logger log.Logger) {
	timeStampString := timeStamp.Format(base.DatetimeLayout)
	timeStampByte := []byte(timeStampString)
	if err := os.WriteFile(filePath, timeStampByte, 0644); err != nil {
		level.Error(logger).
			Log("msg", "Failed to write timestamp to file", "time", timeStampString, "file", filePath, "err", err)
	}
}

// Create a table for storing job stats
func createTable(dbTableName string, db *sql.DB, logger log.Logger) error {
	fieldLines := []string{}
	for _, field := range base.BatchJobFieldNames {
		fieldLines = append(fieldLines, fmt.Sprintf("		\"%s\" TEXT,", field))
	}
	fieldLines[len(fieldLines)-1] = strings.Split(fieldLines[len(fieldLines)-1], ",")[0]
	createBatchJobStatSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		"id" integer NOT NULL PRIMARY KEY,		
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
	if _, err = statement.Exec(); err != nil {
		level.Error(logger).Log("msg", "Failed to execute SQL command creation statement", "err", err)
		return err
	}

	// Prepare SQL DB index creation Statement
	for _, stmt := range indexStatements {
		level.Info(logger).Log("msg", "Creating DB index", "index", stmt)
		createIndexSQL := fmt.Sprintf(stmt, dbTableName)
		statement, err = db.Prepare(createIndexSQL)
		if err != nil {
			level.Error(logger).
				Log("msg", "Failed to prepare SQL statement for index creation", "err", err)
			return err
		}

		// Execute SQL DB index creation Statements
		if _, err = statement.Exec(); err != nil {
			level.Error(logger).Log("msg", "Failed to execute SQL command for index creation statement", "err", err)
			return err
		}
	}
	level.Info(logger).Log("msg", "SQLite DB table for jobstats successfully created")
	return nil
}

// Open DB connection and return connection poiner
func openDBConnection(dbFilePath string) (*sql.DB, *sqlite.Conn, error) {
	var db *sql.DB
	var dbConn *sqlite.Conn
	var err error
	var ok bool
	if db, err = sql.Open(sqlite3Driver, makeDSN(dbFilePath, defaultOpts)); err != nil {
		return nil, nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, nil, err
	}

	if dbConn, ok = sqlite.GetLastConn(); !ok {
		return nil, nil, err
	}
	return db, dbConn, nil
}

// Setup DB and create table
func setupDB(dbFilePath string, dbTableName string, logger log.Logger) (*sql.DB, *sqlite.Conn, error) {
	if _, err := os.Stat(dbFilePath); err == nil {
		// Open the created SQLite File
		db, dbConn, err := openDBConnection(dbFilePath)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to open DB file", "err", err)
			return nil, nil, err
		}
		return db, dbConn, nil
	}

	// If file does not exist, create SQLite file
	file, err := os.Create(dbFilePath)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create DB file", "err", err)
		return nil, nil, err
	}
	file.Close()

	// Open the created SQLite File
	db, dbConn, err := openDBConnection(dbFilePath)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to open DB file", "err", err)
		return nil, nil, err
	}

	// Create Table
	if err = createTable(dbTableName, db, logger); err != nil {
		level.Error(logger).Log("msg", "Failed to create DB table", "err", err)
		return nil, nil, err
	}
	return db, dbConn, nil
}

// Make new JobStatsDB struct
func NewJobStatsDB(c *Config) (*jobStatsDB, error) {
	var lastJobsUpdateTime time.Time
	var err error

	// Check if backup data directory exists and create one if it does not
	if _, err := os.Stat(c.JobstatsDBBackupPath); err != nil {
		level.Info(c.Logger).Log("msg", "Backup data path directory does not exist. Creating...", "path", c.JobstatsDBBackupPath)
		if err := os.Mkdir(c.JobstatsDBBackupPath, 0750); err != nil {
			level.Error(c.Logger).Log("msg", "Could not create backup data path directory", "path", c.JobstatsDBBackupPath, "err", err)
			return nil, err
		}
	}

	// Check if data path exists and attempt to create if it does not exist
	dataPath := filepath.Dir(c.JobstatsDBPath)
	if _, err := os.Stat(dataPath); err != nil {
		level.Info(c.Logger).Log("msg", "Data path directory does not exist. Creating...", "path", dataPath)
		if err := os.Mkdir(dataPath, 0750); err != nil {
			level.Error(c.Logger).Log("msg", "Could not create data path directory", "path", dataPath, "err", err)
			return nil, err
		}
		goto updatetime
	} else {
		// If data directory exists, try to read lastJobsUpdateTimeFile
		if _, err := os.Stat(c.LastUpdateTimeStampFile); err == nil {
			lastUpdateTimeString, err := os.ReadFile(c.LastUpdateTimeStampFile)
			if err != nil {
				level.Error(c.Logger).Log("msg", "Failed to read lastjobsupdatetime file", "err", err)
				goto updatetime
			} else {
				// Trim any spaces and new lines
				lastJobsUpdateTime, err = time.Parse(base.DatetimeLayout, strings.TrimSuffix(strings.TrimSpace(string(lastUpdateTimeString)), "\n"))
				if err != nil {
					level.Error(c.Logger).Log("msg", "Failed to parse time string in lastjobsupdatetime file", "time", lastUpdateTimeString, "err", err)
					goto updatetime
				}
			}
			goto setup
		} else {
			goto updatetime
		}
	}

updatetime:
	lastJobsUpdateTime, err = time.Parse("2006-01-02", c.LastUpdateTimeString)
	if err != nil {
		level.Error(c.Logger).Log("msg", "Failed to parse time string", "time", c.LastUpdateTimeString, "err", err)
		return nil, err
	}

	// Write to file for persistence in case of restarts
	writeTimeStampToFile(c.LastUpdateTimeStampFile, lastJobsUpdateTime, c.Logger)

setup:
	// Setup scheduler struct that retrieves job data
	scheduler, err := c.BatchScheduler(c.Logger)
	if err != nil {
		level.Error(c.Logger).Log("msg", "Batch scheduler setup failed", "err", err)
		return nil, err
	}

	// Setup DB
	db, dbConn, err := setupDB(c.JobstatsDBPath, c.JobstatsDBTable, c.Logger)
	if err != nil {
		level.Error(c.Logger).Log("msg", "DB setup failed", "err", err)
		return nil, err
	}

	// Now make an instance of time.Date with proper format and zone
	lastJobsUpdateTime = time.Date(
		lastJobsUpdateTime.Year(),
		lastJobsUpdateTime.Month(),
		lastJobsUpdateTime.Day(),
		lastJobsUpdateTime.Hour(),
		lastJobsUpdateTime.Minute(),
		lastJobsUpdateTime.Second(),
		lastJobsUpdateTime.Nanosecond(),
		time.Now().Location(),
	)

	// Storage config
	storageConfig := &storageConfig{
		dbPath:             c.JobstatsDBPath,
		dbBackupPath:       c.JobstatsDBBackupPath,
		dbTable:            c.JobstatsDBTable,
		retentionPeriod:    c.RetentionPeriod,
		cutoffPeriod:       c.JobCutoffPeriod,
		backupInterval:     c.BackupInterval,
		lastUpdateTime:     lastJobsUpdateTime,
		lastBackupTime:     time.Now(),
		lastUpdateTimeFile: c.LastUpdateTimeStampFile,
		skipDeleteOldJobs:  c.SkipDeleteOldJobs,
	}

	// Emit debug logs
	level.Debug(c.Logger).Log("msg", "Storage config", "cfg", storageConfig)

	// tsdb config
	var tsdbCfg *tsdbConfig
	if c.TSDBCleanUp {
		tsdbCfg = &tsdbConfig{
			url:                  c.TSDBURL,
			cleanUp:              c.TSDBCleanUp,
			client:               c.HTTPClient,
			deleteSeriesEndpoint: c.TSDBURL.JoinPath("/api/v1/admin/tsdb/delete_series"),
		}
	} else {
		tsdbCfg = &tsdbConfig{
			cleanUp: c.TSDBCleanUp,
		}
	}
	// Emit debug logs
	level.Debug(c.Logger).Log("msg", "TSDB config", "cfg", tsdbCfg)
	return &jobStatsDB{
		logger:    c.Logger,
		db:        db,
		dbConn:    dbConn,
		scheduler: scheduler,
		tsdb:      tsdbCfg,
		storage:   storageConfig,
	}, nil
}

// // Set all necessary PRAGMA statement on DB
// func (j *jobStatsDB) setPragmaDirectives() {
// 	for _, stmt := range pragmaStatements {
// 		_, err := j.db.Exec(stmt)
// 		if err != nil {
// 			level.Error(j.logger).Log("msg", "Failed to execute pragma statement", "statement", stmt, "err", err)
// 		}
// 	}
// }

// // Check if DB is locked
// func (j *jobStatsDB) checkDBLock() error {
// 	if _, err := j.db.Exec("BEGIN EXCLUSIVE TRANSACTION;"); err != nil {
// 		return err
// 	}
// 	if _, err := j.db.Exec("COMMIT;"); err != nil {
// 		level.Error(j.logger).Log("msg", "Failed to commit exclusive transcation", "err", err)
// 		return err
// 	}
// 	return nil
// }

// Collect job stats
func (j *jobStatsDB) Collect() error {
	var currentTime = time.Now()

	// If duration is less than 1 day do single update
	if currentTime.Sub(j.storage.lastUpdateTime) < time.Duration(24 * time.Hour) {
		return j.getJobStats(j.storage.lastUpdateTime, currentTime)
	}
	level.Info(j.logger).
		Log("msg", "DB update duration is more than 1 day. Doing incremental update. This may take a while...")

	// If duration is more than 1 day, do incremental update
	var nextUpdateTime time.Time
	for {
		nextUpdateTime = j.storage.lastUpdateTime.Add(24 * time.Hour)
		if nextUpdateTime.Compare(currentTime) == -1 {
			level.Debug(j.logger).
				Log("msg", "Incremental DB update step", "from", j.storage.lastUpdateTime, "to", nextUpdateTime)
			if err := j.getJobStats(j.storage.lastUpdateTime, nextUpdateTime); err != nil {
				level.Error(j.logger).
					Log("msg", "Failed incremental update", "from", j.storage.lastUpdateTime, "to", nextUpdateTime, "err", err)
				return err
			}
		} else {
			level.Debug(j.logger).Log("msg", "Final incremental DB update step", "from", j.storage.lastUpdateTime, "to", currentTime)
			return j.getJobStats(j.storage.lastUpdateTime, currentTime)
		}

		// Sleep for couple of seconds before making next update
		// This is to let DB breath a bit before serving next request
		time.Sleep(2 * time.Second)
	}
}

// Get job stats and insert them into DB
func (j *jobStatsDB) getJobStats(startTime, endTime time.Time) error {
	// Retrieve jobs from unerlying batch scheduler
	jobs, err := j.scheduler.Fetch(startTime, endTime)
	if err != nil {
		return err
	}

	// Create DB backup at configured interval
	if err = j.createBackup(); err != nil {
		level.Error(j.logger).Log("msg", "Failed to create DB backup", "err", err)
	}

	// Check if DB is already locked.
	// If locked, return with noop
	// if err = j.checkDBLock(); err != nil {
	// 	level.Error(j.logger).Log("msg", "DB is locked. Jobs WILL NOT BE inserted.", "err", err)
	// 	return err
	// }

	// Begin transcation
	tx, err := j.db.Begin()
	if err != nil {
		return err
	}

	// Set pragma statements
	// j.setPragmaDirectives()

	// Make prepare statement and defer closing statement
	stmt, err := j.prepareInsertStatement(tx)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Insert data into DB
	level.Debug(j.logger).Log("msg", "Inserting jobs into DB")
	ignoredJobs := j.insertJobs(stmt, jobs)
	level.Debug(j.logger).Log("msg", "Finished inserting jobs into DB")

	// Delete older entries
	level.Debug(j.logger).Log("msg", "Cleaning up old jobs")
	if err = j.deleteOldJobs(tx); err != nil {
		level.Error(j.logger).Log("msg", "Failed to clean up old job entries", "err", err)
	} else {
		level.Debug(j.logger).Log("msg", "Cleaned up old jobs in DB")
	}

	// Commit changes
	if err = tx.Commit(); err != nil {
		return err
	}

	// Finally make API requests to TSDB to delete timeseries of ignored jobs
	if j.tsdb.cleanUp {
		level.Debug(j.logger).Log("msg", "Cleaning up time series of ignored jobs in TSDB")
		if err = j.deleteTimeSeries(startTime, endTime, ignoredJobs); err != nil {
			level.Error(j.logger).Log("msg", "Failed to clean up time series in TSDB", "err", err)
		} else {
			level.Debug(j.logger).Log("msg", "Cleaned up time series in TSDB")
		}
	}

	// Write endTime to file to keep track upon successful insertion of data
	j.storage.lastUpdateTime = endTime
	writeTimeStampToFile(j.storage.lastUpdateTimeFile, j.storage.lastUpdateTime, j.logger)
	return nil
}

// Close DB connection
func (j *jobStatsDB) Stop() error {
	return j.db.Close()
}

// Backup executes the sqlite3 backup strategy
// Based on https://gist.github.com/bbengfort/452a9d5e74a63d88e5a34a580d6cb6d3
func (j *jobStatsDB) backup(backupDBPath string) error {
	var file *os.File
	var err error
	// Create a backup DB file
	if file, err = os.Create(backupDBPath); err != nil {
		return err
	}
	file.Close()

	// Open a second sqlite3 database at the backup location
	destDB, destConn, err := openDBConnection(backupDBPath)
	if err != nil {
		return err
	}

	// Ensure the database connection is closed when the backup is complete; this will
	// also close the underlying sqlite3 connection.
	defer destDB.Close()

	// Create the backup manager into the destination db from the src connection.
	// NOTE: backup.Finish() MUST be called to prevent panics.
	var backup *sqlite3.SQLiteBackup
	if backup, err = destConn.Backup(sqlite3Main, j.dbConn, sqlite3Main); err != nil {
		return err
	}

	// Execute the backup copying the specified number of pages at each step then
	// sleeping to allow concurrent transactions to acquire write locks. This will
	// increase the amount of backup time but preserve normal operations. This means
	// that backups will be most successful during low-volume times.
	//
	// We will not hit this as we never write and backup at the same time. This is 
	// put in place for clarity and future extensibility
	var isDone bool
	for !isDone {
		// Backing up a smaller number of pages per step is the most effective way of
		// doing online backups and also allow write transactions to make progress.
		if isDone, err = backup.Step(pagesPerStep); err != nil {
			if finishErr := backup.Finish(); err != nil {
				return fmt.Errorf("errors: %s, %s", err, finishErr)
			}
			return err
		}

		level.Debug(j.logger).Log("msg", "DB backup step", "remaining", backup.Remaining(), "page_count", backup.PageCount())

		// This sleep allows other transactions to write during backups.
		time.Sleep(stepSleep)
	}
	return backup.Finish()
}

// vacuum executes sqlite3 vacuum command
func (j *jobStatsDB) vacuum() error {
	level.Debug(j.logger).Log("msg", "Starting to vacuum DB")
	if _, err := j.db.Exec("VACUUM;"); err != nil {
		return err
	}
	return nil
}

// Creates backup of DB after vacuuming DB
func (j *jobStatsDB) createBackup() error {
	hour, _, _ := time.Now().Clock()

	// Next backup time
	nextBackupTime := j.storage.lastBackupTime.Add(time.Duration(j.storage.backupInterval))

	// Check if we are at 02hr and **after** nextBackupTime
	// We try to backup DB during night when there will be smallest activity
	if hour != 02 || time.Now().Compare(nextBackupTime) == -1 {
		return nil
	}

	// First vacuum DB to reduce size
	if err := j.vacuum(); err != nil {
		level.Warn(j.logger).Log("msg", "Failed to vacuum DB", "err", err)
	}
	level.Info(j.logger).Log("msg", "DB vacuumed")

	// Attempt to create DB backup
	// Make a unique backup file name using current time
	backupDBFile := filepath.Join(j.storage.dbBackupPath, fmt.Sprintf("jobstats-%s.bak.db", time.Now().Format("200601021504")))
	if err := j.backup(backupDBFile); err != nil {
		j.storage.backupRetries++
		// If we exceed max retries to make a backup, just update lastBackupTime,
		// reset backupRetries and return
		// This way we wont keep trying to create backups
		if j.storage.backupRetries > maxBackupRetries {
			j.storage.lastBackupTime = time.Now()
			j.storage.backupRetries = 0
		}
		return err
	}
	level.Info(j.logger).Log("msg", "DB backed up")

	// Update last vacuum time and reset backupRetries counter
	j.storage.backupRetries = 0
	j.storage.lastBackupTime = time.Now()
	return nil
}

// Delete old entries in DB
func (j *jobStatsDB) deleteOldJobs(tx *sql.Tx) error {
	// In testing we want to skip this
	if j.storage.skipDeleteOldJobs {
		return nil
	}

	deleteRowQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE Start <= date('now', '-%d day')",
		j.storage.dbTable,
		int(j.storage.retentionPeriod.Hours()/24),
	)
	_, err := tx.Exec(deleteRowQuery)
	if err != nil {
		return err
	}

	// Get changes
	var rowsDeleted int
	_ = tx.QueryRow("SELECT changes();").Scan(&rowsDeleted)
	level.Debug(j.logger).Log("msg", "Queried for changes after deletion", "rowsDeleted", rowsDeleted)
	return nil
}

// Make and return prepare statement for inserting entries
func (j *jobStatsDB) prepareInsertStatement(tx *sql.Tx) (*sql.Stmt, error) {
	placeHolderString := fmt.Sprintf(
		"(%s)",
		strings.Join(strings.Split(strings.Repeat("?", len(base.BatchJobFieldNames)), ""), ","),
	)
	fieldNamesString := strings.Join(base.BatchJobFieldNames, ",")
	insertStatement := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s(%s) VALUES %s",
		j.storage.dbTable,
		fieldNamesString,
		placeHolderString,
	)
	stmt, err := tx.Prepare(insertStatement)
	if err != nil {
		return nil, err
	}
	return stmt, nil
}

// Insert job stat into DB
func (j *jobStatsDB) insertJobs(statement *sql.Stmt, jobStats []base.BatchJob) []string {
	var ignoredJobs []string
	var err error
	for _, jobStat := range jobStats {
		// Empty job
		if jobStat == (base.BatchJob{}) {
			continue
		}

		// Ignore jobs that ran for less than jobCutoffPeriod seconds and check if
		// job has end time stamp. If we decide to populate DB with running jobs,
		// EndTS will be zero as we cannot convert unknown time into time stamp.
		// Check if we EndTS is not zero before ignoring job. If it is zero, it means
		// it must be RUNNING job
		if elapsedTime, err := strconv.Atoi(jobStat.ElapsedRaw); err == nil &&
			elapsedTime < int(j.storage.cutoffPeriod.Seconds()) && jobStat.EndTS != "0" {
			ignoredJobs = append(
				ignoredJobs,
				jobStat.Jobid,
			)
			continue
		}

		// level.Debug(j.logger).Log("msg", "Inserting job", "jobid", jobStat.Jobid)
		_, err = statement.Exec(
			jobStat.Jobid,
			jobStat.Jobuuid,
			jobStat.Partition,
			jobStat.QoS,
			jobStat.Account,
			jobStat.Grp,
			jobStat.Gid,
			jobStat.Usr,
			jobStat.Uid,
			jobStat.Submit,
			jobStat.Start,
			jobStat.End,
			jobStat.SubmitTS,
			jobStat.StartTS,
			jobStat.EndTS,
			jobStat.Elapsed,
			jobStat.ElapsedRaw,
			jobStat.Exitcode,
			jobStat.State,
			jobStat.Nnodes,
			jobStat.Ncpus,
			jobStat.Nodelist,
			jobStat.NodelistExp,
			jobStat.JobName,
			jobStat.WorkDir,
		)
		if err != nil {
			level.Error(j.logger).
				Log("msg", "Failed to insert job in DB", "jobid", jobStat.Jobid, "err", err)
		}
	}
	level.Debug(j.logger).Log("msg", "Ignored jobs", "numjobs", len(ignoredJobs))
	return ignoredJobs
}

// Delete time series data of ignored jobs
func (j *jobStatsDB) deleteTimeSeries(startTime time.Time, endTime time.Time, jobs []string) error {
	// Join them with | as delimiter. We will use regex match to match all series
	// with the label jobid=~"$jobids"
	allJobIds := strings.Join(jobs, "|")

	/*
		We should give start and end query params as well. If not, TSDB has to look over
		"all" time blocks (potentially 1000s or more) and try to find the series.
		The thing is the time series data of these "ignored" jobs should be head block
		as they have started and finished very "recently".

		Imagine we are updating jobs data for every 15 min and we would like to ignore jobs
		that have wall time less than 10 min. If we are updating jobs from, say 10h-10h-15,
		the jobs that have been ignored cannot start earlier than 9h50 to have finished within
		10h-10h15 window. So, all these time series must be in the head block of TSDB and
		we should provide start and end query params corresponding to
		9h50 (lastupdatetime - ignored job duration) and current time, respectively. This
		will help TSDB to narrow the search to head block and hence deletion of time series
		will be easy as they are potentially not yet persisted to disk.
	*/
	start := startTime.Add(-j.storage.cutoffPeriod)
	end := endTime

	// Matcher must be of format "{job=~"<regex>"}"
	// Ref: https://ganeshvernekar.com/blog/prometheus-tsdb-queries/
	matcher := fmt.Sprintf("{jobid=~\"%s\"}", allJobIds)

	// Add form data to request
	// TSDB expects time stamps in UTC zone
	values := url.Values{
		"match[]": []string{matcher},
		"start":   []string{start.UTC().Format(time.RFC3339Nano)},
		"end":     []string{end.UTC().Format(time.RFC3339Nano)},
	}

	// Create a new POST request
	req, err := http.NewRequest(http.MethodPost, j.tsdb.deleteSeriesEndpoint.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	_, err = j.tsdb.client.Do(req)
	return err
}
