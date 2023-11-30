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
	"github.com/prometheus/client_golang/prometheus"

	utils "github.com/mahendrapaipuri/batchjob_monitoring/pkg/utils"
)

const ipmiCollectorSubsystem = "ipmi_dcmi"

type impiCollector struct {
	logger log.Logger

	wattsMetricDesc *prometheus.Desc
}

var (
	ipmiDcmiWrapperExec = kingpin.Flag("collector.ipmi.dcmi.wrapper.path", "Path to IPMI DCMI executable wrapper.").
				Default("ipmi-dcmi-wrapper").
				String()
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
	args := []string{""}
	stdOut, err := utils.Execute(*ipmiDcmiWrapperExec, args, c.logger)
	if err != nil {
		return err
	}
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
