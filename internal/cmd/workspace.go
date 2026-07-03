package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/ui"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// workspaceCmd is the parent command for workspace operations
var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Aliases: []string{"ws"},
	Short:   "Manage mycel workspaces",
	Long: `Manage mycel workspaces: info, config, logs, list.

Examples:
  mycel workspace info                   # Show workspace details
  mycel workspace status                 # Show agents and health
  mycel workspace config show            # Show workspace config
  mycel workspace config set KEY VAL     # Set config value
  mycel workspace list                   # List discovered workspaces
  mycel workspace list --scan ~/Projects # Scan additional paths
  mycel workspace discover               # Discover and register new workspaces`,
}

// workspaceInfoCmd shows detailed workspace information.
var workspaceInfoCmd = &cobra.Command{
	Use:     "info",
	Aliases: []string{"i"},
	Short:   "Show workspace information",
	Long: `Display detailed information about the current workspace.

Shows workspace name, path, version, runtime backend, role count,
and agent summary.

Examples:
  mycel workspace info         # Human-readable output
  mycel workspace info --json  # JSON output`,
	RunE: runWorkspaceInfo,
}

// workspaceStatusCmd shows workspace agent health overview.
var workspaceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace status and agent health",
	Long: `Show a health overview of the workspace: running agents, idle agents,
config validity, and uptime.

Examples:
  mycel workspace status         # Status overview
  mycel workspace status --json  # JSON output`,
	RunE: runWorkspaceStatus,
}

// workspaceConfigCmd groups config management subcommands.
var workspaceConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage workspace configuration",
	Long: `Manage workspace configuration (preferences.json).

Examples:
  mycel workspace config show                    # Show full config
  mycel workspace config get providers.default   # Get a value
  mycel workspace config set providers.default claude # Set a value
  mycel workspace config validate                # Validate config
  mycel workspace config edit                    # Open in $EDITOR`,
	RunE: runConfigShow,
}

var workspaceConfigShowCmd = &cobra.Command{
	Use:   "show [key]",
	Short: "Show configuration",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigShow,
}

var workspaceConfigGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var workspaceConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var workspaceConfigValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	RunE:  runConfigValidate,
}

var workspaceConfigEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration file in $EDITOR",
	RunE:  runConfigEdit,
}

// workspaceListCmd lists all discovered workspaces
var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered workspaces",
	Long: `List all mycel workspaces on this machine.

Searches:
  - Global registry (~/.mycel/workspaces.json)
  - Common directories (~/Projects, ~/Developer, ~/dev, ~/code, ~/repos, ~/src)
  - Additional paths specified with --scan

Examples:
  mycel workspace list                    # List all workspaces
  mycel workspace list --json             # Output as JSON
  mycel workspace list --scan ~/work      # Include additional path
  mycel workspace list --no-cache         # Skip registry, scan only`,
	RunE: runWorkspaceList,
}

// workspaceDiscoverCmd discovers and registers new workspaces
var workspaceDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover and register workspaces",
	Long: `Scan filesystem for mycel workspaces and add them to the registry.

This updates ~/.mycel/workspaces.json with newly found workspaces.

Examples:
  mycel workspace discover                # Scan default locations
  mycel workspace discover --scan ~/work  # Include additional path`,
	RunE: runWorkspaceDiscover,
}

// workspaceAddCmd manually adds a workspace to the registry
// Issue #1218: Multi-workspace orchestration
var workspaceAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a workspace to the registry",
	Long: `Register a workspace in the global registry for quick access.

Examples:
  mycel workspace add .                        # Add current directory
  mycel workspace add ~/projects/frontend      # Add by path
  mycel workspace add . --alias fe             # Add with short alias
  mycel workspace add ~/api --alias backend    # Add with alias`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspaceAdd,
}

// workspaceRemoveCmd removes a workspace from the registry
var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove <alias|path>",
	Short: "Remove a workspace from the registry",
	Long: `Unregister a workspace from the global registry.

This does not delete the workspace, just removes it from the registry.

Examples:
  mycel workspace remove fe                    # Remove by alias
  mycel workspace remove ~/projects/frontend   # Remove by path`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspaceRemove,
}

// workspaceSwitchCmd sets the active workspace
var workspaceSwitchCmd = &cobra.Command{
	Use:   "switch <alias|path>",
	Short: "Switch active workspace",
	Long: `Set the active workspace for cross-workspace operations.

Examples:
  mycel workspace switch fe                    # Switch by alias
  mycel workspace switch ~/projects/frontend   # Switch by path
  mycel workspace switch --clear               # Clear active workspace`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkspaceSwitch,
}

