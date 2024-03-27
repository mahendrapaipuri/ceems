// Package resource defines the interface that each resource manager needs to
// implement to get compute units
package resource

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// Fetcher is the interface resource manager has to implement.
type Fetcher interface {
	// Fetch compute units between start and end times
	Fetch(start time.Time, end time.Time) ([]models.Unit, error)
}

// Manager implements the interface to collect
// manager jobs from different resource managers.
type Manager struct {
	Fetchers []Fetcher
	logger   log.Logger
}

var (
	factories    = make(map[string]func(logger log.Logger) (Fetcher, error))
	managerState = make(map[string]*bool)
)

// RegisterManager registers the resource manager into factory
func RegisterManager(
	manager string,
	factory func(logger log.Logger) (Fetcher, error),
) {
	var isDefaultEnabled = false
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}

	flagName := fmt.Sprintf("resource.manager.%s", manager)
	flagHelp := fmt.Sprintf("Fetch compute units from %s (default: %s).", manager, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	// Hide default manager from CLI
	var flag *bool
	if manager == "default" {
		flag = base.CEEMSServerApp.Flag(flagName, flagHelp).Hidden().Default(defaultValue).Bool()
	} else {
		flag = base.CEEMSServerApp.Flag(flagName, flagHelp).Default(defaultValue).Bool()
	}
	managerState[manager] = flag
	factories[manager] = factory
}

// NewManager creates a new Manager struct instance
func NewManager(logger log.Logger) (*Manager, error) {
	var fetcher Fetcher
	var err error
	var factoryKeys []string

	// Loop over factories and create new instances
	var fetchers []Fetcher
	for key, factory := range factories {
		if key != "default" {
			factoryKeys = append(factoryKeys, key)
		}
		if *managerState[key] {
			fetcher, err = factory(log.With(logger, "manager", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup resource manager", "name", key, "err", err)
				return nil, err
			}
			fetchers = append(fetchers, fetcher)
		}
	}
	level.Warn(logger).Log(
		"msg", "No resource manager enabled. Using a default resource manager",
		"available_resource_managers", strings.Join(factoryKeys, ","),
	)

	// Return an instance of default manager
	if len(fetchers) == 0 {
		fetcher, err = factories["default"](log.With(logger, "manager", "default"))
		if err != nil {
			level.Error(logger).Log("msg", "Failed to setup default resource manager", "err", err)
			return nil, err
		}
		fetchers = append(fetchers, fetcher)
	}
	return &Manager{Fetchers: fetchers, logger: logger}, nil
}

// Fetch implements collection jobs between start and end times
func (b Manager) Fetch(start time.Time, end time.Time) ([]models.Unit, error) {
	var allUnits []models.Unit
	var errs error
	for _, fetcher := range b.Fetchers {
		units, err := fetcher.Fetch(start, end)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		allUnits = append(allUnits, units...)
	}
	return allUnits, errs
}
