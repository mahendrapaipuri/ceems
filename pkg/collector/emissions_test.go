//go:build !noemissions
// +build !noemissions

package collector

import (
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/mahendrapaipuri/batchjob_monitoring/pkg/emissions"
)

var (
	logger = log.NewNopLogger()
)

func TestISO2ToISO3Convertion(t *testing.T) {
	if _, err := kingpin.CommandLine.Parse(
		[]string{
			"--collector.emissions.country.code", "FR",
		},
	); err != nil {
		t.Fatal(err)
	}

	// Mock country codes map
	countryCodesMap = []emissions.CountryCodeFields{
		{
			Alpha2Code: "FR",
			Alpha3Code: "FRA",
		},
	}
	expected := "FRA"
	if expected != convertISO2ToISO3("FR") {
		t.Errorf("Expected %s. Got %s", expected, convertISO2ToISO3("FR"))
	}
}
