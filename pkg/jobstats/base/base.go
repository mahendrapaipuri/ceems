package base

import "github.com/alecthomas/kingpin/v2"

// Name of batchjob_stats_server kingpin app
const BatchJobStatsServerAppName = "batchjob_stats_server"

// `batchjob_stats_server` CLI app
var BatchJobStatsServerApp = *kingpin.New(
	BatchJobStatsServerAppName,
	"API server data source for batch job statistics of users.",
)

// Models
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

// Account struct
type Account struct {
	ID string `json:"id"`
}

// Common API response model
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