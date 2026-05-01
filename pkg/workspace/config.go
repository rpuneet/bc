// Package workspace provides workspace/project management.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/log"
)

// logConfigPromoteInfo is called when settings.json has been successfully
// promoted to preferences.json. Kept as a small helper so test stubs can
// replace it without noise.
func logConfigPromoteInfo(stateDir string) {
	log.Info("promoted settings.json to preferences.json", "state_dir", stateDir)
}

// logConfigPromoteWarn is called when a legacy settings.json could be
// read but the preferences.json write failed. The legacy file remains
// authoritative until the next successful save.
func logConfigPromoteWarn(stateDir string, err error) {
	log.Warn("failed to promote settings.json to preferences.json", "state_dir", stateDir, "error", err)
}

// ConfigVersion is the current config schema version.
const ConfigVersion = 2

// Preferences / settings filename constants.
//
// Before M11c: every workspace stored its config at <StateDir>/settings.json.
// From M11c onward the canonical filename is preferences.json; the legacy
// file is read as a fallback and promoted on first write.
const (
	// PreferencesFileName is the canonical workspace preferences filename (M11c+).
	PreferencesFileName = "preferences.json"
	// LegacySettingsFileName is the pre-M11c filename, still read for
	// backward compatibility.
	LegacySettingsFileName = "settings.json"
)

// Config represents the JSON-based workspace configuration for bc.
type Config struct { //nolint:govet // field order matches JSON/API contract
	User      UserConfig      `json:"user"`
	Providers ProvidersConfig `json:"providers"`
	Gateways  GatewaysConfig  `json:"gateways"`
	Runtime   RuntimeConfig   `json:"runtime"`
	Storage   StorageConfig   `json:"storage"`
	Server    ServerConfig    `json:"server"`
	Cron      CronConfig      `json:"cron"`
	Logs      LogsConfig      `json:"logs"`
	UI        UIConfig        `json:"ui"`
	Version   int             `json:"version"`
}

// UserConfig holds user identity settings.
type UserConfig struct {
	Name string `json:"name"`
}

// ServerConfig configures the bcd HTTP server.
type ServerConfig struct {
	Host       string `json:"host"`
	CORSOrigin string `json:"cors_origin"`
	Port       int    `json:"port"`
}

// Addr returns the host:port string for the server.
func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// RuntimeConfig configures the agent session backend.
type RuntimeConfig struct { //nolint:govet // field order matches JSON/API contract
	K8s     json.RawMessage     `json:"k8s,omitempty"` // future
	Default string              `json:"default"`       // "tmux" or "docker"
	Docker  DockerRuntimeConfig `json:"docker"`
	Tmux    TmuxRuntimeConfig   `json:"tmux"`
}

// DockerRuntimeConfig configures Docker container settings for agents.
type DockerRuntimeConfig struct { //nolint:govet // field order matches JSON/API contract
	ExtraMounts      []string `json:"extra_mounts"`
	Image            string   `json:"image"`
	Network          string   `json:"network"`
	DockerSocketPath string   `json:"docker_socket_path"`
	MemoryMB         int64    `json:"memory_mb"`
	CPUs             float64  `json:"cpus"`
}

// TmuxRuntimeConfig configures tmux session settings.
type TmuxRuntimeConfig struct {
	SessionPrefix string `json:"session_prefix"`
	DefaultShell  string `json:"default_shell"`
	HistoryLimit  int    `json:"history_limit"`
}

// ProvidersConfig configures AI agent providers.
type ProvidersConfig struct { //nolint:govet // field order matches JSON/API contract
	Default   string                    `json:"default"`
	Providers map[string]ProviderConfig `json:"providers,omitempty"`
}

// ProviderConfig defines an AI provider's configuration.
type ProviderConfig struct {
	Command string `json:"command"`
}

// CronConfig configures the cron/job scheduler.
type CronConfig struct {
	PollIntervalSeconds int `json:"poll_interval_seconds"`
	JobTimeoutSeconds   int `json:"job_timeout_seconds"`
}

// StorageConfig configures persistent storage.
type StorageConfig struct {
	Default   string                 `json:"default"` // "sqlite" or "timescale"
	SQLite    SQLiteStorageConfig    `json:"sqlite"`
	Timescale TimescaleStorageConfig `json:"timescale"`
}

// SQLiteStorageConfig configures SQLite storage.
type SQLiteStorageConfig struct {
	Path string `json:"path"`
}

// TimescaleStorageConfig configures TimescaleDB (Postgres) storage.
type TimescaleStorageConfig struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Port     int    `json:"port"`
}

// LogsConfig configures persistent session log streaming.
type LogsConfig struct {
	Path     string `json:"path"`
	MaxBytes int64  `json:"max_bytes"`
}

// UIConfig configures UI appearance.
type UIConfig struct {
	Theme       string `json:"theme"`
	Mode        string `json:"mode"`
	DefaultView string `json:"default_view"`
}

