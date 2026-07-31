package home

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestRoleStore(t *testing.T) *RoleStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewRoleStore(dbPath)
	if err != nil {
		t.Fatalf("NewRoleStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRoleStore_SaveAndLoad(t *testing.T) {
	store := newTestRoleStore(t)

	role := &Role{
		Metadata: RoleMetadata{
			Name:         "engineer",
			Description:  "Feature developer",
			MCPServers:   []string{"mycel", "github"},
			ParentRoles:  []string{"base"},
			Secrets:      []string{"GITHUB_TOKEN"},
			Plugins:      []string{"typescript-lsp"},
			PromptCreate: "Welcome.",
			PromptStart:  "Check channels.",
			PromptStop:   "Save work.",
			PromptDelete: "Goodbye.",
			Review:       "Check tests.",
			Rules:        map[string]string{"lint": "Run make lint."},
			Commands:     map[string]string{"status": "Report status."},
			Skills:       map[string]string{"debug": "Debug issues."},
			Agents:       map[string]string{"helper": "A helper agent."},
			Settings:     map[string]any{"model": "opus"},
		},
		Prompt: "# Engineer\n\nYou implement features.",
	}

	if err := store.Save(role); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("engineer")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Metadata.Name != "engineer" {
		t.Errorf("Name = %q, want engineer", loaded.Metadata.Name)
	}
	if loaded.Metadata.Description != "Feature developer" {
		t.Errorf("Description = %q, want 'Feature developer'", loaded.Metadata.Description)
	}
	if loaded.Prompt != "# Engineer\n\nYou implement features." {
		t.Errorf("Prompt = %q", loaded.Prompt)
	}
	if len(loaded.Metadata.MCPServers) != 2 {
		t.Errorf("MCPServers len = %d, want 2", len(loaded.Metadata.MCPServers))
	}
	if len(loaded.Metadata.ParentRoles) != 1 {
		t.Errorf("ParentRoles len = %d, want 1", len(loaded.Metadata.ParentRoles))
	}
	if len(loaded.Metadata.Secrets) != 1 {
		t.Errorf("Secrets len = %d, want 1", len(loaded.Metadata.Secrets))
	}
	if len(loaded.Metadata.Plugins) != 1 {
		t.Errorf("Plugins len = %d, want 1", len(loaded.Metadata.Plugins))
	}
	if loaded.Metadata.PromptCreate != "Welcome." {
		t.Errorf("PromptCreate = %q", loaded.Metadata.PromptCreate)
	}
	if loaded.Metadata.PromptStart != "Check channels." {
		t.Errorf("PromptStart = %q", loaded.Metadata.PromptStart)
	}
	if loaded.Metadata.PromptStop != "Save work." {
		t.Errorf("PromptStop = %q", loaded.Metadata.PromptStop)
	}
	if loaded.Metadata.PromptDelete != "Goodbye." {
		t.Errorf("PromptDelete = %q", loaded.Metadata.PromptDelete)
	}
	if loaded.Metadata.Review != "Check tests." {
		t.Errorf("Review = %q", loaded.Metadata.Review)
	}
	if loaded.Metadata.Rules["lint"] != "Run make lint." {
		t.Errorf("Rules[lint] = %q", loaded.Metadata.Rules["lint"])
	}
	if loaded.Metadata.Commands["status"] != "Report status." {
		t.Errorf("Commands[status] = %q", loaded.Metadata.Commands["status"])
	}
	if loaded.Metadata.Skills["debug"] != "Debug issues." {
		t.Errorf("Skills[debug] = %q", loaded.Metadata.Skills["debug"])
	}
	if loaded.Metadata.Agents["helper"] != "A helper agent." {
		t.Errorf("Agents[helper] = %q", loaded.Metadata.Agents["helper"])
	}
	if loaded.Metadata.Settings["model"] != "opus" {
		t.Errorf("Settings[model] = %v", loaded.Metadata.Settings["model"])
	}
}

func TestRoleStore_SaveNoName(t *testing.T) {
	store := newTestRoleStore(t)

	role := &Role{Prompt: "no name"}
	if err := store.Save(role); err == nil {
		t.Error("Save should fail without name")
	}
}

func TestRoleStore_LoadNotFound(t *testing.T) {
	store := newTestRoleStore(t)

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("Load should fail for nonexistent role")
	}
}

func TestRoleStore_LoadAll(t *testing.T) {
	store := newTestRoleStore(t)

	roles := []*Role{
		{Metadata: RoleMetadata{Name: "alpha"}, Prompt: "Alpha."},
		{Metadata: RoleMetadata{Name: "beta"}, Prompt: "Beta."},
		{Metadata: RoleMetadata{Name: "gamma"}, Prompt: "Gamma."},
	}

	for _, r := range roles {
		if err := store.Save(r); err != nil {
			t.Fatalf("Save(%s): %v", r.Metadata.Name, err)
		}
	}

	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("LoadAll returned %d roles, want 3", len(all))
	}

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, ok := all[name]; !ok {
			t.Errorf("LoadAll missing role %q", name)
		}
	}
}

