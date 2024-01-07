// Boiler plate code to create a new instance of BatchJobStatsServer entrypoint
package main

// Ensure to import the current mock scheduler
import (
	_ "github.com/mahendrapaipuri/batchjob_metrics_monitor/examples/mock_scheduler/pkg/scheduler"
	batchjob_stats_cli "github.com/mahendrapaipuri/batchjob_metrics_monitor/pkg/jobstats/cli"
)

// Main entry point for `batchjob_stats_server` app
func main() {
	// Create a new app
	batchJobStatsServerApp, err := batchjob_stats_cli.NewBatchJobStatsServer()
	if err != nil {
		panic("Failed to create an instance of Batch Job Stats Server App")
	}

	// Main entrypoint of the app
	batchJobStatsServerApp.Main()
}
