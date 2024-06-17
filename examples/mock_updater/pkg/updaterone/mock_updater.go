// Package updaterone updates the compute units
package updaterone

import (
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

const mockUpdaterHookName = "mock-one"

var (
	mockUpdaterHookCLI = base.CEEMSServerApp.Flag(
		"updater.mock-one.arg",
		"Mock updater CLI arg.",
	).Default("").String()
)

// Register mock updater
func init() {
	updater.RegisterUpdater(mockUpdaterHookName, NewMockUpdaterHook)
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
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	return units
}
