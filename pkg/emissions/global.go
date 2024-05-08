//go:build !emissions
// +build !emissions

package emissions

import (
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
func NewGlobalProvider(logger log.Logger) (Provider, error) {
	return &globalProvider{
		logger: logger,
	}, nil
}

// Get emission factor for a given country
func (s *globalProvider) Update() (EmissionFactors, error) {
	// Use empty string as map key as there should not be a code
	// for global factor
	// Promtheus, by default, drops the empty labels and thus it wont appear
	return EmissionFactors{"": EmissionFactor{"Global", float64(globalEmissionFactor)}}, nil
}
