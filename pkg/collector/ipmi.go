// Taken from prometheus-community/ipmi_exporter/blob/master/collector_ipmi.go
// DCMI spec (old) https://www.intel.com/content/dam/www/public/us/en/documents/technical-specifications/dcmi-v1-5-rev-spec.pdf

//go:build !noimpi
// +build !noimpi

package collector

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/prometheus/client_golang/prometheus"
)

const ipmiCollectorSubsystem = "ipmi_dcmi"

type impiCollector struct {
	logger       log.Logger
	hostname     string
	execMode     string
	ipmiCmd      []string
	cachedMetric map[string]float64
	metricDesc   map[string]*prometheus.Desc
}

/*
	Expected output from DCMI spec
	Ref: https://www.intel.com/content/dam/www/public/us/en/documents/technical-specifications/dcmi-v1-5-rev-spec.pdf

	Different IPMI implementations report stats in different formats:

	**FreeIPMI**:
	Command: `ipmi-dcmi --get-system-power-statistics`
	Output:
	----------------------------------------------------------------------
	Current Power                        : 164 Watts
	Minimum Power over sampling duration : 48 watts
	Maximum Power over sampling duration : 361 watts
	Average Power over sampling duration : 157 watts
	Time Stamp                           : 12/29/2023 - 08:58:00
	Statistics reporting time period     : 1473439000 milliseconds
	Power Measurement                    : Active
	----------------------------------------------------------------------

	**OpenIPMI**:
	Command: `ipmitool dcmi power reading`
	Output:
	----------------------------------------------------------------------

		Instantaneous power reading:                    50 Watts
		Minimum during sampling period:                  6 Watts
		Maximum during sampling period:                304 Watts
		Average power reading over sample period:       49 Watts
		IPMI timestamp:                           Thu Feb 15 08:36:01 2024
		Sampling period:                          00000001 Seconds.
		Power reading state is:                   activated

	----------------------------------------------------------------------

	**IPMIUtils**:
	Command: `ipmiutil dcmi power`
	Output:
	----------------------------------------------------------------------
	ipmiutil dcmi ver 3.17
	-- BMC version 6.10, IPMI version 2.0
	DCMI Version:                   1.5
	DCMI Power Management:          Supported
	DCMI System Interface Access:   Supported
	DCMI Serial TMode Access:       Supported
	DCMI Secondary LAN Channel:     Supported
	Current Power:                   49 Watts
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
	ipmiutil dcmi, completed successfully
	----------------------------------------------------------------------

	**Capmc**: Cray specific
	Command: `capmc get_system_power -w 600` Ref: https://github.com/Cray-HPE/hms-capmc/blob/release/csm-1.0/api/swagger.yaml
	Output:
	----------------------------------------------------------------------
	{
	"start_time":"2015-04-01 17:02:10",
	"avg":5942,
	"min":5748,
	"max":6132,
	"window_len":600,
	"e":0,
	"err_msg":""
	}
	----------------------------------------------------------------------
*/

