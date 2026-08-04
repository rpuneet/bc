package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/container"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/ui"
)

// newAgentManager creates an agent manager with the appropriate runtime backend.
// Uses global config to determine the default backend. Both tmux and docker
// backends are always available so agents can use either runtime.
func newAgentManager(h *home.Home) *agent.Manager {
	backend := ""
	if h.Config != nil {
		backend = h.Config.Runtime.Default
	}

	var mgr *agent.Manager
	if backend == "docker" {
		var homeCfg home.DockerRuntimeConfig
		if h.Config != nil {
			homeCfg = h.Config.Runtime.Docker
		}
		dockerCfg := container.ConfigFromHome(homeCfg)
		be, err := container.NewBackend(dockerCfg, agent.DefaultSessionPrefix, h.RootDir, provider.DefaultRegistry)
		if err != nil {
			log.Warn("Docker unavailable, falling back to tmux", "error", err)
		} else {
			mgr = agent.NewManagerWithRuntime(h.AgentsDir(), h.RootDir, be, "docker")
		}
	}
	if mgr == nil {
		mgr = agent.NewManagerWithRepo(h.AgentsDir(), h.RootDir)
	}
	if h.Config != nil {
		mgr.ApplyConfig(h.Config)
	}
	return mgr
}

// agentCmd is the parent command for all agent operations
var agentCmd = &cobra.Command{
	Use:     "agent",
	Aliases: []string{"ag"},
	Short:   "Manage mycel agents",
	Long: `Manage mycel agent lifecycle: create, list, attach, peek, stop, send.

Examples:
  mycel agent list                          # List all agents
  mycel agent create eng-01 --template engineer # Create new agent
  mycel agent attach eng-01                 # Attach to agent session
  mycel agent peek eng-01                   # View recent output
  mycel agent send eng-01 "run tests"       # Send message to agent
  mycel agent stop eng-01                   # Stop agent
  mycel agent broadcast "check status"      # Send to all agents
  mycel agent send-pattern "eng-*" "test"   # Send to matching agents
  mycel agent                               # List all agents (same as mycel agent list)
  mycel agent send-pattern "eng-*" "hello"  # Send to matching agents`,
	// #925: Default to list for consistency with mycel channel
	RunE: runAgentList,
}

// agentCreateCmd creates a new agent.
var agentCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new agent",
	Long: `Create and start a new agent.

If no name is provided, a random memorable name is generated (e.g., swift-falcon).
Agents are configured via templates (markdown files at ~/.mycel/templates/).
Use --copy to clone settings from an existing agent.

Examples:
  mycel agent create                              # Random name, base template
  mycel agent create worker-01                    # Explicit name, base template
  mycel agent create eng-01 --template engineer   # Use engineer template
  mycel agent create qa-01 --tool cursor          # Base template with Cursor
  mycel agent create clone-01 --copy swift-hawk   # Copy config from swift-hawk`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAgentCreate,
}

// agentListCmd lists all agents (enhanced mycel status)
var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	Long: `List all agents with their status, role, and current task.

Examples:
  mycel agent list          # List all agents
  mycel agent list --json   # Output as JSON
  mycel agent list --role engineer  # Filter by role`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unexpected argument %q. To filter by role, use: mycel agent list --role %s", args[0], args[0])
		}
		return nil
	},
	RunE: runAgentList,
}

// agentAttachCmd attaches to an agent session (replaces mycel attach)
var agentAttachCmd = &cobra.Command{
	Use:   "attach <agent>",
	Short: "Attach to an agent's session",
	Long: `Attach to an agent's tmux session for direct interaction.

Use Ctrl+b d to detach and return to your shell.

Examples:
  mycel agent attach eng-01   # Attach to eng-01`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentAttach,
}

// agentPeekCmd shows recent output from an agent
var agentPeekCmd = &cobra.Command{
	Use:   "peek <agent>",
	Short: "Show recent output from an agent",
	Long: `Capture and display recent output from an agent's session.

Examples:
  mycel agent peek eng-01              # Show last 500 lines
  mycel agent peek eng-01 --lines 100  # Show last 100 lines
  mycel agent peek eng-01 --follow     # Stream live output (Ctrl+C to stop)`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentPeek,
}

// agentShowCmd shows detailed information about an agent
var agentShowCmd = &cobra.Command{
	Use:   "show <agent>",
	Short: "Show agent details",
	Long: `Show detailed information about an agent.

Examples:
  mycel agent show eng-01       # Show eng-01 details
  mycel agent show eng-01 --json  # Output as JSON`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentShow,
}

