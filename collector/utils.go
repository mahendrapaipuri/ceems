//go:build !nostat
// +build !nostat

package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Load cgroups v2 metrics from a given path
func LoadCgroupsV2Metrics(name string, controllers []string) (map[string]float64, error) {
	data := make(map[string]float64)

	for _, fName := range controllers {
		contents, err := os.ReadFile(filepath.Join(*cgroupfsPath, name, fName))
		if err != nil {
			return data, err
		}
		for _, line := range strings.Split(string(contents), "\n") {
			// Some of the above have a single value and others have a "data_name 123"
			parts := strings.Fields(line)
			indName := fName
			indData := 0
			if len(parts) == 1 || len(parts) == 2 {
				if len(parts) == 2 {
					indName += "." + parts[0]
					indData = 1
				}
				if parts[indData] == "max" {
					data[indName] = -1.0
				} else {
					f, err := strconv.ParseFloat(parts[indData], 64)
					if err == nil {
						data[indName] = f
					} else {
						return data, err
					}
				}
			}
		}
	}
	return data, nil
}
