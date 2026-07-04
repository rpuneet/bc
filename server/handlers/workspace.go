package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// WorkspaceHandler handles /api/workspace routes.
type WorkspaceHandler struct {
	svc *agent.AgentService
	ws  *workspace.Workspace
}

// NewWorkspaceHandler creates a WorkspaceHandler.
func NewWorkspaceHandler(svc *agent.AgentService, ws *workspace.Workspace) *WorkspaceHandler {
	return &WorkspaceHandler{svc: svc, ws: ws}
}

// Register mounts workspace routes on mux.
func (h *WorkspaceHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/workspace", h.status) // root = status
	mux.HandleFunc("/api/workspace/status", h.status)
	mux.HandleFunc("/api/workspace/roles", h.roles)
	mux.HandleFunc("/api/workspace/up", h.up)
	mux.HandleFunc("/api/workspace/down", h.down)
}

func (h *WorkspaceHandler) status(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	svc := h.svc
	ws := h.ws
	agents, err := svc.List(r.Context(), agent.ListOptions{})
	if err != nil {
		httpInternalError(w, "list agents", err)
		return
	}
	runningCount := 0
	for _, a := range agents {
		if a.State != agent.StateStopped && a.State != agent.StateError {
			runningCount++
		}
	}
	nickname := ""
	if ws.Config != nil {
		nickname = ws.Config.User.Name
	}
	// Enrich with config details
	result := map[string]any{
		"name":          ws.Name(),
		"nickname":      nickname,
		"root_dir":      ws.RootDir,
		"state_dir":     ws.StateDir(),
		"agent_count":   len(agents),
		"running_count": runningCount,
		"is_healthy":    true,
	}

	if ws.Config != nil {
		cfg := ws.Config
		result["server"] = map[string]any{
			"host": cfg.Server.Host,
			"port": cfg.Server.Port,
		}
		result["runtime"] = map[string]any{
			"default": cfg.Runtime.Default,
		}
		result["storage"] = map[string]any{
			"sqlite_path": cfg.Storage.SQLite.Path,
		}

		// Gateway status
		gateways := map[string]bool{}
		if cfg.Gateways.Slack != nil {
			gateways["slack"] = cfg.Gateways.Slack.Enabled
		}
		if cfg.Gateways.Telegram != nil {
			gateways["telegram"] = cfg.Gateways.Telegram.Enabled
		}
		if cfg.Gateways.Discord != nil {
			gateways["discord"] = cfg.Gateways.Discord.Enabled
		}
		if len(gateways) > 0 {
			result["gateways"] = gateways
		}

		result["version"] = cfg.Version
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *WorkspaceHandler) roles(w http.ResponseWriter, r *http.Request) {
	ws := h.ws
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	roles, err := ws.RoleManager.LoadAllRoles()
	if err != nil {
		httpInternalError(w, "list roles", err)
		return
	}

	// Resolve each role via BFS inheritance so response includes
	// inherited MCP servers, secrets, commands, rules, etc.
	resolved := make(map[string]*workspace.ResolvedRole, len(roles))
	for name := range roles {
		if res, resolveErr := ws.RoleManager.ResolveRole(name); resolveErr == nil {
			resolved[name] = res
		}
	}
	writeJSON(w, http.StatusOK, resolved)
}

func (h *WorkspaceHandler) up(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Tool    string `json:"tool"`
		Runtime string `json:"runtime"`
	}
	if r.ContentLength > 0 {
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	a, err := svc.Create(r.Context(), agent.CreateOptions{
		Name:    "root",
		Role:    agent.RoleRoot,
		Tool:    req.Tool,
		Runtime: req.Runtime,
	})
	if err != nil {
		if isAlreadyRunning(err) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
			return
		}
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "started", "session": a.Session})
}

func (h *WorkspaceHandler) down(w http.ResponseWriter, r *http.Request) {
	svc := h.svc
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	stopped, err := svc.StopAll(r.Context())
	if err != nil {
		httpInternalError(w, "operation failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"stopped": stopped})
}

// isAlreadyRunning detects "already running" errors from Create/Start.
func isAlreadyRunning(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return len(msg) > 0 && (strings.Contains(msg, "already running") || strings.Contains(msg, "session is alive"))
}
