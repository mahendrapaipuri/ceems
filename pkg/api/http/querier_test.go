//go:build cgo
// +build cgo

package http

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB() (*sql.DB, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(
		"sqlite3", filepath.Join(currentDir, "..", "testdata", "ceems.db"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB: %w", err)
	}

	return db, nil
}

func TestUnitsQuerier(t *testing.T) {
	logger := noOpLogger

	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT * FROM %s WHERE ignore = 0 AND username in ('usr1') AND cluster_id in ('slurm-0')",
			base.UnitsDBTableName,
		),
	)

	expectedUnits := []models.Unit{
		{
			ID:              6,
			ResourceManager: "slurm",
			ClusterID:       "slurm-0",
			UUID:            "147973",
			Name:            "test_script2",
			Project:         "acc2",
			Group:           "gr1",
			User:            "usr1",
			CreatedAt:       "2023-12-21T15:48:20+0100",
			StartedAt:       "2023-12-21T15:49:06+0100",
			EndedAt:         "2023-12-21T15:57:23+0100",
			CreatedAtTS:     1703170100000,
			StartedAtTS:     1703170146000,
			EndedAtTS:       1703170643000,
			Elapsed:         "00:00:17",
			State:           "CANCELLED by 1001",
			Allocation: models.Generic{
				"billing": int64(160),
				"cpus":    int64(16),
				"gpus":    int64(8),
				"mem":     int64(343597383680),
				"nodes":   int64(2),
			},
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(497),
				"alloc_cputime":    models.JSONFloat(7952),
				"alloc_cpumemtime": models.JSONFloat(162856960),
				"alloc_gputime":    models.JSONFloat(3976),
				"alloc_gpumemtime": models.JSONFloat(497),
			},
			AveCPUUsage:         models.MetricMap{},
			AveCPUMemUsage:      models.MetricMap{},
			TotalCPUEnergyUsage: models.MetricMap{},
			TotalCPUEmissions:   models.MetricMap{},
			AveGPUUsage:         models.MetricMap{},
			AveGPUMemUsage:      models.MetricMap{},
			TotalGPUEnergyUsage: models.MetricMap{},
			TotalGPUEmissions:   models.MetricMap{},
			TotalIOWriteStats:   models.MetricMap{},
			TotalIOReadStats:    models.MetricMap{},
			TotalIngressStats:   models.MetricMap{},
			TotalEgressStats:    models.MetricMap{},
			Tags: models.Generic{
				"exit_code":   "0:0",
				"gid":         int64(1002),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1001),
				"workdir":     "/home/usr1",
			},
			Ignore:        0,
			NumUpdates:    1,
			LastUpdatedAt: "2024-07-02T14:49:39",
		},
		{
			ID:              1,
			ResourceManager: "slurm",
			ClusterID:       "slurm-0",
			UUID:            "1479763",
			Name:            "test_script1",
			Project:         "acc1",
			Group:           "grp1",
			User:            "usr1",
			CreatedAt:       "2022-02-21T14:37:02+0100",
			StartedAt:       "2022-02-21T14:37:07+0100",
			EndedAt:         "2022-02-21T15:26:29+0100",
			CreatedAtTS:     1645450622000,
			StartedAtTS:     1645450627000,
			EndedAtTS:       1645453589000,
			Elapsed:         "00:49:22",
			State:           "CANCELLED by 1001",
			Allocation: models.Generic{
				"billing": int64(80),
				"cpus":    int64(8),
				"gpus":    int64(8),
				"mem":     int64(343597383680),
				"nodes":   int64(1),
			},
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(2962),
				"alloc_cputime":    models.JSONFloat(23696),
				"alloc_cpumemtime": models.JSONFloat(970588160),
				"alloc_gputime":    models.JSONFloat(23696),
				"alloc_gpumemtime": models.JSONFloat(2962),
			},
			AveCPUUsage: models.MetricMap{
				"total": 14.79,
			},
			AveCPUMemUsage:      models.MetricMap{"total": 14.79},
			TotalCPUEnergyUsage: models.MetricMap{"total": 14.79},
			TotalCPUEmissions:   models.MetricMap{"total": 14.79},
			AveGPUUsage:         models.MetricMap{"total": 14.79},
			AveGPUMemUsage:      models.MetricMap{"total": 14.79},
			TotalGPUEnergyUsage: models.MetricMap{"total": 14.79},
			TotalGPUEmissions:   models.MetricMap{"total": 14.79},
			TotalIOWriteStats:   models.MetricMap{"bytes": 1.479763e+06, "requests": 1.479763e+07},
			TotalIOReadStats:    models.MetricMap{"bytes": 1.479763e+06, "requests": 1.479763e+07},
			TotalIngressStats:   models.MetricMap{"bytes": 1.479763e+08, "packets": 1.479763e+09},
			TotalEgressStats:    models.MetricMap{"bytes": 1.479763e+08, "packets": 1.479763e+09},
			Tags: models.Generic{
				"exit_code":   "0:0",
				"gid":         int64(1001),
				"nodelist":    "compute-0",
				"nodelistexp": "compute-0",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1001),
				"workdir":     "/home/usr1",
			},
			Ignore:        0,
			NumUpdates:    1,
			LastUpdatedAt: "2024-07-02T14:49:39",
		},
	}
	units, err := Querier[models.Unit](t.Context(), db, q, logger)
	require.NoError(t, err)
	assert.Equal(t, expectedUnits, units)
}

