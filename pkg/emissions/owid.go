//go:build !emissions
// +build !emissions

package emissions

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const owidEmissionsProvider = "owid"

type owidProvider struct {
	logger       log.Logger
	emissionData EmissionFactors
}

func init() {
	// Register emissions provider
	Register(owidEmissionsProvider, "OWID", NewOWIDProvider)
}

// readOWIDData reads the carbon intensity CSV file and returns the most "recent"
// factor for each country.
// The file can be fetched from https://ourworldindata.org/grapher/carbon-intensity-electricity?tab=table
// The data is updated every year and the next update will be in December 2024
// Data sources: Ember - Yearly Electricity Data (2023); Ember - European Electricity Review (2022); Energy Institute - Statistical Review of World Energy (2023).
func readOWIDData(contents []byte) (EmissionFactors, error) {
	// Read all records
	// Each record is of format: Name, Code, Year, Value
	csvReader := csv.NewReader(bytes.NewReader(contents))

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("unable to parse file as OWID CSV file: %w", err)
	}

	// Get ISO-3 to ISO-2 map so that we convert all country codes to ISO 2
	codeMap := ISO32Map()

	// Normally the records are sorted by year and it is slice of slices. So, by
	// keeping only last entry for each country we should get the latest factor
	// If the country code is empty string, the factor is for a region that is bigger/smaller
	// than the country.
	emissionFactors := make(EmissionFactors)

	var countryCode string

	var ok bool

	for _, record := range records {
		// If record does not have atleast 4 columns, skip
		if len(record) < 4 {
			continue
		}

		// Only when country code is non empty string
		if record[1] == "" {
			continue
		}

		// Get ISO2 code for the current country
		if countryCode, ok = codeMap[record[1]]; !ok {
			continue
		}

		// Populate emissionFactors map
		if val, err := strconv.ParseFloat(record[3], 64); err == nil {
			emissionFactors[countryCode] = EmissionFactor{record[0], val}
		}
	}

	return emissionFactors, nil
}

// NewOWIDProvider returns a new Provider that returns emission factor from OWID data.
func NewOWIDProvider(logger log.Logger) (Provider, error) {
	// Read CSV file
	carbonIntensityCSV, err := dataDir.ReadFile("data/carbon-intensity-owid.csv")
	if err != nil {
		return nil, fmt.Errorf("failed to read OWID data file: %w", err)
	}

	// Read OWID data CSV file
	emissionData, err := readOWIDData(carbonIntensityCSV)
	if err != nil {
		return nil, err
	}

	level.Info(logger).Log("msg", "Emission factor from OWID data will be reported.")

	return &owidProvider{
		logger:       logger,
		emissionData: emissionData,
	}, nil
}

// Get emission factor for a given country.
func (s *owidProvider) Update() (EmissionFactors, error) {
	return s.emissionData, nil
}
