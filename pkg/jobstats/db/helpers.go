package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/rotationalio/ensign/pkg/utils/sqlite"
)

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
	level.Debug(logger).Log("msg", "Creating DB table for storing job stats")
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
	level.Info(logger).Log("msg", "DB table(s) successfully created")
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
