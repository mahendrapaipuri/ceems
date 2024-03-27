package updater

import (
	"fmt"
	"html/template"
	"maps"
	"math"
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
	"gopkg.in/yaml.v2"
)

// Size of each chunk of UUIDs to make request to TSDB
const (
	chunkSize = 1000
)

// tsdbUpdaterConfig contains the configuration file of tsdb updater
type tsdbUpdaterConfig struct {
	WebURL         string            `yaml:"web_url"`
	SkipTLSVerify  bool              `yaml:"skip_tls_verify"`
	CutoffDuration string            `yaml:"cut_off_duration"`
	Queries        map[string]string `yaml:"queries"`
	LabelsToDrop   []string          `yaml:"labels_to_drop"`
}

// Embed TSDB struct into our TSDBUpdater struct
type tsdbUpdater struct {
	*tsdb.TSDB
	config tsdbUpdaterConfig
}

// Mutex lock
var (
	tsdbConfigFile = base.CEEMSServerApp.Flag(
		"tsdb.config.file",
		"TSDB (Prometheus/Victoria Metrics) config file path.",
	).Default("").String()
	metricLock = sync.RWMutex{}
	cutoff     = time.Duration(0 * time.Second)
)

// Register TSDB updater
// tsdb will estimate time averaged metrics and update units struct
// It will also remove ignored units time series
func init() {
	RegisterUpdater("tsdb", false, NewTSDBUpdater)
}

