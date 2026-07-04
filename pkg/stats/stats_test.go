package stats

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/db"
)

func TestNew(t *testing.T) {
	s := New("/tmp/test-state")
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.path != "/tmp/test-state/stats.json" {
		t.Errorf("path = %q, want %q", s.path, "/tmp/test-state/stats.json")
	}
	if s.CollectedAt.IsZero() {
		t.Error("CollectedAt should not be zero")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	s := New(dir)
	s.WorkspacePath = "/test/workspace"
	s.Agents.TotalAgents = 3

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(dir, "stats.json")) //nolint:gosec // test file read
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("saved file is empty")
	}

	// Load into new struct
	var loaded Stats
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded.WorkspacePath != "/test/workspace" {
		t.Errorf("WorkspacePath = %q, want %q", loaded.WorkspacePath, "/test/workspace")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	s := New(dir)
	s.Agents.TotalAgents = 1

	if err := s.Save(); err != nil {
		t.Fatalf("Save to nested dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "stats.json")); err != nil {
		t.Errorf("stats.json not created: %v", err)
	}
}

func TestUtilization(t *testing.T) {
	tests := []struct {
		name    string
		active  int
		working int
		want    float64
	}{
		{"no agents", 0, 0, 0},
		{"all idle", 5, 0, 0},
		{"all working", 4, 4, 1.0},
		{"half working", 6, 3, 0.5},
		{"one of three", 3, 1, 1.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Stats{}
			s.Agents.ActiveAgents = tt.active
			s.Agents.Working = tt.working

			got := s.Utilization()
			if got != tt.want {
				t.Errorf("Utilization() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		want string
		d    time.Duration
	}{
		{"zero", "0s", 0},
		{"seconds", "45s", 45 * time.Second},
		{"minutes and seconds", "3m 12s", 3*time.Minute + 12*time.Second},
		{"hours and minutes", "2h 30m", 2*time.Hour + 30*time.Minute},
		{"hours only", "1h 0m", 1 * time.Hour},
		{"sub-second rounds down", "1s", 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestSummaryContainsExpectedSections(t *testing.T) {
	s := &Stats{
		CollectedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		Agents: AgentMetrics{
			TotalAgents:  3,
			ActiveAgents: 2,
			Working:      1,
			AgentStats: []AgentStat{
				{Name: "coord", Role: "coordinator", State: "working", Uptime: 1 * time.Hour},
			},
		},
	}

	summary := s.Summary()

	expectedParts := []string{
		"Workspace Stats",
		"Agents",
		"Total:  3 (2 active)",
		"Per Agent:",
		"coord",
	}

	for _, part := range expectedParts {
		if !strings.Contains(summary, part) {
			t.Errorf("Summary missing expected content: %q\nGot:\n%s", part, summary)
		}
	}
}

func TestSummaryNoAgentStatsSection(t *testing.T) {
	s := &Stats{
		CollectedAt: time.Now(),
	}

	summary := s.Summary()
	if strings.Contains(summary, "Per Agent:") {
		t.Error("Summary should not show Per Agent section with no agents")
	}
}

// seedAgentsFile writes agent data to the single global mycel.db
// (pinned to a per-test MYCEL_HOME) tagged with the given workspace
// root so a repo-scoped manager loads exactly these agents.
func seedAgentsFile(t *testing.T, workspaceRoot string, agents map[string]*agent.Agent) {
	t.Helper()
	t.Setenv("MYCEL_HOME", filepath.Join(workspaceRoot, ".mycel-home"))
	bcDir := filepath.Join(workspaceRoot, ".bc")
	if err := os.MkdirAll(filepath.Join(bcDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir .bc/agents: %v", err)
	}
	dbPath, err := db.GlobalDBPath()
	if err != nil {
		t.Fatalf("GlobalDBPath: %v", err)
	}
	store, err := agent.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()
	for _, a := range agents {
		if a.Repo == "" {
			a.Repo = workspaceRoot
		}
	}
	if err := store.SaveAll(context.Background(), agents); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
}

func TestCollectAgentMetricsEmpty(t *testing.T) {
	wsRoot := t.TempDir()
	stateDir := filepath.Join(wsRoot, ".bc")
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		t.Fatal(err)
	}

	s := New(stateDir)
	mgr := agent.NewWorkspaceManager(agentsDir, wsRoot)

	s.collectAgentMetrics(mgr)

	if s.Agents.TotalAgents != 0 {
		t.Errorf("TotalAgents = %d, want 0", s.Agents.TotalAgents)
	}
	if s.Agents.ActiveAgents != 0 {
		t.Errorf("ActiveAgents = %d, want 0", s.Agents.ActiveAgents)
	}
	if len(s.Agents.AgentStats) != 0 {
		t.Errorf("AgentStats len = %d, want 0", len(s.Agents.AgentStats))
	}
}

func TestCollectAgentMetricsRoleCounts(t *testing.T) {
	wsRoot := t.TempDir()
	stateDir := filepath.Join(wsRoot, ".bc")
	agentsDir := filepath.Join(stateDir, "agents")

	agents := map[string]*agent.Agent{
		"coord-01": {Name: "coord-01", Role: agent.RoleRoot, State: agent.StateIdle, StartedAt: time.Now()},
		"coord-02": {Name: "coord-02", Role: agent.RoleRoot, State: agent.StateWorking, StartedAt: time.Now()},
		"eng-01":   {Name: "eng-01", Role: agent.Role("worker"), State: agent.StateIdle, StartedAt: time.Now()},
		"eng-02":   {Name: "eng-02", Role: agent.Role("worker"), State: agent.StateWorking, StartedAt: time.Now()},
		"eng-03":   {Name: "eng-03", Role: agent.Role("worker"), State: agent.StateDone, StartedAt: time.Now()},
	}
	seedAgentsFile(t, wsRoot, agents)

	mgr := agent.NewWorkspaceManager(agentsDir, wsRoot)
	if err := mgr.LoadState(); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	s := New(stateDir)
	s.collectAgentMetrics(mgr)

	if s.Agents.TotalAgents != 5 {
		t.Errorf("TotalAgents = %d, want 5", s.Agents.TotalAgents)
	}
	if s.Agents.Coordinators != 2 {
		t.Errorf("Coordinators = %d, want 2", s.Agents.Coordinators)
	}
	if s.Agents.Workers != 3 {
		t.Errorf("Workers = %d, want 3", s.Agents.Workers)
	}
}

func TestCollectAgentMetricsStateCounts(t *testing.T) {
	wsRoot := t.TempDir()
	stateDir := filepath.Join(wsRoot, ".bc")
	agentsDir := filepath.Join(stateDir, "agents")

	agents := map[string]*agent.Agent{
		"a1": {Name: "a1", Role: agent.Role("worker"), State: agent.StateIdle, StartedAt: time.Now()},
		"a2": {Name: "a2", Role: agent.Role("worker"), State: agent.StateWorking, StartedAt: time.Now()},
		"a3": {Name: "a3", Role: agent.Role("worker"), State: agent.StateDone, StartedAt: time.Now()},
		"a4": {Name: "a4", Role: agent.Role("worker"), State: agent.StateStuck, StartedAt: time.Now()},
		"a5": {Name: "a5", Role: agent.Role("worker"), State: agent.StateError},
		"a6": {Name: "a6", Role: agent.Role("worker"), State: agent.StateStopped},
	}
	seedAgentsFile(t, wsRoot, agents)

	mgr := agent.NewWorkspaceManager(agentsDir, wsRoot)
	if err := mgr.LoadState(); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	s := New(stateDir)
	s.collectAgentMetrics(mgr)

	if s.Agents.Idle != 1 {
		t.Errorf("Idle = %d, want 1", s.Agents.Idle)
	}
	if s.Agents.Working != 1 {
		t.Errorf("Working = %d, want 1", s.Agents.Working)
	}
	if s.Agents.Done != 1 {
		t.Errorf("Done = %d, want 1", s.Agents.Done)
	}
	if s.Agents.Stuck != 1 {
		t.Errorf("Stuck = %d, want 1", s.Agents.Stuck)
	}
	if s.Agents.Error != 1 {
		t.Errorf("Error = %d, want 1", s.Agents.Error)
	}
	if s.Agents.Stopped != 1 {
		t.Errorf("Stopped = %d, want 1", s.Agents.Stopped)
	}
	// Active = idle + working + done + stuck = 4
	if s.Agents.ActiveAgents != 4 {
		t.Errorf("ActiveAgents = %d, want 4", s.Agents.ActiveAgents)
	}
}

func TestCollectAgentMetricsUptime(t *testing.T) {
	wsRoot := t.TempDir()
	stateDir := filepath.Join(wsRoot, ".bc")
	agentsDir := filepath.Join(stateDir, "agents")

	startTime := time.Now().Add(-2 * time.Hour)
	agents := map[string]*agent.Agent{
		"running": {Name: "running", Role: agent.Role("worker"), State: agent.StateWorking, StartedAt: startTime},
		"stopped": {Name: "stopped", Role: agent.Role("worker"), State: agent.StateStopped, StartedAt: startTime},
		"no-time": {Name: "no-time", Role: agent.Role("worker"), State: agent.StateIdle},
	}
	seedAgentsFile(t, wsRoot, agents)

	mgr := agent.NewWorkspaceManager(agentsDir, wsRoot)
	if err := mgr.LoadState(); err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	s := New(stateDir)
	s.collectAgentMetrics(mgr)

	for _, stat := range s.Agents.AgentStats {
		switch stat.Name {
		case "running":
			if stat.Uptime < 1*time.Hour {
				t.Errorf("running agent uptime = %v, want >= 1h", stat.Uptime)
			}
		case "stopped":
			if stat.Uptime != 0 {
				t.Errorf("stopped agent uptime = %v, want 0", stat.Uptime)
			}
		case "no-time":
			if stat.Uptime != 0 {
				t.Errorf("no-time agent uptime = %v, want 0", stat.Uptime)
			}
		}
	}
}

func TestLoadEmptyStateDir(t *testing.T) {
	wsRoot := t.TempDir()
	stateDir := filepath.Join(wsRoot, ".bc")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}

	s, err := Load(stateDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.Agents.TotalAgents != 0 {
		t.Errorf("Agents.TotalAgents = %d, want 0", s.Agents.TotalAgents)
	}
	if s.CollectedAt.IsZero() {
		t.Error("CollectedAt should not be zero")
	}
	if s.WorkspacePath != wsRoot {
		t.Errorf("WorkspacePath = %q, want %q", s.WorkspacePath, wsRoot)
	}
}

func TestLoadWithAgentsData(t *testing.T) {
	wsRoot := t.TempDir()
	stateDir := filepath.Join(wsRoot, ".bc")

	// Seed agents as already stopped
	agents := map[string]*agent.Agent{
		"coord": {Name: "coord", Role: agent.RoleRoot, State: agent.StateStopped},
		"eng-1": {Name: "eng-1", Role: agent.Role("worker"), State: agent.StateStopped},
		"eng-2": {Name: "eng-2", Role: agent.Role("worker"), State: agent.StateStopped},
	}
	seedAgentsFile(t, wsRoot, agents)

	s, err := Load(stateDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.Agents.TotalAgents != 3 {
		t.Errorf("TotalAgents = %d, want 3", s.Agents.TotalAgents)
	}
	if s.Agents.Coordinators != 1 {
		t.Errorf("Coordinators = %d, want 1", s.Agents.Coordinators)
	}
	if s.Agents.Workers != 2 {
		t.Errorf("Workers = %d, want 2", s.Agents.Workers)
	}
	if s.Agents.Stopped != 3 {
		t.Errorf("Stopped = %d, want 3", s.Agents.Stopped)
	}
}

func TestSummaryIncludesDoneState(t *testing.T) {
	s := &Stats{
		CollectedAt: time.Now(),
		Agents: AgentMetrics{
			TotalAgents:  5,
			ActiveAgents: 4,
			Idle:         1,
			Working:      1,
			Done:         2,
			Stuck:        0,
			Stopped:      1,
		},
	}

	summary := s.Summary()

	// Verify done state is included in the States line
	expectedStates := "1 idle, 1 working, 2 done, 0 stuck, 1 stopped"
	if !strings.Contains(summary, expectedStates) {
		t.Errorf("Summary should include done state in States line\nExpected: %q\nGot:\n%s", expectedStates, summary)
	}
}

func TestLoadInvalidAgentsFile(t *testing.T) {
	stateDir := t.TempDir()
	agentsDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Write invalid JSON — migration logs a warning but doesn't error
	if err := os.WriteFile(filepath.Join(agentsDir, "agents.json"), []byte("not json{{{"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := Load(stateDir)
	if err != nil {
		t.Fatalf("Load: %v (migration should be lenient with corrupt JSON)", err)
	}
	// No agents should be loaded from corrupt file
	if s.Agents.TotalAgents != 0 {
		t.Errorf("TotalAgents = %d, want 0 for corrupt JSON", s.Agents.TotalAgents)
	}
}
