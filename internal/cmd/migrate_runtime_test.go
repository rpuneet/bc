package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/bc/pkg/workspace"
)

// Seed helper: create a fake legacy <project>/.bc/ sidecar with a
// settings.json, a state.db, and an agents/ subdir.
func seedLegacyWorkspace(t *testing.T, projectDir string) {
	t.Helper()
	bcDir := filepath.Join(projectDir, ".bc")
	if err := os.MkdirAll(filepath.Join(bcDir, "agents", "alice", "claude"), 0o750); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(bcDir, "settings.json"),
		[]byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`),
		0o600,
	); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bcDir, "state.db"), []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatalf("write state.db: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(bcDir, "agents", "alice", "loop.json"),
		[]byte(`{"iteration":42}`),
		0o600,
	); err != nil {
		t.Fatalf("write loop.json: %v", err)
	}
}

// TestMigrateOneWorkspaceRuntime_Basic verifies the happy path: a legacy
// .bc/ sidecar migrates to ~/.bc/workspaces/<id>/, settings.json becomes
// preferences.json, and the legacy dir is renamed to .bc.migrated/.
func TestMigrateOneWorkspaceRuntime_Basic(t *testing.T) {
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seedLegacyWorkspace(t, projectDir)

	id := workspace.ComputeWorkspaceID(projectDir)
	dataDir, err := workspace.DataDir(id)
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}

	entry := &workspace.RegistryEntry{
		ID:      id,
		Path:    projectDir,
		DataDir: dataDir,
		Name:    "project",
	}

	result := migrateOneWorkspaceRuntime(entry)
	if result.Skipped {
		t.Fatalf("expected migration to run, got skipped: %s", result.SkipReason)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if result.MovedFiles == 0 {
		t.Error("expected at least one file moved")
	}

	// preferences.json lives at the new location with promoted content.
	prefs := filepath.Join(dataDir, workspace.PreferencesFileName)
	data, readErr := os.ReadFile(prefs)
	if readErr != nil {
		t.Fatalf("preferences.json missing: %v", readErr)
	}
	if !strings.Contains(string(data), `"version":2`) {
		t.Errorf("preferences.json content wrong: %q", string(data))
	}

	// state.db moved.
	if _, err := os.Stat(filepath.Join(dataDir, "state.db")); err != nil {
		t.Errorf("state.db not moved: %v", err)
	}

	// Legacy dir renamed.
	if _, err := os.Stat(filepath.Join(projectDir, ".bc")); err == nil {
		t.Error("legacy .bc/ should have been renamed")
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".bc.migrated")); err != nil {
		t.Errorf(".bc.migrated breadcrumb dir missing: %v", err)
	}
	crumb := filepath.Join(projectDir, ".bc.migrated.txt")
	crumbData, err := os.ReadFile(crumb)
	if err != nil {
		t.Errorf(".bc.migrated.txt missing: %v", err)
	} else if !strings.Contains(string(crumbData), dataDir) {
		t.Errorf("breadcrumb doesn't reference DataDir: %q", crumbData)
	}

	// Agent state dir moved.
	if _, err := os.Stat(filepath.Join(dataDir, "agents", "alice", "loop.json")); err != nil {
		t.Errorf("agent state not moved: %v", err)
	}
}

// TestMigrateOneWorkspaceRuntime_SkipsIfAlreadyMigrated ensures a second
// pass over a populated DataDir is a no-op.
func TestMigrateOneWorkspaceRuntime_SkipsIfAlreadyMigrated(t *testing.T) {
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seedLegacyWorkspace(t, projectDir)

	id := workspace.ComputeWorkspaceID(projectDir)
	dataDir, _ := workspace.DataDir(id)
	// Pre-populate DataDir so the migration should skip.
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, workspace.PreferencesFileName), []byte(`{"version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}

	entry := &workspace.RegistryEntry{
		ID:      id,
		Path:    projectDir,
		DataDir: dataDir,
		Name:    "project",
	}
	result := migrateOneWorkspaceRuntime(entry)
	if !result.Skipped {
		t.Errorf("expected Skipped=true, got %+v", result)
	}
	if !strings.Contains(result.SkipReason, "populated") {
		t.Errorf("skip reason should mention 'populated', got %q", result.SkipReason)
	}

	// Legacy .bc/ must still exist — we did NOT touch it.
	if _, err := os.Stat(filepath.Join(projectDir, ".bc")); err != nil {
		t.Errorf("legacy .bc/ unexpectedly removed: %v", err)
	}
}

// TestMigrateOneWorkspaceRuntime_NoLegacy skips when <project>/.bc/ is
// absent (fresh M11 workspace).
func TestMigrateOneWorkspaceRuntime_NoLegacy(t *testing.T) {
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)

	projectDir := filepath.Join(t.TempDir(), "pristine-project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	id := workspace.ComputeWorkspaceID(projectDir)
	dataDir, _ := workspace.DataDir(id)
	entry := &workspace.RegistryEntry{
		ID:      id,
		Path:    projectDir,
		DataDir: dataDir,
		Name:    "pristine",
	}
	result := migrateOneWorkspaceRuntime(entry)
	if !result.Skipped {
		t.Errorf("expected Skipped for pristine project, got %+v", result)
	}
	if !strings.Contains(result.SkipReason, "no legacy") {
		t.Errorf("skip reason should mention 'no legacy', got %q", result.SkipReason)
	}
}

// TestMigrateOneWorkspaceRuntime_GitWorktreeMove end-to-ends a real git
// worktree through the migration. Skipped when git is not on PATH.
func TestMigrateOneWorkspaceRuntime_GitWorktreeMove(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Initialize a real git repo with one commit so 'git worktree add'
	// has something to check out.
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", projectDir}, args...)...) //nolint:gosec // test-controlled args
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=bc", "GIT_AUTHOR_EMAIL=bc@test", "GIT_COMMITTER_NAME=bc", "GIT_COMMITTER_EMAIL=bc@test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	// Legacy agent worktree under .bc/agents/alice/bc-project-alice/
	id := workspace.ComputeWorkspaceID(projectDir)
	dataDir, _ := workspace.DataDir(id)
	legacyAgentDir := filepath.Join(projectDir, ".bc", "agents", "alice")
	if err := os.MkdirAll(legacyAgentDir, 0o750); err != nil {
		t.Fatal(err)
	}
	worktreeName := "bc-project-alice"
	legacyWT := filepath.Join(legacyAgentDir, worktreeName)
	run("worktree", "add", "--detach", legacyWT)

	// Seed minimal legacy files.
	if err := os.WriteFile(
		filepath.Join(projectDir, ".bc", workspace.LegacySettingsFileName),
		[]byte(`{"version":2}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	entry := &workspace.RegistryEntry{
		ID:      id,
		Path:    projectDir,
		DataDir: dataDir,
		Name:    "project",
	}
	result := migrateOneWorkspaceRuntime(entry)
	if result.Skipped {
		t.Fatalf("expected migration, got Skipped: %s", result.SkipReason)
	}
	if result.WorktreesMoved != 1 {
		t.Errorf("expected 1 worktree moved, got %d (skipped: %v, errs: %v)",
			result.WorktreesMoved, result.WorktreesSkipped, result.Errors)
	}

	// New worktree exists in the DataDir.
	newWT := filepath.Join(dataDir, "agents", "alice", worktreeName)
	if _, err := os.Stat(filepath.Join(newWT, ".git")); err != nil {
		t.Errorf("new worktree missing .git: %v", err)
	}
	// git knows about the new path.
	cmd := exec.Command("git", "-C", projectDir, "worktree", "list", "--porcelain") //nolint:gosec // trusted paths
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), newWT) {
		t.Errorf("git doesn't know about new worktree path %q; got:\n%s", newWT, string(out))
	}
}

// TestMaybeRunRuntimeMigration_MarkerSkipsSecondRun verifies the boot-
// time auto-run is a no-op once the marker is in place.
func TestMaybeRunRuntimeMigration_MarkerSkipsSecondRun(t *testing.T) {
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)

	// Write the marker directly.
	if err := os.MkdirAll(bcHome, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bcHome, migrateRuntimeMarker), []byte("ran"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !RuntimeMigrationAlreadyRan() {
		t.Fatal("marker should be detected")
	}

	// Seed a legacy workspace; after MaybeRun, it should NOT have been
	// touched because the marker suppresses the run.
	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seedLegacyWorkspace(t, projectDir)

	MaybeRunRuntimeMigration(context.Background())

	if _, err := os.Stat(filepath.Join(projectDir, ".bc")); err != nil {
		t.Errorf("legacy .bc/ was touched despite marker: %v", err)
	}
}

// TestMaybeRunRuntimeMigration_SkipsWhenBCHomeIsSandboxed verifies that
// MaybeRunRuntimeMigration refuses to run when BC_HOME resolves to
// anything other than the user's default $HOME/.bc. Without this guard,
// any test that boots bcd would walk the host's real registry and
// corrupt production state (the M11e incident).
func TestMaybeRunRuntimeMigration_SkipsWhenBCHomeIsSandboxed(t *testing.T) {
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seedLegacyWorkspace(t, projectDir)

	// No marker present, no BC_SKIP_MIGRATION set — only the BC_HOME
	// guard should suppress the run.
	MaybeRunRuntimeMigration(context.Background())

	if _, err := os.Stat(filepath.Join(projectDir, ".bc")); err != nil {
		t.Errorf("legacy .bc/ was touched under sandboxed BC_HOME: %v", err)
	}
	if RuntimeMigrationAlreadyRan() {
		t.Error("marker was written despite guard — migration ran when it shouldn't")
	}
}

// TestMaybeRunRuntimeMigration_RespectsSkipEnv verifies BC_SKIP_MIGRATION
// is honored even when BC_HOME is the default. Point HOME at a tempdir
// and BC_HOME at $HOME/.bc so the default-home guard would PASS —
// BC_SKIP_MIGRATION is the only thing that should suppress the run.
func TestMaybeRunRuntimeMigration_RespectsSkipEnv(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("BC_HOME", filepath.Join(tmpHome, ".bc"))
	t.Setenv("BC_SKIP_MIGRATION", "1")

	projectDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seedLegacyWorkspace(t, projectDir)

	MaybeRunRuntimeMigration(context.Background())

	if _, err := os.Stat(filepath.Join(projectDir, ".bc")); err != nil {
		t.Errorf("legacy .bc/ touched despite BC_SKIP_MIGRATION=1: %v", err)
	}
}

// TestMigrateAllWorkspaceRuntimes_PrunesStalePathsFirst ensures that
// registry entries whose project directory has vanished are removed
// before iteration. Without this, phantom entries accumulated from
// prior test runs caused the M11e incident (11,698 junk dirs created
// under ~/.bc/workspaces/ from stale tmpdir paths).
func TestMigrateAllWorkspaceRuntimes_PrunesStalePathsFirst(t *testing.T) {
	bcHome := filepath.Join(t.TempDir(), "bc-home")
	t.Setenv("BC_HOME", bcHome)
	t.Setenv("HOME", t.TempDir()) // isolate registry path too

	// Seed a registry with one live path and two phantom ones.
	live := filepath.Join(t.TempDir(), "live-project")
	if err := os.MkdirAll(live, 0o750); err != nil {
		t.Fatal(err)
	}
	seedLegacyWorkspace(t, live)

	ghost1 := filepath.Join(t.TempDir(), "gone-1")
	ghost2 := filepath.Join(t.TempDir(), "gone-2")
	// Intentionally do not create these dirs — they represent
	// registry entries whose t.TempDir() cleanup wiped the project.

	reg, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.Register(live, "live")
	reg.Register(ghost1, "ghost-1")
	reg.Register(ghost2, "ghost-2")
	if saveErr := reg.Save(); saveErr != nil {
		t.Fatal(saveErr)
	}

	results, err := MigrateAllWorkspaceRuntimes(context.Background())
	if err != nil {
		t.Fatalf("MigrateAllWorkspaceRuntimes: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result (only the live workspace), got %d: %+v", len(results), results)
	}
	for _, r := range results {
		if r.ProjectPath == ghost1 || r.ProjectPath == ghost2 {
			t.Errorf("ghost project migrated: %s", r.ProjectPath)
		}
	}

	// Confirm the registry is now pruned.
	reg2, _ := workspace.LoadRegistry()
	if got := len(reg2.List()); got != 1 {
		t.Errorf("registry not pruned: %d entries remain", got)
	}
}
