// Package updater will provide an interface to update the unit stucts before
// inserting into DB
//
// Users can implement their own logic to mutate units struct to manipulate each
// unit struct
package updater

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"time"

	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"gopkg.in/yaml.v3"
)

// Custom errors.
var (
	ErrDuplID         = errors.New("duplicate ID found in updaters config")
	ErrUnknownUpdater = errors.New("unknown updater found in the config")
	ErrInvalidID      = errors.New("invalid updater ID. It must contain only [a-zA-Z0-9-_]")
)

// Instance contains the configuration of the given updater.
type Instance struct {
	ID      string           `yaml:"id"`
	Updater string           `yaml:"updater"`
	Web     models.WebConfig `yaml:"web"`
	CLI     models.CLIConfig `yaml:"cli"`
	Extra   yaml.Node        `yaml:"extra_config"`
}

// Config contains the configuration of updater(s).
type Config[T any] struct {
	Instances []T `yaml:"updaters"`
}

// Updater interface.
type Updater interface {
	Update(
		ctx context.Context,
		startTime time.Time,
		endTime time.Time,
		units []models.ClusterUnits,
	) []models.ClusterUnits
}

// UnitUpdater implements the interface to update compute units from different updaters.
type UnitUpdater struct {
	Updaters map[string]Updater
	Logger   *slog.Logger
}

// Slice of updaters.
var (
	updaterFactories = make(map[string]func(instance Instance, logger *slog.Logger) (Updater, error))
)

// Register registers updater struct into factories.
func Register(
	name string,
	factory func(instance Instance, logger *slog.Logger) (Updater, error),
) {
	updaterFactories[name] = factory
}

// checkConfig verifies for the errors in updater config and returns a map
// of updater to its configs.
func checkConfig(updaters []string, config *Config[Instance]) (map[string][]Instance, error) {
	// Check if IDs are unique and updater is registered
	var IDs []string //nolint:prealloc

	configMap := make(map[string][]Instance)

	for i := range len(config.Instances) {
		if slices.Contains(IDs, config.Instances[i].ID) {
			return nil, fmt.Errorf("%w: %s", ErrDuplID, config.Instances[i].ID)
		}

		if !slices.Contains(updaters, config.Instances[i].Updater) {
			return nil, fmt.Errorf("%w: %s", ErrUnknownUpdater, config.Instances[i].Updater)
		}

		if base.InvalidIDRegex.MatchString(config.Instances[i].ID) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidID, config.Instances[i].ID)
		}

		IDs = append(IDs, config.Instances[i].ID)
		configMap[config.Instances[i].Updater] = append(configMap[config.Instances[i].Updater], config.Instances[i])
	}

	return configMap, nil
}

// updaterConfig returns the configuration of updaters.
func updaterConfig() (*Config[Instance], error) {
	// Merge default config with provided config
	config, err := common.MakeConfig[Config[Instance]](base.ConfigFilePath, base.ConfigFileExpandEnvVars)
	if err != nil {
		return nil, err
	}

	// Set directories
	for i := range len(config.Instances) {
		config.Instances[i].Web.HTTPClientConfig.SetDirectory(filepath.Dir(base.ConfigFilePath))
	}

	return config, nil
}

// New creates a new UnitUpdater.
func New(logger *slog.Logger) (*UnitUpdater, error) {
	var updater Updater

	updaters := make(map[string]Updater)

	var registeredUpdaters []string //nolint:prealloc

	var err error

	// Get all registered updaters
	for updaterName := range updaterFactories {
		registeredUpdaters = append(registeredUpdaters, updaterName)
	}

	// Get current config
	config, err := updaterConfig()
	if err != nil {
		logger.Error("Failed to parse updater config", "err", err)

		return nil, err
	}

	// Preflight checks on config
	configMap, err := checkConfig(registeredUpdaters, config)
	if err != nil {
		logger.Error("Invalid updater config", "err", err)

		return nil, err
	}

	// Loop over factories and create new instances
	for key, factory := range updaterFactories {
		for _, config := range configMap[key] {
			updater, err = factory(config, logger.With("updater", key))
			if err != nil {
				logger.Error("Failed to setup unit updater", "name", key, "err", err)

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

// Update implements updating units using registered updaters.
func (u UnitUpdater) Update(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	clusterUnits []models.ClusterUnits,
) []models.ClusterUnits {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "updater", u.Logger)

	// If there are no registered updaters, return
	if len(u.Updaters) == 0 {
		return clusterUnits
	}

	// Iterate through units and apply updater for each clusterUnit
	for i := range clusterUnits {
		if len(clusterUnits[i].Units) == 0 || len(clusterUnits[i].Cluster.Updaters) == 0 {
			continue
		}

		for j := range len(clusterUnits[i].Cluster.Updaters) {
			updaterID := clusterUnits[i].Cluster.Updaters[j]

			// Check if updaterID is valid
			if updater, ok := u.Updaters[updaterID]; ok {
				// Only update Units slice and do not touch cluster meta data
				updatedClusterUnits := updater.Update(ctx, startTime, endTime, []models.ClusterUnits{clusterUnits[i]})
				// Just to ensure we wont have nil pointer dereferencing errors in runtime
				if len(updatedClusterUnits) > 0 {
					clusterUnits[i].Units = updatedClusterUnits[0].Units
				}

				u.Logger.Info("Updater", "cluster_id", clusterUnits[i].Cluster.ID, "updater_id", updaterID)
			} else {
				u.Logger.Error("Unknown updater ID", "cluster_id", clusterUnits[i].Cluster.ID, "updater_id", updaterID)
			}
		}
	}

	return clusterUnits
}
