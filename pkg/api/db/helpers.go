package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	ceems_sqlite3 "github.com/mahendrapaipuri/ceems/pkg/sqlite3"
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
	if err := os.WriteFile(filePath, timeStampByte, 0600); err != nil {
		level.Error(logger).
			Log("msg", "Failed to write timestamp to file", "time", timeStampString, "file", filePath, "err", err)
	}
}

// Open DB connection and return connection poiner
func openDBConnection(dbFilePath string) (*sql.DB, *ceems_sqlite3.Conn, error) {
	var db *sql.DB
	var dbConn *ceems_sqlite3.Conn
	var err error
	var ok bool
	if db, err = sql.Open(ceems_sqlite3.DriverName, makeDSN(dbFilePath, defaultOpts)); err != nil {
		return nil, nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, nil, err
	}

	if dbConn, ok = ceems_sqlite3.GetLastConn(); !ok {
		return nil, nil, err
	}
	return db, dbConn, nil
}

// Setup DB and create table
func setupDB(dbFilePath string, logger log.Logger) (*sql.DB, *ceems_sqlite3.Conn, error) {
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
	return db, dbConn, nil
}
