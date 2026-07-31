package agent

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/runtime"
)

// mockBackend implements runtime.Backend for testing runtime routing.
type mockBackend struct {
	sessions  map[string]bool
	lastEnv   map[string]string
	onCreate  func()
	createErr error
	name      string
	lastDir   string
	sent      []string
	mu        sync.Mutex
}

func newMockBackend(name string) *mockBackend {
	return &mockBackend{name: name, sessions: make(map[string]bool)}
}

func (m *mockBackend) HasSession(_ context.Context, name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[name]
}
func (m *mockBackend) CreateSession(_ context.Context, name, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[name] = true
	return nil
}
func (m *mockBackend) CreateSessionWithCommand(_ context.Context, name, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[name] = true
	return nil
}
func (m *mockBackend) CreateSessionWithEnv(_ context.Context, name, dir, _ string, env map[string]string) error {
	m.mu.Lock()
	onCreate, createErr := m.onCreate, m.createErr
	m.mu.Unlock()
	if onCreate != nil {
		onCreate()
	}
	if createErr != nil {
		return createErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[name] = true
	m.lastDir = dir
	m.lastEnv = env
	return nil
}

// lastSession returns the dir and env of the most recent
// CreateSessionWithEnv call.
func (m *mockBackend) lastSession() (dir string, env map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastDir, m.lastEnv
}
func (m *mockBackend) KillSession(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, name)
	return nil
}
func (m *mockBackend) RenameSession(_ context.Context, old, new string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[old] {
		delete(m.sessions, old)
		m.sessions[new] = true
	}
	return nil
}
func (m *mockBackend) SendKeys(_ context.Context, name, keys string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, fmt.Sprintf("%s:%s", name, keys))
	return nil
}
func (m *mockBackend) SendKeysWithSubmit(_ context.Context, _, _, _ string) error { return nil }
func (m *mockBackend) Capture(_ context.Context, _ string, _ int) (string, error) {
	return "", nil
}
func (m *mockBackend) ListSessions(_ context.Context) ([]runtime.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessions := make([]runtime.Session, 0, len(m.sessions))
	for name := range m.sessions {
		sessions = append(sessions, runtime.Session{Name: name})
	}
	return sessions, nil
}
func (m *mockBackend) AttachCmd(ctx context.Context, _ string) *exec.Cmd {
	return exec.CommandContext(ctx, "true") //nolint:gosec // test stub
}
func (m *mockBackend) IsRunning(_ context.Context) bool   { return true }
func (m *mockBackend) KillServer(_ context.Context) error { return nil }
func (m *mockBackend) SetEnvironment(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockBackend) SessionName(name string) string { return m.name + "-" + name }
func (m *mockBackend) PipePane(_ context.Context, _, _ string) error {
	return nil
}

// newMockManager creates a Manager with mock backends for testing runtime routing.
func newMockManager(t *testing.T, defaultBackend string, backends map[string]*mockBackend) *Manager {
	t.Helper()
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bes := make(map[string]runtime.Backend, len(backends))
	for k, v := range backends {
		bes[k] = v
	}

	return &Manager{
		agents:         make(map[string]*Agent),
		backends:       bes,
		defaultBackend: defaultBackend,
		stateDir:       dir,
		store:          store,
		agentCmd:       "/bin/true",
	}
}

// --- runtimeForAgent tests ---

func TestRuntimeForAgent_Default(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	// Agent with no RuntimeBackend should use default (docker)
	mgr.agents["eng-01"] = &Agent{Name: "eng-01", RuntimeBackend: ""}
	be := mgr.runtimeForAgent("eng-01")
	if be != dockerBe {
		t.Errorf("expected docker backend for agent with empty RuntimeBackend")
	}
}

func TestRuntimeForAgent_PerAgentOverride(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	// Agent with RuntimeBackend="tmux" should use tmux even though default is docker
	mgr.agents["root"] = &Agent{Name: "root", RuntimeBackend: "tmux"}
	be := mgr.runtimeForAgent("root")
	if be != tmuxBe {
		t.Errorf("expected tmux backend for agent with RuntimeBackend=tmux")
	}
}

