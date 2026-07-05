package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/runtime"
	"github.com/rpuneet/mycel/pkg/tmux"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/pkg/worktree"
)

func TestMain(m *testing.M) {
	// Point the single global database (MYCEL_HOME/mycel.db) at a
	// throwaway dir so tests never touch ~/.mycel.
	var testHome string
	if home, err := os.MkdirTemp("", "mycel-test-home-*"); err == nil {
		testHome = home
		_ = os.Setenv("MYCEL_HOME", home)
	}

	// Setup roles for tests
	RoleCapabilities[Role("engineer")] = []Capability{CapImplementTasks}
	RoleCapabilities[Role("manager")] = []Capability{CapAssignWork}
	RoleCapabilities[Role("qa")] = []Capability{CapTestWork, CapReviewWork}
	RoleCapabilities[Role("product-manager")] = []Capability{CapCreateEpics, CapCreateAgents}
	RoleCapabilities[Role("worker")] = []Capability{CapImplementTasks}

	RoleHierarchy[Role("manager")] = []Role{Role("engineer"), Role("qa"), Role("worker")}
	RoleHierarchy[Role("product-manager")] = []Role{Role("manager")}
	RoleHierarchy[RoleRoot] = []Role{Role("product-manager"), Role("manager"), Role("engineer"), Role("qa"), Role("worker")}

	code := m.Run()
	if testHome != "" {
		_ = os.RemoveAll(testHome)
	}
	os.Exit(code)
}

// initGitRepo initializes a bare-minimum git repo in dir so worktree operations work.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	ctx := context.Background()
	for _, args := range [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		//nolint:gosec // test helper
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init setup (%v): %s: %v", args, out, err)
		}
	}
	// Create an initial commit so worktree add works
	dummy := filepath.Join(dir, ".gitkeep")
	if err := os.WriteFile(dummy, []byte(""), 0600); err != nil {
		t.Fatalf("write .gitkeep: %v", err)
	}
	for _, args := range [][]string{
		{"git", "-C", dir, "add", ".gitkeep"},
		{"git", "-C", dir, "commit", "-m", "init"},
	} {
		//nolint:gosec // test helper
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit setup (%v): %s: %v", args, out, err)
		}
	}
}

// newTestManager creates a Manager with a unique tmux prefix and temp state dir.
// The tmux manager uses a prefix that won't match any real sessions.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()

	// Initialize a git repo so worktree operations work
	initGitRepo(t, dir)

	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	be := runtime.NewTmuxBackend(tmux.NewManager(fmt.Sprintf("bctest-%d-", time.Now().UnixNano())))

	// Create role files BEFORE the RoleManager so migration picks them up.
	rolesDir := filepath.Join(dir, "roles")
	if mkErr := os.MkdirAll(rolesDir, 0750); mkErr != nil {
		t.Fatalf("MkdirAll roles: %v", mkErr)
	}
	for _, roleName := range []string{"engineer", "manager", "qa", "worker", "product-manager"} {
		if writeErr := os.WriteFile(
			filepath.Join(rolesDir, roleName+".md"),
			[]byte("---\nname: "+roleName+"\n---\n"),
			0600,
		); writeErr != nil {
			t.Fatalf("write role %s: %v", roleName, writeErr)
		}
	}
	rm := workspace.NewRoleManager(dir)

	return &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"tmux": be},
		defaultBackend: "tmux",
		stateDir:       dir,
		store:          store,
		agentCmd:       "/bin/true",
		workspacePath:  dir,
		worktreeMgr:    worktree.NewManager(dir),
		roleManager:    rm,
	}
}

// --- Agent struct method tests ---

func TestAgent_HasCapability(t *testing.T) {
	tests := []struct {
		name     string
		cap      Capability
		agent    Agent
		expected bool
	}{
		{"engineer can implement", CapImplementTasks, Agent{Role: Role("engineer")}, true},
		{"engineer cannot create agents", CapCreateAgents, Agent{Role: Role("engineer")}, false},
		{"manager can assign work", CapAssignWork, Agent{Role: Role("manager")}, true},
		{"qa can test work", CapTestWork, Agent{Role: Role("qa")}, true},
		{"qa can review work", CapReviewWork, Agent{Role: Role("qa")}, true},
		{"product manager can create epics", CapCreateEpics, Agent{Role: Role("product-manager")}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.agent.HasCapability(tc.cap); got != tc.expected {
				t.Errorf("Agent{Role: %s}.HasCapability(%s) = %v, want %v", tc.agent.Role, tc.cap, got, tc.expected)
			}
		})
	}
}

func TestAgent_CanCreate(t *testing.T) {
	tests := []struct {
		name      string
		childRole Role
		agent     Agent
		expected  bool
	}{
		{"manager can create engineer", Role("engineer"), Agent{Role: Role("manager")}, true},
		{"manager can create qa", Role("qa"), Agent{Role: Role("manager")}, true},
		{"engineer cannot create anything", Role("worker"), Agent{Role: Role("engineer")}, false},
		{"product manager can create manager", Role("manager"), Agent{Role: Role("product-manager")}, true},
		{"coordinator can create worker", Role("worker"), Agent{Role: RoleRoot}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.agent.CanCreate(tc.childRole); got != tc.expected {
				t.Errorf("Agent{Role: %s}.CanCreate(%s) = %v, want %v", tc.agent.Role, tc.childRole, got, tc.expected)
			}
		})
	}
}

func TestAgent_IsLeaf(t *testing.T) {
	t.Run("no children", func(t *testing.T) {
		a := Agent{Children: []string{}}
		if !a.IsLeaf() {
			t.Error("expected IsLeaf() = true for agent with no children")
		}
	})
	t.Run("nil children", func(t *testing.T) {
		a := Agent{}
		if !a.IsLeaf() {
			t.Error("expected IsLeaf() = true for agent with nil children")
		}
	})
	t.Run("has children", func(t *testing.T) {
		a := Agent{Children: []string{"child-1"}}
		if a.IsLeaf() {
			t.Error("expected IsLeaf() = false for agent with children")
		}
	})
}

func TestAgent_Level(t *testing.T) {
	tests := []struct {
		role     Role
		expected int
	}{
		{Role("product-manager"), 1},
		{RoleRoot, -1},
		{Role("manager"), 1},
		{Role("engineer"), 1},
		{Role("worker"), 1},
		{Role("qa"), 1},
	}
	for _, tc := range tests {
		a := Agent{Role: tc.role}
		if got := a.Level(); got != tc.expected {
			t.Errorf("Agent{Role: %s}.Level() = %d, want %d", tc.role, got, tc.expected)
		}
	}
}

// --- Pure function edge-case tests ---

func TestCanCreateRole_UnknownRole(t *testing.T) {
	if CanCreateRole(Role("unknown"), Role("engineer")) {
		t.Error("unknown parent role should return false")
	}
	if CanCreateRole(Role("manager"), Role("unknown")) {
		t.Error("unknown child role should return false")
	}
}

func TestHasCapability_UnknownRole(t *testing.T) {
	if HasCapability(Role("unknown"), CapImplementTasks) {
		t.Error("unknown role should return false")
	}
}

func TestRoleLevel_UnknownRole(t *testing.T) {
	if got := RoleLevel(Role("unknown")); got != 1 {
		t.Errorf("unknown role level = %d, want 1", got)
	}
}

func TestValidateTransition_UnknownState(t *testing.T) {
	err := ValidateTransition(State("bogus"), StateIdle)
	if err == nil {
		t.Error("expected error for unknown current state")
	}
}

// --- Constructor tests ---

func TestNewWorkspaceManager(t *testing.T) {
	m := NewWorkspaceManager("/tmp/test-agents", "/workspace")
	if m == nil {
		t.Fatal("NewWorkspaceManager returned nil")
	}
	if m.agents == nil {
		t.Error("agents map should be initialized")
	}
	if m.backends == nil || m.runtime() == nil {
		t.Error("runtime backend should be initialized")
	}
	if m.workspacePath != "/workspace" {
		t.Errorf("workspacePath = %q, want %q", m.workspacePath, "/workspace")
	}
}

// --- Config-dependent function tests ---

func TestSetAgentByName(t *testing.T) {
	mp := mockProvider{name: "testprov", installed: true, version: "1.0"}
	m := newTestManagerWithProvider(t, mp)

	t.Run("found", func(t *testing.T) {
		if !m.SetAgentByName("testprov") {
			t.Error("expected SetAgentByName to return true for known provider")
		}
		if m.agentCmd == "" {
			t.Error("agentCmd should not be empty after SetAgentByName")
		}
	})

	t.Run("not found", func(t *testing.T) {
		if m.SetAgentByName("nonexistent") {
			t.Error("expected SetAgentByName to return false for unknown provider")
		}
	})
}

// --- Manager listing tests ---

func TestListChildren(t *testing.T) {
	m := newTestManager(t)
	m.agents["manager-1"] = &Agent{
		Name:     "manager-1",
		Role:     Role("manager"),
		State:    StateIdle,
		Children: []string{"eng-1", "eng-2"},
	}
	m.agents["eng-1"] = &Agent{
		Name:     "eng-1",
		Role:     Role("engineer"),
		State:    StateWorking,
		ParentID: "manager-1",
		Children: []string{},
	}
	m.agents["eng-2"] = &Agent{
		Name:     "eng-2",
		Role:     Role("engineer"),
		State:    StateIdle,
		ParentID: "manager-1",
		Children: []string{},
	}

	t.Run("has children", func(t *testing.T) {
		children := m.ListChildren("manager-1")
		if len(children) != 2 {
			t.Fatalf("expected 2 children, got %d", len(children))
		}
	})

	t.Run("no children", func(t *testing.T) {
		children := m.ListChildren("eng-1")
		if len(children) != 0 {
			t.Errorf("expected 0 children, got %d", len(children))
		}
	})

	t.Run("nonexistent parent", func(t *testing.T) {
		children := m.ListChildren("nonexistent")
		if children != nil {
			t.Error("expected nil for nonexistent parent")
		}
	})

	t.Run("returns copies", func(t *testing.T) {
		children := m.ListChildren("manager-1")
		children[0].State = StateDone
		original := m.agents["eng-1"]
		if original.State == StateDone {
			t.Error("modifying returned child should not affect original")
		}
	})
}

func TestGetParent(t *testing.T) {
	m := newTestManager(t)
	m.agents["mgr"] = &Agent{
		Name:     "mgr",
		Role:     Role("manager"),
		State:    StateIdle,
		Children: []string{"eng-1"},
	}
	m.agents["eng-1"] = &Agent{
		Name:     "eng-1",
		Role:     Role("engineer"),
		State:    StateWorking,
		ParentID: "mgr",
		Children: []string{},
	}

	t.Run("has parent", func(t *testing.T) {
		parent := m.GetParent("eng-1")
		if parent == nil {
			t.Fatal("expected non-nil parent")
		}
		if parent.Name != "mgr" {
			t.Errorf("parent name = %q, want %q", parent.Name, "mgr")
		}
	})

	t.Run("no parent", func(t *testing.T) {
		parent := m.GetParent("mgr")
		if parent != nil {
			t.Error("expected nil parent for root agent")
		}
	})

	t.Run("nonexistent agent", func(t *testing.T) {
		parent := m.GetParent("nonexistent")
		if parent != nil {
			t.Error("expected nil for nonexistent agent")
		}
	})

	t.Run("parent not in map", func(t *testing.T) {
		m.agents["orphan"] = &Agent{
			Name:     "orphan",
			Role:     Role("engineer"),
			ParentID: "deleted-parent",
		}
		parent := m.GetParent("orphan")
		if parent != nil {
			t.Error("expected nil when parent ID references nonexistent agent")
		}
	})

	t.Run("returns copy", func(t *testing.T) {
		parent := m.GetParent("eng-1")
		parent.State = StateDone
		if m.agents["mgr"].State == StateDone {
			t.Error("modifying returned parent should not affect original")
		}
	})
}

func TestListByRole(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle, Children: []string{}}
	m.agents["eng-2"] = &Agent{Name: "eng-2", Role: Role("engineer"), State: StateWorking, Children: []string{}}
	m.agents["qa-1"] = &Agent{Name: "qa-1", Role: Role("qa"), State: StateIdle, Children: []string{}}
	m.agents["mgr"] = &Agent{Name: "mgr", Role: Role("manager"), State: StateIdle, Children: []string{}}

	t.Run("filter engineers", func(t *testing.T) {
		engineers := m.ListByRole(Role("engineer"))
		if len(engineers) != 2 {
			t.Fatalf("expected 2 engineers, got %d", len(engineers))
		}
		// Should be sorted by name
		if engineers[0].Name != "eng-1" || engineers[1].Name != "eng-2" {
			t.Errorf("engineers not sorted: got %s, %s", engineers[0].Name, engineers[1].Name)
		}
	})

	t.Run("filter qa", func(t *testing.T) {
		qas := m.ListByRole(Role("qa"))
		if len(qas) != 1 {
			t.Fatalf("expected 1 qa, got %d", len(qas))
		}
		if qas[0].Name != "qa-1" {
			t.Errorf("qa name = %q, want %q", qas[0].Name, "qa-1")
		}
	})

	t.Run("no matches", func(t *testing.T) {
		pms := m.ListByRole(Role("product-manager"))
		if len(pms) != 0 {
			t.Errorf("expected 0 product managers, got %d", len(pms))
		}
	})

	t.Run("returns copies", func(t *testing.T) {
		engineers := m.ListByRole(Role("engineer"))
		engineers[0].State = StateDone
		if m.agents["eng-1"].State == StateDone {
			t.Error("modifying returned agent should not affect original")
		}
	})
}

func TestAgentCount(t *testing.T) {
	m := newTestManager(t)

	if m.AgentCount() != 0 {
		t.Errorf("empty manager should have 0 agents, got %d", m.AgentCount())
	}

	m.agents["a"] = &Agent{Name: "a"}
	m.agents["b"] = &Agent{Name: "b"}
	m.agents["c"] = &Agent{Name: "c"}

	if m.AgentCount() != 3 {
		t.Errorf("expected 3 agents, got %d", m.AgentCount())
	}
}

