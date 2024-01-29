package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/schedulers"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/tsdb"
	"github.com/mattn/go-sqlite3"
	"github.com/rotationalio/ensign/pkg/utils/sqlite"
)

// DB config
type Config struct {
	Logger                  log.Logger
	JobstatsDBPath          string
	JobstatsDBBackupPath    string
	JobCutoffPeriod         time.Duration
	RetentionPeriod         time.Duration
	SkipDeleteOldJobs       bool
	LastUpdateTimeString    string
	LastUpdateTimeStampFile string
	BatchScheduler          func(log.Logger) (*schedulers.BatchScheduler, error)
	TSDB                    *tsdb.TSDB
}

// Storage
type storageConfig struct {
	dbPath             string
	dbBackupPath       string
	retentionPeriod    time.Duration
	cutoffPeriod       time.Duration
	lastUpdateTime     time.Time
	lastUpdateTimeFile string
	skipDeleteOldJobs  bool
}

// job stats DB struct
type jobStatsDB struct {
	logger    log.Logger
	db        *sql.DB
	dbConn    *sqlite.Conn
	scheduler *schedulers.BatchScheduler
	tsdb      *tsdb.TSDB
	storage   *storageConfig
}

const (
	sqlite3Driver = "ensign_sqlite3"
	sqlite3Main   = "main"
	pagesPerStep  = 25
	stepSleep     = 50 * time.Millisecond
)

var (
	// Ref: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	// Ref: https://gitlab.com/gnufred/logslate/-/blob/8eda5cedc9a28da3793dcf73480d618c95cc322c/playground/sqlite3.go
	// Ref: https://github.com/mattn/go-sqlite3/issues/1145#issuecomment-1519012055
	defaultOpts = map[string]string{
		"_busy_timeout": "5000",
		"_journal_mode": "MEMORY",
		"_synchronous":  "0",
	}
	// defaultOpts = map[string]string{}
	indexStatements = []string{
		`CREATE INDEX IF NOT EXISTS i1 ON %s (Usr,Account,Start);`,
		`CREATE INDEX IF NOT EXISTS i2 ON %s (Usr,Jobuuid);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS i3 ON %s (Jobid,Start);`, // To ensure we dont insert duplicated rows
	}
)

// Stringer receiver for storageConfig
func (s *storageConfig) String() string {
	return fmt.Sprintf(
		"storageConfig{dbPath: %s, retentionPeriod: %s, cutoffPeriod: %s, "+
			"lastUpdateTime: %s, lastUpdateTimeFile: %s}",
		s.dbPath, s.retentionPeriod, s.cutoffPeriod, s.lastUpdateTime,
		s.lastUpdateTimeFile,
	)
}

// Make new JobStatsDB struct
func NewJobStatsDB(c *Config) (*jobStatsDB, error) {
	var lastJobsUpdateTime time.Time
	var err error

	// By this time dataPath **should** exist and we do not need to check for its
	// existence. Check directly for lastupdatetime file
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

updatetime:
	if lastJobsUpdateTime, err = time.Parse("2006-01-02", c.LastUpdateTimeString); err != nil {
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
	db, dbConn, err := setupDB(c.JobstatsDBPath, base.JobStatsDBTable, c.Logger)
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
		retentionPeriod:    c.RetentionPeriod,
		cutoffPeriod:       c.JobCutoffPeriod,
		lastUpdateTime:     lastJobsUpdateTime,
		lastUpdateTimeFile: c.LastUpdateTimeStampFile,
		skipDeleteOldJobs:  c.SkipDeleteOldJobs,
	}

	// Emit debug logs
	level.Debug(c.Logger).Log("msg", "Storage config", "cfg", storageConfig)
	return &jobStatsDB{
		logger:    c.Logger,
		db:        db,
		dbConn:    dbConn,
		scheduler: scheduler,
		tsdb:      c.TSDB,
		storage:   storageConfig,
	}, nil
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
		time.Sleep(time.Second)
	}
}

// Backup DB
func (j *jobStatsDB) Backup() error {
	return j.createBackup()
}

// Close DB connection
func (j *jobStatsDB) Stop() error {
	return j.db.Close()
}

