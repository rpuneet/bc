package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
)

// Flags for report command (enhanced for stuck reports - #675)
var (
	reportReason       string
	reportReproduction string
	reportSeverity     string
)

var agentReportCmd = &cobra.Command{
	Use:   "report <state> [message]",
	Short: "Report agent state (called by agents)",
	Long: `Report the calling agent's current state. This command must be run from within an agent session.

Valid states: idle, working, done, stuck, error

For stuck state, use --reason to provide detailed context:
  mycel agent report stuck --reason "database connection timeout"
  mycel agent report stuck --reason "auth fails" --reproduction "login with test user" --severity critical

Examples:
  mycel agent report working "fixing auth bug"
  mycel agent report done "auth bug fixed"
  mycel agent report stuck "need database credentials"
  mycel agent report stuck --reason "TUI freezes on channel select" --severity high`,
	Args: cobra.MinimumNArgs(1),
	RunE: runReport,
}

func init() {
	agentCmd.AddCommand(agentReportCmd)

	// Enhanced flags for stuck reports (#675)
	agentReportCmd.Flags().StringVar(&reportReason, "reason", "", "Detailed reason for stuck state")
	agentReportCmd.Flags().StringVar(&reportReproduction, "reproduction", "", "Steps to reproduce the issue")
	agentReportCmd.Flags().StringVar(&reportSeverity, "severity", "medium", "Issue severity (critical, high, medium, low)")
}

func runReport(cmd *cobra.Command, args []string) error {
	agentID := os.Getenv("MYCEL_AGENT_ID")
	if agentID == "" {
		return errorAgentNotRunning(fmt.Sprintf("mycel agent report %s", args[0]))
	}

	stateStr := args[0]
	message := ""
	if len(args) > 1 {
		message = strings.Join(args[1:], " ")
	}

	// Validate state
	switch stateStr {
	case "idle", "working", "done", "stuck", "error":
		// valid
	default:
		return fmt.Errorf("invalid state: %s (valid: idle, working, done, stuck, error)", stateStr)
	}

	// Update agent state via daemon
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	if reportErr := c.Agents.Report(cmd.Context(), agentID, stateStr, message); reportErr != nil {
		return fmt.Errorf("failed to update agent state: %w", reportErr)
	}

	// Build event data
	eventData := make(map[string]any)
	eventMsg := fmt.Sprintf("%s: %s", stateStr, message)

	// Enhanced stuck reporting (#675)
	if stateStr == "stuck" {
		if reportReason != "" {
			eventData["reason"] = reportReason
			eventMsg = fmt.Sprintf("%s: %s", stateStr, reportReason)
		}
		if reportReproduction != "" {
			eventData["reproduction"] = reportReproduction
		}
		if reportSeverity != "" {
			eventData["severity"] = reportSeverity
		}
		eventData["stuck"] = true
	}

	// Log the report event via daemon
	ev := client.EventInfo{
		Type:    "agent_report",
		Agent:   agentID,
		Message: eventMsg,
	}
	if len(eventData) > 0 {
		ev.Data = eventData
	}
	if appendErr := c.Events.Append(cmd.Context(), ev); appendErr != nil {
		// Non-fatal: state was already updated
		fmt.Fprintf(os.Stderr, "warning: failed to log event: %v\n", appendErr)
	}

	// Output message
	if stateStr == "stuck" && reportReason != "" {
		fmt.Printf("Reported: %s [%s]\n", stateStr, reportSeverity)
		fmt.Printf("  Reason: %s\n", reportReason)
		if reportReproduction != "" {
			fmt.Printf("  Reproduction: %s\n", reportReproduction)
		}
	} else {
		fmt.Printf("Reported: %s %s\n", stateStr, message)
	}
	return nil
}
