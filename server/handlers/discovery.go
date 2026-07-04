package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/rpuneet/mycel/pkg/workspace"
)

// DiscoveryHandler exposes the repo-discovery scanners used by the web
// folder picker: local filesystem scan, GitHub repo list, clone, plus
// the small GitHub auth surface (/api/auth/github) used to authorize the
// repo-list call. Discovery is list-only — adding a repo to mycel is done
// by creating an agent with that repo path, so nothing is registered here.
type DiscoveryHandler struct{}

// NewDiscoveryHandler constructs the handler.
func NewDiscoveryHandler() *DiscoveryHandler {
	return &DiscoveryHandler{}
}

// Register mounts all /api/repos/discover/* and /api/auth/github
// routes on mux.
func (h *DiscoveryHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/repos/discover/local", h.discoverLocal)
	mux.HandleFunc("/api/repos/discover/github", h.discoverGithub)
	mux.HandleFunc("/api/repos/clone", h.clone)
	mux.HandleFunc("/api/auth/github", h.githubAuth)
}

// discoverLocal scans a filesystem root for git repos.
//
// POST body: {"root": "/abs/path", "depth": 3}
func (h *DiscoveryHandler) discoverLocal(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Root  string `json:"root"`
		Depth int    `json:"depth,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if body.Root == "" {
		http.Error(w, `{"error":"root is required"}`, http.StatusBadRequest)
		return
	}
	cands, err := workspace.ScanLocal(r.Context(), workspace.ScanOptions{
		Root:  body.Root,
		Depth: body.Depth,
	})
	if err != nil {
		http.Error(w, `{"error":"scan failed: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

// discoverGithub lists the authenticated user's repositories.
//
// POST body: {"query": "substring"}
func (h *DiscoveryHandler) discoverGithub(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Query string `json:"query,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	repos, err := workspace.ListGithubRepos(r.Context(), body.Query)
	if err != nil {
		if errors.Is(err, workspace.ErrGithubNotAuthenticated) {
			http.Error(w, `{"error":"github not authenticated"}`, http.StatusUnauthorized)
			return
		}
		http.Error(w, `{"error":"github api: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repos": repos})
}

// clone clones a URL into target. The result is NOT registered anywhere —
// the caller creates an agent with the returned path to bring the repo in.
//
// POST body: {"url": "...", "target": "/abs/parent", "name": "optional"}
func (h *DiscoveryHandler) clone(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var body struct {
		URL    string `json:"url"`
		Target string `json:"target"`
		Name   string `json:"name,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if body.URL == "" || body.Target == "" {
		http.Error(w, `{"error":"url and target are required"}`, http.StatusBadRequest)
		return
	}
	dest, err := workspace.Clone(r.Context(), body.URL, body.Target, body.Name)
	if err != nil {
		http.Error(w, `{"error":"clone failed: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	name := body.Name
	if name == "" {
		name = filepath.Base(dest)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"path": dest,
		"name": name,
	})
}

// githubAuth handles GET/POST/DELETE on /api/auth/github.
//
//	GET    -> {"connected": bool, "login": "..."}
//	POST   body {"token": "..."} validates + stores
//	DELETE removes the token
func (h *DiscoveryHandler) githubAuth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.githubAuthStatus(w, r)
	case http.MethodPost:
		h.githubAuthSet(w, r)
	case http.MethodDelete:
		h.githubAuthDelete(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *DiscoveryHandler) githubAuthStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"connected": workspace.GithubConnected()})
}

func (h *DiscoveryHandler) githubAuthSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if body.Token == "" {
		http.Error(w, `{"error":"token is required"}`, http.StatusBadRequest)
		return
	}
	// Validate before persisting so we don't store a garbage token.
	login, err := workspace.ValidateGithubToken(r.Context(), body.Token)
	if err != nil {
		if errors.Is(err, workspace.ErrGithubNotAuthenticated) {
			http.Error(w, `{"error":"token rejected by github"}`, http.StatusUnauthorized)
			return
		}
		http.Error(w, `{"error":"validate failed: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	if wErr := workspace.WriteGithubToken(body.Token); wErr != nil {
		httpInternalError(w, "write token", wErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "login": login})
}

func (h *DiscoveryHandler) githubAuthDelete(w http.ResponseWriter, _ *http.Request) {
	if err := workspace.DeleteGithubToken(); err != nil {
		httpInternalError(w, "delete token", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
