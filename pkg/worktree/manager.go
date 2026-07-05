// Package worktree manages git worktree lifecycle for agent isolation.
//
// Two layouts are supported:
//
//   - Flat (current): worktrees at <worktreesDir>/<name>/ and Claude state
//     at <stateDir>/<name>/claude/. Agent names are globally unique (DB
//     primary key), so flat name-keyed directories are safe. The daemon
//     uses ~/.mycel/worktrees/ and ~/.mycel/agents/ for these.
//   - Nested (legacy): worktrees at <dataDir>/agents/<name>/bc-<ws>-<name>/
//     with Claude state alongside at <dataDir>/agents/<name>/claude/.
//     Kept so existing agents and tests keep working from their old paths.
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

// Manager handles git worktree lifecycle for agent isolation.
type Manager struct {
	repoRoot     string
	agentsDir    string // nested layout: worktrees under <agentsDir>/<name>/
	worktreesDir string // flat layout: worktrees at <worktreesDir>/<name>/
	stateDir     string // flat layout: Claude state at <stateDir>/<name>/claude/
	hostBaseName string
	mu           sync.Mutex
	flat         bool
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
// Reads MYCEL_HOST_WORKSPACE to determine the host base name for worktree
// naming so containers mounting the host repo get the expected label.
func NewManagerWithDataDir(repoRoot, dataDir string) *Manager {
	hostBase := filepath.Base(repoRoot)
	if hp := os.Getenv("MYCEL_HOST_WORKSPACE"); hp != "" {
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

// NewFlatManager creates a worktree manager using the flat layout:
// worktrees at <worktreesDir>/<agent>/ and Claude state at
// <stateDir>/<agent>/claude/. Agent names are globally unique, so the
// directories are keyed by bare agent name. This is the layout the
// daemon uses: worktreesDir = ~/.mycel/worktrees, stateDir =
// ~/.mycel/agents.
func NewFlatManager(repoRoot, worktreesDir, stateDir string) *Manager {
	return &Manager{
		repoRoot:     repoRoot,
		worktreesDir: worktreesDir,
		stateDir:     stateDir,
		flat:         true,
	}
}

// Name returns the worktree name for an agent. Flat layout uses the bare
// agent name; the nested legacy layout uses bc-<hostBaseName>-<agentName>.
func (m *Manager) Name(agentName string) string {
	if m.flat {
		return agentName
	}
	return fmt.Sprintf("bc-%s-%s", m.hostBaseName, agentName)
}

// Path returns the filesystem path for an agent's worktree:
// <worktreesDir>/<agent> in the flat layout, or the nested
// <agentsDir>/<agent>/bc-<ws>-<agent> in the legacy layout.
func (m *Manager) Path(agentName string) string {
	if m.flat {
		return filepath.Join(m.worktreesDir, agentName)
	}
	return filepath.Join(m.agentsDir, agentName, m.Name(agentName))
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

// ClaudeDir returns the path to the Claude home directory for the given
// agent: <stateDir>/<agent>/claude in the flat layout, or the nested
// <agentsDir>/<agent>/claude in the legacy layout.
func (m *Manager) ClaudeDir(agentName string) string {
	if m.flat {
		return filepath.Join(m.stateDir, agentName, "claude")
	}
	return filepath.Join(m.agentsDir, agentName, "claude")
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