func TestRuntimeForAgent_UnknownAgent(t *testing.T) {
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"docker": dockerBe,
	})

	// Unknown agent should use default
	be := mgr.runtimeForAgent("nonexistent")
	if be != dockerBe {
		t.Errorf("expected default backend for unknown agent")
	}
}

func TestRuntimeForAgent_InvalidBackendFallback(t *testing.T) {
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"docker": dockerBe,
	})

	// Agent with a backend that doesn't exist should fall back to default
	mgr.agents["eng-01"] = &Agent{Name: "eng-01", RuntimeBackend: "kubernetes"}
	be := mgr.runtimeForAgent("eng-01")
	if be != dockerBe {
		t.Errorf("expected default backend when agent's RuntimeBackend is not registered")
	}
}

func TestRuntimeForAgent_MixedBackends(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	// Set up agents with different backends
	mgr.agents["root"] = &Agent{Name: "root", RuntimeBackend: "tmux"}
	mgr.agents["eng-01"] = &Agent{Name: "eng-01", RuntimeBackend: "docker"}
	mgr.agents["eng-02"] = &Agent{Name: "eng-02", RuntimeBackend: ""}

	tests := []struct {
		want *mockBackend
		name string
	}{
		{tmuxBe, "root"},
		{dockerBe, "eng-01"},
		{dockerBe, "eng-02"}, // empty → default (docker)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			be := mgr.runtimeForAgent(tc.name)
			if be != tc.want {
				t.Errorf("runtimeForAgent(%s) got wrong backend", tc.name)
			}
		})
	}
}

// --- RuntimeBackend persistence tests ---

func TestSQLiteStore_RuntimeBackend_Save(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := &Agent{
		Name:           "root",
		Role:           RoleRoot,
		State:          StateIdle,
		Workspace:      "/repo",
		RuntimeBackend: "tmux",
		StartedAt:      time.Now(),
	}

	if saveErr := store.Save(context.Background(), a); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	loaded, loadErr := store.Load(context.Background(), "root")
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if loaded.RuntimeBackend != "tmux" {
		t.Errorf("RuntimeBackend = %q, want tmux", loaded.RuntimeBackend)
	}
}

func TestSQLiteStore_RuntimeBackend_SaveAll(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	agents := map[string]*Agent{
		"root": {
			Name:           "root",
			Role:           RoleRoot,
			State:          StateIdle,
			Workspace:      "/repo",
			RuntimeBackend: "tmux",
			StartedAt:      time.Now(),
		},
		"eng-01": {
			Name:           "eng-01",
			Role:           Role("engineer"),
			State:          StateWorking,
			Workspace:      "/repo",
			RuntimeBackend: "docker",
			StartedAt:      time.Now(),
		},
		"eng-02": {
			Name:           "eng-02",
			Role:           Role("engineer"),
			State:          StateIdle,
			Workspace:      "/repo",
			RuntimeBackend: "", // empty = default
			StartedAt:      time.Now(),
		},
	}

	if saveErr := store.SaveAll(context.Background(), agents); saveErr != nil {
		t.Fatalf("SaveAll: %v", saveErr)
	}

	all, loadErr := store.LoadAll(context.Background())
	if loadErr != nil {
		t.Fatalf("LoadAll: %v", loadErr)
	}

	if all["root"].RuntimeBackend != "tmux" {
		t.Errorf("root RuntimeBackend = %q, want tmux", all["root"].RuntimeBackend)
	}
	if all["eng-01"].RuntimeBackend != "docker" {
		t.Errorf("eng-01 RuntimeBackend = %q, want docker", all["eng-01"].RuntimeBackend)
	}
	if all["eng-02"].RuntimeBackend != "" {
		t.Errorf("eng-02 RuntimeBackend = %q, want empty", all["eng-02"].RuntimeBackend)
	}
}