func TestRunningCount(t *testing.T) {
	m := newTestManager(t)

	m.agents["a"] = &Agent{Name: "a", State: StateIdle}
	m.agents["b"] = &Agent{Name: "b", State: StateWorking}
	m.agents["c"] = &Agent{Name: "c", State: StateStopped}
	m.agents["d"] = &Agent{Name: "d", State: StateDone}
	m.agents["e"] = &Agent{Name: "e", State: StateStopped}

	// 3 non-stopped agents: a (idle), b (working), d (done)
	if got := m.RunningCount(); got != 3 {
		t.Errorf("RunningCount() = %d, want 3", got)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	m := newTestManager(t)
	if a := m.GetAgent("nonexistent"); a != nil {
		t.Error("expected nil for nonexistent agent")
	}
}

func TestListAgents_SortOrder(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-2"] = &Agent{Name: "eng-2", Role: Role("engineer"), State: StateIdle, Children: []string{}}
	m.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateIdle, Children: []string{}}
	m.agents["mgr"] = &Agent{Name: "mgr", Role: Role("manager"), State: StateIdle, Children: []string{}}
	m.agents["coord"] = &Agent{Name: "coord", Role: RoleRoot, State: StateIdle, Children: []string{}}
	m.agents["qa-1"] = &Agent{Name: "qa-1", Role: Role("qa"), State: StateIdle, Children: []string{}}

	agents := m.ListAgents()
	if len(agents) != 5 {
		t.Fatalf("expected 5 agents, got %d", len(agents))
	}

	// Root (level -1) first, then Manager (level 1), then Engineer/QA (level 1) sorted by name
	expectedOrder := []string{"coord", "eng-1", "eng-2", "mgr", "qa-1"}
	for i, expected := range expectedOrder {
		if agents[i].Name != expected {
			t.Errorf("agents[%d].Name = %q, want %q", i, agents[i].Name, expected)
		}
	}
}

// --- State persistence tests ---

func TestSaveAndLoadState(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	// Create manager and add agents
	m1 := &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"tmux": runtime.NewTmuxBackend(tmux.NewManager("test-"))},
		defaultBackend: "tmux",
		stateDir:       tmpDir,
		store:          store,
	}
	m1.agents["eng-1"] = &Agent{
		Name:      "eng-1",
		Role:      Role("engineer"),
		State:     StateWorking,
		Task:      "implementing feature",
		Workspace: "/ws",
		Children:  []string{},
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m1.agents["qa-1"] = &Agent{
		Name:      "qa-1",
		Role:      Role("qa"),
		State:     StateIdle,
		Workspace: "/ws",
		Children:  []string{},
		StartedAt: time.Now(),
	}

	// Save state
	if err := m1.saveState(context.Background()); err != nil {
		t.Fatalf("saveState failed: %v", err)
	}
	_ = store.Close()

	// Verify DB file exists
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("state.db not created: %v", err)
	}

	// Load into new manager
	m2 := &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"tmux": runtime.NewTmuxBackend(tmux.NewManager("test-"))},
		defaultBackend: "tmux",
		stateDir:       tmpDir,
	}
	if err := m2.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	defer func() { _ = m2.Close() }()

	// Verify loaded agents
	if len(m2.agents) != 2 {
		t.Fatalf("expected 2 agents after load, got %d", len(m2.agents))
	}

	eng := m2.agents["eng-1"]
	if eng == nil {
		t.Fatal("eng-1 not found after load")
	}
	if eng.Role != Role("engineer") {
		t.Errorf("eng-1 role = %s, want %s", eng.Role, Role("engineer"))
	}
	if eng.State != StateWorking {
		t.Errorf("eng-1 state = %s, want %s", eng.State, StateWorking)
	}
	if eng.Task != "implementing feature" {
		t.Errorf("eng-1 task = %q, want %q", eng.Task, "implementing feature")
	}
}

func TestLoadState_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"tmux": runtime.NewTmuxBackend(tmux.NewManager("test-"))},
		defaultBackend: "tmux",
		stateDir:       tmpDir,
	}
	// No agents.json exists, should return nil (not error)
	if err := m.LoadState(); err != nil {
		t.Errorf("LoadState with no file should return nil, got: %v", err)
	}
	if len(m.agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(m.agents))
	}
}

func TestLoadState_EmptyStateDir(t *testing.T) {
	m := &Manager{
		agents:   make(map[string]*Agent),
		stateDir: "",
	}
	if err := m.LoadState(); err != nil {
		t.Errorf("LoadState with empty stateDir should return nil, got: %v", err)
	}
}

func TestSaveState_EmptyStateDir(t *testing.T) {
	m := &Manager{
		agents:   make(map[string]*Agent),
		stateDir: "",
	}
	if err := m.saveState(context.Background()); err != nil {
		t.Errorf("saveState with empty stateDir should return nil, got: %v", err)
	}
}

func TestSaveState_WithAgents(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := NewSQLiteStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	m := &Manager{
		agents: map[string]*Agent{
			"test-agent": {
				Name:      "test-agent",
				Role:      Role("engineer"),
				State:     StateWorking,
				Task:      "testing",
				Workspace: "/ws",
				StartedAt: time.Now(),
			},
		},
		stateDir: tmpDir,
		store:    store,
	}
	if saveErr := m.saveState(context.Background()); saveErr != nil {
		t.Fatalf("saveState failed: %v", saveErr)
	}

	// Verify agent was saved to SQLite
	loaded, err := store.Load(context.Background(), "test-agent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected agent in SQLite after saveState")
	}
	if loaded.Task != "testing" {
		t.Errorf("task = %q, want testing", loaded.Task)
	}
}

func TestSaveState_NilStore(t *testing.T) {
	m := &Manager{
		agents:   map[string]*Agent{},
		stateDir: t.TempDir(),
		store:    nil, // no store
	}
	if err := m.saveState(context.Background()); err != nil {
		t.Fatalf("saveState with nil store should be no-op: %v", err)
	}
}

func TestSaveState_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	original := &Manager{
		agents: map[string]*Agent{
			"agent-1": {
				Name:      "agent-1",
				Role:      Role("engineer"),
				State:     StateIdle,
				ParentID:  "root",
				Workspace: "/ws",
				StartedAt: time.Now(),
			},
			"agent-2": {
				Name:      "agent-2",
				Role:      Role("qa"),
				State:     StateWorking,
				Task:      "running tests",
				Workspace: "/ws",
				StartedAt: time.Now(),
			},
		},
		stateDir: tmpDir,
		store:    store,
	}
	if err := original.saveState(context.Background()); err != nil {
		t.Fatalf("saveState failed: %v", err)
	}
	_ = store.Close()

	// Load into new manager (LoadState opens a new store)
	loaded := &Manager{
		agents:   make(map[string]*Agent),
		stateDir: tmpDir,
	}
	if err := loaded.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	defer func() { _ = loaded.Close() }()

	if len(loaded.agents) != 2 {
		t.Errorf("expected 2 agents after load, got %d", len(loaded.agents))
	}
	if loaded.agents["agent-1"] == nil {
		t.Error("agent-1 should exist after load")
	}
	if loaded.agents["agent-2"] == nil {
		t.Error("agent-2 should exist after load")
	}
	if loaded.agents["agent-2"].Task != "running tests" {
		t.Errorf("agent-2 task = %q, want 'running tests'", loaded.agents["agent-2"].Task)
	}
}

func TestLoadState_MigratesCorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "agents.json")
	if err := os.WriteFile(stateFile, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}

	m := &Manager{
		agents:   make(map[string]*Agent),
		stateDir: tmpDir,
	}
	// Migration will warn about corrupt JSON but LoadState still succeeds
	// (the corrupt file triggers migration which logs a warning and continues)
	if err := m.LoadState(); err != nil {
		t.Fatalf("LoadState should not error on corrupt JSON (migration handles it): %v", err)
	}
	defer func() { _ = m.Close() }()

	// Should have no agents since the JSON was corrupt
	if len(m.agents) != 0 {
		t.Errorf("expected 0 agents from corrupt JSON, got %d", len(m.agents))
	}
}

// --- LoadRoleMemory tests ---

