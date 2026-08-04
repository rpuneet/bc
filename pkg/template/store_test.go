package template

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// --- single-layer (legacy) behavior ---

func TestSingleLayerCreateGetDelete(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	tmpl := Template{Name: "foo", Description: "first"}
	if err := s.Create(tmpl, "hello", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, prompt, err := s.Get("foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "foo" || got.Description != "first" {
		t.Errorf("unexpected template: %+v", got)
	}
	if prompt != "hello" {
		t.Errorf("prompt = %q", prompt)
	}

	if err := s.Delete("foo", ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := s.Get("foo"); err == nil {
		t.Fatal("expected not-found after Delete")
	}
}

func TestCreateDuplicateReturnsError(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	tmpl := Template{Name: "dup"}
	if err := s.Create(tmpl, "", ""); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := s.Create(tmpl, "", ""); err == nil {
		t.Fatal("expected duplicate error")
	}
}

// --- layered store: workspace overrides global ---

func TestLayeredOverrideWins(t *testing.T) {
	globalDir := t.TempDir()
	wsDir := t.TempDir()

	s := NewLayeredStore(globalDir, wsDir)

	// Global version
	if err := s.Create(Template{Name: "shared", Description: "global version"}, "global prompt", ScopeGlobal); err != nil {
		t.Fatalf("create global: %v", err)
	}
	// Workspace override
	if err := s.Create(Template{Name: "shared", Description: "workspace version"}, "h prompt", ScopeWorkspace); err != nil {
		t.Fatalf("create workspace override: %v", err)
	}

	got, prompt, err := s.Get("shared")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "workspace version" {
		t.Errorf("override did not win: %q", got.Description)
	}
	if got.Scope != ScopeWorkspace {
		t.Errorf("scope = %q, want workspace", got.Scope)
	}
	if prompt != "h prompt" {
		t.Errorf("prompt = %q", prompt)
	}
}

func TestLayeredFallbackToGlobal(t *testing.T) {
	globalDir := t.TempDir()
	wsDir := t.TempDir()
	s := NewLayeredStore(globalDir, wsDir)

	if err := s.Create(Template{Name: "only-global"}, "", ScopeGlobal); err != nil {
		t.Fatalf("Create global: %v", err)
	}

	got, _, err := s.Get("only-global")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Scope != ScopeGlobal {
		t.Errorf("scope = %q, want global", got.Scope)
	}
}

func TestLayeredListUnion(t *testing.T) {
	globalDir := t.TempDir()
	wsDir := t.TempDir()
	s := NewLayeredStore(globalDir, wsDir)

	if err := s.Create(Template{Name: "alpha"}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Template{Name: "beta"}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	// Override beta in workspace; add gamma workspace-only.
	if err := s.Create(Template{Name: "beta", Description: "h"}, "", ScopeWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Template{Name: "gamma"}, "", ScopeWorkspace); err != nil {
		t.Fatal(err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d templates, want 3: %+v", len(list), list)
	}

	byName := map[string]Template{}
	for _, t := range list {
		byName[t.Name] = t
	}
	if byName["alpha"].Scope != ScopeGlobal {
		t.Errorf("alpha scope = %q", byName["alpha"].Scope)
	}
	if byName["beta"].Scope != ScopeWorkspace || byName["beta"].Description != "h" {
		t.Errorf("beta override lost: %+v", byName["beta"])
	}
	if byName["gamma"].Scope != ScopeWorkspace {
		t.Errorf("gamma scope = %q", byName["gamma"].Scope)
	}
}

func TestLayeredDeleteWorkspaceOnlyWhenGlobalExists(t *testing.T) {
	globalDir := t.TempDir()
	wsDir := t.TempDir()
	s := NewLayeredStore(globalDir, wsDir)

	if err := s.Create(Template{Name: "user-tmpl"}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}

	// Delete with workspace scope should fail with ErrWrongScope
	err := s.Delete("user-tmpl", ScopeWorkspace)
	if err == nil {
		t.Fatal("expected error deleting global via workspace scope")
	}
	if !errors.Is(err, ErrWrongScope) {
		t.Errorf("got %v, want ErrWrongScope", err)
	}

	// Global deletion should succeed
	if err := s.Delete("user-tmpl", ScopeGlobal); err != nil {
		t.Fatalf("Delete global: %v", err)
	}
}

func TestLayeredDeleteDefaultPrefersWorkspace(t *testing.T) {
	globalDir := t.TempDir()
	wsDir := t.TempDir()
	s := NewLayeredStore(globalDir, wsDir)

	if err := s.Create(Template{Name: "both"}, "", ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(Template{Name: "both", Description: "h"}, "", ScopeWorkspace); err != nil {
		t.Fatal(err)
	}

	// Default scope deletes workspace first.
	if err := s.Delete("both", ""); err != nil {
		t.Fatalf("Delete (default): %v", err)
	}

	// Workspace file gone
	if _, err := os.Stat(filepath.Join(wsDir, "both.json")); !os.IsNotExist(err) {
		t.Errorf("workspace override still present: %v", err)
	}
	// Global still there
	got, _, err := s.Get("both")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got.Scope != ScopeGlobal {
		t.Errorf("got scope %q after workspace delete, want global", got.Scope)
	}
}

// --- seed defaults ---

func TestSeedDefaultsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := SeedDefaults(dir); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	s := NewStore(dir)
	list1, listErr := s.List()
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(list1) < 1 {
		t.Fatalf("seed produced %d defaults", len(list1))
	}

	// Second seed should be a no-op.
	if seedErr := SeedDefaults(dir); seedErr != nil {
		t.Fatalf("second SeedDefaults: %v", seedErr)
	}
	list2, listErr := s.List()
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(list1) != len(list2) {
		t.Errorf("list size changed after second seed: %d → %d", len(list1), len(list2))
	}
}

func TestSeedDefaultsWritesNoScopeOnDisk(t *testing.T) {
	dir := t.TempDir()
	if err := SeedDefaults(dir); err != nil {
		t.Fatal(err)
	}
	// Read raw JSON and ensure "scope" is not present on disk.
	data, err := os.ReadFile(filepath.Join(dir, "blank.json")) //nolint:gosec // test path under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); containsScope(got) {
		t.Errorf("disk file contains scope field:\n%s", got)
	}
}

func containsScope(raw string) bool {
	return len(raw) > 0 && (indexOfScope(raw) >= 0)
}

func indexOfScope(s string) int {
	for i := 0; i+len(`"scope"`) <= len(s); i++ {
		if s[i:i+len(`"scope"`)] == `"scope"` {
			return i
		}
	}
	return -1
}

// --- validation ---

func TestInvalidNameRejected(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	bad := []string{"", "..", ".", "../etc", "a/b", `a\b`}
	for _, n := range bad {
		if err := s.Create(Template{Name: n}, "", ""); err == nil {
			t.Errorf("expected error for name %q", n)
		}
	}
}
