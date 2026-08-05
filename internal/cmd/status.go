package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
)

var statusActivity bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show agent status",
	Long: `Show the status of all mycel agents.

Examples:
  mycel status                   # Show all agents
  mycel status --json            # Output as JSON
  mycel status --activity        # Show recent channel activity

Output:
  AGENT     ROLE      STATE    UPTIME    TASK
  eng-01    engineer  working  2h 15m    Implementing feature X
  eng-02    engineer  idle     1h 30m    -

Agent States:
  working   Agent is actively processing
  idle      Agent is waiting for input
  done      Agent has completed task
  error     Agent encountered an error
  stopped   Agent is not running

See Also:
  mycel agent list   List agents with more detail
  mycel logs         View agent event logs
  Web UI (http://localhost:9374)  Live dashboard`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusActivity, "activity", false, "Show recent channel activity")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	log.Debug("status command started")

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	wsName := "workspace"
	if info, infoErr := c.Stats.SystemInfo(cmd.Context()); infoErr == nil && info.Workspace != "" {
		wsName = filepath.Base(info.Workspace)
	}

	agentList, err := c.Agents.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	log.Debug("agents loaded", "count", len(agentList))

	// Count active/working agents
	activeCount := 0
	workingCount := 0
	for _, a := range agentList {
		if a.State != "stopped" && a.State != "error" {
			activeCount++
		}
		if a.State == "working" {
			workingCount++
		}
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		output := struct {
			Workspace string             `json:"workspace"`
			Agents    []client.AgentInfo `json:"agents"`
			Total     int                `json:"total"`
			Active    int                `json:"active"`
			Working   int                `json:"working"`
		}{
			Workspace: wsName,
			Agents:    agentList,
			Total:     len(agentList),
			Active:    activeCount,
			Working:   workingCount,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Summary header
	fmt.Printf("Repo: %s | Agents: %d | Active: %d | Working: %d\n", wsName, len(agentList), activeCount, workingCount)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println()

	if len(agentList) == 0 {
		fmt.Println("No agents configured")
		fmt.Println()
		fmt.Println("Run 'mycel up' to start agents")
		return nil
	}

	// Determine terminal width for dynamic task column
	termWidth := 80
	if w, _, termErr := term.GetSize(os.Stdout.Fd()); termErr == nil && w > 0 {
		termWidth = w
	}

	fixedWidth := 57
	taskWidth := termWidth - fixedWidth
	if taskWidth < 20 {
		taskWidth = 20
	}

	fmt.Printf("%-15s %-12s %-10s %-20s %s\n", "AGENT", "ROLE", "STATE", "UPTIME", "TASK")
	fmt.Println(strings.Repeat("-", termWidth))

	for _, a := range agentList {
		uptime := "-"
		if a.State != "stopped" && !a.StartedAt.IsZero() {
			uptime = formatDuration(time.Since(a.StartedAt))
		}

		task := normalizeTask(a.Task)
		if task == "" {
			task = "-"
		}
		if len(task) > taskWidth {
			task = task[:taskWidth-3] + "..."
		}

		stateStr := colorStateStr(a.State)
		fmt.Printf("%-15s %-12s %s %-20s %s\n", a.Name, a.Role, stateStr, uptime, task)
	}

	fmt.Println()
	fmt.Println("Symbols (in TASK column):")
	fmt.Println("  ✻ ✳ ✽ ·  Thinking (agent is processing)")
	fmt.Println("  ⏺        Tool call (agent is running a tool)")
	fmt.Println("  ❯        Prompt (agent is waiting for input)")

	// Show recent channel activity if requested
	if statusActivity {
		fmt.Println()
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println("Recent Activity:")
		fmt.Println()

		channels, chErr := c.Channels.List(cmd.Context())
		if chErr == nil {
			messageCount := 0
			for _, ch := range channels {
				msgs, histErr := c.Channels.History(cmd.Context(), ch.Name, 3, 0, "")
				if histErr != nil {
					continue
				}
				for _, msg := range msgs {
					age := time.Since(msg.CreatedAt)
					ageStr := formatDuration(age) + " ago"
					msgPreview := truncateActivityMsg(msg.Content, 50)
					fmt.Printf("  [#%s] %s: %s (%s)\n", ch.Name, msg.Sender, msgPreview, ageStr)
					messageCount++
				}
			}
			if messageCount == 0 {
				fmt.Println("  No recent messages")
			}
		}
	}

	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  mycel agent attach <agent>  # Attach to agent's session")
	fmt.Println("  mycel agent health    # Check agent health status")
	fmt.Println("  mycel down            # Stop all agents")

	return nil
}

// truncateActivityMsg truncates a message to maxLen, removing newlines
func truncateActivityMsg(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// normalizeTask transforms cooking metaphors in Claude Code's status line
// to clearer terminology. Issue #970.
func normalizeTask(task string) string {
	replacements := []struct {
		old, new string
	}{
		{"Sautéed", "Working"},
		{"Sauteed", "Working"},
		{"Cooked", "Processed"},
		{"Cogitated", "Thinking"},
		{"Marinated", "Idle"},
		{"Frolicking", "Active"},
	}
	for _, r := range replacements {
		if strings.Contains(task, r.old) {
			return strings.Replace(task, r.old, r.new, 1)
		}
	}
	return task
}

// colorStateStr colors a state string for terminal output.
func colorStateStr(s string) string {
	padded := fmt.Sprintf("%-10s", s)

	switch s {
	case "idle":
		return ui.CyanText(padded)
	case "working":
		return ui.GreenText(padded)
	case "done":
		return ui.GreenText(padded)
	case "stuck":
		return ui.RedText(padded)
	case "error":
		return ui.RedText(padded)
	case "stopped":
		return ui.YellowText(padded)
	default:
		return padded
	}
}
