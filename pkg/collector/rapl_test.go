//go:build !norapl
// +build !norapl

package collector

import (
	"context"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedEnergyMetrics = []float64{258218293244, 130570505826}

func TestRaplCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys",
	})
	require.NoError(t, err)

	collector, err := NewRaplCollector(log.NewNopLogger())
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
}
