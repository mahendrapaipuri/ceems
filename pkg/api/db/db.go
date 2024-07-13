// Package db creates DB tables, call resource manager interfaces and
// populates the DB with compute units
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	ceems_sqlite3 "github.com/mahendrapaipuri/ceems/pkg/sqlite3"
	"github.com/mattn/go-sqlite3"
	"github.com/prometheus/common/model"
)

// Directory containing DB related files
const (
	migrationsDir = "migrations"
	statementsDir = "statements"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

//go:embed statements/*.sql
var StatementsFS embed.FS

// AdminConfig is the container for the admin users related config
type AdminConfig struct {
	Users   []string                `yaml:"users"`
	Grafana common.GrafanaWebConfig `yaml:"grafana"`
}

// DataConfig is the container for the data related config
type DataConfig struct {
	Path               string         `yaml:"path"`
	BackupPath         string         `yaml:"backup_path"`
	RetentionPeriod    model.Duration `yaml:"retention_period"`
	UpdateInterval     model.Duration `yaml:"update_interval"`
	BackupInterval     model.Duration `yaml:"backup_interval"`
	LastUpdateTime     time.Time      `yaml:"update_from"`
	SkipDeleteOldUnits bool
}

// Config makes a DB config from config file
type Config struct {
	Logger          log.Logger
	Data            DataConfig
	Admin           AdminConfig
	ResourceManager func(log.Logger) (*resource.Manager, error)
	Updater         func(log.Logger) (*updater.UnitUpdater, error)
}

// storageConfig is the container for storage related config
type storageConfig struct {
	dbPath             string
	dbBackupPath       string
	retentionPeriod    time.Duration
	lastUpdateTime     time.Time
	lastUpdateTimeFile string
	skipDeleteOldUnits bool
}

// String implements Stringer interface for storageConfig
func (s *storageConfig) String() string {
	return fmt.Sprintf(
		"dbPath: %s; retentionPeriod: %s; lastUpdateTime: %s; lastUpdateTimeFile: %s",
		s.dbPath, s.retentionPeriod, s.lastUpdateTime, s.lastUpdateTimeFile,
	)
}

type adminConfig struct {
	users                map[string]models.List // Map of admin users from different sources
	grafana              *grafana.Grafana
	grafanaAdminTeamsIDs []string
}

// statsDB struct
type statsDB struct {
	logger  log.Logger
	db      *sql.DB
	dbConn  *ceems_sqlite3.Conn
	manager *resource.Manager
	updater *updater.UnitUpdater
	storage *storageConfig
	admin   *adminConfig
}

// SQLite DB related constant vars
const (
	sqlite3Main  = "main"
	pagesPerStep = 25
	stepSleep    = 50 * time.Millisecond
)

var (
	prepareStatements = make(map[string]string)

	// For estimating average values, we do weighted average method using following
	// values as weight for each DB column
	// For CPU and GPU, we use CPU time and GPU time as weights
	// For memory usage, we use Walltime * Mem for CPU as weight and just walltime
	// for GPU
	Weights = map[string]string{
		"avg_cpu_usage":     "alloc_cputime",
		"avg_gpu_usage":     "alloc_gputime",
		"avg_cpu_mem_usage": "alloc_cpumemtime",
		"avg_gpu_mem_usage": "alloc_gpumemtime",
	}

	// Admin users sources
	AdminUsersSources = []string{"ceems", "grafana"}
)

// Init func to set prepareStatements
func init() {
	for _, tableName := range []string{base.UnitsDBTableName, base.UsageDBTableName, base.AdminUsersDBTableName, base.UsersDBTableName, base.ProjectsDBTableName} {
		statements, err := StatementsFS.ReadFile(fmt.Sprintf("statements/%s.sql", tableName))
		if err != nil {
			panic(fmt.Sprintf("failed to read SQL statements file for table %s: %s", tableName, err))
		}
		prepareStatements[tableName] = string(statements)
	}
}

// NewStatsDB returns a new instance of statsDB struct
func NewStatsDB(c *Config) (*statsDB, error) {
	var err error

	// Get file paths
	dbPath := filepath.Join(c.Data.Path, base.CEEMSDBName)
	lastUpdateTimeStampFile := filepath.Join(c.Data.Path, "lastupdatetime")

	// By this time dataPath **should** exist and we do not need to check for its
	// existence. Check directly for lastupdatetime file
	if _, err := os.Stat(lastUpdateTimeStampFile); err == nil {
		lastUpdateTimeString, err := os.ReadFile(lastUpdateTimeStampFile)
		if err != nil {
			level.Error(c.Logger).Log("msg", "Failed to read lastupdatetime file", "err", err)
			goto updatetime
		} else {
			// Trim any spaces and new lines
			c.Data.LastUpdateTime, err = time.Parse(base.DatetimeLayout, strings.TrimSuffix(strings.TrimSpace(string(lastUpdateTimeString)), "\n"))
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
	// Write to file for persistence in case of restarts
	writeTimeStampToFile(lastUpdateTimeStampFile, c.Data.LastUpdateTime, c.Logger)

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
		level.Error(c.Logger).Log("msg", "Updater setup failed", "err", err)
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
	c.Data.LastUpdateTime = time.Date(
		c.Data.LastUpdateTime.Year(),
		c.Data.LastUpdateTime.Month(),
		c.Data.LastUpdateTime.Day(),
		c.Data.LastUpdateTime.Hour(),
		c.Data.LastUpdateTime.Minute(),
		c.Data.LastUpdateTime.Second(),
		c.Data.LastUpdateTime.Nanosecond(),
		time.Now().Location(),
	)

	// Create a new instance of Grafana client
	grafanaClient, err := common.CreateGrafanaClient(&c.Admin.Grafana, c.Logger)
	if err != nil {
		return nil, err
	}

	// Make admin users map
	adminUsers := make(map[string]models.List, len(AdminUsersSources))
	// for _, source := range AdminUsersSources {
	// 	adminUsers[source] = make([]string, 0)
	// }
	for _, user := range c.Admin.Users {
		adminUsers["ceems"] = append(adminUsers["ceems"], user)
	}

	// Admin config
	adminConfig := &adminConfig{
		users:                adminUsers,
		grafana:              grafanaClient,
		grafanaAdminTeamsIDs: c.Admin.Grafana.TeamsIDs,
	}

	// Storage config
	storageConfig := &storageConfig{
		dbPath:             dbPath,
		dbBackupPath:       c.Data.BackupPath,
		retentionPeriod:    time.Duration(c.Data.RetentionPeriod),
		lastUpdateTime:     c.Data.LastUpdateTime,
		lastUpdateTimeFile: lastUpdateTimeStampFile,
		skipDeleteOldUnits: c.Data.SkipDeleteOldUnits,
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
		admin:   adminConfig,
	}, nil
}

// Collect unit stats
func (s *statsDB) Collect() error {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "Data collection", s.logger)

	var currentTime = time.Now()

	// If duration is less than 1 day do single update
	if currentTime.Sub(s.storage.lastUpdateTime) < time.Duration(24*time.Hour) {
		return s.getUnitStats(s.storage.lastUpdateTime, currentTime)
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
			if err := s.getUnitStats(s.storage.lastUpdateTime, nextUpdateTime); err != nil {
				level.Error(s.logger).
					Log("msg", "Failed incremental update", "from", s.storage.lastUpdateTime, "to", nextUpdateTime, "err", err)
				return err
			}
		} else {
			level.Debug(s.logger).Log("msg", "Final incremental DB update step", "from", s.storage.lastUpdateTime, "to", currentTime)
			return s.getUnitStats(s.storage.lastUpdateTime, currentTime)
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

// updateAdminUsers updates the static list of admin users with the ones fetched
// from Grafana teams
func (s *statsDB) updateAdminUsers() error {
	// If no teams IDs are configured or Grafana is not online, return
	if s.admin.grafanaAdminTeamsIDs == nil || !s.admin.grafana.Available() {
		return nil
	}

	users, err := s.admin.grafana.TeamMembers(s.admin.grafanaAdminTeamsIDs)
	if err != nil {
		return err
	}
	for _, u := range users {
		s.admin.users["grafana"] = append(s.admin.users["grafana"], u)
	}
	return nil
}

// Get unit stats and insert them into DB
func (s *statsDB) getUnitStats(startTime, endTime time.Time) error {
	// Retrieve units from underlying resource manager(s)
	// Return error only if **all** resource manager(s) failed
	units, err := s.manager.FetchUnits(startTime, endTime)
	if len(units) == 0 && err != nil {
		return err
	}
	// If atleast one manager passed, and there are failed ones, log the errors
	if err != nil {
		level.Error(s.logger).Log("msg", "Fetching units from atleast one resource manager failed", "err", err)
	}

	// Fetch current users and projects
	// Return error only if **all** resource manager(s) failed
	users, projects, err := s.manager.FetchUsersProjects(endTime)
	if len(users) == 0 && len(projects) == 0 && err != nil {
		return err
	}
	// If atleast one manager passed, and there are failed ones, log the errors
	if err != nil {
		level.Error(s.logger).Log("msg", "Fetching associations from atleast one resource manager failed", "err", err)
	}

	// Update units struct with unit level metrics from TSDB
	units = s.updater.Update(startTime, endTime, units)

	// Update admin users list from Grafana
	if err := s.updateAdminUsers(); err != nil {
		level.Error(s.logger).Log("msg", "Failed to update admin users from Grafana", "err", err)
	}

	// Begin transcation
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin SQL transcation: %s", err)
	}

	// Delete older entries and free up DB pages
	// In testing we want to skip this
	if !s.storage.skipDeleteOldUnits {
		level.Debug(s.logger).Log("msg", "Cleaning up old entries in DB")
		if err = s.purgeExpiredUnits(tx); err != nil {
			level.Error(s.logger).Log("msg", "Failed to clean up old entries", "err", err)
		} else {
			level.Debug(s.logger).Log("msg", "Cleaned up old entries in DB")
		}
	}

	// Make prepare statement and defer closing statement
	level.Debug(s.logger).Log("msg", "Preparing SQL statements")
	sqlStmts, err := s.prepareStatements(tx)
	if err != nil {
		return err
	}
	for _, stmt := range sqlStmts {
		defer stmt.Close()
	}
	level.Debug(s.logger).Log("msg", "Finished preparing SQL statements")

	// Insert data into DB
	level.Debug(s.logger).Log("msg", "Executing SQL statements")
	s.execStatements(sqlStmts, endTime, units, users, projects)
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
	// Purge expired units
	deleteUnitsQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE started_at <= date('now', '-%d day')",
		base.UnitsDBTableName,
		int(s.storage.retentionPeriod.Hours()/24),
	) // #nosec
	if _, err := tx.Exec(deleteUnitsQuery); err != nil {
		return err
	}

	// Get changes
	var unitsDeleted int
	_ = tx.QueryRow("SELECT changes();").Scan(&unitsDeleted)
	level.Debug(s.logger).Log("units_deleted", unitsDeleted)

	// Purge stale usage data
	deleteUsageQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE last_updated_at <= date('now', '-%d day')",
		base.UsageDBTableName,
		int(s.storage.retentionPeriod.Hours()/24),
	) // #nosec
	if _, err := tx.Exec(deleteUsageQuery); err != nil {
		return err
	}

	// Get changes
	var usageDeleted int
	_ = tx.QueryRow("SELECT changes();").Scan(&usageDeleted)
	level.Debug(s.logger).Log("usage_deleted", usageDeleted)
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
func (s *statsDB) execStatements(
	statements map[string]*sql.Stmt,
	currentTime time.Time,
	clusterUnits []models.ClusterUnits,
	clusterUsers []models.ClusterUsers,
	clusterProjects []models.ClusterProjects,
) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "DB insertion", s.logger)

	var ignore = 0
	var err error
	for _, cluster := range clusterUnits {
		for _, unit := range cluster.Units {
			// Empty unit
			if unit.UUID == "" {
				continue
			}

			// level.Debug(s.logger).Log("msg", "Inserting unit", "id", unit.Jobid)
			// Use named parameters to not to repeat the values
			if _, err = statements[base.UnitsDBTableName].Exec(
				sql.Named(base.UnitsDBTableStructFieldColNameMap["ResourceManager"], unit.ResourceManager),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["UUID"], unit.UUID),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Name"], unit.Name),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Project"], unit.Project),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Group"], unit.Group),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["User"], unit.User),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["CreatedAt"], unit.CreatedAt),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["StartedAt"], unit.StartedAt),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["EndedAt"], unit.EndedAt),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["CreatedAtTS"], unit.CreatedAtTS),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["StartedAtTS"], unit.StartedAtTS),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["EndedAtTS"], unit.EndedAtTS),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Elapsed"], unit.Elapsed),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["State"], unit.State),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Allocation"], unit.Allocation),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalTime"], unit.TotalTime),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["AveCPUUsage"], unit.AveCPUUsage),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["AveCPUMemUsage"], unit.AveCPUMemUsage),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalCPUEnergyUsage"], unit.TotalCPUEnergyUsage),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalCPUEmissions"], unit.TotalCPUEmissions),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["AveGPUUsage"], unit.AveGPUUsage),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["AveGPUMemUsage"], unit.AveGPUMemUsage),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalGPUEnergyUsage"], unit.TotalGPUEnergyUsage),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalGPUEmissions"], unit.TotalGPUEmissions),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalIOWriteStats"], unit.TotalIOWriteStats),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalIOReadStats"], unit.TotalIOReadStats),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalIngressStats"], unit.TotalIngressStats),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalOutgressStats"], unit.TotalOutgressStats),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Tags"], unit.Tags),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["ignore"], ignore),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["numupdates"], 1),
				sql.Named(base.UsageDBTableStructFieldColNameMap["lastupdatedat"], currentTime.Format(base.DatetimeLayout)),
			); err != nil {
				level.Error(s.logger).
					Log("msg", "Failed to insert unit in DB", "cluster_id", cluster.Cluster.ID, "uuid", unit.UUID, "err", err)
			}

			// If unit.EndTS is zero, it means a running unit. We shouldnt update stats
			// of running units. They should be updated **ONLY** for finished units
			unitIncr := 1
			if unit.EndedAtTS == 0 {
				unitIncr = 0
			}

			// Update Usage table
			// Use named parameters to not to repeat the values
			if _, err = statements[base.UsageDBTableName].Exec(
				sql.Named(base.UsageDBTableStructFieldColNameMap["ResourceManager"], unit.ResourceManager),
				sql.Named(base.UsageDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.UsageDBTableStructFieldColNameMap["NumUnits"], unitIncr),
				sql.Named(base.UsageDBTableStructFieldColNameMap["Project"], unit.Project),
				sql.Named(base.UsageDBTableStructFieldColNameMap["User"], unit.User),
				sql.Named(base.UsageDBTableStructFieldColNameMap["Group"], unit.Group),
				sql.Named(base.UsageDBTableStructFieldColNameMap["lastupdatedat"], currentTime.Format(base.DatetimeLayout)),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalTime"], unit.TotalTime),
				sql.Named(base.UsageDBTableStructFieldColNameMap["AveCPUUsage"], unit.AveCPUUsage),
				sql.Named(base.UsageDBTableStructFieldColNameMap["AveCPUMemUsage"], unit.AveCPUMemUsage),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalCPUEnergyUsage"], unit.TotalCPUEnergyUsage),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalCPUEmissions"], unit.TotalCPUEmissions),
				sql.Named(base.UsageDBTableStructFieldColNameMap["AveGPUUsage"], unit.AveGPUUsage),
				sql.Named(base.UsageDBTableStructFieldColNameMap["AveGPUMemUsage"], unit.AveGPUMemUsage),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalGPUEnergyUsage"], unit.TotalGPUEnergyUsage),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalGPUEmissions"], unit.TotalGPUEmissions),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalIOWriteStats"], unit.TotalIOWriteStats),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalIOReadStats"], unit.TotalIOReadStats),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalIngressStats"], unit.TotalIngressStats),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalOutgressStats"], unit.TotalOutgressStats),
				sql.Named(base.UsageDBTableStructFieldColNameMap["numupdates"], 1),
			); err != nil {
				level.Error(s.logger).
					Log("msg", "Failed to update usage table in DB", "cluster_id", cluster.Cluster.ID, "uuid", unit.UUID, "err", err)
			}
		}
	}

	// Update users
	for _, cluster := range clusterUsers {
		for _, user := range cluster.Users {
			if _, err = statements[base.UsersDBTableName].Exec(
				sql.Named(base.UsersDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.UsersDBTableStructFieldColNameMap["ResourceManager"], cluster.Cluster.Manager),
				sql.Named(base.UsersDBTableStructFieldColNameMap["UID"], user.UID),
				sql.Named(base.UsersDBTableStructFieldColNameMap["Name"], user.Name),
				sql.Named(base.UsersDBTableStructFieldColNameMap["Projects"], user.Projects),
				sql.Named(base.UsersDBTableStructFieldColNameMap["Tags"], user.Tags),
				sql.Named(base.UsersDBTableStructFieldColNameMap["LastUpdatedAt"], user.LastUpdatedAt),
			); err != nil {
				level.Error(s.logger).
					Log("msg", "Failed to insert user in DB", "cluster_id", cluster.Cluster.ID, "user", user.Name, "err", err)
			}
		}
	}

	// Update projects
	for _, cluster := range clusterProjects {
		for _, project := range cluster.Projects {
			if _, err = statements[base.ProjectsDBTableName].Exec(
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["ResourceManager"], cluster.Cluster.Manager),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["UID"], project.UID),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["Name"], project.Name),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["Users"], project.Users),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["Tags"], project.Tags),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["LastUpdatedAt"], project.LastUpdatedAt),
			); err != nil {
				level.Error(s.logger).
					Log("msg", "Failed to insert project in DB", "cluster_id", cluster.Cluster.ID, "project", project.Name, "err", err)
			}
		}
	}

	// Update admin users table
	for _, source := range AdminUsersSources {
		if _, err = statements[base.AdminUsersDBTableName].Exec(
			sql.Named(base.AdminUsersDBTableStructFieldColNameMap["Source"], source),
			sql.Named(base.AdminUsersDBTableStructFieldColNameMap["Users"], s.admin.users[source]),
			sql.Named(base.AdminUsersDBTableStructFieldColNameMap["LastUpdatedAt"], currentTime.Format(base.DatetimeLayout)),
		); err != nil {
			level.Error(s.logger).
				Log("msg", "Failed to update admin_users table in DB", "source", source, "err", err)
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
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "DB backup", s.logger)

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
