//go:build cgo
// +build cgo

// Package db creates DB tables, call resource manager interfaces and
// populates the DB with compute units
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceems-dev/ceems/internal/common"
	"github.com/ceems-dev/ceems/pkg/api/base"
	db_migrator "github.com/ceems-dev/ceems/pkg/api/db/migrator"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/api/resource"
	"github.com/ceems-dev/ceems/pkg/api/updater"
	"github.com/ceems-dev/ceems/pkg/grafana"
	ceems_sqlite3 "github.com/ceems-dev/ceems/pkg/sqlite3"
	"github.com/mattn/go-sqlite3"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

// Directory containing DB related files.
const (
	migrationsDir = "migrations"
	statementsDir = "statements"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

//go:embed statements/*.sql
var StatementsFS embed.FS

// Custom errors.
var (
	ErrBackupInt = errors.New("backup_interval of less than 1 day is not supported")
	ErrUpdateInt = errors.New("update_interval and/or max_update_interval must be more than 0s")
)

type Timezone struct {
	*time.Location
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *Timezone) UnmarshalYAML(unmarshal func(any) error) error {
	var tmp string

	err := unmarshal(&tmp)
	if err != nil {
		return err
	}

	// Attempt to create location from string
	loc, err := time.LoadLocation(tmp)
	if err != nil {
		return err
	}

	*t = Timezone{loc}

	return nil
}

type DateTime struct {
	time.Time
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *DateTime) UnmarshalYAML(unmarshal func(any) error) error {
	var tmp string

	var err error

	if err = unmarshal(&tmp); err != nil {
		return err
	}

	// Strip as spaces
	tmp = strings.TrimSpace(tmp)

	// Attempt to create location from string
	var tt time.Time
	if tmp == "" || tmp == "today" {
		tt, _ = time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	} else {
		// First attempt to parse as YYYY-MM-DD
		if tt, err = time.Parse("2006-01-02", tmp); err == nil {
			goto outside
		}

		// Second attempt to parse as YYYY-MM-DDTHH:MM
		if tt, err = time.Parse("2006-01-02T15:04", tmp); err == nil {
			goto outside
		}

		// Final attempt to parse as YYYY-MM-DDTHH:MM:SS
		if tt, err = time.Parse("2006-01-02T15:04:05", tmp); err == nil {
			goto outside
		}

		// If everything fails return error
		return fmt.Errorf("failed to parse time string: %s", tmp)
	}

outside:

	*t = DateTime{tt}

	return nil
}

// AdminConfig is the container for the admin users related config.
type AdminConfig struct {
	Users   []string                `yaml:"users"`
	Grafana common.GrafanaWebConfig `yaml:"grafana"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AdminConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Set a default config
	*c = AdminConfig{
		Grafana: common.GrafanaWebConfig{
			HTTPClientConfig: config.DefaultHTTPClientConfig,
		},
	}

	type plain AdminConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Add a CEEM service account to list of admin users. This user will be
	// used in the internal communications between CEEMS components
	c.Users = append(c.Users, base.CEEMSServiceAccount)

	return nil
}

// Validate validates the config.
func (c *AdminConfig) Validate() error {
	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.Grafana.HTTPClientConfig.Validate()
}

// SetDirectory joins any relative file paths with dir.
func (c *AdminConfig) SetDirectory(dir string) {
	c.Grafana.HTTPClientConfig.SetDirectory(dir)
}

// DataConfig is the container for the data related config.
type DataConfig struct {
	Path               string         `yaml:"path"`
	BackupPath         string         `yaml:"backup_path"`
	RetentionPeriod    model.Duration `yaml:"retention_period"`
	UpdateInterval     model.Duration `yaml:"update_interval"`
	MaxUpdateInterval  model.Duration `yaml:"max_update_interval"`
	BackupInterval     model.Duration `yaml:"backup_interval"`
	LastUpdate         DateTime       `yaml:"update_from"`
	Timezone           Timezone       `yaml:"time_zone"`
	SkipDeleteOldUnits bool
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *DataConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Set a default config
	todayMidnight, _ := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	*c = DataConfig{
		Path:              "data",
		RetentionPeriod:   model.Duration(30 * 24 * time.Hour),
		UpdateInterval:    model.Duration(15 * time.Minute),
		MaxUpdateInterval: model.Duration(time.Hour),
		BackupInterval:    model.Duration(24 * time.Hour),
		Timezone:          Timezone{Location: time.Local},
		LastUpdate:        DateTime{todayMidnight},
	}

	type plain DataConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return nil
}

// Validate validates the config.
func (c *DataConfig) Validate() error {
	// Ensure update interval is more than 0
	if time.Duration(c.UpdateInterval).Seconds() == 0 || time.Duration(c.MaxUpdateInterval).Seconds() == 0 {
		return ErrUpdateInt
	}

	// Check if backup interval is non-zero and if it is non-zero
	// ensure that it is >= 1d
	backupInt := time.Duration(c.BackupInterval)

	if backupInt.Seconds() > 0 && backupInt < 24*time.Hour {
		return ErrBackupInt
	}

	return nil
}

// Config makes a DB config from config file.
type Config struct {
	Logger          *slog.Logger
	Data            DataConfig
	Admin           AdminConfig
	ResourceManager func(*slog.Logger) (*resource.Manager, error)
	Updater         func(*slog.Logger) (*updater.UnitUpdater, error)
}

// storageConfig is the container for storage related config.
type storageConfig struct {
	dbPath             string
	dbBackupPath       string
	retentionPeriod    time.Duration
	maxUpdateInterval  time.Duration
	lastUpdateTime     time.Time
	timeLocation       *time.Location
	skipDeleteOldUnits bool
}

// String implements Stringer interface for storageConfig.
func (s *storageConfig) String() string {
	return fmt.Sprintf(
		"DB File Path: %s; Retention Period: %s; Location: %s; Last Updated At: %s; Max Update Interval: %s",
		s.dbPath, s.retentionPeriod, s.timeLocation, s.lastUpdateTime, s.maxUpdateInterval,
	)
}

type adminConfig struct {
	users                map[string][]string // Map of admin users from different sources
	grafana              *grafana.Grafana
	grafanaAdminTeamsIDs []string
}

// stats struct implements fetching compute units, users and project data.
type stats struct {
	logger  *slog.Logger
	db      *sql.DB
	dbConn  *ceems_sqlite3.Conn
	emptyDB bool
	manager *resource.Manager
	updater *updater.UnitUpdater
	storage *storageConfig
	admin   *adminConfig
}

// SQLite DB related constant vars.
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
	// for GPU.
	Weights = map[string]string{
		"avg_cpu_usage":     "alloc_cputime",
		"avg_gpu_usage":     "alloc_gputime",
		"avg_cpu_mem_usage": "alloc_cpumemtime",
		"avg_gpu_mem_usage": "alloc_gpumemtime",
	}

	// Admin users sources.
	AdminUsersSources = []string{"ceems", "grafana"}
)

// Init func to set prepareStatements.
func init() {
	for _, tableName := range []string{base.UnitsDBTableName, base.UsageDBTableName, base.DailyUsageDBTableName, base.AdminUsersDBTableName, base.UsersDBTableName, base.ProjectsDBTableName} {
		statements, err := StatementsFS.ReadFile(fmt.Sprintf("statements/%s.sql", tableName))
		if err != nil {
			panic(fmt.Sprintf("failed to read SQL statements file for table %s: %s", tableName, err))
		}

		prepareStatements[tableName] = string(statements)
	}
}

// New returns a new instance of stats struct.
func New(c *Config) (*stats, error) {
	var err error

	// Get file paths
	dbPath := filepath.Join(c.Data.Path, base.CEEMSDBName)

	// Setup DB
	db, dbConn, err := setupDB(dbPath)
	if err != nil {
		c.Logger.Error("DB setup failed", "err", err)

		return nil, err
	}

	// Setup Migrator
	migrator, err := db_migrator.New(MigrationsFS, migrationsDir, c.Logger)
	if err != nil {
		return nil, err
	}

	// Perform DB migrations
	if err = migrator.ApplyMigrations(db); err != nil {
		return nil, err
	}

	// Get last_updated_at time from DB and overwrite the one provided from config.
	// DB should be the single source of truth.
	var lastUpdatedAt string

	var emptyDB bool

	if err = db.QueryRow("SELECT MAX(last_updated_at) FROM " + base.UsageDBTableName).Scan(&lastUpdatedAt); err == nil {
		// Parse date time string
		c.Data.LastUpdate.Time, err = time.Parse(base.DatetimeLayout, lastUpdatedAt)
		if err != nil {
			c.Logger.Error("Failed to parse last_updated_at fetched from DB", "time", lastUpdatedAt, "err", err)
		}
	} else {
		// If DB is brand new, we get error here as converting NULL to string is unsupported
		emptyDB = true
	}

	// Now make an instance of time.Date with proper format and zone
	c.Data.LastUpdate.Time = time.Date(
		c.Data.LastUpdate.Year(),
		c.Data.LastUpdate.Month(),
		c.Data.LastUpdate.Day(),
		c.Data.LastUpdate.Hour(),
		c.Data.LastUpdate.Minute(),
		c.Data.LastUpdate.Second(),
		c.Data.LastUpdate.Nanosecond(),
		c.Data.Timezone.Location,
	)
	c.Logger.Info("DB will be updated from", "last_update", c.Data.LastUpdate.Time)

	// Create a new instance of Grafana client
	grafanaClient, err := common.NewGrafanaClient(&c.Admin.Grafana, c.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Grafana client: %w", err)
	}

	// Make admin users map
	adminUsers := make(map[string][]string, len(AdminUsersSources))
	adminUsers["ceems"] = c.Admin.Users

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
		maxUpdateInterval:  time.Duration(c.Data.MaxUpdateInterval),
		lastUpdateTime:     c.Data.LastUpdate.Time,
		timeLocation:       c.Data.Timezone.Location,
		skipDeleteOldUnits: c.Data.SkipDeleteOldUnits,
	}

	// Setup manager struct that retrieves unit data
	manager, err := c.ResourceManager(c.Logger)
	if err != nil {
		c.Logger.Error("Resource manager setup failed", "err", err)

		return nil, err
	}

	// Setup updater struct that updates units
	updater, err := c.Updater(c.Logger)
	if err != nil {
		c.Logger.Error("Updater setup failed", "err", err)

		return nil, err
	}

	// Emit debug logs
	c.Logger.Debug("Storage config", "cfg", storageConfig)

	return &stats{
		logger:  c.Logger,
		db:      db,
		dbConn:  dbConn,
		emptyDB: emptyDB,
		manager: manager,
		updater: updater,
		storage: storageConfig,
		admin:   adminConfig,
	}, nil
}

// Collect stats.
func (s *stats) Collect(ctx context.Context) error {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "Data collection", s.logger)

	currentTime := time.Now().In(s.storage.timeLocation)

	// If duration is less than max update interval do single update
	if currentTime.Sub(s.storage.lastUpdateTime) < s.storage.maxUpdateInterval {
		return s.collect(ctx, s.storage.lastUpdateTime, currentTime)
	}

	s.logger.Info("DB update duration is more than max update interval. Doing incremental update. This may take a while...")

	// If duration is more than max update interval, do incremental update
	var nextUpdateTime time.Time

	for {
		nextUpdateTime = s.storage.lastUpdateTime.Add(s.storage.maxUpdateInterval)
		if nextUpdateTime.Compare(currentTime) == -1 {
			s.logger.Debug("Incremental DB update step", "from", s.storage.lastUpdateTime, "to", nextUpdateTime)

			if err := s.collect(ctx, s.storage.lastUpdateTime, nextUpdateTime); err != nil {
				s.logger.Error("Failed incremental update", "from", s.storage.lastUpdateTime, "to", nextUpdateTime, "err", err)

				return err
			}
		} else {
			s.logger.Debug("Final incremental DB update step", "from", s.storage.lastUpdateTime, "to", currentTime)

			return s.collect(ctx, s.storage.lastUpdateTime, currentTime)
		}

		// Sleep for couple of seconds before making next update
		// This is to let DB breath a bit before serving next request
		time.Sleep(time.Second)
	}
}

// Backup DB.
func (s *stats) Backup(ctx context.Context) error {
	return s.createBackup(ctx)
}

// Close DB connection.
func (s *stats) Stop() error {
	return s.db.Close()
}

// updateAdminUsers updates the static list of admin users with the ones fetched
// from Grafana teams.
func (s *stats) updateAdminUsers(ctx context.Context) error {
	// If no teams IDs are configured or Grafana is not online, return
	if s.admin.grafanaAdminTeamsIDs == nil || !s.admin.grafana.Available() {
		return nil
	}

	users, err := s.admin.grafana.TeamMembers(ctx, s.admin.grafanaAdminTeamsIDs)
	if err != nil {
		return err
	}

	// Reset existing grafana admin users
	s.admin.users["grafana"] = users

	return nil
}

// collect fetches unit, user and project stats and insert them into DB.
func (s *stats) collect(ctx context.Context, startTime, endTime time.Time) error {
	// Retrieve units from underlying resource manager(s)
	// Return error only if **all** resource manager(s) failed
	units, err := s.manager.FetchUnits(ctx, startTime, endTime)
	if len(units) == 0 && err != nil {
		return err
	}
	// If atleast one manager passed, and there are failed ones, log the errors
	if err != nil {
		s.logger.Error("Fetching units from atleast one resource manager failed", "err", err)
	}

	// Fetch current users and projects
	// Return error only if **all** resource manager(s) failed
	users, projects, err := s.manager.FetchUsersProjects(ctx, endTime)
	if len(users) == 0 && len(projects) == 0 && err != nil {
		return err
	}
	// If atleast one manager passed, and there are failed ones, log the errors
	if err != nil {
		s.logger.Error("Fetching associations from atleast one resource manager failed", "err", err)
	}

	// Update units struct with unit level metrics from TSDB
	units = s.updater.Update(ctx, startTime, endTime, units)

	// Update admin users list from Grafana
	if err := s.updateAdminUsers(ctx); err != nil {
		s.logger.Error("Failed to update admin users from Grafana", "err", err)
	}

	// Begin transcation
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin SQL transcation: %w", err)
	}

	// Delete older entries and free up DB pages
	// In testing we want to skip this
	if !s.storage.skipDeleteOldUnits {
		s.logger.Debug("Cleaning up old entries in DB")

		if err = s.purgeExpiredUnits(ctx, tx); err != nil {
			s.logger.Error("Failed to clean up old entries", "err", err)
		} else {
			s.logger.Debug("Cleaned up old entries in DB")
		}
	}

	// Insert data into DB
	s.logger.Debug("Executing SQL statements")

	if err := s.execStatements(ctx, tx, startTime, endTime, units, users, projects); err != nil {
		s.logger.Debug("Failed to execute SQL statements", "err", err)

		return fmt.Errorf("failed to execute SQL statements: %w", err)
	} else {
		s.logger.Debug("Finished executing SQL statements")
	}

	// Commit changes
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit SQL transcation: %w", err)
	}

	s.logger.Info("DB updated for period", "from", startTime, "to", endTime)

	// Keep track of last updated time upon successful DB ops
	s.storage.lastUpdateTime = endTime

	return nil
}

// Delete old entries in DB.
func (s *stats) purgeExpiredUnits(ctx context.Context, tx *sql.Tx) error {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "DB cleanup", s.logger)

	// Purge expired units
	// Check the units based on their end time rather than start time. In the
	// case of long running units (like VMs and Pods), we can have these units
	// running longer than retention period which will delete them from DB if
	// we check based on their start time.
	deleteUnitsQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE ended_at <= date('now', '-%d day')",
		base.UnitsDBTableName,
		int(s.storage.retentionPeriod.Hours()/24),
	) // #nosec
	if _, err := tx.ExecContext(ctx, deleteUnitsQuery); err != nil {
		return err
	}

	// Get changes
	var unitsDeleted int
	if err := tx.QueryRowContext(ctx, "SELECT changes()").Scan(&unitsDeleted); err == nil {
		s.logger.Debug("DB update", "units_deleted", unitsDeleted)
	}

	// Purge stale usage data
	deleteUsageQuery := fmt.Sprintf(
		"DELETE FROM %s WHERE last_updated_at <= date('now', '-%d day')",
		base.UsageDBTableName,
		int(s.storage.retentionPeriod.Hours()/24),
	) // #nosec
	if _, err := tx.ExecContext(ctx, deleteUsageQuery); err != nil {
		return err
	}

	// Get changes
	var usageDeleted int
	if err := tx.QueryRowContext(ctx, "SELECT changes()").Scan(&usageDeleted); err == nil {
		s.logger.Debug("DB update", "usage_deleted", usageDeleted)
	}

	return nil
}

// Insert unit stat into DB.
func (s *stats) execStatements(
	ctx context.Context,
	tx *sql.Tx,
	startTime time.Time,
	currentTime time.Time,
	clusterUnits []models.ClusterUnits,
	clusterUsers []models.ClusterUsers,
	clusterProjects []models.ClusterProjects,
) error {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "DB insertion", s.logger)

	// Prepare statements
	stmts := make(map[string]*sql.Stmt, len(prepareStatements))

	var err error

	for table, stmt := range prepareStatements {
		stmts[table], err = tx.PrepareContext(ctx, stmt) //nolint:sqlclosecheck
		if err != nil {
			return fmt.Errorf("failed to prepare statement for table %s: %w", table, err)
		}

		defer stmts[table].Close()
	}

	// Get current day midnight
	todayMidnight := currentTime.Truncate(24 * time.Hour).Format(base.DatetimeLayout)

	var unitIncr int

	for _, cluster := range clusterUnits {
		for _, unit := range cluster.Units {
			// Empty unit
			if unit.UUID == "" {
				continue
			}

			// s.logger.Debug("Inserting unit", "id", unit.Jobid)
			// Use named parameters to not to repeat the values
			if _, err = stmts[base.UnitsDBTableName].ExecContext(
				ctx,
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
				sql.Named(base.UnitsDBTableStructFieldColNameMap["TotalEgressStats"], unit.TotalEgressStats),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Tags"], unit.Tags),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["Ignore"], unit.Ignore),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["NumUpdates"], 1),
				sql.Named(base.UnitsDBTableStructFieldColNameMap["LastUpdatedAt"], currentTime.Format(base.DatetimeLayout)),
			); err != nil {
				s.logger.Error("Failed to insert unit in DB", "cluster_id", cluster.Cluster.ID, "uuid", unit.UUID, "err", err)
			}

			// If the unit has started in this update period, increment num units
			// Or if we start with empty DB, we need to increment for num units for all discovered units
			unitIncr = 0
			if unit.StartedAtTS > startTime.UnixMilli() || s.emptyDB {
				unitIncr = 1
			}

			// Update Usage table
			// Use named parameters to not to repeat the values
			if _, err = stmts[base.UsageDBTableName].ExecContext(
				ctx,
				sql.Named(base.UsageDBTableStructFieldColNameMap["ResourceManager"], unit.ResourceManager),
				sql.Named(base.UsageDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.UsageDBTableStructFieldColNameMap["NumUnits"], unitIncr),
				sql.Named(base.UsageDBTableStructFieldColNameMap["Project"], unit.Project),
				sql.Named(base.UsageDBTableStructFieldColNameMap["User"], unit.User),
				sql.Named(base.UsageDBTableStructFieldColNameMap["Group"], unit.Group),
				sql.Named(base.UsageDBTableStructFieldColNameMap["LastUpdatedAt"], currentTime.Format(base.DatetimeLayout)),
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalTime"], unit.TotalTime),
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
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalEgressStats"], unit.TotalEgressStats),
				sql.Named(base.UsageDBTableStructFieldColNameMap["NumUpdates"], 1),
			); err != nil {
				s.logger.Error("Failed to update usage table in DB", "cluster_id", cluster.Cluster.ID, "uuid", unit.UUID, "err", err)
			}

			// Update DailyUsage table
			// Use named parameters to not to repeat the values
			if _, err = stmts[base.DailyUsageDBTableName].ExecContext(
				ctx,
				sql.Named(base.UsageDBTableStructFieldColNameMap["ResourceManager"], unit.ResourceManager),
				sql.Named(base.UsageDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.UsageDBTableStructFieldColNameMap["NumUnits"], unitIncr),
				sql.Named(base.UsageDBTableStructFieldColNameMap["Project"], unit.Project),
				sql.Named(base.UsageDBTableStructFieldColNameMap["User"], unit.User),
				sql.Named(base.UsageDBTableStructFieldColNameMap["Group"], unit.Group),
				sql.Named(base.UsageDBTableStructFieldColNameMap["LastUpdatedAt"], todayMidnight), // This ensures that we aggregate data for each day
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalTime"], unit.TotalTime),
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
				sql.Named(base.UsageDBTableStructFieldColNameMap["TotalEgressStats"], unit.TotalEgressStats),
				sql.Named(base.UsageDBTableStructFieldColNameMap["NumUpdates"], 1),
			); err != nil {
				s.logger.Error("Failed to update daily_usage table in DB", "cluster_id", cluster.Cluster.ID, "uuid", unit.UUID, "err", err)
			}
		}
	}

	// Update users
	for _, cluster := range clusterUsers {
		for _, user := range cluster.Users {
			if _, err = stmts[base.UsersDBTableName].ExecContext(
				ctx,
				sql.Named(base.UsersDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.UsersDBTableStructFieldColNameMap["ResourceManager"], cluster.Cluster.Manager),
				sql.Named(base.UsersDBTableStructFieldColNameMap["UID"], user.UID),
				sql.Named(base.UsersDBTableStructFieldColNameMap["Name"], user.Name),
				sql.Named(base.UsersDBTableStructFieldColNameMap["Projects"], user.Projects),
				sql.Named(base.UsersDBTableStructFieldColNameMap["Tags"], user.Tags),
				sql.Named(base.UsersDBTableStructFieldColNameMap["LastUpdatedAt"], user.LastUpdatedAt),
			); err != nil {
				s.logger.Error("Failed to insert user in DB", "cluster_id", cluster.Cluster.ID, "user", user.Name, "err", err)
			}
		}
	}

	// Update projects
	for _, cluster := range clusterProjects {
		for _, project := range cluster.Projects {
			if _, err = stmts[base.ProjectsDBTableName].ExecContext(
				ctx,
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["ClusterID"], cluster.Cluster.ID),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["ResourceManager"], cluster.Cluster.Manager),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["UID"], project.UID),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["Name"], project.Name),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["Users"], project.Users),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["Tags"], project.Tags),
				sql.Named(base.ProjectsDBTableStructFieldColNameMap["LastUpdatedAt"], project.LastUpdatedAt),
			); err != nil {
				s.logger.Error("Failed to insert project in DB", "cluster_id", cluster.Cluster.ID, "project", project.Name, "err", err)
			}
		}
	}

	// Update admin users table
	for _, source := range AdminUsersSources {
		for _, user := range s.admin.users[source] {
			if _, err = stmts[base.AdminUsersDBTableName].ExecContext(
				ctx,
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["ClusterID"], "all"),
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["ResourceManager"], ""),
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["UID"], ""),
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["Name"], user),
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["Projects"], models.List{}),
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["Tags"], models.List{source}),
				sql.Named(base.AdminUsersDBTableStructFieldColNameMap["LastUpdatedAt"], currentTime.Format(base.DatetimeLayout)),
			); err != nil {
				s.logger.Error("Failed to update admin_users table in DB", "source", source, "err", err)
			}
		}
	}

	// If emptyDB is true, we have already primed the DB with first update and set it to false
	if s.emptyDB {
		s.emptyDB = false
	}

	return nil
}

// backup executes the sqlite3 backup strategy
// Based on https://gist.github.com/bbengfort/452a9d5e74a63d88e5a34a580d6cb6d3
// Ref: https://github.com/rotationalio/ensign/pull/529/files
func (s *stats) backup(ctx context.Context, backupDBPath string) error {
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
		select {
		case <-ctx.Done():
			s.logger.Debug("DB backup aborted due to cancelled context", "err", ctx.Err())

			return backup.Finish()
		default:
			// Backing up a smaller number of pages per step is the most effective way of
			// doing online backups and also allow write transactions to make progress.
			if isDone, err = backup.Step(pagesPerStep); err != nil {
				if finishErr := backup.Finish(); finishErr != nil {
					return fmt.Errorf("errors: %w, %w", err, finishErr)
				}

				return err
			}

			s.logger.Debug("DB backup step", "remaining", backup.Remaining(), "page_count", backup.PageCount())

			// This sleep allows other transactions to write during backups.
			time.Sleep(stepSleep)
		}
	}

	return backup.Finish()
}

// vacuum executes sqlite3 vacuum command.
func (s *stats) vacuum(ctx context.Context) error {
	s.logger.Debug("Starting to vacuum DB")

	if _, err := s.db.ExecContext(ctx, "VACUUM"); err != nil {
		return err
	}

	return nil
}

// createBackup creates backup of DB after vacuuming DB.
func (s *stats) createBackup(ctx context.Context) error {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "DB backup", s.logger)

	// First vacuum DB to reduce size
	if err := s.vacuum(ctx); err != nil {
		s.logger.Warn("Failed to vacuum DB", "err", err)
	} else {
		s.logger.Debug("DB vacuumed")
	}

	// Attempt to create in-place DB backup
	// Make a unique backup file name using current time
	backupDBFileName := fmt.Sprintf(
		"%s-%s.db",
		strings.Split(base.CEEMSDBName, ".")[0],
		time.Now().In(s.storage.timeLocation).Format("200601021504"),
	)

	backupDBFilePath := filepath.Join(filepath.Dir(s.storage.dbPath), backupDBFileName)
	if err := s.backup(ctx, backupDBFilePath); err != nil {
		return err
	}

	// If back is successful, move it to dbBackupPath
	err := os.Rename(backupDBFilePath, filepath.Join(s.storage.dbBackupPath, backupDBFileName))
	if err != nil {
		return fmt.Errorf("failed to move backup DB file: %w", err)
	}

	s.logger.Info("DB backed up", "file", backupDBFileName)

	return nil
}