func TestSQLiteStore_RuntimeBackend_UpdateField(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	_ = store.Save(context.Background(), &Agent{
		Name:           "root",
		Role:           RoleRoot,
		State:          StateIdle,
		Workspace:      "/repo",
		RuntimeBackend: "docker",
		StartedAt:      time.Now(),
	})

	// Update runtime_backend
	if err := store.UpdateField(context.Background(), "root", "runtime_backend", "tmux"); err != nil {
		t.Fatalf("UpdateField: %v", err)
	}

	loaded, _ := store.Load(context.Background(), "root")
	if loaded.RuntimeBackend != "tmux" {
		t.Errorf("RuntimeBackend = %q, want tmux", loaded.RuntimeBackend)
	}
}

func TestSQLiteStore_RuntimeBackend_LoadRoot(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	_ = store.Save(context.Background(), &Agent{
		Name:           "root",
		Role:           RoleRoot,
		State:          StateIdle,
		Workspace:      "/repo",
		IsRoot:         true,
		RuntimeBackend: "tmux",
		StartedAt:      time.Now(),
	})

	loaded, err := store.LoadRoot(context.Background())
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if loaded.RuntimeBackend != "tmux" {
		t.Errorf("LoadRoot RuntimeBackend = %q, want tmux", loaded.RuntimeBackend)
	}
}

// --- RuntimeBackend round-trip through Manager ---

func TestManager_RuntimeBackend_RoundTrip(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	// Simulate adding agents with different backends
	mgr.agents["root"] = &Agent{
		Name:           "root",
		Role:           RoleRoot,
		State:          StateIdle,
		Workspace:      "/repo",
		RuntimeBackend: "tmux",
		IsRoot:         true,
		StartedAt:      time.Now(),
		Children:       []string{},
	}
	mgr.agents["eng-01"] = &Agent{
		Name:           "eng-01",
		Role:           Role("engineer"),
		State:          StateWorking,
		Workspace:      "/repo",
		RuntimeBackend: "docker",
		StartedAt:      time.Now(),
		Children:       []string{},
	}

	// Save state
	if err := mgr.saveState(context.Background()); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	// Create a new manager and load state
	mgr2 := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})
	// Point to same DB
	store2, err := NewSQLiteStore(filepath.Join(mgr.stateDir, "state.db"))
	if err != nil {
		t.Fatalf("open store2: %v", err)
	}
	mgr2.store = store2

	agents, err := store2.LoadAll(context.Background())
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	mgr2.agents = agents

	// Verify runtime routing works after reload
	if mgr2.runtimeForAgent("root") != tmuxBe {
		t.Error("root should route to tmux after reload")
	}
	if mgr2.runtimeForAgent("eng-01") != dockerBe {
		t.Error("eng-01 should route to docker after reload")
	}
}

// --- SendToAgent routing test ---

func TestSendToAgent_UsesCorrectBackend(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	tmuxBe.sessions["root"] = true
	dockerBe.sessions["eng-01"] = true

	mgr.agents["root"] = &Agent{Name: "root", RuntimeBackend: "tmux"}
	mgr.agents["eng-01"] = &Agent{Name: "eng-01", RuntimeBackend: "docker"}

	if err := mgr.SendToAgent(context.Background(), "root", "hello root"); err != nil {
		t.Fatalf("SendToAgent root: %v", err)
	}
	if err := mgr.SendToAgent(context.Background(), "eng-01", "hello eng"); err != nil {
		t.Fatalf("SendToAgent eng-01: %v", err)
	}

	// Check tmux got root's message
	tmuxBe.mu.Lock()
	tmuxGot := len(tmuxBe.sent)
	tmuxBe.mu.Unlock()
	if tmuxGot != 1 {
		t.Errorf("tmux backend got %d messages, want 1", tmuxGot)
	}

	// Check docker got eng-01's message
	dockerBe.mu.Lock()
	dockerGot := len(dockerBe.sent)
	dockerBe.mu.Unlock()
	if dockerGot != 1 {
		t.Errorf("docker backend got %d messages, want 1", dockerGot)
	}
}

