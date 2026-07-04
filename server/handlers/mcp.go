package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/mcp"
)

// MCPHandler handles /api/mcp routes.
//
// TODO(m8d-followup): the HTTP surface currently reads from the
// workspace-scoped *mcp.Store. pkg/mcp now provides a GlobalStore
// (~/.mycel/mcps.json) + LayeredView for workspace-over-global merging; a
// follow-up will teach this handler to return the merged view and
// accept a scope query parameter for Add/Remove.
type MCPHandler struct {
	store *mcp.Store
}

// NewMCPHandler creates an MCPHandler.
func NewMCPHandler(store *mcp.Store) *MCPHandler {
	return &MCPHandler{store: store}
}

// Register mounts MCP server routes on mux.
func (h *MCPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/mcp", h.list)
	mux.HandleFunc("/api/mcp/", h.byName)
}

func (h *MCPHandler) list(w http.ResponseWriter, r *http.Request) {
	store := h.store
	switch r.Method {
	case http.MethodGet:
		servers, err := store.List()
		if err != nil {
			httpInternalError(w, "list mcp servers", err)
			return
		}
		if servers == nil {
			servers = []*mcp.ServerConfig{}
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(servers) {
			servers = []*mcp.ServerConfig{}
		} else {
			servers = servers[offset:]
			if len(servers) > limit {
				servers = servers[:limit]
			}
		}
		writeJSON(w, http.StatusOK, servers)

	case http.MethodPost:
		var cfg mcp.ServerConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := store.Add(&cfg); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		added, err := store.Get(cfg.Name)
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, added)

	default:
		methodNotAllowed(w)
	}
}

func (h *MCPHandler) byName(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/mcp/"), "/", 2)
	name := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	if name == "" {
		httpError(w, "server name required", http.StatusBadRequest)
		return
	}

	switch sub {
	case "":
		h.server(w, r, name)
	case "enable":
		h.setEnabled(w, r, name, true)
	case "disable":
		h.setEnabled(w, r, name, false)
	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

func (h *MCPHandler) server(w http.ResponseWriter, r *http.Request, name string) {
	store := h.store
	switch r.Method {
	case http.MethodGet:
		cfg, err := store.Get(name)
		if err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		if cfg == nil {
			httpError(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	case http.MethodDelete:
		if err := store.Remove(name); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPatch:
		var body struct {
			Env map[string]string `json:"env"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.Env == nil {
			httpError(w, "env field is required", http.StatusBadRequest)
			return
		}
		if err := store.UpdateEnv(name, body.Env); err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		// Echo back the updated record so the UI can reconcile state.
		cfg, err := store.Get(name)
		if err != nil {
			httpInternalError(w, "reload mcp server", err)
			return
		}
		writeJSON(w, http.StatusOK, cfg)

	default:
		methodNotAllowed(w)
	}
}

func (h *MCPHandler) setEnabled(w http.ResponseWriter, r *http.Request, name string, enabled bool) {
	store := h.store
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := store.SetEnabled(name, enabled); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}
