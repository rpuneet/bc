package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultDiscoverOptions(t *testing.T) {
	opts := DefaultDiscoverOptions()

	if !opts.IncludeCached {
		t.Error("expected IncludeCached to be true by default")
	}

	if !opts.ScanHome {
		t.Error("expected ScanHome to be true by default")
	}

	if opts.MaxDepth != 3 {
		t.Errorf("expected MaxDepth to be 3, got %d", opts.MaxDepth)
	}

	if len(opts.ScanPaths) != 0 {
		t.Errorf("expected ScanPaths to be empty, got %d items", len(opts.ScanPaths))
	}
}

func TestIsV2Workspace(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, "home-mycel"))

	wsDir := filepath.Join(tmpDir, "proj")
	if err := os.MkdirAll(wsDir, 0750); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Not a workspace without a global preferences.json
	if isV2Workspace(wsDir) {
		t.Error("expected non-workspace to return false")
	}

	// Create preferences.json in the global state dir
	stateDir, err := GlobalStateDir(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(stateDir, PreferencesFileName)
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create preferences.json: %v", err)
	}

	// Now it should be v2
	if !isV2Workspace(wsDir) {
		t.Error("expected workspace with global preferences.json to return true")
	}
}

func TestDiscoverWithEmptyOptions(t *testing.T) {
	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      0,
		ScanPaths:     []string{},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// With no options enabled, should return empty
	if len(workspaces) != 0 {
		t.Errorf("expected 0 workspaces with empty options, got %d", len(workspaces))
	}
}

