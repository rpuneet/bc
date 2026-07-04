package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRegistryAlias tests the alias functionality (#1218)
func TestRegistryAlias(t *testing.T) {
	// Create temp directory for test registry
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: registryPath}

	// Register workspace without alias
	err := r.RegisterWithAlias("/projects/frontend", "frontend", "")
	if err != nil {
		t.Fatalf("RegisterWithAlias: %v", err)
	}

	// Register workspace with alias
	err = r.RegisterWithAlias("/projects/backend", "backend", "be")
	if err != nil {
		t.Fatalf("RegisterWithAlias with alias: %v", err)
	}

	// FindByAlias should work
	entry := r.FindByAlias("be")
	if entry == nil {
		t.Fatal("FindByAlias: expected entry, got nil")
	}
	if entry.Path != "/projects/backend" {
		t.Errorf("FindByAlias Path = %q, want %q", entry.Path, "/projects/backend")
	}

	// FindByAlias for non-existent alias should return nil
	entry = r.FindByAlias("nonexistent")
	if entry != nil {
		t.Errorf("FindByAlias for nonexistent: expected nil, got %v", entry)
	}

	// SetAlias should work
	err = r.SetAlias("/projects/frontend", "fe")
	if err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	entry = r.FindByAlias("fe")
	if entry == nil || entry.Path != "/projects/frontend" {
		t.Error("SetAlias: alias not set correctly")
	}

	// SetAlias with conflicting alias should error
	err = r.SetAlias("/projects/frontend", "be")
	if err == nil {
		t.Error("SetAlias with conflicting alias: expected error, got nil")
	}
	if _, ok := err.(*AliasConflictError); !ok {
		t.Errorf("SetAlias with conflicting alias: expected AliasConflictError, got %T", err)
	}
}

// TestRegistryActiveWorkspace tests the active workspace functionality (#1218)
func TestRegistryActiveWorkspace(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: registryPath}

	// Register workspaces
	_ = r.RegisterWithAlias("/projects/frontend", "frontend", "fe")
	_ = r.RegisterWithAlias("/projects/backend", "backend", "be")

	// GetActive should return nil initially
	if active := r.GetActive(); active != nil {
		t.Errorf("GetActive initially: expected nil, got %v", active)
	}

	// SetActive by alias
	err := r.SetActive("fe")
	if err != nil {
		t.Fatalf("SetActive by alias: %v", err)
	}
	active := r.GetActive()
	if active == nil || active.Path != "/projects/frontend" {
		t.Error("SetActive by alias: active workspace not set correctly")
	}
	// Active should be stored as alias
	if r.Active != "fe" {
		t.Errorf("Active stored = %q, want %q", r.Active, "fe")
	}

	// SetActive by path
	err = r.SetActive("/projects/backend")
	if err != nil {
		t.Fatalf("SetActive by path: %v", err)
	}
	active = r.GetActive()
	if active == nil || active.Path != "/projects/backend" {
		t.Error("SetActive by path: active workspace not set correctly")
	}

	// SetActive for non-existent workspace should error
	err = r.SetActive("nonexistent")
	if err == nil {
		t.Error("SetActive for nonexistent: expected error, got nil")
	}

	// SetActive with empty clears active
	err = r.SetActive("")
	if err != nil {
		t.Fatalf("SetActive empty: %v", err)
	}
	if r.GetActive() != nil {
		t.Error("SetActive empty: expected nil active")
	}
}

// TestRegistryFindByNameOrAlias tests the combined lookup (#1218)
func TestRegistryFindByNameOrAlias(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: registryPath}
	_ = r.RegisterWithAlias("/projects/frontend", "frontend", "fe")

	// Find by alias
	entry := r.FindByNameOrAlias("fe")
	if entry == nil || entry.Path != "/projects/frontend" {
		t.Error("FindByNameOrAlias by alias: not found")
	}

	// Find by name
	entry = r.FindByNameOrAlias("frontend")
	if entry == nil || entry.Path != "/projects/frontend" {
		t.Error("FindByNameOrAlias by name: not found")
	}

	// Find by path
	entry = r.FindByNameOrAlias("/projects/frontend")
	if entry == nil || entry.Path != "/projects/frontend" {
		t.Error("FindByNameOrAlias by path: not found")
	}

	// Not found
	entry = r.FindByNameOrAlias("nonexistent")
	if entry != nil {
		t.Error("FindByNameOrAlias nonexistent: expected nil")
	}
}