// updaterConfig will read config file and create tsdbUpdaterConfig instance
func updaterConfig(filePath string) (*tsdbUpdaterConfig, error) {
	// Set a default config
	config := tsdbUpdaterConfig{
		WebURL:         "http://localhost:9090",
		SkipTLSVerify:  true,
		CutoffDuration: "0s",
	}

	// If no config file path provided, return default config
	if filePath == "" {
		return &config, nil
	}

	// Read config file
	configFile, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Update config from YAML file
	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// NewTSDBUpdater create a new updater interface
func NewTSDBUpdater(logger log.Logger) (Updater, error) {
	// Fetch updater config
	config, err := updaterConfig(*tsdbConfigFile)
	if err != nil {
		return nil, err
	}

	// Create an instance of TSDB
	tsdb, err := tsdb.NewTSDB(config.WebURL, config.SkipTLSVerify, logger)
	if err != nil {
		return nil, err
	}

	// Update cutoff duration
	if config.CutoffDuration != "" {
		cutoffDuration, err := model.ParseDuration(config.CutoffDuration)
		if err != nil {
			panic(fmt.Sprintf("failed to parse cutoffDuration in TSDB updater config: %s", err))
		}
		cutoff = time.Duration(cutoffDuration)
	}
	return &tsdbUpdater{tsdb, *config}, nil
}

// Return query string from template
func (t *tsdbUpdater) queryBuilder(name string, queryTemplate string, data map[string]interface{}) (string, error) {
	tmpl := template.Must(template.New(name).Parse(queryTemplate))
	builder := &strings.Builder{}
	if err := tmpl.Execute(builder, data); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// Get time averaged value of each metric identified by label uuid
func (t *tsdbUpdater) fetchAggMetrics(
	queryTime time.Time,
	duration time.Duration,
	uuids string,
) map[string]tsdb.Metric {
	var aggMetrics = make(map[string]tsdb.Metric, len(t.config.Queries))

	// Get rate and scrape intervals
	rateInterval := t.RateInterval()
	scrapeInterval := t.Intervals()["scrape_interval"]
	evaluationInterval := t.Intervals()["evaluation_interval"]

	// If duration is less than rateInterval bail
	if duration < rateInterval {
		return aggMetrics
	}

	// If duration is more than or equal to 1 day, double scrape and rate intervals
	// to reduce number of data points Prometheus has to process
	if duration >= time.Duration(24*time.Hour) {
		scrapeInterval = 2 * scrapeInterval
		evaluationInterval = 2 * evaluationInterval
		rateInterval = 2 * rateInterval
	}

	// Start a wait group
	var wg sync.WaitGroup
	wg.Add(len(t.config.Queries))

	// Template data
	tmplData := map[string]interface{}{
		"UUIDS":                   uuids,
		"ScrapeInterval":          scrapeInterval,
		"ScrapeIntervalMilli":     scrapeInterval.Milliseconds(),
		"EvaluationInterval":      evaluationInterval,
		"EvaluationIntervalMilli": evaluationInterval.Milliseconds(),
		"RateInterval":            rateInterval,
		"Range":                   duration,
	}

	// Loop over t.config.queries map and make queries
	for name, query := range t.config.Queries {
		go func(n string, q string) {
			var aggMetric tsdb.Metric
			var err error
			tsdbQuery, err := t.queryBuilder(n, q, tmplData)
			if err != nil {
				level.Error(t.Logger).Log(
					"msg", "Failed to build query from template", "metric", n,
					"query_template", q, "err", err,
				)
				wg.Done()
				return
			}

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
		if metric, mExists := aggMetrics["avg_cpu_usage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveCPUUsage = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["avg_cpu_mem_usage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveCPUMemUsage = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_cpu_energy_usage_kwh"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalCPUEnergyUsage = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_cpu_emissions_gms"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalCPUEmissions = sanitizeValue(value)
			}
		}

		// Update with GPU metrics
		if metric, mExists := aggMetrics["avg_gpu_usage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveGPUUsage = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["avg_gpu_mem_usage"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].AveGPUMemUsage = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_gpu_energy_usage_kwh"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalGPUEnergyUsage = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_gpu_emissions_gms"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalGPUEmissions = sanitizeValue(value)
			}
		}

		// Update with IO metrics
		if metric, mExists := aggMetrics["total_io_write_hot_gb"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalIOWriteHot = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_io_read_hot_gb"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalIOReadHot = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_io_write_cold_gb"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalIOWriteCold = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_io_read_cold_gb"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalIOReadCold = sanitizeValue(value)
			}
		}

		// Update with network metrics
		if metric, mExists := aggMetrics["total_ingress_in_gb"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalIngress = sanitizeValue(value)
			}
		}
		if metric, mExists := aggMetrics["total_outgress_in_gb"]; mExists {
			if value, exists := metric[units[i].UUID]; exists {
				units[i].TotalOutgress = sanitizeValue(value)
			}
		}
	}

	// Finally delete time series
	if len(ignoredUnits) > 0 || len(t.config.LabelsToDrop) > 0 {
		if err := t.deleteTimeSeries(startTime, endTime, ignoredUnits); err != nil {
			level.Error(t.Logger).
				Log("msg", "Failed to delete time series in TSDB", "err", err)
		}
	}
	return units
}

// Delete time series data of ignored units
func (t *tsdbUpdater) deleteTimeSeries(startTime time.Time, endTime time.Time, unitUUIDs []string) error {
	// Check if there are any units to ignore. If there aren't return immediately
	// We shouldnt make a API request to delete with empty units slice as TSDB will
	// match all units during that period with uuid=~"" matcher
	if len(unitUUIDs) == 0 && len(t.config.LabelsToDrop) == 0 {
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
	start := startTime.Add(-cutoff)
	end := endTime

	// Matcher must be of format "{uuid=~"<regex>"}"
	// Ref: https://ganeshvernekar.com/blog/prometheus-tsdb-queries/
	//
	// Join them with | as delimiter. We will use regex match to match all series
	// with the label uuid=~"$unitids"
	allUUIDs := strings.Join(unitUUIDs, "|")
	matchers := append(t.config.LabelsToDrop, fmt.Sprintf("{uuid=~\"%s\"}", allUUIDs))
	// Make a API request to delete data of ignored units
	return t.Delete(start, end, matchers)
}

// sanitizeValue verifies if value is either NaN/Inf/-Inf.
// If value is any of these, zero will be returned
func sanitizeValue(val float64) models.JSONFloat {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return models.JSONFloat(0)
	}
	return models.JSONFloat(val)
}
