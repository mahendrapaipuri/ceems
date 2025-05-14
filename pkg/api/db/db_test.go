//go:build cgo
// +build cgo

package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/grafana"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFetcherOne struct {
	logger *slog.Logger
}

type mockFetcherTwo struct {
	logger *slog.Logger
}

// slow fetcher.
type mockFetcherThree struct {
	logger *slog.Logger
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

func newMockManager(logger *slog.Logger) (*resource.Manager, error) {
	return &resource.Manager{
		Logger: logger,
		Fetchers: []resource.Fetcher{
			&mockFetcherOne{logger: logger},
			&mockFetcherTwo{logger: logger},
			&mockFetcherThree{logger: logger},
		},
	}, nil
}

// FetchUnits implements collection units between start and end times.
func (m *mockFetcherOne) FetchUnits(_ context.Context, start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return mockUnitsOne, nil
}

// FetchUsersProjects implements collection project user association.
func (m *mockFetcherOne) FetchUsersProjects(
	_ context.Context,
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return mockUsersOne, mockProjectsOne, nil
}

// FetchUnits implements collection units between start and end times.
func (m *mockFetcherTwo) FetchUnits(_ context.Context, start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	return mockUnitsTwo, nil
}

// FetchUsersProjects implements collection project user association.
func (m *mockFetcherTwo) FetchUsersProjects(
	_ context.Context,
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return nil, nil, errors.New("failed to fetch associations")
}

// Return error for this mockFetcher.
func (m *mockFetcherThree) FetchUnits(
	_ context.Context,
	start time.Time,
	end time.Time,
) ([]models.ClusterUnits, error) {
	time.Sleep(10 * time.Millisecond)

	return nil, errors.New("failed to fetch units")
}

// FetchUsersProjects implements collection project user association.
func (m *mockFetcherThree) FetchUsersProjects(
	_ context.Context,
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	time.Sleep(10 * time.Millisecond)

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
	logger *slog.Logger
}

// GetUnits implements collection units between start and end times.
func (m mockUpdaterSlurm00) Update(
	_ context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsSlurm00
}

type mockUpdaterSlurm01 struct {
	logger *slog.Logger
}

// GetUnits implements collection units between start and end times.
func (m mockUpdaterSlurm01) Update(
	_ context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsSlurm01
}

type mockUpdaterSlurm1 struct {
	logger *slog.Logger
}

// GetUnits implements collection units between start and end times.
func (m mockUpdaterSlurm1) Update(
	_ context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsSlurm1
}

type mockUpdaterOS0 struct {
	logger *slog.Logger
}

// GetUnits implements collection units between start and end times.
func (m mockUpdaterOS0) Update(
	_ context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsOS0
}

type mockUpdaterOS1 struct {
	logger *slog.Logger
}

// GetUnits implements collection units between start and end times.
func (m mockUpdaterOS1) Update(
	_ context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return mockUpdatedUnitsOS1
}

func newMockUpdater(logger *slog.Logger) (*updater.UnitUpdater, error) {
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

func prepareMockConfig(tmpDir string) (*Config, error) {
	dataDir := filepath.Join(tmpDir, "data")
	dataBackupDir := filepath.Join(tmpDir, "data-backup")

	// Create data directory
	if err := os.Mkdir(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("Failed to create data directory: %w", err)
	}

	if err := os.Mkdir(dataBackupDir, 0o750); err != nil {
		return nil, fmt.Errorf("Failed to create data directory: %w", err)
	}

	// Create an empty file for sacct
	sacctFile, err := os.Create(filepath.Join(tmpDir, "sacct"))
	if err != nil {
		return nil, err
	}

	sacctFile.Close()

	return &Config{
		Logger: slog.New(slog.DiscardHandler),
		Data: DataConfig{
			Path:              dataDir,
			BackupPath:        dataBackupDir,
			LastUpdate:        DateTime{time.Now()},
			MaxUpdateInterval: model.Duration(time.Hour),
			RetentionPeriod:   model.Duration(24 * time.Hour),
			Timezone:          Timezone{Location: time.UTC},
		},
		Admin: AdminConfig{
			Users: []string{"adm1", "adm2"},
		},
		ResourceManager: newMockManager,
		Updater:         newMockUpdater,
	}, nil
}

func populateDBWithMockData(ctx context.Context, s *stats) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	s.execStatements(ctx, tx, time.Now().Add(-time.Minute), time.Now(), mockUnitsOne, mockUsersOne, mockProjectsOne)
	s.execStatements(ctx, tx, time.Now().Add(-time.Minute), time.Now(), mockUnitsTwo, nil, nil)
	tx.Commit()

	return nil
}

func TestNewUnitStatsDB(t *testing.T) {
	var (
		s   *stats
		err error
	)

	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	c.Data.LastUpdate.Time, _ = time.Parse("2006-01-02", "2023-12-20")
	s, err = New(c)
	require.NoError(t, err, "failed to create new stats")

	// Check DB file exists
	_, err = os.Stat(c.Data.Path)
	require.NoError(t, err, "DB file not created")

	// Insert a dummy entry into DB
	_, err = s.db.Exec(`INSERT INTO usage(last_updated_at) VALUES ("2023-12-20T00:00:00")`)
	require.NoError(t, err, "failed to insert dummy entry into DB")
	s.Stop()

	// Make again a new stats DB with lastUpdateTime in the past of the one in DB
	c.Data.LastUpdate.Time, _ = time.Parse("2006-01-02", "2023-12-19")
	s, err = New(c)
	require.NoError(t, err, "failed to create new stats")

	// Check content of last update time file. It should not change
	assert.Contains(
		t,
		s.storage.lastUpdateTime.String(),
		"2023-12-20 00:00:00",
		"Expected last update time is 2023-12-20 00:00:00",
	)
	s.Stop()
}

func TestUnitStatsDBEntries(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	ctx := t.Context()

	// Make new stats DB
	s, err := New(c)
	require.NoError(t, err, "failed to create new stats")

	// Fetch units
	var expectedUnits []models.ClusterUnits
	expectedUnits = append(expectedUnits, mockUnitsOne...)
	expectedUnits = append(expectedUnits, mockUnitsTwo...)
	fetchedUnits, err := s.manager.FetchUnits(ctx, time.Now(), time.Now())
	require.Error(t, err, "expected one error from fetching units")
	assert.ElementsMatch(t, fetchedUnits, expectedUnits, "expected and got cluster units differ")

	// Try to insert data
	err = s.Collect(ctx)
	require.NoError(t, err, "failed to collect units data")

	// Make units query
	rows, err := s.db.Query(
		"SELECT uuid,username,project,total_time_seconds,avg_cpu_usage,avg_cpu_mem_usage,total_cpu_energy_usage_kwh,total_cpu_emissions_gms,avg_gpu_usage,avg_gpu_mem_usage,total_gpu_energy_usage_kwh,total_gpu_emissions_gms FROM units ORDER BY uuid",
	)
	require.NoError(t, err, "failed to make DB query")

	defer rows.Close()
	require.NoError(t, rows.Err())

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

	assert.ElementsMatch(t, units, expectedUpdatedUnits, "expected and got updated cluster units differ")

	// Make usage query
	rows, err = s.db.Query(
		"SELECT avg_cpu_usage,num_updates FROM usage WHERE username = 'foo1' AND cluster_id = 'slurm-0'",
	)
	require.NoError(t, err, "failed to make DB query")

	defer rows.Close()
	require.NoError(t, rows.Err())

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

	require.NoError(t, rows.Err())
	assert.InEpsilon(t, 15, float64(cpuUsage["usage"]), 0, "expected cpuUsage = 15")

	// Make projects query
	rows, err = s.db.Query(
		"SELECT users FROM projects WHERE name = 'fooprj' AND cluster_id = 'slurm-0'",
	)
	require.NoError(t, err, "Failed to make DB query")

	defer rows.Close()
	require.NoError(t, rows.Err())

	var users models.List
	for rows.Next() {
		if err = rows.Scan(&users); err != nil {
			t.Errorf("failed to scan row: %s", err)
		}
	}

	assert.ElementsMatch(t, models.List{"foo1", "foo2"}, users, "expected and got users differ")

	// Close DB
	s.Stop()
}

func TestCollectContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Start a context with timeout less than 10 milliseconds
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	defer cancel()

	// Make new stats DB
	s, err := New(c)
	require.NoError(t, err, "failed to create new stats")

	// Collect data and it should return an error
	err = s.Collect(ctx)
	require.ErrorIs(t, err, ctx.Err())
}

