package http

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

func setupTestDB() *sql.DB {
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}

	db, err := sql.Open(
		"sqlite3", filepath.Join(currentDir, "..", "testdata", "ceems.db"),
	)
	if err != nil {
		fmt.Println("failed to open DB")
	}
	return db
}

func TestJobsQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB()
	defer db.Close()

	// Query
	q := Query{}
	q.query(
		fmt.Sprintf(
			"SELECT * FROM %s WHERE ignore = 0 AND usr in ('usr1') AND cluster_id in ('slurm-0')",
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
			Grp:             "gr1",
			Usr:             "usr1",
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
			TotalWallTime:   int64(497),
			TotalCPUTime:    int64(7952),
			TotalGPUTime:    int64(3976),
			TotalCPUMemTime: int64(162856960),
			TotalGPUMemTime: int64(497),
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
			LastUpdatedAt: "2024-06-20T13:59:32",
		},
		{
			ID:              1,
			ResourceManager: "slurm",
			ClusterID:       "slurm-0",
			UUID:            "1479763",
			Name:            "test_script1",
			Project:         "acc1",
			Grp:             "grp1",
			Usr:             "usr1",
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
			TotalWallTime:   int64(2962),
			TotalCPUTime:    int64(23696),
			TotalGPUTime:    int64(23696),
			TotalCPUMemTime: int64(970588160),
			TotalGPUMemTime: int64(2962),
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
			LastUpdatedAt: "2024-06-20T13:59:32",
		},
	}
	units, err := Querier[models.Unit](db, q, logger)
	if err != nil {
		t.Errorf("failed to query for units: %s", err)
	}
	if !reflect.DeepEqual(expectedUnits, units) {
		t.Errorf("expected units %#v \n, got %#v", expectedUnits, units)
	}
}

func TestUsageQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB()
	defer db.Close()

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT * FROM %s WHERE usr IN ('usr15') AND cluster_id IN ('slurm-1')", base.UsageDBTableName))

	expectedUsageStats := []models.Usage{
		{
			ID:              15,
			ResourceManager: "slurm",
			ClusterID:       "slurm-1",
			NumUnits:        2,
			Project:         "acc1",
			Usr:             "usr15",
			LastUpdatedAt:   "2024-06-20T13:59:32",
			TotalWallTime:   994,
			TotalCPUTime:    15904,
			TotalGPUTime:    7952,
			TotalCPUMemTime: 325713920,
			TotalGPUMemTime: 994,
			NumUpdates:      2,
		},
	}
	usageStats, err := Querier[models.Usage](db, q, logger)
	if err != nil {
		t.Errorf("failed to query for usage: %s", err)
	}
	if !reflect.DeepEqual(expectedUsageStats, usageStats) {
		t.Errorf("expected usage %#v \n, got %#v", expectedUsageStats, usageStats)
	}
}

func TestProjectQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB()
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
			LastUpdatedAt:   "2024-06-20T13:59:32",
		},
	}
	projects, err := Querier[models.Project](db, q, logger)
	if err != nil {
		t.Errorf("failed to query for projects: %s", err)
	}
	if !reflect.DeepEqual(expectedProjects, projects) {
		t.Errorf("expected projects %#v \n, got %#v", expectedProjects, projects)
	}
}

func TestUserQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB()
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
			LastUpdatedAt:   "2024-06-20T13:59:32",
		},
	}
	users, err := Querier[models.User](db, q, logger)
	if err != nil {
		t.Errorf("failed to query for users: %s", err)
	}
	if !reflect.DeepEqual(expectedUsers, users) {
		t.Errorf("expected users %#v \n, got %#v", expectedUsers, users)
	}
}

func TestClusterQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB()
	defer db.Close()

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT DISTINCT cluster_id, resource_manager FROM %s", base.UnitsDBTableName))

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
	clusters, err := Querier[models.Cluster](db, q, logger)
	if err != nil {
		t.Errorf("failed to query for clusters: %s", err)
	}
	if !reflect.DeepEqual(expectedClusters, clusters) {
		t.Errorf("expected clusters %#v \n, got %#v", expectedClusters, clusters)
	}
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
	if queryString != expectedQueryString {
		t.Errorf("expected query string %s, got %s", expectedQueryString, queryString)
	}
	if !reflect.DeepEqual(expectedQueryParams, queryParams) {
		t.Errorf("expected query parameters %v, got %v", expectedQueryParams, queryParams)
	}
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
	if queryString != expectedQueryString {
		t.Errorf("expected query string %s, got %s", expectedQueryString, queryString)
	}
	if !reflect.DeepEqual(expectedQueryParams, queryParams) {
		t.Errorf("expected query parameters %v, got %v", expectedQueryParams, queryParams)
	}
}
