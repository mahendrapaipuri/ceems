package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"regexp"
)

var (
	metricNameRegex = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)
)

// Check if file exists
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// SanitizeMetricName sanitize the given metric name by replacing invalid characters by underscores.
//
// OpenMetrics and the Prometheus exposition format require the metric name
// to consist only of alphanumericals and "_", ":" and they must not start
// with digits. Since colons in MetricFamily are reserved to signal that the
// MetricFamily is the result of a calculation or aggregation of a general
// purpose monitoring system, colons will be replaced as well.
//
// Note: If not subsequently prepending a namespace and/or subsystem (e.g.,
// with prometheus.BuildFQName), the caller must ensure that the supplied
// metricName does not begin with a digit.
func SanitizeMetricName(metricName string) string {
	return metricNameRegex.ReplaceAllString(metricName, "_")
}

// Load cgroups v2 metrics from a given path
func LoadCgroupsV2Metrics(
	name string,
	cgroupfsPath string,
	controllers []string,
) (map[string]float64, error) {
	data := make(map[string]float64)

	for _, fName := range controllers {
		contents, err := os.ReadFile(filepath.Join(cgroupfsPath, name, fName))
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
