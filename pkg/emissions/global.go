//go:build !emissions
// +build !emissions

package emissions

import (
	"context"

	"github.com/go-kit/log"
)

const (
	globalEmissionFactor    = 475
	globalEmissionsProvider = "global"
)

type globalProvider struct {
	logger log.Logger
}

func init() {
	// Register emissions provider
	RegisterProvider(globalEmissionsProvider, "Global Average", NewGlobalProvider)
}

// NewGlobalProvider returns a new Provider that returns a constant global average emission factor
func NewGlobalProvider(ctx context.Context, logger log.Logger) (Provider, error) {
	return &globalProvider{
		logger: logger,
	}, nil
}

// Get emission factor for a given country
func (s *globalProvider) Update() (float64, error) {
	return float64(globalEmissionFactor), nil
}
