//go:build !noipmi
// +build !noipmi

package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ipmidcmiStdout = map[string]string{
		"freeipmi": `
Current Power                        : 332 Watts
Minimum Power over sampling duration : 68 watts
Maximum Power over sampling duration : 504 watts
Average Power over sampling duration : 348 watts
Time Stamp                           : 11/03/2023 - 08:36:29
Statistics reporting time period     : 2685198000 milliseconds
Power Measurement                    : Active
`, "freeipmiAlt": `
Current Power                        : 332 watts
Minimum Power over sampling duration : 68 Watts
Maximum Power over sampling duration : 504 Watts
Average Power over sampling duration : 348 Watts
Time Stamp                           : 11/03/2023 - 08:36:29
Statistics reporting time period     : 2685198000 milliseconds
Power Measurement                    : Active
`, "ipmitutil": `
ipmiutil dcmi ver 3.17
-- BMC version 6.10, IPMI version 2.0
DCMI Version:                   1.5
DCMI Power Management:          Supported
DCMI System Interface Access:   Supported
DCMI Serial TMode Access:       Supported
DCMI Secondary LAN Channel:     Supported
Current Power:                   332 Watts
Min Power over sample duration:  68 Watts
Max Power over sample duration:  504 Watts
Avg Power over sample duration:  348 Watts
Timestamp:                       Thu Feb 15 09:37:32 2024

Sampling period:                 1000 ms
Power reading state is:          active
Exception Action:  OEM defined
Power Limit:       896 Watts (inactive)
Correction Time:   62914560 ms
Sampling period:   1472 sec
ipmiutil dcmi, completed successfully
`, "ipmitool": `

	Instantaneous power reading:                   332 Watts
	Minimum during sampling period:                 68 Watts
	Maximum during sampling period:                504 Watts
	Average power reading over sample period:      348 Watts
	IPMI timestamp:                           Thu Feb 15 08:36:01 2024
	Sampling period:                          00000001 Seconds.
	Power reading state is:                   activated

`, "capmc": `{
"start_time":"2015-04-01 17:02:10",
"avg":348,
"min":68,
"max":504,
"window_len":600,
"e":0,
"err_msg":""
}`,
	}
	ipmidcmiStdoutDisactive = map[string]string{
		"freeipmi":   "Power Measurement                    : Not Available",
		"ipmitutil":  "Power reading state is:          not active",
		"ipmitool":   "Power reading state is:                   deactivated",
		crayPowerCap: `{"e":1,"err_msg":"failed"}`,
	}
	expectedPower = map[string]float64{
		"current": 332,
		"min":     68,
		"max":     504,
		"avg":     348,
	}
	expectedCapmcPower = map[string]float64{
		"current": 348,
		"min":     68,
		"max":     504,
		"avg":     348,
	}
)

func TestIPMICollector(t *testing.T) {
	_, err := CEEMSExporterApp.Parse([]string{
		"--collector.ipmi_dcmi.cmd", "testdata/ipmi/capmc/capmc",
		"--collector.ipmi_dcmi.test-mode",
	})
	require.NoError(t, err)

	collector, err := NewIPMICollector(noOpLogger)
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

func TestIpmiMetrics(t *testing.T) {
	c := impiCollector{logger: noOpLogger}

	for testName, testString := range ipmidcmiStdout {
		var value map[string]float64

		var err error

		expectedOutput := expectedPower
		if testName == crayPowerCap {
			expectedOutput = expectedCapmcPower
			value, err = c.parseCapmcOutput([]byte(testString))
		} else {
			value, err = c.parseIPMIOutput([]byte(testString))
		}

		require.NoError(t, err)
		assert.Equal(t, expectedOutput, value)
	}
}

func TestIpmiMetricsDisactive(t *testing.T) {
	c := impiCollector{logger: noOpLogger}

	for testName, testString := range ipmidcmiStdoutDisactive {
		var value map[string]float64
		if testName == crayPowerCap {
			value, _ = c.parseCapmcOutput([]byte(testString))
		} else {
			value, _ = c.parseIPMIOutput([]byte(testString))
		}

		assert.Empty(t, value)
	}
}

func TestIpmiDcmiFinder(t *testing.T) {
	tmpDir := t.TempDir()
	tmpIPMIPath := tmpDir + "/ipmi-dcmi"

	// Set path
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmpDir, os.Getenv("PATH")))

	ipmiDcmiPath, err := filepath.Abs("testdata/ipmi/freeipmi/ipmi-dcmi")
	require.NoError(t, err)

	err = os.Link(ipmiDcmiPath, tmpIPMIPath)
	require.NoError(t, err)

	// findIPMICmd() should give ipmi-dcmi command
	ipmiCmdSlice, err := findIPMICmd()
	require.NoError(t, err)
	assert.Equal(t, "ipmi-dcmi", ipmiCmdSlice[0])
}