// agentStartCmd starts a stopped agent (resurrects from saved state)
var agentStartCmd = &cobra.Command{
	Use:   "start <agent>",
	Short: "Start a stopped agent",
	Long: `Start a previously stopped agent from its saved state.

This resurrects the agent's tmux session and memory.
The agent must have been previously created and stopped.
By default, resumes the previous session if available.

The agent's tool (claude, agy, cursor, etc.) is fixed at creation time
and cannot be changed on restart. Use --runtime to switch infrastructure
backends (tmux vs docker) without changing the agent's identity.

Examples:
  mycel agent start eng-01                    # Start stopped agent (resumes session)
  mycel agent start eng-01 --runtime docker   # Override runtime backend`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentStart,
}

// agentStopCmd stops a single agent (different from mycel down which stops all)
var agentStopCmd = &cobra.Command{
	Use:   "stop <agent>",
	Short: "Stop an agent",
	Long: `Stop a specific agent and its tmux session.

Examples:
  mycel agent stop eng-01       # Stop eng-01
  mycel agent stop eng-01 --force  # Force stop`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentStop,
}

// agentSendCmd sends a message to an agent (replaces mycel send)
var agentSendCmd = &cobra.Command{
	Use:   "send <agent> <message>",
	Short: "Send a message to an agent",
	Long: `Send a message or command to an agent's session.

Use --preview to see what action will be taken before sending (Intent Preview).
This shows agent details and asks for confirmation.

Examples:
  mycel agent send eng-01 "run the tests"
  mycel agent send coordinator "check status"
  mycel agent send eng-01 "implement login" --preview  # Preview before sending`,
	Args: cobra.MinimumNArgs(2),
	RunE: runAgentSend,
}

// agentDeleteCmd permanently removes an agent
var agentDeleteCmd = &cobra.Command{
	Use:   "delete <agent>",
	Short: "Permanently delete an agent",
	Long: `Permanently delete an agent from the repo.

This removes the agent's tmux session, channel memberships,
and agent state. Memory is preserved by default for recovery.

Use --force to delete an agent without stopping it first.
Use --purge to also delete the agent's memory directory.

Examples:
  mycel agent delete eng-01              # Delete stopped agent (preserves memory)
  mycel agent delete eng-01 --force      # Force delete (any state)
  mycel agent delete eng-01 --purge      # Delete including memory
  mycel agent delete eng-01 --force --purge  # Force delete with full cleanup`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentDelete,
}

// agentRenameCmd renames an agent
var agentRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename an agent",
	Long: `Rename an agent to a new name.

This updates the agent's name and channel memberships.
By default, running agents cannot be renamed (use --force to override).

Examples:
  mycel agent rename eng-01 engineer-01
  mycel agent rename eng-01 eng-02 --force  # Rename running agent`,
	Args: cobra.ExactArgs(2),
	RunE: runAgentRename,
}

// agentHealthCmd is defined in agent_health.go (issue #1648)

// agentSessionsCmd lists session history for an agent
var agentSessionsCmd = &cobra.Command{
	Use:   "sessions <agent>",
	Short: "List session history for an agent",
	Long: `Show stored session IDs for an agent.

The current session ID (if captured) is listed first, followed by archived
session IDs from previous runs.

Examples:
  mycel agent sessions eng-01       # List session IDs
  mycel agent sessions eng-01 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentSessions,
}

var agentSessionsJSON bool

// agentBroadcastCmd sends a message to all running agents
var agentBroadcastCmd = &cobra.Command{
	Use:   "broadcast <message>",
	Short: "Send a message to all running agents",
	Long: `Broadcast a message to all running agents in the repo.

Examples:
  mycel agent broadcast "run tests"
  mycel agent broadcast "check status"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAgentBroadcast,
}

// agentSendPatternCmd sends a message to agents matching a pattern
var agentSendPatternCmd = &cobra.Command{
	Use:   "send-pattern <pattern> <message>",
	Short: "Send a message to agents matching a pattern",
	Long: `Send a message to all running agents whose names match the given pattern.

Pattern uses glob-style matching (* matches any characters).

Examples:
  mycel agent send-pattern "engineer-*" "run tests"
  mycel agent send-pattern "eng-0*" "check status"
  mycel agent send-pattern "*-lead" "review PRs"`,
	Args: cobra.MinimumNArgs(2),
	RunE: runAgentSendPattern,
}

// Flags
var (
	agentCreateTool     string
	agentStatsJSON      bool
	agentStatsLimit     int
	agentCreateRole     string
	agentCreateTemplate string
	agentCreateCopy     string
	agentCreateParent   string
	agentCreateTeam     string
	agentCreateEnv      string
	agentCreateRuntime  string
	agentCreateTask     string
	agentStartRuntime   string
	agentStartResume    string // explicit session ID to resume
	agentListRole       string
	agentListStatus     string
	agentListJSON       bool
	agentListFull       bool
	agentShowJSON       bool
	agentShowFull       bool
	agentPeekLines      int
	agentPeekFollow     bool
	agentStopForce      bool
	agentDeleteForce    bool
	agentDeletePurge    bool
	agentRenameForce    bool
	agentSendPreview    bool
	agentLogsSince      string
	// Health flags are defined in agent_health.go (issue #1648)
)

