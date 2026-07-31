package mcp

import (
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/db"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	// Set up shared SQLite DB (required after fallback removal)
	dir := t.TempDir()
	d, err := db.Open(dir + "/mycel.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	s, err := NewStore(d, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAddAndGet(t *testing.T) {
	s := setupTestStore(t)

	cfg := &ServerConfig{
		Name:      "github",
		Transport: TransportStdio,
		Command:   "npx",
		Args:      []string{"@modelcontextprotocol/server-github"},
		Env:       map[string]string{"GITHUB_TOKEN": "${secret:GH_TOKEN}"},
		Enabled:   true,
	}
	if err := s.Add(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("github")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.Name != "github" {
		t.Errorf("name = %q, want %q", got.Name, "github")
	}
	if got.Transport != TransportStdio {
		t.Errorf("transport = %q, want %q", got.Transport, TransportStdio)
	}
	if got.Command != "npx" {
		t.Errorf("command = %q, want %q", got.Command, "npx")
	}
	if len(got.Args) != 1 || got.Args[0] != "@modelcontextprotocol/server-github" {
		t.Errorf("args = %v, want [\"@modelcontextprotocol/server-github\"]", got.Args)
	}
	if got.Env["GITHUB_TOKEN"] != "${secret:GH_TOKEN}" {
		t.Errorf("env GITHUB_TOKEN = %q, want %q", got.Env["GITHUB_TOKEN"], "${secret:GH_TOKEN}")
	}
	if !got.Enabled {
		t.Error("expected enabled = true")
	}
}

func TestGetNotFound(t *testing.T) {
	s := setupTestStore(t)

	got, err := s.Get("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestList(t *testing.T) {
	s := setupTestStore(t)

	// Empty list
	configs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 0 {
		t.Errorf("expected 0 configs, got %d", len(configs))
	}

	// Add two configs
	if addErr := s.Add(&ServerConfig{Name: "beta", Transport: TransportStdio, Command: "cmd-b"}); addErr != nil {
		t.Fatal(addErr)
	}
	if addErr := s.Add(&ServerConfig{Name: "alpha", Transport: TransportSSE, URL: "https://example.com"}); addErr != nil {
		t.Fatal(addErr)
	}

	configs, err = s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
	// Sorted by name
	if configs[0].Name != "alpha" {
		t.Errorf("first config name = %q, want %q", configs[0].Name, "alpha")
	}
	if configs[1].Name != "beta" {
		t.Errorf("second config name = %q, want %q", configs[1].Name, "beta")
	}
}

func TestRemove(t *testing.T) {
	s := setupTestStore(t)

	if err := s.Add(&ServerConfig{Name: "test", Transport: TransportStdio, Command: "cmd"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("test"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil after remove")
	}
}

func TestRemoveNotFound(t *testing.T) {
	s := setupTestStore(t)

	err := s.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error for removing nonexistent server")
	}
}

func TestSetEnabled(t *testing.T) {
	s := setupTestStore(t)

	if err := s.Add(&ServerConfig{Name: "srv", Transport: TransportStdio, Command: "cmd", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetEnabled("srv", false); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("srv")
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Error("expected enabled = false after disable")
	}

	if enableErr := s.SetEnabled("srv", true); enableErr != nil {
		t.Fatal(enableErr)
	}
	got, err = s.Get("srv")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled {
		t.Error("expected enabled = true after enable")
	}
}

func TestSetEnabledNotFound(t *testing.T) {
	s := setupTestStore(t)

	// SetEnabled on a server not in the database and no config lookup
	// should return an error.
	err := s.SetEnabled("config-only", false)
	if err == nil {
		t.Fatal("expected error for SetEnabled on nonexistent server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	// Verify no row was created in the database.
	got, getErr := s.Get("config-only")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got != nil {
		t.Error("expected nil, got a row — SetEnabled should not create rows")
	}
}

func TestSetEnabledConfigOnly(t *testing.T) {
	s := setupTestStore(t)

	// Register a config lookup that returns a server config for "github".
	s.SetConfigLookup(func(name string) *ServerConfig {
		if name == "github" {
			return &ServerConfig{
				Name:      "github",
				Transport: TransportStdio,
				Command:   "github-mcp-server",
				Env:       map[string]string{"GITHUB_TOKEN": "tok"},
				Enabled:   true,
			}
		}
		return nil
	})

	// Disabling a config-only server should auto-insert it with enabled=false.
	if err := s.SetEnabled("github", false); err != nil {
		t.Fatalf("SetEnabled config-only server: %v", err)
	}

	got, err := s.Get("github")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected row after SetEnabled on config-only server")
	}
	if got.Enabled {
		t.Error("expected enabled=false after disabling config-only server")
	}
	if got.Command != "github-mcp-server" {
		t.Errorf("command = %q, want %q", got.Command, "github-mcp-server")
	}
	if got.Transport != TransportStdio {
		t.Errorf("transport = %q, want %q", got.Transport, TransportStdio)
	}

	// Toggling back to enabled should work via normal UPDATE path.
	if err := s.SetEnabled("github", true); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	got, _ = s.Get("github")
	if !got.Enabled {
		t.Error("expected enabled=true after re-enabling")
	}
}

func TestSetEnabledConfigLookupMiss(t *testing.T) {
	s := setupTestStore(t)

	// Config lookup that never matches.
	s.SetConfigLookup(func(name string) *ServerConfig { return nil })

	err := s.SetEnabled("nonexistent", false)
	if err == nil {
		t.Fatal("expected error when config lookup returns nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestAddDuplicate(t *testing.T) {
	s := setupTestStore(t)

	cfg := &ServerConfig{Name: "dup", Transport: TransportStdio, Command: "cmd"}
	if err := s.Add(cfg); err != nil {
		t.Fatal(err)
	}
	err := s.Add(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate add")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestValidation(t *testing.T) {
	s := setupTestStore(t)

	tests := []struct { //nolint:govet // test struct, field order matches literal values
		cfg  *ServerConfig
		name string
	}{
		{&ServerConfig{Transport: TransportStdio, Command: "cmd"}, "empty name"},
		{&ServerConfig{Name: "bad", Transport: TransportStdio}, "stdio no command"},
		{&ServerConfig{Name: "bad", Transport: TransportSSE}, "sse no url"},
		{&ServerConfig{Name: "bad", Transport: "grpc"}, "invalid transport"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.Add(tc.cfg); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestUpdateEnvReplaces(t *testing.T) {
	s := setupTestStore(t)

	cfg := &ServerConfig{
		Name:      "gh",
		Transport: TransportStdio,
		Command:   "npx",
		Env:       map[string]string{"TOKEN": "abc", "STALE": "x"},
		Enabled:   true,
	}
	if err := s.Add(cfg); err != nil {
		t.Fatal(err)
	}

	// Replace env: drop STALE via empty string, change TOKEN, add NEW.
	err := s.UpdateEnv("gh", map[string]string{
		"TOKEN": "def",
		"STALE": "",
		"NEW":   "1",
	})
	if err != nil {
		t.Fatalf("UpdateEnv: %v", err)
	}

	got, err := s.Get("gh")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["TOKEN"] != "def" {
		t.Errorf("TOKEN = %q, want %q", got.Env["TOKEN"], "def")
	}
	if _, ok := got.Env["STALE"]; ok {
		t.Errorf("STALE should have been dropped, got %q", got.Env["STALE"])
	}
	if got.Env["NEW"] != "1" {
		t.Errorf("NEW = %q, want %q", got.Env["NEW"], "1")
	}
}

// TestUpdateEnvRollbackOnMissing confirms UpdateEnv is transactional: an
// update that fails (row not found) leaves every OTHER row untouched.
func TestUpdateEnvRollbackOnMissing(t *testing.T) {
	s := setupTestStore(t)

	orig := map[string]string{"TOKEN": "keep-me"}
	if err := s.Add(&ServerConfig{
		Name:      "srv",
		Transport: TransportStdio,
		Command:   "cmd",
		Env:       orig,
	}); err != nil {
		t.Fatal(err)
	}

	// UpdateEnv on a nonexistent server must error without mutating srv.
	err := s.UpdateEnv("does-not-exist", map[string]string{"X": "y"})
	if err == nil {
		t.Fatal("expected 'not found' error on missing server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}

	// Original server's env must be untouched.
	got, err := s.Get("srv")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env["TOKEN"] != "keep-me" {
		t.Errorf("env corrupted after failed UpdateEnv: got %q, want %q",
			got.Env["TOKEN"], "keep-me")
	}
}

func TestSSETransport(t *testing.T) {
	s := setupTestStore(t)

	cfg := &ServerConfig{
		Name:      "remote",
		Transport: TransportSSE,
		URL:       "https://api.example.com/mcp",
		Enabled:   true,
	}
	if err := s.Add(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("remote")
	if err != nil {
		t.Fatal(err)
	}
	if got.Transport != TransportSSE {
		t.Errorf("transport = %q, want %q", got.Transport, TransportSSE)
	}
	if got.URL != "https://api.example.com/mcp" {
		t.Errorf("url = %q, want %q", got.URL, "https://api.example.com/mcp")
	}
}
