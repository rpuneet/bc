package tool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
)

// setupSharedDB creates a temporary SQLite shared database for tests.
func setupSharedDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bc.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetShared(d.DB, "sqlite")
	t.Cleanup(func() {
		db.SetShared(nil, "")
		_ = d.Close()
	})
}

func TestStore_CRUD(t *testing.T) {
	setupSharedDB(t)
	s := NewStore("")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	ctx := context.Background()

	// Built-ins seeded on Open
	tools, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected built-in tools to be seeded")
	}

	// Get built-in
	claude, err := s.Get(ctx, "claude")
	if err != nil {
		t.Fatalf("Get claude: %v", err)
	}
	if claude == nil {
		t.Fatal("expected claude built-in to exist")
	}
	if !claude.Builtin {
		t.Error("expected claude to be marked as builtin")
	}
	if !claude.Enabled {
		t.Error("expected claude to be enabled")
	}
	if len(claude.SlashCmds) == 0 {
		t.Error("expected claude to have slash cmds")
	}

	// Add custom tool
	custom := &Tool{
		Name:       "mytool",
		Command:    "mytool --yes",
		InstallCmd: "pip install mytool",
		UpgradeCmd: "pip install --upgrade mytool",
		SlashCmds:  []string{"/help", "/quit"},
		MCPServers: []string{"mcp-server-1"},
		Enabled:    true,
	}
	if addErr := s.Add(ctx, custom); addErr != nil {
		t.Fatalf("Add: %v", addErr)
	}

	// Get custom tool
	got, err := s.Get(ctx, "mytool")
	if err != nil {
		t.Fatalf("Get mytool: %v", err)
	}
	if got == nil {
		t.Fatal("expected mytool to exist")
	}
	if got.Command != custom.Command {
		t.Errorf("Command: got %q, want %q", got.Command, custom.Command)
	}
	if got.InstallCmd != custom.InstallCmd {
		t.Errorf("InstallCmd: got %q, want %q", got.InstallCmd, custom.InstallCmd)
	}
	if len(got.SlashCmds) != 2 {
		t.Errorf("SlashCmds: got %v, want %v", got.SlashCmds, custom.SlashCmds)
	}
	if len(got.MCPServers) != 1 || got.MCPServers[0] != "mcp-server-1" {
		t.Errorf("MCPServers: got %v", got.MCPServers)
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	// Duplicate add returns error
	if dupErr := s.Add(ctx, custom); dupErr == nil {
		t.Error("expected error adding duplicate tool")
	}

	// Update
	got.Command = "mytool --auto"
	got.SlashCmds = []string{"/exit"}
	if updateErr := s.Update(ctx, got); updateErr != nil {
		t.Fatalf("Update: %v", updateErr)
	}
	updated, err := s.Get(ctx, "mytool")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if updated.Command != "mytool --auto" {
		t.Errorf("updated Command: got %q", updated.Command)
	}
	if len(updated.SlashCmds) != 1 || updated.SlashCmds[0] != "/exit" {
		t.Errorf("updated SlashCmds: got %v", updated.SlashCmds)
	}

	// SetEnabled
	if setErr := s.SetEnabled(ctx, "mytool", false); setErr != nil {
		t.Fatalf("SetEnabled: %v", setErr)
	}
	disabled, err := s.Get(ctx, "mytool")
	if err != nil {
		t.Fatalf("Get after SetEnabled: %v", err)
	}
	if disabled.Enabled {
		t.Error("expected mytool to be disabled")
	}

	// Delete custom
	if delErr := s.Delete(ctx, "mytool"); delErr != nil {
		t.Fatalf("Delete: %v", delErr)
	}
	gone, err := s.Get(ctx, "mytool")
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if gone != nil {
		t.Error("expected mytool to be deleted")
	}

	// Delete non-existent returns error
	if err := s.Delete(ctx, "nonexistent"); err == nil {
		t.Error("expected error deleting nonexistent tool")
	}

	// Update non-existent returns error
	if err := s.Update(ctx, &Tool{Name: "nonexistent", Command: "x"}); err == nil {
		t.Error("expected error updating nonexistent tool")
	}
}

func TestStore_SeededOnce(t *testing.T) {
	setupSharedDB(t)

	// Open twice — built-ins should not duplicate
	for i := range 2 {
		s := NewStore("")
		if err := s.Open(); err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		tools, err := s.List(context.Background())
		if err != nil {
			s.Close() //nolint:errcheck // test cleanup
			t.Fatalf("List %d: %v", i, err)
		}
		s.Close() //nolint:errcheck // test cleanup

		// Count how many "claude" entries exist
		count := 0
		for _, tl := range tools {
			if tl.Name == "claude" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("open %d: expected 1 claude, got %d", i, count)
		}
	}
}

func TestStore_ListOrdering(t *testing.T) {
	setupSharedDB(t)
	s := NewStore("")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	ctx := context.Background()

	// Add custom tool
	if err := s.Add(ctx, &Tool{Name: "ztool", Command: "ztool", Enabled: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tools, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Built-ins should come first (ordered by builtin DESC), then custom
	var firstCustomIdx int
	for i, tl := range tools {
		if !tl.Builtin {
			firstCustomIdx = i
			break
		}
	}
	if firstCustomIdx == 0 {
		t.Error("expected built-ins to appear before custom tools")
	}
}

func TestStore_RequiredFields(t *testing.T) {
	setupSharedDB(t)
	s := NewStore("")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	ctx := context.Background()

	if err := s.Add(ctx, &Tool{Command: "x"}); err == nil {
		t.Error("expected error for missing name")
	}
	if err := s.Add(ctx, &Tool{Name: "x"}); err == nil {
		t.Error("expected error for missing command")
	}
}

func TestStore_SharedDBRequired(t *testing.T) {
	// Without a shared DB, Open should return an error
	db.SetShared(nil, "")
	s := NewStore("")
	if err := s.Open(); err == nil {
		t.Error("expected error when shared DB is not available")
	}
}

func TestTool_JSONSerialization(t *testing.T) {
	setupSharedDB(t)
	original := &Tool{
		Name:       "test",
		Command:    "test --yes",
		InstallCmd: "npm install test",
		UpgradeCmd: "npm update test",
		SlashCmds:  []string{"/help", "/quit"},
		MCPServers: []string{"mcp-1"},
		Config:     map[string]any{"key": "value"},
		Enabled:    true,
		CreatedAt:  time.Now().Truncate(time.Second),
	}

	s := NewStore("")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	ctx := context.Background()
	if err := s.Add(ctx, original); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := s.Get(ctx, "test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.SlashCmds[0] != "/help" || got.SlashCmds[1] != "/quit" {
		t.Errorf("SlashCmds mismatch: %v", got.SlashCmds)
	}
	if got.MCPServers[0] != "mcp-1" {
		t.Errorf("MCPServers mismatch: %v", got.MCPServers)
	}
	if v, ok := got.Config["key"]; !ok || v != "value" {
		t.Errorf("Config mismatch: %v", got.Config)
	}
}
