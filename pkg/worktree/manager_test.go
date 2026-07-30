package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestRepo creates a real git repo with one commit for worktree tests.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	dir := t.TempDir()

	//nolint:gosec // test helper with trusted paths
	if err := exec.CommandContext(ctx, "git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Configure git user for commits
	//nolint:gosec // test helper with trusted paths
	if err := exec.CommandContext(ctx, "git", "-C", dir, "config", "user.email", "test@test.com").Run(); err != nil {
		t.Fatalf("git config email: %v", err)
	}

	//nolint:gosec // test helper with trusted paths
	if err := exec.CommandContext(ctx, "git", "-C", dir, "config", "user.name", "Test").Run(); err != nil {
		t.Fatalf("git config name: %v", err)
	}

	//nolint:gosec // test helper with trusted paths
	if err := exec.CommandContext(ctx, "git", "-C", dir, "commit", "--allow-empty", "-m", "init").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	return dir
}

// newTestManager returns a manager rooted at a temp agents root, mirroring
// ~/.mycel/agents without ever touching the real home.
func newTestManager(t *testing.T, repoRoot string) (*Manager, string) {
	t.Helper()
	t.Setenv("MYCEL_HOME", t.TempDir())
	agentsRoot := filepath.Join(os.Getenv("MYCEL_HOME"), "agents")
	return NewManager(repoRoot, agentsRoot), agentsRoot
}

