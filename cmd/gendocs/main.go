// Package main generates CLI reference documentation from Cobra commands.
package main

import (
	"log"
	"os"

	"github.com/spf13/cobra/doc"

	cmd "github.com/rpuneet/mycel/internal/cmd"
)

func main() {
	outDir := "docs/reference/cli"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0o750); err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	rootCmd := cmd.Root()
	rootCmd.DisableAutoGenTag = true

	if err := doc.GenMarkdownTree(rootCmd, outDir); err != nil {
		log.Fatalf("failed to generate docs: %v", err)
	}
}
