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
	"github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/schedulers"
	_ "github.com/mattn/go-sqlite3"
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
	JobstatsDBTable         string
	JobCutoffPeriod         time.Duration
	RetentionPeriod         time.Duration
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
	dbTable            string
	retentionPeriod    time.Duration
	cutoffPeriod       time.Duration
	lastUpdateTime     time.Time
	lastVacuumTime     time.Time
	lastUpdateTimeFile string
	skipDeleteOldJobs  bool
}

// job stats DB struct
type jobStatsDB struct {
	logger    log.Logger
	db        *sql.DB
	scheduler *schedulers.BatchScheduler
	tsdb      *tsdbConfig
	storage   *storageConfig
}

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
		`CREATE UNIQUE INDEX IF NOT EXISTS i3 ON %s (Jobid,Start);`, // To ensure we dont insert duplicated rows
	}
)

// Stringer receiver for storageConfig
func (s *storageConfig) String() string {
	return fmt.Sprintf(
		"storageConfig{dbPath: %s, dbTable: %s, retentionPeriod: %s, cutoffPeriod: %s, "+
			"lastUpdateTime: %s, lastVacuumTime: %s, lastUpdateTimeFile: %s}",
		s.dbPath, s.dbTable, s.retentionPeriod, s.cutoffPeriod, s.lastUpdateTime,
		s.lastVacuumTime, s.lastUpdateTimeFile,
	)
}

// Stringer receiver for tsdbConfig
func (t *tsdbConfig) String() string {
	return fmt.Sprintf(
		"tsdbConfig{url: %s, cleanUp: %t, deleteSeriesEndpoint: %s}",
		t.url.Redacted(), t.cleanUp, t.deleteSeriesEndpoint.Redacted(),
	)
}

// Write timestamp to a file
func writeTimeStampToFile(filePath string, timeStamp time.Time, logger log.Logger) {
	timeStampString := timeStamp.Format(dateFormat)
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
		level.Info(logger).Log("msg", "Creating DB index with Usr,Account,Start columns")
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
	if err = createTable(dbTableName, db, logger); err != nil {
		level.Error(logger).Log("msg", "Failed to create DB table", "err", err)
		return nil, err
	}
	return db, nil
}

// Make new JobStatsDB struct
func NewJobStatsDB(c *Config) (*jobStatsDB, error) {
	// Check if data path exists and attempt to create if it does not exist
	var lastJobsUpdateTime time.Time
	var err error
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
				lastJobsUpdateTime, err = time.Parse(dateFormat, strings.TrimSuffix(strings.TrimSpace(string(lastUpdateTimeString)), "\n"))
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
	db, err := setupDB(c.JobstatsDBPath, c.JobstatsDBTable, c.Logger)
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
		dbTable:            c.JobstatsDBTable,
		retentionPeriod:    c.RetentionPeriod,
		cutoffPeriod:       c.JobCutoffPeriod,
		lastUpdateTime:     lastJobsUpdateTime,
		lastVacuumTime:     time.Now(),
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
		scheduler: scheduler,
		tsdb:      tsdbCfg,
		storage:   storageConfig,
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
	if _, err := j.db.Exec("BEGIN EXCLUSIVE TRANSACTION;"); err != nil {
		return err
	}
	if _, err := j.db.Exec("COMMIT;"); err != nil {
		level.Error(j.logger).Log("msg", "Failed to commit exclusive transcation", "err", err)
		return err
	}
	return nil
}

// Collect job stats
func (j *jobStatsDB) Collect() error {
	var currentTime = time.Now()

	// If duration is less than 1 day do single update
	if currentTime.Sub(j.storage.lastUpdateTime) < time.Duration(24*time.Hour) {
		return j.getJobStats(j.storage.lastUpdateTime, currentTime)
	}
	level.Info(j.logger).
		Log("msg", "DB update duration is more than 1 day. Doing incremental update. This may take a while...")

	// If duration is more than 1 day, do update for each day
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
		// This is to avoid let DB breath a bit before serving next request
		time.Sleep(2 * time.Second)
	}
}

