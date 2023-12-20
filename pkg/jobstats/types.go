package jobstats

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/exporter-toolkit/web"
)

type jobStatsDB struct {
	logger                 log.Logger
	db                     *sql.DB
	batchScheduler         string
	jobstatDBPath          string
	jobstatDBTable         string
	retentionPeriod        int
	lastJobsUpdateTime     time.Time
	lastDBVacuumTime       time.Time
	lastJobsUpdateTimeFile string
}

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

type Config struct {
	Logger           log.Logger
	Address          string
	WebSystemdSocket bool
	WebConfigFile    string
	JobstatDBFile    string
	JobstatDBTable   string
}

type Account struct {
	ID string `json:"id"`
}

type Response struct {
	Status    string    `json:"status"`
	Data      []Account `json:"data"`
	ErrorType string    `json:"errorType"`
	Error     string    `json:"error"`
	Warnings  []string  `json:"warnings"`
}

type AccountsResponse struct {
	Response
	Data []Account `json:"data"`
}

type JobsResponse struct {
	Response
	Data []BatchJob `json:"data"`
}

type JobstatsServer struct {
	logger         log.Logger
	server         *http.Server
	webConfig      *web.FlagConfig
	AccountsGetter func(string, log.Logger) ([]Account, error)
	JobsGetter     func(string, []string, string, string, log.Logger) ([]BatchJob, error)
	HealthChecker  func(log.Logger) bool
}
