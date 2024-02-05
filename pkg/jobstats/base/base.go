package base

import (
	"fmt"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/helper"
)

// Name of batchjob_stats_server kingpin app
const BatchJobStatsServerAppName = "batchjob_stats_server"

// `batchjob_stats_server` CLI app
var BatchJobStatsServerApp = *kingpin.New(
	BatchJobStatsServerAppName,
	"API server for batch job statistics of users.",
)

// Models
// Batch job struct
type Job struct {
	ID                  int64   `json:"-"                          sql:"id"                         sqlitetype:"integer not null primary key"`
	Jobid               int64   `json:"jobid"                      sql:"jobid"                      sqlitetype:"integer"`
	Jobuuid             string  `json:"jobuuid"                    sql:"jobuuid"                    sqlitetype:"text"`
	Partition           string  `json:"partition"                  sql:"partition"                  sqlitetype:"text"`
	QoS                 string  `json:"qos"                        sql:"qos"                        sqlitetype:"text"`
	Account             string  `json:"account"                    sql:"account"                    sqlitetype:"text"`
	Grp                 string  `json:"grp"                        sql:"grp"                        sqlitetype:"text"`
	Gid                 int64   `json:"gid"                        sql:"gid"                        sqlitetype:"integer"`
	Usr                 string  `json:"usr"                        sql:"usr"                        sqlitetype:"text"`
	Uid                 int64   `json:"uid"                        sql:"uid"                        sqlitetype:"integer"`
	Submit              string  `json:"submit"                     sql:"submit"                     sqlitetype:"text"`
	Start               string  `json:"start"                      sql:"start"                      sqlitetype:"text"`
	End                 string  `json:"end"                        sql:"end"                        sqlitetype:"text"`
	SubmitTS            int64   `json:"submit_ts"                  sql:"submit_ts"                  sqlitetype:"integer"`
	StartTS             int64   `json:"start_ts"                   sql:"start_ts"                   sqlitetype:"integer"`
	EndTS               int64   `json:"end_ts"                     sql:"end_ts"                     sqlitetype:"integer"`
	Elapsed             string  `json:"elapsed"                    sql:"elapsed"                    sqlitetype:"text"`
	ElapsedRaw          int64   `json:"elapsed_raw"                sql:"elapsed_raw"                sqlitetype:"integer"`
	Exitcode            string  `json:"exitcode"                   sql:"exitcode"                   sqlitetype:"text"`
	State               string  `json:"state"                      sql:"state"                      sqlitetype:"text"`
	Nnodes              int     `json:"nnodes"                     sql:"nnodes"                     sqlitetype:"integer"`
	Ncpus               int     `json:"ncpus"                      sql:"ncpus"                      sqlitetype:"integer"`
	Mem                 string  `json:"mem"                        sql:"mem"                        sqlitetype:"text"`
	Ngpus               int     `json:"ngpus"                      sql:"ngpus"                      sqlitetype:"integer"`
	Nodelist            string  `json:"nodelist"                   sql:"nodelist"                   sqlitetype:"text"`
	NodelistExp         string  `json:"nodelist_exp"               sql:"nodelist_exp"               sqlitetype:"text"`
	JobName             string  `json:"jobname"                    sql:"jobname"                    sqlitetype:"text"`
	WorkDir             string  `json:"workdir"                    sql:"workdir"                    sqlitetype:"text"`
	TotalCPUBilling     int64   `json:"total_cpu_billing"          sql:"total_cpu_billing"          sqlitetype:"integer"`
	TotalGPUBilling     int64   `json:"total_gpu_billing"          sql:"total_gpu_billing"          sqlitetype:"integer"`
	TotalMiscBilling    int64   `json:"total_misc_billing"         sql:"total_misc_billing"         sqlitetype:"integer"`
	AveCPUUsage         float64 `json:"avg_cpu_usage"              sql:"avg_cpu_usage"              sqlitetype:"real"`
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage_kwh" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`
	TotalCPUEmissions   float64 `json:"total_cpu_emissions_gms"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`
	AveGPUUsage         float64 `json:"avg_gpu_usage"              sql:"avg_gpu_usage"              sqlitetype:"real"`
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage_kwh" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`
	TotalGPUEmissions   float64 `json:"total_gpu_emissions_gms"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`
	TotalIOWriteHot     float64 `json:"total_io_write_hot_gb"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`
	TotalIOReadHot      float64 `json:"total_io_read_hot_gb"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`
	TotalIOWriteCold    float64 `json:"total_io_write_cold_gb"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`
	TotalIOReadCold     float64 `json:"total_io_read_cold_gb"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`
	AvgICTrafficIn      float64 `json:"avg_ic_traffic_in_gb"       sql:"avg_ic_traffic_in_gb"       sqlitetype:"real"`
	AvgICTrafficOut     float64 `json:"avg_ic_traffic_out_gb"      sql:"avg_ic_traffic_out_gb"      sqlitetype:"real"`
	Comment             string  `json:"comment"                    sql:"comment"                    sqlitetype:"blob"`
	Ignore              int     `json:"-"                          sql:"ignore"                     sqlitetype:"integer"`
}

