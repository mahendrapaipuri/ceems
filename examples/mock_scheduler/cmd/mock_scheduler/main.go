// Boiler plate code to create a new instance of BatchJobStatsServer entrypoint

package main

// Ensure to import the current mock scheduler
import (
	_ "github.com/mahendrapaipuri/batchjob_monitoring/examples/mock_scheduler/pkg/scheduler"
	batchjob_stats "github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"
)

// Main entry point for `batchjob_stats_server` app
func main() {
	// Create a new app
	batchJobStatsServerApp, err := batchjob_stats.NewBatchJobStatsServer()
	if err != nil {
		panic("Failed to create an instance of Batch Job Stats Server App")
	}

	// Main entrypoint of the app
	batchJobStatsServerApp.Main()
}
