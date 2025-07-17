//go:build cgo
// +build cgo

// Boiler plate code to create a new instance of CEEMSServer entrypoint
package main

// Ensure to import the current mock scheduler.
import (
	"fmt"
	"os"

	// For instance to import slurm manager, following import statement must be added.
	_ "github.com/ceems-dev/ceems/examples/mock_resource_manager/pkg/resource"
	"github.com/ceems-dev/ceems/pkg/api/cli"
	_ "github.com/ceems-dev/ceems/pkg/api/resource/slurm"
)

// Main entry point for `usagestats` app.
func main() {
	// Create a new app
	usageStatsServerApp, err := cli.NewCEEMSServer()
	if err != nil {
		panic("Failed to create an instance of Usage Stats Server App")
	}

	// Main entrypoint of the app
	if err := usageStatsServerApp.Main(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
