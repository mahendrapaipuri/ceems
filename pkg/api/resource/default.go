package resource

import (
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

const defaultManager = "default"

// defaultResourceManager struct
type defaultResourceManager struct {
	logger log.Logger
}

func init() {
	// Register resource manager
	RegisterManager(defaultManager, NewDefaultResourceManager)
}

// NewDefaultResourceManager returns a new defaultResourceManager that returns empty compute units
func NewDefaultResourceManager(cluster models.Cluster, logger log.Logger) (Fetcher, error) {
	level.Info(logger).Log("msg", "Default resource manager activated")
	return &defaultResourceManager{
		logger: logger,
	}, nil
}

// Return empty units response
func (d *defaultResourceManager) Fetch(start time.Time, end time.Time) ([]models.ClusterUnits, error) {
	level.Info(d.logger).Log("msg", "Empty units fetched from default resource manager")
	return []models.ClusterUnits{
		{
			Cluster: models.Cluster{ID: "default"},
		},
	}, nil
}
