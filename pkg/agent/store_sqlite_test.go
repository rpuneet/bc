package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStore_SaveLoadDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().Truncate(time.Second)
	a := &Agent{
		Name:      "eng-01",
		ID:        "eng-01",
		Role:      Role("engineer"),
		State:     StateIdle,
		Tool:      "claude",
		Model:     "fable",
		Workspace: "/tmp/repo",
		CreatedAt: now,
		StartedAt: now,
		SessionID: "ses-abc123",
		Children:  []string{"child-1", "child-2"},
	}

	// Save
	if saveErr := store.Save(context.Background(), a); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	// Load
	loaded, err := store.Load(context.Background(), "eng-01")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.Name != "eng-01" {
		t.Errorf("Name = %q, want eng-01", loaded.Name)
	}
	if loaded.Role != Role("engineer") {
		t.Errorf("Role = %q, want engineer", loaded.Role)
	}
	if loaded.Tool != "claude" {
		t.Errorf("Tool = %q, want claude", loaded.Tool)
	}
	if loaded.Model != "fable" {
		t.Errorf("Model = %q, want fable", loaded.Model)
	}
	if len(loaded.Children) != 2 {
		t.Errorf("Children len = %d, want 2", len(loaded.Children))
	}
	if loaded.SessionID != "ses-abc123" {
		t.Errorf("SessionID = %q, want ses-abc123", loaded.SessionID)
	}
	if loaded.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Load non-existent
	missing, err := store.Load(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Load nonexistent: %v", err)
	}
	if missing != nil {
		t.Fatal("expected nil for nonexistent agent")
	}

	// Delete
	if err := store.Delete(context.Background(), "eng-01"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	after, _ := store.Load(context.Background(), "eng-01")
	if after != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSQLiteStore_LoadAll(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	for _, name := range []string{"a", "b", "c"} {
		_ = store.Save(context.Background(), &Agent{
			Name:      name,
			Role:      Role("worker"),
			State:     StateIdle,
			Workspace: "/tmp/repo",
			StartedAt: time.Now(),
		})
	}

	all, err := store.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("LoadAll returned %d agents, want 3", len(all))
	}
}

func TestSQLiteStore_SaveAll(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	agents := map[string]*Agent{
		"x": {Name: "x", Role: "worker", State: StateIdle, Workspace: "/repo", StartedAt: time.Now()},
		"y": {Name: "y", Role: "engineer", State: StateWorking, Workspace: "/repo", StartedAt: time.Now()},
	}
	if err := store.SaveAll(context.Background(), agents); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}

	all, _ := store.LoadAll(context.Background())
	if len(all) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(all))
	}
}

func TestSQLiteStore_UpdateState(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	_ = store.Save(context.Background(), &Agent{Name: "a", Role: "worker", State: StateIdle, Workspace: "/repo", StartedAt: time.Now()})

	if err := store.UpdateState(context.Background(), "a", StateWorking); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	a, _ := store.Load(context.Background(), "a")
	if a.State != StateWorking {
		t.Errorf("State = %q, want working", a.State)
	}

	// Non-existent agent
	if err := store.UpdateState(context.Background(), "zzz", StateIdle); err == nil {
		t.Fatal("expected error for non-existent agent")
	}
}

func TestSQLiteStore_UpdateField(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	_ = store.Save(context.Background(), &Agent{Name: "a", Role: "worker", State: StateIdle, Workspace: "/repo", StartedAt: time.Now()})

	if err := store.UpdateField(context.Background(), "a", "team", "alpha"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}

	a, _ := store.Load(context.Background(), "a")
	if a.Team != "alpha" {
		t.Errorf("Team = %q, want alpha", a.Team)
	}

	// Disallowed field
	if err := store.UpdateField(context.Background(), "a", "name", "evil"); err == nil {
		t.Fatal("expected error for disallowed field")
	}
}

func TestSQLiteStore_RootFields(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().Truncate(time.Second)
	a := &Agent{
		Name:          "root",
		Role:          RoleRoot,
		State:         StateIdle,
		Workspace:     "/repo",
		StartedAt:     now,
		IsRoot:        true,
		CrashCount:    2,
		LastCrashTime: &now,
		RecoveredFrom: "old-session",
		Children:      []string{"eng-01"},
	}

	if err := store.Save(context.Background(), a); err != nil {
		t.Fatalf("Save root: %v", err)
	}

	loaded, _ := store.Load(context.Background(), "root")
	if !loaded.IsRoot {
		t.Error("IsRoot should be true")
	}
	if loaded.CrashCount != 2 {
		t.Errorf("CrashCount = %d, want 2", loaded.CrashCount)
	}
	if loaded.LastCrashTime == nil {
		t.Error("LastCrashTime should not be nil")
	}
	if loaded.RecoveredFrom != "old-session" {
		t.Errorf("RecoveredFrom = %q, want old-session", loaded.RecoveredFrom)
	}
}

