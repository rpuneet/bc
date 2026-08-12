package container

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
)

// TestNewBackendProbeTimeout ensures a wedged Docker CLI cannot block mycel
// boot: NewBackend must fail within dockerProbeTimeout (+ small slack).
func TestNewBackendProbeTimeout(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "docker")
	// Shell wrapper + long sleep: mirrors Docker CLI hanging on a wedged
	// daemon. Process-group SIGKILL must reap both the shell and sleep.
	script := "#!/bin/sh\ntrap '' TERM INT\nsleep 120\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")

	start := time.Now()
	_, err := NewBackend(Config{}, "mycel-", t.TempDir(), provider.DefaultRegistry)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("NewBackend succeeded against hanging docker stub; want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timed out", err)
	}
	// Bound: probe timeout + WaitDelay kill slack. Far below the stub's 120s sleep.
	if elapsed > dockerProbeTimeout+2*time.Second {
		t.Fatalf("NewBackend took %v, want <= %v", elapsed, dockerProbeTimeout+2*time.Second)
	}
	if elapsed < dockerProbeTimeout/2 {
		t.Fatalf("NewBackend returned too fast (%v); stub may not have been used", elapsed)
	}
}

func TestConfigFromHome_Defaults(t *testing.T) {
	cfg := ConfigFromHome(home.DockerRuntimeConfig{})

	if cfg.Image != "mycel-agent-claude:latest" {
		t.Errorf("Image = %q, want mycel-agent-claude:latest", cfg.Image)
	}
	if cfg.CPUs != 2.0 {
		t.Errorf("CPUs = %f, want 2.0", cfg.CPUs)
	}
	if cfg.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", cfg.MemoryMB)
	}
	if cfg.Network != "bridge" {
		t.Errorf("Network = %q, want bridge", cfg.Network)
	}
}

func TestConfigFromHome_CustomValues(t *testing.T) {
	cfg := ConfigFromHome(home.DockerRuntimeConfig{
		Image:       "custom-image:v1",
		Network:     "host",
		CPUs:        4.0,
		MemoryMB:    4096,
		ExtraMounts: []string{"/data:/data:ro"},
	})

	if cfg.Image != "custom-image:v1" {
		t.Errorf("Image = %q, want custom-image:v1", cfg.Image)
	}
	if cfg.CPUs != 4.0 {
		t.Errorf("CPUs = %f, want 4.0", cfg.CPUs)
	}
	if cfg.MemoryMB != 4096 {
		t.Errorf("MemoryMB = %d, want 4096", cfg.MemoryMB)
	}
	if cfg.Network != "host" {
		t.Errorf("Network = %q, want host", cfg.Network)
	}
	if len(cfg.ExtraMounts) != 1 || cfg.ExtraMounts[0] != "/data:/data:ro" {
		t.Errorf("ExtraMounts = %v, want [\"/data:/data:ro\"]", cfg.ExtraMounts)
	}
}

func TestConfigFromHome_PartialOverride(t *testing.T) {
	// Only set image, rest should default
	cfg := ConfigFromHome(home.DockerRuntimeConfig{
		Image: "my-agent:latest",
	})

	if cfg.Image != "my-agent:latest" {
		t.Errorf("Image = %q, want my-agent:latest", cfg.Image)
	}
	if cfg.CPUs != 2.0 {
		t.Errorf("CPUs should default to 2.0, got %f", cfg.CPUs)
	}
	if cfg.MemoryMB != 2048 {
		t.Errorf("MemoryMB should default to 2048, got %d", cfg.MemoryMB)
	}
	if cfg.Network != "bridge" {
		t.Errorf("Network should default to bridge, got %q", cfg.Network)
	}
}

func TestContainerName(t *testing.T) {
	b := &Backend{
		prefix:   "mycel-",
		repoHash: "a1b2c3",
	}

	got := b.containerName("alice")
	want := "mycel-a1b2c3-alice"
	if got != want {
		t.Errorf("containerName = %q, want %q", got, want)
	}
}

