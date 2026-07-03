// Package worktree manages git worktree lifecycle for agent isolation.
// Each agent gets its own worktree at <DataDir>/agents/<name>/bc-<ws>-<name>/.
// Before M11 the worktrees lived under <repoRoot>/.bc/agents/...; after
// M11 they move to ~/.mycel/workspaces/<id>/agents/... so the project
// directory stays a pristine git repo.
package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/rpuneet/mycel/pkg/log"
)

// Manager handles git worktree lifecycle for agent isolation.
type Manager struct {
	repoRoot     string
	agentsDir    string
	hostBaseName string
	mu           sync.Mutex
}

// NewManager creates a worktree manager whose worktrees live under the
// legacy <repoRoot>/.bc/agents/ directory. Prefer NewManagerWithDataDir
// for M11+ layouts where runtime state lives at ~/.mycel/workspaces/<id>/.
//
// This constructor is kept for older call sites and tests that still
// operate on the legacy layout.
func NewManager(repoRoot string) *Manager {
	return NewManagerWithDataDir(repoRoot, filepath.Join(repoRoot, ".bc"))
}

// NewManagerWithDataDir creates a worktree manager rooted at repoRoot
// (the project git repo) whose worktrees live under <dataDir>/agents/.
// This is the M11 constructor: dataDir is the per-workspace runtime
// directory (~/.mycel/workspaces/<id>/) and repoRoot stays untouched.
//
// Reads BC_HOST_WORKSPACE to determine the host base name for worktree
// naming so containers mounting the host repo get the expected label.
func NewManagerWithDataDir(repoRoot, dataDir string) *Manager {
	hostBase := filepath.Base(repoRoot)
	if hp := os.Getenv("BC_HOST_WORKSPACE"); hp != "" {
		hostBase = filepath.Base(hp)
	}
	if dataDir == "" {
		dataDir = filepath.Join(repoRoot, ".bc")
	}
	return &Manager{
		repoRoot:     repoRoot,
		agentsDir:    filepath.Join(dataDir, "agents"),
		hostBaseName: hostBase,
	}
}

// Name returns the worktree name for an agent: bc-<hostBaseName>-<agentName>.
func (m *Manager) Name(agentName string) string {
	return fmt.Sprintf("bc-%s-%s", m.hostBaseName, agentName)
}

// Path returns the filesystem path for an agent's worktree.
// The directory is named bc-<workspace>-<agent> so git's internal
// worktree name matches the naming convention.
func (m *Manager) Path(agentName string) string {
	return filepath.Join(m.agentsDir, agentName, m.Name(agentName))
}

// Create creates a git worktree for the given agent.
// It prunes stale worktrees, removes any existing worktree at the path, and
// creates a new detached worktree to avoid branch conflicts.
func (m *Manager) Create(ctx context.Context, agentName string) (string, error) {
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
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.Path(agentName)

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

// ClaudeDir returns the path to the Claude home directory for the given agent.
func (m *Manager) ClaudeDir(agentName string) string {
	return filepath.Join(m.agentsDir, agentName, "claude")
}

// EnsureClaudeDir creates the Claude home directory for the given agent if it
// does not already exist.
func (m *Manager) EnsureClaudeDir(agentName string) error {
	dir := m.ClaudeDir(agentName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create claude dir: %w", err)
	}

	return nil
}
