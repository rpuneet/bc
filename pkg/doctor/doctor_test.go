package doctor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/rpuneet/mycel/pkg/workspace"
)

// ─── Severity ────────────────────────────────────────────────────────────────

func TestSeverity_String(t *testing.T) {
	tests := []struct { //nolint:govet // test struct, field order matches literal values
		want string
		sev  Severity
	}{
		{"ok", SeverityOK},
		{"warn", SeverityWarn},
		{"fail", SeverityFail},
		{"fail", Severity(99)}, // unknown → fail
	}
	for _, tt := range tests {
		if got := tt.sev.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.sev, got, tt.want)
		}
	}
}

// ─── CategoryReport.Counts ───────────────────────────────────────────────────

func TestCategoryReport_Counts(t *testing.T) {
	cat := CategoryReport{
		Name: "test",
		Items: []Item{
			{Severity: SeverityOK},
			{Severity: SeverityOK},
			{Severity: SeverityWarn},
			{Severity: SeverityFail},
			{Severity: SeverityFail},
			{Severity: SeverityFail},
		},
	}
	ok, warn, fail := cat.Counts()
	if ok != 2 {
		t.Errorf("ok = %d, want 2", ok)
	}
	if warn != 1 {
		t.Errorf("warn = %d, want 1", warn)
	}
	if fail != 3 {
		t.Errorf("fail = %d, want 3", fail)
	}
}

func TestCategoryReport_Counts_Empty(t *testing.T) {
	cat := CategoryReport{Name: "empty"}
	ok, warn, fail := cat.Counts()
	if ok != 0 || warn != 0 || fail != 0 {
		t.Errorf("empty category got ok=%d warn=%d fail=%d, want 0/0/0", ok, warn, fail)
	}
}

// ─── Report.Summary ──────────────────────────────────────────────────────────

func TestReport_Summary(t *testing.T) {
	r := &Report{
		Categories: []CategoryReport{
			{Items: []Item{{Severity: SeverityOK}, {Severity: SeverityFail}}},
			{Items: []Item{{Severity: SeverityWarn}, {Severity: SeverityOK}}},
		},
	}
	ok, warn, fail := r.Summary()
	if ok != 2 {
		t.Errorf("ok = %d, want 2", ok)
	}
	if warn != 1 {
		t.Errorf("warn = %d, want 1", warn)
	}
	if fail != 1 {
		t.Errorf("fail = %d, want 1", fail)
	}
}

// ─── ValidCategories ─────────────────────────────────────────────────────────

func TestValidCategories(t *testing.T) {
	cats := ValidCategories()
	if len(cats) == 0 {
		t.Fatal("ValidCategories() returned empty slice")
	}
	want := map[string]bool{
		"workspace": true,
		"database":  true,
		"agents":    true,
		"tools":     true,
		"git":       true,
		"daemon":    true,
	}
	for _, c := range cats {
		if !want[c] {
			t.Errorf("unexpected category %q", c)
		}
		delete(want, c)
	}
	for missing := range want {
		t.Errorf("missing category %q", missing)
	}
}

// ─── checkEnvVar ─────────────────────────────────────────────────────────────

func TestCheckEnvVar_NotSet(t *testing.T) {
	t.Setenv("BC_TEST_ENV_NOTSET", "")
	item := checkEnvVar("BC_TEST_ENV_NOTSET")
	if item.Severity != SeverityWarn {
		t.Errorf("unset env var: severity = %s, want warn", item.Severity)
	}
	if item.Message != "not set" {
		t.Errorf("unset env var: message = %q, want %q", item.Message, "not set")
	}
}

func TestCheckEnvVar_Set(t *testing.T) {
	t.Setenv("BC_TEST_ENV_SET", "sk-ant-12345678901234567890abcd")
	item := checkEnvVar("BC_TEST_ENV_SET")
	if item.Severity != SeverityOK {
		t.Errorf("set env var: severity = %s, want ok", item.Severity)
	}
	// Value should be masked
	if item.Message == "sk-ant-12345678901234567890abcd" {
		t.Error("env var value should be masked, got raw value")
	}
	if len(item.Message) == 0 {
		t.Error("masked value should not be empty")
	}
}

func TestCheckEnvVar_ShortValue(t *testing.T) {
	// Values shorter than 8 chars: shown as-is (no masking)
	t.Setenv("BC_TEST_SHORT", "abc")
	item := checkEnvVar("BC_TEST_SHORT")
	if item.Severity != SeverityOK {
		t.Errorf("short set env var: severity = %s, want ok", item.Severity)
	}
	if item.Message != "abc" {
		t.Errorf("short env var: message = %q, want %q", item.Message, "abc")
	}
}

