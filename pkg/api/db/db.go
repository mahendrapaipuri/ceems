// Package db creates DB tables, call resource manager interfaces and
// populates the DB with compute units
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/mattn/go-sqlite3"
	"github.com/rotationalio/ensign/pkg/utils/sqlite"
)

// Directory containing DB migrations
const migrationsDir = "migrations"

//go:embed migrations/*.sql
var MigrationsFS embed.FS

// Config makes a DB config from CLI args
type Config struct {
	Logger               log.Logger
	DataPath             string
	DataBackupPath       string
	RetentionPeriod      time.Duration
	SkipDeleteOldUnits   bool
	LastUpdateTimeString string
	ResourceManager      func(log.Logger) (*resource.Manager, error)
	Updater              func(log.Logger) (*updater.UnitUpdater, error)
	TSDB                 *tsdb.TSDB
}

// Storage
type storageConfig struct {
	dbPath             string
	dbBackupPath       string
	retentionPeriod    time.Duration
	cutoffPeriod       time.Duration
	lastUpdateTime     time.Time
	lastUpdateTimeFile string
	skipDeleteOldUnits bool
}

// Stringer receiver for storageConfig
func (s *storageConfig) String() string {
	return fmt.Sprintf(
		"storageConfig{dbPath: %s, retentionPeriod: %s, cutoffPeriod: %s, "+
			"lastUpdateTime: %s, lastUpdateTimeFile: %s}",
		s.dbPath, s.retentionPeriod, s.cutoffPeriod, s.lastUpdateTime,
		s.lastUpdateTimeFile,
	)
}

// statsDB struct
type statsDB struct {
	logger  log.Logger
	db      *sql.DB
	dbConn  *sqlite.Conn
	manager *resource.Manager
	updater *updater.UnitUpdater
	storage *storageConfig
}

// SQLite DB related constant vars
const (
	sqlite3Driver = "ensign_sqlite3"
	sqlite3Main   = "main"
	pagesPerStep  = 25
	stepSleep     = 50 * time.Millisecond
)

var (
	prepareStatements = make(map[string]string, 3)
)

// Init func to set prepareStatements
func init() {
	// Unit update statement placeholders
	var unitTablePlaceholders []string
	for _, col := range base.UnitsDBTableColNames {
		if strings.HasPrefix(col, "num") {
			unitTablePlaceholders = append(unitTablePlaceholders, fmt.Sprintf("  %[1]s = %[1]s + ?", col))
		} else if strings.HasPrefix(col, "avg") {
			unitTablePlaceholders = append(unitTablePlaceholders, fmt.Sprintf("  %[1]s = (%[1]s * num_intervals + ?) / (num_intervals + 1)", col))
		} else if strings.HasPrefix(col, "total") {
			unitTablePlaceholders = append(unitTablePlaceholders, fmt.Sprintf("  %[1]s = (%[1]s + ?)", col))
			// We will need to update end time, elapsed time and state as they change with time
		} else if slices.Contains([]string{"ended_at", "ended_at_ts", "elapsed", "elapsed_raw", "state", "tags"}, col) {
			unitTablePlaceholders = append(unitTablePlaceholders, fmt.Sprintf("  %[1]s = ?", col))
		} else {
			continue
		}
	}

	// Unit update statement
	unitStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s %s",
		base.UnitsDBTableName,
		strings.Join(base.UnitsDBTableColNames, ","),
		fmt.Sprintf(
			"(%s)",
			strings.Join(strings.Split(strings.Repeat("?", len(base.UnitsDBTableColNames)), ""), ","),
		),
		"ON CONFLICT(uuid,started_at) DO UPDATE SET", // Index is defined in 000001_create_unit_table.up.sql
	)

	prepareStatements[base.UnitsDBTableName] = strings.Join(
		[]string{
			unitStmt,
			strings.Join(unitTablePlaceholders, ",\n"),
		},
		"\n",
	)

	// Usage update statement
	var usageTablePlaceholders []string
	for _, col := range base.UsageDBTableColNames {
		if strings.HasPrefix(col, "num") {
			usageTablePlaceholders = append(usageTablePlaceholders, fmt.Sprintf("  %[1]s = %[1]s + ?", col))
		} else if strings.HasPrefix(col, "avg") {
			usageTablePlaceholders = append(usageTablePlaceholders, fmt.Sprintf("  %[1]s = (%[1]s * num_units + ?) / (num_units + 1)", col))
		} else if strings.HasPrefix(col, "total") {
			usageTablePlaceholders = append(usageTablePlaceholders, fmt.Sprintf("  %[1]s = (%[1]s + ?)", col))
		} else if col == "tags" {
			usageTablePlaceholders = append(usageTablePlaceholders, fmt.Sprintf("  %[1]s = ?", col))
		} else {
			continue
		}
	}

	// Usage update statement
	usageStmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s %s",
		base.UsageDBTableName,
		strings.Join(base.UsageDBTableColNames, ","),
		fmt.Sprintf(
			"(%s)",
			strings.Join(strings.Split(strings.Repeat("?", len(base.UsageDBTableColNames)), ""), ","),
		),
		"ON CONFLICT(usr,project) DO UPDATE SET", // Index is defined in 000002_create_usage_table.up.sql
	)
	prepareStatements[base.UsageDBTableName] = strings.Join(
		[]string{
			usageStmt,
			strings.Join(usageTablePlaceholders, ",\n"),
		},
		"\n",
	)
}

