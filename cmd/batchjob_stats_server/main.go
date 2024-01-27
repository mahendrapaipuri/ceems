package main

import (
	"fmt"
	"os"

	"github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/cli"
)

// Main entry point for `batchjob_stats_server` app
func main() {
	// Create a new app
	batchJobStatsServer, err := cli.NewBatchJobStatsServer()
	if err != nil {
		panic("Failed to create an instance of BatchJobStats Server App")
	}

	// Main entrypoint of the app
	if err := batchJobStatsServer.Main(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
