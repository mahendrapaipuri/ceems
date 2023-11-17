//go:build !noemissions
// +build !noemissions

package collector

import (
	"testing"

	"github.com/go-kit/log"
)

func TestEmissionsMetrics(t *testing.T) {
	// Mock GetRteEnergyMixData
	expectedFactor := float64(55)
	getRteEnergyMixData = func() (float64, error) {
		return expectedFactor, nil
	}
	c := emissionsCollector{logger: log.NewNopLogger(), countryCode: "FRA", energyData: map[string]float64{"FRA": 50, "DEU": 51, "CHL": 52}}
	value := c.getCurrentEmissionFactor()
	if value != expectedFactor {
		t.Fatalf("new collection: expected factor %f. Got %f", expectedFactor, value)
	}

	// Change mock to give different value
	getRteEnergyMixData = func() (float64, error) {
		return float64(65), nil
	}
	value = c.getCurrentEmissionFactor()
	// Should give 55 due to caching we are doing
	if value != expectedFactor {
		t.Fatalf("cached collection: expected factor %f. Got %f", expectedFactor, value)
	}

	// Test for different country
	expectedFactor = float64(55)
	c = emissionsCollector{logger: log.NewNopLogger(), countryCode: "DEU", energyData: map[string]float64{"FRA": 50, "DEU": expectedFactor, "CHL": 52}}
	value = c.getCurrentEmissionFactor()
	if value != expectedFactor {
		t.Fatalf("energydata collection: expected factor %f. Got %f", expectedFactor, value)
	}
}
