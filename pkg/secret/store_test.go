package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T, passphrase string) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".mycel"), 0750); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStore_SetGetRoundTrip(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	if err := s.Set("API_KEY", "sk-secret-123", "My API key"); err != nil {
		t.Fatal(err)
	}

	val, err := s.GetValue("API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "sk-secret-123" {
		t.Errorf("value = %q, want %q", val, "sk-secret-123")
	}
}

func TestStore_SetOverwrite(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	if err := s.Set("KEY", "value-1", "desc"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("KEY", "value-2", ""); err != nil {
		t.Fatal(err)
	}

	val, err := s.GetValue("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value-2" {
		t.Errorf("value = %q, want %q after overwrite", val, "value-2")
	}

	// Description should be preserved when empty on update
	meta, err := s.GetMeta("KEY")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Description != "desc" {
		t.Errorf("description = %q, want %q (should be preserved)", meta.Description, "desc")
	}
}

func TestStore_GetValue_NotFound(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	_, err := s.GetValue("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestStore_List_NoValues(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	if err := s.Set("KEY_A", "secret-a", "First key"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("KEY_B", "secret-b", "Second key"); err != nil {
		t.Fatal(err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 secrets, got %d", len(list))
	}
	// Sorted by name
	if list[0].Name != "KEY_A" {
		t.Errorf("first secret name = %q, want %q", list[0].Name, "KEY_A")
	}
	if list[1].Name != "KEY_B" {
		t.Errorf("second secret name = %q, want %q", list[1].Name, "KEY_B")
	}
}

func TestStore_Delete(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	if err := s.Set("TEMP", "val", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("TEMP"); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetValue("TEMP")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	err := s.Delete("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent secret")
	}
}

func TestStore_ResolveEnv(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	if err := s.Set("GH_TOKEN", "ghp_abc123", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("API_KEY", "sk-xyz", ""); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"GITHUB_TOKEN": "${secret:GH_TOKEN}",
		"ANTHROPIC":    "${secret:API_KEY}",
		"PLAIN":        "no-secret-here",
	}

	resolved := s.ResolveEnv(env)
	if resolved["GITHUB_TOKEN"] != "ghp_abc123" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", resolved["GITHUB_TOKEN"], "ghp_abc123")
	}
	if resolved["ANTHROPIC"] != "sk-xyz" {
		t.Errorf("ANTHROPIC = %q, want %q", resolved["ANTHROPIC"], "sk-xyz")
	}
	if resolved["PLAIN"] != "no-secret-here" {
		t.Errorf("PLAIN = %q, want %q", resolved["PLAIN"], "no-secret-here")
	}
}

func TestStore_ResolveEnv_MissingSecret(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	env := map[string]string{
		"KEY": "${secret:MISSING}",
	}

	resolved := s.ResolveEnv(env)
	if resolved["KEY"] != "${secret:MISSING}" {
		t.Errorf("missing secret should be left as-is, got %q", resolved["KEY"])
	}
}

func TestStore_WrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".mycel"), 0750); err != nil {
		t.Fatal(err)
	}

	// Set with passphrase A
	s1, err := NewStore(dir, "passphrase-A")
	if err != nil {
		t.Fatal(err)
	}
	if setErr := s1.Set("SECRET", "my-value", ""); setErr != nil {
		t.Fatal(setErr)
	}
	_ = s1.Close()

	// Try to read with passphrase B
	s2, err := NewStore(dir, "passphrase-B")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()

	_, err = s2.GetValue("SECRET")
	if err == nil {
		t.Fatal("expected error when decrypting with wrong passphrase")
	}
}

func TestStore_GetMeta(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	if err := s.Set("META_KEY", "val", "A test secret"); err != nil {
		t.Fatal(err)
	}

	meta, err := s.GetMeta("META_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil {
		t.Fatal("expected meta, got nil")
	}
	if meta.Name != "META_KEY" {
		t.Errorf("name = %q, want %q", meta.Name, "META_KEY")
	}
	if meta.Description != "A test secret" {
		t.Errorf("description = %q, want %q", meta.Description, "A test secret")
	}
}

func TestStore_GetMeta_NotFound(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	meta, err := s.GetMeta("NONEXISTENT")
	if err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		t.Errorf("expected nil for nonexistent, got %+v", meta)
	}
}

func TestStore_SetEmptyName(t *testing.T) {
	s := setupTestStore(t, "test-pass")

	err := s.Set("", "value", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}