// TestManagerLayout locks the entity-scoped layout: every agent owns one
// directory <agentsRoot>/<name>/ containing worktree/ and session/.
func TestManagerLayout(t *testing.T) {
	m, agentsRoot := newTestManager(t, "/repo")

	if got, want := m.AgentDir("eng-01"), filepath.Join(agentsRoot, "eng-01"); got != want {
		t.Errorf("AgentDir() = %q, want %q", got, want)
	}
	if got, want := m.Path("eng-01"), filepath.Join(agentsRoot, "eng-01", "worktree"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
	if got, want := m.SessionDir("eng-01"), filepath.Join(agentsRoot, "eng-01", "session"); got != want {
		t.Errorf("SessionDir() = %q, want %q", got, want)
	}
	if got, want := m.ClaudeDir("eng-01"), filepath.Join(agentsRoot, "eng-01", "session", "claude"); got != want {
		t.Errorf("ClaudeDir() = %q, want %q", got, want)
	}
}

// TestManagerLayoutIsSiblingSafe: two agents' directories never overlap —
// deleting one agent's dir cannot touch another's state.
func TestManagerLayoutIsSiblingSafe(t *testing.T) {
	m, agentsRoot := newTestManager(t, "/repo")

	a := m.AgentDir("eng-01")
	b := m.AgentDir("eng-02")
	if a == b {
		t.Fatalf("distinct agents share a directory: %q", a)
	}
	if filepath.Dir(a) != agentsRoot || filepath.Dir(b) != agentsRoot {
		t.Errorf("agent dirs %q, %q are not direct children of %q", a, b, agentsRoot)
	}
}

func TestCreateAndRemove(t *testing.T) {
	repo := setupTestRepo(t)
	m, _ := newTestManager(t, repo)
	ctx := context.Background()

	// Create worktree
	path, err := m.Create(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	expectedPath := m.Path("test-agent")
	if path != expectedPath {
		t.Errorf("Create() path = %q, want %q", path, expectedPath)
	}

	// Verify directory exists
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		t.Error("worktree directory does not exist after Create()")
	}

	// Verify it's a valid git worktree
	if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr != nil {
		t.Errorf(".git missing in worktree: %v", statErr)
	}

	// Remove worktree
	if err := m.Remove(ctx, "test-agent"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("worktree directory still exists after Remove()")
	}
}

func TestCreateRejectsInvalidAgentName(t *testing.T) {
	repo := setupTestRepo(t)
	m, _ := newTestManager(t, repo)
	ctx := context.Background()

	for _, name := range []string{"../escape", "/abs", ""} {
		if _, err := m.Create(ctx, name); err == nil {
			t.Errorf("Create(%q) accepted an invalid agent name", name)
		}
	}
}

func TestCreateIdempotent(t *testing.T) {
	repo := setupTestRepo(t)
	m, _ := newTestManager(t, repo)
	ctx := context.Background()

	// Create twice — second call should not error
	path1, err := m.Create(ctx, "idem-agent")
	if err != nil {
		t.Fatalf("first Create() error: %v", err)
	}

	path2, err := m.Create(ctx, "idem-agent")
	if err != nil {
		t.Fatalf("second Create() error: %v", err)
	}

	if path1 != path2 {
		t.Errorf("paths differ: %q vs %q", path1, path2)
	}

	// Verify directory exists
	if _, err := os.Stat(path2); os.IsNotExist(err) {
		t.Error("worktree directory does not exist after second Create()")
	}

	// Cleanup
	if err := m.Remove(ctx, "idem-agent"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
}

func TestExists(t *testing.T) {
	repo := setupTestRepo(t)
	m, _ := newTestManager(t, repo)
	ctx := context.Background()

	// Should not exist before creation
	if m.Exists("exist-agent") {
		t.Error("Exists() = true before Create()")
	}

	// Create
	if _, err := m.Create(ctx, "exist-agent"); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Should exist after creation
	if !m.Exists("exist-agent") {
		t.Error("Exists() = false after Create()")
	}

	// Remove
	if err := m.Remove(ctx, "exist-agent"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	// Should not exist after removal
	if m.Exists("exist-agent") {
		t.Error("Exists() = true after Remove()")
	}
}

func TestPrune(t *testing.T) {
	repo := setupTestRepo(t)
	m, _ := newTestManager(t, repo)
	ctx := context.Background()

	// Prune on a clean repo should not error
	if err := m.Prune(ctx); err != nil {
		t.Errorf("Prune() error: %v", err)
	}
}

func TestEnsureClaudeDir(t *testing.T) {
	m, agentsRoot := newTestManager(t, t.TempDir())

	if err := m.EnsureClaudeDir("eng-01"); err != nil {
		t.Fatalf("EnsureClaudeDir() error: %v", err)
	}

	claudeDir := filepath.Join(agentsRoot, "eng-01", "session", "claude")
	info, err := os.Stat(claudeDir)
	if os.IsNotExist(err) {
		t.Fatal("claude dir does not exist after EnsureClaudeDir()")
	}

	if !info.IsDir() {
		t.Error("claude dir is not a directory")
	}

	// Calling again should not error (idempotent)
	if err := m.EnsureClaudeDir("eng-01"); err != nil {
		t.Fatalf("second EnsureClaudeDir() error: %v", err)
	}
}

func TestEnsureClaudeDirRejectsInvalidName(t *testing.T) {
	m, _ := newTestManager(t, "/repo")
	if err := m.EnsureClaudeDir("../escape"); err == nil {
		t.Error("EnsureClaudeDir accepted an invalid agent name")
	}
}

// TestManagerRemoveAt: RemoveAt targets an explicit stored path instead of
// the recomputed layout path — used for agents whose WorktreeDir predates
// the current layout.
func TestManagerRemoveAt(t *testing.T) {
	repo := setupTestRepo(t)
	m, _ := newTestManager(t, repo)
	ctx := context.Background()

	path, err := m.Create(ctx, "eng-01")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := m.RemoveAt(ctx, "eng-01", path); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree still present at %q", path)
	}
}

func TestManagerRemoveAtRejectsUnsafePaths(t *testing.T) {
	m, _ := newTestManager(t, "/repo")
	if err := m.RemoveAt(context.Background(), "eng-01", "relative/path"); err == nil {
		t.Error("RemoveAt accepted a relative path")
	}
	if err := m.RemoveAt(context.Background(), "eng-01", "/data/../etc"); err == nil {
		t.Error("RemoveAt accepted a traversal path")
	}
}
