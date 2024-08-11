// Package updatertwo updates compute units
package updatertwo

import (
	"context"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
)

type mockUpdater struct {
	logger log.Logger
}

const mockUpdaterHookName = "mock-two"

var (
	mockUpdaterHookCLI = base.CEEMSServerApp.Flag(
		"updater.mock-two.arg",
		"Mock updater CLI arg.",
	).Default("").String()
)

func init() {
	// Register mock updater
	updater.Register(mockUpdaterHookName, NewMockUpdaterHook)
}

// NewMockUpdaterHook returns a new NewMockUpdaterHook to update units
func NewMockUpdaterHook(instance updater.Instance, logger log.Logger) (updater.Updater, error) {
	level.Info(logger).Log("msg", "CLI args", "arg1", mockUpdaterHookCLI)

	return &mockUpdater{
		logger: logger,
	}, nil
}

// Add the logic here to update the units retrieved from batch scheduler
func (u *mockUpdater) Update(
	_ context.Context,
	_ time.Time,
	_ time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return units
}