func TestSQLiteStore_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	// Two stores sharing the same DB
	s1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("s1: %v", err)
	}
	defer func() { _ = s1.Close() }()

	s2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("s2: %v", err)
	}
	defer func() { _ = s2.Close() }()

	// s1 saves agent A
	_ = s1.Save(context.Background(), &Agent{Name: "a", Role: "worker", State: StateIdle, Workspace: "/repo", StartedAt: time.Now()})

	// s2 saves agent B
	_ = s2.Save(context.Background(), &Agent{Name: "b", Role: "engineer", State: StateWorking, Workspace: "/repo", StartedAt: time.Now()})

	// Both should see both agents
	all1, _ := s1.LoadAll(context.Background())
	all2, _ := s2.LoadAll(context.Background())

	if len(all1) != 2 {
		t.Errorf("s1 sees %d agents, want 2", len(all1))
	}
	if len(all2) != 2 {
		t.Errorf("s2 sees %d agents, want 2", len(all2))
	}
}

func TestSQLiteStore_SoftDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Save two agents
	for _, name := range []string{"keep", "remove"} {
		_ = store.Save(context.Background(), &Agent{
			Name:      name,
			Role:      Role("worker"),
			State:     StateIdle,
			Workspace: "/tmp/repo",
			StartedAt: time.Now(),
		})
	}

	// Soft-delete one agent
	if softErr := store.SoftDelete(context.Background(), "remove"); softErr != nil {
		t.Fatalf("SoftDelete: %v", softErr)
	}

	// LoadAll should exclude the soft-deleted agent
	all, err := store.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("LoadAll returned %d agents, want 1", len(all))
	}
	if _, ok := all["keep"]; !ok {
		t.Error("expected 'keep' agent to be present")
	}
	if _, ok := all["remove"]; ok {
		t.Error("soft-deleted 'remove' agent should not appear in LoadAll")
	}

	// Direct Load should still find the soft-deleted agent (row exists)
	removed, err := store.Load(context.Background(), "remove")
	if err != nil {
		t.Fatalf("Load soft-deleted: %v", err)
	}
	if removed == nil {
		t.Fatal("Load should still return the soft-deleted agent row")
	}
	if removed.DeletedAt == nil {
		t.Error("DeletedAt should be set on soft-deleted agent")
	}

	// Hard-delete should remove the row entirely
	if err := store.Delete(context.Background(), "remove"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	after, _ := store.Load(context.Background(), "remove")
	if after != nil {
		t.Error("expected nil after hard delete")
	}
}

func TestSQLiteStore_DeletedAtPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	// First session: create and soft-delete an agent
	store1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	_ = store1.Save(context.Background(), &Agent{
		Name:      "zombie",
		Role:      Role("worker"),
		State:     StateIdle,
		Workspace: "/tmp/repo",
		StartedAt: time.Now(),
	})
	if softErr := store1.SoftDelete(context.Background(), "zombie"); softErr != nil {
		t.Fatalf("SoftDelete: %v", softErr)
	}
	_ = store1.Close()

	// Second session: simulate bcd restart
	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore after restart: %v", err)
	}
	defer func() { _ = store2.Close() }()

	all, err := store2.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll after restart: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 agents after restart, got %d (soft-deleted agent resurrected)", len(all))
	}
}

func TestSQLiteStore_EnvFilePersistence(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	now := time.Now().Truncate(time.Second)
	testEnvPath := "/tmp/test-agent-env.json"

	a := &Agent{
		Name:      "agent-with-env",
		ID:        "agent-with-env",
		Role:      Role("engineer"),
		State:     StateIdle,
		Tool:      "claude",
		Workspace: "/tmp/repo",
		CreatedAt: now,
		StartedAt: now,
		EnvFile:   testEnvPath,
		Children:  []string{},
	}

	// Save agent with EnvFile
	if saveErr := store.Save(context.Background(), a); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	// Load and verify EnvFile persisted
	loaded, err := store.Load(context.Background(), "agent-with-env")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.EnvFile != testEnvPath {
		t.Errorf("EnvFile = %q, want %q", loaded.EnvFile, testEnvPath)
	}

	// Test SaveAll also persists EnvFile
	agents := map[string]*Agent{
		"agent-1": {
			Name:      "agent-1",
			ID:        "agent-1",
			Role:      Role("engineer"),
			State:     StateIdle,
			Tool:      "claude",
			Workspace: "/tmp/repo",
			CreatedAt: now,
			StartedAt: now,
			EnvFile:   "/tmp/agent-1-env.json",
			Children:  []string{},
		},
		"agent-2": {
			Name:      "agent-2",
			ID:        "agent-2",
			Role:      Role("manager"),
			State:     StateWorking,
			Tool:      "pi",
			Workspace: "/tmp/repo2",
			CreatedAt: now,
			StartedAt: now,
			EnvFile:   "/tmp/agent-2-env.json",
			Children:  []string{},
		},
	}

	if saveAllErr := store.SaveAll(context.Background(), agents); saveAllErr != nil {
		t.Fatalf("SaveAll: %v", saveAllErr)
	}

	// Verify both agents loaded with correct EnvFile
	for name, expectedAgent := range agents {
		loaded, err := store.Load(context.Background(), name)
		if err != nil {
			t.Fatalf("Load %s: %v", name, err)
		}
		if loaded.EnvFile != expectedAgent.EnvFile {
			t.Errorf("%s EnvFile = %q, want %q", name, loaded.EnvFile, expectedAgent.EnvFile)
		}
	}
}