// NewStatsDB returns a new instance of statsDB struct
func NewStatsDB(c *Config) (*statsDB, error) {
	var lastUpdateTime time.Time
	var err error

	// Get file paths
	dbPath := filepath.Join(c.DataPath, fmt.Sprintf("%s.db", base.CEEMSServerAppName))
	lastUpdateTimeStampFile := filepath.Join(c.DataPath, "lastupdatetime")

	// By this time dataPath **should** exist and we do not need to check for its
	// existence. Check directly for lastupdatetime file
	if _, err := os.Stat(lastUpdateTimeStampFile); err == nil {
		lastUpdateTimeString, err := os.ReadFile(lastUpdateTimeStampFile)
		if err != nil {
			level.Error(c.Logger).Log("msg", "Failed to read lastupdatetime file", "err", err)
			goto updatetime
		} else {
			// Trim any spaces and new lines
			lastUpdateTime, err = time.Parse(base.DatetimeLayout, strings.TrimSuffix(strings.TrimSpace(string(lastUpdateTimeString)), "\n"))
			if err != nil {
				level.Error(c.Logger).Log("msg", "Failed to parse time string in lastupdatetime file", "time", lastUpdateTimeString, "err", err)
				goto updatetime
			}
		}
		goto setup
	} else {
		goto updatetime
	}

updatetime:
	if lastUpdateTime, err = time.Parse("2006-01-02", c.LastUpdateTimeString); err != nil {
		level.Error(c.Logger).Log("msg", "Failed to parse time string", "time", c.LastUpdateTimeString, "err", err)
		return nil, err
	}

	// Write to file for persistence in case of restarts
	writeTimeStampToFile(lastUpdateTimeStampFile, lastUpdateTime, c.Logger)

setup:
	// Setup manager struct that retrieves unit data
	manager, err := c.ResourceManager(c.Logger)
	if err != nil {
		level.Error(c.Logger).Log("msg", "Resource manager setup failed", "err", err)
		return nil, err
	}

	// Setup updater struct that updates units
	updater, err := c.Updater(c.Logger)
	if err != nil {
		level.Error(c.Logger).Log("msg", "Metric updater setup failed", "err", err)
		return nil, err
	}

	// Setup DB
	db, dbConn, err := setupDB(dbPath, c.Logger)
	if err != nil {
		level.Error(c.Logger).Log("msg", "DB setup failed", "err", err)
		return nil, err
	}

	// Setup Migrator
	migrator, err := NewMigrator(MigrationsFS, migrationsDir, c.Logger)
	if err != nil {
		return nil, err
	}

	// Perform DB migrations
	if err = migrator.ApplyMigrations(db); err != nil {
		return nil, err
	}

	// Now make an instance of time.Date with proper format and zone
	lastUpdateTime = time.Date(
		lastUpdateTime.Year(),
		lastUpdateTime.Month(),
		lastUpdateTime.Day(),
		lastUpdateTime.Hour(),
		lastUpdateTime.Minute(),
		lastUpdateTime.Second(),
		lastUpdateTime.Nanosecond(),
		time.Now().Location(),
	)

	// Storage config
	storageConfig := &storageConfig{
		dbPath:             dbPath,
		dbBackupPath:       c.DataBackupPath,
		retentionPeriod:    c.RetentionPeriod,
		lastUpdateTime:     lastUpdateTime,
		lastUpdateTimeFile: lastUpdateTimeStampFile,
		skipDeleteOldUnits: c.SkipDeleteOldUnits,
	}

	// Emit debug logs
	level.Debug(c.Logger).Log("msg", "Storage config", "cfg", storageConfig)
	return &statsDB{
		logger:  c.Logger,
		db:      db,
		dbConn:  dbConn,
		manager: manager,
		updater: updater,
		storage: storageConfig,
	}, nil
}