// ─── CheckWorkspace ──────────────────────────────────────────────────────────

// newMinimalWorkspace creates a workspace pointing to a temp directory without
// creating any files — used to test missing-directory scenarios.
func newMinimalWorkspace(t *testing.T) (*workspace.Workspace, string) {
	t.Helper()
	dir := t.TempDir()
	return &workspace.Workspace{RootDir: dir}, dir
}

func TestCheckWorkspace_MissingStateDir(t *testing.T) {
	ws, _ := newMinimalWorkspace(t)
	// Don't create .bc/ — should fail immediately
	cat := CheckWorkspace(ws)
	if len(cat.Items) == 0 {
		t.Fatal("expected at least one item")
	}
	if cat.Items[0].Severity != SeverityFail {
		t.Errorf("missing .bc/: severity = %s, want fail", cat.Items[0].Severity)
	}
}

func TestCheckWorkspace_ValidStructure(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")

	// Create all required directories
	if err := os.MkdirAll(filepath.Join(stateDir, "roles"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatal(err)
	}

	// Create a valid settings.json
	cfg := workspace.DefaultConfig()
	configPath := filepath.Join(stateDir, "settings.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	ws.Config = &cfg

	// Create a role file
	if err := os.WriteFile(filepath.Join(stateDir, "roles", "root.md"), []byte("# root"), 0600); err != nil {
		t.Fatal(err)
	}

	cat := CheckWorkspace(ws)

	ok, _, fail := cat.Counts()
	if fail > 0 {
		t.Errorf("valid workspace: got %d failures, want 0", fail)
		for _, item := range cat.Items {
			if item.Severity == SeverityFail {
				t.Logf("  FAIL: %s — %s", item.Name, item.Message)
			}
		}
	}
	if ok == 0 {
		t.Error("valid workspace: expected at least one ok item")
	}
}

func TestCheckWorkspace_MissingRoles(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")

	// Create state dir and agents, but NOT roles/
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatal(err)
	}
	cfg := workspace.DefaultConfig()
	configPath := filepath.Join(stateDir, "settings.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	ws.Config = &cfg

	cat := CheckWorkspace(ws)

	var foundRolesWarn bool
	for _, item := range cat.Items {
		if item.Name == "roles/" && item.Severity == SeverityWarn {
			foundRolesWarn = true
		}
	}
	if !foundRolesWarn {
		t.Error("expected a warn item for missing roles/ directory")
	}
}

func TestCheckWorkspace_EmptyRoles(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")

	// Create empty roles dir
	if err := os.MkdirAll(filepath.Join(stateDir, "roles"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatal(err)
	}
	cfg := workspace.DefaultConfig()
	configPath := filepath.Join(stateDir, "settings.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	ws.Config = &cfg

	cat := CheckWorkspace(ws)

	var foundNoRolesWarn bool
	for _, item := range cat.Items {
		if item.Name == "roles/" && item.Severity == SeverityWarn {
			foundNoRolesWarn = true
		}
	}
	if !foundNoRolesWarn {
		t.Error("expected a warn item for roles/ with no .md files")
	}
}

func TestCheckWorkspace_InvalidConfig(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")

	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Write an invalid config (missing required workspace.name)
	configPath := filepath.Join(stateDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Load the bad config directly into ws
	cfg := workspace.Config{}
	ws.Config = &cfg

	cat := CheckWorkspace(ws)

	var foundConfigFail bool
	for _, item := range cat.Items {
		if item.Name == "settings.json" && item.Severity == SeverityFail {
			foundConfigFail = true
		}
	}
	if !foundConfigFail {
		t.Error("expected a fail item for invalid settings.json")
	}
}

// ─── CheckDatabase ───────────────────────────────────────────────────────────

func TestCheckDatabase_NoDB(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	if err := os.MkdirAll(filepath.Join(dir, ".bc"), 0750); err != nil {
		t.Fatal(err)
	}

	cat := CheckDatabase(context.Background(), ws)

	// With no databases, we expect warnings (not found = will be created on use)
	_, warn, fail := cat.Counts()
	if fail > 0 {
		t.Errorf("no db files: got %d failures, want 0", fail)
	}
	if warn == 0 {
		t.Error("no db files: expected at least one warn item")
	}
}

func TestCheckDatabase_ValidDB(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create a valid state.db with agents table
	stateDBPath := filepath.Join(stateDir, "state.db")
	if err := createTestDB(t, stateDBPath, "agents"); err != nil {
		t.Fatal(err)
	}

	cat := CheckDatabase(context.Background(), ws)

	for _, item := range cat.Items {
		if item.Name == "state.db integrity" && item.Severity != SeverityOK {
			t.Errorf("state.db integrity: severity = %s, want ok", item.Severity)
		}
		if item.Name == `state.db: table "agents"` && item.Severity != SeverityOK {
			t.Errorf("agents table: severity = %s, want ok", item.Severity)
		}
	}
}

func TestCheckDatabase_MissingTable(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create bc.db WITHOUT the agents table
	bcDBPath := filepath.Join(stateDir, "bc.db")
	if err := createTestDB(t, bcDBPath /*, no tables*/); err != nil {
		t.Fatal(err)
	}

	cat := CheckDatabase(context.Background(), ws)

	var foundMissingTable bool
	for _, item := range cat.Items {
		if item.Name == `bc.db: table "agents"` && item.Severity == SeverityFail {
			foundMissingTable = true
		}
	}
	if !foundMissingTable {
		t.Error("expected a fail item for missing agents table")
	}
}

// createTestDB creates a minimal SQLite database with the given tables.
// Always forces file creation by running PRAGMA user_version.
func createTestDB(t *testing.T, path string, tables ...string) error {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close test db: %v", closeErr)
		}
	}()
	// Force SQLite to create the file by running a lightweight pragma.
	if _, err := db.ExecContext(context.Background(), "PRAGMA user_version = 1"); err != nil {
		return err
	}
	for _, table := range tables {
		if _, err := db.ExecContext(context.Background(), "CREATE TABLE IF NOT EXISTS "+table+" (id INTEGER PRIMARY KEY)"); err != nil { //nolint:gosec // test helper, table names are test-controlled
			return err
		}
	}
	return nil
}

// ─── CheckTools ──────────────────────────────────────────────────────────────

func TestCheckTools_ReturnsItems(t *testing.T) {
	ctx := context.Background()
	cat := CheckTools(ctx)

	if cat.Name != "Tools" {
		t.Errorf("category name = %q, want %q", cat.Name, "Tools")
	}
	if len(cat.Items) == 0 {
		t.Error("expected at least one tool check item")
	}

	// tmux and git must always be checked
	var hasTmux, hasGit, hasAPIKey bool
	for _, item := range cat.Items {
		switch item.Name {
		case "tmux":
			hasTmux = true
		case "git":
			hasGit = true
		case "ANTHROPIC_API_KEY":
			hasAPIKey = true
		}
	}
	if !hasTmux {
		t.Error("expected tmux check item")
	}
	if !hasGit {
		t.Error("expected git check item")
	}
	if !hasAPIKey {
		t.Error("expected ANTHROPIC_API_KEY check item")
	}
}

func TestCheckTools_ANTHROPICAPIKey_Warn(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	ctx := context.Background()
	cat := CheckTools(ctx)

	for _, item := range cat.Items {
		if item.Name == "ANTHROPIC_API_KEY" {
			if item.Severity != SeverityWarn {
				t.Errorf("unset ANTHROPIC_API_KEY: severity = %s, want warn", item.Severity)
			}
			return
		}
	}
	t.Error("ANTHROPIC_API_KEY item not found in tools check")
}

func TestCheckTools_ANTHROPICAPIKey_OK(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-1234567890abcdef")
	ctx := context.Background()
	cat := CheckTools(ctx)

	for _, item := range cat.Items {
		if item.Name == "ANTHROPIC_API_KEY" {
			if item.Severity != SeverityOK {
				t.Errorf("set ANTHROPIC_API_KEY: severity = %s, want ok", item.Severity)
			}
			return
		}
	}
	t.Error("ANTHROPIC_API_KEY item not found in tools check")
}

// ─── CheckAgents ─────────────────────────────────────────────────────────────

func TestCheckAgents_NoStateDB(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	if err := os.MkdirAll(filepath.Join(dir, ".bc", "agents"), 0750); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cat := CheckAgents(ctx, ws)

	if cat.Name != "Agents" {
		t.Errorf("category name = %q, want %q", cat.Name, "Agents")
	}
	// With no state.db, LoadState may warn or return no agents
	if len(cat.Items) == 0 {
		t.Error("expected at least one item from agents check")
	}
}

// ─── parseWorktrees ──────────────────────────────────────────────────────────

func TestParseWorktrees_MainOnly(t *testing.T) {
	dir := t.TempDir()
	output := "worktree " + dir + "\nHEAD abc123\nbranch refs/heads/main\n\n"
	valid, orphaned := parseWorktrees(output, dir)
	if valid != 1 {
		t.Errorf("valid = %d, want 1", valid)
	}
	if len(orphaned) != 0 {
		t.Errorf("orphaned = %v, want []", orphaned)
	}
}

func TestParseWorktrees_WithValidWorktree(t *testing.T) {
	mainDir := t.TempDir()
	extraDir := t.TempDir() // exists on disk → valid

	output := "worktree " + mainDir + "\nHEAD abc123\nbranch refs/heads/main\n\n" +
		"worktree " + extraDir + "\nHEAD def456\nbranch refs/heads/feat\n\n"

	valid, orphaned := parseWorktrees(output, mainDir)
	if valid != 2 {
		t.Errorf("valid = %d, want 2", valid)
	}
	if len(orphaned) != 0 {
		t.Errorf("orphaned = %v, want []", orphaned)
	}
}

func TestParseWorktrees_WithOrphanedWorktree(t *testing.T) {
	mainDir := t.TempDir()
	missingDir := filepath.Join(t.TempDir(), "nonexistent", "path")
	// missingDir does not exist

	output := "worktree " + mainDir + "\nHEAD abc123\nbranch refs/heads/main\n\n" +
		"worktree " + missingDir + "\nHEAD def456\nbranch refs/heads/feat\n\n"

	valid, orphaned := parseWorktrees(output, mainDir)
	if valid != 1 {
		t.Errorf("valid = %d, want 1", valid)
	}
	if len(orphaned) != 1 || orphaned[0] != missingDir {
		t.Errorf("orphaned = %v, want [%s]", orphaned, missingDir)
	}
}

func TestParseWorktrees_Empty(t *testing.T) {
	valid, orphaned := parseWorktrees("", "/some/dir")
	if valid != 0 {
		t.Errorf("valid = %d, want 0", valid)
	}
	if len(orphaned) != 0 {
		t.Errorf("orphaned = %v, want []", orphaned)
	}
}

// ─── CategoryByName ──────────────────────────────────────────────────────────

func TestCategoryByName_Unknown(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	if err := os.MkdirAll(filepath.Join(dir, ".bc"), 0750); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	result := CategoryByName(ctx, ws, "nonexistent")
	if result != nil {
		t.Errorf("unknown category: expected nil, got %+v", result)
	}
}

func TestCategoryByName_KnownCategories(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	if err := os.MkdirAll(filepath.Join(dir, ".bc"), 0750); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for _, name := range ValidCategories() {
		result := CategoryByName(ctx, ws, name)
		if result == nil {
			t.Errorf("CategoryByName(%q) returned nil, want non-nil", name)
		}
	}
}

// ─── Fix ─────────────────────────────────────────────────────────────────────

func TestFix_DryRun_NoChanges(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg := workspace.DefaultConfig()
	configPath := filepath.Join(stateDir, "settings.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	ws.Config = &cfg

	// Build a report with a missing agents/ dir
	cat := CategoryReport{
		Name: "Workspace",
		Items: []Item{
			{Name: "agents/", Severity: SeverityWarn, Message: "missing"},
		},
	}
	report := &Report{Categories: []CategoryReport{cat}}

	ctx := context.Background()
	results := Fix(ctx, ws, report, true /* dryRun */)

	// Dry-run should return results but NOT create the directory
	for _, r := range results {
		if !r.Success {
			t.Errorf("dry-run fix reported failure: %s — %s", r.Action, r.Message)
		}
		if r.Message != "[dry-run]" {
			t.Errorf("dry-run result message = %q, want %q", r.Message, "[dry-run]")
		}
	}

	// Verify nothing was actually created
	agentsDir := ws.AgentsDir()
	if _, err := os.Stat(agentsDir); err == nil {
		t.Error("dry-run should not have created agents/ directory")
	}
}

func TestFix_WorkspaceDir_Creates(t *testing.T) {
	ws, dir := newMinimalWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	cfg := workspace.DefaultConfig()
	configPath := filepath.Join(stateDir, "settings.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	ws.Config = &cfg

	// agents/ is missing
	cat := CategoryReport{
		Name: "Workspace",
		Items: []Item{
			{Name: "agents/", Severity: SeverityWarn, Message: "missing"},
		},
	}
	report := &Report{Categories: []CategoryReport{cat}}

	ctx := context.Background()
	results := Fix(ctx, ws, report, false /* not dryRun */)

	if len(results) == 0 {
		t.Error("expected at least one fix result")
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("fix failed: %s — %s", r.Action, r.Message)
		}
	}

	// Verify agents/ was created
	if _, err := os.Stat(ws.AgentsDir()); err != nil {
		t.Errorf("agents/ should have been created: %v", err)
	}
}
