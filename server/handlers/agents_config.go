package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// agentConfigDTO is the response body for GET /api/agents/{name}/config.
type agentConfigDTO struct { //nolint:govet // field order matches JSON/API contract
	WorktreePath   string   `json:"worktree_path"`
	SystemPrompt   string   `json:"system_prompt"`
	RuntimeBackend string   `json:"runtime_backend,omitempty"`
	Tool           string   `json:"tool,omitempty"`
	Session        string   `json:"session,omitempty"`
	MCPServers     []string `json:"mcp_servers,omitempty"`
}

// getAgentConfig handles GET /api/agents/{name}/config.
func (h *AgentHandler) getAgentConfig(w http.ResponseWriter, r *http.Request, name string) {
	a, err := h.svc.Get(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Determine worktree path: use stored WorktreeDir or compute from workspace root.
	wtDir := a.WorktreeDir
	if wtDir == "" {
		wtDir = h.svc.Manager().WorktreePath(name)
	}

	dto := agentConfigDTO{
		WorktreePath:   wtDir,
		RuntimeBackend: a.RuntimeBackend,
		Tool:           a.Tool,
		Session:        a.Session,
		MCPServers:     []string{},
	}

	// Read CLAUDE.md from the agent's worktree.
	if wtDir != "" {
		claudePath := filepath.Join(wtDir, "CLAUDE.md")
		if data, readErr := os.ReadFile(claudePath); readErr == nil { //nolint:gosec // trusted path
			dto.SystemPrompt = string(data)
		}
	}

	// Resolve MCP servers from the agent's role via the role manager.
	if h.ws != nil && h.ws.RoleManager != nil && string(a.Role) != "" {
		if resolved, resolveErr := h.ws.RoleManager.ResolveRole(string(a.Role)); resolveErr == nil && len(resolved.MCPServers) > 0 {
			dto.MCPServers = resolved.MCPServers
		}
	}

	writeJSON(w, http.StatusOK, dto)
}

// patchAgentConfig handles PATCH /api/agents/{name}/config.
func (h *AgentHandler) patchAgentConfig(w http.ResponseWriter, r *http.Request, name string) {
	a, err := h.svc.Get(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		SystemPrompt string `json:"system_prompt"`
	}
	if decErr := json.NewDecoder(r.Body).Decode(&req); decErr != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Determine worktree path: use stored WorktreeDir or compute from workspace root.
	wtDir := a.WorktreeDir
	if wtDir == "" {
		wtDir = h.svc.Manager().WorktreePath(name)
	}
	if wtDir == "" {
		httpError(w, "worktree path not available for this agent", http.StatusUnprocessableEntity)
		return
	}

	// Ensure directory exists.
	if mkErr := os.MkdirAll(wtDir, 0750); mkErr != nil {
		httpInternalError(w, "create worktree dir", mkErr)
		return
	}

	claudePath := filepath.Join(wtDir, "CLAUDE.md")
	//nolint:gosec // trusted path under workspace root
	if writeErr := os.WriteFile(claudePath, []byte(req.SystemPrompt), 0600); writeErr != nil {
		httpInternalError(w, "write CLAUDE.md", writeErr)
		return
	}

	// Return the updated config as the response body.
	dto := agentConfigDTO{
		WorktreePath:   wtDir,
		RuntimeBackend: a.RuntimeBackend,
		Tool:           a.Tool,
		Session:        a.Session,
		SystemPrompt:   req.SystemPrompt,
		MCPServers:     []string{},
	}
	if h.ws != nil && h.ws.RoleManager != nil && string(a.Role) != "" {
		if resolved, resolveErr := h.ws.RoleManager.ResolveRole(string(a.Role)); resolveErr == nil && len(resolved.MCPServers) > 0 {
			dto.MCPServers = resolved.MCPServers
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// forkAgent handles POST /api/agents/{source}/fork.
func (h *AgentHandler) forkAgent(w http.ResponseWriter, r *http.Request, sourceName string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		httpError(w, "name is required", http.StatusBadRequest)
		return
	}

	newAgent, err := h.svc.ForkAgent(r.Context(), sourceName, req.Name)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	dto := toDTO(newAgent)

	// Enrich with MCP servers from role.
	if h.ws != nil && h.ws.RoleManager != nil && dto.Role != "" {
		if resolved, resolveErr := h.ws.RoleManager.ResolveRole(dto.Role); resolveErr == nil {
			dto.MCPServers = resolved.MCPServers
		}
	}

	writeJSON(w, http.StatusCreated, dto)
}