// TestRegistrySaveLoad tests persistence (#1218)
func TestRegistrySaveLoad(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	// Create and save registry
	r := &Registry{path: registryPath}
	_ = r.RegisterWithAlias("/projects/frontend", "frontend", "fe")
	_ = r.SetActive("fe")

	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("Registry file not created: %v", err)
	}

	// Load and verify
	// Note: LoadRegistry uses GlobalDir(), so we test Save/Load manually
	r2 := &Registry{path: registryPath}
	data, err := os.ReadFile(registryPath) //nolint:gosec // test file path
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := json.Unmarshal(data, r2); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(r2.Workspaces) != 1 {
		t.Errorf("Loaded workspaces count = %d, want 1", len(r2.Workspaces))
	}
	if r2.Workspaces[0].Alias != "fe" {
		t.Errorf("Loaded alias = %q, want %q", r2.Workspaces[0].Alias, "fe")
	}
	if r2.Active != "fe" {
		t.Errorf("Loaded active = %q, want %q", r2.Active, "fe")
	}
}

func TestGlobalDir(t *testing.T) {
	dir := GlobalDir()
	if dir == "" {
		t.Skip("no home directory available")
	}

	// Should end with .mycel (unless MYCEL_HOME points elsewhere).
	if os.Getenv("MYCEL_HOME") == "" && filepath.Base(dir) != ".mycel" {
		t.Errorf("GlobalDir should end with .mycel, got %s", dir)
	}
}

func TestRegistryPath(t *testing.T) {
	path := RegistryPath()
	if path == "" {
		t.Skip("no home directory available")
	}

	// Should end with workspaces.json
	if filepath.Base(path) != "workspaces.json" {
		t.Errorf("RegistryPath should end with workspaces.json, got %s", path)
	}
}

func TestLoadRegistryNotFound(t *testing.T) {
	// Use temp HOME with no registry file
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", "") // fall back to $HOME/.mycel despite TestMain sandbox
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	r, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry should not error for missing file: %v", err)
	}
	if r == nil {
		t.Fatal("LoadRegistry should return empty registry")
	}
	if len(r.Workspaces) != 0 {
		t.Errorf("LoadRegistry should return empty workspaces, got %d", len(r.Workspaces))
	}
}

func TestLoadRegistryWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", "") // fall back to $HOME/.mycel despite TestMain sandbox
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Create .mycel directory and registry file
	bcDir := filepath.Join(tmpDir, ".mycel")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create .mycel dir: %v", err)
	}

	registryData := `{
		"active": "test",
		"workspaces": [
			{"path": "/test/path", "name": "test", "alias": ""}
		]
	}`
	registryPath := filepath.Join(bcDir, "workspaces.json")
	if err := os.WriteFile(registryPath, []byte(registryData), 0600); err != nil {
		t.Fatalf("failed to write registry: %v", err)
	}

	r, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	if r == nil {
		t.Fatal("LoadRegistry returned nil")
	}
	if len(r.Workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(r.Workspaces))
	}
	if r.Active != "test" {
		t.Errorf("expected active 'test', got %q", r.Active)
	}
}

func TestLoadRegistryInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", "") // fall back to $HOME/.mycel despite TestMain sandbox
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Create .mycel directory with invalid JSON
	bcDir := filepath.Join(tmpDir, ".mycel")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create .mycel dir: %v", err)
	}

	registryPath := filepath.Join(bcDir, "workspaces.json")
	if err := os.WriteFile(registryPath, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("failed to write registry: %v", err)
	}

	_, err := LoadRegistry()
	if err == nil {
		t.Error("LoadRegistry should error on invalid JSON")
	}
}

