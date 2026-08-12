// Package main is the entry point for the mycel CLI.
package main

import (
	"os"

	"github.com/rpuneet/mycel/internal/cmd"
	"github.com/rpuneet/mycel/pkg/envpath"
)

// Version information set by ldflags during build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// GUI / launchd PATH often omits Homebrew; enrich before any LookPath.
	envpath.Enrich()
	cmd.SetVersionInfo(version, commit, date)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
