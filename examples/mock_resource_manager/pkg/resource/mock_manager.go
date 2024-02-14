package resource

import (
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/stats/base"
	"github.com/mahendrapaipuri/ceems/pkg/stats/resource"
	"github.com/mahendrapaipuri/ceems/pkg/stats/types"
)

type mockManager struct {
	logger log.Logger
}

const mockResourceManager = "mock"

var (
	macctPath = base.CEEMSServerApp.Flag(
		"mock.acct.path",
		"Absolute path to mock scheduler's accounting executable.",
	).Default("/usr/local/bin/macct").String()
)

func init() {
	// Register manager
	resource.RegisterManager(mockResourceManager, NewMockManager)
}

// Do all basic checks here
func preflightChecks(logger log.Logger) error {
	return nil
}

// NewMockManager returns a new MockManager that returns compute units
func NewMockManager(logger log.Logger) (resource.Fetcher, error) {
	err := preflightChecks(logger)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create mock manager.", "err", err)
		return nil, err
	}
	level.Info(logger).Log("msg", "Compute units from mock resource manager will be retrieved.")
	return &mockManager{
		logger: logger,
	}, nil
}

// Add the logic here to get compute units from resource manager and return slice of Unit structs
//
// When making Unit stucts, ensure to format the datetime using base.DatetimeLayout
// Also ensure to set StartTS and EndTS fields to start and end times in unix milliseconds epoch
func (s *mockManager) Fetch(start time.Time, end time.Time) ([]types.Unit, error) {
	return []types.Unit{{UUID: "1000"}, {UUID: "1100"}}, nil
}
