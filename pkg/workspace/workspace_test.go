package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// DefaultConfig tests are in config_test.go

// --- Init ---

func TestInit(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
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

	// State directory was created (in ~/.bc/workspaces/<id>/)
	stateDir := ws.StateDir()
	if _, statErr := os.Stat(stateDir); statErr != nil {
		t.Errorf("state directory not created: %v", statErr)
	}

	// preferences.json was written (M11c+)
	configPath := filepath.Join(stateDir, PreferencesFileName)
	if _, statErr := os.Stat(configPath); statErr != nil {
		t.Fatalf("preferences.json not written: %v", statErr)
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws1, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	if ws1.Name() != ws2.Name() {
		t.Errorf("second Init changed Name: %q vs %q", ws1.Name(), ws2.Name())
	}
}

// --- Load ---

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	if _, err := Init(dir); err != nil {
		t.Fatal(err)
	}

	ws, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config == nil {
		t.Error("Config should not be nil")
	}
}

func TestLoadNotAWorkspace(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load non-workspace: expected error, got nil")
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	bcDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bcDir, "settings.json"), []byte("{{bad"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load invalid TOML: expected error, got nil")
	}
}

func TestLoadUpdatesPathsIfMoved(t *testing.T) {
	// Init in one location, then copy state to a new location with .bc/ and Load
	orig := t.TempDir()
	gitInitDir(t, orig)
	origWS, err := Init(orig)
	if err != nil {
		t.Fatal(err)
	}

	moved := t.TempDir()
	gitInitDir(t, moved)
	// Copy settings from the global state dir to a legacy .bc/ in the moved location
	srcCfg := filepath.Join(origWS.StateDir(), PreferencesFileName)
	dstDir := filepath.Join(moved, ".bc")
	if mkdirErr := os.MkdirAll(dstDir, 0750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	data, err := os.ReadFile(srcCfg) //nolint:gosec // test file read
	if err != nil {
		t.Fatal(err)
	}
	// Write as settings.json to exercise the legacy fallback path (read-only, #3239).
	if writeErr := os.WriteFile(filepath.Join(dstDir, LegacySettingsFileName), data, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if mkErr := os.MkdirAll(filepath.Join(dstDir, "roles"), 0750); mkErr != nil {
		t.Fatal(mkErr)
	}

	ws, err := Load(moved)
	if err != nil {
		t.Fatal(err)
	}

	absDir, err := filepath.Abs(moved)
	if err != nil {
		t.Fatal(err)
	}
	if ws.RootDir != absDir {
		t.Errorf("RootDir = %q, want %q", ws.RootDir, absDir)
	}
	// After loading, workspace may have migrated to ~/.bc/workspaces/<id>/
	// StateDir should be a valid directory regardless of location
	if _, statErr := os.Stat(ws.StateDir()); statErr != nil {
		t.Errorf("StateDir %q does not exist: %v", ws.StateDir(), statErr)
	}
	_ = absDir
}

// --- Find (upward search) ---

func TestFindInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	if _, err := Init(dir); err != nil {
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
	parent := t.TempDir()
	gitInitDir(t, parent)
	if _, err := Init(parent); err != nil {
		t.Fatal(err)
	}

	// Create a child directory (no workspace of its own)
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

func TestFindNestedWorkspaces(t *testing.T) {
	// Outer workspace
	outer := t.TempDir()
	gitInitDir(t, outer)
	if _, err := Init(outer); err != nil {
		t.Fatal(err)
	}

	// Inner workspace inside outer
	inner := filepath.Join(outer, "projects", "sub")
	if err := os.MkdirAll(inner, 0750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, inner)
	if _, err := Init(inner); err != nil {
		t.Fatal(err)
	}

	// Find from inner should find the inner workspace, not outer
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

	// Find from a child of inner should still find inner
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

func TestFindNoWorkspace(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	_, err := Find(dir)
	if err == nil {
		t.Fatal("Find in non-workspace tree: expected error, got nil")
	}
}

// --- Save ---

func TestSave(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify config
	// ws name from directory

	if saveErr := ws.Save(); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	// Reload and verify
	ws2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Name is derived from directory, not config
	if ws2.Name() == "" {
		t.Error("Name should not be empty")
	}
}

// --- Path helpers ---

func TestPathHelpers(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	sd := ws.StateDir()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"StateDir", sd, sd},
		{"AgentsDir", ws.AgentsDir(), filepath.Join(sd, "agents")},
		{"LogsDir", ws.LogsDir(), filepath.Join(sd, "logs")},
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

func TestLogsDirV2CustomPath(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Set a custom logs path in Config
	ws.Config = &Config{
		Logs: LogsConfig{Path: "custom/logs"},
	}

	got := ws.LogsDir()
	want := filepath.Join(absDir, "custom/logs")
	if got != want {
		t.Errorf("LogsDir() = %q, want %q", got, want)
	}
}

func TestLogsDirV2EmptyPath(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Config exists but Logs.Path is empty — should fall back to StateDir/logs
	ws.Config = &Config{
		Logs: LogsConfig{Path: ""},
	}

	got := ws.LogsDir()
	want := filepath.Join(ws.StateDir(), "logs")
	if got != want {
		t.Errorf("LogsDir() = %q, want %q", got, want)
	}
	_ = absDir
}

func TestLogsDirNilConfig(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	// No Config — should use StateDir/logs
	ws.Config = nil

	got := ws.LogsDir()
	want := filepath.Join(ws.StateDir(), "logs")
	if got != want {
		t.Errorf("LogsDir() = %q, want %q", got, want)
	}
}

// --- EnsureDirs ---

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := ws.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	for _, d := range []string{ws.StateDir(), ws.AgentsDir(), ws.LogsDir()} {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %q not created: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", d)
		}
	}
}

func TestEnsureDirsV2(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	// Init creates a v2 workspace
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := ws.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs V2: %v", err)
	}

	// V2 creates additional dirs: roles, channels
	v2Dirs := []string{
		ws.RolesDir(),
		ws.ChannelsDir(),
	}
	for _, d := range v2Dirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("V2 directory %q not created: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", d)
		}
	}
}

