package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/home"
)

const (
	// OfficialClaudePluginSource is what ClaudeConfigAdapter.SetupPlugins
	// writes into installed_plugins.json for blueprint Plugins entries.
	OfficialClaudePluginSource = "claude-plugins-official"

	pluginsFileName = "plugins.json"
)

// InstalledPlugin is one skill/plugin the daemon recorded locally so
// blueprints can reference it without dispatching prose to an agent (#3016).
type InstalledPlugin struct { //nolint:govet // JSON field order preferred over alignment
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	URL         string    `json:"url,omitempty"`
	Description string    `json:"description,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
}

type pluginsFile struct {
	Plugins []InstalledPlugin `json:"plugins"`
}

// GlobalPluginsPath returns ~/.mycel/plugins.json.
func GlobalPluginsPath() (string, error) {
	root, err := home.MycelHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, pluginsFileName), nil
}

// ListInstalledPlugins returns plugins recorded in the global store.
func ListInstalledPlugins() ([]InstalledPlugin, error) {
	path, err := GlobalPluginsPath()
	if err != nil {
		return nil, err
	}
	return readPluginsFile(path)
}

// InstallClaudePlugin records an official Claude marketplace skill locally.
// Idempotent: reinstalling the same name updates metadata and returns nil.
func InstallClaudePlugin(name, url, description string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") ||
		strings.Contains(name, string(filepath.Separator)) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	path, err := GlobalPluginsPath()
	if err != nil {
		return err
	}
	list, err := readPluginsFile(path)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	found := false
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			list[i].Name = name
			list[i].Source = OfficialClaudePluginSource
			list[i].URL = url
			list[i].Description = description
			list[i].InstalledAt = now
			found = true
			break
		}
	}
	if !found {
		list = append(list, InstalledPlugin{
			Name:        name,
			Source:      OfficialClaudePluginSource,
			URL:         url,
			Description: description,
			InstalledAt: now,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return writePluginsFile(path, list)
}

func readPluginsFile(path string) ([]InstalledPlugin, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path from MycelHome
	if errorsIsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f pluginsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return f.Plugins, nil
}

func writePluginsFile(path string, list []InstalledPlugin) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(pluginsFile{Plugins: list}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
