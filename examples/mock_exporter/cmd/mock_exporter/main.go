// Boiler plate code to create a new instance of BatchJobExporter entrypoint
package main

// Ensure to import the current mock collector
import (
	_ "github.com/mahendrapaipuri/batchjob_monitoring/examples/mock_exporter/pkg/collector"
	batchjob_collector "github.com/mahendrapaipuri/batchjob_monitoring/pkg/collector"
)

// Main entry point for `batchjob_exporter` app
func main() {
	// Create a new app
	batchJobExporterApp, err := batchjob_collector.NewBatchJobExporter()
	if err != nil {
		panic("Failed to create an instance of BatchJobExporter App")
	}

	// Main entrypoint of the app
	batchJobExporterApp.Main()
}
