package scheduler

import (
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/schedulers"
)

type mockScheduler struct {
	logger log.Logger
}

const mockBatchScheduler = "mock"

var (
	slurmUserUid int
	slurmUserGid int
	macctPath    = base.BatchJobStatsServerApp.Flag(
		"mock.acct.path",
		"Absolute path to mock scheduler's accounting executable.",
	).Default("/usr/local/bin/macct").String()
)

func init() {
	// Register batch scheduler with jobstats pkg
	schedulers.RegisterBatch(mockBatchScheduler, NewMockScheduler)
}

// Do all basic checks here
func preflightChecks(logger log.Logger) error {
	return nil
}

// NewMockScheduler returns a new MockScheduler that returns batch job stats
func NewMockScheduler(logger log.Logger) (schedulers.BatchJobFetcher, error) {
	err := preflightChecks(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create mock batch scheduler for retreiving jobs.", "err", err)
		return nil, err
	}
	level.Info(logger).Log("msg", "Jobs from mock batch scheduler will be retrieved.")
	return &mockScheduler{
		logger: logger,
	}, nil
}

// Add the logic here to get jobs from batch scheduler and return slice of BatchJob structs
// When making BatchJob stucts, ensure to format the datetime using base.DatetimeLayout
// Also ensure to set StartTS and EndTS fields to start and end times in unix milliseconds epoch
func (s *mockScheduler) Fetch(start time.Time, end time.Time) ([]base.JobStats, error) {
	return []base.JobStats{{Jobid: 1000}, {Jobid: 1100}}, nil
}
