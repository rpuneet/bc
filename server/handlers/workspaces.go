package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// WorkspacesHandler exposes the global multi-workspace registry API.
// Unlike WorkspaceHandler (singular, /api/workspace/*) which reports on the
// currently-active workspace, this handler manages the registry itself:
// list / add / remove / activate entries so bcd can hold several workspaces
// open at once.
type WorkspacesHandler struct {
	registry *workspace.Registry
	// activeRefresh is called after SetActive so the server can reload any
	// caches that depend on the active workspace (optional).
	activeRefresh func()
	// agentSvc is consulted when returning per-workspace detail so the
	// /api/workspaces/{id} response can include the agent count for the
	// active workspace. It may be nil.
	agentSvc *agent.AgentService
}

// NewWorkspacesHandler constructs a registry handler.
func NewWorkspacesHandler(registry *workspace.Registry, agentSvc *agent.AgentService) *WorkspacesHandler {
	return &WorkspacesHandler{registry: registry, agentSvc: agentSvc}
}

// SetActiveRefreshHook installs a callback that fires after a successful
// POST /api/workspaces/{id}/activate. Useful for reloading server state.
func (h *WorkspacesHandler) SetActiveRefreshHook(fn func()) {
	h.activeRefresh = fn
}

// Register mounts all /api/workspaces routes on mux.
//
// REST surface:
//
//	GET    /api/workspaces              -> list
//	POST   /api/workspaces              -> add (body: {path, name?, alias?})
//	GET    /api/workspaces/{id}         -> detail
//	PATCH  /api/workspaces/{id}         -> update name/alias/github
//	DELETE /api/workspaces/{id}         -> unregister
//	POST   /api/workspaces/{id}/activate -> set active
func (h *WorkspacesHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/workspaces", h.collection)
	mux.HandleFunc("/api/workspaces/", h.item)
}