func TestUnitStatsDBEntriesHistorical(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	c.Data.LastUpdate.Time = time.Now().Add(-2 * time.Hour)
	ctx := t.Context()

	// Make new stats DB
	s, err := New(c)
	require.NoError(t, err, "Failed to create new stats")

	// Fetch units
	var expectedUnits []models.ClusterUnits
	expectedUnits = append(expectedUnits, mockUnitsOne...)
	expectedUnits = append(expectedUnits, mockUnitsTwo...)
	fetchedUnits, err := s.manager.FetchUnits(ctx, time.Now(), time.Now())
	require.Error(t, err, "expected one error from fetching units")
	assert.ElementsMatch(t, fetchedUnits, expectedUnits, "expected and got cluster units differ")

	// Try to insert data
	err = s.Collect(ctx)
	require.NoError(t, err, "Failed to collect units data")

	// Make units query
	rows, err := s.db.Query(
		"SELECT uuid,username,project,total_time_seconds,avg_cpu_usage,avg_cpu_mem_usage,total_cpu_energy_usage_kwh,total_cpu_emissions_gms,avg_gpu_usage,avg_gpu_mem_usage,total_gpu_energy_usage_kwh,total_gpu_emissions_gms FROM units ORDER BY uuid",
	)
	require.NoError(t, err, "Failed to make DB query")

	defer rows.Close()
	require.NoError(t, rows.Err())

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

	assert.ElementsMatch(t, units, expectedUpdatedUnits, "expected and got updated cluster units differ")

	// Close DB
	s.Stop()
}

