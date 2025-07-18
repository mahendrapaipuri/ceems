package resource

import (
	"context"
	"log/slog"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/models"
)

const defaultManager = "default"

// defaultResourceManager struct.
type defaultResourceManager struct {
	logger *slog.Logger
}

func init() {
	// Register resource manager
	Register(defaultManager, NewDefaultResourceManager)
}

// NewDefaultResourceManager returns a new defaultResourceManager that returns empty compute units.
func NewDefaultResourceManager(cluster models.Cluster, logger *slog.Logger) (Fetcher, error) {
	logger.Info("Default resource manager activated")

	return &defaultResourceManager{
		logger: logger,
	}, nil
}

// Return empty units response.
func (d *defaultResourceManager) FetchUnits(
	_ context.Context,
	start time.Time,
	end time.Time,
) ([]models.ClusterUnits, error) {
	d.logger.Info("Empty units fetched from default NoOp cluster")

	return []models.ClusterUnits{
		{
			Cluster: models.Cluster{ID: "default"},
		},
	}, nil
}

// Return empty projects response.
func (d *defaultResourceManager) FetchUsersProjects(
	_ context.Context,
	currentTime time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	d.logger.Info("Empty users and projects fetched from default NoOp cluster")

	return []models.ClusterUsers{
			{
				Cluster: models.Cluster{ID: "default"},
			},
		}, []models.ClusterProjects{
			{
				Cluster: models.Cluster{ID: "default"},
			},
		}, nil
}