func TestEnsureDirsIdempotent(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Call twice — should not error
	if err := ws.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := ws.EnsureDirs(); err != nil {
		t.Fatalf("second EnsureDirs: %v", err)
	}
}

// --- IsWorkspace ---

func TestIsWorkspace(t *testing.T) {
	tests := []struct {
		setup func(t *testing.T) string
		name  string
		want  bool
	}{
		{
			func(t *testing.T) string {
				dir := t.TempDir()
				gitInitDir(t, dir)
				if _, err := Init(dir); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			"initialized workspace",
			true,
		},
		{
			func(t *testing.T) string {
				return t.TempDir()
			},
			"empty directory",
			false,
		},
		{
			func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			"nonexistent directory",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			if got := IsWorkspace(dir); got != tt.want {
				t.Errorf("IsWorkspace = %v, want %v", got, tt.want)
			}
		})
	}
}

// =====================
// Registry tests
// =====================

// newTestRegistry creates a Registry backed by a temp file.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	gitInitDir(t, dir)
	return &Registry{
		path: filepath.Join(dir, "workspaces.json"),
	}
}

// --- Register ---

func TestRegistryRegister(t *testing.T) {
	r := newTestRegistry(t)

	r.Register("/projects/foo", "foo")

	if len(r.Workspaces) != 1 {
		t.Fatalf("Workspaces len = %d, want 1", len(r.Workspaces))
	}
	if r.Workspaces[0].Path != "/projects/foo" {
		t.Errorf("Path = %q, want %q", r.Workspaces[0].Path, "/projects/foo")
	}
	if r.Workspaces[0].Name != "foo" {
		t.Errorf("Name = %q, want %q", r.Workspaces[0].Name, "foo")
	}
	if r.Workspaces[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestRegistryRegisterUpdatesExisting(t *testing.T) {
	r := newTestRegistry(t)

	r.Register("/projects/foo", "foo")
	time.Sleep(time.Millisecond) // ensure time difference
	r.Register("/projects/foo", "foo-renamed")

	if len(r.Workspaces) != 1 {
		t.Fatalf("Workspaces len = %d, want 1 (should update, not duplicate)", len(r.Workspaces))
	}
	if r.Workspaces[0].Name != "foo-renamed" {
		t.Errorf("Name = %q, want %q", r.Workspaces[0].Name, "foo-renamed")
	}
}

func TestRegistryRegisterMultiple(t *testing.T) {
	r := newTestRegistry(t)

	r.Register("/a", "a")
	r.Register("/b", "b")
	r.Register("/c", "c")

	if len(r.Workspaces) != 3 {
		t.Fatalf("Workspaces len = %d, want 3", len(r.Workspaces))
	}
}

// --- Unregister ---

func TestRegistryUnregister(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("/a", "a")
	r.Register("/b", "b")

	r.Unregister("/a")

	if len(r.Workspaces) != 1 {
		t.Fatalf("Workspaces len = %d, want 1", len(r.Workspaces))
	}
	if r.Workspaces[0].Path != "/b" {
		t.Errorf("remaining Path = %q, want %q", r.Workspaces[0].Path, "/b")
	}
}

func TestRegistryUnregisterNotFound(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("/a", "a")

	// Should be a no-op, not panic
	r.Unregister("/nonexistent")

	if len(r.Workspaces) != 1 {
		t.Errorf("Workspaces len = %d, want 1", len(r.Workspaces))
	}
}

// --- Touch ---

func TestRegistryTouch(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("/a", "a")
	originalTime := r.Workspaces[0].LastAccessed

	time.Sleep(time.Millisecond)
	r.Touch("/a")

	if !r.Workspaces[0].LastAccessed.After(originalTime) {
		t.Error("Touch did not update LastAccessed")
	}
}

func TestRegistryTouchNotFound(t *testing.T) {
	r := newTestRegistry(t)

	// Should be a no-op, not panic
	r.Touch("/nonexistent")
}

// --- Find ---

func TestRegistryFind(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("/a", "a")
	r.Register("/b", "b")

	entry := r.Find("/b")
	if entry == nil {
		t.Fatal("Find: expected entry, got nil")
	}
	if entry.Name != "b" {
		t.Errorf("Name = %q, want %q", entry.Name, "b")
	}
}

func TestRegistryFindNotFound(t *testing.T) {
	r := newTestRegistry(t)

	if entry := r.Find("/nonexistent"); entry != nil {
		t.Errorf("Find nonexistent: expected nil, got %+v", entry)
	}
}

// --- List ---

func TestRegistryListEmpty(t *testing.T) {
	r := newTestRegistry(t)

	list := r.List()
	if len(list) != 0 {
		t.Errorf("List empty = %d, want 0", len(list))
	}
}

func TestRegistryList(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("/a", "a")
	r.Register("/b", "b")

	if len(r.List()) != 2 {
		t.Errorf("List = %d, want 2", len(r.List()))
	}
}

// --- Prune ---

func TestRegistryPrune(t *testing.T) {
	r := newTestRegistry(t)

	// Create a real workspace
	realDir := t.TempDir()
	gitInitDir(t, realDir)
	if _, err := Init(realDir); err != nil {
		t.Fatal(err)
	}
	absReal, err := filepath.Abs(realDir)
	if err != nil {
		t.Fatal(err)
	}

	// Register it plus a fake one
	r.Register(absReal, "real")
	r.Register("/nonexistent/fake/workspace", "fake")

	pruned := r.Prune()

	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}
	if len(r.Workspaces) != 1 {
		t.Fatalf("Workspaces len = %d, want 1", len(r.Workspaces))
	}
	if r.Workspaces[0].Path != absReal {
		t.Errorf("remaining Path = %q, want %q", r.Workspaces[0].Path, absReal)
	}
}

func TestRegistryPruneAllGone(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("/gone/a", "a")
	r.Register("/gone/b", "b")

	pruned := r.Prune()
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}
	if len(r.Workspaces) != 0 {
		t.Errorf("Workspaces len = %d, want 0", len(r.Workspaces))
	}
}

