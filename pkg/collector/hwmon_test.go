//go:build !nohwmon
// +build !nohwmon

package collector

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedHwmonValues = map[string]map[string]map[string]float64{
	"socket": {
		"input": {
			"energy1": 2.2323e+10,
			"energy2": 9.509e+09,
			"power1":  2.7e+08,
			"power2":  1.56e+08,
		},
		"average": {
			"power1": 2.34e+08,
			"power2": 1.2e+08,
		},
		"input_lowest": {
			"power1": 6.5e+07,
			"power2": 4.6e+07,
		},
		"input_highest": {
			"power1": 3.28e+08,
			"power2": 1.9e+08,
		},
	},
}

func TestHwmonCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys",
	})
	require.NoError(t, err)

	collector, err := NewHwmonCollector(noOpLogger)
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

func TestHwmonMetrics(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys",
	})
	require.NoError(t, err)

	c := &hwmonCollector{
		logger: noOpLogger,
	}

	err = c.discoverMonitors()
	require.NoError(t, err)

	gotValues := make(map[string]map[string]map[string]float64)

	for _, mon := range c.monitors {
		if gotValues[mon.name] == nil {
			gotValues[mon.name] = make(map[string]map[string]float64)
		}

		for _, sensor := range mon.sensors {
			if val := readSensorValue(sensor.sensorFile); val > 0 {
				if gotValues[mon.name][sensor.sensorProperty] == nil {
					gotValues[mon.name][sensor.sensorProperty] = make(map[string]float64)
				}

				gotValues[mon.name][sensor.sensorProperty][fmt.Sprintf("%s%d", sensor.sensorType, sensor.sensorNum)] = val
			}
		}
	}

	assert.Equal(t, expectedHwmonValues, gotValues)
}
