package main

import (
	"log"
	"os"

	"github.com/mahendrapaipuri/ceems/pkg/api/cli"
	// We need to import each resource manager package here to call init function.
	_ "github.com/mahendrapaipuri/ceems/pkg/api/resource/slurm"
	// We need to import each updater package here to call init function.
	_ "github.com/mahendrapaipuri/ceems/pkg/api/updater/tsdb"
)

// Main entry point for `ceems` app.
func main() {
	// Create a new app
	CEEMSServer, err := cli.NewCEEMSServer()
	if err != nil {
		panic("Failed to create an instance of CEEMS Server App")
	}

	// Main entrypoint of the app
	if err := CEEMSServer.Main(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