// Collect unit stats
func (s *statsDB) Collect() error {
	var currentTime = time.Now()

	// If duration is less than 1 day do single update
	if currentTime.Sub(s.storage.lastUpdateTime) < time.Duration(24*time.Hour) {
		return s.getJobStats(s.storage.lastUpdateTime, currentTime)
	}
	level.Info(s.logger).
		Log("msg", "DB update duration is more than 1 day. Doing incremental update. This may take a while...")

	// If duration is more than 1 day, do incremental update
	var nextUpdateTime time.Time
	for {
		nextUpdateTime = s.storage.lastUpdateTime.Add(24 * time.Hour)
		if nextUpdateTime.Compare(currentTime) == -1 {
			level.Debug(s.logger).
				Log("msg", "Incremental DB update step", "from", s.storage.lastUpdateTime, "to", nextUpdateTime)
			if err := s.getJobStats(s.storage.lastUpdateTime, nextUpdateTime); err != nil {
				level.Error(s.logger).
					Log("msg", "Failed incremental update", "from", s.storage.lastUpdateTime, "to", nextUpdateTime, "err", err)
				return err
			}
		} else {
			level.Debug(s.logger).Log("msg", "Final incremental DB update step", "from", s.storage.lastUpdateTime, "to", currentTime)
			return s.getJobStats(s.storage.lastUpdateTime, currentTime)
		}

		// Sleep for couple of seconds before making next update
		// This is to let DB breath a bit before serving next request
		time.Sleep(time.Second)
	}
}

// Backup DB
func (s *statsDB) Backup() error {
	return s.createBackup()
}

// Close DB connection
func (s *statsDB) Stop() error {
	return s.db.Close()
}

// Get unit stats and insert them into DB
func (s *statsDB) getJobStats(startTime, endTime time.Time) error {
	// Retrieve units from unerlying resource manager
	units, err := s.manager.Fetch(startTime, endTime)
	if err != nil {
		return err
	}

	// Update units struct with unit level metrics from TSDB
	units = s.updater.Update(startTime, endTime, units)

	// Begin transcation
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin SQL transcation: %s", err)
	}

	// Delete older entries and free up DB pages
	// In testing we want to skip this
	if !s.storage.skipDeleteOldUnits {
		level.Debug(s.logger).Log("msg", "Cleaning up old units")
		if err = s.purgeExpiredUnits(tx); err != nil {
			level.Error(s.logger).Log("msg", "Failed to clean up old unit entries", "err", err)
		} else {
			level.Debug(s.logger).Log("msg", "Cleaned up old units in DB")
		}
	}

	// Make prepare statement and defer closing statement
	level.Debug(s.logger).Log("msg", "Preparing SQL statements")
	stmtMap, err := s.prepareStatements(tx)
	if err != nil {
		return err
	}
	for _, stmt := range stmtMap {
		defer stmt.Close()
	}
	level.Debug(s.logger).Log("msg", "Finished preparing SQL statements")

	// Insert data into DB
	level.Debug(s.logger).Log("msg", "Executing SQL statements")
	s.execStatements(stmtMap, units)
	level.Debug(s.logger).Log("msg", "Finished executing SQL statements")

	// Commit changes
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit SQL transcation: %s", err)
	}

	// Write endTime to file to keep track upon successful insertion of data
	s.storage.lastUpdateTime = endTime
	writeTimeStampToFile(s.storage.lastUpdateTimeFile, s.storage.lastUpdateTime, s.logger)
	return nil
}