// collection handles list + create at /api/workspaces.
func (h *WorkspacesHandler) collection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.add(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// item routes requests under /api/workspaces/{id}[/action].
//
// The path segments we care about are:
//
//	/api/workspaces/{id}              -> GET/PATCH/DELETE
//	/api/workspaces/{id}/activate     -> POST
//
// Any other sub-path returns 404 at this layer; scoped dispatch for resource
// sub-routes (agents, channels, etc.) lives in the WorkspaceScope middleware.
func (h *WorkspacesHandler) item(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/workspaces/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	// Split at most once so an id containing '/' (should not happen) still
	// carves out a clean head.
	id, tail, _ := strings.Cut(rest, "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	if tail == "" {
		switch r.Method {
		case http.MethodGet:
			h.detail(w, r, id)
		case http.MethodPatch:
			h.update(w, r, id)
		case http.MethodDelete:
			h.unregister(w, r, id)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
		return
	}

	switch tail {
	case "activate":
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		h.activate(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

type registryEntryView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	Alias          string `json:"alias,omitempty"`
	GithubURL      string `json:"github_url,omitempty"`
	GithubFullName string `json:"github_full_name,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	LastUsedAt     string `json:"last_used_at,omitempty"`
	Active         bool   `json:"active"`
}

func viewOf(entry *workspace.RegistryEntry, activeID string) registryEntryView {
	v := registryEntryView{
		ID:             entry.ID,
		Name:           entry.Name,
		Path:           entry.Path,
		Alias:          entry.Alias,
		GithubURL:      entry.GithubURL,
		GithubFullName: entry.GithubFullName,
		Active:         entry.ID != "" && entry.ID == activeID,
	}
	if !entry.CreatedAt.IsZero() {
		v.CreatedAt = entry.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if !entry.LastUsedAt.IsZero() {
		v.LastUsedAt = entry.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return v
}

func (h *WorkspacesHandler) activeID() string {
	if h.registry == nil {
		return ""
	}
	if active := h.registry.GetActive(); active != nil {
		if active.ID != "" {
			return active.ID
		}
		return workspace.ComputeWorkspaceID(active.Path)
	}
	return ""
}

// list returns all registered workspaces.
func (h *WorkspacesHandler) list(w http.ResponseWriter, _ *http.Request) {
	if h.registry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": []registryEntryView{}})
		return
	}
	activeID := h.activeID()
	entries := h.registry.List()
	out := make([]registryEntryView, 0, len(entries))
	for i := range entries {
		out = append(out, viewOf(&entries[i], activeID))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspaces": out,
		"active":     activeID,
	})
}

// add registers a new workspace by path.
// Body: { "path": "/abs/path", "name": "optional", "alias": "optional" }
func (h *WorkspacesHandler) add(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		http.Error(w, `{"error":"registry unavailable"}`, http.StatusInternalServerError)
		return
	}
	var body struct {
		Path  string `json:"path"`
		Name  string `json:"name,omitempty"`
		Alias string `json:"alias,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		http.Error(w, `{"error":"path is required"}`, http.StatusBadRequest)
		return
	}
	absPath, err := filepath.Abs(body.Path)
	if err != nil {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}
	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		http.Error(w, `{"error":"path does not exist or is not a directory"}`, http.StatusBadRequest)
		return
	}
	name := body.Name
	if name == "" {
		name = filepath.Base(absPath)
	}

	if err := h.registry.RegisterWithAlias(absPath, name, body.Alias); err != nil {
		if errors.As(err, new(*workspace.AliasConflictError)) {
			http.Error(w, `{"error":"alias already in use"}`, http.StatusConflict)
			return
		}
		httpInternalError(w, "register workspace", err)
		return
	}
	if saveErr := h.registry.Save(); saveErr != nil {
		httpInternalError(w, "save registry", saveErr)
		return
	}
	entry := h.registry.Find(absPath)
	if entry == nil {
		http.Error(w, `{"error":"register succeeded but entry missing"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, viewOf(entry, h.activeID()))
}

// detail returns one workspace including light health info.
func (h *WorkspacesHandler) detail(w http.ResponseWriter, r *http.Request, id string) {
	entry := h.resolve(id)
	if entry == nil {
		http.NotFound(w, r)
		return
	}
	view := viewOf(entry, h.activeID())
	out := map[string]any{
		"workspace": view,
	}
	// Best-effort worktree list + agent count for the *active* workspace.
	if view.Active && h.agentSvc != nil {
		if agents, err := h.agentSvc.List(r.Context(), agent.ListOptions{}); err == nil {
			out["agent_count"] = len(agents)
		}
	}
	// Worktree directory enumeration (subdirs of .bc/agents/).
	agentsDir := filepath.Join(entry.Path, ".bc", "agents")
	var worktrees []string
	if dents, err := os.ReadDir(agentsDir); err == nil {
		for _, d := range dents {
			if d.IsDir() {
				worktrees = append(worktrees, d.Name())
			}
		}
	}
	out["worktrees"] = worktrees
	writeJSON(w, http.StatusOK, out)
}

// update patches name / alias / github metadata.
func (h *WorkspacesHandler) update(w http.ResponseWriter, r *http.Request, id string) {
	entry := h.resolve(id)
	if entry == nil {
		http.NotFound(w, r)
		return
	}
	var body struct {
		Name           *string `json:"name,omitempty"`
		Alias          *string `json:"alias,omitempty"`
		GithubURL      *string `json:"github_url,omitempty"`
		GithubFullName *string `json:"github_full_name,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if body.Name != nil {
		entry.Name = *body.Name
	}
	if body.Alias != nil {
		if setErr := h.registry.SetAlias(entry.Path, *body.Alias); setErr != nil {
			if errors.As(setErr, new(*workspace.AliasConflictError)) {
				http.Error(w, `{"error":"alias already in use"}`, http.StatusConflict)
				return
			}
			httpInternalError(w, "set alias", setErr)
			return
		}
	}
	if body.GithubURL != nil {
		entry.GithubURL = *body.GithubURL
	}
	if body.GithubFullName != nil {
		entry.GithubFullName = *body.GithubFullName
	}
	if err := h.registry.Save(); err != nil {
		httpInternalError(w, "save registry", err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(entry, h.activeID()))
}

// unregister removes the workspace from the registry. Does NOT delete .bc/.
func (h *WorkspacesHandler) unregister(w http.ResponseWriter, r *http.Request, id string) {
	entry := h.resolve(id)
	if entry == nil {
		http.NotFound(w, r)
		return
	}
	h.registry.Unregister(entry.Path)
	// If we just removed the active workspace, clear it.
	if active := h.registry.GetActive(); active == nil || active.Path == entry.Path {
		if err := h.registry.SetActive(""); err != nil {
			log.Warn("unregister: clear active failed", "error", err)
		}
	}
	if err := h.registry.Save(); err != nil {
		httpInternalError(w, "save registry", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// activate sets the workspace as active.
func (h *WorkspacesHandler) activate(w http.ResponseWriter, r *http.Request, id string) {
	entry := h.resolve(id)
	if entry == nil {
		http.NotFound(w, r)
		return
	}
	if err := h.registry.SetActive(entry.Path); err != nil {
		httpInternalError(w, "set active", err)
		return
	}
	h.registry.Touch(entry.Path)
	if err := h.registry.Save(); err != nil {
		httpInternalError(w, "save registry", err)
		return
	}
	if h.activeRefresh != nil {
		h.activeRefresh()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":    entry.ID,
		"workspace": viewOf(entry, entry.ID),
	})
}

func (h *WorkspacesHandler) resolve(idOrAlias string) *workspace.RegistryEntry {
	if h.registry == nil {
		return nil
	}
	if entry := h.registry.FindByID(idOrAlias); entry != nil {
		return entry
	}
	return h.registry.FindByNameOrAlias(idOrAlias)
}

// decodeJSONBody is a small helper that decodes r.Body into dst, tolerating
// an empty body (all fields zero).
func decodeJSONBody(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }() //nolint:errcheck
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}