// Usage struct
type Usage struct {
	ID                  int64   `json:"-"                          sql:"id"                         sqlitetype:"integer not null primary key"`
	Account             string  `json:"account"                    sql:"account"                    sqlitetype:"text"`
	Usr                 string  `json:"usr"                        sql:"usr"                        sqlitetype:"text"`
	Partition           string  `json:"partition"                  sql:"partition"                  sqlitetype:"text"`
	QoS                 string  `json:"qos"                        sql:"qos"                        sqlitetype:"text"`
	NumJobs             int64   `json:"num_jobs"                   sql:"num_jobs"                   sqlitetype:"integer"`
	TotalCPUBilling     int64   `json:"total_cpu_billing"          sql:"total_cpu_billing"          sqlitetype:"integer"`
	TotalGPUBilling     int64   `json:"total_gpu_billing"          sql:"total_gpu_billing"          sqlitetype:"integer"`
	TotalMiscBilling    int64   `json:"total_misc_billing"         sql:"total_misc_billing"         sqlitetype:"integer"`
	AveCPUUsage         float64 `json:"avg_cpu_usage"              sql:"avg_cpu_usage"              sqlitetype:"real"`
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage"          sql:"avg_cpu_mem_usage"          sqlitetype:"real"`
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage_kwh" sql:"total_cpu_energy_usage_kwh" sqlitetype:"real"`
	TotalCPUEmissions   float64 `json:"total_cpu_emissions_gms"    sql:"total_cpu_emissions_gms"    sqlitetype:"real"`
	AveGPUUsage         float64 `json:"avg_gpu_usage"              sql:"avg_gpu_usage"              sqlitetype:"real"`
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage"          sql:"avg_gpu_mem_usage"          sqlitetype:"real"`
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage_kwh" sql:"total_gpu_energy_usage_kwh" sqlitetype:"real"`
	TotalGPUEmissions   float64 `json:"total_gpu_emissions_gms"    sql:"total_gpu_emissions_gms"    sqlitetype:"real"`
	TotalIOWriteHot     float64 `json:"total_io_write_hot_gb"      sql:"total_io_write_hot_gb"      sqlitetype:"real"`
	TotalIOReadHot      float64 `json:"total_io_read_hot_gb"       sql:"total_io_read_hot_gb"       sqlitetype:"real"`
	TotalIOWriteCold    float64 `json:"total_io_write_cold_gb"     sql:"total_io_write_cold_gb"     sqlitetype:"real"`
	TotalIOReadCold     float64 `json:"total_io_read_cold_gb"      sql:"total_io_read_cold_gb"      sqlitetype:"real"`
	AvgICTrafficIn      float64 `json:"avg_ic_traffic_in_gb"       sql:"avg_ic_traffic_in_gb"       sqlitetype:"real"`
	AvgICTrafficOut     float64 `json:"avg_ic_traffic_out_gb"      sql:"avg_ic_traffic_out_gb"      sqlitetype:"real"`
	Comment             string  `json:"comment"                    sql:"comment"                    sqlitetype:"blob"`
}

// Account struct
type Account struct {
	Name string `json:"name,omitempty" sql:"account" sqlitetype:"text"`
}

// Resources names
var (
	JobsResourceName    = "jobs"
	UsageResourceName   = "usage"
	AccountResourceName = "accounts"
)

// DB table names
var (
	JobsDBTableName  = JobsResourceName
	UsageDBTableName = UsageResourceName
)

// Endpoints
var (
	JobsEndpoint     = JobsResourceName
	UsageEndpoint    = UsageResourceName
	AccountsEndpoint = AccountResourceName
)

// Slice of all field names of JobStats struct
var (
	JobsDBColNames  = helper.GetStructFieldTagValues(Job{}, "sql")
	UsageDBColNames = helper.GetStructFieldTagValues(Usage{}, "sql")
)

// Map of field names to DB column type
var (
	JobsDBTableMap  = helper.GetStructFieldTagMap(Job{}, "sql", "sqlitetype")
	UsageDBTableMap = helper.GetStructFieldTagMap(Usage{}, "sql", "sqlitetype")
)

// Layout of datetime to be used in the package
var DatetimeLayout = fmt.Sprintf("%sT%s", time.DateOnly, time.TimeOnly)
