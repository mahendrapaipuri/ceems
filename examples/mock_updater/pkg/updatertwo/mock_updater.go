// Package updatertwo updates compute units
package updatertwo

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

const mockUpdaterHookName = "mock-two"

var (
	mockUpdaterHookCLI = base.CEEMSServerApp.Flag(
		"updater.mock-two.arg",
		"Mock updater CLI arg.",
	).Default("").String()
)

func init() {
	// Register mock updater
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
