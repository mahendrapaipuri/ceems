package scheduler

import (
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"
)

type mockScheduler struct {
	logger log.Logger
}

const mockBatchScheduler = "mock"

var (
	slurmUserUid int
	slurmUserGid int
	macctPath    = jobstats.BatchJobStatsServerApp.Flag(
		"mock.acct.path",
		"Absolute path to macct executable.",
	).Default("/usr/local/bin/macct").String()
)

func init() {
	// Register batch scheduler with jobstats pkg
	jobstats.RegisterBatch(mockBatchScheduler, true, NewMockScheduler)
}

// Do all basic checks here
func preflightChecks(logger log.Logger) error {
	return nil
}

// NewMockScheduler returns a new MockScheduler that returns batch job stats
func NewMockScheduler(logger log.Logger) (jobstats.Batch, error) {
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
func (s *mockScheduler) GetJobs(start time.Time, end time.Time) ([]jobstats.BatchJob, error) {
	return []jobstats.BatchJob{{Jobid: "1000"}, {Jobid: "1100"}}, nil
}
