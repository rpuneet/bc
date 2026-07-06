package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgyConfigAdapter implements ConfigAdapter for the Antigravity CLI. agy
// discovers customizations from a `.agents/` root at the project root:
// standalone AGENTS.md / GEMINI.md rule files, rules/*.md, skills/<name>/,
// plugins/<name>/, and mcp_config.json. This gives an agy agent the same
// depth of first-class config setup as the Claude provider.
type AgyConfigAdapter struct{}

// PromptFile is AGENTS.md — agy loads it (and GEMINI.md) as top-level rules.
func (a *AgyConfigAdapter) PromptFile() string { return "AGENTS.md" }

// ConfigDir is agy's customization root. Role rules/skills land under it as
// .agents/rules/*.md and .agents/skills/<name>/, which agy discovers.
func (a *AgyConfigAdapter) ConfigDir() string      { return ".agents" }
func (a *AgyConfigAdapter) SupportsRules() bool    { return true }
func (a *AgyConfigAdapter) SupportsCommands() bool { return false }
func (a *AgyConfigAdapter) SupportsSkills() bool   { return true }

// agyMCPServerEntry is one entry in agy's mcp_config.json. agy uses
// `serverUrl` for SSE/remote transport (not the claude-style url + type).
type agyMCPServerEntry struct {
	Env       map[string]string `json:"env,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	ServerURL string            `json:"serverUrl,omitempty"`
}

// SetupMCP writes agy's .agents/mcp_config.json for the given MCP servers.
// Stdio servers use command/args/env; SSE servers use serverUrl.
func (a *AgyConfigAdapter) SetupMCP(_ context.Context, targetDir, _ string, servers map[string]MCPEntry) error {
	if len(servers) == 0 {
		return nil
	}
	targetDir = filepath.Clean(targetDir)
	if strings.Contains(targetDir, "..") {
		return fmt.Errorf("unsafe target dir %q", targetDir)
	}
	agentsDir := filepath.Join(targetDir, ".agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return fmt.Errorf("create .agents dir: %w", err)
	}

	type mcpConfig struct {
		MCPServers map[string]agyMCPServerEntry `json:"mcpServers"`
	}
	cfg := mcpConfig{MCPServers: make(map[string]agyMCPServerEntry, len(servers))}
	for name, entry := range servers {
		e := agyMCPServerEntry{Env: entry.Env}
		if entry.Transport == "sse" || entry.URL != "" {
			e.ServerURL = entry.URL
		} else {
			e.Command = entry.Command
			e.Args = entry.Args
		}
		cfg.MCPServers[name] = e
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agy mcp config: %w", err)
	}
	return os.WriteFile(filepath.Join(agentsDir, "mcp_config.json"), append(data, '\n'), 0600)
}

// SetupPlugins writes agy plugin manifests under .agents/plugins/<name>/.
// agy discovers plugins by directory; a marker keeps the directory present.
func (a *AgyConfigAdapter) SetupPlugins(agentDir string, plugins []string) error {
	if len(plugins) == 0 {
		return nil
	}
	agentDir = filepath.Clean(agentDir)
	if strings.Contains(agentDir, "..") {
		return fmt.Errorf("unsafe agent dir %q", agentDir)
	}
	pluginsRoot := filepath.Join(agentDir, ".agents", "plugins")
	for _, name := range plugins {
		clean := filepath.Base(filepath.Clean(name))
		if clean == "." || clean == ".." || clean == "" {
			continue
		}
		dir := filepath.Join(pluginsRoot, clean)
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("create agy plugin dir: %w", err)
		}
	}
	return nil
}

// Verify AgyConfigAdapter implements ConfigAdapter at compile time.
var _ ConfigAdapter = (*AgyConfigAdapter)(nil)