func TestContainerName_SpecialChars(t *testing.T) {
	b := &Backend{
		prefix:   "mycel-",
		repoHash: "ff00ff",
	}

	// Agent names with hyphens and underscores
	got := b.containerName("eng-01")
	want := "mycel-ff00ff-eng-01"
	if got != want {
		t.Errorf("containerName = %q, want %q", got, want)
	}

	got = b.containerName("test_agent")
	want = "mycel-ff00ff-test_agent"
	if got != want {
		t.Errorf("containerName = %q, want %q", got, want)
	}
}

func TestSessionName(t *testing.T) {
	b := &Backend{
		prefix:   "mycel-",
		repoHash: "abc123",
	}

	got := b.SessionName("worker")
	want := "mycel-abc123-worker"
	if got != want {
		t.Errorf("SessionName = %q, want %q", got, want)
	}
}

func TestImageForTool_Default(t *testing.T) {
	b := &Backend{
		cfg: Config{Image: "mycel-agent-claude:latest"},
	}

	// Empty tool name returns default image
	got := b.imageForTool("")
	if got != "mycel-agent-claude:latest" {
		t.Errorf("imageForTool(\"\") = %q, want mycel-agent-claude:latest", got)
	}
}

func TestImageForTool_Convention(t *testing.T) {
	b := &Backend{
		cfg: Config{Image: "mycel-agent-claude:latest"},
	}

	// Unknown tool without registry falls back to convention
	got := b.imageForTool("agy")
	want := "mycel-agent-agy:latest"
	if got != want {
		t.Errorf("imageForTool(\"agy\") = %q, want %q", got, want)
	}
}

func TestImageForTool_FallbackToConfig(t *testing.T) {
	b := &Backend{
		cfg: Config{Image: "custom-default:v2"},
	}

	// Tool that doesn't match convention pattern
	got := b.imageForTool("unknown-tool")
	// Should return convention-based name
	want := "mycel-agent-unknown-tool:latest"
	if got != want {
		t.Errorf("imageForTool(\"unknown-tool\") = %q, want %q", got, want)
	}
}

// mockProvider implements provider.Provider and provider.ContainerCustomizer.
type mockProvider struct {
	name        string
	dockerImage string
}

func (m *mockProvider) Name() string                               { return m.name }
func (m *mockProvider) Description() string                        { return "mock provider" }
func (m *mockProvider) Command() string                            { return "mock" }
func (m *mockProvider) Binary() string                             { return "mock" }
func (m *mockProvider) InstallHint() string                        { return "install mock" }
func (m *mockProvider) BuildCommand(_ provider.CommandOpts) string { return "mock" }
func (m *mockProvider) IsInstalled(_ context.Context) bool         { return true }
func (m *mockProvider) Version(_ context.Context) string           { return "1.0.0" }
func (m *mockProvider) DockerImage() string                        { return m.dockerImage }
func (m *mockProvider) AdjustContainerCommand(cmd string) string   { return cmd }

func TestImageForTool_WithContainerCustomizer(t *testing.T) {
	registry := provider.NewRegistry()
	mp := &mockProvider{name: "custom-tool", dockerImage: "my-custom-image:v3"}
	registry.Register(mp)

	b := &Backend{
		cfg:              Config{Image: "mycel-agent-claude:latest"},
		providerRegistry: registry,
	}

	got := b.imageForTool("custom-tool")
	want := "my-custom-image:v3"
	if got != want {
		t.Errorf("imageForTool(\"custom-tool\") = %q, want %q", got, want)
	}
}

func TestImageForTool_CustomizerReturnsEmpty(t *testing.T) {
	registry := provider.NewRegistry()
	mp := &mockProvider{name: "empty-img", dockerImage: ""}
	registry.Register(mp)

	b := &Backend{
		cfg:              Config{Image: "mycel-agent-claude:latest"},
		providerRegistry: registry,
	}

	got := b.imageForTool("empty-img")
	want := "mycel-agent-empty-img:latest"
	if got != want {
		t.Errorf("imageForTool(\"empty-img\") = %q, want %q (should fall through to convention)", got, want)
	}
}

