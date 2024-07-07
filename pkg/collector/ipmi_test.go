//go:build !noipmi
// +build !noipmi

package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
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
}`}
	ipmidcmiStdoutDisactive = map[string]string{
		"freeipmi":  "Power Measurement                    : Not Available",
		"ipmitutil": "Power reading state is:          not active",
		"ipmitool":  "Power reading state is:                   deactivated",
		"capmc":     `{"e":1,"err_msg":"failed"}`,
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

func TestIpmiMetrics(t *testing.T) {
	c := impiCollector{logger: log.NewNopLogger()}
	for testName, testString := range ipmidcmiStdout {
		var value map[string]float64
		var err error
		expectedOutput := expectedPower
		if testName == "capmc" {
			expectedOutput = expectedCapmcPower
			value, err = c.parseCapmcOutput([]byte(testString))
		} else {
			value, err = c.parseIPMIOutput([]byte(testString))
		}
		require.NoError(t, err)
		assert.Equal(t, value, expectedOutput)
	}
}

func TestIpmiMetricsDisactive(t *testing.T) {
	c := impiCollector{logger: log.NewNopLogger()}
	for testName, testString := range ipmidcmiStdoutDisactive {
		var value map[string]float64
		if testName == "capmc" {
			value, _ = c.parseCapmcOutput([]byte(testString))
		} else {
			value, _ = c.parseIPMIOutput([]byte(testString))
		}
		assert.Empty(t, value)
		// if value != nil {
		// 	t.Errorf("%s: expected nil output. Got %v", testName, value)
		// }
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
	ipmiCmdSlice := findIPMICmd()
	assert.Equal(t, ipmiCmdSlice[0], "ipmi-dcmi")
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
	ipmiCmdSlice := findIPMICmd()
	assert.Equal(t, ipmiCmdSlice[0], "ipmitool")
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
	ipmiCmdSlice := findIPMICmd()
	assert.Equal(t, ipmiCmdSlice[0], "ipmiutil")
}
