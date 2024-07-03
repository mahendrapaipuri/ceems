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
	"github.com/prometheus/common/model"
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

var mockUnitsOne = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID:       "slurm-0",
			Updaters: []string{"slurm-00", "slurm-01"},
		},
		Units: []models.Unit{
			{
				UUID:    "10000",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(1800),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
			},
			{
				UUID:    "10001",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(900),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(4500),
					"alloc_gpumemtime": models.JSONFloat(0),
				},
			},
		},
	},
	{
		Cluster: models.Cluster{
			ID:       "slurm-1",
			Updaters: []string{"slurm-1"},
		},
		Units: []models.Unit{
			{
				UUID:    "10002",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(2700),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(900),
					"alloc_gpumemtime": models.JSONFloat(0),
				},
			},
			{
				UUID:    "10003",
				User:    "foo2",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(1800),
					"alloc_cputime":    models.JSONFloat(3600),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(90000),
					"alloc_gpumemtime": models.JSONFloat(0),
				},
			},
		},
	},
}
var mockUnitsTwo = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID:       "os-0",
			Updaters: []string{"os-0"},
		},
		Units: []models.Unit{
			{
				UUID:    "20000",
				User:    "bar1",
				Project: "barprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(900),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
			},
		},
	},
	{
		Cluster: models.Cluster{
			ID:       "os-1",
			Updaters: []string{"os-1"},
		},
		Units: []models.Unit{
			{
				UUID:    "20001",
				User:    "bar3",
				Project: "barprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(1800),
					"alloc_gputime":    models.JSONFloat(1800),
					"alloc_cpumemtime": models.JSONFloat(90000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
			},
			{
				UUID:    "20002",
				User:    "bar3",
				Project: "barprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(2700),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
			},
		},
	},
}

var mockProjectsOne = []models.ClusterProjects{
	{
		Cluster: models.Cluster{
			ID:       "slurm-0",
			Updaters: []string{"slurm-00", "slurm-01"},
		},
		Projects: []models.Project{
			{
				Name:  "fooprj",
				Users: models.List{"foo1", "foo2"},
			},
			{
				Name:  "barprj",
				Users: models.List{"bar1", "bar2"},
			},
		},
	},
	{
		Cluster: models.Cluster{
			ID:       "slurm-1",
			Updaters: []string{"slurm-1"},
		},
		Projects: []models.Project{
			{
				Name:  "fooprj",
				Users: models.List{"foo1", "foo2"},
			},
			{
				Name:  "barprj",
				Users: models.List{"bar1", "bar2"},
			},
		},
	},
}

var mockUsersOne = []models.ClusterUsers{
	{
		Cluster: models.Cluster{
			ID:       "slurm-0",
			Updaters: []string{"slurm-00", "slurm-01"},
		},
		Users: []models.User{
			{
				Name:     "foo1",
				Projects: models.List{"fooprj"},
			},
			{
				Name:     "bar1",
				Projects: models.List{"barprj"},
			},
		},
	},
	{
		Cluster: models.Cluster{
			ID:       "slurm-1",
			Updaters: []string{"slurm-1"},
		},
		Users: []models.User{
			{
				Name:     "foo1",
				Projects: models.List{"fooprj"},
			},
			{
				Name:     "bar1",
				Projects: models.List{"barprj"},
			},
		},
	},
}

func newMockManager(logger log.Logger) (*resource.Manager, error) {
	return &resource.Manager{
		Logger: logger,
		Fetchers: []resource.Fetcher{
			&mockFetcherOne{logger: logger},
			&mockFetcherTwo{logger: logger},
			&mockFetcherThree{logger: logger},
		},
	}, nil
}

// FetchUnits implements collection units between start and end times
func (m *mockFetcherOne) FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return mockUnitsOne, nil
}

// FetchUsersProjects implements collection project user association
func (m *mockFetcherOne) FetchUsersProjects(
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return mockUsersOne, mockProjectsOne, nil
}

// FetchUnits implements collection units between start and end times
func (m *mockFetcherTwo) FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return mockUnitsTwo, nil
}

// FetchUsersProjects implements collection project user association
func (m *mockFetcherTwo) FetchUsersProjects(
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return nil, nil, fmt.Errorf("failed to fetch associations")
}

// Return error for this mockFetcher
func (m *mockFetcherThree) FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return nil, fmt.Errorf("failed to fetch units")
}

// FetchUsersProjects implements collection project user association
func (m *mockFetcherThree) FetchUsersProjects(
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return mockUsersOne, mockProjectsOne, nil
}

var mockUpdatedUnitsSlurm00 = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID: "slurm-0",
		},
		Units: []models.Unit{
			{
				UUID:    "10000",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(1800),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
				AveCPUUsage: models.MetricMap{"usage": 10},
			},
			{
				UUID:    "10001",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(4500),
				},
				AveCPUUsage: models.MetricMap{"usage": 25},
			},
		},
	},
}

