// repos.go — the minimal repo surface for single-tenant bcd.
//
//	GET /api/repos
//
// Returns the distinct repos known to the daemon: every repo referenced
// by an agent (agents carry their repo path as a property) plus the repo
// bcd was booted against (`default`). List only — there are no IDs, no
// active state, and no registration endpoint: adding a repo IS creating
// an agent with that repo path (or running `mycel up` inside it).
//
// Response shape:
//
//	{
//	  "repos":   [{"path": "/abs/repo", "name": "repo", "agent_count": 2}],
//	  "default": "/abs/repo"
//	}
package handlers

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/rpuneet/mycel/pkg/agent"
)

// RepoView is one row of the GET /api/repos response.
type RepoView struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	AgentCount int    `json:"agent_count"`
}

// ReposHandler serves GET /api/repos.
type ReposHandler struct {
	svc *agent.AgentService
	// defaultRepo is the repo bcd was booted against — new agents default
	// their repo to it. May be empty (workspace-less boot).
	defaultRepo string
}

// NewReposHandler constructs the handler. svc may be nil (empty list).
func NewReposHandler(svc *agent.AgentService, defaultRepo string) *ReposHandler {
	return &ReposHandler{svc: svc, defaultRepo: defaultRepo}
}

// Register mounts the handler on mux.
func (h *ReposHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/repos", h.list)
}

// list handles GET /api/repos.
func (h *ReposHandler) list(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	counts := map[string]int{}
	if h.svc != nil {
		counts = h.svc.Manager().RepoCounts(r.Context())
	}
	// The boot repo is always listed, even before its first agent exists.
	if h.defaultRepo != "" {
		if _, ok := counts[h.defaultRepo]; !ok {
			counts[h.defaultRepo] = 0
		}
	}

	repos := make([]RepoView, 0, len(counts))
	for path, n := range counts {
		repos = append(repos, RepoView{
			Path:       path,
			Name:       filepath.Base(path),
			AgentCount: n,
		})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Path < repos[j].Path })

	writeJSON(w, http.StatusOK, map[string]any{
		"repos":   repos,
		"default": h.defaultRepo,
	})
}
