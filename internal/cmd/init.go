package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rpuneet/bc/pkg/agent"
	"github.com/rpuneet/bc/pkg/client"
	"github.com/rpuneet/bc/pkg/log"
	"github.com/rpuneet/bc/pkg/ui"
	"github.com/rpuneet/bc/pkg/workspace"
)

var (
	initQuick  bool
	initPreset string
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize a new bc v2 workspace",
	Long: `Initialize a new bc v2 workspace in the specified directory (or current directory).

This creates a .bc directory with v2 configuration for managing agents.

v2 workspace structure:
  .bc/
    settings.json  # Workspace configuration
    roles/         # Agent role definitions
      root.md      # Root agent role
    agents/        # Per-agent state files

Examples:
  bc init                        # Interactive wizard
  bc init --quick                # Quick init with defaults
  bc init --preset solo          # Use solo developer preset
  bc init --preset small-team    # Use small team preset
  bc init --preset full-team     # Use full team preset
  bc init ~/Projects/myapp       # Initialize specific directory`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVar(&initQuick, "quick", false, "Quick init with defaults (skip wizard)")
	initCmd.Flags().StringVar(&initPreset, "preset", "", "Use preset configuration (solo, small-team, full-team)")
}

// isV1Workspace checks if a directory has a v1 workspace (config.json).
func isV1Workspace(dir string) bool {
	configPath := filepath.Join(dir, ".bc", "config.json")
	_, err := os.Stat(configPath)
	return err == nil
}

// isV2Workspace checks if a directory has a v2 workspace (preferences.json,
// settings.json, or legacy config.toml). Checks both the legacy
// <dir>/.bc/ location and the M11+ global ~/.bc/workspaces/<id>/ dir.
func isV2Workspace(dir string) bool {
	bcDir := filepath.Join(dir, ".bc")
	for _, name := range []string{workspace.PreferencesFileName, workspace.LegacySettingsFileName} {
		if _, err := os.Stat(filepath.Join(bcDir, name)); err == nil {
			return true
		}
	}
	configPath := filepath.Join(bcDir, "config.toml")
	if _, err := os.Stat(configPath); err == nil {
		return true
	}
	// M11+ global location.
	if globalDir, err := workspace.GlobalStateDir(dir); err == nil {
		for _, name := range []string{workspace.PreferencesFileName, workspace.LegacySettingsFileName} {
			if _, err := os.Stat(filepath.Join(globalDir, name)); err == nil {
				return true
			}
		}
	}
	return false
}