// Get job stats and insert them into DB
func (j *jobStatsDB) getJobStats(startTime, endTime time.Time) error {
	// Retrieve jobs from unerlying batch scheduler
	jobs, err := j.scheduler.Fetch(startTime, endTime)
	if err != nil {
		return err
	}

	// Update jobs struct with job level metrics from TSDB
	if j.tsdb.Available() {
		level.Debug(j.logger).Log("msg", "Fetching job metrics from TSDB")
		jobs = j.fetchMetricsFromTSDB(endTime, jobs)
	}

	// Begin transcation
	tx, err := j.db.Begin()
	if err != nil {
		return err
	}

	// Delete older entries and free up DB pages
	level.Debug(j.logger).Log("msg", "Cleaning up old jobs")
	if err = j.deleteOldJobs(tx); err != nil {
		level.Error(j.logger).Log("msg", "Failed to clean up old job entries", "err", err)
	} else {
		level.Debug(j.logger).Log("msg", "Cleaned up old jobs in DB")
	}

	// Make prepare statement and defer closing statement
	stmt, err := j.prepareInsertStatement(tx)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Insert data into DB
	level.Debug(j.logger).Log("msg", "Inserting new jobs into DB")
	ignoredJobs := j.insertJobs(stmt, jobs)
	level.Debug(j.logger).Log("msg", "Finished inserting new jobs into DB")

	// Commit changes
	if err = tx.Commit(); err != nil {
		return err
	}

	// Finally make API requests to TSDB to delete timeseries of ignored jobs
	if j.tsdb.Available() {
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

// Fetch job metrics from TSDB and update JobStat struct for each job
func (j *jobStatsDB) fetchMetricsFromTSDB(queryTime time.Time, jobs []base.JobStats) []base.JobStats {
	var minStartTime = queryTime.UnixMilli()
	var allJobIds string

	// Loop over all jobs and find earliest start time of a job
	for _, job := range jobs {
		allJobIds = fmt.Sprintf("%d|", job.Jobid)
		if minStartTime > job.StartTS {
			minStartTime = job.StartTS
		}
	}
	allJobIds = strings.TrimRight(allJobIds, "|")

	// Get max window from minStartTime to queryTime
	maxDuration := time.Duration((queryTime.UnixMilli() - minStartTime) * int64(time.Millisecond))

	// Get metrics from TSDB
	cpuMetrics, err := j.tsdb.CPUMetrics(queryTime, maxDuration, allJobIds)
	if err != nil {
		level.Warn(j.logger).Log("msg", "Errors in fetching CPU job metrics from TSDB", "err", err)
	}
	gpuMetrics, err := j.tsdb.GPUMetrics(queryTime, maxDuration, allJobIds)
	if err != nil {
		level.Warn(j.logger).Log("msg", "Errors in fetching GPU job metrics from TSDB", "err", err)
	}

	// Finally update jobs
	for _, job := range jobs {
		// Update with CPU metrics
		if cpuMetrics.AvgCPUUsage != nil {
			if v, exists := cpuMetrics.AvgCPUUsage[job.Jobid]; exists {
				job.AveCPUUsage = v
			}
		}
		if cpuMetrics.AvgCPUMemUsage != nil {
			if v, exists := cpuMetrics.AvgCPUMemUsage[job.Jobid]; exists {
				job.AveCPUMemUsage = v
			}
		}
		if cpuMetrics.TotalCPUEnergyUsage != nil {
			if v, exists := cpuMetrics.TotalCPUEnergyUsage[job.Jobid]; exists {
				job.TotalCPUEnergyUsage = v
			}
		}
		if cpuMetrics.TotalCPUEmissions != nil {
			if v, exists := cpuMetrics.TotalCPUEmissions[job.Jobid]; exists {
				job.TotalCPUEmissions = v
			}
		}

		// Update with GPU metrics
		if gpuMetrics.AvgGPUUsage != nil {
			if v, exists := gpuMetrics.AvgGPUUsage[job.Jobid]; exists {
				job.AveCPUUsage = v
			}
		}
		if gpuMetrics.AvgGPUMemUsage != nil {
			if v, exists := gpuMetrics.AvgGPUMemUsage[job.Jobid]; exists {
				job.AveCPUMemUsage = v
			}
		}
		if gpuMetrics.TotalGPUEnergyUsage != nil {
			if v, exists := gpuMetrics.TotalGPUEnergyUsage[job.Jobid]; exists {
				job.TotalCPUEnergyUsage = v
			}
		}
		if gpuMetrics.TotalGPUEmissions != nil {
			if v, exists := gpuMetrics.TotalGPUEmissions[job.Jobid]; exists {
				job.TotalCPUEmissions = v
			}
		}
	}
	return jobs
}

// Delete old entries in DB
func (j *jobStatsDB) deleteOldJobs(tx *sql.Tx) error {
	// In testing we want to skip this
	if j.storage.skipDeleteOldJobs {
		return nil
	}

	deleteRowQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE Start <= date('now', '-%d day')",
		base.JobStatsDBTable,
		int(j.storage.retentionPeriod.Hours()/24),
	)
	_, err := tx.Exec(deleteRowQuery)
	if err != nil {
		return err
	}

	// Get changes
	var rowsDeleted int
	_ = tx.QueryRow("SELECT changes();").Scan(&rowsDeleted)
	level.Debug(j.logger).Log("jobs_deleted", rowsDeleted)
	return nil
}

// Make and return prepare statement for inserting entries
func (j *jobStatsDB) prepareInsertStatement(tx *sql.Tx) (*sql.Stmt, error) {
	placeHolderString := fmt.Sprintf(
		"(%s)",
		strings.Join(strings.Split(strings.Repeat("?", len(base.JobStatsFieldNames)), ""), ","),
	)
	fieldNamesString := strings.Join(base.JobStatsFieldNames, ",")
	insertStatement := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s(%s) VALUES %s",
		base.JobStatsDBTable,
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
func (j *jobStatsDB) insertJobs(statement *sql.Stmt, jobStats []base.JobStats) []string {
	var ignoredJobs []string
	var err error
	for _, jobStat := range jobStats {
		// Empty job
		if jobStat == (base.JobStats{}) {
			continue
		}

		// Ignore jobs that ran for less than jobCutoffPeriod seconds and check if
		// job has end time stamp. If we decide to populate DB with running jobs,
		// EndTS will be zero as we cannot convert unknown time into time stamp.
		// Check if we EndTS is not zero before ignoring job. If it is zero, it means
		// it must be RUNNING job
		var ignore = 0
		if jobStat.ElapsedRaw < int64(j.storage.cutoffPeriod.Seconds()) && jobStat.EndTS != 0 {
			ignoredJobs = append(
				ignoredJobs,
				strconv.FormatInt(jobStat.Jobid, 10),
			)
			ignore = 1
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
			jobStat.Mem,
			jobStat.Ngpus,
			jobStat.Nodelist,
			jobStat.NodelistExp,
			jobStat.JobName,
			jobStat.WorkDir,
			jobStat.CPUBilling,
			jobStat.GPUBilling,
			jobStat.MiscBilling,
			jobStat.AveCPUUsage,
			jobStat.AveCPUMemUsage,
			jobStat.TotalCPUEnergyUsage,
			jobStat.TotalCPUEmissions,
			jobStat.AveGPUUsage,
			jobStat.AveGPUMemUsage,
			jobStat.TotalGPUEnergyUsage,
			jobStat.TotalGPUEmissions,
			jobStat.Comment,
			ignore,
		)
		if err != nil {
			level.Error(j.logger).
				Log("msg", "Failed to insert job in DB", "jobid", jobStat.Jobid, "err", err)
		}
	}
	level.Debug(j.logger).Log("jobs_ignored", len(ignoredJobs))
	return ignoredJobs
}

// Delete time series data of ignored jobs
func (j *jobStatsDB) deleteTimeSeries(startTime time.Time, endTime time.Time, jobs []string) error {
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
	//
	// Join them with | as delimiter. We will use regex match to match all series
	// with the label jobid=~"$jobids"
	allJobIds := strings.Join(jobs, "|")
	matcher := fmt.Sprintf("{jobid=~\"%s\"}", allJobIds)
	// Make a API request to delete data of ignored jobs
	return j.tsdb.Delete(start, end, matcher)
}

// Backup executes the sqlite3 backup strategy
// Based on https://gist.github.com/bbengfort/452a9d5e74a63d88e5a34a580d6cb6d3
// Ref: https://github.com/rotationalio/ensign/pull/529/files
func (j *jobStatsDB) backup(backupDBPath string) error {
	var backupDBFile *os.File
	var err error
	// Create a backup DB file
	if backupDBFile, err = os.Create(backupDBPath); err != nil {
		return err
	}
	backupDBFile.Close()

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
	// First vacuum DB to reduce size
	if err := j.vacuum(); err != nil {
		level.Warn(j.logger).Log("msg", "Failed to vacuum DB", "err", err)
	}
	level.Debug(j.logger).Log("msg", "DB vacuumed")

	// Attempt to create DB backup
	// Make a unique backup file name using current time
	backupDBFile := filepath.Join(j.storage.dbBackupPath, fmt.Sprintf("jobstats-%s.bak.db", time.Now().Format("200601021504")))
	if err := j.backup(backupDBFile); err != nil {
		return err
	}
	level.Info(j.logger).Log("msg", "DB backed up", "file", backupDBFile)
	return nil
}
