package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateJSONToSQLite_AgentsJSON(t *testing.T) {
	dir := t.TempDir()
	stateDir := dir
	agentsDir := filepath.Join(stateDir, "agents")
	_ = os.MkdirAll(agentsDir, 0750)

	// Write agents.json with two agents
	agents := map[string]*Agent{
		"eng-01": {
			Name: "eng-01", Role: "engineer", State: StateIdle,
			Workspace: "/ws", StartedAt: time.Now(),
		},
		"eng-02": {
			Name: "eng-02", Role: "worker", State: StateWorking,
			Workspace: "/ws", StartedAt: time.Now(), Tool: "cursor",
		},
	}
	data, _ := json.MarshalIndent(agents, "", "  ")
	_ = os.WriteFile(filepath.Join(stateDir, "agents.json"), data, 0600)

	// Create store and migrate
	store, err := NewSQLiteStore(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	if migErr := migrateJSONToSQLite(store, stateDir, "/ws"); migErr != nil {
		t.Fatalf("migrateJSONToSQLite: %v", migErr)
	}

	// Verify agents are in SQLite
	all, err := store.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(all))
	}
	if all["eng-02"].Tool != "cursor" {
		t.Errorf("eng-02 tool = %q, want cursor", all["eng-02"].Tool)
	}

	// Verify agents.json was renamed to .migrated
	if _, err := os.Stat(filepath.Join(stateDir, "agents.json")); !os.IsNotExist(err) {
		t.Error("agents.json should have been renamed to .migrated")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "agents.json.migrated")); err != nil {
		t.Error("agents.json.migrated should exist")
	}
}

func TestMigrateJSONToSQLite_RootJSON(t *testing.T) {
	dir := t.TempDir()
	stateDir := dir
	agentsDir := filepath.Join(stateDir, "agents")
	_ = os.MkdirAll(agentsDir, 0750)

	// Write root.json (legacy format)
	now := time.Now()
	rootState := AgentState{
		Name:      "root",
		Role:      RoleRoot,
		State:     StateIdle,
		StartedAt: now,
		Session:   "tmux-root",
	}
	data, _ := json.MarshalIndent(rootState, "", "  ")
	_ = os.WriteFile(filepath.Join(agentsDir, "root.json"), data, 0600)

	store, err := NewSQLiteStore(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := migrateJSONToSQLite(store, stateDir, "/ws"); err != nil {
		t.Fatalf("migrateJSONToSQLite: %v", err)
	}

	root, _ := store.Load(context.Background(), "root")
	if root == nil {
		t.Fatal("root agent not found after migration")
	}
	if !root.IsRoot {
		t.Error("root should have IsRoot=true")
	}
}

func TestMigrateJSONToSQLite_PerAgentJSON(t *testing.T) {
	dir := t.TempDir()
	stateDir := dir
	agentsDir := filepath.Join(stateDir, "agents")
	_ = os.MkdirAll(agentsDir, 0750)

	// Write per-agent JSON file
	state := AgentState{
		Name:      "solo",
		Role:      "engineer",
		State:     StateWorking,
		StartedAt: time.Now(),
		Tool:      "agy",
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	_ = os.WriteFile(filepath.Join(agentsDir, "solo.json"), data, 0600)

	store, err := NewSQLiteStore(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := migrateJSONToSQLite(store, stateDir, "/ws"); err != nil {
		t.Fatalf("migrateJSONToSQLite: %v", err)
	}

	solo, _ := store.Load(context.Background(), "solo")
	if solo == nil {
		t.Fatal("solo agent not found after migration")
	}
	if solo.Tool != "agy" {
		t.Errorf("Tool = %q, want agy", solo.Tool)
	}

	// Verify file was renamed
	if _, err := os.Stat(filepath.Join(agentsDir, "solo.json")); !os.IsNotExist(err) {
		t.Error("solo.json should have been renamed")
	}
}

func TestNeedsMigration(t *testing.T) {
	dir := t.TempDir()

	// No files — no migration needed
	if needsMigration(dir) {
		t.Error("needsMigration should be false with no files")
	}

	// Create agents.json
	_ = os.WriteFile(filepath.Join(dir, "agents.json"), []byte("{}"), 0600)
	if !needsMigration(dir) {
		t.Error("needsMigration should be true with agents.json")
	}
}
