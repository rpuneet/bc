package client

import "context"

// ToolsClient provides tool management operations via the daemon.
type ToolsClient struct {
	client *Client
}

// ToolInfo represents a tool configuration returned by the daemon.
type ToolInfo struct {
	Name       string   `json:"name"`
	Command    string   `json:"command,omitempty"`
	InstallCmd string   `json:"install_cmd,omitempty"`
	UpgradeCmd string   `json:"upgrade_cmd,omitempty"`
	MCPServers []string `json:"mcp_servers,omitempty"`
	SlashCmds  []string `json:"slash_cmds,omitempty"`
	Enabled    bool     `json:"enabled"`
	Builtin    bool     `json:"builtin,omitempty"`
}

// List returns all tools.
func (t *ToolsClient) List(ctx context.Context) ([]*ToolInfo, error) {
	var tools []*ToolInfo
	if err := t.client.get(ctx, "/api/tools", &tools); err != nil {
		return nil, err
	}
	return tools, nil
}

// Get returns a specific tool by name.
func (t *ToolsClient) Get(ctx context.Context, name string) (*ToolInfo, error) {
	var tool ToolInfo
	if err := t.client.get(ctx, "/api/tools/"+name, &tool); err != nil {
		return nil, err
	}
	return &tool, nil
}

// Create adds a new tool configuration.
func (t *ToolsClient) Create(ctx context.Context, tool *ToolInfo) (*ToolInfo, error) {
	var created ToolInfo
	if err := t.client.post(ctx, "/api/tools", tool, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// Update updates an existing tool's configuration.
func (t *ToolsClient) Update(ctx context.Context, tool *ToolInfo) (*ToolInfo, error) {
	var updated ToolInfo
	if err := t.client.put(ctx, "/api/tools/"+tool.Name, tool, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// Delete removes a tool by name.
func (t *ToolsClient) Delete(ctx context.Context, name string) error {
	return t.client.delete(ctx, "/api/tools/"+name)
}

// Enable enables a tool by name.
func (t *ToolsClient) Enable(ctx context.Context, name string) error {
	return t.client.post(ctx, "/api/tools/"+name+"/enable", nil, nil)
}

// Disable disables a tool by name.
func (t *ToolsClient) Disable(ctx context.Context, name string) error {
	return t.client.post(ctx, "/api/tools/"+name+"/disable", nil, nil)
}
