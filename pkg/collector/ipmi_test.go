//go:build !noipmi
// +build !noipmi

package collector

import (
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
	expectedPower = float64(332)
)

func TestIpmiMetrics(t *testing.T) {
	c := impiCollector{logger: log.NewNopLogger()}
	value, err := c.getCurrentPowerConsumption([]byte(ipmidcmiStdout))
	if err != nil {
		t.Errorf("failed to parse IPMI DCMI output: %v", err)
	}
	if value != expectedPower {
		t.Fatalf("expected power %f. Got %f", expectedPower, value)
	}
}
