package home

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
const ConfigVersion = 3

// PrefsFileName is the global preferences filename. The one and only
// config file mycel reads lives at ~/.mycel/prefs.json.
const PrefsFileName = "prefs.json"

// PrefsPath returns the absolute path of the global preferences file
// (~/.mycel/prefs.json, respecting MYCEL_HOME).
func PrefsPath() (string, error) {
	return globalPath(PrefsFileName)
}

// Config represents the JSON-based global mycel configuration (~/.mycel/prefs.json).
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
	// Notifications holds delivery preferences: which connected channel
	// should reach the operator, and whether delivery is on at all.
	Notifications NotificationsConfig `json:"notifications"`
	// Onboarding tracks the first-run setup wizard's progress so it can
	// resume where the user left off. Config-only — the wizard never
	// touches agents, secrets, or the database.
	Onboarding OnboardingConfig `json:"onboarding"`
	// InjectedInstructions is mycel-authored guidance appended to every
	// agent's prompt file at spawn time. Never contains secret values.
	InjectedInstructions string `json:"injected_instructions"`
	Version              int    `json:"version"`
}

// OnboardingConfig records where the user is in the first-run setup wizard.
// Step is the id of the last-visited step ("welcome", "runtime", …);
// Completed lists the steps the user finished, ending with "done" once the
// wizard is fully complete.
type OnboardingConfig struct {
	Step      string   `json:"step"`
	Completed []string `json:"completed"`
}

// OnboardingComplete reports whether the wizard has been marked finished.
func (c *OnboardingConfig) OnboardingComplete() bool {
	for _, s := range c.Completed {
		if s == "done" {
			return true
		}
	}
	return false
}

// UserConfig holds user identity settings.
type UserConfig struct {
	Name string `json:"name"`
}

// ServerConfig configures the daemon HTTP server.
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
	Default string              `json:"default"` // "tmux" or "docker"
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
	Default string `json:"default"`
	// DefaultModel is the provider model id new agents use when none is
	// chosen (e.g. "claude-sonnet-4"). Empty = the provider's own default.
	DefaultModel string                    `json:"default_model,omitempty"`
	Providers    map[string]ProviderConfig `json:"providers,omitempty"`
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

// DBStorageSettings converts the storage config into the
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

// NotificationsConfig holds the operator's delivery preferences. It records
// which connected channel ("slack:general", "telegram:alerts") should reach
// the operator and whether delivery is enabled. Channel identities and
// per-agent subscriptions live in the DB — only the top-level preference
// lives here.
type NotificationsConfig struct {
	// DefaultChannel is the "app:channel" key notifications route to by
	// default. Empty = no default chosen yet ("decide later").
	DefaultChannel string `json:"default_channel"`
	// Enabled turns operator delivery on or off globally.
	Enabled bool `json:"enabled"`
}

// DefaultConfig returns sensible defaults for a fresh mycel home.
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
			Default: "tmux",
			Docker: DockerRuntimeConfig{
				Image:            "mycel-agent-claude:latest",
				Network:          "mycel-net",
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
			// Seed every built-in provider the UI can pick as fleet default.
			// Validate requires providers.default to exist in this map; omitting
			// codex/cursor/pi made the setup/tools picker reject those choices
			// with "references undefined provider" (#3720).
			Providers: map[string]ProviderConfig{
				"claude": {Command: "claude --dangerously-skip-permissions"},
				"agy":    {Command: "agy --dangerously-skip-permissions"},
				"cursor": {Command: "cursor-agent --trust"},
				"codex":  {Command: "codex --full-auto"},
				"pi":     {Command: "pi"},
			},
		},
		Storage: StorageConfig{
			Default: "sqlite",
			SQLite: SQLiteStorageConfig{
				Path: ".mycel",
			},
			Timescale: TimescaleStorageConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "mycel",
				Password: "mycel",
				Database: "mycel",
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
		Notifications: NotificationsConfig{
			Enabled: true,
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

// BuiltinProviderCommands is the command seed for every provider the UI
// can offer as providers.default. Kept in sync with pkg/provider defaults
// (claude/agy/cursor/codex/pi). An empty Command is allowed at runtime —
// the provider package still supplies its own Command() — but Validate
// requires the *name* to exist in Providers.Providers.
var BuiltinProviderCommands = map[string]string{
	"claude": "claude --dangerously-skip-permissions",
	"agy":    "agy --dangerously-skip-permissions",
	"cursor": "cursor-agent --trust",
	"codex":  "codex --full-auto",
	"pi":     "pi",
}

// EnsureDefaultProviderDefined seeds Providers.Providers[default] when the
// fleet default names a built-in provider that is missing from prefs (e.g.
// an older prefs.json that only listed claude/agy). Returns false when the
// default is unknown and still missing — Validate should reject that.
func (c *Config) EnsureDefaultProviderDefined() bool {
	name := c.Providers.Default
	if name == "" || c.HasProviderDefined(name) {
		return true
	}
	cmd, ok := BuiltinProviderCommands[name]
	if !ok {
		return false
	}
	if c.Providers.Providers == nil {
		c.Providers.Providers = make(map[string]ProviderConfig)
	}
	c.Providers.Providers[name] = ProviderConfig{Command: cmd}
	return true
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
