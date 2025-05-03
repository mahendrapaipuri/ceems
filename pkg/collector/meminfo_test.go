//go:build !nomemory
// +build !nomemory

package collector

import (
	"context"
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeminfoCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	collector, err := NewMeminfoCollector(noOpLogger)
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

func TestMemInfo(t *testing.T) {
	file, err := os.Open("testdata/proc/meminfo")
	require.NoError(t, err)
	defer file.Close()

	memInfo, err := parseMemInfo(file)
	require.NoError(t, err)

	want, got := 16042172416.0, memInfo["MemTotal_bytes"]
	assert.InEpsilon(t, want, got, 0)

	want, got = 16424894464.0, memInfo["DirectMap2M_bytes"]
	assert.InEpsilon(t, want, got, 0)
}
