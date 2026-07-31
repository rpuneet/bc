package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
)

// Issue #1648: Extracted from agent.go for better code organization
// Health monitoring and stuck agent detection commands

// agentHealthCmd displays health status of agents
var agentHealthCmd = &cobra.Command{
	Use:   "health [agent]",
	Short: "Check agent health status",
	Long: `Check health status of agents including tmux session and state freshness.

Examples:
  mycel agent health              # Check all agents
  mycel agent health eng-01       # Check specific agent
  mycel agent health --json       # Output as JSON
  mycel agent health --detect-stuck --alert eng  # Detect stuck and alert`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAgentHealth,
}

// Health command flags
var (
	agentHealthJSON      bool
	agentHealthTimeout   string
	agentHealthDetect    bool
	agentHealthWorkTmout string
	agentHealthMaxFail   int
	agentHealthAlert     string
)

func initAgentHealthFlags() {
	agentHealthCmd.Flags().BoolVar(&agentHealthJSON, "json", false, "Output as JSON")
	agentHealthCmd.Flags().StringVar(&agentHealthTimeout, "timeout", "60s", "Stale state threshold (e.g., 30s, 2m)")
	agentHealthCmd.Flags().BoolVar(&agentHealthDetect, "detect-stuck", false, "Enable stuck detection analysis")
	agentHealthCmd.Flags().StringVar(&agentHealthWorkTmout, "work-timeout", "30m", "Work timeout for stuck detection (e.g., 30m, 1h)")
	agentHealthCmd.Flags().IntVar(&agentHealthMaxFail, "max-failures", 3, "Max consecutive failures before considered stuck")
	agentHealthCmd.Flags().StringVar(&agentHealthAlert, "alert", "", "Send alert to channel when stuck agents detected (requires --detect-stuck)")
}

