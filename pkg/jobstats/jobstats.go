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

type jobStats struct {
	logger            log.Logger
	batchScheduler    string
	jobstatDBPath     string
	lastTimeStampFile string
}

type batchJob struct {
	Jobid       string
	Jobuuid     string
	Cluster     string
	Partition   string
	Account     string
	Grp         string
	Gid         string
	Usr         string
	Uid         string
	Submit      string
	Eligible    string
	Start       string
	End         string
	Elapsed     string
	Elapsedraw  string
	Exitcode    string
	State       string
	Nnodes      string
	Ncpus       string
	Reqcpus     string
	Reqmem      string
	Reqtres     string
	Timelimit   string
	Nodelist    string
	NodelistExp string
	Jobname     string
	Workdir     string
}

var (
	dateFormat = "2006-01-02T15:04:05"
	checksMap  = map[string]interface{}{
		"slurm": slurmChecks,
	}
	statsMap = map[string]interface{}{
		"slurm": getSlurmJobs,
	}
)

func NewJobStats(logger log.Logger, batchScheduler string, jobstatDBPath string, lastTimeStampFile string) *jobStats {
	return &jobStats{
		logger:            logger,
		batchScheduler:    batchScheduler,
		jobstatDBPath:     jobstatDBPath,
		lastTimeStampFile: lastTimeStampFile,
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

// Prepare DB and create table
func (j *jobStats) prepareDB() (*sql.DB, error) {
	if _, err := os.Stat(j.jobstatDBPath); err == nil {
		// Create SQLite file
		file, err := os.Create(j.jobstatDBPath)
		if err != nil {
			level.Error(j.logger).Log("msg", "Failed to create DB file", "err", err)
			return nil, err
		}
		file.Close()
	}
	// Open the created SQLite File
	dbConn, err := j.openDBConnection()
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to open DB file", "err", err)
		return nil, err
	}
	err = j.createTable(dbConn)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to create DB table", "err", err)
		return nil, err
	}
	// Ref: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	// Ref: https://gitlab.com/gnufred/logslate/-/blob/8eda5cedc9a28da3793dcf73480d618c95cc322c/playground/sqlite3.go
	// Ref: https://github.com/mattn/go-sqlite3/issues/1145#issuecomment-1519012055
	_, err = dbConn.Exec("PRAGMA synchronous = OFF")
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to set synchronous to OFF", "err", err)
	}
	_, err = dbConn.Exec("PRAGMA journal_mode = MEMORY")
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to set journal_mode to MEMORY", "err", err)
	}
	return dbConn, nil
}

