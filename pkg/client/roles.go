package client

import (
	"context"
	"fmt"
)

// RolesClient provides role management operations.
type RolesClient struct {
	client *Client
}

// RoleInfo represents a resolved role from the API.
type RoleInfo struct {
	Commands     map[string]string `json:"Commands"`
	Skills       map[string]string `json:"Skills"`
	Agents       map[string]string `json:"Agents"`
	Rules        map[string]string `json:"Rules"`
	Settings     map[string]any    `json:"Settings"`
	Name         string            `json:"Name"`
	Prompt       string            `json:"Prompt"`
	PromptCreate string            `json:"PromptCreate"`
	PromptStart  string            `json:"PromptStart"`
	PromptStop   string            `json:"PromptStop"`
	PromptDelete string            `json:"PromptDelete"`
	Review       string            `json:"Review"`
	MCPServers   []string          `json:"MCPServers"`
	Secrets      []string          `json:"Secrets"`
	Plugins      []string          `json:"Plugins"`
}

// List returns all resolved roles.
func (r *RolesClient) List(ctx context.Context) (map[string]*RoleInfo, error) {
	var roles map[string]*RoleInfo
	if err := r.client.get(ctx, "/api/roles", &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// Get returns a single resolved role by name.
func (r *RolesClient) Get(ctx context.Context, name string) (*RoleInfo, error) {
	roles, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	role, ok := roles[name]
	if !ok {
		return nil, fmt.Errorf("role %q not found", name)
	}
	return role, nil
}