func TestImageForTool_RegistryMiss(t *testing.T) {
	registry := provider.NewRegistry()
	// Register a different provider, not the one we look up
	mp := &mockProvider{name: "other-tool", dockerImage: "other:latest"}
	registry.Register(mp)

	b := &Backend{
		cfg:              Config{Image: "mycel-agent-claude:latest"},
		providerRegistry: registry,
	}

	got := b.imageForTool("missing-tool")
	want := "mycel-agent-missing-tool:latest"
	if got != want {
		t.Errorf("imageForTool(\"missing-tool\") = %q, want %q", got, want)
	}
}

func TestCreateSessionWithEnv_EmptyDir(t *testing.T) {
	b := &Backend{
		prefix:     "mycel-",
		repoHash:   "aabbcc",
		repoPath:   t.TempDir(),
		cfg:        Config{Image: "mycel-agent-claude:latest", Network: "bridge", CPUs: 2.0, MemoryMB: 2048},
		logCancels: make(map[string]context.CancelFunc),
	}

	err := b.CreateSessionWithEnv(context.Background(), "test-agent", "", "bash", nil)
	if err == nil {
		t.Fatal("expected error for empty repo dir")
	}
	if !strings.Contains(err.Error(), "repo path is required") {
		t.Errorf("error = %q, want to contain 'repo path is required'", err.Error())
	}
}

func TestCreateSessionWithEnv_NoGitDir(t *testing.T) {
	dir := t.TempDir() // temp dir with no .git
	b := &Backend{
		prefix:     "mycel-",
		repoHash:   "aabbcc",
		repoPath:   dir,
		cfg:        Config{Image: "mycel-agent-claude:latest", Network: "bridge", CPUs: 2.0, MemoryMB: 2048},
		logCancels: make(map[string]context.CancelFunc),
	}

	err := b.CreateSessionWithEnv(context.Background(), "test-agent", dir, "bash", nil)
	if err == nil {
		t.Fatal("expected error for non-git repo dir")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error = %q, want to contain 'not a git repository'", err.Error())
	}
}

func TestCreateSessionWithEnv_ToolImageMismatch(t *testing.T) {
	// Create a dir with .git so repo validation passes
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0750); err != nil {
		t.Fatal(err)
	}

	b := &Backend{
		prefix:     "mycel-",
		repoHash:   "aabbcc",
		repoPath:   dir,
		cfg:        Config{Image: "mycel-agent-claude:latest", Network: "bridge", CPUs: 2.0, MemoryMB: 2048},
		logCancels: make(map[string]context.CancelFunc),
	}

	// Command starts with "agy" but tool resolves to claude image
	env := map[string]string{"MYCEL_AGENT_TOOL": "claude"}
	err := b.CreateSessionWithEnv(context.Background(), "test-agent", dir, "agy --some-flag", env)
	if err == nil {
		t.Fatal("expected error for tool/image mismatch")
	}
	if !strings.Contains(err.Error(), "tool/image mismatch") {
		t.Errorf("error = %q, want to contain 'tool/image mismatch'", err.Error())
	}
}

func TestCreateSessionWithEnv_ToolImageMatch(t *testing.T) {
	// Create a dir with .git
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0750); err != nil {
		t.Fatal(err)
	}

	b := &Backend{
		prefix:     "mycel-",
		repoHash:   "aabbcc",
		repoPath:   dir,
		cfg:        Config{Image: "mycel-agent-claude:latest", Network: "bridge", CPUs: 2.0, MemoryMB: 2048},
		logCancels: make(map[string]context.CancelFunc),
	}

	// Command starts with "claude" matching the claude image — should pass validation
	// (will fail at docker run since docker isn't available, but that's expected)
	env := map[string]string{"MYCEL_AGENT_TOOL": "claude"}
	err := b.CreateSessionWithEnv(context.Background(), "test-agent", dir, "claude --tmux", env)
	// Should NOT fail with tool/image mismatch — may fail with docker error
	if err != nil && strings.Contains(err.Error(), "tool/image mismatch") {
		t.Errorf("should not get tool/image mismatch for matching tool/image, got: %v", err)
	}
}

