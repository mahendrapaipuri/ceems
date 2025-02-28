//go:build !nonetdev
// +build !nonetdev

package collector

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetdevCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	collector, err := NewNetdevCollector(slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func TestProcNetDevStats(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.procfs", "testdata/proc",
	})
	require.NoError(t, err)

	// Make an instance of procfs
	fs, err := procfs.NewFS(*procfsPath)
	require.NoError(t, err)

	c := &netdevCollector{
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		fs:           fs,
		deviceFilter: newDeviceFilter("docker0|lo|vethf345468", ""),
		hostname:     hostname,
	}

	expectedStats := netDevStats{
		"eth0": map[string]uint64{
			"receive_bytes":       874354587,
			"receive_compressed":  0,
			"receive_dropped":     0,
			"receive_errors":      0,
			"receive_fifo":        0,
			"receive_frame":       0,
			"receive_multicast":   0,
			"receive_packets":     1036395,
			"transmit_bytes":      563352563,
			"transmit_carrier":    0,
			"transmit_colls":      0,
			"transmit_compressed": 0,
			"transmit_dropped":    0,
			"transmit_errors":     0,
			"transmit_fifo":       0,
			"transmit_packets":    732147,
		},
	}

	// Get stats
	netDevStats, err := c.procNetDevStats()
	require.NoError(t, err)

	assert.EqualValues(t, expectedStats, netDevStats)
}
