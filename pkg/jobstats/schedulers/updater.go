package schedulers

import (
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/base"
)

// Job updater interface
type Updater interface {
	Update(queryTime time.Time, jobs []base.Job) []base.Job
}

// JobUpdater implements the interface to update
// batch jobs from different updaters.
type JobUpdater struct {
	Names    []string
	Updaters []Updater
	Logger   log.Logger
}

// Slice of updaters
var (
	updaterFactoryNames []string
	updaterFactories    = make(map[string]func(logger log.Logger) (Updater, error))
	updaterState        = make(map[string]*bool)
)

// Register updater
func RegisterUpdater(name string, isDefaultEnabled bool, factory func(logger log.Logger) (Updater, error)) {
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}

	flagName := fmt.Sprintf("job.updater.%s", name)
	flagHelp := fmt.Sprintf("Update jobs from %s (default: %s).", name, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := base.BatchJobStatsServerApp.Flag(flagName, flagHelp).
		Default(defaultValue).
		Bool()
	updaterState[name] = flag
	updaterFactories[name] = factory
	updaterFactoryNames = append(updaterFactoryNames, name)
}

// NewJobUpdater creates a new JobUpdater
func NewJobUpdater(logger log.Logger) (*JobUpdater, error) {
	var updater Updater
	var updaters []Updater
	var err error
	// Loop over factories and create new instances
	for key, factory := range updaterFactories {
		if *updaterState[key] {
			updater, err = factory(log.With(logger, "updater", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup job updater", "name", key, "err", err)
				return nil, err
			}
			updaters = append(updaters, updater)
		}
	}
	return &JobUpdater{
		Names:    updaterFactoryNames,
		Updaters: updaters,
		Logger:   logger,
	}, nil
}

// Update implements updating jobs using registered updaters
func (jmu JobUpdater) Update(queryTime time.Time, jobs []base.Job) []base.Job {
	// If there are no registered updaters, return
	if len(jmu.Updaters) == 0 {
		return jobs
	}

	// Iterate through all updaters in reverse
	for i := len(jmu.Updaters) - 1; i >= 0; i-- {
		jobs = jmu.Updaters[i].Update(queryTime, jobs)
		level.Info(jmu.Logger).Log("msg", "Updater", "name", jmu.Names[i])
	}
	return jobs
}
