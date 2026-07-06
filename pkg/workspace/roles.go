// Package workspace provides workspace/project management.
package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RoleMetadata contains the parsed frontmatter from a role file.
type RoleMetadata struct {
	Settings     map[string]any    `yaml:"settings,omitempty"`      // Claude settings overrides (e.g., model, permissions)
	Rules        map[string]string `yaml:"rules,omitempty"`         // Rule files written to .claude/rules/*.md
	Agents       map[string]string `yaml:"agents,omitempty"`        // Agent templates written to .claude/agents/*.md
	Skills       map[string]string `yaml:"skills,omitempty"`        // Skill files written to .claude/skills/*.md
	Commands     map[string]string `yaml:"commands,omitempty"`      // Command files written to .claude/commands/*.md
	PromptStop   string            `yaml:"prompt_stop,omitempty"`   // Sent when agent is stopped
	PromptCreate string            `yaml:"prompt_create,omitempty"` // Sent when agent is created
	PromptStart  string            `yaml:"prompt_start,omitempty"`  // Sent when agent is started/restarted
	Name         string            `yaml:"name"`                    // Role name (e.g., "engineer", "manager")
	PromptDelete string            `yaml:"prompt_delete,omitempty"` // Sent when agent is deleted
	Description  string            `yaml:"description,omitempty"`   // Human-readable role description
	Review       string            `yaml:"review,omitempty"`        // REVIEW.md content for the role
	Plugins      []string          `yaml:"plugins,omitempty"`       // Claude Code plugins to install on agent start
	Secrets      []string          `yaml:"secrets,omitempty"`       // Secret names needed by MCP env vars
	MCPServers   []string          `yaml:"mcp_servers,omitempty"`   // MCP servers available to this role
	ParentRoles  []string          `yaml:"parent_roles,omitempty"`  // Roles to inherit from (capabilities, prompts)
	CLITools     []string          `yaml:"cli_tools,omitempty"`     // CLI tools expected in agent PATH (e.g., gh, aws, wrangler)
}

// Role represents a parsed role file with metadata and prompt content.
type Role struct {
	FilePath string       // Path to the role file
	Prompt   string       // Markdown body after frontmatter
	Metadata RoleMetadata // Parsed YAML frontmatter
}

