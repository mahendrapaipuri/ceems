//go:build !noemissions
// +build !noemissions

package collector

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestEmissionsCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse(
		[]string{
			"--collector.emissions.provider", "owid",
		},
	)
	require.NoError(t, err)

	collector, err := NewEmissionsCollector(noOpLogger)
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
