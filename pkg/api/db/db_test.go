package db

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	_ "github.com/mattn/go-sqlite3"
)

type mockFetcherOne struct {
	logger log.Logger
}

type mockFetcherTwo struct {
	logger log.Logger
}

type mockFetcherThree struct {
	logger log.Logger
}

var mockUnitsOne = []models.Unit{
	{
		UUID:            "10000",
		Usr:             "foo1",
		Project:         "fooprj",
		TotalCPUTime:    int64(1800),
		TotalGPUTime:    int64(900),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(9000),
		TotalGPUMemTime: int64(900),
	},
	{
		UUID:            "10001",
		Usr:             "foo1",
		Project:         "fooprj",
		TotalCPUTime:    int64(900),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(4500),
	},
	{
		UUID:            "10002",
		Usr:             "foo1",
		Project:         "fooprj",
		TotalCPUTime:    int64(2700),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(9000),
	},
	{
		UUID:            "10003",
		Usr:             "foo2",
		Project:         "fooprj",
		TotalCPUTime:    int64(3600),
		TotalWallTime:   int64(1800),
		TotalCPUMemTime: int64(90000),
	},
}
var mockUnitsTwo = []models.Unit{
	{
		UUID:            "20000",
		Usr:             "bar1",
		Project:         "barprj",
		TotalCPUTime:    int64(900),
		TotalGPUTime:    int64(900),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(9000),
		TotalGPUMemTime: int64(900),
	},
	{
		UUID:            "20001",
		Usr:             "bar3",
		Project:         "barprj",
		TotalCPUTime:    int64(1800),
		TotalGPUTime:    int64(1800),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(90000),
		TotalGPUMemTime: int64(900),
	},
	{
		UUID:            "20002",
		Usr:             "bar3",
		Project:         "barprj",
		TotalCPUTime:    int64(2700),
		TotalGPUTime:    int64(900),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(90000),
		TotalGPUMemTime: int64(900),
	},
}
var mockUnits = append(mockUnitsOne, mockUnitsTwo...)

func newMockManager(logger log.Logger) (*resource.Manager, error) {
	return &resource.Manager{
		Fetchers: []resource.Fetcher{
			&mockFetcherOne{logger: logger},
			&mockFetcherTwo{logger: logger},
			&mockFetcherThree{logger: logger},
		},
	}, nil
}

// GetUnits implements collection units between start and end times
func (m *mockFetcherOne) Fetch(start time.Time, end time.Time) ([]models.Unit, error) {
	return mockUnitsOne, nil
}

// GetUnits implements collection units between start and end times
func (m *mockFetcherTwo) Fetch(start time.Time, end time.Time) ([]models.Unit, error) {
	return mockUnitsTwo, nil
}

// Return error for this mockFetcher
func (m *mockFetcherThree) Fetch(start time.Time, end time.Time) ([]models.Unit, error) {
	return nil, fmt.Errorf("failed to fetch units")
}

type mockUpdater struct {
	logger log.Logger
}

var mockUpdatedUnits = []models.Unit{
	{
		UUID:            "10000",
		Usr:             "foo1",
		Project:         "fooprj",
		TotalCPUTime:    int64(1800),
		TotalGPUTime:    int64(900),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(9000),
		TotalGPUMemTime: int64(900),
		AveCPUUsage:     10,
		AveGPUUsage:     20,
	},
	{
		UUID:                "10001",
		Usr:                 "foo1",
		Project:             "fooprj",
		TotalCPUTime:        int64(900),
		TotalWallTime:       int64(900),
		TotalCPUMemTime:     int64(4500),
		AveCPUUsage:         25,
		TotalCPUEnergyUsage: 100,
	},
	{
		UUID:              "10002",
		Usr:               "foo1",
		Project:           "fooprj",
		TotalCPUTime:      int64(2700),
		TotalWallTime:     int64(900),
		TotalCPUMemTime:   int64(9000),
		TotalCPUEmissions: 20,
	},
	{
		UUID:              "10003",
		Usr:               "foo2",
		Project:           "fooprj",
		TotalCPUTime:      int64(3600),
		TotalWallTime:     int64(1800),
		TotalCPUMemTime:   int64(90000),
		TotalCPUEmissions: 40,
	},
	{
		UUID:                "20000",
		Usr:                 "bar1",
		Project:             "barprj",
		TotalCPUTime:        int64(900),
		TotalGPUTime:        int64(900),
		TotalWallTime:       int64(900),
		TotalCPUMemTime:     int64(9000),
		TotalGPUMemTime:     int64(900),
		TotalGPUEnergyUsage: 200,
	},
	{
		UUID:            "20001",
		Usr:             "bar3",
		Project:         "barprj",
		TotalCPUTime:    int64(1800),
		TotalGPUTime:    int64(1800),
		TotalWallTime:   int64(900),
		TotalCPUMemTime: int64(90000),
		TotalGPUMemTime: int64(900),
		AveCPUUsage:     20,
		AveGPUMemUsage:  40,
	},
	{
		UUID:              "20002",
		Usr:               "bar3",
		Project:           "barprj",
		TotalCPUTime:      int64(2700),
		TotalGPUTime:      int64(900),
		TotalWallTime:     int64(900),
		TotalCPUMemTime:   int64(90000),
		TotalGPUMemTime:   int64(900),
		TotalGPUEmissions: 40,
	},
}

