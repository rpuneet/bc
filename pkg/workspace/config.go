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

	"github.com/rpuneet/bc/pkg/log"
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

// GatewaysConfig configures external messaging platform integrations.
//
// JSON keys follow a "platform" or "platform:label" convention. Plain
// "telegram" is a single Telegram bot (backward compat). Keys like
// "telegram:trade_research" register additional bots — parsed into the
// Telegrams map keyed by label.
type GatewaysConfig struct {
	// Telegram is the single default Telegram bot (key "telegram").
	// Deprecated: prefer Telegrams map for multi-bot setups.
	Telegram *TelegramGatewayConfig `json:"-"`
	Discord  *DiscordGatewayConfig  `json:"discord,omitempty"`
	Slack    *SlackGatewayConfig    `json:"slack,omitempty"`
	// Telegrams holds zero or more Telegram bots keyed by label.
	// A plain "telegram" key is stored under label "".
	Telegrams map[string]*TelegramGatewayConfig `json:"-"`
	// GitHubs holds zero or more GitHub webhook configs keyed by label.
	// A plain "github" key is stored under label "".
	GitHubs map[string]*GitHubGatewayConfig `json:"-"`
	// Webhooks holds zero or more generic webhook configs keyed by label.
	// A plain "webhook" key is stored under label "".
	Webhooks map[string]*WebhookGatewayConfig `json:"-"`
	// RSSFeeds holds zero or more RSS/Atom feed configs keyed by label.
	// A plain "rss" key is stored under label "".
	RSSFeeds map[string]*RSSGatewayConfig `json:"-"`
	// GitLabs holds zero or more GitLab webhook configs keyed by label.
	GitLabs map[string]*GitLabGatewayConfig `json:"-"`
	// Jiras holds zero or more Jira webhook configs keyed by label.
	Jiras map[string]*JiraGatewayConfig `json:"-"`
	// Linears holds zero or more Linear webhook configs keyed by label.
	Linears map[string]*LinearGatewayConfig `json:"-"`
	// Sentries holds zero or more Sentry webhook configs keyed by label.
	Sentries map[string]*SentryGatewayConfig `json:"-"`
	// Stripes holds zero or more Stripe webhook configs keyed by label.
	Stripes map[string]*StripeGatewayConfig `json:"-"`
}

