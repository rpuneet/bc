// Package cmd implements the mycel CLI commands.
package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rpuneet/mycel/pkg/log"
)

var (
	// Version info set from main.
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// SetVersionInfo sets the version information from build flags, filling in
// from the build's recorded VCS stamp whatever the linker did not supply.
//
// A binary from `go build ./cmd/mycel` has no -X flags, so it used to identify
// itself as version "dev", commit "none" — and since /api/health substitutes the
// commit for a version of "dev", the About page reported a daemon whose version
// was literally "none". Go stamps the revision into every binary built inside a
// repository, so there is no need to report nothing.
func SetVersionInfo(v, c, d string) {
	version, commit, date = resolveVersionInfo(v, c, d)
}

// resolveVersionInfo fills placeholder build values in from the VCS stamp. Kept
// separate from the package variables it ends up in so it can be tested without
// writing to them.
func resolveVersionInfo(v, c, d string) (outV, outC, outD string) {
	if unstamped(c) {
		if rev, modified, ok := vcsStamp(); ok {
			c = rev
			if modified {
				c += "+dirty"
			}
		}
	}
	if unstamped(d) {
		if t, ok := vcsTime(); ok {
			d = t
		}
	}
	return v, c, d
}

// unstamped reports whether a build value is one of the placeholders that means
// "the linker did not set this".
func unstamped(v string) bool {
	switch v {
	case "", "none", "unknown", "dev":
		return true
	}
	return false
}

// vcsStamp returns the short revision Go recorded at build time, and whether the
// tree had uncommitted changes.
func vcsStamp() (rev string, modified, ok bool) {
	info, available := debug.ReadBuildInfo()
	if !available {
		return "", false, false
	}
	return stampFromSettings(info.Settings)
}

// stampFromSettings is vcsStamp's logic, separated from where the settings come
// from: a test cannot choose how its own binary was built, and a stamp read from
// this binary is absent in exactly the cases (linked worktrees, -buildvcs=false)
// that would silently turn the test into a skip.
func stampFromSettings(settings []debug.BuildSetting) (rev string, modified, ok bool) {
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return "", false, false
	}
	// Short enough to read in a header, long enough to paste into `git show`.
	if len(rev) > 8 {
		rev = rev[:8]
	}
	return rev, modified, true
}

// vcsTime returns the commit time Go recorded at build time.
func vcsTime() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	return timeFromSettings(info.Settings)
}

func timeFromSettings(settings []debug.BuildSetting) (string, bool) {
	for _, s := range settings {
		if s.Key == "vcs.time" && s.Value != "" {
			return s.Value, true
		}
	}
	return "", false
}

// rootCmd is the base command for mycel.
var rootCmd = &cobra.Command{
	Use:   "mycel",
	Short: "A simpler, more controllable agent orchestrator",
	Long: `mycel is a multi-agent orchestration system for AI coding assistants.

Coordinate multiple AI agents with predictable behavior and cost awareness.
Supports Claude Code, Cursor, Codex, and other AI coding tools.

Getting Started:
  mycel up                                   # Start the server (bootstraps ~/.mycel)
  mycel agent create eng-01 --role engineer  # Create engineer agent
  mycel status                               # View agent status

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
  config                       Configuration management
  doctor (dr)                  Health checks

Key Features:
  • Coordinate multiple AI coding agents in parallel
  • Isolated git worktrees per agent
  • Channel-based agent communication
  • Cost tracking and limits
  • Hierarchical agent roles (product-manager, manager, engineer)

Environment Variables:
  MYCEL_AGENT_ID       Current agent name (set automatically in agent sessions)
  MYCEL_AGENT_ROLE     Current agent role
  MYCEL_WORKSPACE      Path to the agent's repo root
  MYCEL_AGENT_WORKTREE Path to agent's worktree
  MYCEL_BIN            Path to mycel binary (default: mycel in PATH)
  MYCEL_ROOT           Override the mycel home root directory
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

// runRoot handles the default mycel command (no subcommand).
// Interactive terminal → make sure the daemon runs and open the
// dashboard (works from any directory; `mycel up` bootstraps ~/.mycel).
// Non-interactive mode → show help.
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

	// The daemon is CWD-free: boot it (if needed) and open the web UI.
	return runHome(cmd, args)
}