func init() {
	// Create flags — the --tool help derives from the provider registry so
	// the listed tools always match what is actually registered.
	agentCreateCmd.Flags().StringVar(&agentCreateTool, "tool", "",
		fmt.Sprintf("Agent tool (%s)", strings.Join(provider.DefaultRegistry.Names(), ", ")))
	agentCreateCmd.Flags().StringVar(&agentCreateRole, "role", "", "Agent role (default: base)")
	agentCreateCmd.Flags().StringVar(&agentCreateTemplate, "template", "", "Template name from ~/.mycel/templates/ (e.g. base, engineer)")
	agentCreateCmd.Flags().StringVar(&agentCreateCopy, "copy", "", "Copy settings from an existing agent")
	agentCreateCmd.Flags().StringVar(&agentCreateParent, "parent", "", "Parent agent ID")
	agentCreateCmd.Flags().StringVar(&agentCreateTeam, "team", "", "Team name (alphanumeric)")
	agentCreateCmd.Flags().StringVar(&agentCreateEnv, "env", "", "Path to env file (KEY=VALUE per line)")
	agentCreateCmd.Flags().StringVar(&agentCreateRuntime, "runtime", "", "Runtime backend override: tmux or docker")
	agentCreateCmd.Flags().StringVar(&agentCreateTask, "task", "", "Initial task recorded on the agent and delivered after spawn")

	// List flags
	agentListCmd.Flags().StringVar(&agentListRole, "role", "", "Filter by role")
	agentListCmd.Flags().StringVar(&agentListStatus, "status", "", "Filter by status (running, stopped, error)")
	agentListCmd.Flags().BoolVar(&agentListJSON, "json", false, "Output as JSON (compact by default)")
	agentListCmd.Flags().BoolVar(&agentListFull, "full", false, "Include full agent data including prompts (with --json)")

	// Show flags
	agentShowCmd.Flags().BoolVar(&agentShowJSON, "json", false, "Output as JSON (compact by default)")
	agentShowCmd.Flags().BoolVar(&agentShowFull, "full", false, "Include full agent data including prompts (with --json)")

	// Peek flags
	agentPeekCmd.Flags().IntVar(&agentPeekLines, "lines", 500, "Number of lines to show")
	agentPeekCmd.Flags().BoolVarP(&agentPeekFollow, "follow", "f", false, "Stream live output (like tail -f)")

	// Stop flags
	agentStopCmd.Flags().BoolVar(&agentStopForce, "force", false, "Force stop without cleanup")

	// Delete flags
	agentDeleteCmd.Flags().BoolVar(&agentDeleteForce, "force", false, "Force delete running agent without stopping first")
	agentDeleteCmd.Flags().BoolVar(&agentDeletePurge, "purge", false, "Also delete agent's memory directory")

	// Rename flags
	agentRenameCmd.Flags().BoolVar(&agentRenameForce, "force", false, "Rename even if agent is running")

	// Health flags are set up in agent_health.go via initAgentHealthFlags() (issue #1648)
	initAgentHealthFlags()

	// Send flags
	agentSendCmd.Flags().BoolVar(&agentSendPreview, "preview", false, "Show preview of action before sending (Intent Preview)")

	// Start flags
	agentStartCmd.Flags().StringVar(&agentStartRuntime, "runtime", "", "Runtime backend override: tmux or docker")
	agentStartCmd.Flags().StringVar(&agentStartResume, "resume", "", "Resume a specific session by ID (e.g. --resume cc78cadf-89ce-4820-ab6e-950afd2b6838)")

	// Sessions flags
	agentSessionsCmd.Flags().BoolVar(&agentSessionsJSON, "json", false, "Output as JSON")

	// Add shell completion for agent name arguments
	agentAttachCmd.ValidArgsFunction = CompleteAgentNames
	agentPeekCmd.ValidArgsFunction = CompleteAgentNames
	agentShowCmd.ValidArgsFunction = CompleteAgentNames
	agentStartCmd.ValidArgsFunction = CompleteAgentNames
	agentStopCmd.ValidArgsFunction = CompleteAgentNames
	agentSendCmd.ValidArgsFunction = CompleteAgentNames
	agentDeleteCmd.ValidArgsFunction = CompleteAgentNames
	agentRenameCmd.ValidArgsFunction = CompleteAgentNames
	agentSessionsCmd.ValidArgsFunction = CompleteAgentNames

	// Add subcommands
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentAttachCmd)
	agentCmd.AddCommand(agentPeekCmd)
	agentCmd.AddCommand(agentShowCmd)
	agentCmd.AddCommand(agentStartCmd)
	agentCmd.AddCommand(agentStopCmd)
	agentCmd.AddCommand(agentSendCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	agentCmd.AddCommand(agentRenameCmd)
	agentCmd.AddCommand(agentHealthCmd)
	agentCmd.AddCommand(agentSessionsCmd)
	agentCmd.AddCommand(agentBroadcastCmd)
	agentCmd.AddCommand(agentSendPatternCmd)
	agentCmd.AddCommand(agentAuthCmd)
	agentCmd.AddCommand(agentCostCmd)
	agentCmd.AddCommand(agentLogsCmd)
	agentCmd.AddCommand(agentStatsCmd)

	// Logs flags
	agentLogsCmd.Flags().StringVar(&agentLogsSince, "since", "", "Show events since duration (e.g., 1h, 30m)")

	// Stats flags
	agentStatsCmd.Flags().BoolVar(&agentStatsJSON, "json", false, "Output as JSON")
	agentStatsCmd.Flags().IntVar(&agentStatsLimit, "limit", 20, "Number of records to show")
	agentStatsCmd.ValidArgsFunction = CompleteAgentNames

	// Add parent command to root
	rootCmd.AddCommand(agentCmd)
}

