package home

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// UserRCConfigPath returns the path to the user's .bcrc file.
// Default: ~/.bcrc
func UserRCConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".bcrc")
}

// UserRCConfig represents user-level defaults stored in ~/.bcrc.
// These values are merged with the global config when loading.
type UserRCConfig struct {
	User     UserRCUserConfig     `json:"user"`
	Defaults UserRCDefaultsConfig `json:"defaults"`
	Tools    UserRCToolsConfig    `json:"tools"`
}

// UserRCUserConfig holds user identity settings.
type UserRCUserConfig struct {
	Nickname string `json:"nickname,omitempty"`
}

// UserRCDefaultsConfig holds default behavior settings.
type UserRCDefaultsConfig struct {
	DefaultRole   string `json:"default_role,omitempty"`
	AutoStartRoot bool   `json:"auto_start_root,omitempty"`
}

// UserRCToolsConfig holds tool preferences.
type UserRCToolsConfig struct {
	Preferred []string `json:"preferred,omitempty"`
}

// DefaultUserRCConfig returns sensible defaults for a new .bcrc file.
func DefaultUserRCConfig() UserRCConfig {
	return UserRCConfig{
		User: UserRCUserConfig{
			Nickname: "@bc",
		},
		Defaults: UserRCDefaultsConfig{
			DefaultRole:   "engineer",
			AutoStartRoot: true,
		},
		Tools: UserRCToolsConfig{
			Preferred: []string{"claude-code", "agy"},
		},
	}
}

// LoadUserRCConfig loads the user's .bcrc file.
func LoadUserRCConfig() (*UserRCConfig, error) {
	path := UserRCConfigPath()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path from UserHomeDir
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read .bcrc: %w", err)
	}
	return ParseUserRCConfig(data)
}

// ParseUserRCConfig parses JSON data into a UserRCConfig.
func ParseUserRCConfig(data []byte) (*UserRCConfig, error) {
	var cfg UserRCConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse .bcrc: %w", err)
	}
	return &cfg, nil
}

// Save writes the user config to the .bcrc file.
func (c *UserRCConfig) Save() error {
	path := UserRCConfigPath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal .bcrc: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil { //nolint:gosec
		return fmt.Errorf("failed to write .bcrc: %w", err)
	}
	return nil
}

// UserRCExists returns true if ~/.bcrc exists.
func UserRCExists() bool {
	path := UserRCConfigPath()
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// MergeWithUserRC merges user-level defaults from .bcrc into the global config.
func (c *Config) MergeWithUserRC(rc *UserRCConfig) {
	if rc == nil {
		return
	}
	if c.User.Name == "" {
		if rc.User.Nickname != "" {
			c.User.Name = rc.User.Nickname
		}
	}
}

// GetPreferredTool returns the first available preferred tool from .bcrc.
func (c *Config) GetPreferredTool(rc *UserRCConfig) string {
	if rc == nil || len(rc.Tools.Preferred) == 0 {
		return c.Providers.Default
	}
	for _, tool := range rc.Tools.Preferred {
		if c.HasProviderDefined(tool) {
			return tool
		}
	}
	return c.Providers.Default
}
