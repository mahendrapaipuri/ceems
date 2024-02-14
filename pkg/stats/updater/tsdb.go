package updater

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/stats/base"
	"github.com/mahendrapaipuri/ceems/pkg/stats/types"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
)

// Embed TSDB struct into our TSDBUpdater struct
type tsdbUpdater struct {
	*tsdb.TSDB
}

// Mutex lock
var (
	metricLock = sync.RWMutex{}
)

// Register TSDB aggregator updater
func init() {
	RegisterUpdater("tsdb-aggregator", false, NewTSDBUpdater)
}

// Factory to create a new updater interface
func NewTSDBUpdater(logger log.Logger) (Updater, error) {
	tsdb, err := tsdb.NewTSDB(base.TSDBWebURL, base.TSDBWebSkipTLSVerify, logger)
	if err != nil {
		return nil, err
	}
	return &tsdbUpdater{tsdb}, nil
}

// Return formatted query string after replacing placeholders
func (t *tsdbUpdater) queryString(query string, uuids string, maxDuration time.Duration) string {
	rateInterval := t.RateInterval()
	scrapeInterval := t.ScrapeInterval()
	return fmt.Sprintf(
		strings.TrimLeft(query, "\n"),
		uuids,
		rateInterval,
		maxDuration,
		scrapeInterval,
		scrapeInterval.Milliseconds(),
	)
}

// Get time averaged value of each metric identified by label uuid
func (t *tsdbUpdater) fetchAggMetrics(
	queryTime time.Time,
	maxDuration time.Duration,
	uuids string,
) map[string]tsdb.Metric {
	var aggMetrics = make(map[string]tsdb.Metric, len(aggMetricQueries))
	var err error

	// Start a wait group
	var wg sync.WaitGroup
	wg.Add(len(aggMetricQueries))

	// Loop over aggMetricQueries map and make queries
	for name, query := range aggMetricQueries {
		go func(n string, q string) {
			var aggMetric tsdb.Metric
			tsdbQuery := t.queryString(q, uuids, maxDuration)
			if aggMetric, err = t.Query(tsdbQuery, queryTime); err != nil {
				level.Error(t.Logger).Log("msg", "Failed to fetch metrics from TSDB", "metric", n, "err", err)
			} else {
				metricLock.Lock()
				aggMetrics[n] = aggMetric
				metricLock.Unlock()
			}
			wg.Done()
		}(name, query)
	}
	// Wait for all go routines
	wg.Wait()
	return aggMetrics
}

// Fetch unit metrics from TSDB and update UnitStat struct for each unit
func (t *tsdbUpdater) Update(queryTime time.Time, units []types.Unit) []types.Unit {
	// Check if TSDB is available
	if !t.Available() {
		return units
	}
	var minStartTime = queryTime.UnixMilli()
	var allUnitIds = make([]string, len(units))

	// Loop over all units and find earliest start time of a unit
	for i := 0; i < len(units); i++ {
		allUnitIds[i] = units[i].UUID
		if units[i].StartTS > 0 && minStartTime > units[i].StartTS {
			minStartTime = units[i].StartTS
		}
	}
	allUnitIdsExp := strings.Join(allUnitIds, "|")

	// Get max window from minStartTime to queryTime
	maxDuration := time.Duration((queryTime.UnixMilli() - minStartTime) * int64(time.Millisecond)).Truncate(time.Minute)

	// Get all aggregate metrics
	aggMetrics := t.fetchAggMetrics(queryTime, maxDuration, allUnitIdsExp)

	// Update all units
	// NOTE: We can improve this by using reflect package by naming queries
	// after field names. That will allow us to dynamically look up struct
	// field using query name and set the properties.
	for _, unit := range units {
		// Update with CPU metrics
		if metric, mExists := aggMetrics["cpuUsage"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.AveCPUUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuMemUsage"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.AveCPUMemUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuEnergyUsage"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.TotalCPUEnergyUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuEmissions"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.TotalCPUEmissions = value
			}
		}

		// Update with GPU metrics
		if metric, mExists := aggMetrics["gpuUsage"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.AveGPUUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuMemUsage"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.AveGPUMemUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuEnergyUsage"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.TotalGPUEnergyUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuEmissions"]; mExists {
			if value, exists := metric[unit.UUID]; exists {
				unit.TotalGPUEmissions = value
			}
		}
	}
	return units
}