func runAgentCreate(cmd *cobra.Command, args []string) error {
	// Validate agent name if provided
	var agentName string
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		agentName = strings.TrimSpace(args[0])
		if !isValidAgentName(agentName) {
			return fmt.Errorf("agent name %q contains invalid characters (use letters, numbers, dash, underscore)", agentName)
		}
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// --copy: resolve source agent's tool from an existing agent.
	tmpl := agentCreateTemplate
	toolName := agentCreateTool
	if agentCreateCopy != "" {
		if tmpl != "" {
			return fmt.Errorf("--copy and --template are mutually exclusive")
		}
		source, getErr := c.Agents.Get(cmd.Context(), agentCreateCopy)
		if getErr != nil {
			return fmt.Errorf("copy source agent %q: %w", agentCreateCopy, getErr)
		}
		if toolName == "" {
			toolName = source.Tool
		}
	}

	// Default template to "base" when nothing is specified.
	if tmpl == "" {
		tmpl = "base"
	}

	// Role is always "base" — kept for backward compat with the server
	// until the roles table is deleted in layout-v2.
	role := agentCreateRole
	if role == "" {
		role = "base"
	}

	// Generate name if not provided
	if agentName == "" {
		generated, genErr := c.Agents.GenerateName(cmd.Context())
		if genErr != nil {
			return fmt.Errorf("failed to generate agent name: %w", genErr)
		}
		agentName = generated
		fmt.Printf("Generated name: %s\n", agentName)
	}

	// Create via client
	label := tmpl
	if agentCreateCopy != "" {
		label = "copy of " + agentCreateCopy
	}
	fmt.Printf("Creating %s (%s)... ", agentName, label)
	info, createErr := c.Agents.Create(cmd.Context(), client.CreateAgentReq{
		Name:     agentName,
		Role:     role,
		Tool:     toolName,
		Runtime:  agentCreateRuntime,
		Parent:   agentCreateParent,
		Team:     agentCreateTeam,
		EnvFile:  agentCreateEnv,
		Template: tmpl,
		Task:     agentCreateTask,
	})
	if createErr != nil {
		fmt.Println("✗")
		return fmt.Errorf("failed to create %s: %w", agentName, createErr)
	}
	fmt.Printf("✓ (session: %s)\n", info.Session)

	fmt.Println()
	fmt.Println("Agent created successfully!")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Printf("  mycel agent attach %s    # Attach to session\n", agentName)
	fmt.Printf("  mycel agent send %s <msg> # Send message\n", agentName)
	fmt.Printf("  mycel agent peek %s       # View output\n", agentName)

	return nil
}

