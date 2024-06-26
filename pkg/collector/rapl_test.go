//go:build !norapl
// +build !norapl

package collector

import (
	"testing"

	"github.com/prometheus/procfs/sysfs"
)

var expectedEnergyMetrics = []float64{258218293244, 130570505826}

func TestRaplMetrics(t *testing.T) {
	if _, err := CEEMSExporterApp.Parse([]string{"--path.sysfs", "testdata/sys"}); err != nil {
		t.Fatal(err)
	}
	fs, err := sysfs.NewFS(*sysPath)
	if err != nil {
		t.Errorf("failed to open procfs: %v", err)
	}
	c := raplCollector{fs: fs}
	zones, err := sysfs.GetRaplZones(c.fs)
	if err != nil {
		t.Errorf("failed to get RAPL zones: %v", err)
	}
	for iz, rz := range zones {
		microJoules, err := rz.GetEnergyMicrojoules()
		if err != nil {
			t.Fatalf("Cannot fetch energy data from GetEnergyMicrojoules function: %v ", err)
		}
		if expectedEnergyMetrics[iz] != float64(microJoules) {
			t.Fatalf(
				"Expected energy value %f: Got: %f ",
				expectedEnergyMetrics[iz],
				float64(microJoules),
			)
		}
	}
}