// workspaceUpCmd starts all agents defined in the roster config.
var workspaceUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Start all roster agents",
	Long: `Start all agents defined in [roster] of preferences.json.

Agents that are already running are skipped. Missing role files are
created from built-in defaults automatically.

Examples:
  mycel workspace up          # Start roster agents
  bc ws up                 # Short alias`,
	RunE: runWorkspaceUp,
}

func init() {
	// List command flags
	workspaceListCmd.Flags().StringSlice("scan", nil, "Additional paths to scan")
	workspaceListCmd.Flags().Bool("no-cache", false, "Skip registry, scan filesystem only")
	workspaceListCmd.Flags().Int("depth", 3, "Maximum scan depth")

	// Discover command flags
	workspaceDiscoverCmd.Flags().StringSlice("scan", nil, "Additional paths to scan")
	workspaceDiscoverCmd.Flags().Int("depth", 3, "Maximum scan depth")

	// Add command flags (#1218)
	workspaceAddCmd.Flags().String("alias", "", "Short alias for quick access")

	// Switch command flags (#1218)
	workspaceSwitchCmd.Flags().Bool("clear", false, "Clear active workspace")

	// Config subcommands — reuse root-level config handlers
	workspaceConfigCmd.AddCommand(workspaceConfigShowCmd)
	workspaceConfigCmd.AddCommand(workspaceConfigGetCmd)
	workspaceConfigCmd.AddCommand(workspaceConfigSetCmd)
	workspaceConfigCmd.AddCommand(workspaceConfigValidateCmd)
	workspaceConfigCmd.AddCommand(workspaceConfigEditCmd)

	// Add subcommands
	workspaceCmd.AddCommand(workspaceInfoCmd)
	workspaceCmd.AddCommand(workspaceStatusCmd)
	workspaceCmd.AddCommand(workspaceUpCmd)
	workspaceCmd.AddCommand(workspaceConfigCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceDiscoverCmd)
	workspaceCmd.AddCommand(workspaceAddCmd)
	workspaceCmd.AddCommand(workspaceRemoveCmd)
	workspaceCmd.AddCommand(workspaceSwitchCmd)

	// Register with root
	rootCmd.AddCommand(workspaceCmd)
}

