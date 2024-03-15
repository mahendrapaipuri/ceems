package updater

import (
	"fmt"
	"maps"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/helper"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/prometheus/common/model"
)

// Size of each chunk of UUIDs to make request to TSDB
const (
	chunkSize = 1000
)

// Embed TSDB struct into our TSDBUpdater struct
type tsdbUpdater struct {
	*tsdb.TSDB
}

// Mutex lock
var (
	tsdbWebURL = base.CEEMSServerApp.Flag(
		"tsdb.web.url",
		"TSDB URL (Prometheus/Victoria Metrics). If basic auth is enabled consider providing this URL using environment variable TSDB_WEBURL.",
	).Default(os.Getenv("TSDB_WEBURL")).String()
	tsdbWebSkipTLSVerify = base.CEEMSServerApp.Flag(
		"tsdb.web.skip-tls-verify",
		"Whether to skip TLS verification when using self signed certificates (default is false).",
	).Default("false").Bool()
	cutoffDurationString = base.CEEMSServerApp.Flag(
		"tsdb.data.cutoff.duration",
		"Compute units (Batch jobs, VMs, Pods) with wall time less than this period will be ignored. By default none will be ignored. Units Supported: y, w, d, h, m, s, ms.",
	).Default("0s").String()
	purgeDataTS = base.CEEMSServerApp.Flag(
		"tsdb.data.purge.ts",
		"Ignored compute units (Batch jobs, VMs, Pods) will be purged from the TSDB. Admin API must be enabled in TSDB.",
	).Default("false").Bool()
	metricLock = sync.RWMutex{}
	cutoff     = time.Duration(0 * time.Second)
)

// Register TSDB updater
// tsdb will estimate time averaged metrics and update units struct
// It will also remove ignored units time series
func init() {
	RegisterUpdater("tsdb", false, NewTSDBUpdater)
}

// NewTSDBUpdater create a new updater interface
func NewTSDBUpdater(logger log.Logger) (Updater, error) {
	tsdb, err := tsdb.NewTSDB(*tsdbWebURL, *tsdbWebSkipTLSVerify, logger)
	if err != nil {
		return nil, err
	}

	// Update cutoff duration
	if *cutoffDurationString != "" {
		cutoffDuration, err := model.ParseDuration(*cutoffDurationString)
		if err != nil {
			panic(fmt.Sprintf("failed to parse --tsdb.data.cutoff.duration flag: %s", err))
		}
		cutoff = time.Duration(cutoffDuration)
	}
	return &tsdbUpdater{tsdb}, nil
}

// Return formatted query string after replacing placeholders
func (t *tsdbUpdater) queryString(
	query string,
	uuids string,
	duration time.Duration,
	scrapeInterval time.Duration,
	rateInterval time.Duration,
) string {
	return fmt.Sprintf(
		strings.TrimLeft(query, "\n"),
		uuids,
		rateInterval,
		duration,
		scrapeInterval,
		scrapeInterval.Milliseconds(),
	)
}

