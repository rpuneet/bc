package doctor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/home"
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
		"home":     true,
		"database": true,
		"agents":   true,
		"tools":    true,
		"git":      true,
		"daemon":   true,
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
	t.Setenv("MYCEL_TEST_ENV_NOTSET", "")
	item := checkEnvVar("MYCEL_TEST_ENV_NOTSET")
	if item.Severity != SeverityWarn {
		t.Errorf("unset env var: severity = %s, want warn", item.Severity)
	}
	if item.Message != "not set" {
		t.Errorf("unset env var: message = %q, want %q", item.Message, "not set")
	}
}

func TestCheckEnvVar_Set(t *testing.T) {
	t.Setenv("MYCEL_TEST_ENV_SET", "sk-ant-12345678901234567890abcd")
	item := checkEnvVar("MYCEL_TEST_ENV_SET")
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
	t.Setenv("MYCEL_TEST_SHORT", "abc")
	item := checkEnvVar("MYCEL_TEST_SHORT")
	if item.Severity != SeverityOK {
		t.Errorf("short set env var: severity = %s, want ok", item.Severity)
	}
	if item.Message != "abc" {
		t.Errorf("short env var: message = %q, want %q", item.Message, "abc")
	}
}

// ─── Test homes ──────────────────────────────────────────────────────────────

// newBootstrappedHome points MYCEL_HOME at a fresh temp dir and
// bootstraps the full ~/.mycel structure (prefs.json, dirs, DB-backed
// roles) via home.Open. Returns the Home and the home dir.
func newBootstrappedHome(t *testing.T) (*home.Home, string) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("MYCEL_HOME", homeDir)
	h, err := home.Open("")
	if err != nil {
		t.Fatalf("home.Open: %v", err)
	}
	return h, homeDir
}

// newEmptyHome points MYCEL_HOME at a path that does NOT exist
// and returns a bare Home value — used for missing-state scenarios.
func newEmptyHome(t *testing.T) (*home.Home, string) {
	t.Helper()
	homeDir := filepath.Join(t.TempDir(), "no-mycel-home")
	t.Setenv("MYCEL_HOME", homeDir)
	return &home.Home{}, homeDir
}

// ─── CheckHome ──────────────────────────────────────────────────────────

func TestCheckHome_MissingHome(t *testing.T) {
	h, _ := newEmptyHome(t)
	// ~/.mycel does not exist — should fail immediately
	cat := CheckHome(h)
	if len(cat.Items) != 1 {
		t.Fatalf("expected exactly one item, got %d: %+v", len(cat.Items), cat.Items)
	}
	if cat.Items[0].Severity != SeverityFail {
		t.Errorf("missing ~/.mycel: severity = %s, want fail", cat.Items[0].Severity)
	}
}

func TestCheckHome_ValidStructure(t *testing.T) {
	h, _ := newBootstrappedHome(t)

	cat := CheckHome(h)

	ok, _, fail := cat.Counts()
	if fail > 0 {
		t.Errorf("valid home: got %d failures, want 0", fail)
		for _, item := range cat.Items {
			if item.Severity == SeverityFail {
				t.Logf("  FAIL: %s — %s", item.Name, item.Message)
			}
		}
	}
	if ok == 0 {
		t.Error("valid home: expected at least one ok item")
	}

	// The bootstrapped home must report prefs.json and DB-backed roles ok.
	var prefsOK, rolesOK, agentsOK bool
	for _, item := range cat.Items {
		switch item.Name {
		case home.PrefsFileName:
			prefsOK = item.Severity == SeverityOK
		case "roles":
			rolesOK = item.Severity == SeverityOK
		case "agents/":
			agentsOK = item.Severity == SeverityOK
		}
	}
	if !prefsOK {
		t.Errorf("expected ok item for %s", home.PrefsFileName)
	}
	if !rolesOK {
		t.Error("expected ok item for DB-backed roles (defaults seeded by Open)")
	}
	if !agentsOK {
		t.Error("expected ok item for agents/ directory")
	}
}

func TestCheckHome_MissingPrefs(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)

	// Remove the global prefs file — doctor must flag it as a failure.
	if err := os.Remove(filepath.Join(homeDir, home.PrefsFileName)); err != nil {
		t.Fatal(err)
	}

	cat := CheckHome(h)

	var foundPrefsFail bool
	for _, item := range cat.Items {
		if item.Name == home.PrefsFileName && item.Severity == SeverityFail {
			foundPrefsFail = true
			if item.Fix == "" {
				t.Error("missing prefs.json fail item should carry a fix hint")
			}
		}
	}
	if !foundPrefsFail {
		t.Errorf("expected a fail item for missing %s", home.PrefsFileName)
	}
}

