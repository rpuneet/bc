package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/rpuneet/mycel/pkg/ui"
)

// bootstrapServerDaemons starts bc-db (unified TimescaleDB) during bc init.
func bootstrapServerDaemons(_ string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := dockerRun(ctx, "bc-db", []string{
		"-p", "5432:5432",
		"-e", "POSTGRES_PASSWORD=bc",
		"-v", "bc-db-data:/var/lib/postgresql/data",
		"--restart", "always",
		"bc-bcdb:latest",
	}); err != nil {
		fmt.Printf("  %s bc-db: %v\n", ui.YellowText("warning"), err)
	}

	fmt.Printf("\n  %s database ready\n\n", ui.GreenText("ok"))
}
