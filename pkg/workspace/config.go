// Package workspace provides workspace/project management.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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

// GatewaysConfig configures external messaging platform integrations.
type GatewaysConfig struct {
	Telegram *TelegramGatewayConfig `json:"telegram,omitempty"`
	Discord  *DiscordGatewayConfig  `json:"discord,omitempty"`
	Slack    *SlackGatewayConfig    `json:"slack,omitempty"`
}

// TelegramGatewayConfig configures the Telegram gateway adapter.
type TelegramGatewayConfig struct {
	BotToken string `json:"bot_token"`
	Mode     string `json:"mode"`
	Enabled  bool   `json:"enabled"`
}

// DiscordGatewayConfig configures the Discord gateway adapter.
type DiscordGatewayConfig struct {
	BotToken string `json:"bot_token"`
	Enabled  bool   `json:"enabled"`
}

// SlackGatewayConfig configures the Slack gateway adapter.
type SlackGatewayConfig struct {
	BotToken string `json:"bot_token"`
	AppToken string `json:"app_token"`
	Mode     string `json:"mode"`
	Enabled  bool   `json:"enabled"`
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

// Valid theme names.
var ValidThemes = []string{"dark", "light", "matrix", "synthwave", "high-contrast"}

// Valid theme modes.
var ValidModes = []string{"auto", "dark", "light"}

// User name limits.
const NameMaxLength = 30

// Validation errors.
var (
	ErrInvalidVersion          = errors.New("version must be 2")
	ErrMissingDefaultProvider  = errors.New("providers.default is required")
	ErrDefaultProviderNotFound = errors.New("providers.default references undefined provider")
	ErrInvalidTheme            = errors.New("ui.theme must be one of: dark, light, matrix, synthwave, high-contrast")
	ErrInvalidThemeMode        = errors.New("ui.mode must be one of: auto, dark, light")
	ErrNameTooLong             = errors.New("user.name is too long")
)

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
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path provided by caller
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
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

// FillDefaults fills zero-valued fields with defaults.
// Called after ParseConfig to handle configs from older versions.
func (c *Config) FillDefaults() {
	d := DefaultConfig()

	if c.Version == 0 {
		c.Version = d.Version
	}
	if c.Server.Host == "" {
		c.Server.Host = d.Server.Host
	}
	if c.Server.Port == 0 {
		c.Server.Port = d.Server.Port
	}
	if c.Server.CORSOrigin == "" {
		c.Server.CORSOrigin = d.Server.CORSOrigin
	}
	if c.Runtime.Default == "" {
		c.Runtime.Default = d.Runtime.Default
	}
	if c.Runtime.Tmux.SessionPrefix == "" {
		c.Runtime.Tmux.SessionPrefix = d.Runtime.Tmux.SessionPrefix
	}
	if c.Runtime.Tmux.HistoryLimit == 0 {
		c.Runtime.Tmux.HistoryLimit = d.Runtime.Tmux.HistoryLimit
	}
	if c.Runtime.Tmux.DefaultShell == "" {
		c.Runtime.Tmux.DefaultShell = d.Runtime.Tmux.DefaultShell
	}
	if c.Runtime.Docker.DockerSocketPath == "" {
		c.Runtime.Docker.DockerSocketPath = d.Runtime.Docker.DockerSocketPath
	}
	if c.Cron.PollIntervalSeconds == 0 {
		c.Cron.PollIntervalSeconds = d.Cron.PollIntervalSeconds
	}
	if c.Cron.JobTimeoutSeconds == 0 {
		c.Cron.JobTimeoutSeconds = d.Cron.JobTimeoutSeconds
	}
	if c.Storage.Default == "" {
		c.Storage.Default = d.Storage.Default
	}
	if c.Storage.SQLite.Path == "" {
		c.Storage.SQLite.Path = d.Storage.SQLite.Path
	}
	if c.Logs.Path == "" {
		c.Logs.Path = d.Logs.Path
	}
	if c.Logs.MaxBytes == 0 {
		c.Logs.MaxBytes = d.Logs.MaxBytes
	}
	if c.UI.Theme == "" {
		c.UI.Theme = d.UI.Theme
	}
	if c.UI.Mode == "" {
		c.UI.Mode = d.UI.Mode
	}
	if c.UI.DefaultView == "" {
		c.UI.DefaultView = d.UI.DefaultView
	}
	if c.Providers.Default == "" {
		c.Providers.Default = d.Providers.Default
	}
}

// Validate checks the config for required fields and consistency.
func (c *Config) Validate() error {
	if c.Version != ConfigVersion {
		return ErrInvalidVersion
	}
	if c.Providers.Default == "" {
		return ErrMissingDefaultProvider
	}
	if !c.HasProviderDefined(c.Providers.Default) {
		return ErrDefaultProviderNotFound
	}
	if err := c.validateUI(); err != nil {
		return err
	}
	if err := c.validateUser(); err != nil {
		return err
	}
	if err := c.validateServer(); err != nil {
		return err
	}
	if err := c.validateStorage(); err != nil {
		return err
	}
	return nil
}

// validateServer validates server configuration.
func (c *Config) validateServer() error {
	if c.Server.Port != 0 && (c.Server.Port < 1 || c.Server.Port > 65535) {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	return nil
}

// validateStorage validates storage configuration.
func (c *Config) validateStorage() error {
	// Accept "timescale" and legacy "sql" for backward compatibility
	if c.Storage.Default != "" && c.Storage.Default != "sqlite" && c.Storage.Default != "timescale" && c.Storage.Default != "sql" {
		return fmt.Errorf("storage.default must be 'sqlite' or 'timescale', got %q", c.Storage.Default)
	}
	if c.Storage.Timescale.Port != 0 && (c.Storage.Timescale.Port < 1 || c.Storage.Timescale.Port > 65535) {
		return fmt.Errorf("storage.timescale.port must be between 1 and 65535, got %d", c.Storage.Timescale.Port)
	}
	return nil
}

// validateUI validates UI config values.
func (c *Config) validateUI() error {
	if c.UI.Theme != "" && !isValidTheme(c.UI.Theme) {
		return ErrInvalidTheme
	}
	if c.UI.Mode != "" && !isValidMode(c.UI.Mode) {
		return ErrInvalidThemeMode
	}
	return nil
}

func isValidTheme(theme string) bool {
	for _, valid := range ValidThemes {
		if theme == valid {
			return true
		}
	}
	return false
}

func isValidMode(mode string) bool {
	for _, valid := range ValidModes {
		if mode == valid {
			return true
		}
	}
	return false
}

// validateUser validates user config values.
func (c *Config) validateUser() error {
	if len(c.User.Name) > NameMaxLength {
		return ErrNameTooLong
	}
	return nil
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
// Checks global state dir first, falls back to legacy .bc/.
func ConfigPath(rootDir string) string {
	stateDir, err := GlobalStateDir(rootDir)
	if err == nil {
		p := filepath.Join(stateDir, "settings.json")
		if _, statErr := os.Stat(p); statErr == nil {
			return p
		}
	}
	return filepath.Join(rootDir, ".bc", "settings.json")
}

// --- Nickname compatibility (used by channel system) ---

var nicknameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ValidateNickname validates a nickname and returns an error if invalid.
func ValidateNickname(nickname string) error {
	if !strings.HasPrefix(nickname, "@") {
		return fmt.Errorf("nickname must start with @")
	}
	if len(nickname) > 15 {
		return fmt.Errorf("nickname must be 15 characters or less")
	}
	body := nickname[1:]
	if body == "" || !nicknameRegex.MatchString(body) {
		return fmt.Errorf("nickname must contain only letters, numbers, and underscores")
	}
	return nil
}

// NormalizeNickname ensures a nickname has the @ prefix and is valid.
func NormalizeNickname(nickname string) (string, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return "@bc", nil
	}
	if !strings.HasPrefix(nickname, "@") {
		nickname = "@" + nickname
	}
	if err := ValidateNickname(nickname); err != nil {
		return "", err
	}
	return nickname, nil
}