// Description returns a brief description for the role.
// Uses Metadata.Description if set, otherwise extracts from the first heading in Prompt.
func (r *Role) Description() string {
	if r.Metadata.Description != "" {
		return r.Metadata.Description
	}

	// Extract from first heading in prompt
	lines := strings.Split(r.Prompt, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// RoleManager handles role operations for a workspace.
// All role data is stored in SQL (SQLite or Postgres) via the RoleStore.
// The rolesDir field is retained only for migration from legacy file-based storage.
type RoleManager struct {
	store    *RoleStore // SQL store (required for all operations)
	roles    map[string]*Role
	rolesDir string // kept for migration only
}

// DefaultBaseRole is the foundational role all other roles inherit from.
// It provides the bc MCP server so every agent can communicate with the workspace.
const DefaultBaseRole = `---
name: base
description: Base role — provides bc MCP server, workspace communication, and shared commands to all agents
mcp_servers:
  - bc
prompt_create: |
  You have been created as a new agent in a mycel workspace.
  Use the report_status MCP tool to set your initial task.
  Check #all and #engineering channels for context.
prompt_start: |
  You are online. Use report_status to update your current task.
  Check channels for any messages sent while you were offline.
prompt_stop: |
  You are being stopped. Save any important state.
  Post a status update to #engineering if you have work in progress.
commands:
  status: |
    Check all channels for recent messages addressed to you.
    Report your current task using the report_status MCP tool.
    Query workspace costs using query_costs.
  notify: |
    Check recent notifications across all subscribed channels.
    Summarize any messages that are relevant to your current work.
  announce: |
    Send an announcement to the #all channel using send_message.
    Include your agent name as the sender.
rules:
  workspace-communication: |
    All workspace operations MUST use bc MCP tools. Never use mycel CLI commands directly.
    Available MCP tools: send_message, report_status, query_costs.
    Always include your agent name as sender when sending messages.
  channel-etiquette: |
    Use the right channel for the right purpose:
    - #all for broadcast announcements only
    - #engineering for technical coordination
    - #general for general discussion
    - #merge for PR review requests
    - #ops for system health and cost reports
    Do not spam #all with routine updates.
---

# bc Agent

You are an agent in a **bc** workspace — a CLI-first AI agent orchestration system.

## MCP Tools

All workspace operations use bc MCP tools (never CLI commands):

| Tool | Purpose | Parameters |
|------|---------|------------|
| **send_message** | Send messages to channels | {channel, message, sender} |
| **report_status** | Update your current task | {agent, task} |
| **query_costs** | Check workspace costs | {agent?} |

## Channels

- **#all** — Broadcast announcements
- **#engineering** — Engineering coordination
- **#general** — General discussion
- **#merge** — PR review pipeline
- **#ops** — System health and costs

## Guidelines

- Report your status when starting or finishing work
- Post to the appropriate channel, not #all, for routine updates
- Use #merge when a PR is ready for review
- Check channels for messages before starting new work
`

// DefaultRootRole returns the default content for root.md.
const DefaultRootRole = `---
name: root
description: Root orchestrator — singleton workspace owner
parent_roles:
  - base
mcp_servers:
  - github
secrets:
  - GITHUB_PERSONAL_ACCESS_TOKEN
prompt_start: |
  You are back online. Check #all channel for any messages you missed.
  Report your status using the report_status MCP tool.
---

# Root Agent

You are the root agent for this mycel workspace.

## Additional MCP Tools
- **create_agent**: Create new agents {name, role, tool}

## Responsibilities
- Oversee all workspace operations
- Create and coordinate agents
- Handle merge queue for the main branch
- Monitor workspace health and costs
`

// DefaultRoles contains the built-in role definitions for the bc agent team.
// These are written to .bc/roles/ if the files don't already exist.
var DefaultRoles = map[string]string{
	"feature-dev": `---
name: feature-dev
description: Feature developer — implements tasks in isolated worktrees
parent_roles:
  - base
mcp_servers:
  - github
secrets:
  - GITHUB_PERSONAL_ACCESS_TOKEN
prompt_start: |
  Check #engineering channel for any new assignments or updates.
---

# Feature Developer

You implement features, fix bugs, and write tests in an isolated git worktree.

## Workflow
1. Read the assigned issue, create a feature branch (feat/<issue>-<slug>)
2. Implement with tests, run make check
3. Open a PR and post to #merge when ready for review
`,
	"go-reviewer": `---
name: go-reviewer
description: Go code quality reviewer
parent_roles:
  - feature-dev
---

# Go Reviewer

Review Go pull requests for correctness, security, and test coverage.
Use the github MCP to fetch PR diffs and leave inline review comments.
Block merges on security issues or broken tests; suggest rather than block on style.
`,
	"web-reviewer": `---
name: web-reviewer
description: Web/TypeScript UI reviewer
parent_roles:
  - feature-dev
---

# Web Reviewer

Review React/TypeScript pull requests for correctness and component patterns.
Use the github MCP to fetch PR diffs and leave inline review comments.
Hooks cannot be tested without a DOM in Ink — test exported helpers instead.
`,
	"designer": `---
name: designer
description: Design system and Web UI specialist
parent_roles:
  - feature-dev
---

# Designer

Maintain the design token system, build React/Ink TUI components, and implement
CSS/Tailwind changes for the web dashboard. Accessibility is non-negotiable.
`,
	"product-manager": `---
name: product-manager
description: Product coordination and epic management
parent_roles:
  - base
mcp_servers:
  - github
secrets:
  - GITHUB_PERSONAL_ACCESS_TOKEN
---

# Product Manager

Break down goals into epics and issues, assign work to agents, and review
completed work against acceptance criteria. Use GitHub issues for all tracking.
`,
	"docs": `---
name: docs
description: Documentation writer
parent_roles:
  - feature-dev
---

# Documentation Writer

Write and maintain README, CONTRIBUTING, API docs, and CLAUDE.md.
Update docs in the same PR as the feature. Use concrete examples.
`,
}

// NewRoleManager creates a new role manager for the given workspace state directory.
// It opens a SQLite-backed RoleStore at stateDir/bc.db automatically and performs
// a best-effort migration of any .md files in the roles directory.
// If the store cannot be opened, operations that require the store will return errors.
func NewRoleManager(stateDir string) *RoleManager {
	rolesDir := filepath.Join(stateDir, "roles")
	rm := &RoleManager{
		rolesDir: rolesDir,
		roles:    make(map[string]*Role),
	}

	dbPath := filepath.Join(stateDir, "bc.db")
	store, err := NewRoleStore(dbPath)
	if err == nil {
		rm.store = store
		// Best-effort migration of legacy filesystem roles
		_, _ = store.MigrateFromFiles(rolesDir) //nolint:errcheck // best-effort
	}

	return rm
}

// NewRoleManagerWithStore creates a role manager backed by a SQLite store.
// The filesystem rolesDir is still used for migration and backward compatibility.
func NewRoleManagerWithStore(stateDir string, store *RoleStore) *RoleManager {
	return &RoleManager{
		store:    store,
		rolesDir: filepath.Join(stateDir, "roles"),
		roles:    make(map[string]*Role),
	}
}

// Store returns the underlying RoleStore, or nil if filesystem-only.
func (rm *RoleManager) Store() *RoleStore {
	return rm.store
}

// RolesDir returns the roles directory path.
func (rm *RoleManager) RolesDir() string {
	return rm.rolesDir
}

// EnsureRolesDir creates the roles directory if it doesn't exist.
func (rm *RoleManager) EnsureRolesDir() error {
	return os.MkdirAll(rm.rolesDir, 0750)
}

// EnsureDefaultRoles writes built-in default roles to the store if they don't already exist.
// Returns the names of any roles that were created.
func (rm *RoleManager) EnsureDefaultRoles() ([]string, error) {
	if rm.store == nil {
		return nil, fmt.Errorf("role store is required")
	}

	var created []string
	for name, content := range DefaultRoles {
		if rm.store.Has(name) {
			continue
		}
		role, err := ParseRoleFile([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse default role %s: %w", name, err)
		}
		if role.Metadata.Name == "" {
			role.Metadata.Name = name
		}
		if err := rm.store.Save(role); err != nil {
			return nil, fmt.Errorf("failed to save default role %s: %w", name, err)
		}
		rm.roles[name] = role
		created = append(created, name)
	}
	return created, nil
}

// EnsureDefaultRoot ensures the base and root roles exist in the store.
// Returns true if the root role was created, false if it already existed.
func (rm *RoleManager) EnsureDefaultRoot() (bool, error) {
	if rm.store == nil {
		return false, fmt.Errorf("role store is required")
	}

	// Always ensure base role exists (root and all roles inherit from it)
	if !rm.store.Has("base") {
		role, err := ParseRoleFile([]byte(DefaultBaseRole))
		if err != nil {
			return false, fmt.Errorf("failed to parse default base role: %w", err)
		}
		if saveErr := rm.store.Save(role); saveErr != nil {
			return false, fmt.Errorf("failed to save default base role: %w", saveErr)
		}
		rm.roles["base"] = role
	}

	if rm.store.Has("root") {
		return false, nil
	}

	role, err := ParseRoleFile([]byte(DefaultRootRole))
	if err != nil {
		return false, fmt.Errorf("failed to parse default root role: %w", err)
	}
	if saveErr := rm.store.Save(role); saveErr != nil {
		return false, fmt.Errorf("failed to save default root role: %w", saveErr)
	}
	rm.roles["root"] = role

	return true, nil
}

// NormalizeRoleName canonicalises a role name by replacing underscores
// with hyphens and lower-casing, preventing duplicates like
// "product_manager" vs "product-manager".
func NormalizeRoleName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// LoadRole loads and parses a single role from the SQL store.
func (rm *RoleManager) LoadRole(name string) (*Role, error) {
	name = NormalizeRoleName(name)

	// Check cache first
	if role, ok := rm.roles[name]; ok {
		return role, nil
	}

	if rm.store == nil {
		return nil, fmt.Errorf("role store is required")
	}

	role, err := rm.store.Load(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load role %q: %w", name, err)
	}

	rm.roles[name] = role
	return role, nil
}

// LoadAllRoles loads all roles from the SQL store.
func (rm *RoleManager) LoadAllRoles() (map[string]*Role, error) {
	if rm.store == nil {
		return nil, fmt.Errorf("role store is required")
	}

	all, err := rm.store.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load roles from store: %w", err)
	}
	for name, role := range all {
		rm.roles[name] = role
	}
	return rm.roles, nil
}

// GetRole returns a cached role by name.
func (rm *RoleManager) GetRole(name string) (*Role, bool) {
	role, ok := rm.roles[name]
	return role, ok
}

// HasRole checks if a role exists (cached or in store).
func (rm *RoleManager) HasRole(name string) bool {
	name = NormalizeRoleName(name)

	// Check cache
	if _, ok := rm.roles[name]; ok {
		return true
	}

	// Check store
	if rm.store != nil {
		return rm.store.Has(name)
	}

	return false
}

// ParseRoleFile parses a role file with YAML frontmatter and markdown body.
// The frontmatter is delimited by --- on its own lines.
func ParseRoleFile(data []byte) (*Role, error) {
	content := string(data)

	// Check for frontmatter delimiter
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		// No frontmatter - treat entire content as prompt
		return &Role{
			Metadata: RoleMetadata{},
			Prompt:   strings.TrimSpace(content),
		}, nil
	}

	// Find the closing delimiter
	// Skip the opening "---\n"
	rest := content[4:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		// No closing delimiter - treat as no frontmatter
		return &Role{
			Metadata: RoleMetadata{},
			Prompt:   strings.TrimSpace(content),
		}, nil
	}

	// Extract frontmatter and body
	frontmatter := rest[:endIdx]
	body := rest[endIdx+4:] // Skip "\n---"

	// Skip any trailing newline after closing delimiter
	if strings.HasPrefix(body, "\n") {
		body = body[1:]
	} else if strings.HasPrefix(body, "\r\n") {
		body = body[2:]
	}

	// Parse YAML frontmatter
	var metadata RoleMetadata
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(frontmatter)))
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return &Role{
		Metadata: metadata,
		Prompt:   strings.TrimSpace(body),
	}, nil
}

