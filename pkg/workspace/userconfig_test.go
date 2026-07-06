package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUserRCConfigPath(t *testing.T) {
	path := UserRCConfigPath()
	if path == "" {
		t.Skip("no home directory available")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got: %s", path)
	}
	if filepath.Base(path) != ".bcrc" {
		t.Errorf("expected .bcrc filename, got: %s", filepath.Base(path))
	}
}
func TestDefaultUserRCConfig(t *testing.T) {
	cfg := DefaultUserRCConfig()
	if cfg.User.Nickname != "@bc" {
		t.Errorf("expected default nickname %s, got: %s", "@bc", cfg.User.Nickname)
	}
	if cfg.Defaults.DefaultRole != "engineer" {
		t.Errorf("expected default role 'engineer', got: %s", cfg.Defaults.DefaultRole)
	}
	if !cfg.Defaults.AutoStartRoot {
		t.Error("expected auto_start_root to be true by default")
	}
	if len(cfg.Tools.Preferred) == 0 {
		t.Error("expected preferred tools to be set")
	}
}
func TestParseUserRCConfig(t *testing.T) {
	data := []byte(`{"user":{"nickname":"@alice"},"defaults":{"default_role":"manager","auto_start_root":false},"tools":{"preferred":["cursor","claude-code"]}}`)
	cfg, err := ParseUserRCConfig(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cfg.User.Nickname != "@alice" {
		t.Errorf("expected nickname '@alice', got: %s", cfg.User.Nickname)
	}
	if cfg.Defaults.DefaultRole != "manager" {
		t.Errorf("expected default role 'manager', got: %s", cfg.Defaults.DefaultRole)
	}
	if cfg.Defaults.AutoStartRoot {
		t.Error("expected auto_start_root to be false")
	}
	if len(cfg.Tools.Preferred) != 2 {
		t.Errorf("expected 2 preferred tools, got: %d", len(cfg.Tools.Preferred))
	}
}
func TestUserRCConfigSaveAndLoad(t *testing.T) {
	// Create a temp directory to use as home
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	// Create and save a config
	cfg := DefaultUserRCConfig()
	cfg.User.Nickname = "@testuser"
	err := cfg.Save()
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	// Verify file exists
	path := filepath.Join(tmpDir, ".bcrc")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("config file not created: %v", statErr)
	}
	// Load and verify
	loaded, err := LoadUserRCConfig()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded config is nil")
	}
	if loaded.User.Nickname != "@testuser" {
		t.Errorf("expected nickname '@testuser', got: %s", loaded.User.Nickname)
	}
}
func TestMergeWithUserRC(t *testing.T) {
	// Create a workspace config with default nickname
	wsCfg := DefaultConfig()
	// Create a user config with custom nickname
	rcCfg := &UserRCConfig{
		User: UserRCUserConfig{
			Nickname: "@custom",
		},
	}
	// Merge
	wsCfg.MergeWithUserRC(rcCfg)
	// User RC nickname should be used since workspace has default
	if wsCfg.User.Name != "@custom" {
		t.Errorf("expected merged nickname '@custom', got: %s", wsCfg.User.Name)
	}
}
func TestHasProviderDefined(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.HasProviderDefined("agy") {
		t.Error("expected agy to be available")
	}
	if !cfg.HasProviderDefined("claude") {
		t.Error("expected claude to be available")
	}
	if cfg.HasProviderDefined("unknown-tool") {
		t.Error("expected unknown-tool to not be available")
	}
}
func TestHasProviderDefinedAllTypes(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		cfg          Config
		want         bool
	}{
		{
			name:         "claude defined",
			providerName: "claude",
			cfg: Config{
				Providers: ProvidersConfig{Providers: map[string]ProviderConfig{"claude": {Command: "claude"}}},
			},
			want: true,
		},
		{
			name:         "cursor defined",
			providerName: "cursor",
			cfg: Config{
				Providers: ProvidersConfig{Providers: map[string]ProviderConfig{"cursor": {Command: "cursor"}}},
			},
			want: true,
		},
		{
			name:         "codex defined",
			providerName: "codex",
			cfg: Config{
				Providers: ProvidersConfig{Providers: map[string]ProviderConfig{"codex": {Command: "codex"}}},
			},
			want: true,
		},
		{
			name:         "agy defined",
			providerName: "agy",
			cfg: Config{
				Providers: ProvidersConfig{Providers: map[string]ProviderConfig{"agy": {Command: "agy"}}},
			},
			want: true,
		},
		{
			name:         "custom provider defined",
			providerName: "my-provider",
			cfg: Config{
				Providers: ProvidersConfig{
					Providers: map[string]ProviderConfig{"my-provider": {Command: "test"}},
				},
			},
			want: true,
		},
		{
			name:         "custom provider not in map",
			providerName: "my-provider",
			cfg:          Config{},
			want:         false,
		},
		{
			name:         "nil providers",
			providerName: "claude",
			cfg:          Config{},
			want:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.HasProviderDefined(tt.providerName)
			if got != tt.want {
				t.Errorf("HasProviderDefined(%q) = %v, want %v", tt.providerName, got, tt.want)
			}
		})
	}
}
func TestUserRCExists(t *testing.T) {
	// Test with temp directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	// Initially should not exist
	if UserRCExists() {
		t.Error("UserRCExists should return false when .bcrc doesn't exist")
	}
	// Create .bcrc file
	rcPath := filepath.Join(tmpDir, ".bcrc")
	if err := os.WriteFile(rcPath, []byte("[user]\nnickname = \"@test\""), 0600); err != nil {
		t.Fatalf("failed to create .bcrc: %v", err)
	}
	// Now should exist
	if !UserRCExists() {
		t.Error("UserRCExists should return true when .bcrc exists")
	}
}
func TestGetPreferredTool(t *testing.T) {
	tests := []struct {
		rc       *UserRCConfig
		name     string
		expected string
		cfg      Config
	}{
		{
			name: "nil rc returns default",
			cfg: Config{
				Providers: ProvidersConfig{
					Default: "claude",
				},
			},
			rc:       nil,
			expected: "claude",
		},
		{
			name: "empty preferred list returns default",
			cfg: Config{
				Providers: ProvidersConfig{
					Default: "claude",
				},
			},
			rc:       &UserRCConfig{},
			expected: "claude",
		},
		{
			name: "first preferred tool available",
			cfg: Config{
				Providers: ProvidersConfig{
					Default:   "claude",
					Providers: map[string]ProviderConfig{"claude": {Command: "claude"}, "cursor": {Command: "cursor"}},
				},
			},
			rc: &UserRCConfig{
				Tools: UserRCToolsConfig{
					Preferred: []string{"cursor", "claude"},
				},
			},
			expected: "cursor",
		},
		{
			name: "skip unavailable provider",
			cfg: Config{
				Providers: ProvidersConfig{
					Default: "claude",
				},
			},
			rc: &UserRCConfig{
				Tools: UserRCToolsConfig{
					Preferred: []string{"cursor", "claude"},
				},
			},
			expected: "claude",
		},
		{
			name: "no preferred providers available",
			cfg: Config{
				Providers: ProvidersConfig{
					Default: "agy",
				},
			},
			rc: &UserRCConfig{
				Tools: UserRCToolsConfig{
					Preferred: []string{"cursor", "claude"},
				},
			},
			expected: "agy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.GetPreferredTool(tt.rc)
			if got != tt.expected {
				t.Errorf("GetPreferredTool() = %q, want %q", got, tt.expected)
			}
		})
	}
}
func TestLoadUserRCConfigNotFound(t *testing.T) {
	// Test with temp directory that has no .bcrc
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	cfg, err := LoadUserRCConfig()
	if err != nil {
		t.Errorf("LoadUserRCConfig should not error for missing file: %v", err)
	}
	if cfg != nil {
		t.Error("LoadUserRCConfig should return nil for missing file")
	}
}
func TestMergeWithUserRCNil(t *testing.T) {
	cfg := DefaultConfig()
	originalName := cfg.User.Name
	// Merge with nil should not change anything
	cfg.MergeWithUserRC(nil)
	if cfg.User.Name != originalName {
		t.Errorf("MergeWithUserRC(nil) changed name from %q to %q", originalName, cfg.User.Name)
	}
}
func TestMergeWithUserRCPreserveWorkspace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.User.Name = "@workspace-user"
	rc := &UserRCConfig{
		User: UserRCUserConfig{
			Nickname: "@rc-user",
		},
	}
	cfg.MergeWithUserRC(rc)
	// Workspace name should be preserved
	if cfg.User.Name != "@workspace-user" {
		t.Errorf("MergeWithUserRC changed workspace name to %q", cfg.User.Name)
	}
}
func TestParseUserRCConfigInvalidTOML(t *testing.T) {
	_, err := ParseUserRCConfig([]byte("{invalid toml!!!"))
	if err == nil {
		t.Error("ParseUserRCConfig should fail on invalid TOML")
	}
}
func TestUserRCConfigSavePathEmpty(t *testing.T) {
	// Unset HOME to simulate path failure
	t.Setenv("HOME", "")
	cfg := DefaultUserRCConfig()
	err := cfg.Save()
	if err == nil {
		t.Error("Save should fail when home directory is empty")
	}
}