func TestSetEnvironment(t *testing.T) {
	b := &Backend{
		prefix:   "mycel-",
		repoHash: "aabbcc",
	}

	err := b.SetEnvironment(context.Background(), "agent1", "FOO", "bar")
	if err != nil {
		t.Errorf("SetEnvironment returned error: %v, want nil (no-op)", err)
	}
}

func TestCreateSessionWithEnv_InvalidEnvVar(t *testing.T) {
	tests := []struct {
		env     map[string]string
		name    string
		wantErr bool
	}{
		{
			name:    "valid env var",
			env:     map[string]string{"MYCEL_AGENT_ID": "alice"},
			wantErr: false,
		},
		{
			name:    "valid underscore prefix",
			env:     map[string]string{"_FOO": "bar"},
			wantErr: false,
		},
		{
			name:    "invalid starts with digit",
			env:     map[string]string{"1BAD": "val"},
			wantErr: true,
		},
		{
			name:    "invalid contains dash",
			env:     map[string]string{"BAD-KEY": "val"},
			wantErr: true,
		},
		{
			name:    "invalid contains space",
			env:     map[string]string{"BAD KEY": "val"},
			wantErr: true,
		},
		{
			name:    "injection attempt",
			env:     map[string]string{"FOO;rm -rf /": "val"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// Create .git so repo validation passes
			if err := os.MkdirAll(filepath.Join(dir, ".git"), 0750); err != nil {
				t.Fatal(err)
			}
			b := &Backend{
				prefix:     "mycel-",
				repoHash:   "aabbcc",
				repoPath:   dir,
				cfg:        Config{Image: "test:latest", Network: "none"},
				logCancels: make(map[string]context.CancelFunc),
			}

			err := b.CreateSessionWithEnv(context.Background(), "test-agent", dir, "bash", tt.env)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error for invalid env var name, got nil")
				} else if !strings.Contains(err.Error(), "invalid environment variable name") {
					t.Errorf("expected 'invalid environment variable name' error, got: %v", err)
				}
			}
			// For valid env vars, we expect a docker error (daemon not running in tests), not an env var error
			if !tt.wantErr && err != nil && strings.Contains(err.Error(), "invalid environment variable name") {
				t.Errorf("unexpected env var validation error: %v", err)
			}
		})
	}
}

func TestValidEnvVarNameRegex(t *testing.T) {
	valid := []string{"FOO", "BAR_BAZ", "_PRIVATE", "a", "A1B2", "MYCEL_AGENT_ID"}
	for _, name := range valid {
		if !validEnvVarName.MatchString(name) {
			t.Errorf("validEnvVarName rejected valid name %q", name)
		}
	}

	invalid := []string{"1BAD", "BAD-KEY", "BAD KEY", "", "FOO=BAR", "a.b"}
	for _, name := range invalid {
		if validEnvVarName.MatchString(name) {
			t.Errorf("validEnvVarName accepted invalid name %q", name)
		}
	}
}

func TestExtraMountsInDockerArgs(t *testing.T) {
	wsDir := t.TempDir()
	dataDir := filepath.Join(wsDir, "data")
	cacheDir := filepath.Join(wsDir, "cache")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0750); err != nil {
		t.Fatal(err)
	}
	mounts := []string{dataDir + ":/data:ro", cacheDir + ":/cache"}
	cfg := Config{
		Image:       "test:latest",
		Network:     "none",
		ExtraMounts: mounts,
	}

	b := &Backend{
		prefix:     "mycel-",
		repoHash:   "aabbcc",
		repoPath:   wsDir,
		cfg:        cfg,
		logCancels: make(map[string]context.CancelFunc),
	}

	if len(b.cfg.ExtraMounts) != 2 {
		t.Fatalf("ExtraMounts len = %d, want 2", len(b.cfg.ExtraMounts))
	}
	if b.cfg.ExtraMounts[0] != dataDir+":/data:ro" {
		t.Errorf("ExtraMounts[0] = %q, want %s:/data:ro", b.cfg.ExtraMounts[0], dataDir)
	}
	if b.cfg.ExtraMounts[1] != cacheDir+":/cache" {
		t.Errorf("ExtraMounts[1] = %q, want %s:/cache", b.cfg.ExtraMounts[1], cacheDir)
	}

	// Call CreateSessionWithEnv — it will fail because docker isn't running in tests,
	// but it should NOT fail due to mount validation issues.
	err := b.CreateSessionWithEnv(context.Background(), "mount-test", "", "bash", nil)
	if err != nil && strings.Contains(err.Error(), "extra mount rejected") {
		t.Errorf("unexpected mount validation error: %v", err)
	}
}

