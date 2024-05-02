package http

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

func setupTestDB(d string) *sql.DB {
	db, err := sql.Open("sqlite3", filepath.Join(d, "test.db"))
	if err != nil {
		fmt.Printf("failed to create DB")
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE schema_migrations (version uint64,dirty bool);
INSERT INTO schema_migrations VALUES(6,0);
CREATE TABLE units (
	"id" integer not null primary key,
	"uuid" text,
	"name" text,
	"project" text,
	"grp" text,
	"usr" text,
	"created_at" text,
	"started_at" text,
	"ended_at" text,
	"created_at_ts" integer,
	"started_at_ts" integer,
	"ended_at_ts" integer,
	"elapsed" text,
	"elapsed_raw" integer,
	"state" text,
	"allocation" text default '{}',
	"total_cputime_seconds" integer,
	"total_gputime_seconds" integer,
	"total_misctime_seconds" integer,
	"avg_cpu_usage" real,
	"avg_cpu_mem_usage" real,
	"total_cpu_energy_usage_kwh" real,
	"total_cpu_emissions_gms" real,
	"avg_gpu_usage" real,
	"avg_gpu_mem_usage" real,
	"total_gpu_energy_usage_kwh" real,
	"total_gpu_emissions_gms" real,
	"total_io_write_hot_gb" real,
	"total_io_read_hot_gb" real,
	"total_io_write_cold_gb" real,
	"total_io_read_cold_gb" real,
	"total_ingress_in_gb" real,
	"total_outgress_in_gb" real,
	"tags" text default '{}',
	"ignore" integer
, num_updates integer default 0, "resource_manager" text default "", "total_walltime_seconds" integer);
INSERT INTO units VALUES(1,'1479763','test_script1','acc1','grp1','usr1','2022-02-21T14:37:02+0100','2022-02-21T14:37:07+0100','2022-02-21T15:26:29+0100',1645450622000,1645450627000,1645453589000,'00:49:22',NULL,'CANCELLED by 1001','{"billing":80,"cpus":8,"gpus":8,"mem":"320G","nodes":1}',23696,23696,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1001,"nodelist":"compute-0","nodelistexp":"compute-0","partition":"part1","qos":"qos1","uid":1001,"workdir":"/home/usr1"}',0,1,'slurm',2962);
INSERT INTO units VALUES(2,'1481508','test_script2','acc2','grp2','usr2','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:08:17',NULL,'CANCELLED by 1002','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1002,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1002,"workdir":"/home/usr2"}',0,1,'slurm',497);
INSERT INTO units VALUES(3,'1481510','test_script2','acc3','grp3','usr3','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:00:17',NULL,'CANCELLED by 1003','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1003,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1003,"workdir":"/home/usr3"}',0,1,'slurm',497);
INSERT INTO units VALUES(4,'147975','test_script1','acc3','grp3','usr3','2023-02-21T14:37:02+0100','2023-02-21T14:37:07+0100','2023-02-21T15:26:29+0100',1676986622000,1676986627000,1676989589000,'00:49:22',NULL,'CANCELLED by 1003','{"billing":80,"cpus":8,"gpus":8,"mem":"320G","nodes":1}',23696,23696,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1003,"nodelist":"compute-0","nodelistexp":"compute-0","partition":"part1","qos":"qos1","uid":1003,"workdir":"/home/usr3"}',0,1,'slurm',2962);
INSERT INTO units VALUES(5,'14508','test_script2','acc4','grp4','usr4','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:08:17',NULL,'CANCELLED by 1004','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1004,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1004,"workdir":"/home/usr4"}',0,1,'slurm',497);
INSERT INTO units VALUES(6,'147973','test_script2','acc2','gr1','usr1','2023-12-21T15:48:20+0100','2023-12-21T15:49:06+0100','2023-12-21T15:57:23+0100',1703170100000,1703170146000,1703170643000,'00:00:17',NULL,'CANCELLED by 1001','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1002,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1001,"workdir":"/home/usr1"}',0,1,'slurm',497);
INSERT INTO units VALUES(7,'1479765','test_script1','acc1','grp8','usr8','2023-02-21T14:37:02+0100','2023-02-21T14:37:07+0100','2023-02-21T15:26:29+0100',1676986622000,1676986627000,1676989589000,'00:49:22',NULL,'CANCELLED by 1008','{"billing":80,"cpus":8,"gpus":8,"mem":"320G","nodes":1}',23696,23696,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1008,"nodelist":"compute-0","nodelistexp":"compute-0","partition":"part1","qos":"qos1","uid":1008,"workdir":"/home/usr8"}',0,1,'slurm',2962);
INSERT INTO units VALUES(8,'11508','test_script2','acc1','grp15','usr15','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:08:17',NULL,'CANCELLED by 1015','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1015,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1015,"workdir":"/home/usr15"}',0,1,'slurm',497);
INSERT INTO units VALUES(9,'81510','test_script2','acc1','grp15','usr15','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:00:17',NULL,'CANCELLED by 1015','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1015,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1015,"workdir":"/home/usr23"}',0,1,'slurm',497);
INSERT INTO units VALUES(10,'1009248','test_script2','testacc','grp15','testusr','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:00:17',NULL,'CANCELLED by 1015','{"billing":160,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1015,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1015,"workdir":"/home/usr23"}',0,1,'slurm',497);
INSERT INTO units VALUES(11,'2009248','test_script2','acc3','grp3','usr3','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','Unknown',1676990900000,1676990946000,0,'00:00:17',NULL,'RUNNING','{"billing":0,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',547616,273808,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1003,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part2","qos":"qos3","uid":1003,"workdir":"/home/usr3"}',0,1,'slurm',34226);
INSERT INTO units VALUES(12,'3009248','test_script2','acc2','grp2','usr2','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','Unknown',1676990900000,1676990946000,0,'00:00:17',NULL,'RUNNING','{"billing":0,"cpus":16,"gpus":8,"mem":"320G","nodes":2}',547616,273808,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"exit_code":"0:0","gid":1002,"nodelist":"compute-[0-2]","nodelistexp":"compute-0|compute-1|compute-2","partition":"part3","qos":"qos3","uid":1002,"workdir":"/home/usr2"}',0,1,'slurm',34226);
CREATE TABLE usage (
	"id" integer not null primary key,
	"num_units" integer,
	"project" text,
	"usr" text,
	"total_cputime_seconds" integer,
	"total_gputime_seconds" integer,
	"total_misctime_seconds" integer,
	"avg_cpu_usage" real,
	"avg_cpu_mem_usage" real,
	"total_cpu_energy_usage_kwh" real,
	"total_cpu_emissions_gms" real,
	"avg_gpu_usage" real,
	"avg_gpu_mem_usage" real,
	"total_gpu_energy_usage_kwh" real,
	"total_gpu_emissions_gms" real,
	"total_io_write_hot_gb" real,
	"total_io_read_hot_gb" real,
	"total_io_write_cold_gb" real,
	"total_io_read_cold_gb" real,
	"total_ingress_in_gb" real,
	"total_outgress_in_gb" real
, num_updates integer default 0, "resource_manager" text default "", "last_updated_at" text, "total_walltime_seconds" integer);
INSERT INTO usage VALUES(1,1,'acc1','usr1',23696,23696,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,1,'slurm','2024-04-24T09:30:26',2962);
INSERT INTO usage VALUES(2,1,'acc2','usr2',555568,277784,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,2,'slurm','2024-04-24T09:30:26',34723);
INSERT INTO usage VALUES(3,2,'acc3','usr3',579264,301480,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,3,'slurm','2024-04-24T09:30:26',37685);
INSERT INTO usage VALUES(4,1,'acc4','usr4',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,1,'slurm','2024-04-24T09:30:26',497);
INSERT INTO usage VALUES(5,1,'acc2','usr1',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,1,'slurm','2024-04-24T09:30:26',497);
INSERT INTO usage VALUES(6,1,'acc1','usr8',23696,23696,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,1,'slurm','2024-04-24T09:30:26',2962);
INSERT INTO usage VALUES(7,2,'acc1','usr15',15904,7952,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,2,'slurm','2024-04-24T09:30:26',994);
INSERT INTO usage VALUES(8,1,'testacc','testusr',7952,3976,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,1,'slurm','2024-04-24T09:30:26',497);
CREATE UNIQUE INDEX version_unique ON schema_migrations (version);
CREATE INDEX idx_usr_project_start ON units (usr,project,started_at);
CREATE INDEX idx_usr_uuid ON units (usr,uuid);
CREATE UNIQUE INDEX uq_rm_uuid_start ON units (resource_manager,uuid,started_at);
CREATE UNIQUE INDEX uq_rm_project_usr ON usage (resource_manager,usr,project);
COMMIT;
	`
	_, err = db.Exec(stmts)
	if err != nil {
		fmt.Printf("failed to insert mock data into DB: %s", err)
	}
	return db
}

func TestJobsQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB(t.TempDir())

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT * FROM %s WHERE ignore = 0 AND usr in ('usr1')", base.UnitsDBTableName))

	expectedUnits := []models.Unit{
		{
			ID:              6,
			ResourceManager: "slurm",
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
				"mem":     "320G",
				"nodes":   int64(2),
			},
			TotalWallTime: int64(497),
			TotalCPUTime:  int64(7952),
			TotalGPUTime:  int64(3976),
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
			Ignore:     0,
			NumUpdates: 1,
		},
		{
			ID:              1,
			ResourceManager: "slurm",
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
				"mem":     "320G",
				"nodes":   int64(1),
			},
			TotalWallTime: int64(2962),
			TotalCPUTime:  int64(23696),
			TotalGPUTime:  int64(23696),
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
			Ignore:     0,
			NumUpdates: 1,
		},
	}
	units, err := querier(db, q, unitsResourceName, logger)
	if err != nil {
		t.Errorf("failed to query for units: %s", err)
	}
	if !reflect.DeepEqual(expectedUnits, units.([]models.Unit)) {
		t.Errorf("expected units %#v \n, got %#v", expectedUnits, units.([]models.Unit))
	}
}

func TestUsageQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB(t.TempDir())

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT * FROM %s WHERE usr IN ('usr15')", base.UsageDBTableName))

	expectedUsageStats := []models.Usage{
		{
			ID:              7,
			ResourceManager: "slurm",
			NumUnits:        2,
			Project:         "acc1",
			Usr:             "usr15",
			LastUpdatedAt:   "2024-04-24T09:30:26",
			TotalWallTime:   994,
			TotalCPUTime:    15904,
			TotalGPUTime:    7952,
			NumUpdates:      2,
		},
	}
	usageStats, err := querier(db, q, usageResourceName, logger)
	if err != nil {
		t.Errorf("failed to query for usage: %s", err)
	}
	if !reflect.DeepEqual(expectedUsageStats, usageStats.([]models.Usage)) {
		t.Errorf("expected usage %#v \n, got %#v", expectedUsageStats, usageStats)
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