func runAgentList(cmd *cobra.Command, args []string) error {
	log.Debug("agent list command started", "role", agentListRole, "json", agentListJSON)

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	agentList, err := c.Agents.List(cmd.Context())
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	// Filter by role if specified
	if agentListRole != "" {
		filterRole, roleErr := parseRoleStr(agentListRole)
		if roleErr != nil {
			return roleErr
		}
		filtered := make([]client.AgentInfo, 0, len(agentList))
		for _, a := range agentList {
			if a.Role == filterRole {
				filtered = append(filtered, a)
			}
		}
		agentList = filtered
	}

	// Filter by status if specified
	if agentListStatus != "" {
		filtered := make([]client.AgentInfo, 0, len(agentList))
		for _, a := range agentList {
			if matchesAgentStatusStr(a.State, agentListStatus) {
				filtered = append(filtered, a)
			}
		}
		agentList = filtered
	}

	log.Debug("agents loaded", "count", len(agentList))

	if agentListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(agentList)
	}

	if len(agentList) == 0 {
		ui.Warning("No agents found")
		if agentListRole != "" {
			fmt.Printf("(filtered by role: %s)\n", agentListRole)
		}
		return nil
	}

	// Determine terminal width for task truncation
	termWidth := 80
	if w, _, termErr := term.GetSize(os.Stdout.Fd()); termErr == nil && w > 0 {
		termWidth = w
	}
	taskWidth := termWidth - 57
	if taskWidth < 20 {
		taskWidth = 20
	}

	// Use pkg/ui table for consistent formatting
	table := ui.NewTable("AGENT", "ROLE", "STATE", "UPTIME", "TASK")

	for _, a := range agentList {
		uptime := "-"
		if a.State != "stopped" {
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

		table.AddRow(a.Name, a.Role, stateStr, uptime, task)
	}

	table.Print()
	return nil
}

func runAgentAttach(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	h, err := getRepo()
	if err != nil {
		return errNoRepo(err)
	}

	mgr := newAgentManager(h)
	if loadErr := mgr.LoadState(); loadErr != nil {
		log.Warn("failed to load agent state", "error", loadErr)
	}

	if !mgr.RuntimeForAgent(agentName).HasSession(cmd.Context(), agentName) {
		return fmt.Errorf("agent %q not running", agentName)
	}

	fmt.Printf("Attaching to %s (use Ctrl+b d to detach)...\n", agentName)
	return mgr.AttachToAgent(cmd.Context(), agentName)
}

func runAgentPeek(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	// --follow mode: keep local tmux access
	if agentPeekFollow {
		h, err := getRepo()
		if err != nil {
			return errNoRepo(err)
		}

		mgr := newAgentManager(h)
		if loadErr := mgr.LoadState(); loadErr != nil {
			log.Warn("failed to load agent state", "error", loadErr)
		}

		a := mgr.GetAgent(agentName)
		if a == nil {
			return fmt.Errorf("agent %q not found (use 'mycel agent list' to see available agents)", agentName)
		}

		if a.State == "stopped" {
			return fmt.Errorf("agent %q is stopped (use 'mycel agent start %s' to start it)", agentName, agentName)
		}

		fmt.Printf("=== %s (following, Ctrl+C to stop) ===\n", agentName)

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		return mgr.FollowOutput(ctx, agentName, agentPeekLines, os.Stdout)
	}

	// Static peek: use daemon client
	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	output, peekErr := c.Agents.Peek(cmd.Context(), agentName, agentPeekLines)
	if peekErr != nil {
		return fmt.Errorf("failed to peek %s: %w", agentName, peekErr)
	}

	fmt.Printf("=== %s (last %d lines) ===\n", agentName, agentPeekLines)
	fmt.Println(output)

	return nil
}

func runAgentShow(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	a, err := c.Agents.Get(cmd.Context(), agentName)
	if err != nil {
		return fmt.Errorf("agent %q not found: %w", agentName, err)
	}

	// JSON output
	if agentShowJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(a)
	}

	// Human-readable output using pkg/ui
	pairs := []string{
		"Agent", a.Name,
		"Role", a.Role,
		"State", a.State,
	}
	if a.Team != "" {
		pairs = append(pairs, "Team", a.Team)
	}
	pairs = append(pairs, "Session", a.Session)
	if a.Task != "" {
		pairs = append(pairs, "Task", normalizeTask(a.Task))
	}
	if a.Tool != "" {
		pairs = append(pairs, "Tool", a.Tool)
	}
	if a.ParentID != "" {
		pairs = append(pairs, "Parent", a.ParentID)
	}
	if len(a.Children) > 0 {
		pairs = append(pairs, "Children", strings.Join(a.Children, ", "))
	}
	if a.SessionID != "" {
		pairs = append(pairs, "Session ID", a.SessionID)
	}
	pairs = append(pairs,
		"Created", a.CreatedAt.Format(time.RFC3339),
		"Started", a.StartedAt.Format(time.RFC3339),
	)
	if a.StoppedAt != nil {
		pairs = append(pairs, "Stopped", a.StoppedAt.Format(time.RFC3339))
	}
	pairs = append(pairs, "Updated", a.UpdatedAt.Format(time.RFC3339))
	ui.SimpleTable(pairs...)

	return nil
}

func runAgentStart(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Starting %s... ", agentName)
	a, startErr := c.Agents.Start(cmd.Context(), agentName, agentStartRuntime, agentStartResume)
	if startErr != nil {
		fmt.Println("✗")
		return fmt.Errorf("failed to start %s: %w", agentName, startErr)
	}
	fmt.Printf("✓ (session: %s)\n", a.Session)

	return nil
}