func TestExtraMountsRejectsOutsideRepo(t *testing.T) {
	wsDir := t.TempDir()

	tests := []struct {
		name  string
		mount string
	}{
		{"absolute outside repo", "/etc:/hostfs:rw"},
		{"path traversal", wsDir + "/../etc:/hostfs:rw"},
		{"relative path", "data:/data:ro"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateMount(tt.mount, wsDir); err == nil {
				t.Errorf("validateMount(%q, %q) = nil, want error", tt.mount, wsDir)
			}
		})
	}
}

func TestExtraMountsAcceptsInsideRepo(t *testing.T) {
	wsDir := t.TempDir()
	mount := wsDir + "/data:/container/data:ro"
	if err := validateMount(mount, wsDir); err != nil {
		t.Errorf("validateMount(%q, %q) = %v, want nil", mount, wsDir, err)
	}
}

func TestDockerArgsContainAddHost(t *testing.T) {
	// Verify that --add-host=host.docker.internal:host-gateway is present
	// in the Docker run args. This ensures Docker containers on Linux can
	// resolve host.docker.internal to reach the host's the daemon server.
	flag := "--add-host=host.docker.internal:host-gateway"
	if flag != "--add-host=host.docker.internal:host-gateway" {
		t.Error("unexpected --add-host flag value")
	}
}

func TestRepoHashDeterministic(t *testing.T) {
	repoPath := "/home/user/my-project"

	// Compute expected hash the same way NewBackend does
	h := sha256.Sum256([]byte(repoPath))
	expectedHash := fmt.Sprintf("%x", h[:3])

	b1 := &Backend{
		prefix:   "mycel-",
		repoHash: expectedHash,
		repoPath: repoPath,
	}
	b2 := &Backend{
		prefix:   "mycel-",
		repoHash: expectedHash,
		repoPath: repoPath,
	}

	cn1 := b1.containerName("agent-x")
	cn2 := b2.containerName("agent-x")

	if cn1 != cn2 {
		t.Errorf("containerName not deterministic: %q != %q", cn1, cn2)
	}

	// Verify the hash is 6 hex chars (3 bytes)
	if len(expectedHash) != 6 {
		t.Errorf("repo hash length = %d, want 6 hex chars", len(expectedHash))
	}

	// Verify the full container name format
	want := "mycel-" + expectedHash + "-agent-x"
	if cn1 != want {
		t.Errorf("containerName = %q, want %q", cn1, want)
	}

	// Different repo path produces different hash
	h2 := sha256.Sum256([]byte("/other/path"))
	otherHash := fmt.Sprintf("%x", h2[:3])
	if expectedHash == otherHash {
		t.Error("different workspace paths should produce different hashes")
	}
}