// --- StopAgent uses correct backend ---

func TestStopAgent_UsesCorrectBackend(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	tmuxBe.sessions["root"] = true
	dockerBe.sessions["eng-01"] = true

	mgr.agents["root"] = &Agent{Name: "root", State: StateIdle, RuntimeBackend: "tmux", Children: []string{}}
	mgr.agents["eng-01"] = &Agent{Name: "eng-01", State: StateWorking, RuntimeBackend: "docker", Children: []string{}}

	// Stop root — should kill tmux session
	if err := mgr.StopAgent(context.Background(), "root"); err != nil {
		t.Fatalf("StopAgent root: %v", err)
	}
	if tmuxBe.sessions["root"] {
		t.Error("root tmux session should be killed")
	}
	if !dockerBe.sessions["eng-01"] {
		t.Error("eng-01 docker session should still be alive")
	}

	// Stop eng-01 — should kill docker session
	if err := mgr.StopAgent(context.Background(), "eng-01"); err != nil {
		t.Fatalf("StopAgent eng-01: %v", err)
	}
	if dockerBe.sessions["eng-01"] {
		t.Error("eng-01 docker session should be killed")
	}
}

// --- RuntimeForAgent public method is thread-safe ---

func TestRuntimeForAgent_ConcurrentReadWrite(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	dockerBe := newMockBackend("docker")

	mgr := newMockManager(t, "docker", map[string]*mockBackend{
		"tmux":   tmuxBe,
		"docker": dockerBe,
	})

	mgr.agents["root"] = &Agent{
		Name: "root", RuntimeBackend: "tmux", Role: RoleRoot,
		State: StateIdle, Workspace: "/repo", StartedAt: time.Now(), Children: []string{},
	}
	mgr.agents["eng-01"] = &Agent{
		Name: "eng-01", RuntimeBackend: "docker", Role: Role("engineer"),
		State: StateWorking, Workspace: "/repo", StartedAt: time.Now(), Children: []string{},
	}

	var wg sync.WaitGroup

	// Concurrent reads via RuntimeForAgent (public, takes RLock)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.RuntimeForAgent("root")
			mgr.RuntimeForAgent("eng-01")
			mgr.RuntimeForAgent("nonexistent")
		}()
	}

	// Concurrent writes via state updates (takes Lock)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.mu.Lock()
			mgr.agents["root"].UpdatedAt = time.Now()
			mgr.mu.Unlock()
		}()
	}

	// Concurrent sends (takes RLock internally)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mgr.SendToAgent(context.Background(), "root", "ping")
		}()
	}

	wg.Wait()
}

// --- Manager constructors ---

func TestNewManagerWithRuntime_RegistersTmuxFallback(t *testing.T) {
	dockerBe := newMockBackend("docker")
	mgr := NewManagerWithRuntime(t.TempDir(), "/tmp/repo", dockerBe, "docker")

	// Should have both docker (explicit) and tmux (fallback)
	if _, ok := mgr.backends["docker"]; !ok {
		t.Error("missing docker backend")
	}
	if _, ok := mgr.backends["tmux"]; !ok {
		t.Error("missing tmux fallback backend")
	}
	if mgr.defaultBackend != "docker" {
		t.Errorf("defaultBackend = %q, want docker", mgr.defaultBackend)
	}
}

func TestNewManagerWithRuntime_TmuxDefault(t *testing.T) {
	tmuxBe := newMockBackend("tmux")
	mgr := NewManagerWithRuntime(t.TempDir(), "/tmp/repo", tmuxBe, "tmux")

	// Only tmux, no duplicate
	if len(mgr.backends) != 1 {
		t.Errorf("expected 1 backend, got %d", len(mgr.backends))
	}
	if mgr.defaultBackend != "tmux" {
		t.Errorf("defaultBackend = %q, want tmux", mgr.defaultBackend)
	}
}