func runAgentStop(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Stopping %s... ", agentName)
	if stopErr := c.Agents.Stop(cmd.Context(), agentName); stopErr != nil {
		fmt.Println("✗")
		return fmt.Errorf("failed to stop %s: %w", agentName, stopErr)
	}
	fmt.Println("✓")

	return nil
}

func runAgentSend(cmd *cobra.Command, args []string) error {
	agentName := args[0]
	message := strings.TrimSpace(strings.Join(args[1:], " "))
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	// Intent Preview: show what will happen and ask for confirmation
	if agentSendPreview {
		a, _ := c.Agents.Get(cmd.Context(), agentName)

		fmt.Println()
		fmt.Println("╭─────────────────────────────────────────────────────────────╮")
		fmt.Println("│                     Intent Preview                          │")
		fmt.Println("╰─────────────────────────────────────────────────────────────╯")
		fmt.Println()

		if a != nil {
			fmt.Printf("  Agent:    %s\n", a.Name)
			fmt.Printf("  Role:     %s\n", a.Role)
			fmt.Printf("  State:    %s\n", a.State)
			if a.Team != "" {
				fmt.Printf("  Team:     %s\n", a.Team)
			}
			if a.Tool != "" {
				fmt.Printf("  Tool:     %s\n", a.Tool)
			}
			if a.Task != "" {
				fmt.Printf("  Current:  %s\n", normalizeTask(a.Task))
			}
		} else {
			fmt.Printf("  Agent:    %s\n", agentName)
		}
		fmt.Println()

		// Message to send
		fmt.Printf("  Message:  %s\n", message)
		fmt.Println()

		// Action summary
		fmt.Println("  Action:   Will send message to agent's tmux session")
		fmt.Printf("            The agent will process: %q\n", truncateMessage(message, 50))
		fmt.Println()

		// Confirmation
		fmt.Print("  Proceed? [y/N]: ")
		var response string
		if _, scanErr := fmt.Scanln(&response); scanErr != nil {
			return fmt.Errorf("send canceled")
		}
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Send canceled.")
			return nil
		}
		fmt.Println()
	}

	if sendErr := c.Agents.Send(cmd.Context(), agentName, message); sendErr != nil {
		return fmt.Errorf("failed to send to %s: %w", agentName, sendErr)
	}

	fmt.Printf("Sent to %s: %s\n", agentName, message)
	return nil
}

