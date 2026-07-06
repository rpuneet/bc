package mcp

import (
	"context"

	"github.com/rpuneet/mycel/pkg/workspace"
)

// ─── bc://workspace/status ───────────────────────────────────────────────────

type workspaceStatusPayload struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	StateDir  string `json:"state_dir"`
	AgentsDir string `json:"agents_dir"`
	IsV2      bool   `json:"is_v2"`
}

func (s *Server) readWorkspaceStatus() (string, error) {
	payload := workspaceStatusPayload{
		Name:      s.ws.Name(),
		Path:      s.ws.RootDir,
		StateDir:  s.ws.StateDir(),
		IsV2:      true, // All current workspaces use the v2 TOML config format
		AgentsDir: s.ws.AgentsDir(),
	}
	return marshalJSON(payload)
}

// ─── bc://agents ─────────────────────────────────────────────────────────────

type agentPayload struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	State    string `json:"state"`
	Tool     string `json:"tool,omitempty"`
	Team     string `json:"team,omitempty"`
	Worktree string `json:"worktree,omitempty"`
	Session  string `json:"session,omitempty"`
	IsRoot   bool   `json:"is_root,omitempty"`
}

func (s *Server) readAgents() (string, error) {
	agents := s.agents.ListAgents()
	payload := make([]agentPayload, 0, len(agents))
	for _, a := range agents {
		payload = append(payload, agentPayload{
			Name:     a.Name,
			Role:     string(a.Role),
			State:    string(a.State),
			Tool:     a.Tool,
			Team:     a.Team,
			Worktree: a.WorktreeDir,
			Session:  a.Session,
			IsRoot:   a.IsRoot,
		})
	}
	return marshalJSON(payload)
}

// ─── bc://channels ────────────────────────────────────────────────────────────

func (s *Server) readChannels() (string, error) {
	// Channels are now managed by pkg/notify — return empty list.
	return marshalJSON([]struct{}{})
}

// ─── bc://costs ───────────────────────────────────────────────────────────────

type costsPayload struct {
	Workspace *costSummaryPayload  `json:"workspace"`
	ByAgent   []costSummaryPayload `json:"by_agent,omitempty"`
}

type costSummaryPayload struct {
	AgentID      string  `json:"agent_id,omitempty"`
	Model        string  `json:"model,omitempty"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

func (s *Server) readCosts() (string, error) {
	payload := costsPayload{}

	ws, err := s.costs.WorkspaceSummary(context.Background())
	if err == nil && ws != nil {
		payload.Workspace = &costSummaryPayload{
			InputTokens:  ws.InputTokens,
			OutputTokens: ws.OutputTokens,
			TotalTokens:  ws.TotalTokens,
			TotalCostUSD: ws.TotalCostUSD,
		}
	}

	byAgent, err := s.costs.SummaryByAgent(context.Background())
	if err == nil {
		payload.ByAgent = make([]costSummaryPayload, 0, len(byAgent))
		for _, a := range byAgent {
			payload.ByAgent = append(payload.ByAgent, costSummaryPayload{
				AgentID:      a.AgentID,
				Model:        a.Model,
				InputTokens:  a.InputTokens,
				OutputTokens: a.OutputTokens,
				TotalTokens:  a.TotalTokens,
				TotalCostUSD: a.TotalCostUSD,
			})
		}
	}

	return marshalJSON(payload)
}

// ─── bc://roles ───────────────────────────────────────────────────────────────

type rolePayload struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	MCPServers  []string `json:"mcp_servers,omitempty"`
	Secrets     []string `json:"secrets,omitempty"`
}

func (s *Server) readRoles() (string, error) {
	// Roles live in the single global database — reuse the workspace's role
	// manager (already backed by the global store) rather than opening a
	// per-workspace bc.db that holds no roles.
	rm := s.ws.RoleManager
	if rm == nil {
		var err error
		rm, err = workspace.NewGlobalRoleManager(s.ws.StateDir())
		if err != nil {
			return marshalJSON([]rolePayload{})
		}
	}
	roles, err := rm.LoadAllRoles()
	if err != nil {
		return marshalJSON([]rolePayload{})
	}

	payload := make([]rolePayload, 0, len(roles))
	for _, r := range roles {
		payload = append(payload, rolePayload{
			Name:        r.Metadata.Name,
			Description: r.Metadata.Description,
			MCPServers:  r.Metadata.MCPServers,
			Secrets:     r.Metadata.Secrets,
		})
	}
	return marshalJSON(payload)
}

// ─── bc://tools ───────────────────────────────────────────────────────────────

type toolInfoPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Configured  bool   `json:"configured"`
}

func (s *Server) readTools() (string, error) {
	// Report the known AI agent tools; check basic availability via PATH.
	tools := []toolInfoPayload{
		{Name: "claude", Description: "Claude Code (Anthropic)", Configured: commandExists("claude")},
		{Name: "agy", Description: "Antigravity CLI (Google Gemini)", Configured: commandExists("agy")},
		{Name: "cursor", Description: "Cursor (terminal mode)", Configured: commandExists("cursor")},
		{Name: "codex", Description: "Codex CLI (OpenAI)", Configured: commandExists("codex")},
	}
	return marshalJSON(tools)
}
