// Package workspace provides workspace/project management.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/db"
)

// ConfigVersion is the current config schema version.
const ConfigVersion = 2

// PrefsFileName is the global preferences filename. The one and only
// config file mycel reads lives at ~/.mycel/prefs.json.
const PrefsFileName = "prefs.json"

// PrefsPath returns the absolute path of the global preferences file
// (~/.mycel/prefs.json, respecting MYCEL_HOME).
func PrefsPath() (string, error) {
	return globalPath(PrefsFileName)
}

// Config represents the JSON-based workspace configuration for bc.
type Config struct { //nolint:govet // field order matches JSON/API contract
	User      UserConfig      `json:"user"`
	Providers ProvidersConfig `json:"providers"`
	// Apps holds connected external integrations keyed by instance name
	// ("slack", "telegram:alerts"). Secret fields never appear here —
	// they live in the vault under app:<instance>:<key>.
	Apps map[string]app.InstanceConfig `json:"apps,omitempty"`
	// Budgets holds cost budget thresholds keyed by scope
	// ("workspace", "agent:<id>"). Spend is computed from provider
	// sources and evaluated against these limits.
	Budgets map[string]cost.BudgetConfig `json:"budgets,omitempty"`
	Runtime RuntimeConfig                `json:"runtime"`
	Storage StorageConfig                `json:"storage"`
	Server  ServerConfig                 `json:"server"`
	Logs    LogsConfig                   `json:"logs"`
	UI      UIConfig                     `json:"ui"`
	// InjectedInstructions is mycel-authored guidance appended to every
	// agent's prompt file at spawn time. Never contains secret values.
	InjectedInstructions string `json:"injected_instructions"`
	Version              int    `json:"version"`
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

// DBStorageSettings converts the workspace storage config into the
// pkg/db settings shape consumed by the global connection. Returns nil
// for a nil config so callers can pass it straight to db.Global.
func (c *Config) DBStorageSettings() *db.StorageSettings {
	if c == nil {
		return nil
	}
	return &db.StorageSettings{
		Default: c.Storage.Default,
		SQLite:  db.SQLiteSettings{Path: c.Storage.SQLite.Path},
		Timescale: db.TimescaleSettings{
			Host:     c.Storage.Timescale.Host,
			Port:     c.Storage.Timescale.Port,
			User:     c.Storage.Timescale.User,
			Password: c.Storage.Timescale.Password,
			Database: c.Storage.Timescale.Database,
		},
	}
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
				Image:            "mycel-agent-claude:latest",
				Network:          "bc-net",
				DockerSocketPath: "/var/run/docker.sock",
				CPUs:             2,
				MemoryMB:         4096,
			},
			Tmux: TmuxRuntimeConfig{
				SessionPrefix: "mycel",
				HistoryLimit:  10000,
				DefaultShell:  "/bin/bash",
			},
		},
		Providers: ProvidersConfig{
			Default: "claude",
			Providers: map[string]ProviderConfig{
				"claude": {Command: "claude --dangerously-skip-permissions"},
				"agy":    {Command: "agy --dangerously-skip-permissions"},
			},
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
			Path:     "",       // empty = StateDir/logs (supports ~/.mycel/ layout)
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
// If path is a directory, LoadConfig treats it as a state dir and reads
// <path>/prefs.json. Loading never writes to disk; prefs.json is only
// written by an explicit Save().
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

// loadConfigFromDir reads <stateDir>/prefs.json.
func loadConfigFromDir(stateDir string) (*Config, error) {
	prefs := filepath.Join(stateDir, PrefsFileName)
	data, err := os.ReadFile(prefs) //nolint:gosec // callsite-constructed
	if err != nil {
		return nil, fmt.Errorf("failed to read config %s: %w", prefs, err)
	}
	return ParseConfig(data)
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
	tmp, err := os.CreateTemp(dir, ".prefs-*.json.tmp")
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