func TestUnitStatsDBLock(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	s, err := New(c)
	defer s.Stop()
	require.NoError(t, err, "Failed to create new stats")

	// Beging exclusive transcation to lock DB
	_, err = s.db.Exec("BEGIN EXCLUSIVE")
	require.NoError(t, err)

	// Try to insert data. It should fail
	err = s.Collect(t.Context())
	require.Error(t, err, "expected error due to DB lock")
	s.db.Exec("COMMIT")
}

func TestUnitStatsDBVacuum(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	s, err := New(c)
	defer s.Stop()
	require.NoError(t, err, "Failed to create new stats")

	// Populate DB with data
	err = populateDBWithMockData(t.Context(), s)
	require.NoError(t, err, "failed to insert data in test DB")

	// Run vacuum
	err = s.vacuum(t.Context())
	require.NoError(t, err, "failed to vacuum DB")

	// Run vacuum with timeout. Hoping that vacuum takes more than nanosecond
	ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()

	err = s.vacuum(ctx)
	require.ErrorIs(t, err, ctx.Err())
}

func TestUnitStatsDBBackup(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	s, err := New(c)
	defer s.Stop()
	require.NoError(t, err, "Failed to create new stats")

	// Populate DB with data
	err = populateDBWithMockData(t.Context(), s)
	require.NoError(t, err, "failed to insert data in test DB")

	// // For debugging
	// source, _ := os.Open(filepath.Join(tmpDir, "data", base.CEEMSDBName))
	// defer source.Close()
	// destination, _ := os.Create("test.db")
	// defer destination.Close()
	// nBytes, _ := io.Copy(destination, source)
	// fmt.Println(nBytes)

	// Run backup
	expectedBackupFile := filepath.Join(c.Data.BackupPath, "backup.db")
	err = s.backup(t.Context(), expectedBackupFile)
	require.NoError(t, err, "failed to backup DB")

	_, err = os.Stat(expectedBackupFile)
	require.NoError(t, err, "Backup DB file not found")

	// Check contents of backed up DB
	var numRows int

	db, _, err := openDBConnection(expectedBackupFile)
	if err != nil {
		t.Errorf("Failed to create DB connection to backup DB: %s", err)
	}

	rows, err := db.Query("SELECT * FROM " + base.UnitsDBTableName) //nolint:gosec
	require.NoError(t, err)

	defer rows.Close()
	require.NoError(t, rows.Err())

	for rows.Next() {
		numRows += 1
	}

	assert.Equal(t, 7, numRows, "Backup DB check failed. Expected rows 7")
}