// Get time averaged value of each metric identified by label uuid
func (t *tsdbUpdater) fetchAggMetrics(
	queryTime time.Time,
	duration time.Duration,
	uuids string,
) map[string]tsdb.Metric {
	var aggMetrics = make(map[string]tsdb.Metric, len(aggMetricQueries))

	// Get rate and scrape intervals
	rateInterval := t.RateInterval()
	scrapeInterval := t.ScrapeInterval()

	// If duration is less than rateInterval bail
	if duration < rateInterval {
		return aggMetrics
	}

	// If duration is more than or equal to 1 day, double scrape and rate intervals
	// to reduce number of data points Prometheus has to process
	if duration >= time.Duration(24*time.Hour) {
		scrapeInterval = 2 * scrapeInterval
		rateInterval = 2 * rateInterval
	}

	// Start a wait group
	var wg sync.WaitGroup
	wg.Add(len(aggMetricQueries))

	// Loop over aggMetricQueries map and make queries
	for name, query := range aggMetricQueries {
		go func(n string, q string) {
			var aggMetric tsdb.Metric
			var err error
			tsdbQuery := t.queryString(q, uuids, duration, scrapeInterval, rateInterval)
			if aggMetric, err = t.Query(tsdbQuery, queryTime); err != nil {
				level.Error(t.Logger).Log(
					"msg", "Failed to fetch metrics from TSDB", "metric", n, "duration",
					duration, "scrape_int", scrapeInterval, "rate_int", rateInterval,
					"err", err,
				)
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
func (t *tsdbUpdater) Update(startTime time.Time, endTime time.Time, units []models.Unit) []models.Unit {
	// Bail if TSDB is unavailable or there are no units to update
	if !t.Available() || len(units) == 0 {
		return units
	}

	// We compute aggregate metrics only for this interval duration and
	// while updating DB we either sum or calculate cumulative average based
	// metric type
	duration := endTime.Sub(startTime).Truncate(time.Minute)

	var allUnitUUIDs = make([]string, len(units))

	// Loop over all units and find earliest start time of a unit
	j := 0
	for i := 0; i < len(units); i++ {
		// If unit is empty struct ignore
		if units[i].UUID == "" {
			continue
		}
		allUnitUUIDs[j] = units[i].UUID
		j++
	}

	// Chunk UUIDs into slices of 1000 so that we make TSDB requests for each 1000 units
	// This is to safeguard against OOM errors due to a very large number of units
	// that can spread across big time interval
	uuidChunks := helper.ChunkBy(allUnitUUIDs[:j], chunkSize)

	var aggMetrics = make(map[string]tsdb.Metric)

	// Loop over each chunk
	for _, chunkUUIDs := range uuidChunks {
		// Get aggregate metrics of present chunk
		chunkAggMetrics := t.fetchAggMetrics(endTime, duration, strings.Join(chunkUUIDs, "|"))

		// Merge metrics map of each metric type. Metric map has uuid as key and hence
		// merging is safe as UUID is "unique" during the given update interval
		for name, metrics := range chunkAggMetrics {
			// If inner map has not been initialized yet, do it
			if aggMetrics[name] == nil {
				aggMetrics[name] = make(tsdb.Metric)
			}
			maps.Copy(aggMetrics[name], metrics)
		}
	}

	// Initialize ignored units slice
	var ignoredUnits []string

	// Update all units
	// NOTE: We can improve this by using reflect package by naming queries
	// after field names. That will allow us to dynamically look up struct
	// field using query name and set the properties.
	for i := 0; i < len(units); i++ {
		// Ignore units that ran for less than cutoffPeriod seconds and check if
		// unit has end time stamp. If we decide to populate DB with running units,
		// EndTS will be zero as we cannot convert unknown time into time stamp.
		// Check if we EndTS is not zero before ignoring unit. If it is zero, it means
		// it must be RUNNING unit
		if units[i].EndedAtTS > 0 && units[i].ElapsedRaw < int64(cutoff.Seconds()) {
			ignoredUnits = append(
				ignoredUnits,
				units[i].UUID,
			)
			units[i].Ignore = 1
		}

		// Update with CPU metrics
		if metric, mExists := aggMetrics["cpuUsage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveCPUUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuMemUsage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveCPUMemUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuEnergyUsage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalCPUEnergyUsage = value
			}
		}
		if metric, mExists := aggMetrics["cpuEmissions"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalCPUEmissions = value
			}
		}

		// Update with GPU metrics
		if metric, mExists := aggMetrics["gpuUsage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveGPUUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuMemUsage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveGPUMemUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuEnergyUsage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalGPUEnergyUsage = value
			}
		}
		if metric, mExists := aggMetrics["gpuEmissions"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalGPUEmissions = value
			}
		}
	}

	// Finally delete time series corresponding to ignoredUnits
	if *purgeDataTS {
		if err := t.deleteTimeSeries(startTime, endTime, ignoredUnits); err != nil {
			level.Error(t.Logger).
				Log("msg", "Failed delete ignored units' time series in TSDB", "err", err)
		}
	}
	return units
}

// Delete time series data of ignored units
func (t *tsdbUpdater) deleteTimeSeries(startTime time.Time, endTime time.Time, unitUUIDs []string) error {
	// Check if there are any units to ignore. If there aren't return immediately
	// We shouldnt make a API request to delete with empty units slice as TSDB will
	// match all units during that period with uuid=~"" matcher
	if len(unitUUIDs) == 0 {
		return nil
	}
	level.Debug(t.Logger).Log("units_ignored", len(unitUUIDs))

	/*
		We should give start and end query params as well. If not, TSDB has to look over
		"all" time blocks (potentially 1000s or more) and try to find the series.
		The thing is the time series data of these "ignored" units should be head block
		as they have started and finished very "recently".

		Imagine we are updating units data for every 15 min and we would like to ignore units
		that have wall time less than 10 min. If we are updating units from, say 10h-10h-15,
		the units that have been ignored cannot start earlier than 9h50 to have finished within
		10h-10h15 window. So, all these time series must be in the head block of TSDB and
		we should provide start and end query params corresponding to
		9h50 (lastupdatetime - ignored unit duration) and current time, respectively. This
		will help TSDB to narrow the search to head block and hence deletion of time series
		will be easy as they are potentially not yet persisted to disk.
	*/
	start := startTime.Add(cutoff)
	end := endTime

	// Matcher must be of format "{uuid=~"<regex>"}"
	// Ref: https://ganeshvernekar.com/blog/prometheus-tsdb-queries/
	//
	// Join them with | as delimiter. We will use regex match to match all series
	// with the label uuid=~"$unitids"
	allUUIDs := strings.Join(unitUUIDs, "|")
	matcher := fmt.Sprintf("{uuid=~\"%s\"}", allUUIDs)
	// Make a API request to delete data of ignored units
	return t.Delete(start, end, matcher)
}
