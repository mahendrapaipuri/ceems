//go:build !nohwmon
// +build !nohwmon

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sys/unix"
)

// Nicked a lot of utility functions from node_exporter

const (
	hwmonCollectorSubSystem = "hwmon"
)

var (
	hwmonInvalidMetricChars = regexp.MustCompile("[^a-z0-9:_]")
	hwmonFilenameFormat     = regexp.MustCompile(`^(?P<type>[^0-9]+)(?P<id>[0-9]*)?(_(?P<property>.+))?$`)
	hwmonSensorTypes        = []string{"power", "energy"}
	hwmonSensorProperties   = []string{"input_lowest", "input_highest", "average", "input"}
)

func init() {
	RegisterCollector(hwmonCollectorSubSystem, defaultDisabled, NewHwmonCollector)
}

type hwmon struct {
	dir      string
	sensors  []hwmonSensor
	name     string
	chipName string
}

type hwmonSensor struct {
	sensorType     string
	sensorProperty string
	sensorNum      int
	sensorFile     string
}

type hwmonCollector struct {
	logger     *slog.Logger
	hostname   string
	metricDesc map[string]*prometheus.Desc
	monitors   []*hwmon
}

// NewHwmonCollector returns a new Collector exposing /sys/class/hwmon stats
// (similar to lm-sensors).
func NewHwmonCollector(logger *slog.Logger) (Collector, error) {
	// Initialize metricDesc map
	metricDesc := make(map[string]*prometheus.Desc, 5)

	metricDesc["power_input"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, hwmonCollectorSubSystem, "power_current_watts"),
		"Current Power consumption in watts", []string{"hostname", "sensor", "chip", "chip_name"}, nil,
	)
	metricDesc["power_input_lowest"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, hwmonCollectorSubSystem, "power_min_watts"),
		"Minimum Power consumption in watts", []string{"hostname", "sensor", "chip", "chip_name"}, nil,
	)
	metricDesc["power_input_highest"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, hwmonCollectorSubSystem, "power_max_watts"),
		"Maximum Power consumption in watts", []string{"hostname", "sensor", "chip", "chip_name"}, nil,
	)
	metricDesc["power_average"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, hwmonCollectorSubSystem, "power_avg_watts"),
		"Average Power consumption in watts", []string{"hostname", "sensor", "chip", "chip_name"}, nil,
	)
	metricDesc["energy_input"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, hwmonCollectorSubSystem, "energy_current_joules"),
		"Current Energy consumption in joules", []string{"hostname", "sensor", "chip", "chip_name"}, nil,
	)

	hwmonCollector := &hwmonCollector{
		logger:     logger,
		metricDesc: metricDesc,
		hostname:   hostname,
	}

	// Discover monitors
	if err := hwmonCollector.discoverMonitors(); err != nil {
		logger.Error("Failed to discover power and/or energy hwmon", "err", err)

		return nil, err
	}

	return hwmonCollector, nil
}

