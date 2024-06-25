// Package updater will provide an interface to update the unit stucts before
// inserting into DB
//
// Users can implement their own logic to mutate units struct to manipulate each
// unit struct
package updater

import (
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"gopkg.in/yaml.v3"
)

// Instance contains the configuration of the given updater
type Instance struct {
	ID      string           `yaml:"id"`
	Updater string           `yaml:"updater"`
	Web     models.WebConfig `yaml:"web"`
	CLI     models.CLIConfig `yaml:"cli"`
	Extra   yaml.Node        `yaml:"extra_config"`
}

// Config contains the configuration of updater(s)
type Config[T any] struct {
	Instances []T `yaml:"updaters"`
}

// Updater interface
type Updater interface {
	Update(startTime time.Time, endTime time.Time, units []models.ClusterUnits) []models.ClusterUnits
}

// UnitUpdater implements the interface to update compute units from different updaters.
type UnitUpdater struct {
	Updaters map[string]Updater
	Logger   log.Logger
}

// Slice of updaters
var (
	updaterFactories = make(map[string]func(instance Instance, logger log.Logger) (Updater, error))
)

// RegisterUpdater registers updater struct into factories
func RegisterUpdater(
	name string,
	factory func(instance Instance, logger log.Logger) (Updater, error),
) {
	updaterFactories[name] = factory
}

// checkConfig verifies for the errors in updater config and returns a map
// of updater to its configs
func checkConfig(updaters []string, config *Config[Instance]) (map[string][]Instance, error) {
	// Check if IDs are unique and updater is registered
	var IDs []string
	var configMap = make(map[string][]Instance)
	for i := 0; i < len(config.Instances); i++ {
		if slices.Contains(IDs, config.Instances[i].ID) {
			return nil, fmt.Errorf("duplicate ID found in updaters config")
		}
		if !slices.Contains(updaters, config.Instances[i].Updater) {
			return nil, fmt.Errorf("unknown updater found in the config: %s", config.Instances[i].Updater)
		}
		if base.InvalidIDRegex.MatchString(config.Instances[i].ID) {
			return nil, fmt.Errorf("invalid ID %s found in updaters config. It must contain only [a-zA-Z0-9-_]", config.Instances[i].ID)
		}
		IDs = append(IDs, config.Instances[i].ID)
		configMap[config.Instances[i].Updater] = append(configMap[config.Instances[i].Updater], config.Instances[i])
	}
	return configMap, nil
}

// updaterConfig returns the configuration of updaters
func updaterConfig() (*Config[Instance], error) {
	// Merge default config with provided config
	config, err := common.MakeConfig[Config[Instance]](base.ConfigFilePath)
	if err != nil {
		return nil, err
	}

	// Set directories
	for i := 0; i < len(config.Instances); i++ {
		config.Instances[i].Web.HTTPClientConfig.SetDirectory(filepath.Dir(base.ConfigFilePath))
	}
	return config, nil
}

// NewUnitUpdater creates a new UnitUpdater
func NewUnitUpdater(logger log.Logger) (*UnitUpdater, error) {
	var updater Updater
	var updaters = make(map[string]Updater)
	var registeredUpdaters []string
	var err error

	// Get all registered updaters
	for updaterName := range updaterFactories {
		registeredUpdaters = append(registeredUpdaters, updaterName)
	}

	// Get current config
	config, err := updaterConfig()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to parse updater config", "err", err)
		return nil, err
	}

	// Preflight checks on config
	configMap, err := checkConfig(registeredUpdaters, config)
	if err != nil {
		level.Error(logger).Log("msg", "Invalid updater config", "err", err)
		return nil, err
	}

	// Loop over factories and create new instances
	for key, factory := range updaterFactories {
		for _, config := range configMap[key] {
			updater, err = factory(config, log.With(logger, "updater", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup unit updater", "name", key, "err", err)
				return nil, err
			}
			updaters[config.ID] = updater
		}
	}
	return &UnitUpdater{
		Updaters: updaters,
		Logger:   logger,
	}, nil
}

// Update implements updating units using registered updaters
func (u UnitUpdater) Update(
	startTime time.Time,
	endTime time.Time,
	clusterUnits []models.ClusterUnits,
) []models.ClusterUnits {
	// If there are no registered updaters, return
	if len(u.Updaters) == 0 {
		return clusterUnits
	}

	// Iterate through units and apply updater for each clusterUnit
	for i := 0; i < len(clusterUnits); i++ {
		if len(clusterUnits[i].Units) == 0 || len(clusterUnits[i].Cluster.Updaters) == 0 {
			continue
		}

		for j := 0; j < len(clusterUnits[i].Cluster.Updaters); j++ {
			updaterID := clusterUnits[i].Cluster.Updaters[j]

			// Check if updaterID is valid
			if updater, ok := u.Updaters[updaterID]; ok {
				// Only update Units slice and do not touch cluster meta data
				updatedClusterUnits := updater.Update(startTime, endTime, []models.ClusterUnits{clusterUnits[i]})
				// Just to ensure we wont have nil pointer dereferencing errors in runtime
				if len(updatedClusterUnits) > 0 {
					clusterUnits[i].Units = updatedClusterUnits[0].Units
				}
				level.Info(u.Logger).
					Log("msg", "Updater", "cluster_id", clusterUnits[i].Cluster.ID, "updater_id", updaterID)
			} else {
				level.Error(u.Logger).Log("msg", "Unknown updater ID", "cluster_id", clusterUnits[i].Cluster.ID, "updater_id", updaterID)
			}
		}
	}
	return clusterUnits
}