func newMockUpdater(logger log.Logger) (*updater.UnitUpdater, error) {
	return &updater.UnitUpdater{
		Names:    []string{"mock"},
		Updaters: []updater.Updater{&mockUpdater{logger: logger}},
		Logger:   logger,
	}, nil
}

// GetUnits implements collection units between start and end times
func (m *mockUpdater) Update(startTime time.Time, endTime time.Time, units []models.Unit) []models.Unit {
	return mockUpdatedUnits
}

func prepareMockConfig(tmpDir string) *Config {
	dataDir := filepath.Join(tmpDir, "data")
	dataBackupDir := filepath.Join(tmpDir, "data-backup")

	// Create data directory
	if err := os.Mkdir(dataDir, 0750); err != nil {
		fmt.Printf("Failed to create data directory: %s", err)
	}
	if err := os.Mkdir(dataBackupDir, 0750); err != nil {
		fmt.Printf("Failed to create data directory: %s", err)
	}

	// Create an empty file for sacct
	sacctFile, err := os.Create(filepath.Join(tmpDir, "sacct"))
	if err != nil {
		fmt.Println(err)
	}
	sacctFile.Close()
	return &Config{
		Logger:               log.NewNopLogger(),
		DataPath:             dataDir,
		DataBackupPath:       dataBackupDir,
		LastUpdateTimeString: time.Now().Format("2006-01-02"),
		RetentionPeriod:      time.Duration(24 * time.Hour),
		ResourceManager:      newMockManager,
		TSDB:                 &tsdb.TSDB{},
		Updater:              newMockUpdater,
	}
}

func populateDBWithMockData(s *statsDB) {
	tx, _ := s.db.Begin()
	stmtMap, err := s.prepareStatements(tx)
	if err != nil {
		fmt.Println(err)
	}
	s.execStatements(stmtMap, mockUnits)
	tx.Commit()
}

