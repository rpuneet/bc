package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/provider"
)

// promptFileForTool returns the filename the given tool writes its system
// prompt to (CLAUDE.md for claude, AGENTS.md for agy, .cursorrules for
// cursor, etc.), resolved via the provider registry's ConfigAdapter. Falls
// back to "CLAUDE.md" with a warning when the tool is empty or unknown.
func promptFileForTool(tool string) string {
	if tool == "" {
		log.Warn("promptFileForTool: empty tool, defaulting to CLAUDE.md")
		return "CLAUDE.md"
	}
	p, ok := provider.DefaultRegistry.Get(tool)
	if !ok {
		log.Warn("promptFileForTool: unknown tool, defaulting to CLAUDE.md", "tool", tool)
		return "CLAUDE.md"
	}
	if adapter := provider.GetConfigAdapter(p); adapter != nil {
		return adapter.PromptFile()
	}
	return provider.NewGenericAdapter(tool).PromptFile()
}

// agentConfigDTO is the response body for GET /api/agents/{name}/config.
type agentConfigDTO struct { //nolint:govet // field order matches JSON/API contract
	WorktreePath   string   `json:"worktree_path"`
	SystemPrompt   string   `json:"system_prompt"`
	RuntimeBackend string   `json:"runtime_backend,omitempty"`
	Tool           string   `json:"tool,omitempty"`
	Model          string   `json:"model,omitempty"`
	Session        string   `json:"session,omitempty"`
	MCPServers     []string `json:"mcp_servers,omitempty"`
	// CPUs / MemoryMB are the agent's per-agent Docker resource caps.
	// Zero means "inherit the fleet default" — the UI shows the fleet
	// value as the placeholder in that case. Enforced only for Docker.
	CPUs     float64 `json:"cpus"`
	MemoryMB int64   `json:"memory_mb"`
}