func runInit(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	log.Debug("init command started", "dir", dir, "quick", initQuick, "preset", initPreset)

	// Handle preset flag
	if initPreset != "" {
		preset := WizardPreset(initPreset)
		switch preset {
		case PresetSolo, PresetSmallTeam, PresetFullTeam:
			return InitWithPreset(dir, preset)
		default:
			return fmt.Errorf("unknown preset: %s (valid: solo, small-team, full-team)", initPreset)
		}
	}

	// Handle quick flag - use solo preset with defaults
	if initQuick {
		return InitWithPreset(dir, PresetSolo)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory: %w", err)
	}
	log.Debug("resolved directory", "absDir", absDir)

	// Check for existing v2 workspace
	if isV2Workspace(absDir) {
		log.Debug("v2 workspace already exists", "dir", absDir)
		return fmt.Errorf("v2 workspace already initialized in %s", absDir)
	}

	// Check for existing v1 workspace
	if isV1Workspace(absDir) {
		log.Debug("v1 workspace detected", "dir", absDir)
		fmt.Fprintln(os.Stderr, "Warning: Existing v1 workspace detected.")
		fmt.Fprintln(os.Stderr, "bc v2 is a clean break - v1 data will not be migrated.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "To proceed:")
		fmt.Fprintln(os.Stderr, "  - Backup .bc/ if needed")
		fmt.Fprintln(os.Stderr, "  - Remove .bc/ directory")
		fmt.Fprintln(os.Stderr, "  - Run bc init again")
		return fmt.Errorf("cannot initialize: v1 workspace exists")
	}

	// Run interactive wizard
	return RunWizard(dir)
}

// initV2Workspace creates a new v2 workspace structure.
func initV2Workspace(rootDir string) error {
	// M11: runtime state lives at ~/.bc/workspaces/<id>/ — the project
	// directory stays a pristine git repo. Use the high-level Init()
	// which creates the global dir, writes preferences.json, seeds
	// roles, and registers the workspace.
	ws, err := workspace.Init(rootDir)
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	stateDir := ws.StateDir()

	fmt.Printf("Initialized bc v2 workspace in %s\n", rootDir)
	fmt.Printf("\n")
	fmt.Printf("  Runtime state: %s\n", stateDir)
	fmt.Printf("    preferences.json   # Workspace configuration\n")
	fmt.Printf("    agents/            # Agent state directory\n")
	fmt.Printf("    state.db           # Events, channels, roles\n")
	fmt.Printf("\n")
	fmt.Printf("  Default provider: %s\n", ws.Config.Providers.Default)
	fmt.Printf("\n")
	fmt.Printf("Next steps:\n")
	fmt.Printf("  bc up       # Start agents\n")
	fmt.Printf("  bc status   # Check agent status\n")

	return nil
}

// getWorkspace finds the current workspace.
// Supports both v1 (config.json) and v2 (settings.json) workspaces.
// Checks BC_WORKSPACE env var first (for agents in worktrees), then walks up directory tree.
func getWorkspace() (*workspace.Workspace, error) {
	// Check BC_WORKSPACE first (agents set this to point to main workspace)
	if wsPath := os.Getenv("BC_WORKSPACE"); wsPath != "" {
		return workspace.Load(wsPath)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return workspace.Find(cwd)
}

// errorAgentNotRunning returns an error message for commands that require BC_AGENT_ID.
func errorAgentNotRunning(commandUsage string) error {
	return fmt.Errorf("this command can only be run by agents in the bc system (use: bc agent send <agent-name> %q)", commandUsage)
}

// newDaemonClient creates a client connected to the bcd daemon.
// Returns an error if the daemon is not running.
// Checks for a valid workspace first to provide clear error messages.
func newDaemonClient(ctx context.Context) (*client.Client, error) {
	// Verify we're in a workspace before trying to connect to daemon
	if _, err := getWorkspace(); err != nil {
		return nil, errNotInWorkspace(err)
	}
	c := client.New("")
	if err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("bcd is not running — start it with 'bcd' or 'bc up' first\n(%w)", err)
	}
	return c, nil
}

// errNotInWorkspace returns an actionable error for commands that require a bc workspace.
func errNotInWorkspace(err error) error {
	if err != nil {
		return fmt.Errorf("not in a bc workspace (run 'bc init' to initialize one): %w", err)
	}
	return fmt.Errorf("not in a bc workspace. Run 'bc init' in your project directory to create one")
}

// requireWorkspace returns the current workspace or an actionable error.
// This is a convenience wrapper around getWorkspace() with standard error handling.
func requireWorkspace() (*workspace.Workspace, error) {
	ws, err := getWorkspace()
	if err != nil {
		return nil, errNotInWorkspace(err)
	}
	return ws, nil
}

// WorkspaceContext holds workspace and agent manager for command handlers.
// Use withWorkspace() or withAgentManager() to create instances.
type WorkspaceContext struct {
	Workspace *workspace.Workspace
	Manager   *agent.Manager
}

// withWorkspace executes fn with the current workspace.
// Returns errNotInWorkspace if not in a bc workspace.
// Used by commands that only need workspace access (config, stats, etc.).
func withWorkspace(fn func(ws *workspace.Workspace) error) error { //nolint:unused // Will be used as commands migrate to this pattern
	ws, err := requireWorkspace()
	if err != nil {
		return err
	}
	return fn(ws)
}

// runInitInteractive runs an interactive workspace initialization with nickname prompt.
func runInitInteractive(_ *cobra.Command, dir string) error {
	log.Debug("interactive init started", "dir", dir)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory: %w", err)
	}

	// Check for existing workspaces
	if isV2Workspace(absDir) {
		return fmt.Errorf("workspace already initialized in %s", absDir)
	}
	if isV1Workspace(absDir) {
		fmt.Fprintln(os.Stderr, "Warning: Existing v1 workspace detected.")
		fmt.Fprintln(os.Stderr, "Run 'bc init' after removing .bc/ directory to migrate.")
		return fmt.Errorf("cannot initialize: v1 workspace exists")
	}

	// Prompt for nickname
	nickname, err := promptNickname()
	if err != nil {
		return err
	}

	// Initialize workspace with nickname
	return initV2WorkspaceWithNickname(absDir, nickname)
}

// promptNickname prompts the user for their display name.
func promptNickname() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("  Your nickname [%s]: ", "@bc")
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)

	// Normalize and validate
	nickname, err := workspace.NormalizeNickname(input)
	if err != nil {
		// Show helpful error
		fmt.Printf("  %s\n", ui.RedText(fmt.Sprintf("Error: %s", err)))
		fmt.Printf("  Using default: %s\n", "@bc")
		return "@bc", nil
	}

	// Show auto-correction if @ was added
	if input != "" && !strings.HasPrefix(input, "@") {
		fmt.Printf("  → Auto-corrected to %s\n", nickname)
	}

	return nickname, nil
}

// initV2WorkspaceWithNickname creates a new v2 workspace with a custom nickname.
// M11: runtime state is stored at ~/.bc/workspaces/<id>/ — the project
// directory is left as a pristine git repo.
func initV2WorkspaceWithNickname(rootDir string, nickname string) error {
	ws, err := workspace.Init(rootDir)
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	ws.Config.User.Name = nickname
	if saveErr := ws.Save(); saveErr != nil {
		return fmt.Errorf("save config: %w", saveErr)
	}
	stateDir := ws.StateDir()

	// Print success message
	fmt.Println()
	fmt.Printf("  %s Workspace initialized at %s\n", ui.GreenText("✓"), rootDir)
	fmt.Printf("  %s Nickname set to %s\n", ui.GreenText("✓"), nickname)
	fmt.Println()
	fmt.Printf("  Runtime state: %s\n", stateDir)
	fmt.Println("    preferences.json   # Workspace configuration")
	fmt.Println("    agents/            # Agent state directory")
	fmt.Println("    state.db           # Events, channels, roles")
	fmt.Println()

	// Bootstrap server daemons (non-fatal; warns if Docker unavailable)
	bootstrapServerDaemons(rootDir)

	fmt.Println("  Next steps:")
	fmt.Println("    bc          # Open the dashboard")
	fmt.Println("    bc up       # Start agents")
	fmt.Println("    bc status   # Check agent status")

	return nil
}