func TestCheckHome_MissingAgentsDir(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)

	// Remove agents/ — doctor should warn, not fail.
	if err := os.RemoveAll(filepath.Join(homeDir, "agents")); err != nil {
		t.Fatal(err)
	}

	cat := CheckHome(h)

	var foundAgentsWarn bool
	for _, item := range cat.Items {
		if item.Name == "agents/" && item.Severity == SeverityWarn {
			foundAgentsWarn = true
		}
	}
	if !foundAgentsWarn {
		t.Error("expected a warn item for missing agents/ directory")
	}
}

func TestCheckHome_InvalidConfig(t *testing.T) {
	h, _ := newBootstrappedHome(t)

	// Point the home at an invalid config (bad version) — the
	// prefs.json file exists, so doctor validates the loaded config.
	h.Config = &home.Config{Version: 99}

	cat := CheckHome(h)

	var foundConfigFail bool
	for _, item := range cat.Items {
		if item.Name == home.PrefsFileName && item.Severity == SeverityFail {
			foundConfigFail = true
		}
	}
	if !foundConfigFail {
		t.Errorf("expected a fail item for invalid %s", home.PrefsFileName)
	}
}

// ─── CheckDatabase ───────────────────────────────────────────────────────────

func TestCheckDatabase_NoDB(t *testing.T) {
	h, homeDir := newEmptyHome(t)
	if err := os.MkdirAll(homeDir, 0750); err != nil {
		t.Fatal(err)
	}

	cat := CheckDatabase(context.Background(), h)

	// With no mycel.db, we expect warnings (not found = will be created on use)
	_, warn, fail := cat.Counts()
	if fail > 0 {
		t.Errorf("no db file: got %d failures, want 0", fail)
	}
	if warn == 0 {
		t.Error("no db file: expected at least one warn item")
	}
}

