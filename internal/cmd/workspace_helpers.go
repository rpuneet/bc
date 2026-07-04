package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// getWorkspace finds the current workspace.
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
		return nil, fmt.Errorf("bcd is not running — start it with 'bcd' or 'mycel up' first\n(%w)", err)
	}
	return c, nil
}

// errNotInWorkspace returns an actionable error for commands that require a mycel workspace.
func errNotInWorkspace(err error) error {
	if err != nil {
		return fmt.Errorf("not in a mycel workspace — run 'mycel up' from your repo (or add one in the web UI): %w", err)
	}
	return fmt.Errorf("not in a mycel workspace. Run 'mycel up' from your repo (or add one in the web UI)")
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
