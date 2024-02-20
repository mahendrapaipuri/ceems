// Package updaterone updates the compute units
package updaterone

import (
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/stats/base"
	"github.com/mahendrapaipuri/ceems/pkg/stats/models"
	"github.com/mahendrapaipuri/ceems/pkg/stats/updater"
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
	updater.RegisterUpdater(mockUpdaterHookName, false, NewMockUpdaterHook)
}

// NewMockUpdaterHook returns a new NewMockUpdaterHook to update units
func NewMockUpdaterHook(logger log.Logger) (updater.Updater, error) {
	level.Error(logger).Log("msg", "CLI args", "arg1", mockUpdaterHookCLI)
	return &mockUpdater{
		logger: logger,
	}, nil
}

// Add the logic here to update the units retrieved from batch scheduler
func (u *mockUpdater) Update(startTime time.Time, endTime time.Time, units []models.Unit) []models.Unit {
	return units
}
