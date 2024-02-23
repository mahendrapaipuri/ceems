// Package resource defines the interface that each resource manager needs to
// implement to get compute units
package resource

import (
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
	Fetcher
	logger log.Logger
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

	flag := base.CEEMSServerApp.Flag(flagName, flagHelp).
		Default(defaultValue).
		Bool()
	managerState[manager] = flag
	factories[manager] = factory
}

// NewManager creates a new Manager struct instance
func NewManager(logger log.Logger) (*Manager, error) {
	var fetcher Fetcher
	var err error
	var factoryKeys []string

	// Loop over factories and create new instances
	for key, factory := range factories {
		factoryKeys = append(factoryKeys, key)
		if *managerState[key] {
			fetcher, err = factory(log.With(logger, "manager", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup resource manager", "name", key, "err", err)
				return nil, err
			}
			return &Manager{Fetcher: fetcher, logger: logger}, nil
		}
	}
	return nil, fmt.Errorf(
		"no resource manager enabled. Please choose one of [%s] using flag --manager.manager.<name>",
		strings.Join(factoryKeys, ", "),
	)
}

// Fetch implements collection jobs between start and end times
func (b Manager) Fetch(start time.Time, end time.Time) ([]models.Unit, error) {
	return b.Fetcher.Fetch(start, end)
}