var (
	ipmiDcmiCmd = CEEMSExporterApp.Flag(
		"collector.ipmi.dcmi.cmd",
		"IPMI DCMI command to get system power statistics. Use full path to executables.",
	).Default("").String()

	ipmiDcmiCmds = []string{
		"ipmi-dcmi --get-system-power-statistics",
		"ipmitool dcmi power reading",
		"ipmiutil dcmi power",
		// Cray capmc
		// Ref: https://cug.org/proceedings/cug2015_proceedings/includes/files/pap132.pdf
		"capmc get_system_power -w 600",
	}

	// IPMIUtil Ref: https://sourceforge.net/p/ipmiutil/code-git/ci/master/tree/util/idcmi.c#l343
	// FreeIPMI Ref: https://github.com/chu11/freeipmi-mirror/blob/f4057a93cbc11ecf4bfadb4bcd5d375f65f01f19/ipmi-dcmi/ipmi-dcmi.c#L1828-L1830
	// IPMITool Ref: https://github.com/ipmitool/ipmitool/blob/be11d948f89b10be094e28d8a0a5e8fb532c7b60/lib/ipmi_dcmi.c#L1447-L1451
	ipmiDCMIPowerMeasurementRegex = regexp.MustCompile(
		`^\s*(?:Power Measurement|Power reading state is)\s*:\s*(?P<value>[A|a]ctive|Not\sAvailable|not\sactive|activated|deactivated).*`,
	)
	ipmiDCMIPowerReadingRegexMap = map[string]*regexp.Regexp{
		"current": regexp.MustCompile(
			`^\s*(?:Current Power|Instantaneous power reading)\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
		"min": regexp.MustCompile(
			`^\s*(?:Minimum Power over sampling duration|Minimum during sampling period|Min Power over sample duration)\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
		"max": regexp.MustCompile(
			`^\s*(?:Maximum Power over sampling duration|Maximum during sampling period|Max Power over sample duration)\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
		"avg": regexp.MustCompile(
			`^\s*(?:Average Power over sampling duration|Average power reading over sample period|Avg Power over sample duration)\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
	}
)

func init() {
	RegisterCollector(ipmiCollectorSubsystem, defaultEnabled, NewIPMICollector)
}

// NewIPMICollector returns a new Collector exposing IMPI DCMI power metrics.
func NewIPMICollector(logger log.Logger) (Collector, error) {
	var execMode string

	// Initialize metricDesc map
	var metricDesc = make(map[string]*prometheus.Desc, 3)
	var cachedMetric = make(map[string]float64, 3)
	metricDesc["current"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "current_watts"),
		"Current Power consumption in watts", []string{"hostname"}, nil,
	)
	metricDesc["min"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "min_watts"),
		"Minimum Power consumption in watts", []string{"hostname"}, nil,
	)
	metricDesc["max"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "max_watts"),
		"Maximum Power consumption in watts", []string{"hostname"}, nil,
	)
	metricDesc["avg"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "avg_watts"),
		"Average Power consumption in watts", []string{"hostname"}, nil,
	)

	// If no IPMI command is provided, try to find one
	var cmdSlice []string
	if *ipmiDcmiCmd == "" {
		cmdSlice = findIPMICmd()
	} else {
		cmdSlice = strings.Split(*ipmiDcmiCmd, " ")
	}
	level.Debug(logger).Log(
		"msg", "Using IPMI command", "ipmi", strings.Join(cmdSlice, " "),
	)

	// Append to cmdSlice an empty string if it has len of 1
	// We dont want nil pointer references when we execute command
	if len(cmdSlice) == 1 {
		cmdSlice = append(cmdSlice, "")
	}

	// Verify if running ipmiDcmiCmd works
	if _, err := osexec.Execute(cmdSlice[0], cmdSlice[1:], nil, logger); err == nil {
		execMode = "native"
		goto outside
	}

	// If ipmiDcmiCmd failed to run and if sudo is not already present in command,
	// add sudo to command and execute. If current user has sudo rights it will be a success
	if cmdSlice[0] != "sudo" {
		if _, err := osexec.ExecuteWithTimeout("sudo", cmdSlice, 2, nil, logger); err == nil {
			execMode = "sudo"
			goto outside
		}
	}

	// As last attempt, run the command as root user by forking subprocess
	// as root. If there is setuid cap on the process, it will be a success
	if _, err := osexec.ExecuteAs(cmdSlice[0], cmdSlice[1:], 0, 0, nil, logger); err == nil {
		execMode = "cap"
		goto outside
	}

outside:
	collector := impiCollector{
		logger:       logger,
		hostname:     hostname,
		execMode:     execMode,
		ipmiCmd:      cmdSlice,
		metricDesc:   metricDesc,
		cachedMetric: cachedMetric,
	}
	return &collector, nil
}

// Find IPMI command from list of different IPMI implementations
func findIPMICmd() []string {
	for _, cmd := range ipmiDcmiCmds {
		cmdSlice := strings.Split(cmd, " ")
		if _, err := exec.LookPath(cmdSlice[0]); err == nil {
			return cmdSlice
		}
	}
	// Return a sane default and collector will handle even if bin does not exist
	return strings.Split(ipmiDcmiCmds[0], " ")
}

// Get value based on regex from IPMI output
func getValue(ipmiOutput []byte, regex *regexp.Regexp) (string, error) {
	for _, line := range strings.Split(string(ipmiOutput), "\n") {
		match := regex.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		for i, name := range regex.SubexpNames() {
			if name != "value" {
				continue
			}
			return match[i], nil
		}
	}
	return "", fmt.Errorf("could not find value in output: %s", string(ipmiOutput))
}