func TestRoleStore_Delete(t *testing.T) {
	store := newTestRoleStore(t)

	role := &Role{Metadata: RoleMetadata{Name: "deleteme"}, Prompt: "Delete me."}
	if err := store.Save(role); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete("deleteme"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if store.Has("deleteme") {
		t.Error("Has should return false after Delete")
	}
}

func TestRoleStore_DeleteNotFound(t *testing.T) {
	store := newTestRoleStore(t)

	err := store.Delete("nonexistent")
	if err == nil {
		t.Error("Delete should fail for nonexistent role")
	}
}

func TestRoleStore_Has(t *testing.T) {
	store := newTestRoleStore(t)

	if store.Has("nope") {
		t.Error("Has should return false for nonexistent role")
	}

	role := &Role{Metadata: RoleMetadata{Name: "exists"}, Prompt: "I exist."}
	if err := store.Save(role); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !store.Has("exists") {
		t.Error("Has should return true after Save")
	}
}

func TestRoleStore_SavePreservesCreatedAt(t *testing.T) {
	store := newTestRoleStore(t)

	role := &Role{Metadata: RoleMetadata{Name: "timey"}, Prompt: "v1"}
	if err := store.Save(role); err != nil {
		t.Fatalf("Save v1: %v", err)
	}

	// Update the role
	role.Prompt = "v2"
	if err := store.Save(role); err != nil {
		t.Fatalf("Save v2: %v", err)
	}

	loaded, err := store.Load("timey")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Prompt != "v2" {
		t.Errorf("Prompt = %q, want v2", loaded.Prompt)
	}
}

func TestRoleStore_MigrateFromFiles(t *testing.T) {
	store := newTestRoleStore(t)

	// Create temp roles directory with .md files
	rolesDir := filepath.Join(t.TempDir(), "roles")
	if err := os.MkdirAll(rolesDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Write role files
	files := map[string]string{
		"engineer.md": `---
name: engineer
description: Feature developer
mcp_servers:
  - mycel
  - github
parent_roles:
  - base
---

# Engineer

You implement features.
`,
		"manager.md": `---
name: manager
---

Manager prompt.
`,
		"readme.txt": "not a role file",
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(rolesDir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Create a subdirectory (should be skipped)
	if err := os.Mkdir(filepath.Join(rolesDir, "subdir.md"), 0750); err != nil {
		t.Fatal(err)
	}

	migrated, err := store.MigrateFromFiles(rolesDir)
	if err != nil {
		t.Fatalf("MigrateFromFiles: %v", err)
	}

	if migrated != 2 {
		t.Errorf("migrated = %d, want 2", migrated)
	}

	// Verify roles were migrated
	eng, err := store.Load("engineer")
	if err != nil {
		t.Fatalf("Load engineer: %v", err)
	}
	if eng.Metadata.Description != "Feature developer" {
		t.Errorf("engineer description = %q", eng.Metadata.Description)
	}
	if len(eng.Metadata.MCPServers) != 2 {
		t.Errorf("engineer MCPServers len = %d, want 2", len(eng.Metadata.MCPServers))
	}

	if !store.Has("manager") {
		t.Error("manager should exist after migration")
	}

	// txt file should not be migrated
	if store.Has("readme") {
		t.Error("readme.txt should not be migrated")
	}
}

func TestRoleStore_MigrateFromFiles_SkipsExisting(t *testing.T) {
	store := newTestRoleStore(t)

	// Pre-save a role
	role := &Role{
		Metadata: RoleMetadata{Name: "existing", Description: "Original"},
		Prompt:   "Original prompt.",
	}
	if err := store.Save(role); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Create a file with different content
	rolesDir := filepath.Join(t.TempDir(), "roles")
	if err := os.MkdirAll(rolesDir, 0750); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: existing\ndescription: Updated\n---\nUpdated prompt.\n"
	if err := os.WriteFile(filepath.Join(rolesDir, "existing.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	migrated, err := store.MigrateFromFiles(rolesDir)
	if err != nil {
		t.Fatalf("MigrateFromFiles: %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0 (should skip existing)", migrated)
	}

	// Verify original is preserved
	loaded, err := store.Load("existing")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Metadata.Description != "Original" {
		t.Errorf("Description = %q, want Original (should not overwrite)", loaded.Metadata.Description)
	}
}

func TestRoleStore_MigrateFromFiles_MissingDir(t *testing.T) {
	store := newTestRoleStore(t)

	migrated, err := store.MigrateFromFiles("/nonexistent/path")
	if err != nil {
		t.Fatalf("MigrateFromFiles should handle missing dir: %v", err)
	}
	if migrated != 0 {
		t.Errorf("migrated = %d, want 0", migrated)
	}
}

func TestRoleStore_MigrateDefaults(t *testing.T) {
	store := newTestRoleStore(t)

	if err := store.MigrateDefaults(); err != nil {
		t.Fatalf("MigrateDefaults: %v", err)
	}

	// Should have base + root + all default roles
	expectedCount := 2 + len(DefaultRoles)
	all, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != expectedCount {
		t.Errorf("LoadAll returned %d roles, want %d", len(all), expectedCount)
	}

	if !store.Has("base") {
		t.Error("base role should exist")
	}
	if !store.Has("root") {
		t.Error("root role should exist")
	}

	for name := range DefaultRoles {
		if !store.Has(name) {
			t.Errorf("default role %q should exist", name)
		}
	}

	// Second call should be idempotent
	if err := store.MigrateDefaults(); err != nil {
		t.Fatalf("MigrateDefaults (second call): %v", err)
	}
}

func TestRoleStore_SaveEmptySlicesAndMaps(t *testing.T) {
	store := newTestRoleStore(t)

	role := &Role{
		Metadata: RoleMetadata{Name: "minimal"},
		Prompt:   "Minimal role.",
	}

	if err := store.Save(role); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("minimal")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// nil slices should stay nil (not get unmarshaled to empty slices)
	if loaded.Metadata.MCPServers != nil {
		t.Errorf("MCPServers should be nil, got %v", loaded.Metadata.MCPServers)
	}
	if loaded.Metadata.ParentRoles != nil {
		t.Errorf("ParentRoles should be nil, got %v", loaded.Metadata.ParentRoles)
	}
	if loaded.Metadata.Settings != nil {
		t.Errorf("Settings should be nil, got %v", loaded.Metadata.Settings)
	}
}