func TestAdminUsersDBUpdate(t *testing.T) {
	// Start test server
	expected := []grafana.GrafanaTeamsReponse{
		{Login: "foo"}, {Login: "bar"},
	}

	t.Setenv("GRAFANA_API_TOKEN", "foo")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(&expected); err != nil {
			w.Write([]byte("KO"))
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	s, err := New(c)
	defer s.Stop()
	require.NoError(t, err, "failed to create new stats")

	// Make backup dir non existent
	s.admin.grafana, err = grafana.New(server.URL, config_util.HTTPClientConfig{}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	// Setup a mock teamIDs
	s.admin.grafanaAdminTeamsIDs = []string{"1"}

	// update admin users
	err = s.updateAdminUsers(t.Context())
	require.NoError(t, err, "failed to update admin users")

	// Check admin users from grafana
	assert.ElementsMatch(t, s.admin.users["grafana"], []string{"foo", "bar"})

	// do second update of admin users and users should not be duplicated
	err = s.updateAdminUsers(t.Context())
	require.NoError(t, err, "failed to update admin users")

	// Check admin users from grafana
	assert.ElementsMatch(t, s.admin.users["grafana"], []string{"foo", "bar"})
}

func TestStatsDBBackup(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	s, err := New(c)
	defer s.Stop()
	require.NoError(t, err, "failed to create new stats")

	// Make backup dir non existent
	s.storage.dbBackupPath = tmpDir

	// Populate DB with data
	err = populateDBWithMockData(t.Context(), s)
	require.NoError(t, err, "failed to insert data in test DB")

	// Run backup
	err = s.createBackup(t.Context())
	require.NoError(t, err, "failed to backup DB")
}

func TestUnitStatsDeleteOldUnits(t *testing.T) {
	tmpDir := t.TempDir()
	unitID := "1111"
	c, err := prepareMockConfig(tmpDir)
	require.NoError(t, err, "failed to create mock config")

	// Make new stats DB
	s, err := New(c)
	defer s.Stop()
	require.NoError(t, err, "failed to create new stats")

	// Add new row that should be deleted
	units := []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID: "default",
			},
			Units: []models.Unit{
				{
					UUID:      unitID,
					StartedAt: time.Now().Add(-s.storage.retentionPeriod * 2).Format(base.DatetimeLayout),
				},
			},
		},
	}
	ctx := t.Context()
	tx, err := s.db.Begin()
	require.NoError(t, err)
	// stmtMap, err := s.prepareStatements(ctx, tx)
	// require.NoError(t, err)
	err = s.execStatements(ctx, tx, time.Now().Add(-time.Minute), time.Now(), units, nil, nil)
	require.NoError(t, err)

	// Now clean up DB for old units
	err = s.purgeExpiredUnits(ctx, tx)
	require.NoError(t, err, "failed to delete old entries in DB")
	tx.Commit()

	// Query for deleted unit
	result, err := s.db.Prepare(
		fmt.Sprintf("SELECT COUNT(uuid) FROM %s WHERE uuid = ?;", base.UnitsDBTableName),
	)
	require.NoError(t, err)
	defer result.Close()

	var numRows int
	err = result.QueryRow(unitID).Scan(&numRows)
	require.NoError(t, err, "failed to query DB")
	assert.Equal(t, 0, numRows, "expected 0 rows after deletion")
}