func TestIpmiToolFinder(t *testing.T) {
	tmpDir := t.TempDir()
	tmpIPMIPath := tmpDir + "/ipmitool"

	// Set path
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmpDir, os.Getenv("PATH")))

	ipmiDcmiPath, err := filepath.Abs("testdata/ipmi/openipmi/ipmitool")
	require.NoError(t, err)

	err = os.Link(ipmiDcmiPath, tmpIPMIPath)
	require.NoError(t, err)

	// findIPMICmd() should give ipmitool command
	ipmiCmdSlice, err := findIPMICmd()
	require.NoError(t, err)
	assert.Equal(t, "ipmitool", ipmiCmdSlice[0])
}

func TestIpmiUtilFinder(t *testing.T) {
	tmpDir := t.TempDir()
	tmpIPMIPath := tmpDir + "/ipmiutil"

	// Set path
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmpDir, os.Getenv("PATH")))

	ipmiDcmiPath, err := filepath.Abs("testdata/ipmi/ipmiutils/ipmiutil")
	require.NoError(t, err)

	err = os.Link(ipmiDcmiPath, tmpIPMIPath)
	require.NoError(t, err)

	// findIPMICmd() should give ipmiutil command
	ipmiCmdSlice, err := findIPMICmd()
	require.NoError(t, err)
	assert.Equal(t, "ipmiutil", ipmiCmdSlice[0])
}

func TestCachedPowerReadings(t *testing.T) {
	tmpDir := t.TempDir()
	tmpIPMIPath := tmpDir + "/ipmiutil"

	// Set path
	t.Setenv("PATH", fmt.Sprintf("%s:%s", tmpDir, os.Getenv("PATH")))

	d1 := []byte(`#!/bin/bash

echo """ipmiutil dcmi ver 3.17
-- BMC version 6.10, IPMI version 2.0 
DCMI Version:                   1.5
DCMI Power Management:          Supported
DCMI System Interface Access:   Supported
DCMI Serial TMode Access:       Supported
DCMI Secondary LAN Channel:     Supported
  Current Power:                   304 Watts
  Min Power over sample duration:  6 Watts
  Max Power over sample duration:  304 Watts
  Avg Power over sample duration:  49 Watts
  Timestamp:                       Thu Feb 15 09:37:32 2024

  Sampling period:                 1000 ms
  Power reading state is:          active
  Exception Action:  OEM defined
  Power Limit:       896 Watts (inactive)
  Correction Time:   62914560 ms
  Sampling period:   1472 sec
ipmiutil dcmi, completed successfully"""`)
	err := os.WriteFile(tmpIPMIPath, d1, 0o700) //nolint:gosec
	require.NoError(t, err)

	// Expected values
	expected := map[string]float64{"avg": 49, "current": 304, "max": 304, "min": 6}

	_, err = CEEMSExporterApp.Parse([]string{
		"--collector.ipmi_dcmi.cmd", tmpIPMIPath,
		"--collector.ipmi_dcmi.test-mode",
	})
	require.NoError(t, err)

	collector, err := NewIPMICollector(noOpLogger)
	require.NoError(t, err)

	c := collector.(*impiCollector) //nolint:forcetypeassert

	// Setup background goroutine to capture metrics.
	metrics := make(chan prometheus.Metric)
	defer close(metrics)

	go func() {
		i := 0
		for range metrics {
			i++
		}
	}()

	// Get readings
	err = collector.Update(metrics)
	require.NoError(t, err)

	assert.Equal(t, expected, c.cachedMetric)

	// Modify IPMI command to give 0 current usage
	d1 = []byte(`#!/bin/bash

echo """ipmiutil dcmi ver 3.17
-- BMC version 6.10, IPMI version 2.0 
DCMI Version:                   1.5
DCMI Power Management:          Supported
DCMI System Interface Access:   Supported
DCMI Serial TMode Access:       Supported
DCMI Secondary LAN Channel:     Supported
  Current Power:                   0 Watts
  Min Power over sample duration:  6 Watts
  Max Power over sample duration:  304 Watts
  Avg Power over sample duration:  49 Watts
  Timestamp:                       Thu Feb 15 09:37:32 2024

  Sampling period:                 1000 ms
  Power reading state is:          active
  Exception Action:  OEM defined
  Power Limit:       896 Watts (inactive)
  Correction Time:   62914560 ms
  Sampling period:   1472 sec
ipmiutil dcmi, completed successfully"""`)
	err = os.WriteFile(tmpIPMIPath, d1, 0o700) //nolint:gosec
	require.NoError(t, err)

	// Get readings again and we should get last cached values
	got, err := c.update()
	require.NoError(t, err)

	assert.Equal(t, expected, got)
}
