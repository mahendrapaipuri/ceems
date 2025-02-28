//go:build !nonetdev
// +build !nonetdev

package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const (
	netdevSubsystem = "netdev"
)

var (
	netdevDeviceInclude = CEEMSExporterApp.Flag(
		"collector.netdev.device-include", "Regexp of net devices to include (mutually exclusive to device-exclude).",
	).String()
	netdevDeviceExclude = CEEMSExporterApp.Flag(
		"collector.netdev.device-exclude", "Regexp of net devices to exclude (mutually exclusive to device-include).",
	).String()
)

type netDevStats map[string]map[string]uint64

type deviceFilter struct {
	ignorePattern *regexp.Regexp
	acceptPattern *regexp.Regexp
}

func newDeviceFilter(ignoredPattern, acceptPattern string) deviceFilter {
	var f deviceFilter

	if ignoredPattern != "" {
		f.ignorePattern = regexp.MustCompile(ignoredPattern)
	}

	if acceptPattern != "" {
		f.acceptPattern = regexp.MustCompile(acceptPattern)
	}

	return f
}

// ignored returns whether the device should be ignored.
func (f *deviceFilter) ignored(name string) bool {
	return (f.ignorePattern != nil && f.ignorePattern.MatchString(name)) ||
		(f.acceptPattern != nil && !f.acceptPattern.MatchString(name))
}

type netdevCollector struct {
	logger       *slog.Logger
	fs           procfs.FS
	hostname     string
	deviceFilter deviceFilter
	metricDesc   map[string]*prometheus.Desc
}

func init() {
	RegisterCollector(netdevSubsystem, defaultDisabled, NewNetdevCollector)
}

// NewNetdevCollector returns a new Collector exposing node network stats.
func NewNetdevCollector(logger *slog.Logger) (Collector, error) {
	// Sanitize CLI args
	if *netdevDeviceExclude != "" && *netdevDeviceInclude != "" {
		return nil, errors.New("device-exclude & device-include are mutually exclusive")
	}

	// Make an instance of procfs
	fs, err := procfs.NewFS(*procfsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open procfs: %w", err)
	}

	// Make device filter
	deviceFilter := newDeviceFilter(*netdevDeviceExclude, *netdevDeviceInclude)

	// Metric descriptions
	metricDesc := map[string]*prometheus.Desc{
		"receive_bytes": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_bytes_total"),
			"Total received bytes by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_packets": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_packets_total"),
			"Total received packets by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_errors": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_errors_total"),
			"Total received errors by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_dropped": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_dropped_total"),
			"Total received dropped packets by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_fifo": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_fifo_total"),
			"Total received FIFO errors by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_frame": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_frame_total"),
			"Total received frames by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_compressed": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_compressed_total"),
			"Total received compressed packets by the network device", []string{"hostname", "device"}, nil,
		),
		"receive_multicast": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "receive_multicast_total"),
			"Total received multicast packets by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_bytes": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_bytes_total"),
			"Total transmitted bytes by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_packets": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_packets_total"),
			"Total transmitted packets by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_errors": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_errors_total"),
			"Total transmitted bytes by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_dropped": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_dropped_total"),
			"Total transmitted dropped packets by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_fifo": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_fifo_total"),
			"Total frame transmission errors FIFO by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_colls": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_colls_total"),
			"Total transmitted collisions by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_carrier": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_carrier_total"),
			"Total frame transmission errors errors due to loss of carrier by the network device", []string{"hostname", "device"}, nil,
		),
		"transmit_compressed": prometheus.NewDesc(
			prometheus.BuildFQName(Namespace, netdevSubsystem, "transmit_compressed_total"),
			"Total transmitted compressed packets by the network device", []string{"hostname", "device"}, nil,
		),
	}

	return &netdevCollector{
		logger:       logger,
		fs:           fs,
		metricDesc:   metricDesc,
		deviceFilter: deviceFilter,
		hostname:     hostname,
	}, nil
}

// Update updates metric channel with network stats.
func (c *netdevCollector) Update(ch chan<- prometheus.Metric) error {
	// Get stats
	netDevStats, err := c.procNetDevStats()
	if err != nil {
		return fmt.Errorf("couldn't get netstats: %w", err)
	}

	for dev, devStats := range netDevStats {
		for key, value := range devStats {
			if desc, ok := c.metricDesc[key]; ok {
				ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, float64(value), c.hostname, dev)
			}
		}
	}

	return nil
}

// Stop releases system resources used by the collector.
func (c *netdevCollector) Stop(_ context.Context) error {
	c.logger.Debug("Stopping", "collector", netdevSubsystem)

	return nil
}

// procNetDevStats returns the network stats from /proc/net/dev.
func (c *netdevCollector) procNetDevStats() (netDevStats, error) {
	metrics := netDevStats{}

	netDev, err := c.fs.NetDev()
	if err != nil {
		return metrics, fmt.Errorf("failed to parse /proc/net/dev: %w", err)
	}

	for _, stats := range netDev {
		name := stats.Name

		if c.deviceFilter.ignored(name) {
			c.logger.Debug("Ignoring device", "device", name)

			continue
		}

		metrics[name] = map[string]uint64{
			"receive_bytes":       stats.RxBytes,
			"receive_packets":     stats.RxPackets,
			"receive_errors":      stats.RxErrors,
			"receive_dropped":     stats.RxDropped,
			"receive_fifo":        stats.RxFIFO,
			"receive_frame":       stats.RxFrame,
			"receive_compressed":  stats.RxCompressed,
			"receive_multicast":   stats.RxMulticast,
			"transmit_bytes":      stats.TxBytes,
			"transmit_packets":    stats.TxPackets,
			"transmit_errors":     stats.TxErrors,
			"transmit_dropped":    stats.TxDropped,
			"transmit_fifo":       stats.TxFIFO,
			"transmit_colls":      stats.TxCollisions,
			"transmit_carrier":    stats.TxCarrier,
			"transmit_compressed": stats.TxCompressed,
		}
	}

	return metrics, nil
}
