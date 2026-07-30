package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// DefaultConfig tests are in config_test.go

// --- Open ---

func TestOpen(t *testing.T) {
	home := setTestHome(t)
	dir := newTestRepo(t)

	ws, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if ws.RootDir == "" {
		t.Error("RootDir is empty")
	}
	if ws.Config == nil {
		t.Fatal("Config is nil")
	}
	if ws.Name() != filepath.Base(dir) {
		t.Errorf("Name() = %q, want %q", ws.Name(), filepath.Base(dir))
	}

	// State is global: StateDir is MYCEL_HOME, not inside the repo.
	if ws.StateDir() != home {
		t.Errorf("StateDir() = %q, want %q", ws.StateDir(), home)
	}
	if _, statErr := os.Stat(home); statErr != nil {
		t.Errorf("mycel home not created: %v", statErr)
	}

	// prefs.json was written into MYCEL_HOME.
	prefsPath := filepath.Join(home, PrefsFileName)
	if _, statErr := os.Stat(prefsPath); statErr != nil {
		t.Fatalf("prefs.json not written: %v", statErr)
	}
	if ws.SettingsFile() != prefsPath {
		t.Errorf("SettingsFile() = %q, want %q", ws.SettingsFile(), prefsPath)
	}

	// The repo stays pristine — no .bc/ marker.
	if _, statErr := os.Stat(filepath.Join(dir, ".bc")); !os.IsNotExist(statErr) {
		t.Errorf(".bc marker should not exist in repo, stat err = %v", statErr)
	}
}

func TestOpenNoAnchorRepo(t *testing.T) {
	home := setTestHome(t)

	ws, err := Open("")
	if err != nil {
		t.Fatalf(`Open(""): %v`, err)
	}
	if ws.RootDir != "" {
		t.Errorf("RootDir = %q, want empty", ws.RootDir)
	}
	if ws.Name() != "mycel" {
		t.Errorf("Name() = %q, want %q", ws.Name(), "mycel")
	}
	if ws.Config == nil {
		t.Fatal("Config is nil")
	}
	if _, statErr := os.Stat(filepath.Join(home, PrefsFileName)); statErr != nil {
		t.Errorf("prefs.json not written: %v", statErr)
	}
}

func TestOpenRejectsNonGitDir(t *testing.T) {
	setTestHome(t)
	dir := t.TempDir() // no .git

	if _, err := Open(dir); err == nil {
		t.Fatal("Open on a non-git dir: expected error, got nil")
	}
}

func TestOpenIdempotent(t *testing.T) {
	setTestHome(t)
	dir := newTestRepo(t)

	ws1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if ws1.Name() != ws2.Name() {
		t.Errorf("second Open changed Name: %q vs %q", ws1.Name(), ws2.Name())
	}
}

// TestOpenPreservesExistingConfig: a second Open must load the existing
// prefs.json instead of overwriting it with defaults.
func TestOpenPreservesExistingConfig(t *testing.T) {
	setTestHome(t)
	dir := newTestRepo(t)

	ws, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws.Config.User.Name = "@custom-user"
	if saveErr := ws.Save(); saveErr != nil {
		t.Fatal(saveErr)
	}

	ws2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws2.Config.User.Name != "@custom-user" {
		t.Errorf("Open overwrote existing config: User.Name = %q, want %q",
			ws2.Config.User.Name, "@custom-user")
	}
}

// --- Load ---

func TestLoad(t *testing.T) {
	setTestHome(t)
	dir := newTestRepo(t)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}

	ws, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config == nil {
		t.Error("Config should not be nil")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RootDir != absDir {
		t.Errorf("RootDir = %q, want %q", ws.RootDir, absDir)
	}
}

// TestLoadNotBootstrapped: Load is strict — without prefs.json (no prior
// Open / `mycel up`) it must fail.
func TestLoadNotBootstrapped(t *testing.T) {
	setTestHome(t)
	dir := newTestRepo(t)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load before bootstrap: expected error, got nil")
	}
}

func TestLoadRejectsNonGitDir(t *testing.T) {
	setTestHome(t)
	if _, err := Open(""); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("Load on a non-git dir: expected error, got nil")
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	home := setTestHome(t)
	dir := newTestRepo(t)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	// Corrupt the active prefs.json — Load must fail loudly.
	prefs := filepath.Join(home, PrefsFileName)
	if writeErr := os.WriteFile(prefs, []byte("{{bad"), 0600); writeErr != nil {
		t.Fatal(writeErr)
	}

	if _, loadErr := Load(dir); loadErr == nil {
		t.Fatal("Load invalid config: expected error, got nil")
	}
}

