package emissions

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadOWIDData(t *testing.T) {
	expectedFactors := EmissionFactors{
		"AF": EmissionFactor{"Afghanistan", 144.92754},
		"AL": EmissionFactor{"Albania", 23.437498},
		"DZ": EmissionFactor{"Algeria", 494.60645},
	}
	testData := `ASEAN (Ember),,2020,534.61163
ASEAN (Ember),,2021,525.5969
ASEAN (Ember),,2022,508.20422
Afghanistan,AFG,2000,255.31914
Afghanistan,AFG,2001,118.644066
Afghanistan,AFG,2002,144.92754
Albania,ALB,2016,23.376627
Albania,ALB,2017,24.55357
Albania,ALB,2018,23.61275
Albania,ALB,2019,23.21083
Albania,ALB,2020,24.482107
Albania,ALB,2021,23.437498
Algeria,DZA,2000,495.18628
Algeria,DZA,2001,494.60645
`
	gotFactors, err := readOWIDData([]byte(testData))
	require.NoError(t, err)
	assert.Equal(t, expectedFactors, gotFactors)
}

func TestNewOWIDProvider(t *testing.T) {
	_, err := NewOWIDProvider(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
}