// WriteRole writes a role to the SQL store.
func (rm *RoleManager) WriteRole(role *Role) error {
	name := NormalizeRoleName(role.Metadata.Name)
	if name == "" {
		return fmt.Errorf("role name is required")
	}
	role.Metadata.Name = name

	if rm.store == nil {
		return fmt.Errorf("role store is required")
	}

	if err := rm.store.Save(role); err != nil {
		return fmt.Errorf("failed to save role to store: %w", err)
	}
	rm.roles[name] = role
	return nil
}

// FormatRoleFile formats a role as a markdown file with YAML frontmatter.
func FormatRoleFile(role *Role) (string, error) {
	var buf bytes.Buffer

	// Write frontmatter
	buf.WriteString("---\n")

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(role.Metadata); err != nil {
		return "", fmt.Errorf("failed to encode metadata: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("failed to close encoder: %w", err)
	}

	buf.WriteString("---\n\n")

	// Write prompt body
	buf.WriteString(role.Prompt)
	if !strings.HasSuffix(role.Prompt, "\n") {
		buf.WriteString("\n")
	}

	return buf.String(), nil
}

// ResolvedRole contains the fully resolved role after BFS inheritance merge.
type ResolvedRole struct { //nolint:govet // field order matches API contract
	Settings     map[string]any    // Merged settings (child overrides parent)
	Rules        map[string]string // Merged rule files (child overrides parent)
	Agents       map[string]string // Merged agent templates
	Skills       map[string]string // Merged skill files
	Commands     map[string]string // Merged command files
	PromptStart  string            // Lifecycle: sent on agent start/restart
	Name         string            // Role name
	PromptStop   string            // Lifecycle: sent on agent stop
	PromptDelete string            // Lifecycle: sent on agent delete
	PromptCreate string            // Lifecycle: sent on agent create
	Prompt       string            // Merged prompt body (child + parent)
	Review       string            // REVIEW.md content
	Plugins      []string          // Unioned plugins from all ancestors
	Secrets      []string          // Unioned secret names from all ancestors
	MCPServers   []string          // Unioned MCP servers from all ancestors
	CLITools     []string          // Unioned CLI tools from all ancestors
	Description  string            // Human-readable role description
}

// ResolveRole returns a fully resolved role after BFS inheritance merge.
// It walks ParentRoles breadth-first, collecting all ancestors, then merges
// them in ancestor-first order so the target role's values take precedence:
//   - Prompt: parent prompts first, child prompt appended last (blank-line separated).
//   - Lists (MCPServers, Secrets, Plugins, CLITools): parent ∪ child, deduped, stable order.
//   - Maps (Settings, Rules, Agents, Skills, Commands): child keys override parent keys.
//   - Lifecycle strings (PromptCreate/Start/Stop/Delete): child wins if non-empty, else nearest ancestor.
//   - Name, Description, Review: target role always wins.
//
// Missing parent roles are skipped silently. Cycles are guarded — no infinite loop.
func (rm *RoleManager) ResolveRole(name string) (*ResolvedRole, error) {
	root, err := rm.LoadRole(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load role %q: %w", name, err)
	}

	// BFS from the target role outward through ParentRoles.
	// visited keys are normalised role names.
	visited := make(map[string]bool)
	queue := []*Role{root}
	var order []*Role

	visited[NormalizeRoleName(root.Metadata.Name)] = true

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)

		for _, parentName := range cur.Metadata.ParentRoles {
			pn := NormalizeRoleName(parentName)
			if visited[pn] {
				continue // already queued or visited — cycle/dup guard
			}
			visited[pn] = true
			parent, loadErr := rm.LoadRole(pn)
			if loadErr != nil {
				// Missing parent role: skip silently rather than fail.
				continue
			}
			queue = append(queue, parent)
		}
	}

	// Reverse so deepest ancestors come first; target role comes last.
	// This gives us "parent text first, child text last" merge order.
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	// Merge all roles ancestor-first, target-last.
	var prompts []string

	mcpSeen := make(map[string]bool)
	var mcpServers []string

	secretSeen := make(map[string]bool)
	var secrets []string

	pluginSeen := make(map[string]bool)
	var plugins []string

	cliToolSeen := make(map[string]bool)
	var cliTools []string

	var settings map[string]any
	var rules map[string]string
	var agents map[string]string
	var skills map[string]string
	var commands map[string]string

	var promptCreate, promptStart, promptStop, promptDelete, review string

	for _, r := range order {
		if r.Prompt != "" {
			prompts = append(prompts, r.Prompt)
		}

		for _, s := range r.Metadata.MCPServers {
			if !mcpSeen[s] {
				mcpSeen[s] = true
				mcpServers = append(mcpServers, s)
			}
		}
		for _, s := range r.Metadata.Secrets {
			if !secretSeen[s] {
				secretSeen[s] = true
				secrets = append(secrets, s)
			}
		}
		for _, p := range r.Metadata.Plugins {
			if !pluginSeen[p] {
				pluginSeen[p] = true
				plugins = append(plugins, p)
			}
		}
		for _, t := range r.Metadata.CLITools {
			if !cliToolSeen[t] {
				cliToolSeen[t] = true
				cliTools = append(cliTools, t)
			}
		}

		// Maps: later (child) entries override earlier (parent) entries.
		for k, v := range r.Metadata.Settings {
			if settings == nil {
				settings = make(map[string]any)
			}
			settings[k] = v
		}
		for k, v := range r.Metadata.Rules {
			if rules == nil {
				rules = make(map[string]string)
			}
			rules[k] = v
		}
		for k, v := range r.Metadata.Agents {
			if agents == nil {
				agents = make(map[string]string)
			}
			agents[k] = v
		}
		for k, v := range r.Metadata.Skills {
			if skills == nil {
				skills = make(map[string]string)
			}
			skills[k] = v
		}
		for k, v := range r.Metadata.Commands {
			if commands == nil {
				commands = make(map[string]string)
			}
			commands[k] = v
		}

		// Lifecycle strings: last non-empty wins (child comes last so child overrides parent).
		if r.Metadata.PromptCreate != "" {
			promptCreate = r.Metadata.PromptCreate
		}
		if r.Metadata.PromptStart != "" {
			promptStart = r.Metadata.PromptStart
		}
		if r.Metadata.PromptStop != "" {
			promptStop = r.Metadata.PromptStop
		}
		if r.Metadata.PromptDelete != "" {
			promptDelete = r.Metadata.PromptDelete
		}
		if r.Metadata.Review != "" {
			review = r.Metadata.Review
		}
	}

	return &ResolvedRole{
		Name:         root.Metadata.Name,
		Description:  root.Metadata.Description,
		Prompt:       strings.Join(prompts, "\n\n"),
		MCPServers:   mcpServers,
		Secrets:      secrets,
		Plugins:      plugins,
		CLITools:     cliTools,
		Settings:     settings,
		Rules:        rules,
		Agents:       agents,
		Skills:       skills,
		Commands:     commands,
		PromptCreate: promptCreate,
		PromptStart:  promptStart,
		PromptStop:   promptStop,
		PromptDelete: promptDelete,
		Review:       review,
	}, nil
}

