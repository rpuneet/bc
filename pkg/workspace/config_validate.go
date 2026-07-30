package workspace

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

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
	if c.Storage.Default != "" && c.Storage.Default != "sqlite" && c.Storage.Default != "timescale" {
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
	// Use rune count, not byte length, so multi-byte Unicode names are
	// not unfairly truncated against NameMaxLength.
	if utf8.RuneCountInString(c.User.Name) > NameMaxLength {
		return ErrNameTooLong
	}
	return nil
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
