//go:build cgo
// +build cgo

package main

// We need to import each resource manager and updater package here.
import (
	"log"
	"os"

	"github.com/ceems-dev/ceems/pkg/api/cli"
	_ "github.com/ceems-dev/ceems/pkg/api/resource/k8s"
	_ "github.com/ceems-dev/ceems/pkg/api/resource/openstack"
	_ "github.com/ceems-dev/ceems/pkg/api/resource/slurm"
	_ "github.com/ceems-dev/ceems/pkg/api/updater/tsdb"
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
