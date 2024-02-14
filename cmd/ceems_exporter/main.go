package main

import (
	"fmt"
	"os"

	"github.com/mahendrapaipuri/ceems/pkg/collector"
)

// Main entry point for `ceems_exporter` app
func main() {
	// Create a new app
	ceemsExporterApp, err := collector.NewCEEMSExporter()
	if err != nil {
		panic("Failed to create an instance of CEEMSExporter App")
	}

	// Main entrypoint of the app
	if err := ceemsExporterApp.Main(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
