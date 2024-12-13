//go:build cgo
// +build cgo

// Boiler plate code to create a new instance of usageStatsServerApp entrypoint
package main

// Ensure to import the current mock updater hook
import (
	"fmt"
	"os"

	// The order of execution of updaters can be controlled in the config file.
	// Order of registration of updaters do not matter.
	_ "github.com/mahendrapaipuri/ceems/examples/mock_updater/pkg/updaterone"
	_ "github.com/mahendrapaipuri/ceems/examples/mock_updater/pkg/updatertwo"
	"github.com/mahendrapaipuri/ceems/pkg/api/cli"
)

// Main entry point for `usagestats` app
func main() {
	// Create a new app
	usageStatsServerApp, err := cli.NewCEEMSServer()
	if err != nil {
		panic("Failed to create an instance of Batch Job Stats Server App")
	}

	// Main entrypoint of the app
	if err := usageStatsServerApp.Main(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
