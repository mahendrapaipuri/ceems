// Package collector implements different collectors of the exporter
package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// Namespace defines the common namespace to be used by all metrics.
const Namespace = "ceems"

var (
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, "scrape", "collector_duration_seconds"),
		CEEMSExporterAppName+": Duration of a collector scrape.",
		[]string{"collector", "subcollector"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, "scrape", "collector_success"),
		CEEMSExporterAppName+": Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)
)

const (
	defaultEnabled  = true
	defaultDisabled = false
)

var (
	factories               = make(map[string]func(logger *slog.Logger) (Collector, error))
	initiatedCollectorsMtx  = sync.Mutex{}
	initiatedCollectors     = make(map[string]Collector)
	collectorState          = make(map[string]*bool)
	collectorCaps           = make([]cap.Value, 0) // Unique slice of all required caps of currently enabled collectors
	collectorReadPaths      = make([]string, 0)
	collectorReadWritePaths = make([]string, 0)
	forcedCollectors        = map[string]bool{} // collectors which have been explicitly enabled or disabled
)

// Collector is the interface a collector has to implement.
type Collector interface {
	// Get new metrics and expose them via prometheus registry.
	Update(ch chan<- prometheus.Metric) error
	// Stops each collector and cleans up system resources
	Stop(ctx context.Context) error
}

// ErrNoData indicates the collector found no data to collect, but had no other error.
var ErrNoData = errors.New("collector returned no data")

// IsNoDataError returns true if error is ErrNoData.
func IsNoDataError(err error) bool {
	return errors.Is(ErrNoData, err)
}

// RegisterCollector registers collector into collector factory.
func RegisterCollector(
	collector string,
	isDefaultEnabled bool,
	factory func(logger *slog.Logger) (Collector, error),
) {
	var helpDefaultState string
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	} else {
		helpDefaultState = "disabled"
	}

	flagName := "collector." + collector
	flagHelp := fmt.Sprintf("Enable the %s collector (default: %s).", collector, helpDefaultState)
	defaultValue := strconv.FormatBool(isDefaultEnabled)

	flag := CEEMSExporterApp.Flag(flagName, flagHelp).
		Default(defaultValue).
		Action(collectorFlagAction(collector)).
		Bool()
	collectorState[collector] = flag

	factories[collector] = factory
}

// CEEMSCollector implements the prometheus.Collector interface.
type CEEMSCollector struct {
	Collectors map[string]Collector
	logger     *slog.Logger
}

// DisableDefaultCollectors sets the collector state to false for all collectors which
// have not been explicitly enabled on the command line.
func DisableDefaultCollectors() {
	for c := range collectorState {
		if _, ok := forcedCollectors[c]; !ok {
			*collectorState[c] = false
		}
	}
}

// collectorFlagAction generates a new action function for the given collector
// to track whether it has been explicitly enabled or disabled from the command line.
// A new action function is needed for each collector flag because the ParseContext
// does not contain information about which flag called the action.
// See: https://github.com/alecthomas/kingpin/issues/294
func collectorFlagAction(collector string) func(ctx *kingpin.ParseContext) error {
	return func(ctx *kingpin.ParseContext) error {
		forcedCollectors[collector] = true

		return nil
	}
}

// NewCEEMSCollector creates a new CEEMSCollector.
func NewCEEMSCollector(logger *slog.Logger) (*CEEMSCollector, error) {
	f := make(map[string]bool)

	collectors := make(map[string]Collector)

	initiatedCollectorsMtx.Lock()
	defer initiatedCollectorsMtx.Unlock()

	for key, enabled := range collectorState {
		if !*enabled || (len(f) > 0 && !f[key]) {
			continue
		}

		if collector, ok := initiatedCollectors[key]; ok {
			collectors[key] = collector
		} else {
			collector, err := factories[key](logger.With("collector", key))
			if err != nil {
				return nil, err
			}

			collectors[key] = collector
			initiatedCollectors[key] = collector
		}
	}

	// Log all enabled collectors
	logger.Info("Enabled collectors")

	coll := []string{}
	for n := range collectors {
		coll = append(coll, n)
	}

	sort.Strings(coll)

	for _, coll := range coll {
		logger.Info(coll)
	}

	// // Remove duplicates of caps
	// for subSystem, caps := range collectorCaps {
	// 	slices.Sort(caps)
	// 	uniqueCaps := slices.Compact(caps)
	// 	collectorCaps[subSystem] = uniqueCaps

	// 	allCollectorCaps = append(allCollectorCaps, caps...)
	// }

	// slices.Sort(allCollectorCaps)
	// allCollectorCaps = slices.Compact(allCollectorCaps)

	return &CEEMSCollector{Collectors: collectors, logger: logger}, nil
}

// Describe implements the prometheus.Collector interface.
func (n CEEMSCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

// Collect implements the prometheus.Collector interface.
func (n CEEMSCollector) Collect(ch chan<- prometheus.Metric) {
	wg := sync.WaitGroup{}
	wg.Add(len(n.Collectors))

	for name, c := range n.Collectors {
		go func(name string, c Collector) {
			execute(name, c, ch, n.logger)
			wg.Done()
		}(name, c)
	}

	wg.Wait()
}

// Close stops all the collectors and release system resources.
func (n CEEMSCollector) Close(ctx context.Context) error {
	var errs error

	for _, c := range n.Collectors {
		if err := c.Stop(ctx); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

// execute collects the metrics from each collector.
func execute(name string, c Collector, ch chan<- prometheus.Metric, logger *slog.Logger) {
	begin := time.Now()
	err := c.Update(ch)
	duration := time.Since(begin)

	var success float64

	if err != nil {
		if IsNoDataError(err) {
			logger.Debug("collector returned no data", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		} else {
			logger.Error("collector failed", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		}

		success = 0
	} else {
		logger.Debug("collector succeeded", "name", name, "duration_seconds", duration.Seconds())

		success = 1
	}
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), name, "")
	ch <- prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, name)
}

// subCollectorDuration updates the metrics channel with duration of each sub collector.
func subCollectorDuration(mainColl string, subColl string, start time.Time, ch chan<- prometheus.Metric) {
	duration := time.Since(start)

	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), mainColl, subColl)
}
