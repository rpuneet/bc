package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CursorConfigAdapter implements ConfigAdapter for Cursor.
// Cursor uses .cursorrules for prompts and .cursor/mcp.json for MCP config.
type CursorConfigAdapter struct{}

func (a *CursorConfigAdapter) PromptFile() string     { return ".cursorrules" }
func (a *CursorConfigAdapter) ConfigDir() string      { return ".cursor" }
func (a *CursorConfigAdapter) SupportsRules() bool    { return true }
func (a *CursorConfigAdapter) SupportsCommands() bool { return false }
func (a *CursorConfigAdapter) SupportsSkills() bool   { return false }

// SetupMCP writes .cursor/mcp.json for Cursor's MCP support.
func (a *CursorConfigAdapter) SetupMCP(_ context.Context, targetDir, _ string, servers map[string]MCPEntry) error {
	if len(servers) == 0 {
		return nil
	}

	type cursorMCPServer struct { //nolint:govet // field order matches JSON/API contract
		Env     map[string]string `json:"env,omitempty"`
		Args    []string          `json:"args,omitempty"`
		Command string            `json:"command,omitempty"`
		URL     string            `json:"url,omitempty"`
	}
	type cursorMCPConfig struct {
		MCPServers map[string]cursorMCPServer `json:"mcpServers"`
	}

	cfg := cursorMCPConfig{MCPServers: make(map[string]cursorMCPServer, len(servers))}
	for name, entry := range servers {
		cfg.MCPServers[name] = cursorMCPServer{
			Command: entry.Command,
			URL:     entry.URL,
			Args:    entry.Args,
			Env:     entry.Env,
		}
	}

	cursorDir := filepath.Join(targetDir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0750); err != nil {
		return fmt.Errorf("create .cursor dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cursor mcp config: %w", err)
	}
	return os.WriteFile(filepath.Join(cursorDir, "mcp.json"), append(data, '\n'), 0600)
}

// SetupPlugins is a no-op for Cursor (no plugin system).
func (a *CursorConfigAdapter) SetupPlugins(_ string, _ []string) error { return nil }

var _ ConfigAdapter = (*CursorConfigAdapter)(nil)
