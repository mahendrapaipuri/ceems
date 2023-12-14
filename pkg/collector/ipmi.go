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
	logger log.Logger

	wattsMetricDesc *prometheus.Desc
}

var (
	ipmiDcmiExec = kingpin.Flag(
		"collector.ipmi.dcmi.exec.path",
		"Path to IPMI DCMI executable.",
	).Default("ipmi-dcmi-wrapper").String()
	ipmiDcmiExecAsRoot = kingpin.Flag(
		"collector.ipmi.dcmi.exec.run.as.root",
		"Execute IPMI DCMI command as root. This requires batchjob_exporter to run as root or to have appropriate capabilities (cap_setuid).",
	).Default("false").Bool()
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

	collector := impiCollector{
		logger:          logger,
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
	args := []string{"--get-system-power-statistics"}
	var stdOut []byte
	var err error

	// Execute ipmi-dcmi command
	if *ipmiDcmiExecAsRoot {
		stdOut, err = helpers.ExecuteAs(*ipmiDcmiExec, args, 0, 0, c.logger)
	} else {
		stdOut, err = helpers.Execute(*ipmiDcmiExec, args, c.logger)
	}
	if err != nil {
		return err
	}

	// Parse power consumption from output
	currentPowerConsumption, err := c.getCurrentPowerConsumption(stdOut)
	if err != nil {
		level.Error(c.logger).Log("msg", "Failed to collect IPMI DCMI data", "error", err)
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
