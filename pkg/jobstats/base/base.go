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

// Grafana teams API response
type GrafanaTeamsReponse struct {
	OrgID      int      `json:"orgId"`
	TeamID     int      `json:"teamId"`
	TeamUID    string   `json:"teamUID"`
	UserID     int      `json:"userId"`
	AuthModule string   `json:"auth_module"`
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	Login      string   `json:"login"`
	AvatarURL  string   `json:"avatarUrl"`
	Labels     []string `json:"labels"`
	Permission int      `json:"permission"`
}

// Models
// Batch job struct
type JobStats struct {
	Jobid               int64   `json:"jobid" sqlitetype:"integer"`
	Jobuuid             string  `json:"jobuuid" sqlitetype:"text"`
	Partition           string  `json:"partition" sqlitetype:"text"`
	QoS                 string  `json:"qos" sqlitetype:"text"`
	Account             string  `json:"account" sqlitetype:"text"`
	Grp                 string  `json:"grp" sqlitetype:"text"`
	Gid                 int64   `json:"gid" sqlitetype:"integer"`
	Usr                 string  `json:"usr" sqlitetype:"text"`
	Uid                 int64   `json:"uid" sqlitetype:"integer"`
	Submit              string  `json:"submit" sqlitetype:"text"`
	Start               string  `json:"start" sqlitetype:"text"`
	End                 string  `json:"end" sqlitetype:"text"`
	SubmitTS            int64   `json:"submit_ts" sqlitetype:"integer"`
	StartTS             int64   `json:"start_ts" sqlitetype:"integer"`
	EndTS               int64   `json:"end_ts" sqlitetype:"integer"`
	Elapsed             string  `json:"elapsed" sqlitetype:"text"`
	ElapsedRaw          int64   `json:"elapsed_raw" sqlitetype:"integer"`
	Exitcode            string  `json:"exitcode" sqlitetype:"text"`
	State               string  `json:"state" sqlitetype:"text"`
	Nnodes              int     `json:"nnodes" sqlitetype:"integer"`
	Ncpus               int     `json:"ncpus" sqlitetype:"integer"`
	Mem                 string  `json:"mem" sqlitetype:"text"`
	Ngpus               int     `json:"ngpus" sqlitetype:"integer"`
	Nodelist            string  `json:"nodelist" sqlitetype:"text"`
	NodelistExp         string  `json:"nodelist_exp" sqlitetype:"text"`
	JobName             string  `json:"jobname" sqlitetype:"text"`
	WorkDir             string  `json:"workdir" sqlitetype:"text"`
	CPUBilling          int64   `json:"cpu_billing" sqlitetype:"integer"`
	GPUBilling          int64   `json:"gpu_billing" sqlitetype:"integer"`
	MiscBilling         int64   `json:"misc_billing" sqlitetype:"integer"`
	AveCPUUsage         float64 `json:"avg_cpu_usage" sqlitetype:"real"`
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage" sqlitetype:"real"`
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage" sqlitetype:"real"`
	TotalCPUEmissions   float64 `json:"total_cpu_emissions" sqlitetype:"real"`
	AveGPUUsage         float64 `json:"avg_gpu_usage" sqlitetype:"real"`
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage" sqlitetype:"real"`
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage" sqlitetype:"real"`
	TotalGPUEmissions   float64 `json:"total_gpu_emissions" sqlitetype:"real"`
	Comment             string  `json:"comment" sqlitetype:"blob"`
	Ignore              int     `json:"-" sqlitetype:"integer"`
}

// Account stats struct
type AccountStats struct {
	Name                string  `json:"name" sqlitetype:"text unique"`
	NumJobs             int64   `json:"num_jobs" sqlitetype:"integer"`
	CPUBilling          int64   `json:"cpu_billing" sqlitetype:"integer"`
	GPUBilling          int64   `json:"gpu_billing" sqlitetype:"integer"`
	MiscBilling         int64   `json:"misc_billing" sqlitetype:"integer"`
	AveCPUUsage         float64 `json:"avg_cpu_usage" sqlitetype:"real"`
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage" sqlitetype:"real"`
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage" sqlitetype:"real"`
	TotalCPUEmissions   float64 `json:"total_cpu_emissions" sqlitetype:"real"`
	AveGPUUsage         float64 `json:"avg_gpu_usage" sqlitetype:"real"`
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage" sqlitetype:"real"`
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage" sqlitetype:"real"`
	TotalGPUEmissions   float64 `json:"total_gpu_emissions" sqlitetype:"real"`
}

// User stats struct
type UserStats struct {
	Name                string  `json:"name" sqlitetype:"text unique"`
	NumJobs             int64   `json:"num_jobs" sqlitetype:"integer"`
	CPUBilling          int64   `json:"cpu_billing" sqlitetype:"integer"`
	GPUBilling          int64   `json:"gpu_billing" sqlitetype:"integer"`
	MiscBilling         int64   `json:"misc_billing" sqlitetype:"integer"`
	AveCPUUsage         float64 `json:"avg_cpu_usage" sqlitetype:"real"`
	AveCPUMemUsage      float64 `json:"avg_cpu_mem_usage" sqlitetype:"real"`
	TotalCPUEnergyUsage float64 `json:"total_cpu_energy_usage" sqlitetype:"real"`
	TotalCPUEmissions   float64 `json:"total_cpu_emissions" sqlitetype:"real"`
	AveGPUUsage         float64 `json:"avg_gpu_usage" sqlitetype:"real"`
	AveGPUMemUsage      float64 `json:"avg_gpu_mem_usage" sqlitetype:"real"`
	TotalGPUEnergyUsage float64 `json:"total_gpu_energy_usage" sqlitetype:"real"`
	TotalGPUEmissions   float64 `json:"total_gpu_emissions" sqlitetype:"real"`
}

// Account struct
type Account struct {
	ID string `json:"id"`
}

// Common API response model
type Response struct {
	Status    string   `json:"status"`
	ErrorType string   `json:"errorType,omitempty"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

// /api/accounts response struct
type AccountsResponse struct {
	Response
	Data []Account `json:"data,omitempty"`
}

// /api/jobs response struct
type JobsResponse struct {
	Response
	Data []JobStats `json:"data,omitempty"`
}

// DB table names
var (
	JobStatsDBTableName     = "jobstats"
	UserStatsDBTableName    = "userstats"
	AccountStatsDBTableName = "accountstats"
)

// Slice of all field names of JobStats struct
var JobStatsDBColNames = helper.GetStructFieldTagValues(JobStats{}, "json")
var AccountStatsDBColNames = helper.GetStructFieldTagValues(AccountStats{}, "json")
var UserStatsDBColNames = helper.GetStructFieldTagValues(UserStats{}, "json")

// Map of field names to DB column type
var JobStatsDBTableMap = helper.GetStructFieldTagMap(JobStats{}, "json", "sqlitetype")
var AccountStatsDBTableMap = helper.GetStructFieldTagMap(AccountStats{}, "json", "sqlitetype")
var UserStatsDBTableMap = helper.GetStructFieldTagMap(UserStats{}, "json", "sqlitetype")

// Layout of datetime to be used in the package
var DatetimeLayout = fmt.Sprintf("%sT%s", time.DateOnly, time.TimeOnly)
