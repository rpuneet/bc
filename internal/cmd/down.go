package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop mycel services",
	Long: `Stop the mycel daemon and database Docker containers.

The daemon is user-scoped — 'mycel down' works from any directory.

Examples:
  mycel down`,
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	fmt.Printf("Stopping mycel\n\n")

	// Stop local daemon if running via PID file. The daemon is
	// user-scoped, so no repo/CWD is required.
	pidPath, pidErr := home.DaemonPidPath()
	if pidErr != nil {
		log.Warn("failed to resolve daemon pid path", "error", pidErr)
	}
	wasDaemon := false
	if pidData, readErr := os.ReadFile(pidPath); readErr == nil && pidPath != "" { //nolint:gosec // controlled home path
		wasDaemon = true
		pid := strings.TrimSpace(string(pidData))
		fmt.Printf("  Stopping local bcd (PID %s)... ", pid)
		killCmd := exec.CommandContext(ctx, "kill", pid) //nolint:gosec // trusted
		if killCmd.Run() == nil {
			fmt.Println(ui.GreenText("stopped"))
			_ = os.Remove(pidPath)
		} else {
			fmt.Println(ui.YellowText("not running"))
			_ = os.Remove(pidPath)
		}
	}

	var containers []string
	if !wasDaemon {
		// Only stop bc-db in full Docker mode (mycel up without -d)
		containers = append(containers, "bc-db")
	}

	var stopped int
	for _, name := range containers {
		//nolint:gosec // trusted
		out, _ := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", name).Output()
		if strings.TrimSpace(string(out)) != "true" {
			continue
		}
		fmt.Printf("  Stopping %s... ", name)
		//nolint:gosec // trusted
		if output, stopErr := exec.CommandContext(ctx, "docker", "stop", name).CombinedOutput(); stopErr != nil {
			fmt.Println(ui.YellowText(fmt.Sprintf("failed (%v)", stopErr)))
			log.Debug("docker stop failed", "name", name, "output", string(output))
			continue
		}
		fmt.Println(ui.GreenText("stopped"))
		stopped++
	}

	if stopped == 0 && !wasDaemon {
		fmt.Println("  No services running")
	} else if stopped > 0 {
		fmt.Println()
		fmt.Printf("  %s Stopped %d service(s)\n", ui.GreenText("ok"), stopped)
	}
	return nil
}
