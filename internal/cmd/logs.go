package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show the event log",
	Long: `View the mycel event log showing agent spawns, stops, work assignments, and reports.

Examples:
  mycel logs                     # Show all events
  mycel logs --agent eng-01      # Filter by agent
  mycel logs --type agent.report # Filter by event type
  mycel logs --since 1h          # Events from last hour
  mycel logs --tail 20           # Last N events
  mycel logs --full              # Show full messages (no truncation)
  mycel logs --json              # JSON output

Event Types:
  agent.started    Agent was created and started
  agent.stopped    Agent was stopped
  agent.report     Agent submitted a progress report
  state.working    Agent started working on task
  state.idle       Agent became idle
  state.stuck      Agent is stuck (may need intervention)

Output:
  TIME      AGENT     TYPE           MESSAGE
  10:15:32  eng-01    state.working  Starting implementation
  10:16:45  eng-01    agent.report   Completed feature X

See Also:
  mycel status    Quick agent status overview
  mycel home      TUI with activity timeline`,
	RunE: runLogs,
}

var (
	logsAgent string
	logsTail  int
	logsType  string
	logsSince string
	logsFull  bool
)

const logsMaxMessageLen = 80

func init() {
	logsCmd.Flags().StringVar(&logsAgent, "agent", "", "Filter by agent name")
	logsCmd.Flags().IntVar(&logsTail, "tail", 0, "Show last N events")
	logsCmd.Flags().StringVar(&logsType, "type", "", "Filter by event type (e.g. agent.report)")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show events since duration ago (e.g. 1h, 30m)")
	logsCmd.Flags().BoolVar(&logsFull, "full", false, "Show full messages without truncation")
	rootCmd.AddCommand(logsCmd)
}

// parseSinceDuration parses a duration string like "1h", "30m", "2h30m" and
// returns the cutoff time (now minus duration).
func parseSinceDuration(s string) (time.Time, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid duration %q: %w (use e.g. 1h, 30m, 2h30m)", s, err)
	}
	return time.Now().Add(-d), nil
}

// truncateMessage truncates a string to maxLen characters, appending "..." if truncated.
func truncateMessage(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func runLogs(cmd *cobra.Command, args []string) error {
	if cmd.Flags().Changed("tail") && logsTail <= 0 {
		return fmt.Errorf("tail must be a positive number")
	}

	log.Debug("logs command started", "agent", logsAgent, "type", logsType, "since", logsSince, "tail", logsTail)

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// Fetch events from daemon
	var evts []client.EventInfo
	if logsAgent != "" {
		evts, err = c.Events.ListByAgent(cmd.Context(), logsAgent)
	} else {
		evts, err = c.Events.List(cmd.Context())
	}
	if err != nil {
		return fmt.Errorf("failed to read events: %w", err)
	}

	// Filter by event type
	if logsType != "" {
		filtered := evts[:0]
		for _, ev := range evts {
			if ev.Type == logsType {
				filtered = append(filtered, ev)
			}
		}
		evts = filtered
	}

	// Filter by time (--since)
	if logsSince != "" {
		cutoff, parseErr := parseSinceDuration(logsSince)
		if parseErr != nil {
			return parseErr
		}
		filtered := evts[:0]
		for _, ev := range evts {
			if !ev.Timestamp.Before(cutoff) {
				filtered = append(filtered, ev)
			}
		}
		evts = filtered
	}

	// Apply tail last (take only the last N events)
	if logsTail > 0 && len(evts) > logsTail {
		evts = evts[len(evts)-logsTail:]
	}

	log.Debug("events filtered", "count", len(evts))

	if len(evts) == 0 {
		if logsAgent != "" {
			ui.Warning("No events found for agent '%s'", logsAgent)
		} else {
			ui.Warning("No events found")
		}
		return nil
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(evts)
	}

	const messageSentType = "message.sent"
	for _, ev := range evts {
		ts := ev.Timestamp.Format("15:04:05")
		agentStr := ""
		if ev.Agent != "" {
			agentStr = fmt.Sprintf(" [%s]", ev.Agent)
		}
		// For message.sent, show sender → recipient
		if ev.Type == messageSentType && ev.Data != nil {
			if recipient, ok := ev.Data["recipient"].(string); ok && recipient != "" {
				agentStr = fmt.Sprintf(" [%s] → [%s]", ev.Agent, recipient)
			}
		}
		msg := ""
		if ev.Message != "" {
			m := ev.Message
			if !logsFull {
				m = truncateMessage(m, logsMaxMessageLen)
			}
			msg = " " + m
		}
		fmt.Printf("%s %-20s%s%s\n", ts, ev.Type, agentStr, msg)
	}

	return nil
}
