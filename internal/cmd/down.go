package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
	"github.com/rpuneet/mycel/pkg/workspace"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop mycel services",
	Long: `Stop the mycel daemon and database Docker containers for this workspace.

Examples:
  mycel down
  mycel down --workspace /path/to/workspace`,
	RunE: runDown,
}

var downWorkspace string

func init() {
	downCmd.Flags().StringVar(&downWorkspace, "workspace", "", "Workspace directory (defaults to current workspace)")
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, _ []string) error {
	var ws *workspace.Workspace
	var err error
	if downWorkspace != "" {
		ws, err = workspace.Load(downWorkspace)
		if err != nil {
			return fmt.Errorf("cannot load workspace at %s: %w", downWorkspace, err)
		}
	} else {
		ws, err = getRepo()
		if err != nil {
			return errNoRepo(err)
		}
	}

	ctx := cmd.Context()

	fmt.Printf("Stopping mycel in %s\n\n", ws.RootDir)

	id := wsID(ws.RootDir)
	daemonName := fmt.Sprintf("bc-%s-daemon", id)

	// Stop local daemon if running via PID file
	pidPath, pidErr := workspace.DaemonPidPath()
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

	containers := []string{daemonName}
	if !wasDaemon {
		// Only stop bc-db in full Docker mode (bc up without -d)
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

	if stopped == 0 {
		fmt.Println("  No services running")
	} else {
		fmt.Println()
		fmt.Printf("  %s Stopped %d service(s)\n", ui.GreenText("ok"), stopped)
	}
	return nil
}
