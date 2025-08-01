// Boiler plate code to create a new instance of ComputeResourceExporterApp entrypoint
package main

// Ensure to import the current mock collector.
import (
	"fmt"
	"os"

	_ "github.com/ceems-dev/ceems/examples/mock_collector/pkg/collector"
	"github.com/ceems-dev/ceems/pkg/collector"
)

// Main entry point for `ceems_exporter` app.
func main() {
	// Create a new app
	ceemsExporterApp, err := collector.NewCEEMSExporter()
	if err != nil {
		panic("Failed to create an instance of CEEMSExporterApp App")
	}

	// Main entrypoint of the app
	if err := ceemsExporterApp.Main(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