// Update updates the metrics channel with hwmon metrics.
func (c *hwmonCollector) Update(ch chan<- prometheus.Metric) error {
	// Loop over all available hwmons
	for _, mon := range c.monitors {
		for _, sensor := range mon.sensors {
			if val := readSensorValue(sensor.sensorFile); val > 0 {
				metricKey := fmt.Sprintf("%s_%s", sensor.sensorType, sensor.sensorProperty)

				// sensorType and sensorProperties are standardised and they **should**
				// have different names in theory. So, metricDesc should never return a nil
				// pointer. But just in case...
				if desc, ok := c.metricDesc[metricKey]; ok {
					ch <- prometheus.MustNewConstMetric(
						desc,
						prometheus.GaugeValue,
						val/1000000.0,
						c.hostname,
						fmt.Sprintf("%s%d", sensor.sensorType, sensor.sensorNum),
						mon.name,
						mon.chipName,
					)
				}
			}
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *hwmonCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", hwmonCollectorSubSystem)

	return nil
}

// discoverMonitors walks through all the folders in /sys/class/hwmon and returns
// power and energy monitors, when available.
func (c *hwmonCollector) discoverMonitors() error {
	// Step 1: scan /sys/class/hwmon, resolve all symlinks and call
	//         make sensors list
	hwmonPathName := filepath.Join(sysFilePath("class"), "hwmon")

	hwmonFiles, err := os.ReadDir(hwmonPathName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.logger.Debug("hwmon collector metrics are not available for this system")

			return ErrNoData
		}

		return err
	}

	for _, hwDir := range hwmonFiles {
		hwmonXPathName := filepath.Join(hwmonPathName, hwDir.Name())

		fileInfo, err := os.Lstat(hwmonXPathName)
		if err != nil {
			continue
		}

		if fileInfo.Mode()&os.ModeSymlink > 0 {
			fileInfo, err = os.Stat(hwmonXPathName)
			if err != nil {
				continue
			}
		}

		if !fileInfo.IsDir() {
			continue
		}

		if mon, err := getHwmon(hwmonXPathName); err == nil && mon != nil {
			c.monitors = append(c.monitors, mon)
		}
	}

	// Check if we discovered any monitors at all
	if len(c.monitors) == 0 {
		return errors.New("power or energy hwmon are not available for this system")
	}

	return nil
}

// getHwmon returns a list of relevant (power and energy) sensors and monitor metadata.
func getHwmon(dir string) (*hwmon, error) {
	hwmonName, err := hwmonName(dir)
	if err != nil {
		return nil, err
	}

	sensors, err := collectSensors(dir)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(filepath.Join(dir, "device")); err == nil {
		s, err := collectSensors(filepath.Join(dir, "device"))
		if err != nil {
			return nil, err
		}

		sensors = append(sensors, s...)
	}

	// If no sensors found, return
	if len(sensors) == 0 {
		return nil, errors.New("no sensors found")
	}

	hwmon := &hwmon{dir: dir, name: hwmonName, sensors: sensors}

	if hwmonChipName, err := hwmonHumanReadableChipName(dir); err == nil {
		hwmon.chipName = hwmonChipName
	}

	return hwmon, nil
}

// hwmonName returns the sensor name.
func hwmonName(dir string) (string, error) {
	// generate a name for a sensor path
	// sensor numbering depends on the order of linux module loading and
	// is thus unstable.
	// However the path of the device has to be stable:
	// - /sys/devices/<bus>/<device>
	// Some hardware monitors have a "name" file that exports a human
	// readable name that can be used.
	// human readable names would be bat0 or coretemp, while a path string
	// could be platform_applesmc.768
	// preference 1: construct a name based on device name, always unique
	devicePath, devErr := filepath.EvalSymlinks(filepath.Join(dir, "device"))
	if devErr == nil {
		devPathPrefix, devName := filepath.Split(devicePath)
		_, devType := filepath.Split(strings.TrimRight(devPathPrefix, "/"))

		cleanDevName := cleanMetricName(devName)
		cleanDevType := cleanMetricName(devType)

		if cleanDevType != "" && cleanDevName != "" {
			return cleanDevType + "_" + cleanDevName, nil
		}

		if cleanDevName != "" {
			return cleanDevName, nil
		}
	}

	// preference 2: is there a name file
	sysnameRaw, nameErr := os.ReadFile(filepath.Join(dir, "name"))
	if nameErr == nil && string(sysnameRaw) != "" {
		cleanName := cleanMetricName(string(sysnameRaw))
		if cleanName != "" {
			return cleanName, nil
		}
	}

	// it looks bad, name and device don't provide enough information
	// return a hwmon[0-9]* name

	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}

	// take the last path element, this will be hwmonX
	_, name := filepath.Split(realDir)

	cleanName := cleanMetricName(name)
	if cleanName != "" {
		return cleanName, nil
	}

	return "", errors.New("Could not derive a monitoring name for " + dir)
}

