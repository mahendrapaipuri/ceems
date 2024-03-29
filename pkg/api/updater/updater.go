// Package updater will provide an interface to update the unit stucts before
// inserting into DB
//
// Users can implement their own logic to mutate units struct to manipulate each
// unit struct
package updater

import (
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// Updater interface
type Updater interface {
	Update(startTime time.Time, endTime time.Time, units []models.Unit) []models.Unit
}

// UnitUpdater implements the interface to update
// compute units from different updaters.
type UnitUpdater struct {
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

// RegisterUpdater registers updater struct into factories
func RegisterUpdater(name string, isDefaultEnabled bool, factory func(logger log.Logger) (Updater, error)) {
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}

	flagName := fmt.Sprintf("updater.%s", name)
	flagHelp := fmt.Sprintf("Update compute units from %s (default: %s).", name, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := base.CEEMSServerApp.Flag(flagName, flagHelp).
		Default(defaultValue).
		Bool()
	updaterState[name] = flag
	updaterFactories[name] = factory
	updaterFactoryNames = append(updaterFactoryNames, name)
}

// NewUnitUpdater creates a new UnitUpdater
func NewUnitUpdater(logger log.Logger) (*UnitUpdater, error) {
	var updater Updater
	var updaters []Updater
	var err error
	// Loop over factories and create new instances
	for key, factory := range updaterFactories {
		if *updaterState[key] {
			updater, err = factory(log.With(logger, "updater", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup unit updater", "name", key, "err", err)
				return nil, err
			}
			updaters = append(updaters, updater)
		}
	}
	return &UnitUpdater{
		Names:    updaterFactoryNames,
		Updaters: updaters,
		Logger:   logger,
	}, nil
}

// Update implements updating units using registered updaters
func (u UnitUpdater) Update(startTime time.Time, endTime time.Time, units []models.Unit) []models.Unit {
	// If there are no registered updaters, return
	if len(u.Updaters) == 0 {
		return units
	}

	// Iterate through all updaters in reverse
	for i := len(u.Updaters) - 1; i >= 0; i-- {
		units = u.Updaters[i].Update(startTime, endTime, units)
		level.Info(u.Logger).Log("msg", "Updater", "name", u.Names[i])
	}
	return units
}
