//go:build cgo
// +build cgo

package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	ceems_sqlite3 "github.com/ceems-dev/ceems/pkg/sqlite3"
)

// Ref: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
// Ref: https://gitlab.com/gnufred/logslate/-/blob/8eda5cedc9a28da3793dcf73480d618c95cc322c/playground/sqlite3.go
// Ref: https://github.com/mattn/go-sqlite3/issues/1145#issuecomment-1519012055
// Use WAL as journal mode by default as using litestream alongside ceems API server can result
// in DB locked problem when restarting CEEMS API server. This is due to the starting DB connection
// attempts to open DB in DELETE journal mode which cannot be possible when WAL is activated by
// litestream.
var defaultOpts = map[string]string{
	"_busy_timeout": "5000",
	"_journal_mode": "WAL",
	"_synchronous":  "0",
}

// Make DSN from DB file path and opts map.
func makeDSN(filePath string, opts map[string]string) string {
	dsn := "file:" + filePath

	optsSlice := []string{}
	for opt, val := range opts {
		optsSlice = append(optsSlice, fmt.Sprintf("%s=%s", opt, val))
	}

	optString := strings.Join(optsSlice, "&")

	return fmt.Sprintf("%s?%s", dsn, optString)
}

// Open DB connection and return connection poiner.
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

// Setup DB and create table.
func setupDB(dbFilePath string) (*sql.DB, *ceems_sqlite3.Conn, error) {
	if _, err := os.Stat(dbFilePath); err == nil {
		// Open the created SQLite File
		db, dbConn, err := openDBConnection(dbFilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open DB file: %w", err)
		}

		return db, dbConn, nil
	}

	// If file does not exist, create SQLite file
	file, err := os.Create(dbFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create DB file: %w", err)
	}

	file.Close()

	// Set strict permissions
	if err := os.Chmod(dbFilePath, 0o640); err != nil {
		return nil, nil, fmt.Errorf("failed to harden permissions on DB file: %w", err)
	}

	// Open the created SQLite File
	db, dbConn, err := openDBConnection(dbFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open DB connection: %w", err)
	}

	return db, dbConn, nil
}
