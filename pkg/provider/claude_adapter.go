package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ClaudeConfigAdapter implements ConfigAdapter for Claude Code.
// It writes CLAUDE.md, .mcp.json (or uses `claude mcp add`),
// .claude/rules/, .claude/commands/, and plugin configs.
type ClaudeConfigAdapter struct{}

func (a *ClaudeConfigAdapter) PromptFile() string     { return "CLAUDE.md" }
func (a *ClaudeConfigAdapter) ConfigDir() string      { return ".claude" }
func (a *ClaudeConfigAdapter) SupportsRules() bool    { return true }
func (a *ClaudeConfigAdapter) SupportsCommands() bool { return true }
func (a *ClaudeConfigAdapter) SupportsSkills() bool   { return true }

// SetupMCP configures MCP servers for Claude Code.
// Prefers `claude mcp add` CLI; falls back to .mcp.json file write.
func (a *ClaudeConfigAdapter) SetupMCP(ctx context.Context, targetDir, agentName string, servers map[string]MCPEntry) error {
	if len(servers) == 0 {
		return nil
	}

	// Try claude CLI first
	if a.setupMCPViaCLI(ctx, targetDir, servers) {
		return nil
	}

	// Fallback: write .mcp.json
	return a.writeMCPJSON(targetDir, servers)
}

// SetupPlugins writes Claude Code plugin configuration.
func (a *ClaudeConfigAdapter) SetupPlugins(agentDir string, plugins []string) error {
	if len(plugins) == 0 {
		return nil
	}

	// agentDir derives from validated agent names; reject traversal
	// segments as defense in depth.
	agentDir = filepath.Clean(agentDir)
	if strings.Contains(agentDir, "..") {
		return fmt.Errorf("unsafe agent dir %q", agentDir)
	}

	claudeDir := filepath.Join(agentDir, "claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		return fmt.Errorf("create claude dir: %w", err)
	}

	type pluginEntry struct {
		Name    string `json:"name"`
		Source  string `json:"source"`
		Enabled bool   `json:"enabled"`
	}
	type manifest struct {
		Plugins map[string]pluginEntry `json:"plugins"`
	}

	m := manifest{Plugins: make(map[string]pluginEntry, len(plugins))}
	for _, name := range plugins {
		m.Plugins[name] = pluginEntry{Name: name, Source: "claude-plugins-official", Enabled: true}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugins: %w", err)
	}
	return os.WriteFile(filepath.Join(claudeDir, "installed_plugins.json"), data, 0600)
}

// setupMCPViaCLI uses `claude mcp add` commands.
func (a *ClaudeConfigAdapter) setupMCPViaCLI(ctx context.Context, targetDir string, servers map[string]MCPEntry) bool {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return false
	}

	for name, entry := range servers {
		// Remove existing to avoid duplicates
		rmCmd := exec.CommandContext(ctx, claudePath, "mcp", "remove", name, "--scope", "project") //nolint:gosec
		rmCmd.Dir = targetDir
		_ = rmCmd.Run() //nolint:errcheck

		args := []string{"mcp", "add", "--scope", "project"}
		if entry.Transport == "sse" || entry.URL != "" {
			args = append(args, "--transport", "sse")
			for k, v := range entry.Env {
				args = append(args, "-e", k+"="+v)
			}
			args = append(args, name, entry.URL)
		} else if entry.Command != "" {
			for k, v := range entry.Env {
				args = append(args, "-e", k+"="+v)
			}
			args = append(args, name, "--")
			args = append(args, strings.Fields(entry.Command)...)
			args = append(args, entry.Args...)
		} else {
			continue
		}

		cmd := exec.CommandContext(ctx, claudePath, args...) //nolint:gosec
		cmd.Dir = targetDir
		_ = cmd.Run() //nolint:errcheck
	}
	return true
}

// writeMCPJSON writes a .mcp.json file (fallback when claude CLI unavailable).
func (a *ClaudeConfigAdapter) writeMCPJSON(targetDir string, servers map[string]MCPEntry) error {
	type mcpServerEntry struct {
		Env     map[string]string `json:"env,omitempty"`
		Command string            `json:"command,omitempty"`
		URL     string            `json:"url,omitempty"`
		Type    string            `json:"type,omitempty"`
		Args    []string          `json:"args,omitempty"`
	}
	type mcpConfig struct {
		MCPServers map[string]mcpServerEntry `json:"mcpServers"`
	}

	cfg := mcpConfig{MCPServers: make(map[string]mcpServerEntry, len(servers))}
	for name, entry := range servers {
		e := mcpServerEntry{
			Command: entry.Command,
			URL:     entry.URL,
			Args:    entry.Args,
			Env:     entry.Env,
		}
		if entry.Transport == "sse" {
			e.Type = "sse"
		}
		cfg.MCPServers[name] = e
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}
	return os.WriteFile(filepath.Join(targetDir, ".mcp.json"), append(data, '\n'), 0600)
}

// Verify ClaudeProvider implements ConfigAdapter at compile time.
var _ ConfigAdapter = (*ClaudeConfigAdapter)(nil)