func runWorkspaceUp(_ *cobra.Command, _ []string) error {
	if _, err := requireWorkspace(); err != nil {
		return err
	}

	// Roster was removed from settings. bc up starts infrastructure containers
	// individually via the up command (internal/cmd/up.go).
	fmt.Println("Use 'mycel up --port <port>' to start bc infrastructure.")
	return nil
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	scanPaths, _ := cmd.Flags().GetStringSlice("scan")
	noCache, _ := cmd.Flags().GetBool("no-cache")
	maxDepth, _ := cmd.Flags().GetInt("depth")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	opts := workspace.DiscoverOptions{
		IncludeCached: !noCache,
		ScanHome:      true,
		ScanPaths:     scanPaths,
		MaxDepth:      maxDepth,
	}

	workspaces, err := workspace.Discover(opts)
	if err != nil {
		return fmt.Errorf("failed to discover workspaces: %w", err)
	}

	if jsonOutput {
		output := struct {
			Workspaces []workspace.DiscoveredWorkspace `json:"workspaces"`
		}{
			Workspaces: workspaces,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Text output
	if len(workspaces) == 0 {
		fmt.Println("No workspaces found")
		return nil
	}

	fmt.Printf("Found %d workspace(s):\n\n", len(workspaces))
	for _, ws := range workspaces {
		icon := "📁"
		if ws.IsV2 {
			icon = "📦"
		}
		source := ""
		if ws.FromCache {
			source = " (registered)"
		}
		fmt.Printf("  %s %s%s\n", icon, ws.Name, source)
		fmt.Printf("     %s\n", ws.Path)
	}

	return nil
}

func runWorkspaceDiscover(cmd *cobra.Command, args []string) error {
	scanPaths, _ := cmd.Flags().GetStringSlice("scan")
	maxDepth, _ := cmd.Flags().GetInt("depth")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	opts := workspace.DiscoverOptions{
		IncludeCached: true,
		ScanHome:      true,
		ScanPaths:     scanPaths,
		MaxDepth:      maxDepth,
	}

	newCount, err := workspace.DiscoverAndRegister(opts)
	if err != nil {
		return fmt.Errorf("failed to discover workspaces: %w", err)
	}

	if jsonOutput {
		output := struct {
			NewWorkspaces int `json:"new_workspaces"`
		}{
			NewWorkspaces: newCount,
		}
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(output)
	}

	if newCount == 0 {
		fmt.Println("No new workspaces found")
	} else {
		fmt.Printf("Registered %d new workspace(s)\n", newCount)
	}

	return nil
}

// runWorkspaceAdd handles the workspace add command.
// Issue #1218: Multi-workspace orchestration.
func runWorkspaceAdd(cmd *cobra.Command, args []string) error {
	path := args[0]
	alias, _ := cmd.Flags().GetString("alias")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Verify it's a workspace
	if !workspace.IsWorkspace(absPath) {
		return fmt.Errorf("not a mycel workspace: %s (no .bc directory found)", absPath)
	}

	// Load workspace to get name
	ws, err := workspace.Load(absPath)
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}
	name := ws.Name()

	// Load registry and add
	reg, err := workspace.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if err := reg.RegisterWithAlias(absPath, name, alias); err != nil {
		return err
	}

	if err := reg.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	if jsonOutput {
		output := struct {
			Path  string `json:"path"`
			Name  string `json:"name"`
			Alias string `json:"alias,omitempty"`
		}{
			Path:  absPath,
			Name:  name,
			Alias: alias,
		}
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(output)
	}

	if alias != "" {
		fmt.Printf("Added workspace '%s' (%s) as '%s'\n", name, absPath, alias)
	} else {
		fmt.Printf("Added workspace '%s' (%s)\n", name, absPath)
	}

	return nil
}

// runWorkspaceRemove handles the workspace remove command.
func runWorkspaceRemove(cmd *cobra.Command, args []string) error {
	identifier := args[0]
	jsonOutput, _ := cmd.Flags().GetBool("json")

	reg, err := workspace.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	entry := reg.FindByNameOrAlias(identifier)
	if entry == nil {
		return fmt.Errorf("workspace not found: %s", identifier)
	}

	// Store info before removal
	removedPath := entry.Path
	removedName := entry.Name

	reg.Unregister(entry.Path)

	// Clear active if this was the active workspace
	if active := reg.GetActive(); active != nil && active.Path == removedPath {
		_ = reg.SetActive("")
	}

	if err := reg.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	if jsonOutput {
		output := struct {
			Removed string `json:"removed"`
			Name    string `json:"name"`
		}{
			Removed: removedPath,
			Name:    removedName,
		}
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(output)
	}

	fmt.Printf("Removed workspace '%s' from registry\n", removedName)
	return nil
}

// runWorkspaceSwitch handles the workspace switch command.
func runWorkspaceSwitch(cmd *cobra.Command, args []string) error {
	clearActive, _ := cmd.Flags().GetBool("clear")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	reg, err := workspace.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if clearActive || len(args) == 0 {
		if err := reg.SetActive(""); err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
			return fmt.Errorf("failed to save registry: %w", err)
		}
		if jsonOutput {
			fmt.Println("{\"active\": null}")
		} else {
			fmt.Println("Cleared active workspace")
		}
		return nil
	}

	identifier := args[0]
	if err := reg.SetActive(identifier); err != nil {
		return err
	}

	if err := reg.Save(); err != nil {
		return fmt.Errorf("failed to save registry: %w", err)
	}

	active := reg.GetActive()
	if jsonOutput {
		output := struct {
			Active string `json:"active"`
			Path   string `json:"path"`
			Name   string `json:"name"`
		}{
			Active: reg.Active,
			Path:   active.Path,
			Name:   active.Name,
		}
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(output)
	}

	fmt.Printf("Switched to workspace '%s' (%s)\n", active.Name, active.Path)
	return nil
}