func TestSetAliasWorkspaceNotFound(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: registryPath}

	err := r.SetAlias("/nonexistent", "alias")
	if err == nil {
		t.Error("SetAlias should error for nonexistent workspace")
	}
	if _, ok := err.(*WorkspaceNotFoundError); !ok {
		t.Errorf("expected WorkspaceNotFoundError, got %T", err)
	}
}

func TestRegisterWithAliasConflict(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: registryPath}

	// Register first with alias
	err := r.RegisterWithAlias("/path1", "ws1", "myalias")
	if err != nil {
		t.Fatalf("first RegisterWithAlias failed: %v", err)
	}

	// Try to register second with same alias
	err = r.RegisterWithAlias("/path2", "ws2", "myalias")
	if err == nil {
		t.Error("RegisterWithAlias should error on alias conflict")
	}
	if _, ok := err.(*AliasConflictError); !ok {
		t.Errorf("expected AliasConflictError, got %T", err)
	}
}

func TestRegisterWithAliasUpdate(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "workspaces.json")

	r := &Registry{path: registryPath}

	// Register workspace
	err := r.RegisterWithAlias("/path1", "ws1", "")
	if err != nil {
		t.Fatalf("RegisterWithAlias failed: %v", err)
	}

	// Update same path with alias
	err = r.RegisterWithAlias("/path1", "ws1-updated", "newalias")
	if err != nil {
		t.Fatalf("RegisterWithAlias update failed: %v", err)
	}

	if len(r.Workspaces) != 1 {
		t.Errorf("expected 1 workspace after update, got %d", len(r.Workspaces))
	}
	if r.Workspaces[0].Name != "ws1-updated" {
		t.Errorf("expected updated name, got %q", r.Workspaces[0].Name)
	}
	if r.Workspaces[0].Alias != "newalias" {
		t.Errorf("expected alias 'newalias', got %q", r.Workspaces[0].Alias)
	}
}

func TestAliasConflictErrorMessage(t *testing.T) {
	err := &AliasConflictError{Alias: "test", ExistingPath: "/existing"}
	msg := err.Error()
	if msg == "" {
		t.Error("AliasConflictError.Error() should return message")
	}
}

func TestWorkspaceNotFoundErrorMessage(t *testing.T) {
	err := &WorkspaceNotFoundError{Identifier: "test"}
	msg := err.Error()
	if msg == "" {
		t.Error("WorkspaceNotFoundError.Error() should return message")
	}
}

// TestComputeWorkspaceID verifies that IDs are stable, deterministic, and the
// expected length.
func TestComputeWorkspaceID(t *testing.T) {
	if got := ComputeWorkspaceID(""); got != "" {
		t.Errorf("ComputeWorkspaceID(\"\") = %q, want empty", got)
	}
	id1 := ComputeWorkspaceID("/Users/test/project")
	id2 := ComputeWorkspaceID("/Users/test/project")
	if id1 != id2 {
		t.Errorf("ComputeWorkspaceID not deterministic: %q vs %q", id1, id2)
	}
	if len(id1) != registryIDLength {
		t.Errorf("ID length = %d, want %d", len(id1), registryIDLength)
	}
	id3 := ComputeWorkspaceID("/Users/test/other")
	if id1 == id3 {
		t.Errorf("different paths produced same ID: %q", id1)
	}
}

// TestRegistryRegisterPopulatesID ensures Register sets the ID field.
func TestRegistryRegisterPopulatesID(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{path: filepath.Join(dir, "workspaces.json")}
	if err := r.RegisterWithAlias("/projects/foo", "foo", ""); err != nil {
		t.Fatalf("RegisterWithAlias: %v", err)
	}
	if r.Workspaces[0].ID == "" {
		t.Fatal("Register did not populate ID")
	}
	if r.Workspaces[0].ID != ComputeWorkspaceID("/projects/foo") {
		t.Errorf("ID mismatch: got %q want %q", r.Workspaces[0].ID, ComputeWorkspaceID("/projects/foo"))
	}
	if r.Workspaces[0].LastUsedAt.IsZero() {
		t.Error("LastUsedAt should be populated on Register")
	}
}