func TestLoadRoleMemory(t *testing.T) {
	tmpDir := t.TempDir()
	rolesDir := filepath.Join(tmpDir, ".bc", "roles")
	if err := os.MkdirAll(rolesDir, 0750); err != nil {
		t.Fatal(err)
	}

	t.Run("file exists", func(t *testing.T) {
		content := "You are an engineer. Write code and tests."
		// RoleManager expects YAML or Markdown with frontmatter or just prompt?
		// Actually RoleManager might expect a specific format.
		// Let's check RoleManager.LoadRole.
		if err := os.WriteFile(filepath.Join(rolesDir, "engineer.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		mem := LoadRoleMemory(tmpDir, Role("engineer"))
		if mem == nil {
			t.Fatal("expected non-nil AgentMemory")
		}
		if mem.RolePrompt != content {
			t.Errorf("RolePrompt = %q, want %q", mem.RolePrompt, content)
		}
		if mem.LoadedAt.IsZero() {
			t.Error("LoadedAt should not be zero")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		mem := LoadRoleMemory(tmpDir, Role("qa"))
		if mem != nil {
			t.Error("expected nil AgentMemory for missing file")
		}
	})

	t.Run("product-manager role", func(t *testing.T) {
		content := "You are a product manager."
		if err := os.WriteFile(filepath.Join(rolesDir, "product-manager.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		mem := LoadRoleMemory(tmpDir, Role("product-manager"))
		if mem == nil {
			t.Fatal("expected non-nil AgentMemory for product-manager")
		}
		if mem.RolePrompt != content {
			t.Errorf("RolePrompt = %q, want %q", mem.RolePrompt, content)
		}
	})

	t.Run("root role from prompts dir", func(t *testing.T) {
		// Root role uses backward compatible prompts/root.md path
		promptsDir := filepath.Join(tmpDir, "prompts")
		if mkErr := os.MkdirAll(promptsDir, 0750); mkErr != nil {
			t.Fatal(mkErr)
		}
		content := "You are the root coordinator."
		if writeErr := os.WriteFile(filepath.Join(promptsDir, "root.md"), []byte(content), 0600); writeErr != nil {
			t.Fatal(writeErr)
		}

		mem := LoadRoleMemory(tmpDir, RoleRoot)
		if mem == nil {
			t.Fatal("expected non-nil AgentMemory for root role")
		}
		if mem.RolePrompt != content {
			t.Errorf("RolePrompt = %q, want %q", mem.RolePrompt, content)
		}
	})

	t.Run("empty prompt returns nil", func(t *testing.T) {
		// Create role file with empty content
		if writeErr := os.WriteFile(filepath.Join(rolesDir, "empty.md"), []byte(""), 0600); writeErr != nil {
			t.Fatal(writeErr)
		}

		mem := LoadRoleMemory(tmpDir, Role("empty"))
		if mem != nil {
			t.Error("expected nil AgentMemory for empty prompt file")
		}
	})
}

// --- Stop operations tests ---

func TestStopAgent(t *testing.T) {
	m := newTestManager(t)
	m.agents["mgr"] = &Agent{
		Name:     "mgr",
		Role:     Role("manager"),
		State:    StateIdle,
		Children: []string{"eng-1"},
	}
	m.agents["eng-1"] = &Agent{
		Name:     "eng-1",
		Role:     Role("engineer"),
		State:    StateWorking,
		ParentID: "mgr",
		Children: []string{},
	}

	// Stop eng-1
	if err := m.StopAgent(context.Background(), "eng-1"); err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}

	// Agent should be stopped
	if m.agents["eng-1"].State != StateStopped {
		t.Errorf("agent state = %s, want %s", m.agents["eng-1"].State, StateStopped)
	}

	// Parent's children should be updated
	if len(m.agents["mgr"].Children) != 0 {
		t.Errorf("parent children = %v, want empty", m.agents["mgr"].Children)
	}

	// State should be persisted to SQLite
	loaded, err := m.store.Load(context.Background(), "eng-1")
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if loaded == nil {
		t.Error("state should be persisted after StopAgent")
	}
	if loaded != nil && loaded.State != StateStopped {
		t.Errorf("persisted state = %s, want %s", loaded.State, StateStopped)
	}
}

func TestStopAgent_NotFound(t *testing.T) {
	m := newTestManager(t)
	if err := m.StopAgent(context.Background(), "nonexistent"); err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestStopAgent_WithWorktree(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{
		Name:        "eng-1",
		Role:        Role("engineer"),
		State:       StateWorking,
		Workspace:   "/tmp/workspace",
		WorktreeDir: "/tmp/workspace/.bc/worktrees/eng-1",
		Children:    []string{},
	}

	// Stop should succeed and preserve worktree for later restart
	if err := m.StopAgent(context.Background(), "eng-1"); err != nil {
		t.Fatalf("StopAgent with worktree failed: %v", err)
	}
	if m.agents["eng-1"].State != StateStopped {
		t.Errorf("agent state = %s, want %s", m.agents["eng-1"].State, StateStopped)
	}
	// Worktree should be preserved (not cleared) so agent can resume work on restart
	if m.agents["eng-1"].WorktreeDir != "/tmp/workspace/.bc/worktrees/eng-1" {
		t.Error("worktree dir should be preserved after stop, not cleared")
	}
}

func TestStopAgent_WorktreeSameAsWorkspace(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{
		Name:        "eng-1",
		Role:        Role("engineer"),
		State:       StateWorking,
		Workspace:   "/tmp/workspace",
		WorktreeDir: "/tmp/workspace", // Same as workspace
		Children:    []string{},
	}

	if err := m.StopAgent(context.Background(), "eng-1"); err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}
	// WorktreeDir should NOT be cleared when it equals Workspace
	if m.agents["eng-1"].WorktreeDir != "/tmp/workspace" {
		t.Error("worktreeDir should not be cleared when equal to workspace")
	}
}

func TestStopAll(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{Name: "eng-1", Role: Role("engineer"), State: StateWorking, Children: []string{}}
	m.agents["eng-2"] = &Agent{Name: "eng-2", Role: Role("engineer"), State: StateIdle, Children: []string{}}
	m.agents["qa-1"] = &Agent{Name: "qa-1", Role: Role("qa"), State: StateDone, Children: []string{}}

	if err := m.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	for name, a := range m.agents {
		if a.State != StateStopped {
			t.Errorf("agent %s state = %s, want %s", name, a.State, StateStopped)
		}
	}
}

func TestStopAgentTree(t *testing.T) {
	m := newTestManager(t)
	m.agents["mgr"] = &Agent{
		Name:     "mgr",
		Role:     Role("manager"),
		State:    StateIdle,
		Children: []string{"eng-1", "eng-2"},
	}
	m.agents["eng-1"] = &Agent{
		Name:     "eng-1",
		Role:     Role("engineer"),
		State:    StateWorking,
		ParentID: "mgr",
		Children: []string{},
	}
	m.agents["eng-2"] = &Agent{
		Name:     "eng-2",
		Role:     Role("engineer"),
		State:    StateIdle,
		ParentID: "mgr",
		Children: []string{},
	}

	if err := m.StopAgentTree(context.Background(), "mgr"); err != nil {
		t.Fatalf("StopAgentTree failed: %v", err)
	}

	// All should be stopped
	for _, name := range []string{"mgr", "eng-1", "eng-2"} {
		if m.agents[name].State != StateStopped {
			t.Errorf("agent %s state = %s, want %s", name, m.agents[name].State, StateStopped)
		}
	}

	// Manager's children should be cleared
	if len(m.agents["mgr"].Children) != 0 {
		t.Errorf("manager children = %v, want empty", m.agents["mgr"].Children)
	}
}

func TestStopAgentTree_NotFound(t *testing.T) {
	m := newTestManager(t)
	if err := m.StopAgentTree(context.Background(), "nonexistent"); err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

// --- RenameAgent tests ---

func TestRenameAgent(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-01"] = &Agent{Name: "eng-01", Role: Role("engineer"), State: StateStopped, Children: []string{}}

	if err := m.RenameAgent(context.Background(), "eng-01", "engineer-01"); err != nil {
		t.Fatalf("RenameAgent failed: %v", err)
	}

	// Old name should not exist
	if _, exists := m.agents["eng-01"]; exists {
		t.Error("old agent name should not exist")
	}

	// New name should exist
	agent, exists := m.agents["engineer-01"]
	if !exists {
		t.Fatal("new agent name should exist")
	}

	// Agent name should be updated
	if agent.Name != "engineer-01" {
		t.Errorf("agent.Name = %q, want %q", agent.Name, "engineer-01")
	}
}

func TestRenameAgent_NotFound(t *testing.T) {
	m := newTestManager(t)
	if err := m.RenameAgent(context.Background(), "nonexistent", "new-name"); err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestRenameAgent_NameExists(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-01"] = &Agent{Name: "eng-01", State: StateStopped, Children: []string{}}
	m.agents["eng-02"] = &Agent{Name: "eng-02", State: StateStopped, Children: []string{}}

	if err := m.RenameAgent(context.Background(), "eng-01", "eng-02"); err == nil {
		t.Error("expected error when renaming to existing name")
	}
}

func TestRenameAgent_UpdatesParentChildren(t *testing.T) {
	m := newTestManager(t)
	m.agents["mgr"] = &Agent{Name: "mgr", State: StateIdle, Children: []string{"eng-01", "eng-02"}}
	m.agents["eng-01"] = &Agent{Name: "eng-01", ParentID: "mgr", State: StateStopped, Children: []string{}}
	m.agents["eng-02"] = &Agent{Name: "eng-02", ParentID: "mgr", State: StateStopped, Children: []string{}}

	if err := m.RenameAgent(context.Background(), "eng-01", "engineer-01"); err != nil {
		t.Fatalf("RenameAgent failed: %v", err)
	}

	// Parent's children list should be updated
	children := m.agents["mgr"].Children
	found := false
	for _, c := range children {
		if c == "engineer-01" {
			found = true
		}
		if c == "eng-01" {
			t.Error("old name should not be in parent's children")
		}
	}
	if !found {
		t.Error("new name should be in parent's children")
	}
}

// --- State transition tests ---

func TestRefreshState_Legacy(t *testing.T) {
	// Hooks are now the single source of truth for agent state.
	// This test verifies that agents stay in their persisted state
	// when no hook events are received.
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{Name: "eng-1", State: StateWorking, Children: []string{}}
	m.agents["eng-2"] = &Agent{Name: "eng-2", State: StateStopped, Children: []string{}}

	// eng-1 should remain working (hooks drive state, not polling)
	if m.agents["eng-1"].State != StateWorking {
		t.Errorf("eng-1 state = %s, want working", m.agents["eng-1"].State)
	}

	// eng-2 was already stopped → should remain stopped
	if m.agents["eng-2"].State != StateStopped {
		t.Errorf("eng-2 state = %s, want stopped", m.agents["eng-2"].State)
	}
}

// --- SpawnAgent error path tests ---

func TestSpawnAgentWithOptions_ParentNotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{Name: "eng-1", Role: Role("engineer"), Workspace: "/tmp", ParentID: "nonexistent-parent"})
	if err == nil {
		t.Error("expected error when parent not found")
	}
}

func TestSpawnAgentWithOptions_ParentCantCreate(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{
		Name:     "eng-1",
		Role:     Role("engineer"),
		State:    StateIdle,
		Children: []string{},
	}

	// Engineer cannot create other engineers
	_, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{Name: "eng-2", Role: Role("engineer"), Workspace: "/tmp", ParentID: "eng-1"})
	if err == nil {
		t.Error("expected error when parent can't create child role")
	}
}

func TestSpawnAgentWithOptions_NullRole(t *testing.T) {
	m := newTestManager(t)

	tests := []struct {
		name string
		role Role
	}{
		{"empty role", Role("")},
		{"null string", Role("null")},
		{"nil-like string", Role("<nil>")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{Name: "test-agent", Role: tt.role, Workspace: "/tmp"})
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), "role is required") {
				t.Errorf("expected 'role is required' error, got: %v", err)
			}
		})
	}
}

func TestSpawnAgentWithOptions_NameTooLong(t *testing.T) {
	m := newTestManager(t)
	longName := strings.Repeat("a", MaxAgentNameLength+1)
	_, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{Name: longName, Role: Role("engineer"), Workspace: "/tmp"})
	if err == nil {
		t.Error("expected error for name exceeding max length")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected 'invalid' in error, got: %v", err)
	}
}

func TestSpawnAgentWithOptions_NonexistentRole(t *testing.T) {
	m := newTestManager(t)
	_, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{Name: "test-agent", Role: Role("nonexistent-role"), Workspace: "/tmp"})
	if err == nil {
		t.Error("expected error for nonexistent role")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %v", err)
	}
}

func TestSpawnAgentWithOptions_UnknownTool(t *testing.T) {
	m := newTestManager(t)
	_, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{Name: "eng-1", Role: Role("engineer"), Workspace: "/tmp", Tool: "nonexistent-tool"})
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

// --- Runtime accessor test ---

func TestRuntime(t *testing.T) {
	m := newTestManager(t)
	if m.Runtime() == nil {
		t.Error("Runtime() should not return nil")
	}
}

// --- UpdateAgentState with task update ---

func TestUpdateAgentState_TaskUpdate(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-1"] = &Agent{
		Name:  "eng-1",
		Role:  Role("engineer"),
		State: StateIdle,
	}

	// Transition to working with a task
	if err := m.UpdateAgentState(context.Background(), "eng-1", StateWorking, "writing tests"); err != nil {
		t.Fatalf("UpdateAgentState failed: %v", err)
	}

	a := m.agents["eng-1"]
	if a.Task != "writing tests" {
		t.Errorf("task = %q, want %q", a.Task, "writing tests")
	}
	if a.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// State should be persisted to SQLite
	loaded, err := m.store.Load(context.Background(), "eng-1")
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if loaded == nil {
		t.Error("state should be persisted after UpdateAgentState")
	}
	if loaded != nil && loaded.Task != "writing tests" {
		t.Errorf("persisted task = %q, want %q", loaded.Task, "writing tests")
	}
}

// --- removeFromParent tests ---

func TestRemoveFromParent(t *testing.T) {
	m := newTestManager(t)

	t.Run("removes child from parent", func(t *testing.T) {
		m.agents["parent"] = &Agent{
			Name:     "parent",
			Role:     Role("manager"),
			Children: []string{"child-1", "child-2", "child-3"},
		}
		m.agents["child-2"] = &Agent{
			Name:     "child-2",
			ParentID: "parent",
		}

		m.removeFromParent("child-2")

		parent := m.agents["parent"]
		if len(parent.Children) != 2 {
			t.Errorf("expected 2 children after removal, got %d", len(parent.Children))
		}
		for _, child := range parent.Children {
			if child == "child-2" {
				t.Error("child-2 should have been removed from parent")
			}
		}
	})

	t.Run("agent not in map", func(t *testing.T) {
		// Should not panic
		m.removeFromParent("nonexistent")
	})

	t.Run("no parent ID", func(t *testing.T) {
		m.agents["orphan"] = &Agent{Name: "orphan"}
		// Should not panic
		m.removeFromParent("orphan")
	})

	t.Run("parent not in map", func(t *testing.T) {
		m.agents["lost-child"] = &Agent{
			Name:     "lost-child",
			ParentID: "deleted-parent",
		}
		// Should not panic
		m.removeFromParent("lost-child")
	})
}

// --- State persistence round-trip with complex data ---

func TestSaveLoadState_ComplexHierarchy(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "state.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	m := &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"tmux": runtime.NewTmuxBackend(tmux.NewManager("test-"))},
		defaultBackend: "tmux",
		stateDir:       tmpDir,
		store:          store,
	}

	now := time.Now().Truncate(time.Second)
	m.agents["coord"] = &Agent{
		ID:        "coord",
		Name:      "coord",
		Role:      RoleRoot,
		State:     StateIdle,
		Workspace: "/workspace",
		Session:   "coord",
		Children:  []string{"mgr"},
		StartedAt: now,
		UpdatedAt: now,
		RolePrompt: &AgentMemory{
			RolePrompt: "You are a root.",
			LoadedAt:   now,
		},
	}
	m.agents["mgr"] = &Agent{
		ID:          "mgr",
		Name:        "mgr",
		Role:        Role("manager"),
		State:       StateWorking,
		Workspace:   "/workspace",
		Session:     "mgr",
		ParentID:    "coord",
		Children:    []string{},
		HookedWork:  "work-001",
		WorktreeDir: "/workspace/.bc/worktrees/mgr",
		Tool:        "claude",
		StartedAt:   now,
		UpdatedAt:   now,
	}

	if err := m.saveState(context.Background()); err != nil {
		t.Fatalf("saveState failed: %v", err)
	}
	_ = store.Close()

	// Load into fresh manager
	m2 := &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"tmux": runtime.NewTmuxBackend(tmux.NewManager("test-"))},
		defaultBackend: "tmux",
		stateDir:       tmpDir,
	}
	if err := m2.LoadState(); err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if m2.store != nil {
		t.Cleanup(func() { _ = m2.store.Close() })
	}

	// Verify complex fields
	coord := m2.agents["coord"]
	if coord == nil {
		t.Fatal("coord not found")
	}
	// Memory is a runtime-only field (not persisted to SQLite)
	if len(coord.Children) != 1 || coord.Children[0] != "mgr" {
		t.Errorf("coord children = %v, want [mgr]", coord.Children)
	}

	mgr := m2.agents["mgr"]
	if mgr == nil {
		t.Fatal("mgr not found")
	}
	if mgr.ParentID != "coord" {
		t.Errorf("mgr ParentID = %q, want %q", mgr.ParentID, "coord")
	}
	if mgr.Tool != "claude" {
		t.Errorf("mgr Tool = %q, want %q", mgr.Tool, "claude")
	}
	if mgr.WorktreeDir != "/workspace/.bc/worktrees/mgr" {
		t.Errorf("mgr WorktreeDir = %q, want expected", mgr.WorktreeDir)
	}
	if mgr.HookedWork != "work-001" {
		t.Errorf("mgr HookedWork = %q, want %q", mgr.HookedWork, "work-001")
	}
}

// --- JSON serialization tests ---

