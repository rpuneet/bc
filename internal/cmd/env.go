package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment variable management (deprecated)",
	Long: `Environment variables are no longer stored in settings.json.
Use the secrets store for sensitive values (bc secret set NAME VALUE)
or set environment variables on the host / in Docker.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("Environment variable management has been removed from settings.")
		fmt.Println("Use 'mycel secret set NAME VALUE' for sensitive values,")
		fmt.Println("or set environment variables on the host / in Docker.")
		return fmt.Errorf("mycel env has been removed; use 'mycel secret set NAME VALUE' or host/container env vars")
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