// DeleteRole removes a role by name from the SQL store.
func (rm *RoleManager) DeleteRole(name string) error {
	name = NormalizeRoleName(name)
	if name == "" {
		return fmt.Errorf("role name is required")
	}

	if rm.store == nil {
		return fmt.Errorf("role store is required")
	}

	if err := rm.store.Delete(name); err != nil {
		return fmt.Errorf("failed to delete role from store: %w", err)
	}
	delete(rm.roles, name)
	return nil
}

// GetMCPServers returns the MCP server associations for a role.
func (rm *RoleManager) GetMCPServers(roleName string) ([]string, error) {
	role, err := rm.LoadRole(roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to load role: %w", err)
	}
	return role.Metadata.MCPServers, nil
}

// SetMCPServers replaces the MCP server list for a role.
func (rm *RoleManager) SetMCPServers(roleName string, servers []string) error {
	role, err := rm.LoadRole(roleName)
	if err != nil {
		return fmt.Errorf("failed to load role: %w", err)
	}

	role.Metadata.MCPServers = servers

	if err := rm.WriteRole(role); err != nil {
		return fmt.Errorf("failed to save role: %w", err)
	}

	return nil
}

// AddMCPServer adds an MCP server association to a role if not already present.
func (rm *RoleManager) AddMCPServer(roleName, server string) error {
	role, err := rm.LoadRole(roleName)
	if err != nil {
		return fmt.Errorf("failed to load role: %w", err)
	}

	for _, s := range role.Metadata.MCPServers {
		if s == server {
			return nil // Already associated
		}
	}

	role.Metadata.MCPServers = append(role.Metadata.MCPServers, server)

	if err := rm.WriteRole(role); err != nil {
		return fmt.Errorf("failed to save role: %w", err)
	}

	return nil
}

// RemoveMCPServer removes an MCP server association from a role.
func (rm *RoleManager) RemoveMCPServer(roleName, server string) error {
	role, err := rm.LoadRole(roleName)
	if err != nil {
		return fmt.Errorf("failed to load role: %w", err)
	}

	filtered := make([]string, 0, len(role.Metadata.MCPServers))
	for _, s := range role.Metadata.MCPServers {
		if s != server {
			filtered = append(filtered, s)
		}
	}
	role.Metadata.MCPServers = filtered

	if err := rm.WriteRole(role); err != nil {
		return fmt.Errorf("failed to save role: %w", err)
	}

	return nil
}
