// Package template provides agent template definitions and file-based storage.
// Templates replace Roles as the agent creation primitive.
package template

// Template defines an agent template — a reusable configuration for spawning agents.
type Template struct {
	ToolPolicies     *ToolPolicies `json:"tool_policies,omitempty"`
	Name             string        `json:"name"`
	Description      string        `json:"description,omitempty"`
	SystemPromptFile string        `json:"system_prompt_file,omitempty"`
	MCPs             []string      `json:"mcps,omitempty"`
	Secrets          []string      `json:"secrets,omitempty"`
	Plugins          []string      `json:"plugins,omitempty"`
	ContextFiles     []string      `json:"context_files,omitempty"`
	MaxCostUSD       float64       `json:"max_cost_usd,omitempty"`
	StuckTimeoutMin  int           `json:"stuck_timeout_min,omitempty"`
}

// ToolPolicies defines allow/deny lists for agent tool access.
type ToolPolicies struct {
	Allowed []string `json:"allowed,omitempty"`
	Denied  []string `json:"denied,omitempty"`
}
