package jobstats

import "github.com/go-kit/log"

type jobStats struct {
	logger                  log.Logger
	batchScheduler          string
	jobstatDBPath           string
	jobstatDBTable          string
	retentionPeriod         int
	jobsLastTimeStampFile   string
	vacuumLastTimeStampFile string
}

type BatchJob struct {
	Jobid       string
	Jobuuid     string
	Partition   string
	Account     string
	Grp         string
	Gid         string
	Usr         string
	Uid         string
	Submit      string
	Start       string
	End         string
	Elapsed     string
	Exitcode    string
	State       string
	Nnodes      string
	Nodelist    string
	NodelistExp string
}