// Get job stats and insert them into DB
func (j *jobStatsDB) getJobStats(startTime, endTime time.Time) error {
	// Retrieve jobs from unerlying batch scheduler
	jobs, err := j.scheduler.Fetch(startTime, endTime)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to fetch jobs from batch scheduler", "err", err)
		return err
	}

	// Vacuum DB every Monday after 02h:00 (Sunday after midnight)
	if err = j.vacuumDB(); err != nil {
		level.Error(j.logger).Log("msg", "Failed to vacuum DB", "err", err)
	}

	// Check if DB is already locked.
	// If locked, return with noop
	if err = j.checkDBLock(); err != nil {
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

	// Make prepare statement and defer closing statement
	stmt, err := j.prepareInsertStatement(tx)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to prepare insert job statement in DB", "err", err)
		return err
	}
	defer stmt.Close()

	// Insert data into DB
	level.Debug(j.logger).Log("msg", "Inserting jobs into DB")
	ignoredJobs := j.insertJobs(stmt, jobs)
	level.Debug(j.logger).Log("msg", "Finished inserting jobs into DB")

	// Delete older entries
	level.Debug(j.logger).Log("msg", "Deleting old jobs")
	if err = j.deleteOldJobs(tx); err != nil {
		level.Error(j.logger).Log("msg", "Failed to delete old job entries", "err", err)
	} else {
		level.Debug(j.logger).Log("msg", "Finished deleting old jobs in DB")
	}

	// Commit changes
	if err = tx.Commit(); err != nil {
		level.Error(j.logger).Log("msg", "Failed to commit DB transcation", "err", err)
		return err
	}

	// Finally make API requests to TSDB to delete timeseries of ignored jobs
	if j.tsdb.cleanUp {
		level.Debug(j.logger).Log("msg", "Deleting time series in TSDB of ignored jobs")
		if err = j.deleteTimeSeries(startTime, endTime, ignoredJobs); err != nil {
			level.Error(j.logger).Log("msg", "Failed to delete time series in TSDB", "err", err)
		} else {
			level.Debug(j.logger).Log("msg", "Finished deleting time series in TSDB")
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

// Vacuum DB to reduce fragementation and size
func (j *jobStatsDB) vacuumDB() error {
	hour, _, _ := time.Now().Clock()

	// Next vacuum time is 7 days after last vacuum
	nextVacuumTime := j.storage.lastVacuumTime.Add(time.Duration(168) * time.Hour)

	// Check if we are at 02hr and **after** nextVacuumTime
	// We try to vacuum DB during night when there will be smallest activity
	if hour != 02 || time.Now().Compare(nextVacuumTime) == -1 {
		return nil
	}

	// Start vacuuming
	level.Info(j.logger).Log("msg", "Starting to vacuum DB")
	_, err := j.db.Exec("VACUUM;")
	if err != nil {
		return err
	}
	level.Info(j.logger).Log("msg", "DB vacuum successfully finished")

	// Update last vacuum time
	j.storage.lastVacuumTime = time.Now()
	return nil
}

// Delete old entries in DB
func (j *jobStatsDB) deleteOldJobs(tx *sql.Tx) error {
	// In testing we want to skip this
	if j.storage.skipDeleteOldJobs {
		level.Debug(j.logger).Log("msg", "Skipping deletion of old jobs for testing")
		return nil
	}

	deleteRowQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE Start <= date('now', '-%d day')",
		j.storage.dbTable,
		int(j.storage.retentionPeriod.Hours()/24),
	)
	_, err := tx.Exec(deleteRowQuery)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to delete old jobs", "err", err)
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
		level.Error(j.logger).
			Log("msg", "Failed to make a new HTTP request for deleting time series in TSDB", "err", err)
	}

	// Add necessary headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	_, err = j.tsdb.client.Do(req)
	return err
}
