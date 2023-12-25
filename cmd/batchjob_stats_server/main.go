package main

import "github.com/mahendrapaipuri/batchjob_monitoring/pkg/jobstats"

// Main entry point for `batchjob_stats_server` app
func main() {
	// Create a new app
	batchJobStatsServer, err := jobstats.NewBatchJobStatsServer()
	if err != nil {
		panic("Failed to create an instance of BatchJobStats Server App")
	}

	// Main entrypoint of the app
	batchJobStatsServer.Main()
}
