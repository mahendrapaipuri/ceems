// Boiler plate code to create a new instance of BatchJobStatsServer entrypoint
package main

// Ensure to import the current mock updater hook
import (
	"fmt"
	"os"

	// Updaters are executed in reverse order of registration. Here updater_two will be
	// first executed and then updater_one.
	//
	// If in-built tsdb updater is also enabled, that will be executed before
	// updater_two and upater_one.
	//
	// This is a design decision made to ensure that third party updaters can always
	// override changes made by in-built tsdb updater.
	//
	// If there are multiple third party updaters and if the order of execution is
	// important, ensure to import them in the reverse order to the desired order
	// of execution
	_ "github.com/mahendrapaipuri/batchjob_monitor/examples/mock_updaters/pkg/updater_one"
	_ "github.com/mahendrapaipuri/batchjob_monitor/examples/mock_updaters/pkg/updater_two"
	batchjob_stats_cli "github.com/mahendrapaipuri/batchjob_monitor/pkg/jobstats/cli"
)

// Main entry point for `batchjob_stats_server` app
func main() {
	// Create a new app
	batchJobStatsServerApp, err := batchjob_stats_cli.NewBatchJobStatsServer()
	if err != nil {
		panic("Failed to create an instance of Batch Job Stats Server App")
	}

	// Main entrypoint of the app
	if err := batchJobStatsServerApp.Main(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