// getAgentConfig handles GET /api/agents/{name}/config.
func (h *AgentHandler) getAgentConfig(w http.ResponseWriter, r *http.Request, name string) {
	svc := h.svc
	hm := h.home
	a, err := svc.Get(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Determine worktree path: use stored WorktreeDir or compute from repo root.
	wtDir := a.WorktreeDir
	if wtDir == "" {
		wtDir = svc.Manager().WorktreePath(name)
	}

	dto := agentConfigDTO{
		WorktreePath:   wtDir,
		RuntimeBackend: a.RuntimeBackend,
		Tool:           a.Tool,
		Model:          a.Model,
		Session:        a.Session,
		MCPServers:     []string{},
		CPUs:           a.CPUs,
		MemoryMB:       a.MemoryMB,
	}

	// Read the per-tool prompt file (CLAUDE.md / GEMINI.md / .cursorrules / ...)
	// from the agent's worktree. Reject traversal segments so a corrupt
	// worktree path can never read outside the repo.
	wtDir = filepath.Clean(wtDir)
	if wtDir != "." && !strings.Contains(wtDir, "..") {
		promptPath := filepath.Join(wtDir, promptFileForTool(a.Tool))
		if data, readErr := os.ReadFile(promptPath); readErr == nil { //nolint:gosec // trusted path
			dto.SystemPrompt = string(data)
		}
	}

	// Resolve MCP servers from the agent's role via the role manager.
	if hm != nil && hm.RoleManager != nil && string(a.Role) != "" {
		if resolved, resolveErr := hm.RoleManager.ResolveRole(string(a.Role)); resolveErr == nil && len(resolved.MCPServers) > 0 {
			dto.MCPServers = resolved.MCPServers
		}
	}

	writeJSON(w, http.StatusOK, dto)
}

// patchAgentConfig handles PATCH /api/agents/{name}/config.
func (h *AgentHandler) patchAgentConfig(w http.ResponseWriter, r *http.Request, name string) {
	svc := h.svc
	hm := h.home
	a, err := svc.Get(r.Context(), name)
	if err != nil {
		httpError(w, err.Error(), http.StatusNotFound)
		return
	}

	// All fields are optional so a caller can update just the resource caps
	// without clobbering the system prompt (and vice-versa). Pointers let us
	// tell "field omitted" apart from "field set to zero/empty".
	var req struct {
		SystemPrompt *string  `json:"system_prompt"`
		Model        *string  `json:"model"`
		CPUs         *float64 `json:"cpus"`
		MemoryMB     *int64   `json:"memory_mb"`
	}
	if decErr := json.NewDecoder(r.Body).Decode(&req); decErr != nil {
		httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Reject nonsensical caps up front — a negative limit would silently
	// disable the docker flag, which is not what the caller asked for.
	if req.CPUs != nil && *req.CPUs < 0 {
		httpError(w, "cpus must be >= 0", http.StatusBadRequest)
		return
	}
	if req.MemoryMB != nil && *req.MemoryMB < 0 {
		httpError(w, "memory_mb must be >= 0", http.StatusBadRequest)
		return
	}

	// Determine worktree path: use stored WorktreeDir or compute from repo root.
	wtDir := a.WorktreeDir
	if wtDir == "" {
		wtDir = svc.Manager().WorktreePath(name)
	}
	wtDir = filepath.Clean(wtDir)
	if strings.Contains(wtDir, "..") {
		httpError(w, "unsafe worktree path", http.StatusBadRequest)
		return
	}

	// System prompt — only touched when provided (writes the per-tool file).
	if req.SystemPrompt != nil {
		if wtDir == "" || wtDir == "." {
			httpError(w, "worktree path not available for this agent", http.StatusUnprocessableEntity)
			return
		}
		if mkErr := os.MkdirAll(wtDir, 0750); mkErr != nil {
			httpInternalError(w, "create worktree dir", mkErr)
			return
		}
		promptFile := promptFileForTool(a.Tool)
		promptPath := filepath.Join(wtDir, promptFile)
		//nolint:gosec // trusted path under repo root
		if writeErr := os.WriteFile(promptPath, []byte(*req.SystemPrompt), 0600); writeErr != nil {
			httpInternalError(w, "write "+promptFile, writeErr)
			return
		}
	}

	// Model override — persisted on the agent record; reused on next restart.
	if req.Model != nil {
		if setErr := svc.Manager().SetAgentModel(r.Context(), name, *req.Model); setErr != nil {
			httpInternalError(w, "set agent model", setErr)
			return
		}
	}

	// Resource caps — persisted on the agent record; applied on next restart.
	if req.CPUs != nil || req.MemoryMB != nil {
		cpus := a.CPUs
		if req.CPUs != nil {
			cpus = *req.CPUs
		}
		memoryMB := a.MemoryMB
		if req.MemoryMB != nil {
			memoryMB = *req.MemoryMB
		}
		if setErr := svc.SetResources(r.Context(), name, cpus, memoryMB); setErr != nil {
			httpInternalError(w, "set agent resources", setErr)
			return
		}
	}

	// Re-read so the response reflects the persisted record (model/caps).
	if updated, getErr := svc.Get(r.Context(), name); getErr == nil {
		a = updated
	}

	// Return the updated config as the response body.
	dto := agentConfigDTO{
		WorktreePath:   wtDir,
		RuntimeBackend: a.RuntimeBackend,
		Tool:           a.Tool,
		Model:          a.Model,
		Session:        a.Session,
		MCPServers:     []string{},
		CPUs:           a.CPUs,
		MemoryMB:       a.MemoryMB,
	}
	if req.SystemPrompt != nil {
		dto.SystemPrompt = *req.SystemPrompt
	}
	if hm != nil && hm.RoleManager != nil && string(a.Role) != "" {
		if resolved, resolveErr := hm.RoleManager.ResolveRole(string(a.Role)); resolveErr == nil && len(resolved.MCPServers) > 0 {
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

	svc := h.svc
	hm := h.home
	newAgent, err := svc.ForkAgent(r.Context(), sourceName, req.Name)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}

	dto := toDTO(newAgent)

	// Enrich with MCP servers from role.
	if hm != nil && hm.RoleManager != nil && dto.Role != "" {
		if resolved, resolveErr := hm.RoleManager.ResolveRole(dto.Role); resolveErr == nil {
			dto.MCPServers = resolved.MCPServers
		}
	}

	writeJSON(w, http.StatusCreated, dto)
}
