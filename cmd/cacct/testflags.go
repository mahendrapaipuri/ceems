//go:build test
// +build test

package main

func init() {
	// Test related hidden flags
	// We include this file in the compilation only for
	// testing. In production builds, this file will be
	// omitted and hence, users will not be able to control
	// user from CLI flag
	cacctApp.Flag(
		"current-user", "Mock current user.",
	).Hidden().StringVar(&mockCurrentUser)
	cacctApp.Flag(
		"config-path", "Mock configuration file path.",
	).Hidden().StringVar(&mockConfigPath)
}
