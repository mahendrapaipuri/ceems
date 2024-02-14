package updater_one

import (
	"time"

	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/ceems/pkg/stats/base"
	"github.com/mahendrapaipuri/ceems/pkg/stats/types"
	"github.com/mahendrapaipuri/ceems/pkg/stats/updater"
)

type mockUpdater struct {
	logger log.Logger
}

const mockUpdaterHookName = "mock-one"

var (
	slurmUserUid       int
	slurmUserGid       int
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
	return &mockUpdater{
		logger: logger,
	}, nil
}

// Add the logic here to update the units retrieved from batch scheduler
func (u *mockUpdater) Update(queryTime time.Time, units []types.Unit) []types.Unit {
	return units
}