// DefaultConfig returns sensible defaults for a new workspace.
func DefaultConfig() Config {
	return Config{
		Version: ConfigVersion,
		User: UserConfig{
			Name: "",
		},
		Server: ServerConfig{
			Host:       "127.0.0.1",
			Port:       9374,
			CORSOrigin: "*",
		},
		Runtime: RuntimeConfig{
			Default: "docker",
			Docker: DockerRuntimeConfig{
				Image:            "bc-agent:latest",
				Network:          "bc-net",
				DockerSocketPath: "/var/run/docker.sock",
				CPUs:             2,
				MemoryMB:         4096,
			},
			Tmux: TmuxRuntimeConfig{
				SessionPrefix: "bc",
				HistoryLimit:  10000,
				DefaultShell:  "/bin/bash",
			},
		},
		Providers: ProvidersConfig{
			Default: "claude",
			Providers: map[string]ProviderConfig{
				"claude": {Command: "claude --dangerously-skip-permissions"},
				"gemini": {Command: "gemini --yolo"},
			},
		},
		Gateways: GatewaysConfig{},
		Cron: CronConfig{
			PollIntervalSeconds: 30,
			JobTimeoutSeconds:   300,
		},
		Storage: StorageConfig{
			Default: "sqlite",
			SQLite: SQLiteStorageConfig{
				Path: ".bc",
			},
			Timescale: TimescaleStorageConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "bc",
				Password: "bc",
				Database: "bc",
			},
		},
		Logs: LogsConfig{
			Path:     "",       // empty = StateDir/logs (supports ~/.bc/ layout)
			MaxBytes: 10485760, // 10MB
		},
		UI: UIConfig{
			Theme:       "dark",
			Mode:        "auto",
			DefaultView: "dashboard",
		},
	}
}

// LoadConfig reads and parses a JSON config file.
//
// If path is a directory, LoadConfig treats it as a state dir and looks
// for preferences.json first (M11c+), falling back to the legacy
// settings.json. When only the legacy file is present it is read in
// place; the caller is responsible for writing the promoted file via
// Save() or LoadConfig's higher-level wrappers.
//
// If path points at a file, it is read directly.
func LoadConfig(path string) (*Config, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return loadConfigFromDir(path)
	}
	data, err := os.ReadFile(path) //nolint:gosec // path provided by caller
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	return ParseConfig(data)
}

// loadConfigFromDir looks in stateDir for preferences.json first, then
// falls back to settings.json. When the legacy file is used it is
// promoted: the parsed config is re-serialized to preferences.json so
// subsequent reads find the canonical file. The legacy file is left on
// disk for the user to audit and remove manually.
func loadConfigFromDir(stateDir string) (*Config, error) {
	prefs := filepath.Join(stateDir, PreferencesFileName)
	if data, err := os.ReadFile(prefs); err == nil { //nolint:gosec // callsite-constructed
		return ParseConfig(data)
	}
	legacy := filepath.Join(stateDir, LegacySettingsFileName)
	data, err := os.ReadFile(legacy) //nolint:gosec // callsite-constructed
	if err != nil {
		return nil, fmt.Errorf("failed to read config (tried %s, %s): %w",
			PreferencesFileName, LegacySettingsFileName, err)
	}
	cfg, parseErr := ParseConfig(data)
	if parseErr != nil {
		return nil, parseErr
	}
	// Promote: write preferences.json so future reads skip the fallback.
	if saveErr := cfg.Save(prefs); saveErr != nil {
		// Not fatal — the legacy file is still valid.
		// We log here once; callers do not need to retry.
		// (Use structured log; package log is already imported elsewhere.)
		logConfigPromoteWarn(stateDir, saveErr)
	} else {
		logConfigPromoteInfo(stateDir)
	}
	return cfg, nil
}

// ParseConfig parses JSON data into a Config.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &cfg, nil
}

// GetProvider returns an AI provider's configuration by name.
func (c *Config) GetProvider(name string) *ProviderConfig {
	if c.Providers.Providers == nil {
		return nil
	}
	if cfg, ok := c.Providers.Providers[name]; ok {
		return &cfg
	}
	return nil
}

// GetDefaultProvider returns the default AI provider name.
func (c *Config) GetDefaultProvider() string {
	return c.Providers.Default
}

// HasProviderDefined checks if an AI provider is configured.
func (c *Config) HasProviderDefined(name string) bool {
	return c.GetProvider(name) != nil
}

// ListProviders returns the names of all configured AI providers.
func (c *Config) ListProviders() []string {
	if c.Providers.Providers == nil {
		return nil
	}
	names := make([]string, 0, len(c.Providers.Providers))
	for name := range c.Providers.Providers {
		names = append(names, name)
	}
	return names
}

// Save writes the config to a JSON file atomically (temp+rename).
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')

	// Write to temp file then rename for crash safety.
	tmp, err := os.CreateTemp(dir, ".settings-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	tmpName := tmp.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to rename config: %w", err)
	}

	success = true
	return nil
}

// ConfigPath returns the standard config file path for a workspace root.
// Checks the global state dir first (preferences.json then settings.json),
// then the legacy <rootDir>/.bc/ directory. When no file exists anywhere
// yet, returns the canonical preferences.json path under the global dir
// so callers writing a fresh config land in the right place.
func ConfigPath(rootDir string) string {
	if stateDir, err := GlobalStateDir(rootDir); err == nil {
		prefs := filepath.Join(stateDir, PreferencesFileName)
		if _, statErr := os.Stat(prefs); statErr == nil {
			return prefs
		}
		legacy := filepath.Join(stateDir, LegacySettingsFileName)
		if _, statErr := os.Stat(legacy); statErr == nil {
			return legacy
		}
		// Nothing on disk yet — prefer the canonical name.
		// Fall through to legacy-root check below in case caller is
		// operating on a pre-M11 workspace.
	}
	legacyBC := filepath.Join(rootDir, ".bc", PreferencesFileName)
	if _, err := os.Stat(legacyBC); err == nil {
		return legacyBC
	}
	legacyBCSettings := filepath.Join(rootDir, ".bc", LegacySettingsFileName)
	if _, err := os.Stat(legacyBCSettings); err == nil {
		return legacyBCSettings
	}
	// Default for callers writing a fresh config — global dir if possible.
	if stateDir, err := GlobalStateDir(rootDir); err == nil {
		return filepath.Join(stateDir, PreferencesFileName)
	}
	return legacyBC
}