func TestRegistryPruneNothingToPrune(t *testing.T) {
	r := newTestRegistry(t)

	realDir := t.TempDir()
	gitInitDir(t, realDir)
	if _, err := Init(realDir); err != nil {
		t.Fatal(err)
	}
	absReal, err := filepath.Abs(realDir)
	if err != nil {
		t.Fatal(err)
	}
	r.Register(absReal, "real")

	pruned := r.Prune()
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

// --- Registry Save / Load round-trip ---

func TestRegistrySaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)
	path := filepath.Join(dir, "workspaces.json")

	r1 := &Registry{path: path}
	r1.Register("/projects/alpha", "alpha")
	r1.Register("/projects/beta", "beta")

	if err := r1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into a fresh registry
	r2 := &Registry{path: path}
	data, err := os.ReadFile(path) //nolint:gosec // test file read
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, r2); err != nil {
		t.Fatal(err)
	}

	if len(r2.Workspaces) != 2 {
		t.Fatalf("loaded Workspaces len = %d, want 2", len(r2.Workspaces))
	}

	// Verify entries
	names := map[string]bool{}
	for _, w := range r2.Workspaces {
		names[w.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("loaded names = %v, want alpha and beta", names)
	}
}

func TestRegistrySaveCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: path}
	r.Register("/test", "test")

	if err := r.Save(); err != nil {
		t.Fatalf("Save with nested dir: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// =====================
// V2 Workspace Tests
// =====================

func TestInitV2Format(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Check Config is set
	if ws.Config == nil {
		t.Fatal("Config is nil")
	}
	if ws.Config == nil {
		t.Error("Config is nil")
	}

	// Check preferences.json was created in the state directory (M11c+)
	settingsPath := filepath.Join(ws.StateDir(), PreferencesFileName)
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("preferences.json not created at %s: %v", settingsPath, err)
	}

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

func TestLoadV2Workspace(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	// Initialize v2 workspace
	_, err := Init(dir)
	if err != nil {
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
		t.Error("RoleManager is nil after load")
	}

	// Check that root role was loaded
	role, ok := ws.RoleManager.GetRole("root")
	if !ok {
		t.Error("root role should be loaded")
	}
	if role.Metadata.Name != "root" {
		t.Errorf("root role name = %q, want %q", role.Metadata.Name, "root")
	}
}

func TestWorkspaceV2Directories(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Check all v2 directories exist
	dirs := map[string]string{
		"RolesDir":    ws.RolesDir(),
		"ChannelsDir": ws.ChannelsDir(),
	}

	for name, path := range dirs {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s (%s) not created: %v", name, path, err)
		}
	}
}

func TestWorkspaceGetRole(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

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
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

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

func TestWorkspaceDefaultProvider(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	// v2 workspace - default provider is gemini (minimal root-only startup)
	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	if ws.DefaultProvider() != "claude" {
		t.Errorf("DefaultProvider = %q, want %q", ws.DefaultProvider(), "claude")
	}

	cmd := ws.DefaultProviderCommand()
	if cmd != "claude --dangerously-skip-permissions" {
		t.Errorf("DefaultProviderCommand = %q, want %q", cmd, "claude --dangerously-skip-permissions")
	}
}

func TestWorkspaceSaveV2(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify config
	// ws name from directory

	// Save
	if saveErr := ws.Save(); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	// Reload and verify
	ws2, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	if ws2.Name() == "" {
		t.Error("Name after reload should not be empty")
	}
}

func TestWorkspaceDefaultProviderCustom(t *testing.T) {
	dir := t.TempDir()
	gitInitDir(t, dir)

	ws, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Set custom provider in config
	ws.Config.Providers.Default = "cursor"

	if ws.DefaultProvider() != "cursor" {
		t.Errorf("DefaultProvider custom = %q, want cursor", ws.DefaultProvider())
	}
}

func TestCopyDefaultPrompts(t *testing.T) {
	// Create source directory with prompts
	rootDir := t.TempDir()
	gitInitDir(t, rootDir)
	sourceDir := filepath.Join(rootDir, "prompts")
	if err := os.MkdirAll(sourceDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create a test prompt file
	testPrompt := "This is a test prompt."
	if err := os.WriteFile(filepath.Join(sourceDir, "test.md"), []byte(testPrompt), 0600); err != nil {
		t.Fatal(err)
	}

	// Create state directory and prompts subdirectory
	stateDir := filepath.Join(rootDir, ".bc")
	destDir := filepath.Join(stateDir, "prompts")
	if err := os.MkdirAll(destDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Copy prompts
	if err := copyDefaultPrompts(rootDir, stateDir); err != nil {
		t.Fatalf("copyDefaultPrompts: %v", err)
	}

	// Verify prompt was copied
	destPath := filepath.Join(stateDir, "prompts", "test.md")
	data, err := os.ReadFile(destPath) //nolint:gosec // test file path
	if err != nil {
		t.Fatalf("copied file not found: %v", err)
	}
	if string(data) != testPrompt {
		t.Errorf("copied content = %q, want %q", string(data), testPrompt)
	}
}

func TestCopyDefaultPromptsNoSource(t *testing.T) {
	// When no prompts directory exists, should silently succeed
	rootDir := t.TempDir()
	gitInitDir(t, rootDir)
	stateDir := filepath.Join(rootDir, ".bc")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Should not error
	if err := copyDefaultPrompts(rootDir, stateDir); err != nil {
		t.Errorf("copyDefaultPrompts without source should not error: %v", err)
	}
}

func TestCopyDefaultPromptsExistingDest(t *testing.T) {
	// Create source directory with prompts
	rootDir := t.TempDir()
	gitInitDir(t, rootDir)
	sourceDir := filepath.Join(rootDir, "prompts")
	if err := os.MkdirAll(sourceDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "test.md"), []byte("source content"), 0600); err != nil {
		t.Fatal(err)
	}

	// Create state directory with existing prompts
	stateDir := filepath.Join(rootDir, ".bc")
	destDir := filepath.Join(stateDir, "prompts")
	if err := os.MkdirAll(destDir, 0750); err != nil {
		t.Fatal(err)
	}
	// Create existing file with different content
	if err := os.WriteFile(filepath.Join(destDir, "test.md"), []byte("existing content"), 0600); err != nil {
		t.Fatal(err)
	}

	// Copy should skip existing files (not overwrite)
	if err := copyDefaultPrompts(rootDir, stateDir); err != nil {
		t.Fatalf("copyDefaultPrompts: %v", err)
	}

	// Verify existing content was preserved
	data, err := os.ReadFile(filepath.Join(destDir, "test.md")) //nolint:gosec // test file path
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing content" {
		t.Errorf("existing file was overwritten, got %q", string(data))
	}
}
