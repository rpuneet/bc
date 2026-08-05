package client

import (
	"context"
	"fmt"
	"net/url"
)

// TemplatesClient manages agent templates via the daemon.
type TemplatesClient struct {
	client *Client
}

// TemplateInfo is a template as returned by /api/templates.
type TemplateInfo struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Label            string   `json:"label,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Scope            string   `json:"scope,omitempty"`
	MCPs             []string `json:"mcps,omitempty"`
	Secrets          []string `json:"secrets,omitempty"`
	Plugins          []string `json:"plugins,omitempty"`
	Composes         []string `json:"composes,omitempty"`
	SystemPrompt     string   `json:"system_prompt,omitempty"`
	SystemPromptFile string   `json:"system_prompt_file,omitempty"`
	MaxCostUSD       float64  `json:"max_cost_usd,omitempty"`
	StuckTimeoutMin  int      `json:"stuck_timeout_min,omitempty"`
}

// templateWriteBody is the create/update payload. SystemPrompt is a pointer
// so PUT can clear the prompt with an explicit empty string.
type templateWriteBody struct {
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	Label           string   `json:"label,omitempty"`
	Provider        string   `json:"provider,omitempty"`
	MCPs            []string `json:"mcps,omitempty"`
	Secrets         []string `json:"secrets,omitempty"`
	Plugins         []string `json:"plugins,omitempty"`
	Composes        []string `json:"composes,omitempty"`
	SystemPrompt    *string  `json:"system_prompt,omitempty"`
	MaxCostUSD      float64  `json:"max_cost_usd,omitempty"`
	StuckTimeoutMin int      `json:"stuck_timeout_min,omitempty"`
}

// List returns all templates.
func (t *TemplatesClient) List(ctx context.Context) ([]TemplateInfo, error) {
	var out []TemplateInfo
	if err := t.client.get(ctx, "/api/templates", &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []TemplateInfo{}
	}
	return out, nil
}

// Get returns one template by name (includes system_prompt).
func (t *TemplatesClient) Get(ctx context.Context, name string) (*TemplateInfo, error) {
	var out TemplateInfo
	if err := t.client.get(ctx, "/api/templates/"+url.PathEscape(name), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create registers a new template.
func (t *TemplatesClient) Create(ctx context.Context, tmpl TemplateInfo) (*TemplateInfo, error) {
	body := templateWriteBody{
		Name:            tmpl.Name,
		Description:     tmpl.Description,
		Label:           tmpl.Label,
		Provider:        tmpl.Provider,
		MCPs:            tmpl.MCPs,
		Secrets:         tmpl.Secrets,
		Plugins:         tmpl.Plugins,
		Composes:        tmpl.Composes,
		MaxCostUSD:      tmpl.MaxCostUSD,
		StuckTimeoutMin: tmpl.StuckTimeoutMin,
	}
	if tmpl.SystemPrompt != "" {
		p := tmpl.SystemPrompt
		body.SystemPrompt = &p
	}
	var out TemplateInfo
	if err := t.client.post(ctx, "/api/templates", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update replaces an existing template.
func (t *TemplatesClient) Update(ctx context.Context, name string, tmpl TemplateInfo) (*TemplateInfo, error) {
	prompt := tmpl.SystemPrompt
	body := templateWriteBody{
		Name:            tmpl.Name,
		Description:     tmpl.Description,
		Label:           tmpl.Label,
		Provider:        tmpl.Provider,
		MCPs:            tmpl.MCPs,
		Secrets:         tmpl.Secrets,
		Plugins:         tmpl.Plugins,
		Composes:        tmpl.Composes,
		SystemPrompt:    &prompt,
		MaxCostUSD:      tmpl.MaxCostUSD,
		StuckTimeoutMin: tmpl.StuckTimeoutMin,
	}
	if body.Name == "" {
		body.Name = name
	}
	var out TemplateInfo
	if err := t.client.put(ctx, "/api/templates/"+url.PathEscape(name), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a template by name.
func (t *TemplatesClient) Delete(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("template name required")
	}
	return t.client.delete(ctx, "/api/templates/"+url.PathEscape(name))
}
