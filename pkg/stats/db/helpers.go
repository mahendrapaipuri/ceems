package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/stats/base"
	"github.com/rotationalio/ensign/pkg/utils/sqlite"
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
	// Key of this map should be name of DB table
	// Inner map key should be name of index and value should be slice of column names
	sqlIndexMap = map[string]map[string][]string{
		base.UnitsDBTableName: {
			"idx_usr_project_start": []string{"usr", "project", "start"},
			"idx_usr_uuid":          []string{"usr", "uuid"},
			"uq_uuid_start":         []string{"uuid", "start"}, // To ensure we dont insert duplicated rows
		},
		base.UsageDBTableName: {
			"uq_project_usr": []string{"project", "usr"},
		},
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
	if err := os.WriteFile(filePath, timeStampByte, 0644); err != nil {
		level.Error(logger).
			Log("msg", "Failed to write timestamp to file", "time", timeStampString, "file", filePath, "err", err)
	}
}

// Create a table for storing unit stats
func createTable(
	dbTableName string,
	dbColumnNames []string,
	dbColumnTypes map[string]string,
	db *sql.DB,
	logger log.Logger,
) error {
	// Iterate through dbColumnNames and access column type from dbColumnTypes map
	// As map iteration in golang does not have order, directly iterating on map will
	// create table in random column order.
	// By iterating through slice and accessing type from map ensures we preserve order
	//
	// Add id column manually as dbColumnNames will not have id column as it is auto incremental column
	fieldLines := []string{fmt.Sprintf(" \"id\" %s,", dbColumnTypes["id"])}
	for _, field := range dbColumnNames {
		fieldLines = append(fieldLines, fmt.Sprintf(" \"%s\" %s,", field, dbColumnTypes[field]))
	}
	fieldLines[len(fieldLines)-1] = strings.Split(fieldLines[len(fieldLines)-1], ",")[0]
	createTableCmd := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (	
 %s
);`, dbTableName, strings.Join(fieldLines, "\n"))

	// Prepare SQL DB creation Statement
	level.Debug(logger).Log("msg", "Creating DB table", "table", dbTableName)
	statement, err := db.Prepare(createTableCmd)
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
	for indexName, indexCols := range sqlIndexMap[dbTableName] {
		var createIndexSQL string
		if strings.HasPrefix(indexName, "uq") {
			createIndexSQL = fmt.Sprintf(
				"CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)",
				indexName,
				dbTableName,
				strings.Join(indexCols, ","),
			)
		} else {
			createIndexSQL = fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", indexName, dbTableName, strings.Join(indexCols, ","))
		}
		level.Info(logger).Log("msg", "Creating DB index", "statement", createIndexSQL, "table", dbTableName)
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
	level.Info(logger).Log("msg", "DB table successfully created", "table", dbTableName)
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
func setupDB(dbFilePath string, logger log.Logger) (*sql.DB, *sqlite.Conn, error) {
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

	// Create Table for Unitstats
	if err = createTable(base.UnitsDBTableName, base.UnitsDBTableColNames, base.UnitsDBTableColTypeMap, db, logger); err != nil {
		level.Error(logger).Log("msg", "Failed to create DB table", "table", base.UnitsDBTableName, "err", err)
		return nil, nil, err
	}
	// Create Table for Usage
	if err = createTable(base.UsageDBTableName, base.UsageDBTableColNames, base.UsageDBTableColTypeMap, db, logger); err != nil {
		level.Error(logger).Log("msg", "Failed to create DB table", "table", base.UsageDBTableName, "err", err)
		return nil, nil, err
	}
	// // Create Table for Userstats
	// if err = createTable(base.UsersDBTableName, base.UsersDBColNames, base.UsersDBTableMap, db, logger); err != nil {
	// 	level.Error(logger).Log("msg", "Failed to create DB table", "table", base.UsersDBTableName, "err", err)
	// 	return nil, nil, err
	// }
	return db, dbConn, nil
}
