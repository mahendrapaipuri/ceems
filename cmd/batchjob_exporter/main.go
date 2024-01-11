package main

import "github.com/mahendrapaipuri/batchjob_monitor/pkg/collector"

// Main entry point for `batchjob_exporter` app
func main() {
	// Create a new app
	batchJobExporterApp, err := collector.NewBatchJobExporter()
	if err != nil {
		panic("Failed to create an instance of BatchJobExporter App")
	}

	// Main entrypoint of the app
	batchJobExporterApp.Main()
}