// --- Find (upward git-root search) ---

func TestFindInCurrentDir(t *testing.T) {
	setTestHome(t)
	dir := newTestRepo(t)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}

	ws, err := Find(dir)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RootDir != absDir {
		t.Errorf("RootDir = %q, want %q", ws.RootDir, absDir)
	}
}

func TestFindInParentDir(t *testing.T) {
	setTestHome(t)
	parent := newTestRepo(t)
	if _, err := Open(parent); err != nil {
		t.Fatal(err)
	}

	// Create a child directory (no git repo of its own)
	child := filepath.Join(parent, "subdir", "deep")
	if err := os.MkdirAll(child, 0750); err != nil {
		t.Fatal(err)
	}

	ws, err := Find(child)
	if err != nil {
		t.Fatalf("Find from child: %v", err)
	}

	absParent, err := filepath.Abs(parent)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RootDir != absParent {
		t.Errorf("RootDir = %q, want %q (parent)", ws.RootDir, absParent)
	}
}

// TestFindNestedRepos: Find anchors on the NEAREST enclosing git root.
func TestFindNestedRepos(t *testing.T) {
	setTestHome(t)
	outer := newTestRepo(t)
	if _, err := Open(outer); err != nil {
		t.Fatal(err)
	}

	// Inner git repo inside outer
	inner := filepath.Join(outer, "projects", "sub")
	if err := os.MkdirAll(inner, 0750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, inner)

	// Find from inner should anchor on the inner repo, not outer
	ws, err := Find(inner)
	if err != nil {
		t.Fatal(err)
	}
	absInner, err := filepath.Abs(inner)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RootDir != absInner {
		t.Errorf("RootDir = %q, want inner %q", ws.RootDir, absInner)
	}

	// Find from a child of inner should still anchor on inner
	deepChild := filepath.Join(inner, "src", "pkg")
	if mkdirErr := os.MkdirAll(deepChild, 0750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	ws2, err := Find(deepChild)
	if err != nil {
		t.Fatal(err)
	}
	if ws2.RootDir != absInner {
		t.Errorf("RootDir = %q, want inner %q", ws2.RootDir, absInner)
	}
}

func TestFindNoGitRepo(t *testing.T) {
	setTestHome(t)
	if _, err := Open(""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir() // not a git repo

	_, err := Find(dir)
	if err == nil {
		t.Fatal("Find outside any git repo: expected error, got nil")
	}
}

// --- Save ---

func TestSave(t *testing.T) {
	home := setTestHome(t)
	dir := newTestRepo(t)
	ws, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	ws.Config.User.Name = "@saved-user"
	if saveErr := ws.Save(); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	// Save writes to the global prefs.json.
	if _, statErr := os.Stat(filepath.Join(home, PrefsFileName)); statErr != nil {
		t.Fatalf("prefs.json missing after Save: %v", statErr)
	}

	// Reload and verify the change round-tripped.
	ws2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws2.Config.User.Name != "@saved-user" {
		t.Errorf("User.Name after reload = %q, want %q", ws2.Config.User.Name, "@saved-user")
	}
}

// --- Path helpers ---

func TestPathHelpers(t *testing.T) {
	home := setTestHome(t)
	dir := newTestRepo(t)
	ws, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"StateDir", ws.StateDir(), home},
		{"AgentsDir", ws.AgentsDir(), filepath.Join(home, "agents")},
		{"LogsDir", ws.LogsDir(), filepath.Join(home, "logs")},
		{"RolesDir", ws.RolesDir(), filepath.Join(home, "roles")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// --- LogsDir ---

func TestLogsDirAbsoluteCustomPath(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	custom := t.TempDir()
	ws.Config = &Config{
		Logs: LogsConfig{Path: custom},
	}

	if got := ws.LogsDir(); got != custom {
		t.Errorf("LogsDir() = %q, want absolute custom %q", got, custom)
	}
}

func TestLogsDirRelativeCustomPath(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	// A relative path resolves under the global state dir (~/.mycel).
	ws.Config = &Config{
		Logs: LogsConfig{Path: "custom/logs"},
	}

	got := ws.LogsDir()
	want := filepath.Join(ws.StateDir(), "custom/logs")
	if got != want {
		t.Errorf("LogsDir() = %q, want %q", got, want)
	}
}

func TestLogsDirEmptyPath(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	// Config exists but Logs.Path is empty — should fall back to StateDir/logs
	ws.Config = &Config{
		Logs: LogsConfig{Path: ""},
	}

	got := ws.LogsDir()
	want := filepath.Join(ws.StateDir(), "logs")
	if got != want {
		t.Errorf("LogsDir() = %q, want %q", got, want)
	}
}

func TestLogsDirNilConfig(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	// No Config — should use StateDir/logs
	ws.Config = nil

	got := ws.LogsDir()
	want := filepath.Join(ws.StateDir(), "logs")
	if got != want {
		t.Errorf("LogsDir() = %q, want %q", got, want)
	}
}

// --- Global home structure ---

func TestOpenCreatesGlobalStructure(t *testing.T) {
	home := setTestHome(t)
	if _, err := Open(""); err != nil {
		t.Fatal(err)
	}

	for _, sub := range []string{"agents", "apps", "templates", "logs", "run"} {
		info, err := os.Stat(filepath.Join(home, sub))
		if err != nil {
			t.Errorf("directory %q not created: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", sub)
		}
	}
}

// --- Roles ---

func TestOpenSeedsDefaultRoles(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	// Check RoleManager is initialized with a store
	if ws.RoleManager == nil {
		t.Fatal("RoleManager is nil")
	}
	if ws.RoleManager.Store() == nil {
		t.Fatal("RoleManager.Store() is nil")
	}

	// Check default roles exist in the store
	if !ws.RoleManager.HasRole("root") {
		t.Error("root role not found in store")
	}
	if !ws.RoleManager.HasRole("base") {
		t.Error("base role not found in store")
	}
}

func TestLoadRestoresRoles(t *testing.T) {
	setTestHome(t)
	dir := newTestRepo(t)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}

	// Load it back
	ws, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if ws.Config == nil {
		t.Fatal("Config is nil after load")
	}
	if ws.Config.Version != 2 {
		t.Errorf("ConfigVersion = %d, want 2", ws.Config.Version)
	}
	if ws.RoleManager == nil {
		t.Fatal("RoleManager is nil after load")
	}

	// Check that root role was loaded
	role, ok := ws.RoleManager.GetRole("root")
	if !ok {
		t.Fatal("root role should be loaded")
	}
	if role.Metadata.Name != "root" {
		t.Errorf("root role name = %q, want %q", role.Metadata.Name, "root")
	}
}

func TestWorkspaceGetRole(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	// Get default root role
	role, err := ws.GetRole("root")
	if err != nil {
		t.Fatalf("GetRole(root): %v", err)
	}
	if role.Metadata.Name != "root" {
		t.Error("root role should have name 'root'")
	}

	// Get nonexistent role
	_, err = ws.GetRole("nonexistent")
	if err == nil {
		t.Error("GetRole should fail for nonexistent role")
	}
}

func TestWorkspaceGetRolePrompt(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	prompt := ws.GetRolePrompt("root")
	if prompt == "" {
		t.Error("GetRolePrompt(root) should not be empty")
	}

	// Nonexistent role returns empty
	prompt = ws.GetRolePrompt("nonexistent")
	if prompt != "" {
		t.Error("GetRolePrompt(nonexistent) should be empty")
	}
}

// --- Providers ---

func TestWorkspaceDefaultProvider(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	if ws.DefaultProvider() != "claude" {
		t.Errorf("DefaultProvider = %q, want %q", ws.DefaultProvider(), "claude")
	}

	cmd := ws.DefaultProviderCommand()
	if cmd != "claude --dangerously-skip-permissions" {
		t.Errorf("DefaultProviderCommand = %q, want %q", cmd, "claude --dangerously-skip-permissions")
	}
}

func TestWorkspaceDefaultProviderCustom(t *testing.T) {
	ws, _ := openTestWorkspace(t)

	// Set custom provider in config
	ws.Config.Providers.Default = "cursor"

	if ws.DefaultProvider() != "cursor" {
		t.Errorf("DefaultProvider custom = %q, want cursor", ws.DefaultProvider())
	}
}
