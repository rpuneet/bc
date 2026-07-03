// Package cmd implements the mycel CLI commands.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/ui"
)

var (
	// Version info set from main.
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetVersionInfo sets the version information from build flags.
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

// rootCmd is the base command for mycel.
var rootCmd = &cobra.Command{
	Use:   "mycel",
	Short: "A simpler, more controllable agent orchestrator",
	Long: `mycel is a multi-agent orchestration system for AI coding assistants.

Coordinate multiple AI agents with predictable behavior and cost awareness.
Supports Claude Code, Cursor, Codex, and other AI coding tools.

Getting Started:
  mycel init                                 # Initialize workspace
  mycel up                                   # Start root agent
  mycel agent create eng-01 --role engineer  # Create engineer agent
  mycel status                               # View agent status
  mycel up                                   # Start server

Common Workflows:
  Start working:    mycel up && mycel status
  Monitor agents:   mycel status --activity
  Send message:     mycel channel send eng "message"
  Debug agent:      mycel logs --agent eng-01 --tail 50
  Cost check:       mycel cost show

Command Groups (with short aliases):
  agent                        Manage agents
  channel (ch)                 Communication channels
  cost (co)                    Cost tracking and budgets
  workspace (ws)               Workspace management
  doctor (dr)                  Health checks
  cron (cr)                    Scheduled tasks

Key Features:
  • Coordinate multiple AI coding agents in parallel
  • Isolated git worktrees per agent
  • Channel-based agent communication
  • Cost tracking and limits
  • Hierarchical agent roles (product-manager, manager, engineer)

Environment Variables:
  BC_AGENT_ID       Current agent name (set automatically in agent sessions)
  BC_AGENT_ROLE     Current agent role
  BC_WORKSPACE      Path to workspace root
  BC_AGENT_WORKTREE Path to agent's worktree
  BC_BIN            Path to mycel binary (default: mycel in PATH)
  BC_ROOT           Workspace root directory
  NO_COLOR          Disable colored output

Documentation: https://github.com/rpuneet/mycel
Full CLI reference: https://github.com/rpuneet/mycel/docs/cli.md`,
	// PersistentPreRun initializes logging based on flags.
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		verbose, err := cmd.Flags().GetBool("verbose")
		if err == nil {
			log.SetVerbose(verbose)
		}
	},
	// Run with no args: open home if initialized, else prompt for init
	RunE: runRoot,
}

// versionCmd shows version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long: `Print version, commit hash, and build date.

Examples:
  mycel version       # Show version info
  mycel --version     # Same as above (short flag)
  mycel -V            # Same as above`,
	Run: func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "mycel %s\n", version)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  commit: %s\n", commit)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  built:  %s\n", date)
	},
}

func init() {
	// Disable auto-generated completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().Bool("json", false, "Output in JSON format")

	// Version flag
	rootCmd.Flags().BoolP("version", "V", false, "Print version information")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// Root returns the root command for testing and extension.
func Root() *cobra.Command {
	return rootCmd
}

// runRoot handles the default bc command (no subcommand).
// If workspace is initialized → open TUI home
// If not initialized → prompt to init
// In non-interactive mode → show help
func runRoot(cmd *cobra.Command, args []string) error {
	// Check for version flag
	showVersion, err := cmd.Flags().GetBool("version")
	if err == nil && showVersion {
		versionCmd.Run(cmd, args)
		return nil
	}

	// Check if running in an interactive terminal
	// If not (e.g., piped input, test environment), show help
	if !term.IsTerminal(os.Stdin.Fd()) {
		return cmd.Help()
	}

	// Try to find workspace
	ws, err := getWorkspace()
	if err == nil && ws != nil {
		// Workspace exists - open TUI home
		log.Debug("workspace found, opening home", "root", ws.RootDir)
		return runHome(cmd, args)
	}

	// No workspace - prompt to initialize
	return promptInit(cmd)
}

// promptInit displays an interactive prompt to initialize a new workspace.
func promptInit(cmd *cobra.Command) error {
	fmt.Println()
	fmt.Printf("  %s\n", ui.BoldText("mycel - AI Agent Orchestration"))
	fmt.Println()
	fmt.Println("  No workspace found in current directory.")
	fmt.Println()
	fmt.Print("  Would you like to initialize a new workspace? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))

	// Default to yes if empty or 'y'
	if input == "" || input == "y" || input == "yes" {
		return runInteractiveInit(cmd)
	}

	// User said no - show help
	fmt.Println()
	return cmd.Help()
}

// runInteractiveInit runs an interactive workspace initialization.
func runInteractiveInit(cmd *cobra.Command) error {
	fmt.Println()
	fmt.Println("  Initializing mycel workspace...")
	fmt.Println()

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Run init with interactive nickname prompt
	return runInitInteractive(cmd, cwd)
}