// Delete old entries in DB
func (s *statsDB) purgeExpiredUnits(tx *sql.Tx) error {
	deleteRowQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE started_at <= date('now', '-%d day')",
		base.UnitsDBTableName,
		int(s.storage.retentionPeriod.Hours()/24),
	)
	if _, err := tx.Exec(deleteRowQuery); err != nil {
		return err
	}

	// Get changes
	var rowsDeleted int
	_ = tx.QueryRow("SELECT changes();").Scan(&rowsDeleted)
	level.Debug(s.logger).Log("units_deleted", rowsDeleted)
	return nil
}

// Make and return a map of prepare statements for inserting entries into different
// DB tables. The key of map is DB table name and value will be pointer to statement
func (s *statsDB) prepareStatements(tx *sql.Tx) (map[string]*sql.Stmt, error) {
	var stmts = make(map[string]*sql.Stmt, len(prepareStatements))
	var err error
	for dbTable, stmt := range prepareStatements {
		stmts[dbTable], err = tx.Prepare(stmt)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare statement for table %s: %s", dbTable, err)
		}
	}
	return stmts, nil
}

// Insert unit stat into DB
func (s *statsDB) execStatements(statements map[string]*sql.Stmt, units []models.Unit) {
	var ignore = 0
	var err error
	for _, unit := range units {
		// Empty unit
		if unit.UUID == "" {
			continue
		}

		// level.Debug(s.logger).Log("msg", "Inserting unit", "id", unit.Jobid)
		if _, err = statements[base.UnitsDBTableName].Exec(
			unit.UUID,
			unit.Name,
			unit.Project,
			unit.Grp,
			unit.Usr,
			unit.CreatedAt,
			unit.StartedAt,
			unit.EndedAt,
			unit.CreatedAtTS,
			unit.StartedAtTS,
			unit.EndedAtTS,
			unit.Elapsed,
			unit.ElapsedRaw,
			unit.State,
			unit.Allocation,
			unit.TotalCPUBilling,
			unit.TotalGPUBilling,
			unit.TotalMiscBilling,
			unit.AveCPUUsage,
			unit.AveCPUMemUsage,
			unit.TotalCPUEnergyUsage,
			unit.TotalCPUEmissions,
			unit.AveGPUUsage,
			unit.AveGPUMemUsage,
			unit.TotalGPUEnergyUsage,
			unit.TotalGPUEmissions,
			unit.TotalIOWriteHot,
			unit.TotalIOReadHot,
			unit.TotalIOWriteCold,
			unit.TotalIOReadCold,
			unit.TotalIngress,
			unit.TotalOutgress,
			unit.Tags,
			ignore,
			1, // NumIntervals
			unit.EndedAt,
			unit.EndedAtTS,
			unit.Elapsed,
			unit.ElapsedRaw,
			unit.State,
			unit.TotalCPUBilling,
			unit.TotalGPUBilling,
			unit.TotalMiscBilling,
			unit.AveCPUUsage,
			unit.AveCPUMemUsage,
			unit.TotalCPUEnergyUsage,
			unit.TotalCPUEmissions,
			unit.AveGPUUsage,
			unit.AveGPUMemUsage,
			unit.TotalGPUEnergyUsage,
			unit.TotalGPUEmissions,
			unit.TotalIOWriteHot,
			unit.TotalIOReadHot,
			unit.TotalIOWriteCold,
			unit.TotalIOReadCold,
			unit.TotalIngress,
			unit.TotalOutgress,
			unit.Tags,
			1, // NumIntervals
		); err != nil {
			level.Error(s.logger).
				Log("msg", "Failed to insert unit in DB", "id", unit.UUID, "err", err)
		}

		// If unit.EndTS is zero, it means a running unit. We shouldnt update stats
		// of running units. They should be updated **ONLY** for finished units
		unitIncr := 1
		if unit.EndedAtTS == 0 {
			unitIncr = 0
		}

		// Update Usage table
		if _, err = statements[base.UsageDBTableName].Exec(
			unitIncr,
			unit.Project,
			unit.Usr,
			unit.TotalCPUBilling,
			unit.TotalGPUBilling,
			unit.TotalMiscBilling,
			unit.AveCPUUsage,
			unit.AveCPUMemUsage,
			unit.TotalCPUEnergyUsage,
			unit.TotalCPUEmissions,
			unit.AveGPUUsage,
			unit.AveGPUMemUsage,
			unit.TotalGPUEnergyUsage,
			unit.TotalGPUEmissions,
			unit.TotalIOWriteHot,
			unit.TotalIOReadHot,
			unit.TotalIOWriteCold,
			unit.TotalIOReadCold,
			unit.TotalIngress,
			unit.TotalOutgress,
			unitIncr,
			unit.TotalCPUBilling,
			unit.TotalGPUBilling,
			unit.TotalMiscBilling,
			unit.AveCPUUsage,
			unit.AveCPUMemUsage,
			unit.TotalCPUEnergyUsage,
			unit.TotalCPUEmissions,
			unit.AveGPUUsage,
			unit.AveGPUMemUsage,
			unit.TotalGPUEnergyUsage,
			unit.TotalGPUEmissions,
			unit.TotalIOWriteHot,
			unit.TotalIOReadHot,
			unit.TotalIOWriteCold,
			unit.TotalIOReadCold,
			unit.TotalIngress,
			unit.TotalOutgress,
		); err != nil {
			level.Error(s.logger).
				Log("msg", "Failed to update usage table in DB", "id", unit.UUID, "err", err)
		}
	}
}

