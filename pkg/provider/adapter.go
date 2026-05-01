// Package provider — ConfigAdapter extends Provider with config file setup.
package provider

import "context"

// MCPEntry represents an MCP server configuration for adapter setup.
type MCPEntry struct { //nolint:govet // field order matches API contract
	Env       map[string]string
	Args      []string
	Name      string
	Transport string // "sse" or "stdio"
	Command   string
	URL       string
}

// ConfigAdapter handles provider-specific configuration file setup.
// Providers that implement this interface get custom file layouts during
// agent role setup. Providers without it get a generic fallback.
type ConfigAdapter interface {
	// PromptFile returns the filename for the role prompt (e.g., "CLAUDE.md", ".cursorrules").
	PromptFile() string

	// ConfigDir returns the provider-specific config directory name (e.g., ".claude", ".cursor").
	// Empty string means no config directory.
	ConfigDir() string

	// SetupMCP configures MCP servers for this provider in the target directory.
	SetupMCP(ctx context.Context, targetDir, agentName string, servers map[string]MCPEntry) error

	// SetupPlugins writes plugin configuration for this provider.
	SetupPlugins(agentDir string, plugins []string) error

	// SupportsRules returns true if the provider supports rule files in ConfigDir/rules/.
	SupportsRules() bool

	// SupportsCommands returns true if the provider supports command files in ConfigDir/commands/.
	SupportsCommands() bool

	// SupportsSkills returns true if the provider supports skill files.
	SupportsSkills() bool
}

// GetConfigAdapter returns the ConfigAdapter for a provider, or nil if it
// doesn't implement one. Use this for type-safe adapter access.
func GetConfigAdapter(p Provider) ConfigAdapter {
	if adapter, ok := p.(ConfigAdapter); ok {
		return adapter
	}
	return nil
}

// GenericAdapter is a fallback for providers that don't implement ConfigAdapter.
// It writes a prompt to {PROVIDER}.md and skips MCP/rules/commands.
type GenericAdapter struct {
	providerName string
}

// NewGenericAdapter creates a fallback adapter for any provider.
func NewGenericAdapter(name string) *GenericAdapter {
	return &GenericAdapter{providerName: name}
}

func (a *GenericAdapter) PromptFile() string     { return toUpperFirst(a.providerName) + ".md" }
func (a *GenericAdapter) ConfigDir() string      { return "" }
func (a *GenericAdapter) SupportsRules() bool    { return false }
func (a *GenericAdapter) SupportsCommands() bool { return false }
func (a *GenericAdapter) SupportsSkills() bool   { return false }

func (a *GenericAdapter) SetupMCP(_ context.Context, _, _ string, _ map[string]MCPEntry) error {
	return nil
}
func (a *GenericAdapter) SetupPlugins(_ string, _ []string) error { return nil }

func toUpperFirst(s string) string {
	if s == "" {
		return ""
	}
	return string(s[0]-32) + s[1:] // ASCII uppercase first char
}
