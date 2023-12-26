//go:build !emissions
// +build !emissions

package emissions

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const owidEmissionsSource = "owid"

type owidSource struct {
	logger      log.Logger
	countryCode string
}

var (
	StaticEmissionData = make(map[string]float64)
)

func init() {
	// Read global_energy_mix JSON file
	globalEnergyMixContents, err := dataDir.ReadFile("data/global_energy_mix.json")
	// JSON might contain NaN, replace it with null that is allowed in JSON
	globalEnergyMixContents = bytes.Replace(globalEnergyMixContents, []byte("NaN"), []byte("null"), -1)
	if err != nil {
		return
	}

	// Unmarshal JSON file into struct
	var globalEmissionData map[string]energyMixDataFields
	if err := json.Unmarshal(globalEnergyMixContents, &globalEmissionData); err != nil {
		return
	}

	// Populate map with country code and static carbon intensity
	for country, data := range globalEmissionData {
		// Set unavaible values to -1 to indicate they are indeed unavailable
		if data.CarbonIntensity == 0 {
			StaticEmissionData[country] = -1
		} else {
			StaticEmissionData[country] = data.CarbonIntensity
		}
	}

	// Register emissions source
	RegisterSource(owidEmissionsSource, NewOWIDSource)
}

// NewOWIDSource returns a new Source that returns emission factor from OWID data
func NewOWIDSource(ctx context.Context, logger log.Logger) (Source, error) {
	// Retrieve context values
	contextValues := ctx.Value(ContextKey{}).(ContextValues)
	level.Info(logger).Log("msg", "Emission factor from OWID data will be reported.")
	return &owidSource{
		logger:      logger,
		countryCode: contextValues.CountryCodeAlpha3,
	}, nil
}

// Get emission factor for a given country
func (s *owidSource) Update() (float64, error) {
	emissionFactor, ok := StaticEmissionData[s.countryCode]
	if ok {
		return float64(emissionFactor), nil
	} else {
		level.Error(s.logger).Log("msg", "Failed to retrieve data for OWID source", "err", "Country data not found")
	}
	return float64(-1), nil
}