func TestUsageQuerier(t *testing.T) {
	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT * FROM %s WHERE username IN ('usr15') AND cluster_id IN ('slurm-1')",
			base.UsageDBTableName,
		),
	)

	expectedUsageStats := []models.Usage{
		{
			ID:              15,
			ResourceManager: "slurm",
			ClusterID:       "slurm-1",
			NumUnits:        2,
			Project:         "acc1",
			Group:           "grp15",
			User:            "usr15",
			LastUpdatedAt:   "2024-07-02T14:49:39",
			TotalTime: models.MetricMap{
				"walltime":         models.JSONFloat(994),
				"alloc_cputime":    models.JSONFloat(15904),
				"alloc_cpumemtime": models.JSONFloat(325713920),
				"alloc_gputime":    models.JSONFloat(7952),
				"alloc_gpumemtime": models.JSONFloat(994),
			},
			AveCPUUsage:         models.MetricMap{"total": 46.505},
			AveCPUMemUsage:      models.MetricMap{"total": 46.505},
			TotalCPUEnergyUsage: models.MetricMap{"total": 93.01},
			TotalCPUEmissions:   models.MetricMap{"total": 93.01},
			AveGPUUsage:         models.MetricMap{"total": 46.505},
			AveGPUMemUsage:      models.MetricMap{"total": 46.505},
			TotalGPUEnergyUsage: models.MetricMap{"total": 93.01},
			TotalGPUEmissions:   models.MetricMap{"total": 93.01},
			TotalIOWriteStats:   models.MetricMap{"bytes": 93018, "requests": 930180},
			TotalIOReadStats:    models.MetricMap{"bytes": 93018, "requests": 930180},
			TotalIngressStats:   models.MetricMap{"bytes": 9.3018e+06, "packets": 9.3018e+07},
			TotalEgressStats:    models.MetricMap{"bytes": 9.3018e+06, "packets": 9.3018e+07},
			NumUpdates:          2,
		},
	}
	usageStats, err := Querier[models.Usage](t.Context(), db, q, noOpLogger)
	require.NoError(t, err)
	assert.Equal(t, expectedUsageStats, usageStats)
}

func TestProjectQuerier(t *testing.T) {
	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT * FROM %s WHERE name IN ('acc1') AND cluster_id IN ('slurm-1')",
			base.ProjectsDBTableName,
		),
	)

	expectedProjects := []models.Project{
		{
			ID:              6,
			Name:            "acc1",
			ResourceManager: "slurm",
			ClusterID:       "slurm-1",
			Users:           models.List{"usr1", "usr15", "usr8"},
			LastUpdatedAt:   "2024-07-02T14:49:39",
		},
	}
	projects, err := Querier[models.Project](t.Context(), db, q, noOpLogger)
	require.NoError(t, err)
	assert.Equal(t, expectedProjects, projects)
}