// AgentHealth represents the health status of an agent.
type AgentHealth struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	LastUpdated   string `json:"last_updated"`
	StaleDuration string `json:"stale_duration,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	StuckReason   string `json:"stuck_reason,omitempty"`
	StuckDetails  string `json:"stuck_details,omitempty"`
	TmuxAlive     bool   `json:"tmux_alive"`
	StateFresh    bool   `json:"state_fresh"`
	IsStuck       bool   `json:"is_stuck,omitempty"`
}

func runAgentHealth(cmd *cobra.Command, args []string) error {
	// Parse timeout duration (for validation and stuck detection)
	timeout, parseErr := time.ParseDuration(agentHealthTimeout)
	if parseErr != nil {
		return fmt.Errorf("invalid timeout format: %w", parseErr)
	}

	// Parse work timeout for stuck detection
	workTimeout, workParseErr := time.ParseDuration(agentHealthWorkTmout)
	if workParseErr != nil {
		return fmt.Errorf("invalid work-timeout format: %w", workParseErr)
	}

	// Validate --alert flag requires --detect-stuck
	if agentHealthAlert != "" && !agentHealthDetect {
		return fmt.Errorf("--alert requires --detect-stuck to be enabled")
	}

	ctx := cmd.Context()
	c, err := newDaemonClient(ctx)
	if err != nil {
		return err
	}

	// Get health data from daemon
	agentFilter := ""
	if len(args) > 0 {
		agentFilter = args[0]
	}
	healthData, healthErr := c.Agents.Health(ctx, agentHealthTimeout, agentFilter)
	if healthErr != nil {
		return fmt.Errorf("health check failed: %w", healthErr)
	}

	if agentFilter != "" && len(healthData) == 0 {
		return fmt.Errorf("agent %q not found (use 'mycel agent list' to see available agents)", agentFilter)
	}

	if len(healthData) == 0 {
		fmt.Println("No agents found")
		return nil
	}

	// Convert to AgentHealth structs
	healthResults := make([]AgentHealth, 0, len(healthData))
	for _, h := range healthData {
		healthResults = append(healthResults, AgentHealth{
			Name:          h.Name,
			Role:          h.Role,
			Status:        h.Status,
			LastUpdated:   h.LastUpdated,
			StaleDuration: h.StaleDuration,
			ErrorMessage:  h.ErrorMessage,
			TmuxAlive:     h.TmuxAlive,
			StateFresh:    h.StateFresh,
		})
	}

	// Stuck detection via daemon event log
	if agentHealthDetect {
		stuckConfig := client.StuckConfig{
			ActivityTimeout: timeout,
			WorkTimeout:     workTimeout,
			MaxFailures:     agentHealthMaxFail,
		}

		for i := range healthResults {
			agentEvents, readErr := c.Events.ListByAgent(ctx, healthResults[i].Name)
			if readErr != nil {
				log.Warn("failed to read agent events", "agent", healthResults[i].Name, "error", readErr)
				continue
			}

			stuck := client.DetectStuck(agentEvents, stuckConfig)
			if stuck.IsStuck {
				healthResults[i].IsStuck = true
				healthResults[i].StuckReason = string(stuck.Reason)
				healthResults[i].StuckDetails = stuck.Details
				if healthResults[i].Status == "healthy" || healthResults[i].Status == "degraded" {
					healthResults[i].Status = "stuck"
					healthResults[i].ErrorMessage = stuck.Details
				}
			}
		}
	}

	// Send alert to channel if --alert is set and there are stuck agents
	if agentHealthAlert != "" {
		if alertErr := sendStuckAlertViaClient(ctx, c, agentHealthAlert, healthResults); alertErr != nil {
			log.Warn("failed to send stuck alert", "error", alertErr)
		}
	}

	// Output
	if agentHealthJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(healthResults)
	}

	// Table output
	fmt.Printf("%-15s %-12s %-10s %-8s %-8s %s\n", "AGENT", "ROLE", "STATUS", "TMUX", "FRESH", "LAST UPDATED")
	fmt.Println(strings.Repeat("-", 75))

	for _, h := range healthResults {
		tmuxStr := "✗"
		if h.TmuxAlive {
			tmuxStr = "✓"
		}
		freshStr := "✗"
		if h.StateFresh {
			freshStr = "✓"
		}

		statusColor := h.Status
		switch h.Status {
		case "healthy":
			statusColor = ui.GreenText(h.Status)
		case "degraded":
			statusColor = ui.YellowText(h.Status)
		case "unhealthy":
			statusColor = ui.RedText(h.Status)
		case "stuck":
			statusColor = ui.MagentaText(h.Status)
		}

		fmt.Printf("%-15s %-12s %-10s %-8s %-8s %s\n",
			h.Name,
			h.Role,
			statusColor,
			tmuxStr,
			freshStr,
			h.LastUpdated,
		)

		if h.ErrorMessage != "" {
			fmt.Printf("  └─ %s\n", h.ErrorMessage)
		}
	}

	// Summary
	var healthy, degraded, unhealthy, stuck int
	for _, h := range healthResults {
		switch h.Status {
		case "healthy":
			healthy++
		case "degraded":
			degraded++
		case "unhealthy":
			unhealthy++
		case "stuck":
			stuck++
		}
	}
	if agentHealthDetect {
		fmt.Printf("\nSummary: %d healthy, %d degraded, %d unhealthy, %d stuck (threshold: %s, work-timeout: %s)\n",
			healthy, degraded, unhealthy, stuck, timeout, agentHealthWorkTmout)
	} else {
		fmt.Printf("\nSummary: %d healthy, %d degraded, %d unhealthy (threshold: %s)\n",
			healthy, degraded, unhealthy, timeout)
	}

	return nil
}

// sendStuckAlertViaClient sends an alert to the specified channel via daemon client.
func sendStuckAlertViaClient(ctx context.Context, c *client.Client, channelName string, healthResults []AgentHealth) error {
	// Collect stuck agents
	var stuckAgents []AgentHealth
	for _, h := range healthResults {
		if h.IsStuck || h.Status == "stuck" {
			stuckAgents = append(stuckAgents, h)
		}
	}

	if len(stuckAgents) == 0 {
		return nil
	}

	// Build alert message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚠️ ALERT: %d stuck agent(s) detected\n", len(stuckAgents)))
	for _, h := range stuckAgents {
		reason := h.StuckReason
		if reason == "" {
			reason = "unknown"
		}
		details := h.StuckDetails
		if details == "" {
			details = h.ErrorMessage
		}
		sb.WriteString(fmt.Sprintf("  • %s (%s): %s - %s\n", h.Name, h.Role, reason, details))
	}

	message := sb.String()

	// Send via daemon channel API
	_, sendErr := c.Channels.Send(ctx, channelName, "mycel-health", message)
	if sendErr != nil {
		return fmt.Errorf("failed to send alert to channel %q: %w", channelName, sendErr)
	}

	fmt.Printf("Alert sent to channel %q\n", channelName)
	return nil
}
