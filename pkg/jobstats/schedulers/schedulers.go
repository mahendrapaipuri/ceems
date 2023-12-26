package schedulers

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats/base"
)

// Fetcher is the interface batch scheduler has to implement.
type Fetcher interface {
	// Fetch BatchJobs between start and end times
	Fetch(start time.Time, end time.Time) ([]base.BatchJob, error)
}

// BatchScheduler implements the interface to collect
// batch jobs from different batch schedulers.
type BatchScheduler struct {
	Scheduler Fetcher
	logger    log.Logger
}

var (
	factories      = make(map[string]func(logger log.Logger) (Fetcher, error))
	schedulerState = make(map[string]*bool)
)

// Register batch scheduler
func RegisterBatch(
	scheduler string,
	isDefaultEnabled bool,
	factory func(logger log.Logger) (Fetcher, error),
) {
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}

	flagName := fmt.Sprintf("batch.scheduler.%s", scheduler)
	flagHelp := fmt.Sprintf("Fetch jobs from %s scheduler (default: %s).", scheduler, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := base.BatchJobStatsServerApp.Flag(flagName, flagHelp).
		Default(defaultValue).
		Bool()
	schedulerState[scheduler] = flag
	factories[scheduler] = factory
}

// NewBatchSchedulers creates a new BatchSchedulers
func NewBatchScheduler(logger log.Logger) (*BatchScheduler, error) {
	var scheduler Fetcher
	var err error
	var factoryKeys []string

	// Loop over factories and create new instances
	for key, factory := range factories {
		factoryKeys = append(factoryKeys, key)
		if *schedulerState[key] {
			scheduler, err = factory(log.With(logger, "batch", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup batch scheduler", "name", key, "err", err)
				return nil, err
			}
			return &BatchScheduler{Scheduler: scheduler, logger: logger}, nil
		}
	}
	return nil, fmt.Errorf("No batch scheduler enabled. Please choose one of [%s] using flag --batch.scheduler.<name>", strings.Join(factoryKeys, ", "))
}

// Fetch implements collection jobs between start and end times
func (b BatchScheduler) Fetch(start time.Time, end time.Time) ([]base.BatchJob, error) {
	return b.Scheduler.Fetch(start, end)
}
