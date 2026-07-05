package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/rpuneet/mycel/pkg/client"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// getRepo resolves the enclosing mycel-adopted git repo. Checks
// MYCEL_WORKSPACE env var first (for agents in worktrees), then walks up
// the directory tree probing each candidate's global state dir.
func getRepo() (*workspace.Workspace, error) {
	// Check MYCEL_WORKSPACE first (agents set this to point to the main repo)
	if wsPath := os.Getenv("MYCEL_WORKSPACE"); wsPath != "" {
		return workspace.Load(wsPath)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return workspace.Find(cwd)
}

// errorAgentNotRunning returns an error message for commands that require MYCEL_AGENT_ID.
func errorAgentNotRunning(commandUsage string) error {
	return fmt.Errorf("this command can only be run by agents in the mycel system (use: mycel agent send <agent-name> %q)", commandUsage)
}

// newDaemonClient creates a client connected to the bcd daemon.
// Returns an error if the daemon is not running.
// Checks for a mycel-adopted repo first to provide clear error messages.
func newDaemonClient(ctx context.Context) (*client.Client, error) {
	// Verify we're in an adopted repo before trying to connect to daemon
	if _, err := getRepo(); err != nil {
		return nil, errNoRepo(err)
	}
	c := client.New("")
	if err := c.Ping(ctx); err != nil {
		return nil, fmt.Errorf("bcd is not running — start it with 'bcd' or 'mycel up' first\n(%w)", err)
	}
	return c, nil
}

// errNoRepo returns an actionable error for commands that require a
// mycel-adopted repo.
func errNoRepo(err error) error {
	if err != nil {
		return fmt.Errorf("not in a mycel-adopted repo — run 'mycel up' from your repo (or add one in the web UI): %w", err)
	}
	return fmt.Errorf("not in a mycel-adopted repo. Run 'mycel up' from your repo (or add one in the web UI)")
}

// requireRepo returns the current repo's workspace state or an
// actionable error. Convenience wrapper around getRepo() with standard
// error handling.
func requireRepo() (*workspace.Workspace, error) {
	ws, err := getRepo()
	if err != nil {
		return nil, errNoRepo(err)
	}
	return ws, nil
}
