// Package worktree manages git worktree lifecycle for agent isolation.
//
// Layout is entity-scoped: every agent owns one directory at
// <agentsRoot>/<name>/ (normally ~/.mycel/agents/<name>/) containing
//
//	worktree/  — the agent's git worktree
//	session/   — provider state (e.g. the Claude home dir)
//
// Agent names are globally unique (DB primary key), so name-keyed
// directories are safe. Deleting <agentsRoot>/<name>/ removes all of
// the agent's filesystem state.
package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rpuneet/mycel/pkg/log"
)

// Subdirectories of an agent's entity directory.
const (
	worktreeDirName = "worktree"
	sessionDirName  = "session"
)

// Manager handles git worktree lifecycle for agent isolation.
type Manager struct {
	repoRoot   string // git repo worktrees are created from
	agentsRoot string // entity root: <agentsRoot>/<name>/{worktree,session}
	mu         sync.Mutex
}

// NewManager creates a worktree manager that creates worktrees from
// repoRoot under <agentsRoot>/<name>/worktree.
func NewManager(repoRoot, agentsRoot string) *Manager {
	return &Manager{repoRoot: repoRoot, agentsRoot: agentsRoot}
}

// AgentDir returns the agent's entity directory: <agentsRoot>/<name>.
func (m *Manager) AgentDir(agentName string) string {
	return filepath.Join(m.agentsRoot, agentName)
}

// Path returns the filesystem path for an agent's worktree:
// <agentsRoot>/<name>/worktree.
func (m *Manager) Path(agentName string) string {
	return filepath.Join(m.agentsRoot, agentName, worktreeDirName)
}

// Create creates a git worktree for the given agent.
// It prunes stale worktrees, removes any existing worktree at the path, and
// creates a new detached worktree to avoid branch conflicts.
func (m *Manager) Create(ctx context.Context, agentName string) (string, error) {
	if !filepath.IsLocal(agentName) {
		return "", fmt.Errorf("invalid agent name %q", agentName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.Path(agentName)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", fmt.Errorf("create agent dir: %w", err)
	}

	// Prune stale worktree refs
	//nolint:gosec // trusted paths
	prune := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "prune")
	if out, err := prune.CombinedOutput(); err != nil {
		log.Warn("worktree prune failed", "error", err, "output", string(out))
	}

	// Remove existing worktree if present
	if _, err := os.Stat(path); err == nil {
		log.Debug("removing existing worktree", "agent", agentName, "path", path)
		//nolint:gosec // trusted paths
		rm := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "remove", "--force", path)
		if out, err := rm.CombinedOutput(); err != nil {
			log.Warn("git worktree remove failed, falling back to os.RemoveAll",
				"error", err, "output", string(out))
			if rmErr := os.RemoveAll(path); rmErr != nil {
				return "", fmt.Errorf("remove stale worktree: %w", rmErr)
			}
			// Re-prune after manual removal
			//nolint:gosec // trusted paths
			reprune := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "prune")
			if out, err := reprune.CombinedOutput(); err != nil {
				log.Warn("worktree re-prune failed", "error", err, "output", string(out))
			}
		}
	}

	// Create detached worktree
	//nolint:gosec // trusted paths
	add := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "add", "--detach", path)
	if out, err := add.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s: %w", string(out), err)
	}

	log.Debug("created worktree", "agent", agentName, "path", path)

	return path, nil
}

// Remove removes the git worktree for the given agent.
func (m *Manager) Remove(ctx context.Context, agentName string) error {
	if !filepath.IsLocal(agentName) {
		return fmt.Errorf("invalid agent name %q", agentName)
	}
	return m.removeAt(ctx, agentName, m.Path(agentName))
}

// RemoveAt removes the git worktree at an explicit path — used for agents
// whose stored WorktreeDir predates the current layout, so deletion targets
// the directory the agent actually used instead of a recomputed path.
func (m *Manager) RemoveAt(ctx context.Context, agentName, path string) error {
	// ".." is checked on the raw value so traversal cannot be smuggled
	// past filepath.Clean.
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid worktree path %q", path)
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("invalid worktree path %q", path)
	}
	return m.removeAt(ctx, agentName, path)
}

func (m *Manager) removeAt(ctx context.Context, agentName, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	//nolint:gosec // trusted paths
	rm := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "remove", "--force", path)
	if out, err := rm.CombinedOutput(); err != nil {
		log.Warn("git worktree remove failed, falling back to os.RemoveAll",
			"error", err, "output", string(out))
		if rmErr := os.RemoveAll(path); rmErr != nil {
			return fmt.Errorf("remove worktree: %w", rmErr)
		}
	}

	// Prune stale refs
	//nolint:gosec // trusted paths
	prune := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "prune")
	if out, err := prune.CombinedOutput(); err != nil {
		log.Warn("worktree prune failed", "error", err, "output", string(out))
	}

	log.Debug("removed worktree", "agent", agentName, "path", path)

	return nil
}

// Exists checks whether the worktree directory exists for the given agent.
func (m *Manager) Exists(agentName string) bool {
	if !filepath.IsLocal(agentName) {
		return false
	}
	_, err := os.Stat(m.Path(agentName))
	return err == nil
}

// Prune runs git worktree prune to clean stale worktree refs.
func (m *Manager) Prune(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	//nolint:gosec // trusted paths
	cmd := exec.CommandContext(ctx, "git", "-C", m.repoRoot, "worktree", "prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune: %s: %w", string(out), err)
	}

	return nil
}

// SessionDir returns the agent's provider-state directory:
// <agentsRoot>/<name>/session. Provider config and transcripts (e.g.
// the Claude home for docker agents) persist here on the host.
func (m *Manager) SessionDir(agentName string) string {
	return filepath.Join(m.agentsRoot, agentName, sessionDirName)
}

// ClaudeDir returns the Claude home directory inside the agent's
// session dir: <agentsRoot>/<name>/session/claude.
func (m *Manager) ClaudeDir(agentName string) string {
	return filepath.Join(m.SessionDir(agentName), "claude")
}

// EnsureClaudeDir creates the Claude home directory for the given agent if it
// does not already exist.
func (m *Manager) EnsureClaudeDir(agentName string) error {
	if !filepath.IsLocal(agentName) {
		return fmt.Errorf("invalid agent name %q", agentName)
	}
	dir := m.ClaudeDir(agentName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create claude dir: %w", err)
	}

	return nil
}