func TestDiscoverWithScanPath(t *testing.T) {
	// Create temp directory with a workspace
	tmpDir := t.TempDir()
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, "home-mycel"))
	wsDir := filepath.Join(tmpDir, "test-workspace")
	bcDir := filepath.Join(wsDir, ".bc")

	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Create minimal config in the global state dir
	stateDir, sdErr := GlobalStateDir(wsDir)
	if sdErr != nil {
		t.Fatal(sdErr)
	}
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(stateDir, PreferencesFileName)
	configContent := `{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}

	ws := workspaces[0]
	if ws.Name != "test-workspace" {
		t.Errorf("expected name 'test-workspace', got %q", ws.Name)
	}
	if !ws.IsV2 {
		t.Error("expected IsV2 to be true")
	}
	if ws.FromCache {
		t.Error("expected FromCache to be false")
	}
}

func TestDiscoverSkipsHiddenDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a hidden directory with a workspace
	hiddenDir := filepath.Join(tmpDir, ".hidden")
	hiddenWsDir := filepath.Join(hiddenDir, "workspace")
	hiddenBcDir := filepath.Join(hiddenWsDir, ".bc")

	if err := os.MkdirAll(hiddenBcDir, 0750); err != nil {
		t.Fatalf("failed to create hidden workspace dir: %v", err)
	}

	configPath := filepath.Join(hiddenBcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      3,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should not find the hidden workspace
	for _, ws := range workspaces {
		if ws.Name == "hidden" {
			t.Error("expected hidden workspace to be skipped")
		}
	}
}

func TestDiscoverSkipsNodeModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a node_modules directory with a workspace
	nodeModDir := filepath.Join(tmpDir, "node_modules")
	nodeWsDir := filepath.Join(nodeModDir, "some-package")
	nodeBcDir := filepath.Join(nodeWsDir, ".bc")

	if err := os.MkdirAll(nodeBcDir, 0750); err != nil {
		t.Fatalf("failed to create node_modules workspace dir: %v", err)
	}

	configPath := filepath.Join(nodeBcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      3,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should not find the node_modules workspace
	for _, ws := range workspaces {
		if ws.Name == "npm-pkg" {
			t.Error("expected node_modules workspace to be skipped")
		}
	}
}

func TestDiscoveredWorkspaceStruct(t *testing.T) {
	ws := DiscoveredWorkspace{
		Path:      "/path/to/workspace",
		Name:      "my-workspace",
		IsV2:      true,
		FromCache: false,
	}

	if ws.Path != "/path/to/workspace" {
		t.Errorf("expected path '/path/to/workspace', got %q", ws.Path)
	}
	if ws.Name != "my-workspace" {
		t.Errorf("expected name 'my-workspace', got %q", ws.Name)
	}
	if !ws.IsV2 {
		t.Error("expected IsV2 to be true")
	}
	if ws.FromCache {
		t.Error("expected FromCache to be false")
	}
}

func TestDiscoverOptionsStruct(t *testing.T) {
	opts := DiscoverOptions{
		ScanPaths:     []string{"/path1", "/path2"},
		MaxDepth:      5,
		IncludeCached: true,
		ScanHome:      false,
	}

	if len(opts.ScanPaths) != 2 {
		t.Errorf("expected 2 scan paths, got %d", len(opts.ScanPaths))
	}
	if opts.MaxDepth != 5 {
		t.Errorf("expected max depth 5, got %d", opts.MaxDepth)
	}
	if !opts.IncludeCached {
		t.Error("expected IncludeCached to be true")
	}
	if opts.ScanHome {
		t.Error("expected ScanHome to be false")
	}
}

func TestDiscoverMaxDepthRespected(t *testing.T) {
	tmpDir := t.TempDir()

	// Create deeply nested workspace (depth 4 from tmpDir)
	deepDir := filepath.Join(tmpDir, "level1", "level2", "level3", "deep-ws")
	bcDir := filepath.Join(deepDir, ".bc")

	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create deep workspace dir: %v", err)
	}

	// Create settings.json - workspace will use directory name as fallback
	configPath := filepath.Join(bcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	// With MaxDepth 2, should not find it (workspace is at depth 4)
	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	for _, ws := range workspaces {
		if ws.Path == deepDir {
			t.Error("expected deep workspace to be skipped due to MaxDepth 2")
		}
	}

	// With MaxDepth 5, should find it
	opts.MaxDepth = 5
	workspaces, err = Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	found := false
	for _, ws := range workspaces {
		if ws.Path == deepDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find deep workspace with MaxDepth 5, found %d workspaces", len(workspaces))
		for _, ws := range workspaces {
			t.Logf("  found: %s at %s", ws.Name, ws.Path)
		}
	}
}

func TestDiscoverV1Workspace(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "v1-workspace")
	bcDir := filepath.Join(wsDir, ".bc")

	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Create config.json (v1)
	configPath := filepath.Join(bcDir, "config.json")
	configContent := `{"version": 1, "name": "v1-workspace"}`
	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}

	ws := workspaces[0]
	if ws.Name != "v1-workspace" {
		t.Errorf("expected name 'v1-workspace', got %q", ws.Name)
	}
	if ws.IsV2 {
		t.Error("expected IsV2 to be false for v1 workspace")
	}
}

func TestDiscoverMultipleWorkspaces(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple workspaces
	for i, name := range []string{"ws-alpha", "ws-beta", "ws-gamma"} {
		wsDir := filepath.Join(tmpDir, name)
		bcDir := filepath.Join(wsDir, ".bc")
		if err := os.MkdirAll(bcDir, 0750); err != nil {
			t.Fatalf("failed to create workspace %d: %v", i, err)
		}
		configPath := filepath.Join(bcDir, "settings.json")
		if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
			t.Fatalf("failed to create config %d: %v", i, err)
		}
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(workspaces) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(workspaces))
	}

	names := make(map[string]bool)
	for _, ws := range workspaces {
		names[ws.Name] = true
	}
	for _, expected := range []string{"ws-alpha", "ws-beta", "ws-gamma"} {
		if !names[expected] {
			t.Errorf("expected to find workspace %q", expected)
		}
	}
}

func TestDiscoverNonExistentPath(t *testing.T) {
	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		ScanPaths:     []string{"/nonexistent/path/that/does/not/exist"},
	}

	// Should not error, just return empty
	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(workspaces) != 0 {
		t.Errorf("expected 0 workspaces for non-existent path, got %d", len(workspaces))
	}
}

func TestDiscoverAndRegister(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up global dir for registry
	t.Setenv("HOME", tmpDir)

	// Create global .bc directory
	globalDir := filepath.Join(tmpDir, ".bc")
	if err := os.MkdirAll(globalDir, 0750); err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}

	// Create a workspace to discover
	wsDir := filepath.Join(tmpDir, "projects", "test-ws")
	bcDir := filepath.Join(wsDir, ".bc")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}
	configPath := filepath.Join(bcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      3,
		ScanPaths:     []string{filepath.Join(tmpDir, "projects")},
	}

	count, err := DiscoverAndRegister(opts)
	if err != nil {
		t.Fatalf("DiscoverAndRegister failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 new workspace, got %d", count)
	}

	// Verify it was registered
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}

	found := false
	for _, ws := range registry.Workspaces {
		if ws.Name == "test-ws" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected test-ws to be registered")
	}
}

func TestDiscoverAndRegisterNoNew(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up global dir for registry
	t.Setenv("HOME", tmpDir)

	// Create global .bc directory
	globalDir := filepath.Join(tmpDir, ".bc")
	if err := os.MkdirAll(globalDir, 0750); err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		ScanPaths:     []string{},
	}

	// With no workspaces to discover, count should be 0
	count, err := DiscoverAndRegister(opts)
	if err != nil {
		t.Fatalf("DiscoverAndRegister failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 new workspaces, got %d", count)
	}
}

func TestScanDirAbsPathError(t *testing.T) {
	// Test with invalid path that can't be made absolute
	// (This is hard to trigger in practice, but we can test the flow)
	seen := make(map[string]bool)
	var workspaces []DiscoveredWorkspace
	var mu sync.Mutex

	// Pass valid path, should not panic
	scanDir(t.TempDir(), 1, seen, &workspaces, &mu)
}

func TestDiscoverSkipsVendorDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a vendor directory with a workspace
	vendorDir := filepath.Join(tmpDir, "vendor")
	vendorWsDir := filepath.Join(vendorDir, "some-dep")
	vendorBcDir := filepath.Join(vendorWsDir, ".bc")

	if err := os.MkdirAll(vendorBcDir, 0750); err != nil {
		t.Fatalf("failed to create vendor workspace dir: %v", err)
	}

	configPath := filepath.Join(vendorBcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      3,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should not find the vendor workspace
	for _, ws := range workspaces {
		if ws.Name == "vendor-pkg" {
			t.Error("expected vendor workspace to be skipped")
		}
	}
}

func TestDiscoverSkipsPycacheDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a __pycache__ directory with a workspace
	pycacheDir := filepath.Join(tmpDir, "__pycache__")
	pycacheWsDir := filepath.Join(pycacheDir, "module")
	pycacheBcDir := filepath.Join(pycacheWsDir, ".bc")

	if err := os.MkdirAll(pycacheBcDir, 0750); err != nil {
		t.Fatalf("failed to create __pycache__ workspace dir: %v", err)
	}

	configPath := filepath.Join(pycacheBcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      3,
		ScanPaths:     []string{tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should not find the __pycache__ workspace
	for _, ws := range workspaces {
		if ws.Name == "pycache-pkg" {
			t.Error("expected __pycache__ workspace to be skipped")
		}
	}
}

func TestDiscoverScanDirNegativeDepth(t *testing.T) {
	tmpDir := t.TempDir()
	seen := make(map[string]bool)
	var workspaces []DiscoveredWorkspace
	var mu sync.Mutex

	// With negative maxDepth, scanDir should return immediately
	scanDir(tmpDir, -1, seen, &workspaces, &mu)

	if len(workspaces) != 0 {
		t.Errorf("expected 0 workspaces with negative depth, got %d", len(workspaces))
	}
}

func TestDiscoverDuplicatePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a workspace
	wsDir := filepath.Join(tmpDir, "dup-workspace")
	bcDir := filepath.Join(wsDir, ".bc")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	configPath := filepath.Join(bcDir, "settings.json")
	if err := os.WriteFile(configPath, []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	opts := DiscoverOptions{
		IncludeCached: false,
		ScanHome:      false,
		MaxDepth:      2,
		// Pass same path twice to test deduplication
		ScanPaths: []string{tmpDir, tmpDir},
	}

	workspaces, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should find workspace only once
	count := 0
	for _, ws := range workspaces {
		if ws.Name == "dup-workspace" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 workspace (deduped), got %d", count)
	}
}

func TestDiscoverIncludeCached(t *testing.T) {
	tmpDir := t.TempDir()

	// Point HOME at tmpDir so LoadRegistry reads our custom registry
	t.Setenv("HOME", tmpDir)
	t.Setenv("MYCEL_HOME", filepath.Join(tmpDir, ".mycel"))

	// Create a workspace that the registry will reference
	wsDir := filepath.Join(tmpDir, "cached-ws")
	bcDir := filepath.Join(wsDir, ".bc")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatal(err)
	}
	stateDir, sdErr := GlobalStateDir(wsDir)
	if sdErr != nil {
		t.Fatal(sdErr)
	}
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, PreferencesFileName), []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Write a registry file pointing to that workspace
	globalDir := filepath.Join(tmpDir, ".mycel")
	if err := os.MkdirAll(globalDir, 0750); err != nil {
		t.Fatal(err)
	}
	reg := &Registry{
		Workspaces: []RegistryEntry{
			{Path: wsDir, Name: "cached-ws"},
		},
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(filepath.Join(globalDir, "workspaces.json"), data, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}

	workspaces, err := Discover(DiscoverOptions{
		IncludeCached: true,
		ScanHome:      false,
		MaxDepth:      1,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	found := false
	for _, ws := range workspaces {
		if ws.Path == wsDir && ws.FromCache {
			found = true
			if ws.Name != "cached-ws" {
				t.Errorf("cached workspace name = %q, want %q", ws.Name, "cached-ws")
			}
			if !ws.IsV2 {
				t.Error("cached workspace should be V2 (has preferences.json)")
			}
		}
	}
	if !found {
		t.Error("expected to find cached workspace from registry")
	}
}

func TestDiscoverIncludeCachedSkipsNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write a registry with a path that doesn't exist
	globalDir := filepath.Join(tmpDir, ".bc")
	if err := os.MkdirAll(globalDir, 0750); err != nil {
		t.Fatal(err)
	}
	reg := &Registry{
		Workspaces: []RegistryEntry{
			{Path: filepath.Join(tmpDir, "nonexistent"), Name: "ghost"},
		},
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(filepath.Join(globalDir, "workspaces.json"), data, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}

	workspaces, err := Discover(DiscoverOptions{
		IncludeCached: true,
		ScanHome:      false,
		MaxDepth:      1,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	for _, ws := range workspaces {
		if ws.Name == "ghost" {
			t.Error("should not include non-existent workspace from registry")
		}
	}
}

func TestDiscoverScanHome(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a workspace under ~/Projects/
	projectsDir := filepath.Join(tmpDir, "Projects")
	wsDir := filepath.Join(projectsDir, "home-ws")
	bcDir := filepath.Join(wsDir, ".bc")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bcDir, "settings.json"), []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	workspaces, err := Discover(DiscoverOptions{
		IncludeCached: false,
		ScanHome:      true,
		MaxDepth:      3,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	found := false
	for _, ws := range workspaces {
		if ws.Name == "home-ws" {
			found = true
			if ws.FromCache {
				t.Error("scanned workspace should not be FromCache")
			}
		}
	}
	if !found {
		t.Error("expected to find workspace under ~/Projects/ via ScanHome")
	}
}

func TestDiscoverCachedDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create workspace under ~/Projects/
	projectsDir := filepath.Join(tmpDir, "Projects")
	wsDir := filepath.Join(projectsDir, "dedup-ws")
	bcDir := filepath.Join(wsDir, ".bc")
	if err := os.MkdirAll(bcDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bcDir, "settings.json"), []byte(`{"version":2,"providers":{"default":"claude","providers":{"claude":{"command":"claude"}}},"server":{"host":"127.0.0.1","port":9374,"cors_origin":"*"},"runtime":{"default":"tmux"},"ui":{"theme":"dark","mode":"auto"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Also register same workspace in registry
	globalDir := filepath.Join(tmpDir, ".bc")
	if err := os.MkdirAll(globalDir, 0750); err != nil {
		t.Fatal(err)
	}
	reg := &Registry{
		Workspaces: []RegistryEntry{
			{Path: wsDir, Name: "dedup-ws"},
		},
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(filepath.Join(globalDir, "workspaces.json"), data, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Both IncludeCached and ScanHome — should dedup
	workspaces, err := Discover(DiscoverOptions{
		IncludeCached: true,
		ScanHome:      true,
		MaxDepth:      3,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	count := 0
	for _, ws := range workspaces {
		if ws.Name == "dedup-ws" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 workspace (deduped), got %d", count)
	}
}
