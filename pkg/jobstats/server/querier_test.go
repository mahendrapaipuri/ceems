package server

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
)

func setupTestDB(d string) *sql.DB {
	db, err := sql.Open("sqlite3", filepath.Join(d, "test.db"))
	if err != nil {
		fmt.Printf("failed to create DB")
	}

	stmts := `
PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE jobs (
	"id" integer not null primary key,
	"jobid" integer,
	"jobuuid" text,
	"partition" text,
	"qos" text,
	"account" text,
	"grp" text,
	"gid" integer,
	"usr" text,
	"uid" integer,
	"submit" text,
	"start" text,
	"end" text,
	"submit_ts" integer,
	"start_ts" integer,
	"end_ts" integer,
	"elapsed" text,
	"elapsed_raw" integer,
	"exitcode" text,
	"state" text,
	"nnodes" integer,
	"ncpus" integer,
	"mem" text,
	"ngpus" integer,
	"nodelist" text,
	"nodelist_exp" text,
	"jobname" text,
	"workdir" text,
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
	"avg_ic_traffic_in_gb" real,
	"avg_ic_traffic_out_gb" real,
	"comment" blob,
	"ignore" integer
);
INSERT INTO jobs VALUES(1,1479763,'9d8487af-d86e-8ab5-241d-e580e1f4acc2','part1','qos1','acc1','grp1',1001,'usr1',1001,'2022-02-21T14:37:02+0100','2022-02-21T14:37:07+0100','2022-02-21T15:26:29+0100',1645450622000,1645450627000,1645453589000,'00:49:22',3000,'0:0','CANCELLED by 1001',1,8,'320G',8,'compute-0','compute-0','test_script1','/home/usr1',0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(2,1481508,'3e821675-0fec-9519-635e-ff219cdaa6e5','part1','qos1','acc2','grp2',1002,'usr2',1002,'2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:08:17',4500,'0:0','CANCELLED by 1002',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr2',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(3,1481510,'ba910ca8-b946-a3f2-91ad-b710355eed53','part1','qos1','acc3','grp3',1003,'usr3',1003,'2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:00:17',789,'0:0','CANCELLED by 1003',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr3',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(4,147975,'6b2cd1a8-178d-e049-0834-151ca7262546','part1','qos1','acc3','grp3',1003,'usr3',1003,'2023-02-21T14:37:02+0100','2023-02-21T14:37:07+0100','2023-02-21T15:26:29+0100',1676986622000,1676986627000,1676989589000,'00:49:22',3000,'0:0','CANCELLED by 1003',1,8,'320G',8,'compute-0','compute-0','test_script1','/home/usr3',0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(5,14508,'49d85d7c-b35a-49fd-4d7c-e93057cf642a','part1','qos1','acc4','grp4',1004,'usr4',1004,'2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:08:17',4500,'0:0','CANCELLED by 1004',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr4',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(6,147973,'bf2eac2d-4971-2bfe-a7c2-77cdfd37ef2a','part1','qos1','acc2','gr1',1002,'usr1',1001,'2023-12-21T15:48:20+0100','2023-12-21T15:49:06+0100','2023-12-21T15:57:23+0100',1703170100000,1703170146000,1703170643000,'00:00:17',567,'0:0','CANCELLED by 1001',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr1',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(7,1479765,'620b1ee4-fbfe-7121-61f7-66e1f8b3af27','part1','qos1','acc1','grp8',1008,'usr8',1008,'2023-02-21T14:37:02+0100','2023-02-21T14:37:07+0100','2023-02-21T15:26:29+0100',1676986622000,1676986627000,1676989589000,'00:49:22',3000,'0:0','CANCELLED by 1008',1,8,'320G',8,'compute-0','compute-0','test_script1','/home/usr8',0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(8,11508,'91d4c95d-de3e-3e52-35fa-0df5c20a6399','part1','qos1','acc1','grp15',1015,'usr15',1015,'2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:08:17',4500,'0:0','CANCELLED by 1015',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr15',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(9,81510,'5a86aff2-c108-2b97-9d87-463b401f88c9','part1','qos1','acc1','grp15',1015,'usr15',1015,'2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:00:17',3533,'0:0','CANCELLED by 1015',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr23',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',0);
INSERT INTO jobs VALUES(10,1009248,'0f0ac288-dbd4-a9a3-df3a-ab14ef9d51d5','part1','qos1','testacc','grp15',1015,'testusr',1015,'2023-02-21T15:48:20+0100','2023-02-21T15:49:06+0100','2023-02-21T15:57:23+0100',1676990900000,1676990946000,1676991443000,'00:00:17',17,'0:0','CANCELLED by 1015',2,16,'320G',8,'compute-[0-2]','compute-0|compute-1|compute-2','test_script2','/home/usr23',0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'',1);
CREATE TABLE usage (
	"id" integer not null primary key,
	"account" text,
	"usr" text,
	"partition" text,
	"qos" text,
	"num_jobs" integer,
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
	"avg_ic_traffic_in_gb" real,
	"avg_ic_traffic_out_gb" real,
	"comment" blob
);
INSERT INTO usage VALUES(1,'acc1','usr1','part1','qos1',1,0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(2,'acc2','usr2','part1','qos1',1,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(3,'acc3','usr3','part1','qos1',2,0,240,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(4,'acc4','usr4','part1','qos1',1,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(5,'acc2','usr1','part1','qos1',1,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(6,'acc1','usr8','part1','qos1',1,0,80,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(7,'acc1','usr15','part1','qos1',2,0,320,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
INSERT INTO usage VALUES(8,'testacc','testusr','part1','qos1',1,0,160,0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,0.0,'');
CREATE INDEX i1 ON jobs (usr,account,start);
CREATE INDEX i2 ON jobs (usr,jobid);
CREATE UNIQUE INDEX i3 ON jobs (jobid,start);
CREATE UNIQUE INDEX i4 ON usage (account,usr,partition,qos);
COMMIT;`
	_, err = db.Exec(stmts)
	if err != nil {
		fmt.Printf("failed to insert mock data into DB")
	}
	return db
}

func TestJobsQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB(t.TempDir())

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT * FROM %s WHERE ignore = 0 AND usr in ('usr1')", base.JobsDBTableName))

	expectedJobs := []base.Job{
		{
			ID:                  6,
			Jobid:               147973,
			Jobuuid:             "bf2eac2d-4971-2bfe-a7c2-77cdfd37ef2a",
			Partition:           "part1",
			QoS:                 "qos1",
			Account:             "acc2",
			Grp:                 "gr1",
			Gid:                 1002,
			Usr:                 "usr1",
			Uid:                 1001,
			Submit:              "2023-12-21T15:48:20+0100",
			Start:               "2023-12-21T15:49:06+0100",
			End:                 "2023-12-21T15:57:23+0100",
			SubmitTS:            1703170100000,
			StartTS:             1703170146000,
			EndTS:               1703170643000,
			Elapsed:             "00:00:17",
			ElapsedRaw:          567,
			Exitcode:            "0:0",
			State:               "CANCELLED by 1001",
			Nnodes:              2,
			Ncpus:               16,
			Mem:                 "320G",
			Ngpus:               8,
			Nodelist:            "compute-[0-2]",
			NodelistExp:         "compute-0|compute-1|compute-2",
			JobName:             "test_script2",
			WorkDir:             "/home/usr1",
			TotalCPUBilling:     0,
			TotalGPUBilling:     160,
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
			AvgICTrafficIn:      0,
			AvgICTrafficOut:     0,
			Comment:             "",
			Ignore:              0,
		},
		{
			ID:                  1,
			Jobid:               1479763,
			Jobuuid:             "9d8487af-d86e-8ab5-241d-e580e1f4acc2",
			Partition:           "part1",
			QoS:                 "qos1",
			Account:             "acc1",
			Grp:                 "grp1",
			Gid:                 1001,
			Usr:                 "usr1",
			Uid:                 1001,
			Submit:              "2022-02-21T14:37:02+0100",
			Start:               "2022-02-21T14:37:07+0100",
			End:                 "2022-02-21T15:26:29+0100",
			SubmitTS:            1645450622000,
			StartTS:             1645450627000,
			EndTS:               1645453589000,
			Elapsed:             "00:49:22",
			ElapsedRaw:          3000,
			Exitcode:            "0:0",
			State:               "CANCELLED by 1001",
			Nnodes:              1,
			Ncpus:               8,
			Mem:                 "320G",
			Ngpus:               8,
			Nodelist:            "compute-0",
			NodelistExp:         "compute-0",
			JobName:             "test_script1",
			WorkDir:             "/home/usr1",
			TotalCPUBilling:     0,
			TotalGPUBilling:     80,
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
			AvgICTrafficIn:      0,
			AvgICTrafficOut:     0,
			Comment:             "",
			Ignore:              0,
		},
	}
	jobs, err := querier(db, q, base.JobsEndpoint, logger)
	if err != nil {
		t.Errorf("failed to query for jobs: %s", err)
	}
	if !reflect.DeepEqual(expectedJobs, jobs.([]base.Job)) {
		t.Errorf("expected jobs %#v \n, got %#v", expectedJobs, jobs)
	}
}

func TestUsageQuerier(t *testing.T) {
	logger := log.NewNopLogger()
	db := setupTestDB(t.TempDir())

	// Query
	q := Query{}
	q.query(fmt.Sprintf("SELECT * FROM %s WHERE usr IN ('usr15')", base.UsageDBTableName))

	expectedUsageStats := []base.Usage{
		{
			ID:                  7,
			Account:             "acc1",
			Usr:                 "usr15",
			Partition:           "part1",
			QoS:                 "qos1",
			NumJobs:             2,
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
			AvgICTrafficIn:      0,
			AvgICTrafficOut:     0,
			Comment:             "",
		},
	}
	usageStats, err := querier(db, q, base.UsageResourceName, logger)
	if err != nil {
		t.Errorf("failed to query for usage: %s", err)
	}
	if !reflect.DeepEqual(expectedUsageStats, usageStats.([]base.Usage)) {
		t.Errorf("expected usage %#v \n, got %#v", expectedUsageStats, usageStats)
	}
}

func TestQueryBuilder(t *testing.T) {
	expectedQueryString := "SELECT * FROM table WHERE a IN (?,?) AND b IN (?,?) AND c BETWEEN (?) AND (?)"
	expectedQueryParams := []string{"a1", "a2", "10", "20", "2023-01-01", "2023-02-01"}

	// Start query
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

	// Start query
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
