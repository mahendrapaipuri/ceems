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
	RegisterProvider(globalEmissionsProvider, "World Average", NewGlobalProvider)
}

// NewGlobalProvider returns a new Provider that returns a constant global average emission factor
func NewGlobalProvider(logger log.Logger) (Provider, error) {
	return &globalProvider{
		logger: logger,
	}, nil
}

// Get emission factor for a given country
func (s *globalProvider) Update() (EmissionFactors, error) {
	// We use a fake country code for world as we will need to setup Grafana
	// dashboards properly
	return EmissionFactors{"ZZ": EmissionFactor{"World", float64(globalEmissionFactor)}}, nil
}
