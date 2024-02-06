package updater_one

import (
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/schedulers"
)

type mockUpdater struct {
	logger log.Logger
}

const mockUpdaterHookName = "mock-one"

var (
	slurmUserUid       int
	slurmUserGid       int
	mockUpdaterHookCLI = base.BatchJobStatsServerApp.Flag(
		"job.updater.mock-one.arg",
		"Mock updater CLI arg.",
	).Default("").String()
)

// Register mock updater
func init() {
	schedulers.RegisterUpdater(mockUpdaterHookName, false, NewMockUpdaterHook)
}

// NewMockUpdaterHook returns a new NewMockUpdaterHook to update jobs
func NewMockUpdaterHook(logger log.Logger) (schedulers.Updater, error) {
	return &mockUpdater{
		logger: logger,
	}, nil
}

// Add the logic here to update the jobs retrieved from batch scheduler
func (u *mockUpdater) Update(queryTime time.Time, jobs []base.Job) []base.Job {
	return jobs
}