var mockUpdatedUnitsSlurm01 = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID: "slurm-0",
		},
		Units: []models.Unit{
			{
				UUID:    "10000",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(1800),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
				AveCPUUsage: models.MetricMap{"usage": 10},
				AveGPUUsage: models.MetricMap{"usage": 20},
			},
			{
				UUID:    "10001",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(900),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(4500),
					"alloc_gpumemtime": models.JSONFloat(0),
				},
				AveCPUUsage:         models.MetricMap{"usage": 25},
				TotalCPUEnergyUsage: models.MetricMap{"usage": 100},
			},
		},
	},
}

var mockUpdatedUnitsSlurm1 = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID: "slurm-1",
		},
		Units: []models.Unit{
			{
				UUID:    "10002",
				User:    "foo1",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(2700),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(0),
				},
				TotalCPUEmissions: models.MetricMap{"rte": 25},
			},
			{
				UUID:    "10003",
				User:    "foo2",
				Project: "fooprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(1800),
					"alloc_cputime":    models.JSONFloat(3600),
					"alloc_gputime":    models.JSONFloat(0),
					"alloc_cpumemtime": models.JSONFloat(90000),
					"alloc_gpumemtime": models.JSONFloat(0),
				},
				TotalCPUEmissions: models.MetricMap{"rte": 40},
			},
		},
	},
}

var mockUpdatedUnitsOS0 = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID: "os-0",
		},
		Units: []models.Unit{
			{
				UUID:    "20000",
				User:    "bar1",
				Project: "barprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(900),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(9000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
				TotalGPUEnergyUsage: models.MetricMap{"usage": 200},
			},
		},
	},
}

var mockUpdatedUnitsOS1 = []models.ClusterUnits{
	{
		Cluster: models.Cluster{
			ID: "os-1",
		},
		Units: []models.Unit{
			{
				UUID:    "20001",
				User:    "bar3",
				Project: "barprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(1800),
					"alloc_gputime":    models.JSONFloat(1800),
					"alloc_cpumemtime": models.JSONFloat(90000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
				AveCPUUsage:    models.MetricMap{"usage": 20},
				AveGPUMemUsage: models.MetricMap{"usage": 40},
			},
			{
				UUID:    "20002",
				User:    "bar3",
				Project: "barprj",
				TotalTime: models.MetricMap{
					"walltime":         models.JSONFloat(900),
					"alloc_cputime":    models.JSONFloat(2700),
					"alloc_gputime":    models.JSONFloat(900),
					"alloc_cpumemtime": models.JSONFloat(90000),
					"alloc_gpumemtime": models.JSONFloat(900),
				},
				TotalGPUEmissions: models.MetricMap{"rte": 40},
			},
		},
	},
}

type mockUpdaterSlurm00 struct {
	logger log.Logger
}

// GetUnits implements collection units between start and end times
func (m mockUpdaterSlurm00) Update(
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsSlurm00
}

type mockUpdaterSlurm01 struct {
	logger log.Logger
}

// GetUnits implements collection units between start and end times
func (m mockUpdaterSlurm01) Update(
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsSlurm01
}

type mockUpdaterSlurm1 struct {
	logger log.Logger
}

// GetUnits implements collection units between start and end times
func (m mockUpdaterSlurm1) Update(
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsSlurm1
}

type mockUpdaterOS0 struct {
	logger log.Logger
}

// GetUnits implements collection units between start and end times
func (m mockUpdaterOS0) Update(
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsOS0
}

type mockUpdaterOS1 struct {
	logger log.Logger
}

// GetUnits implements collection units between start and end times
func (m mockUpdaterOS1) Update(
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsOS1
}

func newMockUpdater(logger log.Logger) (*updater.UnitUpdater, error) {
	return &updater.UnitUpdater{
		Updaters: map[string]updater.Updater{
			"slurm-00": mockUpdaterSlurm00{logger: logger},
			"slurm-01": mockUpdaterSlurm01{logger: logger},
			"slurm-1":  mockUpdaterSlurm1{logger: logger},
			"os-0":     mockUpdaterOS0{logger: logger},
			"os-1":     mockUpdaterOS1{logger: logger},
		},
		Logger: logger,
	}, nil
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
		Logger: log.NewNopLogger(),
		Data: DataConfig{
			Path:            dataDir,
			BackupPath:      dataBackupDir,
			LastUpdateTime:  time.Now(),
			RetentionPeriod: model.Duration(24 * time.Hour),
		},
		Admin: AdminConfig{
			Users: []string{"adm1", "adm2"},
		},
		ResourceManager: newMockManager,
		Updater:         newMockUpdater,
	}
}

func populateDBWithMockData(s *statsDB) {
	tx, _ := s.db.Begin()
	stmtMap, err := s.prepareStatements(tx)
	if err != nil {
		fmt.Println(err)
	}
	s.execStatements(stmtMap, time.Now(), mockUnitsOne, mockUsersOne, mockProjectsOne)
	s.execStatements(stmtMap, time.Now(), mockUnitsTwo, nil, nil)
	tx.Commit()
}