func TestCheckDatabase_ValidDB(t *testing.T) {
	h, homeDir := newEmptyHome(t)
	if err := os.MkdirAll(homeDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create a valid mycel.db with all required tables.
	dbPath := filepath.Join(homeDir, db.GlobalDBFileName)
	if err := createTestDB(t, dbPath, "agents", "channels", "messages"); err != nil {
		t.Fatal(err)
	}

	cat := CheckDatabase(context.Background(), h)

	_, _, fail := cat.Counts()
	if fail > 0 {
		t.Errorf("valid db: got %d failures, want 0: %+v", fail, cat.Items)
	}
	var integrityOK, agentsOK bool
	for _, item := range cat.Items {
		if item.Name == db.GlobalDBFileName+" integrity" && item.Severity == SeverityOK {
			integrityOK = true
		}
		if item.Name == db.GlobalDBFileName+`: table "agents"` && item.Severity == SeverityOK {
			agentsOK = true
		}
	}
	if !integrityOK {
		t.Error("expected ok integrity item for mycel.db")
	}
	if !agentsOK {
		t.Error("expected ok item for agents table")
	}
}

func TestCheckDatabase_MissingTable(t *testing.T) {
	h, homeDir := newEmptyHome(t)
	if err := os.MkdirAll(homeDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create mycel.db WITHOUT any tables.
	dbPath := filepath.Join(homeDir, db.GlobalDBFileName)
	if err := createTestDB(t, dbPath /*, no tables*/); err != nil {
		t.Fatal(err)
	}

	cat := CheckDatabase(context.Background(), h)

	var foundMissingTable bool
	for _, item := range cat.Items {
		if item.Name == db.GlobalDBFileName+`: table "agents"` && item.Severity == SeverityFail {
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
	sqlDB, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			t.Logf("close test db: %v", closeErr)
		}
	}()
	// Force SQLite to create the file by running a lightweight pragma.
	if _, err := sqlDB.ExecContext(context.Background(), "PRAGMA user_version = 1"); err != nil {
		return err
	}
	for _, table := range tables {
		if _, err := sqlDB.ExecContext(context.Background(), "CREATE TABLE IF NOT EXISTS "+table+" (id INTEGER PRIMARY KEY)"); err != nil { //nolint:gosec // test helper, table names are test-controlled
			return err
		}
	}
	return nil
}

// ─── CheckTools ──────────────────────────────────────────────────────────────

func TestCheckTools_ReturnsItems(t *testing.T) {
	ctx := context.Background()
	cat := CheckTools(ctx, nil)

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
	cat := CheckTools(ctx, nil)

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
	cat := CheckTools(ctx, nil)

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

func TestCheckAgents_FreshHome(t *testing.T) {
	h, _ := newBootstrappedHome(t)

	ctx := context.Background()
	cat := CheckAgents(ctx, h)

	if cat.Name != "Agents" {
		t.Errorf("category name = %q, want %q", cat.Name, "Agents")
	}
	// A fresh home has no agents — the check must still report something.
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
	h, _ := newBootstrappedHome(t)
	ctx := context.Background()

	result := CategoryByName(ctx, h, "nonexistent")
	if result != nil {
		t.Errorf("unknown category: expected nil, got %+v", result)
	}
}

func TestCategoryByName_KnownCategories(t *testing.T) {
	h, _ := newBootstrappedHome(t)
	ctx := context.Background()

	for _, name := range ValidCategories() {
		result := CategoryByName(ctx, h, name)
		if result == nil {
			t.Errorf("CategoryByName(%q) returned nil, want non-nil", name)
		}
	}
}

// ─── Fix ─────────────────────────────────────────────────────────────────────

func TestFix_DryRun_NoChanges(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)

	// Remove agents/ so a fix would have something to create.
	if err := os.RemoveAll(filepath.Join(homeDir, "agents")); err != nil {
		t.Fatal(err)
	}

	// Build a report with a missing agents/ dir
	cat := CategoryReport{
		Name: "Home",
		Items: []Item{
			{Name: "agents/", Severity: SeverityWarn, Message: "missing"},
		},
	}
	report := &Report{Categories: []CategoryReport{cat}}

	ctx := context.Background()
	results := Fix(ctx, h, report, true /* dryRun */)

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
	agentsDir := h.AgentsDir()
	if _, err := os.Stat(agentsDir); err == nil {
		t.Error("dry-run should not have created agents/ directory")
	}
}

func TestFix_HomeAgentsDir_Creates(t *testing.T) {
	h, homeDir := newBootstrappedHome(t)

	// agents/ is missing
	if err := os.RemoveAll(filepath.Join(homeDir, "agents")); err != nil {
		t.Fatal(err)
	}
	cat := CategoryReport{
		Name: "Home",
		Items: []Item{
			{Name: "agents/", Severity: SeverityWarn, Message: "missing"},
		},
	}
	report := &Report{Categories: []CategoryReport{cat}}

	ctx := context.Background()
	results := Fix(ctx, h, report, false /* not dryRun */)

	if len(results) == 0 {
		t.Error("expected at least one fix result")
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("fix failed: %s — %s", r.Action, r.Message)
		}
	}

	// Verify agents/ was created
	if _, err := os.Stat(h.AgentsDir()); err != nil {
		t.Errorf("agents/ should have been created: %v", err)
	}
}

// TestCheckAgentImages covers the docker agent-image doctor check: a
// provider without its mycel-agent image warns with a build hint; no
// docker → no items at all.
func TestCheckAgentImages(t *testing.T) {
	ctx := context.Background()
	orig := listDockerImages
	t.Cleanup(func() { listDockerImages = orig })

	t.Run("no docker yields no items", func(t *testing.T) {
		listDockerImages = func(context.Context) []string { return nil }
		if items := checkAgentImages(ctx); items != nil {
			t.Errorf("expected no items without docker, got %d", len(items))
		}
	})

	t.Run("missing and present images", func(t *testing.T) {
		listDockerImages = func(context.Context) []string {
			return []string{"mycel-agent-claude:latest", "mycel-agent-agy:latest", "ubuntu:24.04"}
		}
		items := checkAgentImages(ctx)
		if len(items) == 0 {
			t.Fatal("expected one item per provider")
		}
		bySeverity := map[string]Severity{}
		for _, it := range items {
			bySeverity[it.Name] = it.Severity
		}
		if got := bySeverity["image:mycel-agent-claude:latest"]; got != SeverityOK {
			t.Errorf("claude image severity = %v, want OK", got)
		}
		if got := bySeverity["image:mycel-agent-agy:latest"]; got != SeverityOK {
			t.Errorf("agy image severity = %v, want OK", got)
		}
		if got := bySeverity["image:mycel-agent-cursor:latest"]; got != SeverityWarn {
			t.Errorf("cursor missing image severity = %v, want Warn", got)
		}
		for _, it := range items {
			if it.Severity == SeverityWarn && it.Fix == "" {
				t.Errorf("warn item %q has no fix hint", it.Name)
			}
		}
	})
}