// Backup executes the sqlite3 backup strategy
// Based on https://gist.github.com/bbengfort/452a9d5e74a63d88e5a34a580d6cb6d3
// Ref: https://github.com/rotationalio/ensign/pull/529/files
func (s *statsDB) backup(backupDBPath string) error {
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
	if backup, err = destConn.Backup(sqlite3Main, s.dbConn, sqlite3Main); err != nil {
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
			if finishErr := backup.Finish(); finishErr != nil {
				return fmt.Errorf("errors: %s, %s", err, finishErr)
			}
			return err
		}

		level.Debug(s.logger).
			Log("msg", "DB backup step", "remaining", backup.Remaining(), "page_count", backup.PageCount())

		// This sleep allows other transactions to write during backups.
		time.Sleep(stepSleep)
	}
	return backup.Finish()
}

// vacuum executes sqlite3 vacuum command
func (s *statsDB) vacuum() error {
	level.Debug(s.logger).Log("msg", "Starting to vacuum DB")
	if _, err := s.db.Exec("VACUUM;"); err != nil {
		return err
	}
	return nil
}

// Creates backup of DB after vacuuming DB
func (s *statsDB) createBackup() error {
	// First vacuum DB to reduce size
	if err := s.vacuum(); err != nil {
		level.Warn(s.logger).Log("msg", "Failed to vacuum DB", "err", err)
	}
	level.Debug(s.logger).Log("msg", "DB vacuumed")

	// Attempt to create in-place DB backup
	// Make a unique backup file name using current time
	backupDBFileName := fmt.Sprintf("%s-%s.bak.db", base.CEEMSServerAppName, time.Now().Format("200601021504"))
	backupDBFilePath := filepath.Join(filepath.Dir(s.storage.dbPath), backupDBFileName)
	if err := s.backup(backupDBFilePath); err != nil {
		return err
	}

	// If back is successful, move it to dbBackupPath
	err := os.Rename(backupDBFilePath, filepath.Join(s.storage.dbBackupPath, backupDBFileName))
	if err != nil {
		return fmt.Errorf("failed to move backup DB file: %s", err)
	}
	level.Info(s.logger).Log("msg", "DB backed up", "file", backupDBFileName)
	return nil
}
