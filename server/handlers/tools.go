package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/tool"
)

// ToolHandler handles /api/tools routes.
type ToolHandler struct {
	store *tool.Store
}

// NewToolHandler creates a ToolHandler.
func NewToolHandler(store *tool.Store) *ToolHandler {
	return &ToolHandler{store: store}
}

// Register mounts tool routes on mux.
func (h *ToolHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/tools/check", h.checkAll)
	mux.HandleFunc("/api/tools", h.list)
	mux.HandleFunc("/api/tools/", h.byName)
}

func (h *ToolHandler) list(w http.ResponseWriter, r *http.Request) {
	store := h.store
	switch r.Method {
	case http.MethodGet:
		// Support ?type=cli&type=mcp filtering
		opts := tool.ListOptions{}
		if types := r.URL.Query()["type"]; len(types) > 0 {
			opts.Types = types
		}
		tools, err := store.ListWithOptions(r.Context(), opts)
		if err != nil {
			httpInternalError(w, "list tools", err)
			return
		}
		if tools == nil {
			tools = []*tool.Tool{}
		}
		limit, offset := parsePagination(r, 50)
		if offset >= len(tools) {
			tools = []*tool.Tool{}
		} else {
			tools = tools[offset:]
			if len(tools) > limit {
				tools = tools[:limit]
			}
		}
		writeJSON(w, http.StatusOK, tools)

	case http.MethodPost:
		var t tool.Tool
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := store.Add(r.Context(), &t); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		created, err := store.Get(r.Context(), t.Name)
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, created)

	default:
		methodNotAllowed(w)
	}
}

// checkAll is the manual force-refresh: it runs a fresh health check on
// every tool right now and persists the result via store.CheckAll,
// independent of the background auto-check loop's own schedule (see
// runToolHealthLoop in server/build_services.go).
func (h *ToolHandler) checkAll(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	results, err := h.store.CheckAll(r.Context())
	if err != nil {
		httpInternalError(w, "check tools", err)
		return
	}
	if results == nil {
		results = []tool.HealthResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *ToolHandler) byName(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/tools/"), "/", 2)
	name := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	if name == "" {
		httpError(w, "tool name required", http.StatusBadRequest)
		return
	}

	switch sub {
	case "":
		h.tool(w, r, name)
	case "enable":
		h.setEnabled(w, r, name, true)
	case "disable":
		h.setEnabled(w, r, name, false)
	default:
		httpError(w, "not found", http.StatusNotFound)
	}
}

func (h *ToolHandler) tool(w http.ResponseWriter, r *http.Request, name string) {
	store := h.store
	switch r.Method {
	case http.MethodGet:
		t, err := store.Get(r.Context(), name)
		if err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		if t == nil {
			httpError(w, "tool not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, t)

	case http.MethodPut:
		var t tool.Tool
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			httpError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		t.Name = name
		if err := store.Update(r.Context(), &t); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := store.Get(r.Context(), name)
		if err != nil {
			httpInternalError(w, "operation failed", err)
			return
		}
		writeJSON(w, http.StatusOK, updated)

	case http.MethodDelete:
		if err := store.Delete(r.Context(), name); err != nil {
			httpError(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		methodNotAllowed(w)
	}
}

func (h *ToolHandler) setEnabled(w http.ResponseWriter, r *http.Request, name string, enabled bool) {
	store := h.store
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := store.SetEnabled(r.Context(), name, enabled); err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}