// Create a table for storing job stats
func (j *jobStats) createTable(db *sql.DB) error {
	allFields := utils.GetStructFieldName(batchJob{})
	fieldLines := []string{}
	for _, field := range allFields {
		fieldLines = append(fieldLines, fmt.Sprintf("		\"%s\" TEXT,", field))
	}
	fieldLines[len(fieldLines)-1] = strings.Split(fieldLines[len(fieldLines)-1], ",")[0]
	createBatchJobStatSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS jobs (
		"id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,		
		%s
	  );`, strings.Join(fieldLines, "\n"))

	level.Info(j.logger).Log("msg", "Creating SQLite DB table for storing job stats")
	statement, err := db.Prepare(createBatchJobStatSQL) // Prepare SQL Statement
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to prepare SQL statement duirng DB creation", "err", err)
		return err
	}
	_, err = statement.Exec() // Execute SQL Statements
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to execute SQL DB creation statement", "err", err)
		return err
	}
	level.Info(j.logger).Log("msg", "SQLite DB table for jobstats successfully created")
	return nil
}

// Make and return prepare statement for inserting entries
func (j *jobStats) getSQLPrepareStatement(db *sql.DB) (*sql.Stmt, error) {
	allFields := utils.GetStructFieldName(batchJob{})
	fieldNamesString := strings.Join(allFields[:], ", ")
	placeholderSlice := make([]string, len(allFields))
	for i := range placeholderSlice {
		placeholderSlice[i] = "?"
	}
	placeholderString := strings.Join(placeholderSlice, ", ")
	insertSQLPlaceholder := fmt.Sprintf("INSERT INTO jobs(%s) VALUES (%s)", fieldNamesString, placeholderString)
	statement, err := db.Prepare(insertSQLPlaceholder)
	if err != nil {
		return nil, err
	}
	return statement, nil
}

// Get start and end time to query for jobs
func (j *jobStats) getStartEndTimes() (time.Time, time.Time) {
	var startTime time.Time
	var foundStartTime bool = false
	if _, err := os.Stat(j.lastTimeStampFile); err == nil {
		timestamp, err := os.ReadFile(j.lastTimeStampFile)
		if err != nil {
			level.Error(j.logger).Log("msg", "Failed to read lasttimestamp file", "err", err)
		} else {
			startTime, err = time.Parse(dateFormat, string(timestamp))
			if err != nil {
				level.Error(j.logger).Log("msg", "Failed to parse lasttimestamp file", "err", err)
			} else {
				foundStartTime = true
			}
		}
	}
	if !foundStartTime {
		// If not lasttimestamp file found, get data from last 365 days
		startTime = time.Now().Add(time.Duration(-8760) * time.Hour)
	}
	endTime := time.Now()
	lastTimeStamp := []byte(endTime.Format(dateFormat))
	err := os.WriteFile(j.lastTimeStampFile, lastTimeStamp, 0644)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to write lasttimestamp file", "err", err)
	}
	return startTime, endTime
}

// Get job stats and insert them into DB
func (j *jobStats) GetJobStats() error {
	// First do basic checks
	j.checks()
	// Get start and end times for retrieving jobs
	startTime, endTime := j.getStartEndTimes()
	var jobs []batchJob
	var err error
	if statsFunc, ok := statsMap[j.batchScheduler]; ok {
		jobs, err = statsFunc.(func(time.Time, time.Time, log.Logger) ([]batchJob, error))(startTime, endTime, j.logger)
		if err != nil {
			return err
		}
	}
	dbConn, err := j.prepareDB()
	if err != nil {
		level.Error(j.logger).Log("msg", "Preparation of DB failed", "err", err)
		return err
	}
	statement, err := j.getSQLPrepareStatement(dbConn)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to prepare insert job statement in DB", "err", err)
		return err
	}
	level.Info(j.logger).Log("msg", "Inserting jobs into DB")
	for _, job := range jobs {
		// Empty job
		if job == (batchJob{}) {
			continue
		}
		j.insertJob(statement, dbConn, job)
	}
	level.Info(j.logger).Log("msg", "Finished inserting jobs into DB")
	// Defer Closing the database
	defer dbConn.Close()
	return nil
}

// Insert job stat into DB
func (j *jobStats) insertJob(statement *sql.Stmt, db *sql.DB, jobStat batchJob) {
	level.Debug(j.logger).Log("msg", "Inserting job", "jobid", jobStat.Jobid)
	_, err := statement.Exec(
		jobStat.Jobid, jobStat.Jobuuid, jobStat.Cluster,
		jobStat.Partition, jobStat.Account, jobStat.Grp, jobStat.Gid,
		jobStat.Usr, jobStat.Uid, jobStat.Submit, jobStat.Eligible, jobStat.Start,
		jobStat.End, jobStat.Elapsed, jobStat.Elapsedraw, jobStat.Exitcode,
		jobStat.State, jobStat.Nnodes, jobStat.Ncpus, jobStat.Reqcpus,
		jobStat.Reqmem, jobStat.Reqtres, jobStat.Timelimit, jobStat.Nodelist,
		jobStat.NodelistExp, jobStat.Jobname, jobStat.Workdir,
	)
	if err != nil {
		level.Error(j.logger).Log("msg", "Failed to insert job in DB", "jobid", jobStat.Jobid, "err", err)
	}
}
