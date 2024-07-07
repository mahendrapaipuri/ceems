//go:build !norapl
// +build !norapl

package collector

import (
	"testing"

	"github.com/prometheus/procfs/sysfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedEnergyMetrics = []float64{258218293244, 130570505826}

func TestRaplMetrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{"--path.sysfs", "testdata/sys"})
	require.NoError(t, err)

	fs, err := sysfs.NewFS(*sysPath)
	require.NoError(t, err)

	c := raplCollector{fs: fs}
	zones, err := sysfs.GetRaplZones(c.fs)
	require.NoError(t, err)

	for iz, rz := range zones {
		microJoules, err := rz.GetEnergyMicrojoules()
		require.NoError(t, err)
		assert.Equal(t, expectedEnergyMetrics[iz], float64(microJoules))
	}
}
