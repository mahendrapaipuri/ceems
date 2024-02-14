//go:build !nomeminfo
// +build !nomeminfo

package collector

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	memInfoSubsystem = "meminfo"
)

type meminfoCollector struct {
	logger   log.Logger
	hostname string
}

var meminfoAllStatistics = CEEMSExporterApp.Flag(
	"collector.meminfo.all.stats",
	"Enable collecting all meminfo stats (default is disabled).",
).Default("false").Bool()

func init() {
	RegisterCollector(memInfoSubsystem, defaultEnabled, NewMeminfoCollector)
}

// NewMeminfoCollector returns a new Collector exposing memory stats.
func NewMeminfoCollector(logger log.Logger) (Collector, error) {
	return &meminfoCollector{
		logger:   logger,
		hostname: hostname,
	}, nil
}

// Update calls (*meminfoCollector).getMemInfo to get the platform specific
// memory metrics.
func (c *meminfoCollector) Update(ch chan<- prometheus.Metric) error {
	var metricType prometheus.ValueType
	memInfo, err := c.getMemInfo()
	if err != nil {
		return fmt.Errorf("couldn't get meminfo: %w", err)
	}
	level.Debug(c.logger).Log("msg", "Set node_mem", "memInfo", fmt.Sprintf("%v", memInfo))

	// Export only MemTotal, MemFree and MemAvailable fields if meminfoAllStatistics is false
	var memInfoStats map[string]float64
	if *meminfoAllStatistics {
		memInfoStats = memInfo
	} else {
		memInfoStats = map[string]float64{
			"MemTotal_bytes":     memInfo["MemTotal_bytes"],
			"MemFree_bytes":      memInfo["MemFree_bytes"],
			"MemAvailable_bytes": memInfo["MemAvailable_bytes"],
		}
	}
	for k, v := range memInfoStats {
		if strings.HasSuffix(k, "_total") {
			metricType = prometheus.CounterValue
		} else {
			metricType = prometheus.GaugeValue
		}
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(Namespace, memInfoSubsystem, k),
				fmt.Sprintf("Memory information field %s.", k),
				[]string{"hostname"}, nil,
			),
			metricType, v, c.hostname,
		)
	}
	return nil
}

// Get memory info from /proc/meminfo
func (c *meminfoCollector) getMemInfo() (map[string]float64, error) {
	file, err := os.Open(procFilePath("meminfo"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseMemInfo(file)
}

// Parse /proc/meminfo file and get memory info
func parseMemInfo(r io.Reader) (map[string]float64, error) {
	var (
		memInfo = map[string]float64{}
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		// Workaround for empty lines occasionally occur in CentOS 6.2 kernel 3.10.90.
		if len(parts) == 0 {
			continue
		}
		fv, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid value in meminfo: %w", err)
		}
		key := parts[0][:len(parts[0])-1] // remove trailing : from key
		// Active(anon) -> Active_anon
		key = reParens.ReplaceAllString(key, "_${1}")
		switch len(parts) {
		case 2: // no unit
		case 3: // has unit, we presume kB
			fv *= 1024
			key = key + "_bytes"
		default:
			return nil, fmt.Errorf("invalid line in meminfo: %s", line)
		}
		memInfo[key] = fv
	}

	return memInfo, scanner.Err()
}