// hwmonHumanReadableChipName is similar to the methods in hwmonName, but with
// different precedences -- we can allow duplicates here.
func hwmonHumanReadableChipName(dir string) (string, error) {
	sysnameRaw, nameErr := os.ReadFile(filepath.Join(dir, "name"))
	if nameErr != nil {
		return "", nameErr
	}

	if string(sysnameRaw) != "" {
		cleanName := cleanMetricName(string(sysnameRaw))
		if cleanName != "" {
			return cleanName, nil
		}
	}

	return "", errors.New("Could not derive a human-readable chip type for " + dir)
}

// parseSensorFilename splits a sensor name into <type><num>_<property>.
func parseSensorFilename(filename string) (bool, string, int, string) {
	var sensorType, sensorProperty string

	var sensorNum int

	matches := hwmonFilenameFormat.FindStringSubmatch(filename)
	if len(matches) == 0 {
		return false, sensorType, sensorNum, sensorProperty
	}

	for i, match := range hwmonFilenameFormat.SubexpNames() {
		if i >= len(matches) {
			return true, sensorType, sensorNum, sensorProperty
		}

		if match == "type" {
			sensorType = matches[i]
		}

		if match == "property" {
			sensorProperty = matches[i]
		}

		if match == "id" && len(matches[i]) > 0 {
			if num, err := strconv.Atoi(matches[i]); err == nil {
				sensorNum = num
			} else {
				return false, sensorType, sensorNum, sensorProperty
			}
		}
	}

	return true, sensorType, sensorNum, sensorProperty
}

// collectSensors collects power and/or energy sensors in the current monitor directory.
func collectSensors(dir string) ([]hwmonSensor, error) {
	sensorFiles, dirError := os.ReadDir(dir)
	if dirError != nil {
		return nil, dirError
	}

	var sensors []hwmonSensor

	for _, file := range sensorFiles {
		filename := file.Name()

		ok, sensorType, sensorNum, sensorProperty := parseSensorFilename(filename)
		if !ok {
			continue
		}

		for _, t := range hwmonSensorTypes {
			for _, p := range hwmonSensorProperties {
				if t == sensorType && p == sensorProperty {
					sensor := hwmonSensor{
						sensorType:     sensorType,
						sensorProperty: sensorProperty,
						sensorNum:      sensorNum,
						sensorFile:     filepath.Join(dir, file.Name()),
					}
					sensors = append(sensors, sensor)

					break
				}
			}
		}
	}

	return sensors, nil
}

// cleanMetricName removes any invalid characters from hwmon name and replaces them
// with underscore.
func cleanMetricName(name string) string {
	lower := strings.ToLower(name)
	replaced := hwmonInvalidMetricChars.ReplaceAllLiteralString(lower, "_")
	cleaned := strings.Trim(replaced, "_")

	return cleaned
}

// readSensorValues reads the value of sensor file.
func readSensorValue(file string) float64 {
	raw, err := sysReadFile(file)
	if err != nil {
		return 0
	}

	if parsedValue, err := strconv.ParseFloat(strings.Trim(string(raw), "\n"), 64); err == nil {
		return parsedValue
	}

	return 0
}

// sysReadFile is a simplified os.ReadFile that invokes syscall.Read directly.
func sysReadFile(file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// On some machines, hwmon drivers are broken and return EAGAIN.  This causes
	// Go's os.ReadFile implementation to poll forever.
	//
	// Since we either want to read data or bail immediately, do the simplest
	// possible read using system call directly.
	b := make([]byte, 128)

	n, err := unix.Read(int(f.Fd()), b)
	if err != nil {
		return nil, err
	}

	if n < 0 {
		return nil, fmt.Errorf("failed to read file: %q, read returned negative bytes value: %d", file, n)
	}

	return b[:n], nil
}
