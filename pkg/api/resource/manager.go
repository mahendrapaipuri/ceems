// Package resource defines the interface that each resource manager needs to
// implement to get compute units
package resource

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

// Custom errors.
var (
	ErrDuplID         = errors.New("duplicate ID found in clusters config")
	ErrUnknownManager = errors.New("unknown resource manager found in the config")
)

// Config contains the configuration of resource manager cluster(s).
type Config[T any] struct {
	Clusters []T `yaml:"clusters"`
}

// Fetcher is the interface resource manager has to implement.
type Fetcher interface {
	// FetchUnits fetches compute units between start and end times
	FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error)
	// FetchUsersProjects fetches latest projects, users and their associations
	FetchUsersProjects(currentTime time.Time) ([]models.ClusterUsers, []models.ClusterProjects, error)
}

// Manager implements the interface to fetch compute units from different resource managers.
type Manager struct {
	Fetchers []Fetcher
	Logger   log.Logger
}

var factories = make(map[string]func(cluster models.Cluster, logger log.Logger) (Fetcher, error))

// Mutex lock.
var (
	unitFetcherLock = sync.RWMutex{}
	userFetcherLock = sync.RWMutex{}
)

// Register registers the resource manager into factory.
func Register(
	manager string,
	factory func(cluster models.Cluster, logger log.Logger) (Fetcher, error),
) {
	factories[manager] = factory
}

// checkConfig verifies for the errors in resource manager config and returns a map
// of manager to its configs.
func checkConfig(managers []string, config *Config[models.Cluster]) (map[string][]models.Cluster, error) {
	// Check if IDs are unique and manager is registered
	var IDs []string

	configMap := make(map[string][]models.Cluster)

	for i := range len(config.Clusters) {
		if slices.Contains(IDs, config.Clusters[i].ID) {
			return nil, fmt.Errorf("%w: %s", ErrDuplID, config.Clusters[i].ID)
		}

		if !slices.Contains(managers, config.Clusters[i].Manager) {
			return nil, fmt.Errorf("%w: %s", ErrUnknownManager, config.Clusters[i].Manager)
		}

		if base.InvalidIDRegex.MatchString(config.Clusters[i].ID) {
			return nil, fmt.Errorf(
				"invalid ID %s found in clusters config. It must contain only [a-zA-Z0-9-_]",
				config.Clusters[i].ID,
			)
		}

		IDs = append(IDs, config.Clusters[i].ID)
		configMap[config.Clusters[i].Manager] = append(configMap[config.Clusters[i].Manager], config.Clusters[i])
	}

	return configMap, nil
}

// managerConfig returns the configuration of resource managers.
func managerConfig() (*Config[models.Cluster], error) {
	// Make config from file
	config, err := common.MakeConfig[Config[models.Cluster]](base.ConfigFilePath)
	if err != nil {
		return nil, err
	}

	// Set directories
	for i := range len(config.Clusters) {
		config.Clusters[i].Web.HTTPClientConfig.SetDirectory(filepath.Dir(base.ConfigFilePath))
	}

	return config, nil
}

// New creates a new Manager struct instance.
func New(logger log.Logger) (*Manager, error) {
	var fetcher Fetcher

	var registeredManagers []string

	var fetchers []Fetcher

	var err error

	// Get all registered managers
	for manager := range factories {
		if manager != defaultManager {
			registeredManagers = append(registeredManagers, manager)
		}
	}

	// Get current config
	config, err := managerConfig()
	if err != nil {
		level.Error(logger).Log("msg", "Failed to parse resource manager config", "err", err)

		return nil, err
	}

	// Preflight checks on config
	configMap, err := checkConfig(registeredManagers, config)
	if err != nil {
		level.Error(logger).Log("msg", "Invalid resource manager config", "err", err)

		return nil, err
	}

	// Loop over factories and create new instances
	for key, factory := range factories {
		for _, config := range configMap[key] {
			fetcher, err = factory(config, log.With(logger, "manager", key))
			if err != nil {
				level.Error(logger).Log("msg", "Failed to setup resource manager", "name", key, "err", err)

				return nil, err
			}

			fetchers = append(fetchers, fetcher)
		}
	}

	// Return an instance of default manager
	if len(fetchers) == 0 {
		level.Warn(logger).Log(
			"msg", "No clusters found in config. Using a default cluster",
			"available_resource_managers", strings.Join(registeredManagers, ","),
		)

		fetcher, err = factories[defaultManager](models.Cluster{}, log.With(logger, "manager", defaultManager))
		if err != nil {
			level.Error(logger).Log("msg", "Failed to setup default resource manager", "err", err)

			return nil, err
		}

		fetchers = append(fetchers, fetcher)
	}

	return &Manager{Fetchers: fetchers, Logger: logger}, nil
}

// FetchUnits implements collection jobs between start and end times.
func (b Manager) FetchUnits(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "units fetcher", b.Logger)

	var clusterUnits []models.ClusterUnits

	var errs error

	var wg sync.WaitGroup

	wg.Add((len(b.Fetchers)))

	for _, fetcher := range b.Fetchers {
		go func(f Fetcher) {
			units, err := f.FetchUnits(start, end)
			if err != nil {
				unitFetcherLock.Lock()
				errs = errors.Join(errs, err)
				unitFetcherLock.Unlock()
				wg.Done()

				return
			}

			unitFetcherLock.Lock()
			clusterUnits = append(clusterUnits, units...)
			unitFetcherLock.Unlock()
			wg.Done()
		}(fetcher)
	}

	wg.Wait()

	return clusterUnits, errs
}

// FetchUsersProjects fetches latest projects and users for each cluster.
func (b Manager) FetchUsersProjects(currentTime time.Time) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	// Measure elapsed time
	defer common.TimeTrack(time.Now(), "users and projects fetcher", b.Logger)

	var clusterUsers []models.ClusterUsers

	var clusterProjects []models.ClusterProjects

	var errs error

	var wg sync.WaitGroup

	wg.Add((len(b.Fetchers)))

	for _, fetcher := range b.Fetchers {
		go func(f Fetcher) {
			users, projects, err := f.FetchUsersProjects(currentTime)
			if err != nil {
				userFetcherLock.Lock()
				errs = errors.Join(errs, err)
				userFetcherLock.Unlock()
				wg.Done()

				return
			}

			userFetcherLock.Lock()
			clusterUsers = append(clusterUsers, users...)
			clusterProjects = append(clusterProjects, projects...)
			userFetcherLock.Unlock()
			wg.Done()
		}(fetcher)
	}

	wg.Wait()

	return clusterUsers, clusterProjects, errs
}
