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
	"elapsed_raw" integer,
	"state" text,
	"alloc_cpus" integer,
	"alloc_mem" text,
	"alloc_gpus" integer,
	"total_cpu_billing" integer,
	"total_gpu_billing" integer,
	"total_misc_billing" integer,
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
);
INSERT INTO units VALUES(1,'1479763','test_script1','acc1','grp1','usr1','2022-02-21T14:37:02+0100','2022-02-21T14:37:07+0100','2022-02-21T15:26:29+0100',1645450622000,1645450627000,1645453589000,3000,'CANCELLED by 1001',8,'320G',8,0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":1,"gid":1001,"nodelist":"compute-0","exit_code":"0:0","nodelistexp":"compute-0","partition":"part1","qos":"qos1","uid":1001,"workdir":"/home/usr1"}',0);
INSERT INTO units VALUES(2,'1481508','test_script2','acc2','grp2','usr2','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,4500,'CANCELLED by 1002',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1002,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1002,"workdir":"/home/usr2"}',0);
INSERT INTO units VALUES(3,'1481510','test_script2','acc3','grp3','usr3','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,789,'CANCELLED by 1003',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1003,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1003,"workdir":"/home/usr3"}',0);
INSERT INTO units VALUES(4,'147975','test_script1','acc3','grp3','usr3','2023-02-21T14:37:02+0100','2023-02-21T14:37:07+0100','2023-02-21T15:26:29+0100',1676986622000,1676986627000,1676989589000,3000,'CANCELLED by 1003',8,'320G',8,0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":1,"gid":1003,"nodelist":"compute-0","exit_code":"0:0","nodelistexp":"compute-0","partition":"part1","qos":"qos1","uid":1003,"workdir":"/home/usr3"}',0);
INSERT INTO units VALUES(5,'14508','test_script2','acc4','grp4','usr4','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,4500,'CANCELLED by 1004',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1004,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1004,"workdir":"/home/usr4"}',0);
INSERT INTO units VALUES(6,'147973','test_script2','acc2','gr1','usr1','2023-12-21T15:48:20+0100','2023-12-21T15:49:06+0100','2023-12-21T15:57:23+0100',1703170100000,1703170146000,1703170643000,567,'CANCELLED by 1001',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1002,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1001,"workdir":"/home/usr1"}',0);
INSERT INTO units VALUES(7,'1479765','test_script1','acc1','grp8','usr8','2023-02-21T14:37:02+0100','2023-02-21T14:37:07+0100','2023-02-21T15:26:29+0100',1676986622000,1676986627000,1676989589000,3000,'CANCELLED by 1008',8,'320G',8,0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":1,"gid":1008,"nodelist":"compute-0","exit_code":"0:0","nodelistexp":"compute-0","partition":"part1","qos":"qos1","uid":1008,"workdir":"/home/usr8"}',0);
INSERT INTO units VALUES(8,'11508','test_script2','acc1','grp15','usr15','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,4500,'CANCELLED by 1015',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1015,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1015,"workdir":"/home/usr15"}',0);
INSERT INTO units VALUES(9,'81510','test_script2','acc1','grp15','usr15','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,3533,'CANCELLED by 1015',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1015,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1015,"workdir":"/home/usr23"}',0);
INSERT INTO units VALUES(10,'1009248','test_script2','testacc','grp15','testusr','2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,17,'CANCELLED by 1015',16,'320G',8,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'{"allocnodes":2,"gid":1015,"nodelist":"compute-[0-2]","exit_code":"0:0","nodelistexp":"compute-0|compute-1|compute-2","partition":"part1","qos":"qos1","uid":1015,"workdir":"/home/usr23"}',0);
CREATE TABLE usage (
	"id" integer not null primary key,
	"num_units" integer,
	"project" text,
	"usr" text,
	"total_cpu_billing" integer,
	"total_gpu_billing" integer,
	"total_misc_billing" integer,
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
);
INSERT INTO usage VALUES(1,1,'acc1','usr1',0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(2,1,'acc2','usr2',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(3,2,'acc3','usr3',0,240,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(4,1,'acc4','usr4',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(5,1,'acc2','usr1',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(6,1,'acc1','usr8',0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(7,2,'acc1','usr15',0,320,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
INSERT INTO usage VALUES(8,1,'testacc','testusr',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0);
CREATE INDEX idx_usr_project_start ON units (usr,project,started_at);
CREATE INDEX idx_usr_uuid ON units (usr,uuid);
CREATE UNIQUE INDEX uq_uuid_start ON units (uuid,started_at);
CREATE UNIQUE INDEX uq_project ON usage (project,usr);
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
			ElapsedRaw:      567,
			State:           "CANCELLED by 1001",
			TotalGPUBilling: int64(160),
			Tags: models.Tag{
				"allocnodes":  int64(2),
				"gid":         int64(1002),
				"nodelist":    "compute-[0-2]",
				"nodelistexp": "compute-0|compute-1|compute-2",
				"exit_code":   "0:0",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1001),
				"workdir":     "/home/usr1",
			},
			Ignore: 0,
		},
		{
			ID:              1,
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
			ElapsedRaw:      3000,
			State:           "CANCELLED by 1001",
			TotalGPUBilling: int64(80),
			Tags: models.Tag{
				"allocnodes":  int64(1),
				"gid":         int64(1001),
				"nodelist":    "compute-0",
				"nodelistexp": "compute-0",
				"exit_code":   "0:0",
				"partition":   "part1",
				"qos":         "qos1",
				"uid":         int64(1001),
				"workdir":     "/home/usr1",
			},
			Ignore: 0,
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
			ID:                  7,
			Project:             "acc1",
			Usr:                 "usr15",
			NumUnits:            2,
			TotalCPUBilling:     0,
			TotalGPUBilling:     320,
			TotalMiscBilling:    0,
			AveCPUUsage:         0,
			AveCPUMemUsage:      0,
			TotalCPUEnergyUsage: 0,
			TotalCPUEmissions:   0,
			AveGPUUsage:         0,
			AveGPUMemUsage:      0,
			TotalGPUEnergyUsage: 0,
			TotalGPUEmissions:   0,
			TotalIOWriteHot:     0,
			TotalIOReadHot:      0,
			TotalIOWriteCold:    0,
			TotalIOReadCold:     0,
			TotalIngress:        0,
			TotalOutgress:       0,
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