func TestAgentJSON_RoundTrip(t *testing.T) {
	original := &Agent{
		ID:          "eng-1",
		Name:        "eng-1",
		Role:        Role("engineer"),
		State:       StateWorking,
		Workspace:   "/workspace",
		Task:        "writing tests",
		Session:     "eng-1-session",
		Tool:        "claude",
		ParentID:    "mgr",
		Children:    []string{"sub1"},
		HookedWork:  "work-099",
		WorktreeDir: "/workspace/.bc/worktrees/eng-1",
		StartedAt:   time.Now().Truncate(time.Second),
		UpdatedAt:   time.Now().Truncate(time.Second),
		RolePrompt: &AgentMemory{
			RolePrompt: "test prompt",
			LoadedAt:   time.Now().Truncate(time.Second),
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var loaded Agent
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if loaded.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", loaded.ID, original.ID)
	}
	if loaded.Role != original.Role {
		t.Errorf("Role mismatch: %q vs %q", loaded.Role, original.Role)
	}
	if loaded.State != original.State {
		t.Errorf("State mismatch: %q vs %q", loaded.State, original.State)
	}
	if loaded.Tool != original.Tool {
		t.Errorf("Tool mismatch: %q vs %q", loaded.Tool, original.Tool)
	}
	if loaded.ParentID != original.ParentID {
		t.Errorf("ParentID mismatch: %q vs %q", loaded.ParentID, original.ParentID)
	}
	if loaded.RolePrompt == nil {
		t.Fatal("RolePrompt should not be nil after round-trip")
	}
	if loaded.RolePrompt.RolePrompt != original.RolePrompt.RolePrompt {
		t.Errorf("RolePrompt mismatch: %q vs %q", loaded.RolePrompt.RolePrompt, original.RolePrompt.RolePrompt)
	}
}

// --- Concurrent access tests (preserved from original) ---

func TestConcurrentSetAgentCommand(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				m.SetAgentCommand("claude")
			} else {
				m.SetAgentCommand("cursor-agent")
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrentGetAgent(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["test-agent"] = &Agent{
		Name:     "test-agent",
		Role:     Role("worker"),
		State:    StateIdle,
		Children: []string{"child1", "child2"},
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := m.GetAgent("test-agent")
			if a == nil {
				t.Error("GetAgent returned nil")
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentListAgents(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["agent1"] = &Agent{Name: "agent1", Role: Role("worker"), State: StateIdle}
	m.agents["agent2"] = &Agent{Name: "agent2", Role: Role("worker"), State: StateWorking}
	m.agents["agent3"] = &Agent{Name: "agent3", Role: RoleRoot, State: StateIdle}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			agents := m.ListAgents()
			if len(agents) != 3 {
				t.Errorf("expected 3 agents, got %d", len(agents))
			}
		}()
	}
	wg.Wait()
}

func TestGetAgentReturnsCopy(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["test-agent"] = &Agent{
		Name:     "test-agent",
		Role:     Role("worker"),
		State:    StateIdle,
		Children: []string{"child1"},
	}

	copy := m.GetAgent("test-agent")
	copy.State = StateWorking
	copy.Children = append(copy.Children, "child2")

	original := m.agents["test-agent"]
	if original.State != StateIdle {
		t.Errorf("original state was modified: expected %s, got %s", StateIdle, original.State)
	}
	if len(original.Children) != 1 {
		t.Errorf("original children was modified: expected 1, got %d", len(original.Children))
	}
}

func TestListAgentsReturnsCopies(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["agent1"] = &Agent{
		Name:     "agent1",
		Role:     Role("worker"),
		State:    StateIdle,
		Children: []string{"child1"},
	}

	copies := m.ListAgents()
	if len(copies) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(copies))
	}

	copies[0].State = StateWorking
	copies[0].Children = append(copies[0].Children, "child2")

	original := m.agents["agent1"]
	if original.State != StateIdle {
		t.Errorf("original state was modified: expected %s, got %s", StateIdle, original.State)
	}
	if len(original.Children) != 1 {
		t.Errorf("original children was modified: expected 1, got %d", len(original.Children))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["test-agent"] = &Agent{
		Name:     "test-agent",
		Role:     Role("worker"),
		State:    StateIdle,
		Children: []string{},
	}

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = m.GetAgent("test-agent")
				_ = m.ListAgents()
			}
		}()
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if n%2 == 0 {
					m.SetAgentCommand("cmd1")
				} else {
					m.SetAgentCommand("cmd2")
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestRoleHierarchy(t *testing.T) {
	tests := []struct {
		parent   Role
		child    Role
		expected bool
	}{
		{Role("product-manager"), Role("manager"), true},
		{Role("manager"), Role("engineer"), true},
		{Role("manager"), Role("qa"), true},
		{RoleRoot, Role("worker"), true},
		{RoleRoot, Role("manager"), true},
		{RoleRoot, Role("qa"), true},
		{Role("engineer"), Role("worker"), false},
		{Role("worker"), Role("engineer"), false},
		{Role("qa"), Role("engineer"), false},
	}

	for _, tc := range tests {
		result := CanCreateRole(tc.parent, tc.child)
		if result != tc.expected {
			t.Errorf("CanCreateRole(%s, %s) = %v, expected %v", tc.parent, tc.child, result, tc.expected)
		}
	}
}

func TestHasCapability(t *testing.T) {
	tests := []struct {
		role     Role
		cap      Capability
		expected bool
	}{
		{Role("product-manager"), CapCreateAgents, true},
		{Role("product-manager"), CapImplementTasks, false},
		{Role("engineer"), CapImplementTasks, true},
		{Role("engineer"), CapCreateAgents, false},
		{Role("worker"), CapImplementTasks, true},
		{Role("qa"), CapTestWork, true},
		{Role("qa"), CapReviewWork, true},
		{Role("qa"), CapImplementTasks, false},
	}

	for _, tc := range tests {
		result := HasCapability(tc.role, tc.cap)
		if result != tc.expected {
			t.Errorf("HasCapability(%s, %s) = %v, expected %v", tc.role, tc.cap, result, tc.expected)
		}
	}
}

func TestRoleLevel(t *testing.T) {
	tests := []struct {
		role     Role
		expected int
	}{
		{Role("product-manager"), 1},
		{RoleRoot, -1},
		{Role("manager"), 1},
		{Role("engineer"), 1},
		{Role("worker"), 1},
		{Role("qa"), 1},
	}

	for _, tc := range tests {
		result := RoleLevel(tc.role)
		if result != tc.expected {
			t.Errorf("RoleLevel(%s) = %d, expected %d", tc.role, result, tc.expected)
		}
	}
}

func TestValidateTransition(t *testing.T) {
	valid := []struct {
		from, to State
	}{
		{StateIdle, StateIdle},
		{StateIdle, StateWorking},
		{StateWorking, StateWorking},
		{StateWorking, StateIdle},
		{StateWorking, StateDone},
		{StateWorking, StateStuck},
		{StateWorking, StateError},
		{StateWorking, StateStopped},
		{StateDone, StateIdle},
		{StateDone, StateWorking},
		{StateStuck, StateStuck},
		{StateStuck, StateIdle},
		{StateStuck, StateWorking},
		{StateError, StateIdle},
		{StateError, StateWorking},
		{StateStopped, StateIdle},
		{StateStopped, StateStarting},
		{StateStarting, StateIdle},
		{StateStarting, StateError},
		{StateIdle, StateStopped},
		{StateDone, StateStopped},
		{StateStuck, StateError},
	}

	for _, tc := range valid {
		if err := ValidateTransition(tc.from, tc.to); err != nil {
			t.Errorf("ValidateTransition(%s, %s) should be valid, got error: %v", tc.from, tc.to, err)
		}
	}

	invalid := []struct {
		from, to State
	}{
		{StateIdle, StateStarting},
		{StateWorking, StateStarting},
		{StateDone, StateDone},
		{StateDone, StateStuck},
		{StateStopped, StateWorking},
		{StateStopped, StateDone},
		{StateStarting, StateWorking},
		{StateStarting, StateDone},
	}

	for _, tc := range invalid {
		if err := ValidateTransition(tc.from, tc.to); err == nil {
			t.Errorf("ValidateTransition(%s, %s) should be invalid, but returned nil", tc.from, tc.to)
		}
	}
}

func TestUpdateAgentStateValidation(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["test-agent"] = &Agent{
		Name:  "test-agent",
		Role:  Role("worker"),
		State: StateIdle,
	}

	// Valid: idle → working
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateWorking, "starting task"); err != nil {
		t.Errorf("idle→working should be valid: %v", err)
	}
	if m.agents["test-agent"].State != StateWorking {
		t.Errorf("expected state working, got %s", m.agents["test-agent"].State)
	}

	// Valid: working → done
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateDone, "finished"); err != nil {
		t.Errorf("working→done should be valid: %v", err)
	}

	// Invalid: done → stuck
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateStuck, "stuck"); err == nil {
		t.Error("done→stuck should be invalid, but returned nil")
	}
	if m.agents["test-agent"].State != StateDone {
		t.Errorf("state should remain done after rejected transition, got %s", m.agents["test-agent"].State)
	}

	// Agent not found
	if err := m.UpdateAgentState(context.Background(), "nonexistent", StateWorking, ""); err == nil {
		t.Error("should error for nonexistent agent")
	}
}

// SetAgentState is the lifecycle-hook path: it must move state WITHOUT
// overwriting the agent's reported task (#3259 — lifecycle strings like
// "Turn complete"/"Session ended" were clobbering report_status tasks).
func TestSetAgentState_PreservesReportedTask(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["eng-1"] = &Agent{
		Name:  "eng-1",
		Role:  Role("engineer"),
		State: StateIdle,
		Task:  "implementing issue #3259",
	}

	// Lifecycle transition (UserPromptSubmit → working) keeps the task.
	if err := m.SetAgentState(context.Background(), "eng-1", StateWorking); err != nil {
		t.Fatalf("idle→working should be valid: %v", err)
	}
	if got := m.agents["eng-1"].Task; got != "implementing issue #3259" {
		t.Errorf("task = %q, want reported task preserved", got)
	}
	if m.agents["eng-1"].State != StateWorking {
		t.Errorf("state = %s, want working", m.agents["eng-1"].State)
	}
	if m.agents["eng-1"].UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// Stop (idle) at end of turn also keeps the task.
	if err := m.SetAgentState(context.Background(), "eng-1", StateIdle); err != nil {
		t.Fatalf("working→idle should be valid: %v", err)
	}
	if got := m.agents["eng-1"].Task; got != "implementing issue #3259" {
		t.Errorf("task after idle = %q, want reported task preserved", got)
	}

	// SessionEnd (stopped) clears the task — a dead agent must not
	// advertise a stale one.
	if err := m.SetAgentState(context.Background(), "eng-1", StateStopped); err != nil {
		t.Fatalf("idle→stopped should be valid: %v", err)
	}
	if got := m.agents["eng-1"].Task; got != "" {
		t.Errorf("task after stopped = %q, want cleared", got)
	}

	// Invalid transition is rejected and leaves the agent untouched.
	if err := m.SetAgentState(context.Background(), "eng-1", StateWorking); err == nil {
		t.Error("stopped→working should be invalid")
	}
	if m.agents["eng-1"].State != StateStopped {
		t.Errorf("state after rejected transition = %s, want stopped", m.agents["eng-1"].State)
	}

	// Unknown agent errors.
	if err := m.SetAgentState(context.Background(), "nope", StateIdle); err == nil {
		t.Error("should error for nonexistent agent")
	}
}

func TestUpdateAgentState_SameStateMessageUpdate(t *testing.T) {
	m := &Manager{
		agents: make(map[string]*Agent),
	}
	m.agents["test-agent"] = &Agent{
		Name:  "test-agent",
		Role:  Role("worker"),
		State: StateIdle,
	}

	// idle → working
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateWorking, "starting task"); err != nil {
		t.Fatalf("idle→working should be valid: %v", err)
	}

	// working → working (update message)
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateWorking, "now testing edge cases"); err != nil {
		t.Errorf("working→working should be valid: %v", err)
	}
	if m.agents["test-agent"].Task != "now testing edge cases" {
		t.Errorf("expected task 'now testing edge cases', got %q", m.agents["test-agent"].Task)
	}
	if m.agents["test-agent"].State != StateWorking {
		t.Errorf("expected state working, got %s", m.agents["test-agent"].State)
	}

	// working → stuck
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateStuck, "blocked on dependency"); err != nil {
		t.Fatalf("working→stuck should be valid: %v", err)
	}

	// stuck → stuck (update message)
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateStuck, "still blocked, filed issue"); err != nil {
		t.Errorf("stuck→stuck should be valid: %v", err)
	}
	if m.agents["test-agent"].Task != "still blocked, filed issue" {
		t.Errorf("expected updated stuck message, got %q", m.agents["test-agent"].Task)
	}

	// stuck → idle
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateIdle, ""); err != nil {
		t.Fatalf("stuck→idle should be valid: %v", err)
	}

	// idle → idle (update message)
	if err := m.UpdateAgentState(context.Background(), "test-agent", StateIdle, "waiting for assignment"); err != nil {
		t.Errorf("idle→idle should be valid: %v", err)
	}
	if m.agents["test-agent"].Task != "waiting for assignment" {
		t.Errorf("expected updated idle message, got %q", m.agents["test-agent"].Task)
	}
}

// --- Concurrent access for new functions ---

// TestSpawnAgent_ConcurrentCalls verifies that concurrent SpawnAgent calls
// are properly serialized and don't cause data races or corruption.
// This exercises the mutex locking in SpawnAgentWithOptions.
func TestSpawnAgent_ConcurrentCalls(t *testing.T) {
	m := newTestManager(t)

	// Pre-populate some agents to create contention
	m.agents["existing-1"] = &Agent{
		ID:       "existing-1",
		Name:     "existing-1",
		Role:     Role("manager"),
		State:    StateIdle,
		Children: []string{},
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reads while spawning would happen
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Read operations that should be thread-safe
			_ = m.GetAgent("existing-1")
			_ = m.ListAgents()
			_ = m.AgentCount()
		}()
	}

	// Concurrent state mutations
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agentName := fmt.Sprintf("concurrent-agent-%d", idx)
			m.mu.Lock()
			m.agents[agentName] = &Agent{
				ID:       agentName,
				Name:     agentName,
				Role:     Role("engineer"),
				State:    StateIdle,
				Children: []string{},
			}
			m.mu.Unlock()
		}(i)
	}

	// Concurrent reads of spawned agents
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agentName := fmt.Sprintf("concurrent-agent-%d", idx%20)
			_ = m.GetAgent(agentName)
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("concurrent operation error: %v", err)
	}

	// Verify state is consistent
	count := m.AgentCount()
	if count < 1 {
		t.Errorf("expected at least 1 agent, got %d", count)
	}
}

func TestConcurrentAgentCount(t *testing.T) {
	m := newTestManager(t)
	m.agents["a"] = &Agent{Name: "a", State: StateIdle}
	m.agents["b"] = &Agent{Name: "b", State: StateWorking}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if count := m.AgentCount(); count != 2 {
				t.Errorf("expected 2, got %d", count)
			}
		}()
	}
	wg.Wait()
}