// TestRegistryFindByID verifies the new ID-based lookup.
func TestRegistryFindByID(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{path: filepath.Join(dir, "workspaces.json")}
	_ = r.RegisterWithAlias("/projects/foo", "foo", "f")

	id := ComputeWorkspaceID("/projects/foo")
	entry := r.FindByID(id)
	if entry == nil {
		t.Fatalf("FindByID(%q) returned nil", id)
	}
	if entry.Path != "/projects/foo" {
		t.Errorf("FindByID path = %q", entry.Path)
	}

	if r.FindByID("") != nil {
		t.Error("FindByID(\"\") should return nil")
	}
	if r.FindByID("deadbeef1234") != nil {
		t.Error("FindByID for unknown id should return nil")
	}

	// Resolve should also accept the ID.
	if got := r.Resolve(id); got == nil || got.Path != "/projects/foo" {
		t.Error("Resolve by ID failed")
	}
}

// TestRegistryEntryGetDataDir verifies the DataDir accessor used by M11+.
// Empty DataDir falls back to computing the path from the ID.
func TestRegistryEntryGetDataDir(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)

	t.Run("explicit-field", func(t *testing.T) {
		e := &RegistryEntry{ID: "abc123", DataDir: "/explicit/override"}
		if got := e.GetDataDir(); got != "/explicit/override" {
			t.Errorf("explicit DataDir returned %q", got)
		}
	})

	t.Run("fallback-to-id", func(t *testing.T) {
		e := &RegistryEntry{ID: "abc123456789"}
		want := filepath.Join(bcHome, "workspaces", "abc123456789")
		if got := e.GetDataDir(); got != want {
			t.Errorf("ID-based fallback: got %q, want %q", got, want)
		}
	})

	t.Run("fallback-to-path", func(t *testing.T) {
		path := "/some/project"
		e := &RegistryEntry{Path: path}
		wantID := ComputeWorkspaceID(path)
		want := filepath.Join(bcHome, "workspaces", wantID)
		if got := e.GetDataDir(); got != want {
			t.Errorf("path-based fallback: got %q, want %q", got, want)
		}
	})

	t.Run("nil-entry", func(t *testing.T) {
		var e *RegistryEntry
		if got := e.GetDataDir(); got != "" {
			t.Errorf("nil entry: got %q, want empty", got)
		}
	})

	t.Run("no-id-no-path", func(t *testing.T) {
		e := &RegistryEntry{}
		if got := e.GetDataDir(); got != "" {
			t.Errorf("empty entry: got %q, want empty", got)
		}
	})
}

// TestRegisterPopulatesDataDir ensures new registrations carry the DataDir
// field so downstream code can look it up without re-deriving.
func TestRegisterPopulatesDataDir(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)

	dir := t.TempDir()
	r := &Registry{path: filepath.Join(dir, "workspaces.json")}
	if err := r.RegisterWithAlias("/projects/foo", "foo", ""); err != nil {
		t.Fatalf("RegisterWithAlias: %v", err)
	}

	entry := r.Workspaces[0]
	if entry.DataDir == "" {
		t.Fatal("Register did not populate DataDir")
	}
	want := filepath.Join(bcHome, "workspaces", entry.ID)
	if entry.DataDir != want {
		t.Errorf("DataDir = %q, want %q", entry.DataDir, want)
	}
}

// TestRegistryAtomicSave verifies Save writes atomically via tmp+rename.
// We can't easily simulate a kill, but we can check that a tmp file is not
// left behind on success.
func TestRegistryAtomicSave(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{path: filepath.Join(dir, "workspaces.json")}
	_ = r.RegisterWithAlias("/projects/x", "x", "")

	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// No leftover .tmp files
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}

	// File is readable and parses.
	data, err := os.ReadFile(r.path) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r2 := &Registry{path: r.path}
	if err := json.Unmarshal(data, r2); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if r2.Version != CurrentRegistryVersion {
		t.Errorf("serialized Version = %d, want %d", r2.Version, CurrentRegistryVersion)
	}
	if len(r2.Workspaces) != 1 || r2.Workspaces[0].ID == "" {
		t.Error("Saved registry missing ID")
	}
}
