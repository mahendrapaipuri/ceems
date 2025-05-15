//go:build !nocraypmc
// +build !nocraypmc

package collector

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs/sysfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrayPMCCollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys", "--collector.empty-hostname-label",
	})
	require.NoError(t, err)

	collector, err := NewCrayPMCCollector(noOpLogger)
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

func TestGetCrayPMCDomains(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--path.sysfs", "testdata/sys", "--collector.empty-hostname-label",
	})
	require.NoError(t, err)

	fs, err := sysfs.NewFS(*sysPath)
	require.NoError(t, err)

	domains, err := GetCrayPMCDomains(fs)
	require.NoError(t, err)

	expectedDomains := []PMCDomain{
		{Name: "accel0", Path: "testdata/sys/cray/pm_counters/accel0_"},
		{Name: "accel1", Path: "testdata/sys/cray/pm_counters/accel1_"},
		{Name: "accel2", Path: "testdata/sys/cray/pm_counters/accel2_"},
		{Name: "accel3", Path: "testdata/sys/cray/pm_counters/accel3_"},
		{Name: "cpu0", Path: "testdata/sys/cray/pm_counters/cpu0_"},
		{Name: "cpu", Path: "testdata/sys/cray/pm_counters/cpu_"},
		{Name: "node", Path: "testdata/sys/cray/pm_counters/"},
		{Name: "memory", Path: "testdata/sys/cray/pm_counters/memory_"},
	}

	assert.Equal(t, expectedDomains, domains)

	expectedCounterValues := map[string]map[string]uint64{
		"energy": {
			"accel0": 208970342,
			"accel1": 268461017,
			"accel2": 271696890,
			"accel3": 251713932,
			"cpu":    139431088,
			"node":   1360621494,
			"memory": 104283458,
			"cpu0":   0,
		},
		"power": {
			"accel0": 99,
			"accel1": 197,
			"accel2": 263,
			"accel3": 123,
			"cpu":    83,
			"node":   873,
			"memory": 75,
			"cpu0":   0,
		},
		"power_cap": {
			"accel0": 0,
			"accel1": 0,
			"accel2": 0,
			"accel3": 0,
			"cpu":    0,
			"node":   0,
			"memory": 0,
			"cpu0":   0,
		},
		"temp": {
			"cpu0": 48,
		},
	}

	const tempDomain = "cpu0"

	// Test counter values
	for _, domain := range expectedDomains {
		val, err := domain.GetEnergyJoules()
		if domain.Name == tempDomain {
			require.Error(t, err, domain.Name)
		} else {
			require.NoError(t, err, domain.Name)
		}

		assert.Equal(t, expectedCounterValues["energy"][domain.Name], val, domain.Name)

		val, err = domain.GetPowerWatts()
		if domain.Name == tempDomain {
			require.Error(t, err, domain.Name)
		} else {
			require.NoError(t, err, domain.Name)
		}

		assert.Equal(t, expectedCounterValues["power"][domain.Name], val, domain.Name)

		val, err = domain.GetPowerLimitWatts()
		if domain.Name == tempDomain || domain.Name == "cpu" || domain.Name == "memory" {
			require.Error(t, err, domain.Name)
		} else {
			require.NoError(t, err, domain.Name)
		}

		assert.Equal(t, expectedCounterValues["power_cap"][domain.Name], val, domain.Name)

		val, err = domain.GetTempCelsius()
		if domain.Name == tempDomain {
			require.NoError(t, err, domain.Name)
		} else {
			require.Error(t, err, domain.Name)
		}

		assert.Equal(t, expectedCounterValues["temp"][domain.Name], val, domain.Name)
	}
}