func TestNewUnitStatsDB(t *testing.T) {
	tmpDir := t.TempDir()
	c := prepareMockConfig(tmpDir)

	lastUnitsUpdateTimeFile := filepath.Join(c.Data.Path, "lastupdatetime")

	// Make new stats DB
	c.Data.LastUpdateTime, _ = time.Parse("2006-01-02", "2023-12-20")
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
	if _, err := os.Stat(c.Data.Path); err != nil {
		t.Errorf("DB file not created")
	}

	// Make again a new stats DB with new lastUpdateTime
	c.Data.LastUpdateTime, _ = time.Parse("2006-01-02", "2023-12-21")
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
	c.Data.LastUpdateTime, _ = time.Parse("2006-01-02", "2023-12-21")
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
	c.Data.LastUpdateTime, _ = time.Parse("2006-01-02", "2023-12-22")
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
	var expectedUnits []models.ClusterUnits
	expectedUnits = append(expectedUnits, mockUnitsOne...)
	expectedUnits = append(expectedUnits, mockUnitsTwo...)
	fetchedUnits, err := s.manager.FetchUnits(time.Now(), time.Now())
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
		"SELECT uuid,username,project,total_time_seconds,avg_cpu_usage,avg_cpu_mem_usage,total_cpu_energy_usage_kwh,total_cpu_emissions_gms,avg_gpu_usage,avg_gpu_mem_usage,total_gpu_energy_usage_kwh,total_gpu_emissions_gms FROM units ORDER BY uuid",
	)
	if err != nil {
		t.Errorf("Failed to make DB query")
	}
	defer rows.Close()

	var units []models.Unit
	for rows.Next() {
		var unit models.Unit

		if err = rows.Scan(
			&unit.UUID, &unit.User, &unit.Project, &unit.TotalTime,
			&unit.AveCPUUsage,
			&unit.AveCPUMemUsage, &unit.TotalCPUEnergyUsage,
			&unit.TotalCPUEmissions, &unit.AveGPUUsage, &unit.AveGPUMemUsage,
			&unit.TotalGPUEnergyUsage, &unit.TotalGPUEmissions); err != nil {
			t.Errorf("failed to scan row: %s", err)
		}
		units = append(units, unit)
	}

	var mockUpdatedUnits []models.ClusterUnits
	mockUpdatedUnits = append(mockUpdatedUnits, mockUpdatedUnitsSlurm01...)
	mockUpdatedUnits = append(mockUpdatedUnits, mockUpdatedUnitsSlurm1...)
	mockUpdatedUnits = append(mockUpdatedUnits, mockUpdatedUnitsOS0...)
	mockUpdatedUnits = append(mockUpdatedUnits, mockUpdatedUnitsOS1...)
	var expectedUpdatedUnits []models.Unit
	for _, units := range mockUpdatedUnits {
		expectedUpdatedUnits = append(expectedUpdatedUnits, units.Units...)
	}

	if !reflect.DeepEqual(units, expectedUpdatedUnits) {
		t.Errorf("expected %#v, \n\n\n got %#v", expectedUpdatedUnits, units)
	}

	// Make usage query
	rows, err = s.db.Query(
		"SELECT avg_cpu_usage,num_updates FROM usage WHERE username = 'foo1' AND cluster_id = 'slurm-0'",
	)
	if err != nil {
		t.Errorf("Failed to make DB query: %s", err)
	}
	defer rows.Close()

	// // For debugging
	// source, _ := os.Open(filepath.Join(tmpDir, "data", base.CEEMSDBName))
	// defer source.Close()
	// destination, _ := os.Create("test.db")
	// defer destination.Close()
	// nBytes, _ := io.Copy(destination, source)
	// fmt.Println(nBytes)

	var cpuUsage models.MetricMap
	var numUpdates int64
	for rows.Next() {
		if err = rows.Scan(&cpuUsage, &numUpdates); err != nil {
			t.Errorf("failed to scan row: %s", err)
		}
	}

	if cpuUsage["usage"] < 15 {
		t.Errorf("expected 15, \n got %f", cpuUsage["usage"])
	}

	// Make projects query
	rows, err = s.db.Query(
		"SELECT users FROM projects WHERE name = 'fooprj' AND cluster_id = 'slurm-0'",
	)
	if err != nil {
		t.Errorf("Failed to make DB query: %s", err)
	}
	defer rows.Close()

	var users models.List
	for rows.Next() {
		if err = rows.Scan(&users); err != nil {
			t.Errorf("failed to scan row: %s", err)
		}
	}
	if !reflect.DeepEqual(models.List{"foo1", "foo2"}, users) {
		t.Errorf("expected users %#v, got %#v", models.List{"foo1", "foo2"}, users)
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
	expectedBackupFile := filepath.Join(c.Data.BackupPath, "backup.db")
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
	units := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID: "default",
			},
			Units: []models.Unit{
				{
					UUID: unitID,
					CreatedAt: time.Now().
						Add(time.Duration(-s.storage.retentionPeriod*24*2) * time.Hour).
						Format(base.DatetimeLayout),
				},
			},
		},
	}
	tx, _ := s.db.Begin()
	stmtMap, err := s.prepareStatements(tx)
	if err != nil {
		t.Errorf("Failed to prepare SQL statements: %s", err)
	}
	s.execStatements(stmtMap, time.Now(), units, nil, nil)

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