// UnmarshalJSON parses gateway config, routing "telegram:*" keys into the
// Telegrams map and keeping Discord/Slack on their typed fields.
func (g *GatewaysConfig) UnmarshalJSON(data []byte) error {
	// Decode known typed fields via an alias to avoid recursion.
	type Alias struct {
		Discord *DiscordGatewayConfig `json:"discord,omitempty"`
		Slack   *SlackGatewayConfig   `json:"slack,omitempty"`
	}
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	g.Discord = alias.Discord
	g.Slack = alias.Slack

	// Decode the full map to pick up telegram keys.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.Telegrams = make(map[string]*TelegramGatewayConfig)
	g.GitHubs = make(map[string]*GitHubGatewayConfig)
	g.Webhooks = make(map[string]*WebhookGatewayConfig)
	g.RSSFeeds = make(map[string]*RSSGatewayConfig)
	g.GitLabs = make(map[string]*GitLabGatewayConfig)
	g.Jiras = make(map[string]*JiraGatewayConfig)
	g.Linears = make(map[string]*LinearGatewayConfig)
	g.Sentries = make(map[string]*SentryGatewayConfig)
	g.Stripes = make(map[string]*StripeGatewayConfig)
	for key, val := range raw {
		switch {
		case key == "telegram":
			var tc TelegramGatewayConfig
			if err := json.Unmarshal(val, &tc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Telegram = &tc
			g.Telegrams[""] = &tc
		case strings.HasPrefix(key, "telegram:"):
			label := strings.TrimPrefix(key, "telegram:")
			var tc TelegramGatewayConfig
			if err := json.Unmarshal(val, &tc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Telegrams[label] = &tc
		case key == "github":
			var gc GitHubGatewayConfig
			if err := json.Unmarshal(val, &gc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GitHubs[""] = &gc
		case strings.HasPrefix(key, "github:"):
			label := strings.TrimPrefix(key, "github:")
			var gc GitHubGatewayConfig
			if err := json.Unmarshal(val, &gc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GitHubs[label] = &gc
		case key == "webhook":
			var wc WebhookGatewayConfig
			if err := json.Unmarshal(val, &wc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Webhooks[""] = &wc
		case strings.HasPrefix(key, "webhook:"):
			label := strings.TrimPrefix(key, "webhook:")
			var wc WebhookGatewayConfig
			if err := json.Unmarshal(val, &wc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Webhooks[label] = &wc
		case key == "rss":
			var rc RSSGatewayConfig
			if err := json.Unmarshal(val, &rc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.RSSFeeds[""] = &rc
		case strings.HasPrefix(key, "rss:"):
			label := strings.TrimPrefix(key, "rss:")
			var rc RSSGatewayConfig
			if err := json.Unmarshal(val, &rc); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.RSSFeeds[label] = &rc
		case key == "gitlab" || strings.HasPrefix(key, "gitlab:"):
			label := strings.TrimPrefix(key, "gitlab:")
			if key == "gitlab" {
				label = ""
			}
			var c GitLabGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.GitLabs[label] = &c
		case key == "jira" || strings.HasPrefix(key, "jira:"):
			label := strings.TrimPrefix(key, "jira:")
			if key == "jira" {
				label = ""
			}
			var c JiraGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Jiras[label] = &c
		case key == "linear" || strings.HasPrefix(key, "linear:"):
			label := strings.TrimPrefix(key, "linear:")
			if key == "linear" {
				label = ""
			}
			var c LinearGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Linears[label] = &c
		case key == "sentry" || strings.HasPrefix(key, "sentry:"):
			label := strings.TrimPrefix(key, "sentry:")
			if key == "sentry" {
				label = ""
			}
			var c SentryGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Sentries[label] = &c
		case key == "stripe" || strings.HasPrefix(key, "stripe:"):
			label := strings.TrimPrefix(key, "stripe:")
			if key == "stripe" {
				label = ""
			}
			var c StripeGatewayConfig
			if err := json.Unmarshal(val, &c); err != nil {
				return fmt.Errorf("parse gateway %q: %w", key, err)
			}
			g.Stripes[label] = &c
		}
	}
	return nil
}

// MarshalJSON serializes the gateway config, emitting "telegram:label"
// keys for each entry in Telegrams.
func (g GatewaysConfig) MarshalJSON() ([]byte, error) {
	m := make(map[string]any)
	for label, tc := range g.Telegrams {
		if label == "" {
			m["telegram"] = tc
		} else {
			m["telegram:"+label] = tc
		}
	}
	// Backward compat: if Telegram is set but not in Telegrams, emit it.
	if g.Telegram != nil {
		if _, ok := g.Telegrams[""]; !ok {
			m["telegram"] = g.Telegram
		}
	}
	for label, gc := range g.GitHubs {
		if label == "" {
			m["github"] = gc
		} else {
			m["github:"+label] = gc
		}
	}
	for label, wc := range g.Webhooks {
		if label == "" {
			m["webhook"] = wc
		} else {
			m["webhook:"+label] = wc
		}
	}
	for label, rc := range g.RSSFeeds {
		if label == "" {
			m["rss"] = rc
		} else {
			m["rss:"+label] = rc
		}
	}
	for label, c := range g.GitLabs {
		if label == "" {
			m["gitlab"] = c
		} else {
			m["gitlab:"+label] = c
		}
	}
	for label, c := range g.Jiras {
		if label == "" {
			m["jira"] = c
		} else {
			m["jira:"+label] = c
		}
	}
	for label, c := range g.Linears {
		if label == "" {
			m["linear"] = c
		} else {
			m["linear:"+label] = c
		}
	}
	for label, c := range g.Sentries {
		if label == "" {
			m["sentry"] = c
		} else {
			m["sentry:"+label] = c
		}
	}
	for label, c := range g.Stripes {
		if label == "" {
			m["stripe"] = c
		} else {
			m["stripe:"+label] = c
		}
	}
	if g.Discord != nil {
		m["discord"] = g.Discord
	}
	if g.Slack != nil {
		m["slack"] = g.Slack
	}
	return json.Marshal(m)
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

// GitHubGatewayConfig configures the GitHub webhook gateway adapter.
type GitHubGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// WebhookGatewayConfig configures a generic webhook gateway adapter.
type WebhookGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// RSSGatewayConfig configures an RSS/Atom feed poll adapter.
type RSSGatewayConfig struct {
	URL      string `json:"url"`
	Interval int    `json:"interval"` // seconds, default 300
	Enabled  bool   `json:"enabled"`
}

// GitLabGatewayConfig configures the GitLab webhook gateway adapter.
type GitLabGatewayConfig struct {
	Token   string `json:"token"`
	Enabled bool   `json:"enabled"`
}

// JiraGatewayConfig configures the Jira webhook gateway adapter.
type JiraGatewayConfig struct {
	Secret  string `json:"secret,omitempty"`
	Enabled bool   `json:"enabled"`
}

// LinearGatewayConfig configures the Linear webhook gateway adapter.
type LinearGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// SentryGatewayConfig configures the Sentry webhook gateway adapter.
type SentryGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
}

// StripeGatewayConfig configures the Stripe webhook gateway adapter.
type StripeGatewayConfig struct {
	Secret  string `json:"secret"`
	Enabled bool   `json:"enabled"`
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