func TestUserQuerier(t *testing.T) {
	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT * FROM %s WHERE name IN ('usr1') AND cluster_id IN ('slurm-1')",
			base.UsersDBTableName,
		),
	)

	expectedUsers := []models.User{
		{
			ID:              9,
			Name:            "usr1",
			ResourceManager: "slurm",
			ClusterID:       "slurm-1",
			Projects:        models.List{"acc1", "acc2"},
			LastUpdatedAt:   "2024-07-02T14:49:39",
		},
	}
	users, err := Querier[models.User](t.Context(), db, q, noOpLogger)
	require.NoError(t, err)
	assert.Equal(t, expectedUsers, users)
}

func TestClusterQuerier(t *testing.T) {
	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query("SELECT DISTINCT cluster_id, resource_manager FROM " + base.UnitsDBTableName)

	expectedClusters := []models.Cluster{
		{
			ID:      "slurm-0",
			Manager: "slurm",
		},
		{
			ID:      "slurm-1",
			Manager: "slurm",
		},
	}
	clusters, err := Querier[models.Cluster](t.Context(), db, q, noOpLogger)
	require.NoError(t, err)
	assert.Equal(t, expectedClusters, clusters)
}

func TestStatsQuerier(t *testing.T) {
	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT %s FROM %s", statsQuery, base.UnitsDBTableName))

	expectedStats := []models.Stat{
		{
			ClusterID:        "slurm-0",
			ResourceManager:  "slurm",
			NumUnits:         24,
			NumInActiveUnits: 20,
			NumActiveUnits:   4,
			NumProjects:      5,
			NumUsers:         7,
		},
	}
	stats, err := Querier[models.Stat](t.Context(), db, q, noOpLogger)
	require.NoError(t, err)
	assert.Equal(t, expectedStats, stats)
}

func TestKeysQuerier(t *testing.T) {
	db, err := setupTestDB()
	require.NoError(t, err, "failed to setup test DB")
	defer db.Close()

	// Query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT DISTINCT json_each.key AS name FROM %s, json_each(%s)",
			base.UnitsDBTableName,
			"total_io_read_stats",
		),
	)

	expectedKeys := []models.Key{
		{
			Name: "bytes",
		},
		{
			Name: "requests",
		},
	}
	keys, err := Querier[models.Key](t.Context(), db, q, noOpLogger)
	require.NoError(t, err)
	assert.Equal(t, expectedKeys, keys)
}

func TestQueryBuilder(t *testing.T) {
	expectedQueryString := "SELECT * FROM table WHERE a IN (?,?) AND b IN (?,?) AND c BETWEEN (?) AND (?)"
	expectedQueryParams := []string{"a1", "a2", "10", "20", "2023-01-01", "2023-02-01"}

	// StartedAt query
	q := Query{}
	q.query("SELECT * FROM table")
	q.query(" WHERE a IN ")
	q.param([]string{"a1", "a2"})

	q.query(" AND b IN ")
	q.param([]string{"10", "20"})

	q.query(" AND c BETWEEN ")
	q.param([]string{"2023-01-01"})
	q.query(" AND ")
	q.param([]string{"2023-02-01"})

	// Get built query
	queryString, queryParams := q.get()
	require.Equal(t, expectedQueryString, queryString)
	assert.Equal(t, expectedQueryParams, queryParams)
}

func TestSubQueryBuilder(t *testing.T) {
	expectedQueryString := "SELECT * FROM table WHERE a IN (SELECT a FROM table1 WHERE d IN (?,?)) AND b IN (?,?)"
	expectedQueryParams := []string{"d1", "d2", "10", "20"}

	// Sub query
	qSub := Query{}
	qSub.query("SELECT a FROM table1")
	qSub.query(" WHERE d IN ")
	qSub.param([]string{"d1", "d2"})

	// StartedAt query
	q := Query{}
	q.query("SELECT * FROM table")
	q.query(" WHERE a IN ")
	q.subQuery(qSub)

	q.query(" AND b IN ")
	q.param([]string{"10", "20"})

	// Get built query
	queryString, queryParams := q.get()
	require.Equal(t, expectedQueryString, queryString)
	assert.Equal(t, expectedQueryParams, queryParams)
}
