package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestNewManager(t *testing.T) {
	t.Setenv("MYCEL_HOST_WORKSPACE", "")

	dir := t.TempDir()
	m := NewManager(dir)

	if m.repoRoot != dir {
		t.Errorf("repoRoot = %q, want %q", m.repoRoot, dir)
	}

	if m.hostBaseName != filepath.Base(dir) {
		t.Errorf("hostBaseName = %q, want %q", m.hostBaseName, filepath.Base(dir))
	}

	expected := filepath.Join(dir, ".bc", "agents")
	if m.agentsDir != expected {
		t.Errorf("agentsDir = %q, want %q", m.agentsDir, expected)
	}
}

func TestNewManagerWithEnv(t *testing.T) {
	t.Setenv("MYCEL_HOST_WORKSPACE", "my-project")

	m := NewManager(t.TempDir())

	if m.hostBaseName != "my-project" {
		t.Errorf("hostBaseName = %q, want %q", m.hostBaseName, "my-project")
	}
}

func TestName(t *testing.T) {
	t.Setenv("MYCEL_HOST_WORKSPACE", "myrepo")

	m := NewManager("/tmp/myrepo")

	got := m.Name("eng-01")
	want := "bc-myrepo-eng-01"

	if got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestPath(t *testing.T) {
	dir := "/tmp/myrepo"
	m := NewManager(dir)

	got := m.Path("eng-01")
	want := filepath.Join(dir, ".bc", "agents", "eng-01", "bc-"+filepath.Base(dir)+"-eng-01")

	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestCreateAndRemove(t *testing.T) {
	repo := setupTestRepo(t)
	m := NewManager(repo)
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
	//nolint:gosec // test with trusted paths
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("not a git worktree: %v: %s", err, out)
	}

	if strings.TrimSpace(string(out)) != "true" {
		t.Errorf("rev-parse = %q, want %q", strings.TrimSpace(string(out)), "true")
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

func TestCreateIdempotent(t *testing.T) {
	repo := setupTestRepo(t)
	m := NewManager(repo)
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
	m := NewManager(repo)
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
	m := NewManager(repo)
	ctx := context.Background()

	// Prune on a clean repo should not error
	if err := m.Prune(ctx); err != nil {
		t.Errorf("Prune() error: %v", err)
	}
}

func TestClaudeDir(t *testing.T) {
	dir := "/tmp/myrepo"
	m := NewManager(dir)

	got := m.ClaudeDir("eng-01")
	want := filepath.Join(dir, ".bc", "agents", "eng-01", "claude")

	if got != want {
		t.Errorf("ClaudeDir() = %q, want %q", got, want)
	}
}

func TestEnsureClaudeDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	if err := m.EnsureClaudeDir("eng-01"); err != nil {
		t.Fatalf("EnsureClaudeDir() error: %v", err)
	}

	claudeDir := m.ClaudeDir("eng-01")
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

// --- flat layout ---

func TestFlatManagerPaths(t *testing.T) {
	wt := "/data/worktrees"
	st := "/data/agents"
	m := NewFlatManager("/repo", wt, st)

	if got, want := m.Name("eng-01"), "eng-01"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if got, want := m.Path("eng-01"), filepath.Join(wt, "eng-01"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
	if got, want := m.ClaudeDir("eng-01"), filepath.Join(st, "eng-01", "claude"); got != want {
		t.Errorf("ClaudeDir() = %q, want %q", got, want)
	}
}

func TestFlatManagerCreateRemove(t *testing.T) {
	repo := setupTestRepo(t)
	base := t.TempDir()
	m := NewFlatManager(repo, filepath.Join(base, "worktrees"), filepath.Join(base, "agents"))

	ctx := context.Background()
	path, err := m.Create(ctx, "eng-01")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if want := filepath.Join(base, "worktrees", "eng-01"); path != want {
		t.Errorf("Create path = %q, want %q", path, want)
	}
	if !m.Exists("eng-01") {
		t.Error("Exists() = false after Create")
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		t.Errorf(".git missing in flat worktree: %v", err)
	}

	if err := m.Remove(ctx, "eng-01"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if m.Exists("eng-01") {
		t.Error("Exists() = true after Remove")
	}
}

func TestFlatManagerEnsureClaudeDir(t *testing.T) {
	base := t.TempDir()
	m := NewFlatManager("/repo", filepath.Join(base, "worktrees"), filepath.Join(base, "agents"))

	if err := m.EnsureClaudeDir("eng-01"); err != nil {
		t.Fatalf("EnsureClaudeDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "agents", "eng-01", "claude"))
	if err != nil || !info.IsDir() {
		t.Fatalf("claude dir not created: %v", err)
	}
}

func TestManagerRemoveAt(t *testing.T) {
	repo := setupTestRepo(t)
	base := t.TempDir()
	// Create with a nested manager, remove via a flat one using the
	// stored path — mirrors deleting a pre-migration agent.
	nested := NewManagerWithDataDir(repo, base)
	ctx := context.Background()
	oldPath, err := nested.Create(ctx, "eng-01")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	flat := NewFlatManager(repo, filepath.Join(base, "worktrees"), filepath.Join(base, "agents"))
	if err := flat.RemoveAt(ctx, "eng-01", oldPath); err != nil {
		t.Fatalf("RemoveAt: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old worktree still present at %q", oldPath)
	}
}

func TestManagerRemoveAtRejectsUnsafePaths(t *testing.T) {
	m := NewFlatManager("/repo", "/data/worktrees", "/data/agents")
	if err := m.RemoveAt(context.Background(), "eng-01", "relative/path"); err == nil {
		t.Error("RemoveAt accepted a relative path")
	}
	if err := m.RemoveAt(context.Background(), "eng-01", "/data/../etc"); err == nil {
		t.Error("RemoveAt accepted a traversal path")
	}
}
