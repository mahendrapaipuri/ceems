package emissions

import (
	"testing"

	"github.com/go-kit/log"
)

var (
	expectedOWIDFactor = float64(67.39)
)

func TestOWIDDataSource(t *testing.T) {
	s := owidSource{
		logger:      log.NewNopLogger(),
		countryCode: "FRA",
	}

	// Get current emission factor
	factor, err := s.Update()
	if err != nil {
		t.Errorf("failed update emission factor for owid: %v", err)
	}
	if factor != expectedOWIDFactor {
		t.Errorf("Expected %f got %f for owid", expectedOWIDFactor, factor)
	}
}