func runAgentDelete(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	// Confirm deletion (show what will happen)
	if !agentDeleteForce {
		fmt.Printf("Delete agent %q? This will remove:\n", agentName)
		fmt.Println("  - tmux session")
		fmt.Println("  - channel memberships")
		fmt.Println("  - agent state")
		if agentDeletePurge {
			fmt.Println("  - memory directory (--purge)")
		} else {
			fmt.Printf("  Note: Memory preserved at .mycel/memory/%s (use --purge to delete)\n", agentName)
		}
		fmt.Print("\nType 'yes' to confirm: ")

		var response string
		if _, scanErr := fmt.Scanln(&response); scanErr != nil {
			return fmt.Errorf("deletion canceled")
		}
		if strings.TrimSpace(strings.ToLower(response)) != "yes" {
			return fmt.Errorf("deletion canceled")
		}
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Deleting %s... ", agentName)
	if delErr := c.Agents.Delete(cmd.Context(), agentName, agentDeleteForce); delErr != nil {
		fmt.Println("✗")
		return fmt.Errorf("failed to delete %s: %w", agentName, delErr)
	}
	fmt.Println("✓")

	fmt.Printf("Agent '%s' has been permanently deleted.\n", agentName)

	// Purge memory directory if requested (local file operation)
	if agentDeletePurge {
		h, homeErr := getRepo()
		if homeErr == nil {
			memDir := filepath.Join(h.StateDir(), "memory", agentName)
			if purgeErr := os.RemoveAll(memDir); purgeErr != nil {
				fmt.Printf("Warning: failed to purge memory directory: %v\n", purgeErr)
			} else {
				fmt.Printf("Memory directory purged.\n")
			}
		}
	}
	return nil
}

func runAgentRename(cmd *cobra.Command, args []string) error {
	oldName := args[0]
	newName := args[1]

	if oldName == newName {
		return fmt.Errorf("old and new names are the same")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	fmt.Printf("Renaming agent %q to '%s'...\n", oldName, newName)
	if renameErr := c.Agents.Rename(cmd.Context(), oldName, newName); renameErr != nil {
		return fmt.Errorf("failed to rename agent: %w", renameErr)
	}

	fmt.Printf("\nAgent '%s' has been renamed to '%s'.\n", oldName, newName)
	return nil
}

func runAgentBroadcast(cmd *cobra.Command, args []string) error {
	message := strings.TrimSpace(strings.Join(args, " "))
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	sent, broadcastErr := c.Agents.Broadcast(cmd.Context(), message)
	if broadcastErr != nil {
		return fmt.Errorf("broadcast failed: %w", broadcastErr)
	}

	fmt.Printf("Broadcast sent to %d agents\n", sent)
	return nil
}

func runAgentSendPattern(cmd *cobra.Command, args []string) error {
	pattern := args[0]
	message := strings.TrimSpace(strings.Join(args[1:], " "))
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	result, sendErr := c.Agents.SendToPattern(cmd.Context(), pattern, message)
	if sendErr != nil {
		return fmt.Errorf("send-pattern failed: %w", sendErr)
	}

	if len(result.Matched) == 0 {
		fmt.Printf("No agents matching pattern %q found\n", pattern)
		return nil
	}

	for _, name := range result.Matched {
		fmt.Printf("  %s: sent\n", name)
	}

	fmt.Printf("\nSent to %d of %d matching agents (%d skipped, %d failed)\n", result.Sent, len(result.Matched), result.Skipped, result.Failed)
	return nil
}

// parseRoleStr parses and validates a role string, returning a plain string.
func parseRoleStr(roleStr string) (string, error) {
	roleStr = strings.ToLower(strings.TrimSpace(roleStr))
	if roleStr == "" {
		return "root", nil // Default to root if not specified
	}
	// "null" role is a special case - represents an agent with no system prompt
	if roleStr == "null" {
		return "null", nil
	}
	// All roles are now custom - loaded from .mycel/roles/<role>.md files
	// Just validate that the role name is sensible
	if !isValidRoleName(roleStr) {
		return "", fmt.Errorf("invalid role name %q (must be alphanumeric with hyphens)", roleStr)
	}
	return roleStr, nil
}

// isValidTeamName validates that a team name is alphanumeric with optional hyphens/underscores.
func isValidTeamName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		isLower := c >= 'a' && c <= 'z'
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		isAllowed := isLower || isUpper || isDigit || c == '-' || c == '_'
		if !isAllowed {
			return false
		}
	}
	return true
}

// isValidAgentName checks if an agent name contains only safe characters
func isValidAgentName(name string) bool {
	return isValidTeamName(name)
}

// matchesAgentStatusStr checks if an agent state string matches a status filter.
// Maps detailed internal states to the simplified 4-state model from #1918.
func matchesAgentStatusStr(state, status string) bool {
	switch status {
	case "running":
		return state == "idle" || state == "working" || state == "starting"
	case "stopped":
		return state == "stopped"
	case "error":
		return state == "error"
	case "starting":
		return state == "starting"
	default:
		// Allow matching by exact internal state name
		return state == status
	}
}

// agentCostCmd shows per-agent cost breakdown
var agentCostCmd = &cobra.Command{
	Use:   "cost <agent>",
	Short: "Show per-agent cost breakdown",
	Long: `Show the cost breakdown for a specific agent including tokens and USD cost.

Examples:
  mycel agent cost eng-01       # Show eng-01 cost
  mycel agent cost eng-01 --json  # Output as JSON`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentCost,
}

// agentLogsCmd shows agent event history
var agentLogsCmd = &cobra.Command{
	Use:   "logs <agent>",
	Short: "Show agent event history",
	Long: `Show the event log history for a specific agent.

Examples:
  mycel agent logs eng-01               # Show all events
  mycel agent logs eng-01 --since 1h    # Show events from last hour`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentLogs,
}

func runAgentCost(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	summary, costErr := c.Agents.Cost(cmd.Context(), agentName)
	if costErr != nil {
		fmt.Printf("Agent: %s\n", agentName)
		fmt.Println("No cost data available (cost tracking not enabled)")
		return nil
	}

	fmt.Printf("Agent: %s\n", agentName)
	fmt.Printf("  Input tokens:  %d\n", summary.InputTokens)
	fmt.Printf("  Output tokens: %d\n", summary.OutputTokens)
	fmt.Printf("  Total tokens:  %d\n", summary.TotalTokens)
	fmt.Printf("  Total cost:    $%.4f\n", summary.TotalCostUSD)
	fmt.Printf("  Requests:      %d\n", summary.RequestCount)

	return nil
}

