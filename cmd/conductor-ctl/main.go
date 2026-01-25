// Package main is the entry point for the conductor-ctl CLI tool.
package main

import (
	"os"
)

// Build information, set by ldflags during build.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	// Set build information for the version command
	Version = version
	Commit = commit
	BuildTime = buildTime

	if err := Execute(); err != nil {
		os.Exit(1)
	}
}
