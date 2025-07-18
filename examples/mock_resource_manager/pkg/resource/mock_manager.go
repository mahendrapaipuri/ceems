// Package resource implements the Fetcher interface that retrieves compute units
// from resource manager
package resource

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/base"
	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/api/resource"
)

type mockManager struct {
	logger *slog.Logger
}

const mockResourceManager = "mock"

var macctPath = base.CEEMSServerApp.Flag(
	"mock.acct.path",
	"Absolute path to mock scheduler's accounting executable.",
).Default("/usr/local/bin/macct").String()

func init() {
	// Register manager
	resource.Register(mockResourceManager, NewMockManager)
}

// Do all basic checks here.
func preflightChecks(logger *slog.Logger) error {
	if _, err := os.Stat(*macctPath); err != nil {
		logger.Error("Failed to open executable", "path", *macctPath, "err", err)

		return err
	}

	return nil
}

// NewMockManager returns a new MockManager that returns compute units.
func NewMockManager(cluster models.Cluster, logger *slog.Logger) (resource.Fetcher, error) {
	err := preflightChecks(logger)
	if err != nil {
		logger.Error("Failed to create mock manager.", "err", err)

		return nil, err
	}

	logger.Info("Compute units from mock resource manager will be retrieved.")

	return &mockManager{
		logger: logger,
	}, nil
}

// Also ensure to set StartTS and EndTS fields to start and end times in unix milliseconds epoch.
func (s *mockManager) FetchUnits(_ context.Context, _ time.Time, _ time.Time) ([]models.ClusterUnits, error) {
	return []models.ClusterUnits{
		{
			Cluster: models.Cluster{
				ID: "mock",
			},
			Units: []models.Unit{
				{
					UUID: "10000",
				},
				{
					UUID: "11000",
				},
			},
		},
	}, nil
}

// resource manager.
func (s *mockManager) FetchUsersProjects(
	_ context.Context,
	_ time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	return []models.ClusterUsers{
			{
				Cluster: models.Cluster{
					ID: "mock",
				},
				Users: []models.User{
					{
						Name:     "usr1",
						Projects: models.List{"prj1", "prj2"},
					},
				},
			},
		}, []models.ClusterProjects{
			{
				Cluster: models.Cluster{
					ID: "mock",
				},
				Projects: []models.Project{
					{
						Name:  "usr1",
						Users: models.List{"prj1", "prj2"},
					},
				},
			},
		}, nil
}
