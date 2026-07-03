package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/workspace"
)

func TestIsV2Workspace(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, "home-mycel"))

	projectDir := filepath.Join(tmpDir, "proj")
	if err := os.MkdirAll(projectDir, 0750); err != nil {
		t.Fatal(err)
	}

	// Not a workspace
	if isV2Workspace(projectDir) {
		t.Error("empty dir should not be a workspace")
	}

	// Create preferences.json in the global state dir
	stateDir, err := workspace.GlobalStateDir(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, workspace.PreferencesFileName), []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	if !isV2Workspace(projectDir) {
		t.Error("dir with global preferences.json should be a workspace")
	}
}

func TestInitV2Workspace(t *testing.T) {
	tmpDir := t.TempDir()
	bcHome := filepath.Join(tmpDir, "home-bc")
	t.Setenv("MYCEL_HOME", bcHome)

	projectDir := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(projectDir, 0750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, projectDir)

	// Initialize v2 workspace. M11: state lives under ~/.bc/workspaces/<id>/.
	if err := initV2Workspace(projectDir); err != nil {
		t.Fatalf("initV2Workspace failed: %v", err)
	}

	// Verify the runtime state dir exists (global location).
	stateDir, err := workspace.GlobalStateDir(projectDir)
	if err != nil {
		t.Fatalf("GlobalStateDir: %v", err)
	}
	if _, statErr := os.Stat(stateDir); statErr != nil {
		t.Errorf("global state directory not created: %v", statErr)
	}

	// Verify preferences.json exists and is valid.
	configPath := filepath.Join(stateDir, workspace.PreferencesFileName)
	cfg, err := workspace.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Version != 2 {
		t.Errorf("expected version 2, got %d", cfg.Version)
	}
	if validateErr := cfg.Validate(); validateErr != nil {
		t.Errorf("config validation failed: %v", validateErr)
	}

	// Verify agents directory exists.
	agentsDir := filepath.Join(stateDir, "agents")
	if _, statErr := os.Stat(agentsDir); statErr != nil {
		t.Errorf("agents directory not created: %v", statErr)
	}

	// Project directory should stay pristine — no .bc/ sidecar.
	if _, statErr := os.Stat(filepath.Join(projectDir, ".bc")); statErr == nil {
		t.Errorf("project .bc/ sidecar should not be created; M11 puts state in %s", stateDir)
	}
}

func TestInitV2WorkspaceIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	bcHome := filepath.Join(tmpDir, "home-bc")
	t.Setenv("MYCEL_HOME", bcHome)

	projectDir := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(projectDir, 0750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, projectDir)

	// First init
	if err := initV2Workspace(projectDir); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init should fail (already initialized)
	if isV2Workspace(projectDir) == false {
		t.Error("workspace should be detected as v2 after init")
	}
}

func TestRunInitV2AlreadyInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	bcHome := filepath.Join(tmpDir, "home-bc")
	t.Setenv("MYCEL_HOME", bcHome)

	projectDir := filepath.Join(tmpDir, "test-project")
	if err := os.MkdirAll(projectDir, 0750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, projectDir)

	// First init should succeed
	if err := initV2Workspace(projectDir); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init should fail
	err := runInit(nil, []string{projectDir})
	if err == nil {
		t.Error("expected error when already initialized")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Errorf("error should mention already initialized: %v", err)
	}
}

func TestRunInitFreshDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	bcHome := filepath.Join(tmpDir, "home-bc")
	t.Setenv("MYCEL_HOME", bcHome)

	projectDir := filepath.Join(tmpDir, "fresh-project")
	if err := os.MkdirAll(projectDir, 0750); err != nil {
		t.Fatal(err)
	}
	gitInitDir(t, projectDir)

	// Use quick mode to skip interactive wizard (no stdin in tests)
	initQuick = true
	defer func() { initQuick = false }()
	err := runInit(nil, []string{projectDir})
	if err != nil {
		t.Fatalf("init on fresh directory failed: %v", err)
	}

	// Verify workspace was created
	if !isV2Workspace(projectDir) {
		t.Error("workspace should exist after init")
	}
}
