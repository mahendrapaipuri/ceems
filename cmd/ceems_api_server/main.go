package main

import (
	"fmt"
	"os"

	"github.com/mahendrapaipuri/ceems/pkg/api/cli"
)

// Main entry point for `ceems` app
func main() {
	// Create a new app
	CEEMSServer, err := cli.NewCEEMSServer()
	if err != nil {
		panic("Failed to create an instance of CEEMS Server App")
	}

	// Main entrypoint of the app
	if err := CEEMSServer.Main(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
