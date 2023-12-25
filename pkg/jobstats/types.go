package jobstats

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/exporter-toolkit/web"
)

// BatchJobStatsServer represents the `batchjob_stats_server` cli.
type BatchJobStatsServer struct {
	logger        log.Logger
	promlogConfig promlog.Config
	appName       string
	App           kingpin.Application
}

// Batch is the interface batch scheduler has to implement.
type Batch interface {
	// Get BatchJobs between start and end times
	GetJobs(start time.Time, end time.Time) ([]BatchJob, error)
}

// BatchScheduler implements the interface to collect
// batch jobs from different batch schedulers.
type BatchScheduler struct {
	Scheduler Batch
	logger    log.Logger
}

// job stats DB struct
type jobStatsDB struct {
	logger                 log.Logger
	db                     *sql.DB
	scheduler              *BatchScheduler
	jobstatDBPath          string
	jobstatDBTable         string
	retentionPeriod        int
	lastJobsUpdateTime     time.Time
	lastDBVacuumTime       time.Time
	lastJobsUpdateTimeFile string
}

// Batch job struct
type BatchJob struct {
	Jobid       string `json:"jobid"`
	Jobuuid     string `json:"id"`
	Partition   string `json:"partition"`
	Account     string `json:"account"`
	Grp         string `json:"group"`
	Gid         string `json:"gid"`
	Usr         string `json:"user"`
	Uid         string `json:"uid"`
	Submit      string `json:"submit"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Elapsed     string `json:"elapsed"`
	Exitcode    string `json:"exitcode"`
	State       string `json:"state"`
	Nnodes      string `json:"nnodes"`
	Nodelist    string `json:"nodelist"`
	NodelistExp string `json:"nodelistexp"`
}

// Server config
type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	JobstatDBFile    string
	JobstatDBTable   string
}

// API response struct for account
type Account struct {
	ID string `json:"id"`
}

// Common API response struct
type Response struct {
	Status    string    `json:"status"`
	Data      []Account `json:"data"`
	ErrorType string    `json:"errorType"`
	Error     string    `json:"error"`
	Warnings  []string  `json:"warnings"`
}

// /api/account response struct
type AccountsResponse struct {
	Response
	Data []Account `json:"data"`
}

// /api/jobs response struct
type JobsResponse struct {
	Response
	Data []BatchJob `json:"data"`
}

// Job Stats Server config
type JobstatsServer struct {
	logger         log.Logger
	server         *http.Server
	webConfig      *web.FlagConfig
	AccountsGetter func(string, log.Logger) ([]Account, error)
	JobsGetter     func(string, []string, string, string, log.Logger) ([]BatchJob, error)
	HealthChecker  func(log.Logger) bool
}
