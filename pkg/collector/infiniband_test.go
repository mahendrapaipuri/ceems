//go:build !noinfiniband
// +build !noinfiniband

package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestIBCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys",
	})
	require.NoError(t, err)

	collector, err := NewInfiniBandCollector(noOpLogger)
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

	err = collector.Stop(t.Context())
	require.NoError(t, err)
}