func runAgentLogs(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	agentEvents, readErr := c.Events.ListByAgent(cmd.Context(), agentName)
	if readErr != nil {
		return fmt.Errorf("failed to read agent events: %w", readErr)
	}

	// Filter by --since if specified
	if agentLogsSince != "" {
		since, parseErr := time.ParseDuration(agentLogsSince)
		if parseErr != nil {
			return fmt.Errorf("invalid --since duration %q: %w", agentLogsSince, parseErr)
		}
		cutoff := time.Now().Add(-since)
		filtered := agentEvents[:0]
		for _, e := range agentEvents {
			if e.Timestamp.After(cutoff) {
				filtered = append(filtered, e)
			}
		}
		agentEvents = filtered
	}

	if len(agentEvents) == 0 {
		fmt.Printf("No events found for agent %q\n", agentName)
		return nil
	}

	fmt.Printf("=== Events for %s (%d total) ===\n\n", agentName, len(agentEvents))
	for _, e := range agentEvents {
		fmt.Printf("[%s] %s: %s\n", e.Timestamp.Format("15:04:05"), e.Type, e.Message)
	}

	return nil
}

// agentAuthCmd manages per-agent authentication for Docker containers.
var agentAuthCmd = &cobra.Command{
	Use:   "auth <agent-name>",
	Short: "Authenticate an agent for Docker containers",
	Long: `Run OAuth login for a specific agent. Each agent has its own isolated
credentials directory. Opens a browser for authentication.

Usage:
  mycel agent auth my-agent        # Login for a specific agent
  mycel agent auth my-agent status # Check auth status`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		fmt.Printf("Agent auth is handled inside the container.\n\n")
		fmt.Printf("To authenticate agent %q:\n", agentName)
		fmt.Printf("  1. Attach: mycel agent attach %s\n", agentName)
		fmt.Printf("  2. Run /login inside Claude Code\n")
		fmt.Printf("\nOr set ANTHROPIC_API_KEY in the repo env:\n")
		fmt.Printf("  mycel env set ANTHROPIC_API_KEY sk-ant-...\n")
		return nil
	},
}

func runAgentSessions(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	sessions, sessErr := c.Agents.Sessions(cmd.Context(), agentName)
	if sessErr != nil {
		return fmt.Errorf("failed to get sessions for %q: %w", agentName, sessErr)
	}

	if agentSessionsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	if len(sessions) == 0 {
		fmt.Printf("No session IDs stored for agent %s.\n", agentName)
		fmt.Printf("Session IDs are captured automatically when the agent stops.\n")
		return nil
	}

	fmt.Printf("Sessions for %s:\n\n", agentName)
	for _, s := range sessions {
		current := ""
		if s.Current {
			current = " " + ui.GreenText("(current)")
		}
		ts := ""
		if !s.Timestamp.IsZero() {
			ts = "  " + ui.DimText(s.Timestamp.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("  %s%s%s\n", s.ID, current, ts)
	}
	fmt.Printf("\nResume a session: mycel agent start %s --resume <id>\n", agentName)

	return nil
}

// agentStatsCmd shows Docker resource stats for a given agent.
var agentStatsCmd = &cobra.Command{
	Use:   "stats <name>",
	Short: "Show Docker resource stats for an agent",
	Long: `Display recorded Docker CPU and memory stats for an agent.

Stats are collected every 30 s by the daemon while the agent is running with a
Docker runtime backend. They are stored in the global mycel.db.

Examples:
  mycel agent stats eng-01              # Human-readable table
  mycel agent stats eng-01 --json       # JSON output
  mycel agent stats eng-01 --limit 50   # Show more records`,
	Args: cobra.ExactArgs(1),
	RunE: runAgentStats,
}

func runAgentStats(cmd *cobra.Command, args []string) error {
	agentName := args[0]

	c, err := newDaemonClient(cmd.Context())
	if err != nil {
		return err
	}

	records, err := c.Agents.Stats(cmd.Context(), agentName, agentStatsLimit)
	if err != nil {
		return fmt.Errorf("query stats: %w", err)
	}

	if agentStatsJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if records == nil {
			records = []*client.AgentStatsRecord{}
		}
		return enc.Encode(records)
	}

	if len(records) == 0 {
		fmt.Printf("No stats recorded for agent %s.\n", agentName)
		fmt.Println("Stats are collected when the agent is running with runtime=docker.")
		return nil
	}

	fmt.Printf("Stats for %s (newest first):\n\n", agentName)
	fmt.Printf("%-20s  %6s  %8s  %8s  %8s  %8s\n",
		"Time", "CPU%", "Mem(MB)", "MemLim", "NetRx", "NetTx")
	fmt.Println(strings.Repeat("-", 72))
	for _, r := range records {
		fmt.Printf("%-20s  %6.1f  %8.1f  %8.1f  %8.1f  %8.1f\n",
			r.CollectedAt.Format("2006-01-02 15:04:05"),
			r.CPUPct, r.MemUsedMB, r.MemLimitMB, r.NetRxMB, r.NetTxMB)
	}
	return nil
}
