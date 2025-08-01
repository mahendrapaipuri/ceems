package main

import (
	"log"
	"os"

	"github.com/ceems-dev/ceems/pkg/collector"
)

// Main entry point for `ceems_exporter` app.
func main() {
	// Create a new app
	ceemsExporterApp, err := collector.NewCEEMSExporter()
	if err != nil {
		panic("Failed to create an instance of CEEMSExporter App")
	}

	// Main entrypoint of the app
	if err := ceemsExporterApp.Main(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
