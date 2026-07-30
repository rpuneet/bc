package secret

import (
	"os"
	"path/filepath"
	"testing"
)

// mkGlobalVault returns a freshly-initialized user-global vault Store.
func mkGlobalVault(t *testing.T, passphrase string) *Store {
	t.Helper()
	dir := t.TempDir()
	vaultPath := filepath.Join(dir, "secrets.vault")
	s, err := OpenVaultFile(vaultPath, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// mkRepoStore returns a workspace-scoped secret Store (legacy path).
func mkRepoStore(t *testing.T, passphrase string) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".bc"), 0750); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestLayeredReadPrefersWorkspace(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")

	if err := g.Set("API_KEY", "global-val", "global"); err != nil {
		t.Fatal(err)
	}
	if err := w.Set("API_KEY", "h-val", "workspace"); err != nil {
		t.Fatal(err)
	}

	l := NewLayeredStore(g, w)
	got, err := l.GetValue("API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "h-val" {
		t.Errorf("value = %q, want h-val", got)
	}

	meta, err := l.GetMeta("API_KEY")
	if err != nil || meta == nil {
		t.Fatalf("meta: %v", err)
	}
	if meta.Scope != ScopeWorkspace {
		t.Errorf("scope = %q", meta.Scope)
	}
}

func TestLayeredFallbackToGlobal(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")
	if err := g.Set("GLOBAL_ONLY", "gv", "g"); err != nil {
		t.Fatal(err)
	}
	l := NewLayeredStore(g, w)

	got, err := l.GetValue("GLOBAL_ONLY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gv" {
		t.Errorf("value %q", got)
	}
	meta, err := l.GetMeta("GLOBAL_ONLY")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Scope != ScopeGlobal {
		t.Errorf("scope = %q", meta.Scope)
	}
}

func TestLayeredSetDefaultsGlobal(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")
	l := NewLayeredStore(g, w)

	if err := l.Set("NEW", "v", "d"); err != nil {
		t.Fatal(err)
	}
	// Value must land in global, not workspace.
	if _, err := g.GetValue("NEW"); err != nil {
		t.Errorf("global missing: %v", err)
	}
	if _, err := w.GetValue("NEW"); err == nil {
		t.Errorf("value leaked into workspace scope")
	}
}

func TestLayeredSetWorkspace(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")
	l := NewLayeredStore(g, w)

	if err := l.SetWorkspace("WS_ONLY", "wv", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := g.GetValue("WS_ONLY"); err == nil {
		t.Errorf("value leaked into global scope")
	}
	if _, err := w.GetValue("WS_ONLY"); err != nil {
		t.Errorf("workspace missing: %v", err)
	}
}

func TestLayeredListReportsScopes(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")

	_ = g.Set("A", "a", "")
	_ = g.Set("B", "b-global", "")
	_ = w.Set("B", "b-h", "")
	_ = w.Set("C", "c", "")

	l := NewLayeredStore(g, w)
	list, err := l.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}

	byName := map[string]Scope{}
	for _, m := range list {
		byName[m.Name] = m.Scope
	}
	if byName["A"] != ScopeGlobal {
		t.Errorf("A scope %q", byName["A"])
	}
	if byName["B"] != ScopeWorkspace {
		t.Errorf("B scope %q (override should win)", byName["B"])
	}
	if byName["C"] != ScopeWorkspace {
		t.Errorf("C scope %q", byName["C"])
	}
}

func TestLayeredDeleteScoped(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")
	if err := g.Set("K", "gv", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.Set("K", "wv", ""); err != nil {
		t.Fatal(err)
	}
	l := NewLayeredStore(g, w)

	// Delete just the workspace override.
	if err := l.DeleteScoped(ScopeWorkspace, "K"); err != nil {
		t.Fatal(err)
	}
	got, err := l.GetValue("K")
	if err != nil {
		t.Fatal(err)
	}
	if got != "gv" {
		t.Errorf("value = %q after workspace delete", got)
	}

	// Now delete global.
	if err := l.DeleteScoped(ScopeGlobal, "K"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.GetValue("K"); err == nil {
		t.Error("expected error after deleting both scopes")
	}
}

func TestLayeredResolveEnv(t *testing.T) {
	g := mkGlobalVault(t, "pass")
	w := mkRepoStore(t, "pass")
	_ = g.Set("API_KEY", "sk-global", "")
	_ = w.Set("API_KEY", "sk-h", "")
	_ = g.Set("MODE", "prod", "")

	l := NewLayeredStore(g, w)
	out := l.ResolveEnv(map[string]string{
		"AUTH":  "${secret:API_KEY}",
		"MODE":  "${secret:MODE}",
		"PLAIN": "no-secret",
	})
	if out["AUTH"] != "sk-h" {
		t.Errorf("AUTH = %q (workspace should win)", out["AUTH"])
	}
	if out["MODE"] != "prod" {
		t.Errorf("MODE = %q", out["MODE"])
	}
	if out["PLAIN"] != "no-secret" {
		t.Errorf("PLAIN = %q", out["PLAIN"])
	}
}

func TestOpenVaultFileIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.vault")
	s1, err := OpenVaultFile(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if setErr := s1.Set("A", "v", ""); setErr != nil {
		t.Fatal(setErr)
	}
	_ = s1.Close()

	// Reopen — same passphrase should recover the salt + decrypt.
	s2, err := OpenVaultFile(path, "pass")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()

	got, err := s2.GetValue("A")
	if err != nil {
		t.Fatal(err)
	}
	if got != "v" {
		t.Errorf("got %q, want v", got)
	}
}