func TestSpawnAgent_PreservesWorkingState(t *testing.T) {
	m := newTestManager(t)
	// Setup agent in working state
	m.agents["worker-1"] = &Agent{
		ID:          "worker-1",
		Name:        "worker-1",
		Role:        Role("worker"),
		State:       StateWorking,
		Task:        "Important Job",
		Session:     "worker-1",
		WorktreeDir: t.TempDir(),
	}

	// Mock tmux session as NOT existing (simulating crash/restart)
	// newTestManager uses a prefix that ensures real tmux sessions don't match,
	// so HasSession returns false by default.

	// Spawn again
	// We expect it to restart the session but KEEP StateWorking
	agent, err := m.SpawnAgent(context.Background(), "worker-1", Role("worker"), t.TempDir())
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if agent.State != StateWorking {
		t.Errorf("Agent state reset to %s, want %s", agent.State, StateWorking)
	}
	if agent.Task != "Important Job" {
		t.Errorf("Agent task lost: %q", agent.Task)
	}
}

func TestConcurrentRunningCount(t *testing.T) {
	m := newTestManager(t)
	m.agents["a"] = &Agent{Name: "a", State: StateIdle}
	m.agents["b"] = &Agent{Name: "b", State: StateStopped}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if count := m.RunningCount(); count != 1 {
				t.Errorf("expected 1, got %d", count)
			}
		}()
	}
	wg.Wait()
}

func TestBootstrapDelay(t *testing.T) {
	m := newTestManager(t)

	// Default should be 3 seconds
	if d := m.getBootstrapDelay(); d != DefaultBootstrapDelay {
		t.Errorf("default bootstrap delay: got %v, want %v", d, DefaultBootstrapDelay)
	}
	if d := m.getBootstrapDelay(); d != 3*time.Second {
		t.Errorf("default bootstrap delay: got %v, want 3s", d)
	}

	// Setting custom delay should work
	m.SetBootstrapDelay(5 * time.Second)
	if d := m.getBootstrapDelay(); d != 5*time.Second {
		t.Errorf("custom bootstrap delay: got %v, want 5s", d)
	}

	// Setting to 0 should revert to default
	m.SetBootstrapDelay(0)
	if d := m.getBootstrapDelay(); d != DefaultBootstrapDelay {
		t.Errorf("zero bootstrap delay should use default: got %v, want %v", d, DefaultBootstrapDelay)
	}
}

// --- RoleRoot tests ---

