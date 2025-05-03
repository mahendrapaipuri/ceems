//go:build !norapl
// +build !norapl

package collector

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	expectedEnergyMetrics = []float64{258218293244, 24468409791, 130570505826, 64930394974}
	expectedPowerLimits   = map[sysfs.RaplZone]uint64{
		{
			Name:           "package",
			Index:          0,
			Path:           "testdata/sys/class/powercap/intel-rapl:0",
			MaxMicrojoules: 0x3d08f5c252,
		}: 0xaba9500,
		{
			Name:           "dram",
			Index:          0,
			Path:           "testdata/sys/class/powercap/intel-rapl:0:0",
			MaxMicrojoules: 65712999613,
		}: 0,
		{
			Name:           "package",
			Index:          1,
			Path:           "testdata/sys/class/powercap/intel-rapl:1",
			MaxMicrojoules: 0x3d08f5c252,
		}: 0xaba9500,
		{
			Name:           "dram",
			Index:          1,
			Path:           "testdata/sys/class/powercap/intel-rapl:1:0",
			MaxMicrojoules: 65712999613,
		}: 0,
	}
)

func TestRaplCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys",
	})
	require.NoError(t, err)

	collector, err := NewRaplCollector(noOpLogger)
	require.NoError(t, err)

	// Setup background goroutine to capture metrics.
	metrics := make(chan prometheus.Metric)
	defer close(metrics)

	go func() {
		i := 0
		for range metrics {
			i++
		}
	}()

	err = collector.Update(metrics)
	require.NoError(t, err)

	err = collector.Stop(context.Background())
	require.NoError(t, err)
}

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
		assert.InEpsilon(t, expectedEnergyMetrics[iz], float64(microJoules), 0)
	}

	powerLimits, err := readPowerLimits(zones)
	require.NoError(t, err)
	assert.Equal(t, expectedPowerLimits, powerLimits)
}
