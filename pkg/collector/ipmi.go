// Taken from prometheus-community/ipmi_exporter/blob/master/collector_ipmi.go
// DCMI spec (old) https://www.intel.com/content/dam/www/public/us/en/documents/technical-specifications/dcmi-v1-5-rev-spec.pdf

//go:build !noimpi
// +build !noimpi

package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/batchjob_monitoring/internal/helpers"
	"github.com/prometheus/client_golang/prometheus"
)

const ipmiCollectorSubsystem = "ipmi_dcmi"

type impiCollector struct {
	logger          log.Logger
	execMode        string
	wattsMetricDesc *prometheus.Desc
}

var (
	ipmiDcmiCmd = kingpin.Flag(
		"collector.ipmi.dcmi.cmd",
		"IPMI DCMI command to get system power statistics. Use full path to executables.",
	).Default("/usr/sbin/ipmi-dcmi --get-system-power-statistics").String()
	// ipmiDcmiExecAsRoot = kingpin.Flag(
	// 	"collector.ipmi.dcmi.exec.run.as.root",
	// 	"Execute IPMI DCMI command as root. This requires the current process to have CAP_SET(UID,GID) capabilities.",
	// ).Default("false").Bool()
	// ipmiDcmiExecWithSudo = kingpin.Flag(
	// 	"collector.ipmi.dcmi.exec.run.with.sudo",
	// 	"Execute IPMI DCMI command with sudo. This requires the current has sudo privileges on command set in --collector.ipmi.dcmi.cmd.",
	// ).Default("false").Bool()
	ipmiDCMIPowerMeasurementRegex = regexp.MustCompile(
		`^Power Measurement\s*:\s*(?P<value>Active|Not\sAvailable).*`,
	)
	ipmiDCMICurrentPowerRegex = regexp.MustCompile(
		`^Current Power\s*:\s*(?P<value>[0-9.]*)\s*Watts.*`,
	)
)

func init() {
	registerCollector(ipmiCollectorSubsystem, defaultEnabled, NewIpmiCollector)
}

// NewIpmiCollector returns a new Collector exposing IMPI DCMI power metrics.
func NewIpmiCollector(logger log.Logger) (Collector, error) {

	wattsMetricDesc := prometheus.NewDesc(
		prometheus.BuildFQName(namespace, ipmiCollectorSubsystem, "watts_total"),
		"Current Power consumption in watts", []string{}, nil,
	)

	var execMode string = ""

	// Split command
	cmdSlice := strings.Split(*ipmiDcmiCmd, " ")

	// Verify if running ipmiDcmiCmd works
	if _, err := helpers.Execute(cmdSlice[0], cmdSlice[1:], logger); err == nil {
		execMode = "native"
		goto outside
	}

	// If ipmiDcmiCmd failed to run and if sudo is not already present in command,
	// add sudo to command and execute. If current user has sudo rights it will be a success
	if cmdSlice[0] != "sudo" {
		if _, err := helpers.ExecuteWithTimeout("sudo", cmdSlice, 2, logger); err == nil {
			execMode = "sudo"
			goto outside
		}
	}

	// As last attempt, run the command as root user by forking subprocess
	// as root. If there is setuid cap on the process, it will be a success
	if _, err := helpers.ExecuteAs(cmdSlice[0], cmdSlice[1:], 0, 0, logger); err == nil {
		execMode = "cap"
		goto outside
	}

outside:
	collector := impiCollector{
		logger:          logger,
		execMode:        execMode,
		wattsMetricDesc: wattsMetricDesc,
	}
	return &collector, nil
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
	var stdOut []byte
	var err error

	// Execute ipmi-dcmi command
	cmdSlice := strings.Split(*ipmiDcmiCmd, " ")
	if c.execMode == "cap" {
		stdOut, err = helpers.ExecuteAs(cmdSlice[0], cmdSlice[1:], 0, 0, c.logger)
	} else if c.execMode == "sudo" {
		stdOut, err = helpers.ExecuteWithTimeout("sudo", cmdSlice, 1, c.logger)
	} else if c.execMode == "native" {
		stdOut, err = helpers.Execute(cmdSlice[0], cmdSlice[1:], c.logger)
	} else {
		err = fmt.Errorf("Current process do not have permissions to execute %s", *ipmiDcmiCmd)
	}
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to collect IPMI DCMI data", "error", err)
		return err
	}

	// Parse power consumption from output
	currentPowerConsumption, err := c.getCurrentPowerConsumption(stdOut)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to parse IPMI DCMI command output", "error", err)
		return err
	}

	// Returned value negative == Power Measurement is not avail
	if currentPowerConsumption > -1 {
		ch <- prometheus.MustNewConstMetric(c.wattsMetricDesc, prometheus.CounterValue, float64(currentPowerConsumption))
	}
	return nil
}

// Get current power consumption
func (c *impiCollector) getCurrentPowerConsumption(ipmiOutput []byte) (float64, error) {
	// Check for Power Measurement are avail
	value, err := getValue(ipmiOutput, ipmiDCMIPowerMeasurementRegex)
	if err != nil {
		return -1, err
	}

	// When Power Measurement in 'Active' state - we can get watts
	if value == "Active" {
		value, err := getValue(ipmiOutput, ipmiDCMICurrentPowerRegex)
		if err != nil {
			return -1, err
		}
		return strconv.ParseFloat(value, 64)
	}
	return -1, nil
}