// runWorkspaceInfo displays detailed workspace information.
func runWorkspaceInfo(cmd *cobra.Command, _ []string) error {
	ws, err := requireWorkspace()
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	// Get agent count via daemon client
	agentCount := 0
	if c, clientErr := newDaemonClient(cmd.Context()); clientErr == nil {
		if agents, listErr := c.Agents.List(cmd.Context()); listErr == nil {
			agentCount = len(agents)
		}
	}

	// Count roles
	roleCount := 0
	if entries, readErr := os.ReadDir(ws.RolesDir()); readErr == nil {
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
				roleCount++
			}
		}
	}

	backend := "tmux"
	if ws.Config != nil && ws.Config.Runtime.Default != "" {
		backend = ws.Config.Runtime.Default
	}

	if jsonOutput {
		info := struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			StateDir    string `json:"state_dir"`
			Backend     string `json:"backend"`
			Version     int    `json:"version"`
			RoleCount   int    `json:"role_count"`
			AgentCount  int    `json:"agent_count"`
			ConfigValid bool   `json:"config_valid"`
		}{
			Name:       ws.Name(),
			Path:       ws.RootDir,
			StateDir:   ws.StateDir(),
			Version:    workspace.ConfigVersion,
			Backend:    backend,
			RoleCount:  roleCount,
			AgentCount: agentCount,
		}
		if ws.Config != nil {
			info.ConfigValid = ws.Config.Validate() == nil
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	fmt.Println(ui.BoldText("Workspace Info"))
	fmt.Println()
	fmt.Printf("  %-18s %s\n", "Name:", ws.Name())
	fmt.Printf("  %-18s %s\n", "Path:", ws.RootDir)
	fmt.Printf("  %-18s %s\n", "State dir:", ws.StateDir())
	fmt.Printf("  %-18s v%d\n", "Version:", workspace.ConfigVersion)
	fmt.Printf("  %-18s %s\n", "Runtime:", backend)
	fmt.Printf("  %-18s %d\n", "Roles:", roleCount)
	fmt.Printf("  %-18s %d\n", "Agents:", agentCount)

	if ws.Config != nil {
		if err := ws.Config.Validate(); err != nil {
			fmt.Printf("  %-18s %s\n", "Config:", ui.RedText("invalid — "+err.Error()))
		} else {
			fmt.Printf("  %-18s %s\n", "Config:", ui.GreenText("valid"))
		}
	}

	fmt.Println()
	return nil
}

// runWorkspaceStatus shows a health overview: agent counts and state breakdown.
func runWorkspaceStatus(cmd *cobra.Command, _ []string) error {
	ws, err := requireWorkspace()
	if err != nil {
		return err
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	c, clientErr := newDaemonClient(cmd.Context())
	if clientErr != nil {
		return clientErr
	}

	agents, listErr := c.Agents.List(cmd.Context())
	if listErr != nil {
		return fmt.Errorf("list agents: %w", listErr)
	}

	var running, idle, working, stopped int
	for _, a := range agents {
		switch a.State {
		case "working":
			working++
			running++
		case "idle", "starting":
			idle++
			running++
		case "stopped", "error", "done":
			stopped++
		default:
			stopped++
		}
	}

	configValid := ws.Config != nil && ws.Config.Validate() == nil

	if jsonOutput {
		out := struct { //nolint:govet // fieldalignment: inline struct for JSON, alignment not critical
			Workspace   string `json:"workspace"`
			Path        string `json:"path"`
			Total       int    `json:"total"`
			Running     int    `json:"running"`
			Working     int    `json:"working"`
			Idle        int    `json:"idle"`
			Stopped     int    `json:"stopped"`
			ConfigValid bool   `json:"config_valid"`
		}{
			Workspace:   ws.Name(),
			Path:        ws.RootDir,
			Total:       len(agents),
			Running:     running,
			Working:     working,
			Idle:        idle,
			Stopped:     stopped,
			ConfigValid: configValid,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Header
	fmt.Printf("%s  %s\n", ui.BoldText(ws.Name()), ui.DimText(ws.RootDir))
	fmt.Println()

	// Config
	cfgStatus := ui.GreenText("✓ valid")
	if !configValid {
		cfgStatus = ui.RedText("✗ invalid")
	}
	fmt.Printf("  %-18s %s\n", "Config:", cfgStatus)

	// Agents
	fmt.Printf("  %-18s %d total  %d running  %d working  %d stopped\n",
		"Agents:", len(agents), running, working, stopped)

	if len(agents) > 0 {
		fmt.Println()
		fmt.Printf("  %-16s %-12s %-10s %s\n", "AGENT", "ROLE", "STATE", "UPTIME")
		for _, a := range agents {
			uptime := "-"
			if a.State != "stopped" && a.State != "error" {
				uptime = formatDuration(time.Since(a.StartedAt))
			}
			fmt.Printf("  %-16s %-12s %-10s %s\n",
				a.Name, a.Role, a.State, uptime)
		}
	}

	fmt.Println()
	return nil
}