func TestNewUnitStatsDB(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	lastUnitsUpdateTimeFile := filepath.Join(c.DataPath, "lastupdatetime")

	// Make new stats DB
	c.LastUpdateTimeString = "2023-12-20"
	_, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Check if last update time file has been written
	if _, err := os.Stat(lastUnitsUpdateTimeFile); err != nil {
		t.Errorf("Last update time file not created")
	}

	// Check content of last update time file
	if timeString, _ := os.ReadFile(lastUnitsUpdateTimeFile); string(timeString) != "2023-12-20T00:00:00" {
		t.Errorf("Last update time string test failed. Expected %s got %s", "2023-12-20T00:00:00", string(timeString))
	}

	// Check DB file exists
	if _, err := os.Stat(c.DataPath); err != nil {
		t.Errorf("DB file not created")
	}

	// Make again a new stats DB with new lastUpdateTime
	c.LastUpdateTimeString = "2023-12-21"
	_, err = NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Check content of last update time file. It should not change
	if timeString, _ := os.ReadFile(lastUnitsUpdateTimeFile); string(timeString) != "2023-12-20T00:00:00" {
		t.Errorf("Last update time string test failed. Expected %s got %s", "2023-12-20T00:00:00", string(timeString))
	}

	// Remove read permissions on lastupdatetimefile
	err = os.Chmod(lastUnitsUpdateTimeFile, 0200)
	if err != nil {
		t.Fatal(err)
	}

	// Make again a new stats DB with new lastUpdateTime
	_, err = NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Add back read permissions on lastupdatetimefile
	err = os.Chmod(lastUnitsUpdateTimeFile, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Check content of last update time file. It should change
	if timeString, err := os.ReadFile(lastUnitsUpdateTimeFile); string(timeString) != "2023-12-21T00:00:00" {
		t.Errorf(
			"Last update time string test failed. Expected %s got %s %s",
			"2023-12-21T00:00:00",
			string(timeString),
			err,
		)
	}

	// Remove last update time file
	err = os.Remove(lastUnitsUpdateTimeFile)
	if err != nil {
		t.Fatal(err)
	}

	// Make again a new stats DB with new lastUpdateTime
	c.LastUpdateTimeString = "2023-12-22"
	_, err = NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Check content of last update time file. It should change
	if timeString, err := os.ReadFile(lastUnitsUpdateTimeFile); string(timeString) != "2023-12-22T00:00:00" {
		t.Errorf(
			"Last update time string test failed. Expected %s got %s %s",
			"2023-12-22T00:00:00",
			string(timeString),
			err,
		)
	}
}

func TestUnitStatsDBEntries(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new stats DB
	s, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Fetch units
	expectedUnits := append(mockUnitsOne, mockUnitsTwo...)
	fetchedUnits, err := s.manager.Fetch(time.Now(), time.Now())
	if !reflect.DeepEqual(fetchedUnits, expectedUnits) {
		t.Errorf("expected %#v, got %#v", expectedUnits, fetchedUnits)
	}
	if err == nil {
		t.Errorf("expected one err from fetcher got none")
	}

	// Try to insert data
	err = s.Collect()
	if err != nil {
		t.Errorf("Failed to collect units data: %s", err)
	}

	// Make units query
	rows, err := s.db.Query(
		"SELECT uuid,usr,project,total_cputime_seconds,total_gputime_seconds,total_walltime_seconds,total_cpumemtime_seconds,total_gpumemtime_seconds,avg_cpu_usage,avg_cpu_mem_usage,total_cpu_energy_usage_kwh,total_cpu_emissions_gms,avg_gpu_usage,avg_gpu_mem_usage,total_gpu_energy_usage_kwh,total_gpu_emissions_gms FROM units ORDER BY uuid",
	)
	if err != nil {
		t.Errorf("Failed to make DB query")
	}
	defer rows.Close()

	var units []models.Unit
	for rows.Next() {
		var unit models.Unit

		if err = rows.Scan(
			&unit.UUID, &unit.Usr, &unit.Project, &unit.TotalCPUTime,
			&unit.TotalGPUTime, &unit.TotalWallTime, &unit.TotalCPUMemTime,
			&unit.TotalGPUMemTime,
			&unit.AveCPUUsage,
			&unit.AveCPUMemUsage, &unit.TotalCPUEnergyUsage,
			&unit.TotalCPUEmissions, &unit.AveGPUUsage, &unit.AveGPUMemUsage,
			&unit.TotalGPUEnergyUsage, &unit.TotalGPUEmissions); err != nil {
			t.Errorf("failed to scan row: %s", err)
		}
		units = append(units, unit)
	}

	if !reflect.DeepEqual(units, mockUpdatedUnits) {
		t.Errorf("expected %#v, \n\n\n got %#v", mockUpdatedUnits, units)
	}

	// Make usage query
	rows, err = s.db.Query("SELECT avg_cpu_usage,num_updates FROM usage WHERE usr = 'foo1'")
	if err != nil {
		t.Errorf("Failed to make DB query: %s", err)
	}
	defer rows.Close()

	source, _ := os.Open(filepath.Join(tmpDir, "data", "ceems_api_server.db"))
	defer source.Close()

	// For debugging
	// destination, _ := os.Create("test.db")
	// defer destination.Close()
	// nBytes, _ := io.Copy(destination, source)
	// fmt.Println(nBytes)

	var cpuUsage float64
	var numUpdates int64
	for rows.Next() {
		if err = rows.Scan(&cpuUsage, &numUpdates); err != nil {
			t.Errorf("failed to scan row: %s", err)
		}
	}

	if cpuUsage < 15 {
		t.Errorf("expected 15, \n got %f", cpuUsage)
	}

	// Close DB
	s.Stop()
}

func TestUnitStatsDBLock(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new stats DB
	s, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Beging exclusive transcation to lock DB
	_, err = s.db.Exec("BEGIN EXCLUSIVE;")
	if err != nil {
		t.Errorf("Failed to lock DB due to %s", err)
	}

	// Try to insert data. It should fail
	err = s.Collect()
	if err == nil {
		t.Errorf("Failed to skip data insertion when DB is locked")
	}
	s.db.Exec("COMMIT;")

	// Close DB
	s.Stop()
}

func TestUnitStatsDBVacuum(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new stats DB
	s, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Populate DB with data
	populateDBWithMockData(s)

	// Run vacuum
	err = s.vacuum()
	if err != nil {
		t.Errorf("Failed to vacuum DB due to %s", err)
	}
}

func TestUnitStatsDBBackup(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new stats DB
	s, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Populate DB with data
	populateDBWithMockData(s)

	// Run backup
	expectedBackupFile := filepath.Join(c.DataBackupPath, "backup.db")
	err = s.backup(expectedBackupFile)
	if err != nil {
		t.Errorf("Failed to backup DB %s", err)
	}

	if _, err := os.Stat(expectedBackupFile); err != nil {
		t.Errorf("Backup DB file not found")
	}

	// Check contents of backed up DB
	var numRows int
	db, _, err := openDBConnection(expectedBackupFile)
	if err != nil {
		t.Errorf("Failed to create DB connection to backup DB: %s", err)
	}
	rows, _ := db.Query(fmt.Sprintf("SELECT * FROM %s;", base.UnitsDBTableName))
	for rows.Next() {
		numRows += 1
	}
	if numRows != 7 {
		t.Errorf("Backup DB check failed. Expected rows 7 , Got %d.", numRows)
	}
}

func TestStatsDBBackup(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	// Make new stats DB
	s, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Make backup dir non existent
	s.storage.dbBackupPath = tmpDir

	// Populate DB with data
	populateDBWithMockData(s)

	// Run backup
	if err := s.createBackup(); err != nil {
		t.Errorf("Failed to backup DB: %s", err)
	}
}

func TestUnitStatsDeleteOldUnits(t *testing.T) {
	tmpDir := t.TempDir()
	unitID := "1111"
	c := prepareMockConfig(tmpDir)

	// Make new stats DB
	s, err := NewStatsDB(c)
	if err != nil {
		t.Errorf("Failed to create new statsDB struct due to %s", err)
	}

	// Add new row that should be deleted
	units := []models.Unit{
		{
			UUID: unitID,
			CreatedAt: time.Now().
				Add(time.Duration(-s.storage.retentionPeriod*24*2) * time.Hour).
				Format(base.DatetimeLayout),
		},
	}
	tx, _ := s.db.Begin()
	stmtMap, err := s.prepareStatements(tx)
	if err != nil {
		t.Errorf("Failed to prepare SQL statements: %s", err)
	}
	s.execStatements(stmtMap, units)

	// Now clean up DB for old units
	err = s.purgeExpiredUnits(tx)
	if err != nil {
		t.Errorf("Failed to delete old entries in DB")
	}
	tx.Commit()

	// Query for deleted unit
	result, err := s.db.Prepare(
		fmt.Sprintf("SELECT COUNT(uuid) FROM %s WHERE uuid = ?;", base.UnitsDBTableName),
	)
	if err != nil {
		t.Errorf("Failed to prepare SQL statement")
	}
	var numRows string
	err = result.QueryRow(unitID).Scan(&numRows)
	if err != nil {
		t.Errorf("Failed to get query result due to %s.", err)
	}
	if numRows != "0" {
		t.Errorf("Deleting old units failed. Expected 0 rows. Returned %s", numRows)
	}
}
