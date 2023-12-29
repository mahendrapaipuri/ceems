//go:build !noipmi
// +build !noipmi

package collector

import (
	"reflect"
	"testing"

	"github.com/go-kit/log"
)

var (
	ipmidcmiStdout = `
Current Power                        : 332 Watts
Minimum Power over sampling duration : 68 watts
Maximum Power over sampling duration : 504 watts
Average Power over sampling duration : 348 watts
Time Stamp                           : 11/03/2023 - 08:36:29
Statistics reporting time period     : 2685198000 milliseconds
Power Measurement                    : Active
`
	ipmidcmiStdoutAlt = `
Current Power                        : 332 watts
Minimum Power over sampling duration : 68 Watts
Maximum Power over sampling duration : 504 Watts
Average Power over sampling duration : 348 Watts
Time Stamp                           : 11/03/2023 - 08:36:29
Statistics reporting time period     : 2685198000 milliseconds
Power Measurement                    : Active
`
	ipmidcmiStdoutDisactive = `
Power Measurement                    : Disable
`
	expectedPower = map[string]float64{
		"current": 332,
		"min":     68,
		"max":     504,
	}
)

func TestIpmiMetrics(t *testing.T) {
	c := impiCollector{logger: log.NewNopLogger()}
	value, err := c.getPowerReadings([]byte(ipmidcmiStdout))
	if err != nil {
		t.Errorf("failed to parse IPMI DCMI output: %v", err)
	}
	if !reflect.DeepEqual(value, expectedPower) {
		t.Fatalf("expected power %v. Got %v", expectedPower, value)
	}
}

func TestIpmiMetricsAlt(t *testing.T) {
	c := impiCollector{logger: log.NewNopLogger()}
	value, err := c.getPowerReadings([]byte(ipmidcmiStdoutAlt))
	if err != nil {
		t.Errorf("failed to parse IPMI DCMI output: %v", err)
	}
	if !reflect.DeepEqual(value, expectedPower) {
		t.Fatalf("expected power %v. Got %v", expectedPower, value)
	}
}

func TestIpmiMetricsDisactive(t *testing.T) {
	c := impiCollector{logger: log.NewNopLogger()}
	value, _ := c.getPowerReadings([]byte(ipmidcmiStdoutDisactive))
	if value != nil {
		t.Errorf("Expected nil output. Got %v", value)
	}
}