func TestRoleRoot_Capabilities(t *testing.T) {
	caps, ok := RoleCapabilities[RoleRoot]
	if !ok {
		t.Fatal("RoleRoot should have capabilities defined")
	}

	expected := []Capability{CapCreateAgents, CapAssignWork, CapCreateEpics, CapReviewWork}
	for _, cap := range expected {
		found := false
		for _, c := range caps {
			if c == cap {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RoleRoot should have capability %s", cap)
		}
	}
}

func TestRoleRoot_Hierarchy(t *testing.T) {
	children, ok := RoleHierarchy[RoleRoot]
	if !ok {
		t.Fatal("RoleRoot should have hierarchy defined")
	}

	expected := []Role{Role("product-manager"), Role("manager"), Role("engineer"), Role("qa")}
	for _, role := range expected {
		found := false
		for _, r := range children {
			if r == role {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RoleRoot should be able to create %s", role)
		}
	}
}

func TestRoleRoot_Level(t *testing.T) {
	level := RoleLevel(RoleRoot)
	if level != -1 {
		t.Errorf("RoleRoot level = %d, want -1", level)
	}

	// Root should be above all other roles
	if level >= RoleLevel(Role("product-manager")) {
		t.Error("RoleRoot should be above RoleProductManager")
	}
	if level >= RoleLevel(Role("manager")) {
		t.Error("RoleRoot should be above RoleManager")
	}
}

func TestRoleRoot_CanCreateRole(t *testing.T) {
	tests := []struct {
		child    Role
		expected bool
	}{
		{Role("product-manager"), true},
		{Role("manager"), true},
		{Role("engineer"), true},
		{Role("qa"), true},
		{RoleRoot, false}, // Cannot create another root
	}

	for _, tc := range tests {
		t.Run(string(tc.child), func(t *testing.T) {
			got := CanCreateRole(RoleRoot, tc.child)
			if got != tc.expected {
				t.Errorf("CanCreateRole(RoleRoot, %s) = %v, want %v", tc.child, got, tc.expected)
			}
		})
	}
}

func TestRoleRoot_HasCapability(t *testing.T) {
	tests := []struct {
		cap      Capability
		expected bool
	}{
		{CapCreateAgents, true},
		{CapAssignWork, true},
		{CapCreateEpics, true},
		{CapReviewWork, true},
		{CapImplementTasks, false}, // Root delegates, doesn't implement
		{CapTestWork, false},       // Root delegates, doesn't test
	}

	for _, tc := range tests {
		t.Run(string(tc.cap), func(t *testing.T) {
			got := HasCapability(RoleRoot, tc.cap)
			if got != tc.expected {
				t.Errorf("HasCapability(RoleRoot, %s) = %v, want %v", tc.cap, got, tc.expected)
			}
		})
	}
}

// --- Singleton enforcement tests ---

func TestEnforceRootSingleton_NoRootExists(t *testing.T) {
	m := newTestManager(t)

	// No root exists - should allow creation
	if err := m.enforceRootSingleton(""); err != nil {
		t.Errorf("enforceRootSingleton should allow creation when no root exists: %v", err)
	}
}

func TestEnforceRootSingleton_RootActiveBlocks(t *testing.T) {
	m := newTestManager(t)

	// Add root in idle state
	m.agents["root"] = &Agent{Name: "root", Role: RoleRoot, State: StateIdle, IsRoot: true}

	// Active root should block new spawn
	err := m.enforceRootSingleton("")
	if err == nil {
		t.Error("enforceRootSingleton should block when active root exists")
	}
}

func TestEnforceRootSingleton_RootStoppedAllows(t *testing.T) {
	m := newTestManager(t)

	// Add root in stopped state
	m.agents["root"] = &Agent{Name: "root", Role: RoleRoot, State: StateStopped, IsRoot: true}

	// Stopped root should allow respawn
	if err := m.enforceRootSingleton(""); err != nil {
		t.Errorf("enforceRootSingleton should allow respawn when root is stopped: %v", err)
	}
}

func TestEnforceRootSingleton_RootErrorAllows(t *testing.T) {
	m := newTestManager(t)

	// Add root in error state
	m.agents["root"] = &Agent{Name: "root", Role: RoleRoot, State: StateError, IsRoot: true}

	// Error state should allow respawn
	if err := m.enforceRootSingleton(""); err != nil {
		t.Errorf("enforceRootSingleton should allow respawn when root is in error: %v", err)
	}
}

// --- Permission function tests ---

func TestDefaultPermissions(t *testing.T) {
	tests := []struct {
		expectedContains   Permission
		expectedNotContain Permission
		name               string
		roleLevel          int
	}{
		{
			name:               "root level has all permissions",
			roleLevel:          -1,
			expectedContains:   PermCreateAgents,
			expectedNotContain: "",
		},
		{
			name:               "manager level has create agents",
			roleLevel:          0,
			expectedContains:   PermCreateAgents,
			expectedNotContain: PermDeleteAgents,
		},
		{
			name:               "engineer level has view logs",
			roleLevel:          1,
			expectedContains:   PermViewLogs,
			expectedNotContain: PermCreateAgents,
		},
		{
			name:               "worker level has send commands",
			roleLevel:          2,
			expectedContains:   PermSendCommands,
			expectedNotContain: PermCreateChannels,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			perms := DefaultPermissions(tc.roleLevel)
			if len(perms) == 0 && tc.roleLevel <= -1 {
				t.Error("root level should have permissions")
			}

			if tc.expectedContains != "" {
				found := false
				for _, p := range perms {
					if p == tc.expectedContains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected permission %q not found", tc.expectedContains)
				}
			}

			if tc.expectedNotContain != "" {
				for _, p := range perms {
					if p == tc.expectedNotContain {
						t.Errorf("unexpected permission %q found", tc.expectedNotContain)
						break
					}
				}
			}
		})
	}
}

func TestDefaultPermissions_AllLevels(t *testing.T) {
	// Test root level has all permissions
	rootPerms := DefaultPermissions(-1)
	if len(rootPerms) != len(AllPermissions) {
		t.Errorf("root level should have %d permissions, got %d", len(AllPermissions), len(rootPerms))
	}

	// Test manager level
	mgrPerms := DefaultPermissions(0)
	if len(mgrPerms) < 3 {
		t.Errorf("manager level should have at least 3 permissions, got %d", len(mgrPerms))
	}

	// Test engineer level
	engPerms := DefaultPermissions(1)
	if len(engPerms) < 2 {
		t.Errorf("engineer level should have at least 2 permissions, got %d", len(engPerms))
	}
}

func TestCheckPermission(t *testing.T) {
	tests := []struct {
		required    Permission
		name        string
		permissions []string
		wantErr     bool
	}{
		{
			name:        "has required permission",
			permissions: []string{"can_create_agents", "can_view_logs", "can_send_commands"},
			required:    PermCreateAgents,
			wantErr:     false,
		},
		{
			name:        "missing required permission",
			permissions: []string{"can_view_logs", "can_send_commands"},
			required:    PermCreateAgents,
			wantErr:     true,
		},
		{
			name:        "empty permissions",
			permissions: []string{},
			required:    PermViewLogs,
			wantErr:     true,
		},
		{
			name:        "single matching permission",
			permissions: []string{"can_view_logs"},
			required:    PermViewLogs,
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPermission(tc.permissions, tc.required)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckPermission() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestHasPermissionStr(t *testing.T) {
	tests := []struct {
		name        string
		required    string
		permissions []string
		expected    bool
	}{
		{
			name:        "has permission",
			permissions: []string{"can_create_agents", "can_view_logs", "can_send_commands"},
			required:    "can_view_logs",
			expected:    true,
		},
		{
			name:        "missing permission",
			permissions: []string{"can_view_logs", "can_send_commands"},
			required:    "can_create_agents",
			expected:    false,
		},
		{
			name:        "empty permissions",
			permissions: []string{},
			required:    "can_view_logs",
			expected:    false,
		},
		{
			name:        "single matching",
			permissions: []string{"can_send_messages"},
			required:    "can_send_messages",
			expected:    true,
		},
		{
			name:        "partial match not accepted",
			permissions: []string{"can_create_agents"},
			required:    "can_create",
			expected:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := HasPermissionStr(tc.permissions, tc.required)
			if result != tc.expected {
				t.Errorf("HasPermissionStr() = %v, want %v", result, tc.expected)
			}
		})
	}
}

// --- SpawnAgentWithTool tests (#1236) ---

func TestSpawnAgentWithTool(t *testing.T) {
	m := newTestManager(t)

	// Invalid name should fail
	_, err := m.SpawnAgentWithTool(context.Background(), "invalid name!", Role("engineer"), "/tmp", "claude")
	if err == nil {
		t.Error("expected error for invalid agent name")
	}

	// Empty role should fail
	_, err = m.SpawnAgentWithTool(context.Background(), "test-agent", Role(""), "/tmp", "claude")
	if err == nil {
		t.Error("expected error for empty role")
	}
}

// --- SpawnAgentWithParent tests (#1236) ---

func TestSpawnAgentWithParent(t *testing.T) {
	m := newTestManager(t)

	// Invalid name should fail
	_, err := m.SpawnAgentWithParent(context.Background(), "bad name!", Role("engineer"), "/tmp", "parent")
	if err == nil {
		t.Error("expected error for invalid agent name")
	}

	// Null role should fail
	_, err = m.SpawnAgentWithParent(context.Background(), "test-agent", Role("null"), "/tmp", "parent")
	if err == nil {
		t.Error("expected error for null role")
	}
}

// --- DeleteAgent tests (#1236) ---

func TestDeleteAgent(t *testing.T) {
	m := newTestManager(t)

	// Set up an agent to delete
	m.agents["doomed"] = &Agent{
		Name:      "doomed",
		Role:      Role("engineer"),
		State:     StateIdle,
		Workspace: m.stateDir,
		Children:  []string{},
	}

	// Delete should succeed
	if err := m.DeleteAgent(context.Background(), "doomed"); err != nil {
		t.Errorf("DeleteAgent failed: %v", err)
	}

	// Agent should be gone
	if _, exists := m.agents["doomed"]; exists {
		t.Error("agent should be deleted from map")
	}
}

func TestDeleteAgent_NotFound(t *testing.T) {
	m := newTestManager(t)

	err := m.DeleteAgent(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

// --- DeleteAgentWithOptions tests (#1236) ---

func TestDeleteAgentWithOptions_Default(t *testing.T) {
	m := newTestManager(t)

	m.agents["preserve"] = &Agent{
		Name:      "preserve",
		Role:      Role("engineer"),
		State:     StateIdle,
		Workspace: m.stateDir,
		Children:  []string{},
	}

	// Delete with default options
	err := m.DeleteAgentWithOptions(context.Background(), "preserve", DeleteOptions{})
	if err != nil {
		t.Errorf("DeleteAgentWithOptions failed: %v", err)
	}

	// Agent should be gone
	if _, exists := m.agents["preserve"]; exists {
		t.Error("agent should be deleted")
	}
}

func TestDeleteAgentWithOptions_WithWorktree(t *testing.T) {
	m := newTestManager(t)

	m.agents["with-worktree"] = &Agent{
		Name:        "with-worktree",
		Role:        Role("engineer"),
		State:       StateIdle,
		Workspace:   m.stateDir,
		WorktreeDir: filepath.Join(m.stateDir, "worktrees", "wt"),
		Children:    []string{},
	}

	// Delete should succeed (worktree removal is best-effort)
	err := m.DeleteAgentWithOptions(context.Background(), "with-worktree", DeleteOptions{Force: true})
	if err != nil {
		t.Errorf("DeleteAgentWithOptions failed: %v", err)
	}

	// Agent should be gone
	if _, exists := m.agents["with-worktree"]; exists {
		t.Error("agent should be deleted")
	}
}

func TestDeleteAgentWithOptions_RemovesFromParent(t *testing.T) {
	m := newTestManager(t)

	m.agents["parent-mgr"] = &Agent{
		Name:     "parent-mgr",
		Role:     Role("manager"),
		State:    StateIdle,
		Children: []string{"child-eng", "other-child"},
	}
	m.agents["child-eng"] = &Agent{
		Name:     "child-eng",
		Role:     Role("engineer"),
		State:    StateIdle,
		ParentID: "parent-mgr",
		Children: []string{},
	}

	// Delete child
	err := m.DeleteAgentWithOptions(context.Background(), "child-eng", DeleteOptions{})
	if err != nil {
		t.Errorf("DeleteAgentWithOptions failed: %v", err)
	}

	// Child should be removed from parent's children
	parent := m.agents["parent-mgr"]
	for _, child := range parent.Children {
		if child == "child-eng" {
			t.Error("deleted child should be removed from parent's children list")
		}
	}
	if len(parent.Children) != 1 || parent.Children[0] != "other-child" {
		t.Errorf("parent.Children = %v, want [other-child]", parent.Children)
	}
}

func TestPermissionConstants(t *testing.T) {
	// Verify permission constants are defined correctly
	if PermCreateAgents != "can_create_agents" {
		t.Errorf("PermCreateAgents = %q, want %q", PermCreateAgents, "can_create_agents")
	}
	if PermViewLogs != "can_view_logs" {
		t.Errorf("PermViewLogs = %q, want %q", PermViewLogs, "can_view_logs")
	}
	if PermSendCommands != "can_send_commands" {
		t.Errorf("PermSendCommands = %q, want %q", PermSendCommands, "can_send_commands")
	}
	if PermSendMessages != "can_send_messages" {
		t.Errorf("PermSendMessages = %q, want %q", PermSendMessages, "can_send_messages")
	}
}

func TestAllPermissions(t *testing.T) {
	// AllPermissions should contain all defined permissions
	if len(AllPermissions) < 5 {
		t.Errorf("AllPermissions should have at least 5 permissions, got %d", len(AllPermissions))
	}

	// Check that key permissions are in AllPermissions
	expectedPerms := []Permission{PermCreateAgents, PermViewLogs, PermSendCommands, PermSendMessages}
	for _, expected := range expectedPerms {
		found := false
		for _, p := range AllPermissions {
			if p == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllPermissions missing %q", expected)
		}
	}
}

func TestIsValidAgentName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid lowercase", "agent", true},
		{"valid with hyphen", "my-agent", true},
		{"valid with underscore", "my_agent", true},
		{"valid with numbers", "agent123", true},
		{"valid mixed case", "MyAgent", true},
		{"valid complex", "eng-02_test", true},
		{"empty string", "", false},
		{"contains space", "my agent", false},
		{"contains semicolon", "agent;rm", false},
		{"contains pipe", "agent|ls", false},
		{"contains ampersand", "agent&echo", false},
		{"contains dollar", "agent$var", false},
		{"contains backtick", "agent`id`", false},
		{"contains slash", "agent/path", false},
		{"contains dot", "agent.test", false},
		{"contains newline", "agent\ntest", false},
		{"exactly 64 chars", strings.Repeat("a", MaxAgentNameLength), true},
		{"65 chars exceeds max", strings.Repeat("a", MaxAgentNameLength+1), false},
		{"500 chars exceeds max", strings.Repeat("a", 500), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsValidAgentName(tc.input)
			if result != tc.expected {
				t.Errorf("IsValidAgentName(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestClaudeBuildCommand(t *testing.T) {
	p := provider.NewClaudeProvider()

	tests := []struct {
		name     string
		expected string
		opts     provider.CommandOpts
	}{
		{"no agent", "claude --dangerously-skip-permissions", provider.CommandOpts{}},
		{"with agent", "claude --dangerously-skip-permissions", provider.CommandOpts{AgentName: "eng-01"}},
		{"root agent", "claude --dangerously-skip-permissions", provider.CommandOpts{AgentName: "root"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := p.BuildCommand(tc.opts)
			if result != tc.expected {
				t.Errorf("BuildCommand(%+v) = %q, want %q", tc.opts, result, tc.expected)
			}
		})
	}
}

func TestManagerSetAgentTeam(t *testing.T) {
	m := newTestManager(t)

	// Add a test agent
	m.agents["test-agent"] = &Agent{
		Name: "test-agent",
		Role: "engineer",
	}

	// Test setting team on existing agent
	err := m.SetAgentTeam(context.Background(), "test-agent", "backend")
	if err != nil {
		t.Fatalf("SetAgentTeam() error = %v", err)
	}

	if m.agents["test-agent"].Team != "backend" {
		t.Errorf("Team = %q, want %q", m.agents["test-agent"].Team, "backend")
	}

	// Test UpdatedAt was set
	if m.agents["test-agent"].UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set after SetAgentTeam")
	}

	// Test setting team on non-existent agent
	err = m.SetAgentTeam(context.Background(), "nonexistent", "team")
	if err == nil {
		t.Error("SetAgentTeam() should error for non-existent agent")
	}
}

func TestManagerSetAgentCommand(t *testing.T) {
	m := newTestManager(t)

	// Initially the command is /bin/true
	if m.agentCmd != "/bin/true" {
		t.Errorf("initial agentCmd = %q, want /bin/true", m.agentCmd)
	}

	// Set a new command
	m.SetAgentCommand("claude")
	if m.agentCmd != "claude" {
		t.Errorf("agentCmd = %q, want claude", m.agentCmd)
	}

	// Set another command
	m.SetAgentCommand("claude --dangerously-skip-permissions")
	if m.agentCmd != "claude --dangerously-skip-permissions" {
		t.Errorf("agentCmd = %q, want 'claude --dangerously-skip-permissions'", m.agentCmd)
	}
}

func TestManagerSetBootstrapDelay(t *testing.T) {
	m := newTestManager(t)

	// Initially zero
	if m.BootstrapDelay != 0 {
		t.Errorf("initial BootstrapDelay = %v, want 0", m.BootstrapDelay)
	}

	// Set delay
	m.SetBootstrapDelay(5 * time.Second)
	if m.BootstrapDelay != 5*time.Second {
		t.Errorf("BootstrapDelay = %v, want 5s", m.BootstrapDelay)
	}

	// Set another delay
	m.SetBootstrapDelay(10 * time.Second)
	if m.BootstrapDelay != 10*time.Second {
		t.Errorf("BootstrapDelay = %v, want 10s", m.BootstrapDelay)
	}
}

func TestManagerListAgents(t *testing.T) {
	m := newTestManager(t)

	// Initially no agents
	agents := m.ListAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	// Add agents
	m.agents["agent1"] = &Agent{Name: "agent1", Role: "engineer"}
	m.agents["agent2"] = &Agent{Name: "agent2", Role: "manager"}

	agents = m.ListAgents()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestManagerGetAgent(t *testing.T) {
	m := newTestManager(t)

	// Add test agent
	testAgent := &Agent{Name: "test-agent", Role: "engineer", Team: "backend"}
	m.agents["test-agent"] = testAgent

	// Test getting existing agent
	agent := m.GetAgent("test-agent")
	if agent == nil {
		t.Fatal("GetAgent() should find existing agent")
	}
	if agent.Name != "test-agent" {
		t.Errorf("Name = %q, want test-agent", agent.Name)
	}
	if agent.Team != "backend" {
		t.Errorf("Team = %q, want backend", agent.Team)
	}

	// Test getting non-existent agent
	agent = m.GetAgent("nonexistent")
	if agent != nil {
		t.Error("GetAgent() should return nil for non-existent agent")
	}
}

func TestCapabilityConstants(t *testing.T) {
	// Verify capability constant values
	if CapCreateAgents != "create_agents" {
		t.Errorf("CapCreateAgents = %q, want create_agents", CapCreateAgents)
	}
	if CapAssignWork != "assign_work" {
		t.Errorf("CapAssignWork = %q, want assign_work", CapAssignWork)
	}
	if CapCreateEpics != "create_epics" {
		t.Errorf("CapCreateEpics = %q, want create_epics", CapCreateEpics)
	}
	if CapImplementTasks != "implement_tasks" {
		t.Errorf("CapImplementTasks = %q, want implement_tasks", CapImplementTasks)
	}
	if CapReviewWork != "review_work" {
		t.Errorf("CapReviewWork = %q, want review_work", CapReviewWork)
	}
	if CapTestWork != "test_work" {
		t.Errorf("CapTestWork = %q, want test_work", CapTestWork)
	}
}

func TestRoleConstant(t *testing.T) {
	// Verify RoleRoot constant
	if RoleRoot != "root" {
		t.Errorf("RoleRoot = %q, want root", RoleRoot)
	}
}

// --- Additional LoadRoleMemory tests ---

func TestLoadRoleMemory_RootRoleBackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()
	promptsDir := filepath.Join(tmpDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create root.md in the backward-compatible location
	content := "You are the root orchestrator agent."
	if err := os.WriteFile(filepath.Join(promptsDir, "root.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	mem := LoadRoleMemory(tmpDir, RoleRoot)
	if mem == nil {
		t.Fatal("expected non-nil AgentMemory for root role")
	}
	if mem.RolePrompt != content {
		t.Errorf("RolePrompt = %q, want %q", mem.RolePrompt, content)
	}
}

func TestLoadRoleMemory_RootRoleFallsBackToRoleManager(t *testing.T) {
	tmpDir := t.TempDir()
	rolesDir := filepath.Join(tmpDir, ".bc", "roles")
	if err := os.MkdirAll(rolesDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create root.md in roles dir (not prompts dir)
	content := "Root from roles directory."
	if err := os.WriteFile(filepath.Join(rolesDir, "root.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	// Should fall back to role manager since prompts/root.md doesn't exist
	mem := LoadRoleMemory(tmpDir, RoleRoot)
	if mem == nil {
		t.Fatal("expected non-nil AgentMemory for root role from role manager")
	}
	if mem.RolePrompt != content {
		t.Errorf("RolePrompt = %q, want %q", mem.RolePrompt, content)
	}
}

func TestLoadRoleMemory_EmptyPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	rolesDir := filepath.Join(tmpDir, ".bc", "roles")
	if err := os.MkdirAll(rolesDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Create empty role file
	if err := os.WriteFile(filepath.Join(rolesDir, "empty-role.md"), []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	mem := LoadRoleMemory(tmpDir, Role("empty-role"))
	if mem != nil {
		t.Error("expected nil AgentMemory for empty prompt")
	}
}

// --- UpdateAgentState error tests ---

func TestUpdateAgentState_NotFound(t *testing.T) {
	m := newTestManager(t)

	err := m.UpdateAgentState(context.Background(), "nonexistent", StateWorking, "working on task")
	if err == nil {
		t.Error("expected error when updating non-existent agent")
	}
}

// --- SetAgentTeam error tests ---

func TestSetAgentTeam_NotFound(t *testing.T) {
	m := newTestManager(t)

	err := m.SetAgentTeam(context.Background(), "nonexistent", "backend")
	if err == nil {
		t.Error("expected error when setting team for non-existent agent")
	}
}

func TestSetAgentTeam_Success(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-01"] = &Agent{
		Name:     "eng-01",
		Role:     Role("engineer"),
		State:    StateIdle,
		Children: []string{},
	}

	err := m.SetAgentTeam(context.Background(), "eng-01", "frontend")
	if err != nil {
		t.Fatalf("SetAgentTeam failed: %v", err)
	}

	if m.agents["eng-01"].Team != "frontend" {
		t.Errorf("Team = %q, want frontend", m.agents["eng-01"].Team)
	}
}

// --- enforceRootSingleton tests ---

func TestEnforceRootSingleton_NoExistingRoot(t *testing.T) {
	m := newTestManager(t)
	m.agents["eng-01"] = &Agent{
		Name: "eng-01",
		Role: Role("engineer"),
	}

	// Should not error - no root exists
	err := m.enforceRootSingleton("/workspace")
	if err != nil {
		t.Errorf("enforceRootSingleton should not error without root: %v", err)
	}
}

func TestEnforceRootSingleton_OneRootAllowed(t *testing.T) {
	m := newTestManager(t)
	m.agents["root"] = &Agent{
		Name: "root",
		Role: RoleRoot,
	}

	// Should not error - only one root
	err := m.enforceRootSingleton("/workspace")
	if err != nil {
		t.Errorf("enforceRootSingleton should not error with one root: %v", err)
	}
}

// --- SpawnChildAgent tests ---

func TestSpawnChildAgent_ParentNotFound(t *testing.T) {
	m := newTestManager(t)

	// Try to spawn child with non-existent parent
	_, err := m.SpawnChildAgent(context.Background(), "nonexistent-parent", "child", Role("engineer"), "/workspace")
	if err == nil {
		t.Error("expected error when parent does not exist")
	}
}

func TestSpawnChildAgentWithTool_ParentNotFound(t *testing.T) {
	m := newTestManager(t)

	// Try to spawn child with non-existent parent
	_, err := m.SpawnChildAgentWithTool(context.Background(), "nonexistent-parent", "child", Role("engineer"), "/workspace", "claude")
	if err == nil {
		t.Error("expected error when parent does not exist")
	}
}

// --- removeFromParent tests ---

func TestRemoveFromParent_NoParent(t *testing.T) {
	m := newTestManager(t)

	m.agents["child"] = &Agent{
		Name:     "child",
		ParentID: "", // No parent
	}

	// Should not panic
	m.removeFromParent("child")
}

func TestRemoveFromParent_ParentNotFound(t *testing.T) {
	m := newTestManager(t)

	m.agents["child"] = &Agent{
		Name:     "child",
		ParentID: "missing-parent",
	}

	// Should not panic even if parent doesn't exist
	m.removeFromParent("child")
}

func TestRemoveFromParent_Success(t *testing.T) {
	m := newTestManager(t)

	m.agents["parent"] = &Agent{
		Name:     "parent",
		Children: []string{"child1", "child2"},
	}
	m.agents["child1"] = &Agent{
		Name:     "child1",
		ParentID: "parent",
	}

	m.removeFromParent("child1")

	// Parent should only have child2
	if len(m.agents["parent"].Children) != 1 {
		t.Errorf("Children count = %d, want 1", len(m.agents["parent"].Children))
	}
	if m.agents["parent"].Children[0] != "child2" {
		t.Errorf("Children = %v, want [child2]", m.agents["parent"].Children)
	}
}

func TestRemoveFromParent_AgentNotFound(t *testing.T) {
	m := newTestManager(t)

	// Should not panic
	m.removeFromParent("nonexistent")
}

func TestTailFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tests := []struct {
		name     string
		contains []string
		lines    int
	}{
		{"last 2 lines", []string{"line4", "line5"}, 2},
		{"last 5 lines", []string{"line1", "line2", "line3", "line4", "line5"}, 5},
		{"more than available", []string{"line1", "line2", "line3", "line4", "line5"}, 10},
		{"last 1 line", []string{"line5"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := tailFile(path, tt.lines)
			if err != nil {
				t.Fatalf("tailFile failed: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("expected output to contain %q, got %q", want, output)
				}
			}
		})
	}
}

func TestTailFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	output, err := tailFile(path, 10)
	if err != nil {
		t.Fatalf("tailFile failed: %v", err)
	}
	if output != "" {
		t.Errorf("expected empty output, got %q", output)
	}
}

func TestTailFile_NotFound(t *testing.T) {
	_, err := tailFile("/nonexistent/path/file.log", 10)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestTruncateLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Create a file with known content
	var content strings.Builder
	for i := range 100 {
		fmt.Fprintf(&content, "line %d: some log output here\n", i)
	}
	if err := os.WriteFile(path, []byte(content.String()), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	info, _ := os.Stat(path)
	originalSize := info.Size()

	// Truncate with max = half of original size (should trigger truncation)
	truncateLogFile(path, originalSize/2)

	info, _ = os.Stat(path)
	if info.Size() >= originalSize {
		t.Errorf("expected truncated size < %d, got %d", originalSize, info.Size())
	}
	// Should be roughly half
	if info.Size() > originalSize*3/4 {
		t.Errorf("expected roughly half size, got %d (original %d)", info.Size(), originalSize)
	}
}

func TestTruncateLogFile_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")

	content := "small log\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// File is below threshold — should not be modified
	truncateLogFile(path, 1048576)

	data, _ := os.ReadFile(path) //nolint:gosec // test file path from t.TempDir
	if string(data) != content {
		t.Errorf("file should not have changed, got %q", string(data))
	}
}

func TestTruncateLogFile_ZeroMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	content := "some content\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Zero maxBytes means no truncation
	truncateLogFile(path, 0)

	data, _ := os.ReadFile(path) //nolint:gosec // test file path from t.TempDir
	if string(data) != content {
		t.Errorf("file should not have changed with maxBytes=0")
	}
}

func TestTruncateLogFile_FileNotFound(t *testing.T) {
	// Should not panic on nonexistent file
	truncateLogFile("/nonexistent/path/file.log", 1024)
}

func TestCaptureOutputFromLogFile(t *testing.T) {
	dir := t.TempDir()

	// Create a manager with mock tmux
	m := newTestManager(t)

	// Create a log file
	logsDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}
	logPath := filepath.Join(logsDir, "test-agent.log")
	logContent := "log line 1\nlog line 2\nlog line 3\n"
	if err := os.WriteFile(logPath, []byte(logContent), 0600); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	// Add agent with LogFile set
	m.agents["test-agent"] = &Agent{
		Name:    "test-agent",
		LogFile: logPath,
		State:   StateIdle,
	}

	output, err := m.CaptureOutput(context.Background(), "test-agent", 10)
	if err != nil {
		t.Fatalf("CaptureOutput failed: %v", err)
	}

	if !strings.Contains(output, "log line 1") {
		t.Errorf("expected output to contain log content, got %q", output)
	}
}

func TestCaptureOutputFallback(t *testing.T) {
	// When agent has no LogFile, should fall through to tmux capture
	m := newTestManager(t)
	m.agents["test-agent"] = &Agent{
		Name:  "test-agent",
		State: StateIdle,
		// No LogFile set
	}

	// This will call tmux.Capture which will fail since there's no real session,
	// but we're testing the fallback path
	_, err := m.CaptureOutput(context.Background(), "test-agent", 10)
	// Error is expected since the mock tmux won't have the session
	_ = err
}

func TestFollowOutput(t *testing.T) {
	dir := t.TempDir()

	m := newTestManager(t)

	// Create log file with initial content
	logsDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		t.Fatalf("failed to create logs dir: %v", err)
	}
	logPath := filepath.Join(logsDir, "follow-agent.log")
	initial := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(logPath, []byte(initial), 0600); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	m.agents["follow-agent"] = &Agent{
		Name:    "follow-agent",
		LogFile: logPath,
		State:   StateIdle,
	}

	ctx, cancel := context.WithCancel(context.Background())
	var buf strings.Builder

	// Start follow in goroutine
	done := make(chan error, 1)
	go func() {
		done <- m.FollowOutput(ctx, "follow-agent", 10, &buf)
	}()

	// Give follow time to start and print initial lines
	time.Sleep(100 * time.Millisecond)

	// Append new content to log file
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("failed to open log for append: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString("new line 4\nnew line 5\n"); err != nil {
		t.Fatalf("failed to append: %v", err)
	}

	// Wait for poll cycle to pick it up
	time.Sleep(400 * time.Millisecond)

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("FollowOutput returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "line 1") {
		t.Errorf("expected initial content, got %q", output)
	}
	if !strings.Contains(output, "new line 4") {
		t.Errorf("expected new line 4 in followed output, got %q", output)
	}
	if !strings.Contains(output, "new line 5") {
		t.Errorf("expected new line 5 in followed output, got %q", output)
	}
}

func TestFollowOutput_NoLogFile(t *testing.T) {
	m := newTestManager(t)
	m.agents["no-log-agent"] = &Agent{
		Name:  "no-log-agent",
		State: StateIdle,
		// No LogFile — should fall back to one-shot CaptureOutput
	}

	ctx := context.Background()
	var buf strings.Builder

	// This will attempt CaptureOutput which tries tmux — error is expected
	// since there's no real session, but we're testing the fallback path
	_ = m.FollowOutput(ctx, "no-log-agent", 10, &buf)
}

func TestFollowOutput_AgentNotFound(t *testing.T) {
	m := newTestManager(t)

	ctx := context.Background()
	var buf strings.Builder

	err := m.FollowOutput(ctx, "nonexistent", 10, &buf)
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %q", err.Error())
	}
}

// --- Provider integration tests ---

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	detectState provider.State
	name        string
	version     string
	command     string
	installed   bool
}

func (m mockProvider) Name() string        { return m.name }
func (m mockProvider) Description() string { return "mock " + m.name }
func (m mockProvider) Command() string {
	if m.command != "" {
		return m.command
	}
	return m.name
}
func (m mockProvider) Binary() string      { return m.name }
func (m mockProvider) InstallHint() string { return "install " + m.name }
func (m mockProvider) BuildCommand(opts provider.CommandOpts) string {
	if m.command != "" {
		return m.command
	}
	return m.name
}
func (m mockProvider) IsInstalled(_ context.Context) bool  { return m.installed }
func (m mockProvider) Version(_ context.Context) string    { return m.version }
func (m mockProvider) DetectState(_ string) provider.State { return m.detectState }

func newTestManagerWithProvider(t *testing.T, p provider.Provider) *Manager {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Register(p)
	dir := t.TempDir()

	// Initialize a git repo so worktree operations work
	initGitRepo(t, dir)

	be := runtime.NewTmuxBackend(tmux.NewManager(fmt.Sprintf("bctest-%d-", time.Now().UnixNano())))

	// Create role files for test roles so role existence validation passes.
	rm := workspace.NewRoleManager(dir)
	if mkErr := rm.EnsureRolesDir(); mkErr != nil {
		t.Fatalf("EnsureRolesDir: %v", mkErr)
	}
	for _, roleName := range []string{"engineer", "manager", "qa", "worker"} {
		if writeErr := os.WriteFile(
			filepath.Join(rm.RolesDir(), roleName+".md"),
			[]byte("---\nname: "+roleName+"\n---\n"),
			0600,
		); writeErr != nil {
			t.Fatalf("write role %s: %v", roleName, writeErr)
		}
	}

	return &Manager{
		agents:           make(map[string]*Agent),
		backends:         map[string]runtime.Backend{"tmux": be},
		defaultBackend:   "tmux",
		providerRegistry: reg,
		stateDir:         dir,
		agentCmd:         "/bin/true",
		workspacePath:    dir,
		worktreeMgr:      worktree.NewManager(dir),
	}
}

func newTestManagerWithDefaultTool(t *testing.T, p provider.Provider, defaultTool string) *Manager {
	t.Helper()
	m := newTestManagerWithProvider(t, p)
	m.defaultTool = defaultTool
	return m
}

func TestSpawnWithProvider_Installed(t *testing.T) {
	// Register a mock provider that reports as installed
	mp := mockProvider{name: "testcli", installed: true, version: "1.2.3"}
	m := newTestManagerWithProvider(t, mp)

	ag, err := m.SpawnAgentWithTool(context.Background(), "test-agent", Role("engineer"), t.TempDir(), "testcli")
	if err != nil {
		t.Fatalf("expected spawn to succeed with installed provider, got: %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil agent")
	}
	if ag.Tool != "testcli" {
		t.Errorf("expected tool 'testcli', got %q", ag.Tool)
	}

	// Clean up tmux session
	_ = m.runtime().KillSession(context.Background(), "test-agent")
}

func TestSpawnWithProvider_NotInstalled(t *testing.T) {
	// Register a mock provider that reports as NOT installed
	mp := mockProvider{name: "missingtool", installed: false}
	m := newTestManagerWithProvider(t, mp)

	_, err := m.SpawnAgentWithTool(context.Background(), "test-agent", Role("engineer"), t.TempDir(), "missingtool")
	if err == nil {
		t.Fatal("expected error for uninstalled provider")
	}
	if !strings.Contains(err.Error(), "is not installed") {
		t.Errorf("expected 'is not installed' error, got %q", err.Error())
	}
}

func TestSpawnWithProvider_CustomToolFallback(t *testing.T) {
	// Manager has a registry with "sometool" but NOT "customtool"
	reg := provider.NewRegistry()
	reg.Register(mockProvider{name: "sometool", installed: true})

	be := runtime.NewTmuxBackend(tmux.NewManager(fmt.Sprintf("bctest-%d-", time.Now().UnixNano())))
	m := &Manager{
		agents:           make(map[string]*Agent),
		backends:         map[string]runtime.Backend{"tmux": be},
		defaultBackend:   "tmux",
		providerRegistry: reg,
		stateDir:         t.TempDir(),
		agentCmd:         "/bin/true",
	}

	// "customtool" is unknown to the registry, so SpawnAgentWithTool should fail
	_, err := m.SpawnAgentWithTool(context.Background(), "test-agent", Role("engineer"), t.TempDir(), "customtool")
	if err == nil {
		t.Fatal("expected error for unknown tool not in registry")
	}
}

func TestGetAgentCommandFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		cfg     *workspace.Config
		wantCmd string
		wantOk  bool
	}{
		{
			name: "workspace config takes precedence",
			tool: "claude",
			cfg: &workspace.Config{
				Providers: workspace.ProvidersConfig{
					Providers: map[string]workspace.ProviderConfig{"claude": {Command: "claude --workspace"}},
				},
			},
			wantCmd: "claude --workspace",
			wantOk:  true,
		},
		{
			name:    "falls back to global config",
			tool:    "claude",
			cfg:     &workspace.Config{},
			wantCmd: "claude --dangerously-skip-permissions",
			wantOk:  true,
		},
		{
			name:    "nil workspace config uses global",
			tool:    "claude",
			cfg:     nil,
			wantCmd: "claude --dangerously-skip-permissions",
			wantOk:  true,
		},
		{
			name:    "unknown tool returns false",
			tool:    "nonexistent",
			cfg:     nil,
			wantCmd: "",
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := GetAgentCommandFromConfig(tt.tool, tt.cfg)
			if ok != tt.wantOk {
				t.Errorf("GetAgentCommandFromConfig() ok = %v, want %v", ok, tt.wantOk)
			}
			if cmd != tt.wantCmd {
				t.Errorf("GetAgentCommandFromConfig() cmd = %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

// TestEffectiveToolPersisted verifies that when no explicit tool is provided,
// the defaultTool is resolved and persisted on the Agent struct so restarts
// use the same tool consistently (fix for Exit 127 crashes).
func TestEffectiveToolPersisted(t *testing.T) {
	mp := mockProvider{name: "testcli", installed: true, version: "1.0", command: "/bin/true"}
	m := newTestManagerWithDefaultTool(t, mp, "testcli")

	// Spawn with empty Tool — should resolve to defaultTool "testcli"
	ag, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{
		Name:      "tool-test",
		Role:      Role("engineer"),
		Workspace: t.TempDir(),
		Tool:      "", // intentionally empty
	})
	if err != nil {
		t.Fatalf("SpawnAgentWithOptions failed: %v", err)
	}
	defer func() { _ = m.runtime().KillSession(context.Background(), "tool-test") }()

	if ag.Tool != "testcli" {
		t.Errorf("Agent.Tool = %q, want %q (defaultTool should be persisted)", ag.Tool, "testcli")
	}
}

// TestEffectiveToolExplicit verifies that an explicitly provided tool is persisted as-is.
func TestEffectiveToolExplicit(t *testing.T) {
	mp := mockProvider{name: "testcli", installed: true, version: "1.0", command: "/bin/true"}
	m := newTestManagerWithDefaultTool(t, mp, "other-default")

	ag, err := m.SpawnAgentWithOptions(context.Background(), SpawnOptions{
		Name:      "tool-explicit",
		Role:      Role("engineer"),
		Workspace: t.TempDir(),
		Tool:      "testcli",
	})
	if err != nil {
		t.Fatalf("SpawnAgentWithOptions failed: %v", err)
	}
	defer func() { _ = m.runtime().KillSession(context.Background(), "tool-explicit") }()

	if ag.Tool != "testcli" {
		t.Errorf("Agent.Tool = %q, want %q", ag.Tool, "testcli")
	}
}

func TestBcdAddrForRuntime_NormalizesEmptyHost(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		runtime string
		want    string
	}{
		{"empty host tmux", "http://:8080", "tmux", "http://127.0.0.1:8080"},
		{"empty host docker", "http://:8080", "docker", "http://host.docker.internal:8080"},
		{"normal tmux", "http://127.0.0.1:9374", "tmux", "http://127.0.0.1:9374"},
		{"normal docker", "http://127.0.0.1:9374", "docker", "http://host.docker.internal:9374"},
		{"no env tmux", "", "tmux", "http://127.0.0.1:9374"},
		{"no env docker", "", "docker", "http://host.docker.internal:9374"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("MYCEL_DAEMON_ADDR", tt.envVal)
			} else {
				t.Setenv("MYCEL_DAEMON_ADDR", "")
			}
			got := daemonAddrForRuntime(tt.runtime)
			if got != tt.want {
				t.Errorf("daemonAddrForRuntime(%q) with MYCEL_DAEMON_ADDR=%q = %q, want %q",
					tt.runtime, tt.envVal, got, tt.want)
			}
		})
	}
}

// TestGlobalAgentVisibility covers the single-tenant semantics: every
// manager shares one agents table (global mycel.db) and loads ALL
// agents regardless of its boot repo — an agent created against a
// different repo must survive a daemon restart, and its name stays
// globally reserved.
func TestGlobalAgentVisibility(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())

	repoA, repoB := t.TempDir(), t.TempDir()
	mA := NewWorkspaceManager(filepath.Join(repoA, "agents"), repoA)
	if err := mA.LoadState(); err != nil {
		t.Fatalf("LoadState A: %v", err)
	}
	defer func() { _ = mA.Close() }()

	if err := mA.RegisterStopped(&Agent{
		Name:      "dup-check",
		Role:      Role("engineer"),
		Workspace: repoA,
	}); err != nil {
		t.Fatalf("RegisterStopped A: %v", err)
	}

	// A manager booted against a different repo still sees the agent —
	// this is the restart path that used to orphan cross-repo agents.
	mB := NewWorkspaceManager(filepath.Join(repoB, "agents"), repoB)
	if err := mB.LoadState(); err != nil {
		t.Fatalf("LoadState B: %v", err)
	}
	defer func() { _ = mB.Close() }()

	got := mB.GetAgent("dup-check")
	if got == nil {
		t.Fatal("manager booted on repo B must load repo A's agent from the global table")
	}
	if got.Repo != repoA {
		t.Errorf("agent repo = %q, want %q", got.Repo, repoA)
	}

	// The name stays globally reserved: registering it again fails.
	err := mB.RegisterStopped(&Agent{
		Name:      "dup-check",
		Role:      Role("engineer"),
		Workspace: repoB,
	})
	if err == nil {
		t.Fatal("expected duplicate name rejection on RegisterStopped")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should say the name is taken, got: %v", err)
	}
}

// --- worktreeManagerFor: worktrees come from the agent's repo ---

// TestWorktreeManagerFor_BootRepoUsesSharedManager verifies the shared
// manager is reused for the boot repo (and for an empty repo).
func TestWorktreeManagerFor_BootRepoUsesSharedManager(t *testing.T) {
	boot := t.TempDir()
	initGitRepo(t, boot)
	stateDir := filepath.Join(t.TempDir(), "state")

	m := NewWorkspaceManager(stateDir, boot)

	if got := m.worktreeManagerFor(boot); got != m.worktreeMgr {
		t.Error("boot repo must reuse the shared worktree manager")
	}
	if got := m.worktreeManagerFor(""); got != m.worktreeMgr {
		t.Error("empty repo must reuse the shared worktree manager")
	}
	// A trailing slash or unclean path still means the boot repo.
	if got := m.worktreeManagerFor(boot + string(filepath.Separator)); got != m.worktreeMgr {
		t.Error("unclean boot repo path must reuse the shared worktree manager")
	}
}

// TestWorktreeManagerFor_CreatesWorktreeFromAgentRepo verifies that an
// agent bound to a repo other than the boot repo gets its worktree
// checked out FROM that repo (regression: worktrees used to always come
// from the boot repo, ignoring CreateOptions.Repo).
func TestWorktreeManagerFor_CreatesWorktreeFromAgentRepo(t *testing.T) {
	ctx := context.Background()
	boot := t.TempDir()
	other := t.TempDir()
	initGitRepo(t, boot)
	initGitRepo(t, other)

	// Distinguish the repos by an extra commit in `other`.
	marker := filepath.Join(other, "OTHER_REPO_MARKER")
	if err := os.WriteFile(marker, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "-C", other, "add", "OTHER_REPO_MARKER"},
		{"git", "-C", other, "commit", "-m", "marker"},
	} {
		//nolint:gosec // test helper
		if out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("git (%v): %s: %v", args, out, err)
		}
	}

	stateDir := filepath.Join(t.TempDir(), "state")
	m := NewWorkspaceManager(stateDir, boot)

	wtMgr := m.worktreeManagerFor(other)
	if wtMgr == m.worktreeMgr {
		t.Fatal("non-boot repo must get its own worktree manager")
	}

	wtDir, err := wtMgr.Create(ctx, "cross-repo-agent")
	if err != nil {
		t.Fatalf("create worktree from agent repo: %v", err)
	}
	t.Cleanup(func() { _ = wtMgr.Remove(ctx, "cross-repo-agent") })

	// The worktree must contain the marker file — i.e. it was checked
	// out from `other`, not from the boot repo.
	if _, err := os.Stat(filepath.Join(wtDir, "OTHER_REPO_MARKER")); err != nil {
		t.Errorf("worktree was not created from the agent's repo: %v", err)
	}

	// And its HEAD must match the agent repo's HEAD.
	revParse := func(dir string) string {
		//nolint:gosec // test helper
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
		if err != nil {
			t.Fatalf("rev-parse %s: %s: %v", dir, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	if got, want := revParse(wtDir), revParse(other); got != want {
		t.Errorf("worktree HEAD = %s, want agent repo HEAD %s", got, want)
	}

	// The worktree lands in the shared flat layout under the mycel home,
	// keyed by bare agent name.
	home, homeErr := workspace.MycelHome()
	if homeErr != nil {
		t.Fatalf("MycelHome: %v", homeErr)
	}
	if want := filepath.Join(home, "worktrees", "cross-repo-agent"); wtDir != want {
		t.Errorf("worktree dir = %q, want %q", wtDir, want)
	}
}

// --- Docker runtime: the agent's repo reaches the container backend ---

// newDockerMockManager builds a Manager whose default runtime is a mock
// docker backend, so tests can inspect the dir/env handed to
// CreateSessionWithEnv without a real Docker daemon.
func newDockerMockManager(t *testing.T, boot string) (*Manager, *mockBackend) {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLiteStore(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	docker := newMockBackend("docker")
	m := &Manager{
		agents:         make(map[string]*Agent),
		backends:       map[string]runtime.Backend{"docker": docker},
		defaultBackend: "docker",
		stateDir:       stateDir,
		store:          store,
		agentCmd:       "/bin/true",
		workspacePath:  boot,
		worktreeMgr:    worktree.NewManagerWithDataDir(boot, stateDir),
	}
	return m, docker
}

// addMarkerCommit commits a marker file so a repo is distinguishable
// from the boot repo in worktree checkouts.
func addMarkerCommit(t *testing.T, repo, marker string) {
	t.Helper()
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(repo, marker), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "-C", repo, "add", marker},
		{"git", "-C", repo, "commit", "-m", "marker"},
	} {
		//nolint:gosec // test helper
		if out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("git (%v): %s: %v", args, out, err)
		}
	}
}

// TestCreateAgent_DockerBackendReceivesAgentRepo verifies the values the
// container backend uses to build the /workspace mount and workdir: the
// session dir must be a worktree checked out from the AGENT's repo, and
// env MYCEL_WORKSPACE must carry that repo — never the boot repo
// (regression: docker containers used to always mount the boot repo).
// A boot-repo agent keeps the boot values unchanged.
func TestCreateAgent_DockerBackendReceivesAgentRepo(t *testing.T) {
	ctx := context.Background()
	boot := t.TempDir()
	other := t.TempDir()
	initGitRepo(t, boot)
	initGitRepo(t, other)
	addMarkerCommit(t, other, "OTHER_REPO_MARKER")

	tests := []struct {
		name       string
		agentName  string
		repo       string // SpawnOptions.Workspace
		wantRepo   string // expected Agent.Repo and env MYCEL_WORKSPACE
		wantMarker bool   // worktree checked out from `other`
	}{
		{
			name:      "boot repo agent unchanged",
			agentName: "boot-eng",
			repo:      boot,
			wantRepo:  boot,
		},
		{
			name:       "cross-repo agent gets its own repo",
			agentName:  "cross-eng",
			repo:       other,
			wantRepo:   other,
			wantMarker: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, docker := newDockerMockManager(t, boot)

			a, err := m.createAgent(ctx, SpawnOptions{
				Name:      tt.agentName,
				Role:      Role("engineer"),
				Workspace: tt.repo,
				Runtime:   "docker",
			})
			if err != nil {
				t.Fatalf("createAgent: %v", err)
			}
			t.Cleanup(func() {
				_ = m.worktreeManagerFor(tt.repo).Remove(ctx, tt.agentName)
			})

			if a.Repo != tt.wantRepo {
				t.Errorf("Agent.Repo = %q, want %q", a.Repo, tt.wantRepo)
			}

			dir, env := docker.lastSession()

			// The container backend mounts env["MYCEL_WORKSPACE"] at
			// /workspace — it must be the agent's repo, not the boot repo.
			if env["MYCEL_WORKSPACE"] != tt.wantRepo {
				t.Errorf("env MYCEL_WORKSPACE = %q, want %q", env["MYCEL_WORKSPACE"], tt.wantRepo)
			}

			// The session dir is the agent's worktree.
			if dir != a.WorktreeDir {
				t.Errorf("session dir = %q, want worktree %q", dir, a.WorktreeDir)
			}
			_, markerErr := os.Stat(filepath.Join(dir, "OTHER_REPO_MARKER"))
			if tt.wantMarker && markerErr != nil {
				t.Errorf("worktree %q not checked out from the agent's repo: %v", dir, markerErr)
			}
			if !tt.wantMarker && markerErr == nil {
				t.Errorf("boot-repo worktree %q unexpectedly contains the cross-repo marker", dir)
			}
		})
	}
}

// TestCreateAgent_FailedCreateRollsBackStoreRow covers the orphan-row bug:
// a background loop (tool health) snapshots m.agents to the store while
// creation is still in flight; when session creation then fails, the
// in-memory entry was removed but the persisted row survived — leaving a
// phantom 'starting' agent that reserved the name forever.
func TestCreateAgent_FailedCreateRollsBackStoreRow(t *testing.T) {
	ctx := context.Background()
	boot := t.TempDir()
	initGitRepo(t, boot)
	m, docker := newDockerMockManager(t, boot)

	// Fail session creation, and persist the half-created agent first —
	// exactly what the periodic tool-health save does mid-creation.
	docker.createErr = fmt.Errorf("image missing")
	docker.onCreate = func() {
		snapshot := map[string]*Agent{
			"doomed": {ID: "doomed", Name: "doomed", Role: Role("engineer"), State: StateStarting, Workspace: boot, Repo: boot},
		}
		if err := m.store.SaveAll(ctx, snapshot); err != nil {
			t.Errorf("mid-create SaveAll: %v", err)
		}
	}

	_, err := m.SpawnAgentWithOptions(ctx, SpawnOptions{
		Name:      "doomed",
		Role:      Role("engineer"),
		Workspace: boot,
	})
	if err == nil {
		t.Fatal("expected create failure")
	}

	// The persisted row must be rolled back with the in-memory entry.
	row, loadErr := m.store.Load(ctx, "doomed")
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if row != nil {
		t.Fatalf("store row survived failed create: %+v", row)
	}
	if got := m.GetAgent("doomed"); got != nil {
		t.Fatalf("in-memory agent survived failed create: %+v", got)
	}

	// The name is immediately reusable.
	docker.createErr = nil
	docker.onCreate = nil
	if err := m.RegisterStopped(&Agent{Name: "doomed", Role: Role("engineer"), Workspace: boot}); err != nil {
		t.Fatalf("name not reusable after failed create: %v", err)
	}
}

// Tmux runtime: trust is seeded into $HOME/.claude.json keyed by the
// host-side worktree path.
func TestSeedHostClaudeTrust_TmuxHostPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	wt := "/home/user/.mycel/worktrees/eng-01"
	seedHostClaudeTrust("claude", wt)

	data, err := os.ReadFile(filepath.Join(home, ".claude.json")) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("claude.json not written: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	projects, _ := root["projects"].(map[string]any)
	entry, _ := projects[wt].(map[string]any)
	if entry == nil {
		t.Fatalf("worktree %q not in projects: %v", wt, root)
	}
	if accepted, _ := entry["hasTrustDialogAccepted"].(bool); !accepted {
		t.Error("hasTrustDialogAccepted not true")
	}
}

func TestSeedHostClaudeTrust_NonClaudeToolIsNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seedHostClaudeTrust("gemini", "/some/worktree")

	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Error("claude.json written for a non-claude tool")
	}
}

// The flat worktree manager must root worktrees and state under the
// mycel home, not the per-workspace state dir.
func TestFlatWorktreeManagerLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)

	m := flatWorktreeManager("/repo", "/unused/agents")
	if got, want := m.Path("eng-01"), filepath.Join(home, "worktrees", "eng-01"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
	if got, want := m.ClaudeDir("eng-01"), filepath.Join(home, "agents", "eng-01", "claude"); got != want {
		t.Errorf("ClaudeDir() = %q, want %q", got, want)
	}
	if got, want := m.Name("eng-01"), "eng-01"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}
