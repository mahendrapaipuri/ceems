package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ceems-dev/ceems/pkg/api/models"
	"github.com/ceems-dev/ceems/pkg/tsdb"
	"github.com/prometheus/common/model"
)

var (
	queryMDMu = sync.RWMutex{}
	queryMD   []queryMetadata
)

// queryMetadata contains metadata information for each TSDB series. We dump
// metadata.json file in the output directory containing fingerprint of each
// query and name CSV files after this fingerprint.
// This allows end users to programatically reads CSV files and their metadata
// and do data processing in their favorite tools like pandas, numpy, etc.
type queryMetadata struct {
	Fingerprint string       `json:"fingerprint"`
	Labels      model.Metric `json:"labels"`
}

// tsdbData saves time series data of units in CSV files.
func tsdbData(ctx context.Context, config *Config, units []models.Unit, outDir string) error {
	// New TSDB client
	tsdb, err := tsdb.New(config.TSDB.Web.URL, config.TSDB.Web.HTTPClientConfig, slog.New(slog.DiscardHandler))
	if err != nil {
		return fmt.Errorf("failed to create tsdb API client: %w", err)
	}

	// Create outDir for saving CSV files
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory for saving CSV files: %w", err)
	}

	// Start a wait group
	wg := sync.WaitGroup{}

	// Fetch time series of each metric in separate go routine
	for _, unit := range units {
		for _, query := range config.TSDB.Queries {
			wg.Add(1)

			// Fetch metrics from TSDB and write to CSV files
			go fetchData(ctx, fmt.Sprintf(query, unit.UUID), unit.StartedAtTS, unit.EndedAtTS, outDir, tsdb, &wg)
		}
	}

	// Wait for all routines
	wg.Wait()

	// Dump metadata.json
	writeMetadata(queryMD, outDir)

	fmt.Fprintln(os.Stderr, "time series data saved to directory", outDir)

	return nil
}

// fetchData retrieves time series data from TSDB.
func fetchData(ctx context.Context, query string, start int64, end int64, outDir string, tsdb *tsdb.Client, wg *sync.WaitGroup) {
	defer wg.Done()

	// Make a range query
	results, err := tsdb.RangeQuery(ctx, query, time.UnixMilli(start), time.UnixMilli(end), 10*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to fetch time series for query", query, "err:", err)

		return
	}

	var md []queryMetadata

	// Open metric file for each UUID and write data
	for _, result := range results {
		if _, ok := result.Metric["uuid"]; ok {
			// Get fingerprint
			fp := result.Metric.Fingerprint().String()

			// Add metadata of query
			md = append(md, queryMetadata{
				Fingerprint: fp,
				Labels:      result.Metric,
			})

			// Create file name based on fingerprint
			csvFilepath := filepath.Join(outDir, fp+".csv")

			writer, f, err := newCSVWriter(csvFilepath)
			if err != nil {
				fmt.Fprintln(os.Stderr, "failed to open file to write metrics:", err, "file:", csvFilepath)

				continue
			}

			defer f.Close()

			// Write header
			if err := writer.Write([]string{"timestamp", "value"}); err != nil {
				fmt.Fprintln(os.Stderr, "failed to write header:", err, "file:", csvFilepath)
			}

			// Write records
			for _, value := range result.Values {
				if err := writer.Write([]string{value.Timestamp.String(), value.Value.String()}); err != nil {
					fmt.Fprintln(os.Stderr, "failed to write data:", err, "file:", csvFilepath)
				}
			}

			// Flush writer
			writer.Flush()

			if writer.Error() != nil {
				fmt.Fprintln(os.Stderr, "failed to write data:", writer.Error(), "file:", csvFilepath)
			}
		}
	}

	// Append metadata to global var
	queryMDMu.Lock()
	defer queryMDMu.Unlock()

	queryMD = append(queryMD, md...)
}

// writeMetadata dumps the metadata.json file to outDir.
func writeMetadata(mds []queryMetadata, outDir string) {
	// Dump metadata json
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "\t")

	if err := encoder.Encode(mds); err != nil {
		fmt.Fprintln(os.Stderr, "failed to encode metadata", "err:", err)

		return
	}

	metadataFilepath := filepath.Join(outDir, "metadata.json")

	file, err := os.OpenFile(metadataFilepath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create metadata.json file", "err:", err)

		return
	}

	defer file.Close()

	if _, err := file.Write(buffer.Bytes()); err != nil {
		fmt.Fprintln(os.Stderr, "failed to write content to metadata.json file", "err:", err)

		return
	}
}

// newCSVWriter returns a new CSV writer.
func newCSVWriter(filename string) (*csv.Writer, *os.File, error) {
	f, err := os.Create(filename)
	if err != nil {
		return nil, nil, err
	}

	// New instance of CSV writer
	writer := csv.NewWriter(f)

	return writer, f, nil
}