// Update implements Collector and exposes IPMI DCMI power related metrics.
func (c *impiCollector) Update(ch chan<- prometheus.Metric) error {
	// Get power consumption from IPMI
	// IPMI commands tend to fail frequently. If that happens we use last cached metric
	powerReadings, err := c.getPowerReadings()
	if err != nil {
		level.Error(c.logger).Log(
			"msg", "Failed to get power statistics from IPMI. Using last cached values",
			"err", err, "cached_metrics", fmt.Sprintf("%#v", c.cachedMetric),
		)
		powerReadings = c.cachedMetric
	}

	// Returned value 0 means Power Measurement is not avail
	for rType, rValue := range powerReadings {
		if rValue > 0 {
			ch <- prometheus.MustNewConstMetric(c.metricDesc[rType], prometheus.GaugeValue, float64(rValue), c.hostname)
			c.cachedMetric[rType] = rValue
		}
	}
	return nil
}

// Get current, min and max power readings
func (c *impiCollector) getPowerReadings() (map[string]float64, error) {
	// Execute IPMI command
	ipmiOutput, err := c.executeIPMICmd()
	if err != nil {
		return nil, err
	}

	// For capmc, we get JSON output, so treat it differently
	var values map[string]float64
	if filepath.Base(c.ipmiCmd[0]) == "capmc" {
		values, err = c.parseCapmcOutput(ipmiOutput)
	} else {
		// Parse IPMI output
		values, err = c.parseIPMIOutput(ipmiOutput)
	}
	if err != nil {
		return nil, err
	}
	return values, nil
}

// Parse current, min and max power readings for capmc output
func (c *impiCollector) parseCapmcOutput(stdOut []byte) (map[string]float64, error) {
	// Unmarshal JSON output
	var data map[string]interface{}
	if err := json.Unmarshal(stdOut, &data); err != nil {
		return nil, fmt.Errorf("capmc Power readings command failed")
	}

	// Check error code
	if data["e"].(float64) != 0 {
		return nil, fmt.Errorf("capmc Power readings not Active: %s", data["err_msg"].(string))
	}

	// Get power readings
	var powerReadings = make(map[string]float64, 3)
	for rType := range ipmiDCMIPowerReadingRegexMap {
		if value, ok := data[rType]; ok {
			powerReadings[rType] = value.(float64)
		}
	}

	// capmc does not return current power. So we use avg as proxy for current
	if powerReadings["avg"] > 0 {
		powerReadings["current"] = powerReadings["avg"]
	}
	return powerReadings, nil
}

// Parse current, min and max power readings for IPMI commands
func (c *impiCollector) parseIPMIOutput(stdOut []byte) (map[string]float64, error) {
	// Check for Power Measurement are avail
	value, err := getValue(stdOut, ipmiDCMIPowerMeasurementRegex)
	if err != nil {
		return nil, err
	}

	// When Power Measurement in 'Active' state - we can get watts
	var powerReadings = make(map[string]float64, 3)
	if value == "active" || value == "Active" || value == "activated" {
		// Get power readings
		for rType, regex := range ipmiDCMIPowerReadingRegexMap {
			if reading, err := getValue(stdOut, regex); err == nil {
				if readingValue, err := strconv.ParseFloat(reading, 64); err == nil {
					powerReadings[rType] = readingValue
				}
			}
		}
		return powerReadings, nil
	}
	return nil, fmt.Errorf("IPMI Power readings not Active")
}

// Execute IPMI command based
func (c *impiCollector) executeIPMICmd() ([]byte, error) {
	var stdOut []byte
	var err error

	// Execute found ipmi command
	if c.execMode == "cap" {
		stdOut, err = osexec.ExecuteAs(c.ipmiCmd[0], c.ipmiCmd[1:], 0, 0, nil, c.logger)
	} else if c.execMode == "sudo" {
		stdOut, err = osexec.ExecuteWithTimeout("sudo", c.ipmiCmd, 1, nil, c.logger)
	} else if c.execMode == "native" {
		stdOut, err = osexec.Execute(c.ipmiCmd[0], c.ipmiCmd[1:], nil, c.logger)
	} else {
		err = fmt.Errorf("current process do not have permissions to execute %s", strings.Join(c.ipmiCmd, " "))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to execute IPMI command: %s", err)
	}
	return stdOut, nil
}
