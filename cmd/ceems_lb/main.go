//go:build cgo
// +build cgo

package main

import (
	"log"
	"os"

	"github.com/mahendrapaipuri/ceems/pkg/lb/cli"
)

// Main entry point for `ceems_lb` app.
func main() {
	// Create a new app
	CEEMSLoadBalancer, err := cli.NewCEEMSLoadBalancer()
	if err != nil {
		panic("Failed to create an instance of CEEMS Load Balancer App")
	}

	// Main entrypoint of the app
	if err := CEEMSLoadBalancer.Main(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