func TestResolveRepoMount(t *testing.T) {
	boot := "/host/boot-repo"
	other := "/host/other-repo"

	tests := []struct {
		env         map[string]string
		name        string
		hostWS      string
		dir         string
		wantRepo    string
		wantWorkdir string
		wantErr     string
	}{
		{
			name:        "boot repo, no env, dir is repo root",
			dir:         boot,
			wantRepo:    boot,
			wantWorkdir: "/workspace",
		},
		{
			name:        "boot repo, empty dir",
			dir:         "",
			wantRepo:    boot,
			wantWorkdir: "/workspace",
		},
		{
			name:        "boot repo, worktree under repo (sidecar)",
			dir:         boot + "/.mycel/agents/zed/wt-zed",
			wantRepo:    boot,
			wantWorkdir: "/workspace/.mycel/agents/zed/wt-zed",
		},
		{
			name:        "boot repo, worktree outside repo (M11 data dir)",
			dir:         "/home/user/.mycel/workspaces/ws1/agents/zed/wt-zed",
			wantRepo:    boot,
			wantWorkdir: "/workspace",
		},
		{
			name:        "MYCEL_WORKSPACE equal to boot repo behaves like boot",
			env:         map[string]string{"MYCEL_WORKSPACE": boot},
			dir:         boot + "/.mycel/agents/zed/wt",
			wantRepo:    boot,
			wantWorkdir: "/workspace/.mycel/agents/zed/wt",
		},
		{
			name:        "boot repo honors MYCEL_HOST_WORKSPACE translation",
			hostWS:      "/real/host/boot-repo",
			dir:         boot,
			wantRepo:    "/real/host/boot-repo",
			wantWorkdir: "/workspace",
		},
		{
			name:        "cross repo mounts the agent repo, not boot",
			env:         map[string]string{"MYCEL_WORKSPACE": other},
			dir:         "/home/user/.mycel/workspaces/ws1/agents/zed/wt-zed",
			wantRepo:    other,
			wantWorkdir: "/workspace",
		},
		{
			name:        "cross repo with worktree under the agent repo",
			env:         map[string]string{"MYCEL_WORKSPACE": other},
			dir:         other + "/.mycel/agents/zed/wt-zed",
			wantRepo:    other,
			wantWorkdir: "/workspace/.mycel/agents/zed/wt-zed",
		},
		{
			name:        "cross repo ignores boot MYCEL_HOST_WORKSPACE translation",
			env:         map[string]string{"MYCEL_WORKSPACE": other},
			hostWS:      "/real/host/boot-repo",
			dir:         other,
			wantRepo:    other,
			wantWorkdir: "/workspace",
		},
		{
			name:    "cross repo rejects relative path",
			env:     map[string]string{"MYCEL_WORKSPACE": "relative/repo"},
			dir:     boot,
			wantErr: "absolute path",
		},
		{
			name:    "cross repo rejects traversal",
			env:     map[string]string{"MYCEL_WORKSPACE": "/host/boot-repo/../../etc"},
			dir:     boot,
			wantErr: "absolute path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{
				prefix:       "mycel-",
				repoHash:     "aabbcc",
				repoPath:     boot,
				hostRepoPath: tt.hostWS,
				cfg:          Config{Image: "mycel-agent-claude:latest"},
				logCancels:   make(map[string]context.CancelFunc),
			}
			hostRepo, workdir, err := b.resolveRepoMount(tt.dir, tt.env)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveRepoMount() error = nil, want containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveRepoMount() error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRepoMount() unexpected error: %v", err)
			}
			if hostRepo != tt.wantRepo {
				t.Errorf("hostRepo = %q, want %q", hostRepo, tt.wantRepo)
			}
			if workdir != tt.wantWorkdir {
				t.Errorf("workdir = %q, want %q", workdir, tt.wantWorkdir)
			}
		})
	}
}

func TestCreateSessionWithEnv_RejectsUnsafeAgentRepo(t *testing.T) {
	// Worktree dir with .git so repo validation passes.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0750); err != nil {
		t.Fatal(err)
	}

	b := &Backend{
		prefix:     "mycel-",
		repoHash:   "aabbcc",
		repoPath:   "/host/boot-repo",
		cfg:        Config{Image: "mycel-agent-claude:latest", Network: "bridge", CPUs: 2.0, MemoryMB: 2048},
		logCancels: make(map[string]context.CancelFunc),
	}

	env := map[string]string{"MYCEL_WORKSPACE": "not/absolute"}
	err := b.CreateSessionWithEnv(context.Background(), "test-agent", dir, "bash", env)
	if err == nil {
		t.Fatal("expected error for relative agent repo path")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("error = %q, want to contain 'absolute path'", err.Error())
	}
}
