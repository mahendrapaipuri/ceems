// Package updaterone updates the compute units
package updaterone

import (
	"context"
	"log/slog"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
)

type mockUpdater struct {
	logger *slog.Logger
}

const mockUpdaterHookName = "mock-one"

var mockUpdaterHookCLI = base.CEEMSServerApp.Flag(
	"updater.mock-one.arg",
	"Mock updater CLI arg.",
).Default("").String()

// Register mock updater.
func init() {
	updater.Register(mockUpdaterHookName, NewMockUpdaterHook)
}

// NewMockUpdaterHook returns a new NewMockUpdaterHook to update units.
func NewMockUpdaterHook(instance updater.Instance, logger *slog.Logger) (updater.Updater, error) {
	logger.Info("CLI args", "arg1", mockUpdaterHookCLI)

	return &mockUpdater{
		logger: logger,
	}, nil
}

// Add the logic here to update the units retrieved from batch scheduler.
func (u *mockUpdater) Update(
	_ context.Context,
	_ time.Time,
	_ time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return units
}
