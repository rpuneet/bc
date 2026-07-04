package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
)

// reposHarness wires a ReposHandler backed by an in-memory agent manager.
func reposHarness(t *testing.T, defaultRepo string, agents ...*agent.Agent) *http.ServeMux {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), ".bc")
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0o750); err != nil {
		t.Fatal(err)
	}
	mgr := agent.NewManager(stateDir)
	for _, a := range agents {
		if err := mgr.RegisterStopped(a); err != nil {
			t.Fatalf("register %s: %v", a.Name, err)
		}
	}
	svc := agent.NewAgentService(mgr, nil, nil)
	mux := http.NewServeMux()
	NewReposHandler(svc, defaultRepo).Register(mux)
	return mux
}

type reposResponse struct {
	Default string     `json:"default"`
	Repos   []RepoView `json:"repos"`
}

func TestRepos_ListDistinct(t *testing.T) {
	mux := reposHarness(t, "/repos/mycel",
		&agent.Agent{Name: "a1", Role: agent.Role("engineer"), Repo: "/repos/mycel"},
		&agent.Agent{Name: "a2", Role: agent.Role("engineer"), Repo: "/repos/mycel"},
		&agent.Agent{Name: "b1", Role: agent.Role("engineer"), Repo: "/repos/other"},
		&agent.Agent{Name: "orphan", Role: agent.Role("engineer")}, // no repo — excluded
	)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/repos", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp reposResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Default != "/repos/mycel" {
		t.Errorf("default = %q, want /repos/mycel", resp.Default)
	}
	if len(resp.Repos) != 2 {
		t.Fatalf("repos = %+v, want 2 distinct entries", resp.Repos)
	}
	// Sorted by path: /repos/mycel before /repos/other.
	if resp.Repos[0].Path != "/repos/mycel" || resp.Repos[0].Name != "mycel" || resp.Repos[0].AgentCount != 2 {
		t.Errorf("first = %+v, want /repos/mycel (mycel) count 2", resp.Repos[0])
	}
	if resp.Repos[1].Path != "/repos/other" || resp.Repos[1].AgentCount != 1 {
		t.Errorf("second = %+v, want /repos/other count 1", resp.Repos[1])
	}
}

func TestRepos_DefaultListedWithoutAgents(t *testing.T) {
	mux := reposHarness(t, "/repos/fresh")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/repos", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp reposResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Repos) != 1 || resp.Repos[0].Path != "/repos/fresh" || resp.Repos[0].AgentCount != 0 {
		t.Errorf("repos = %+v, want the boot repo with 0 agents", resp.Repos)
	}
}

func TestRepos_NilServiceEmptyList(t *testing.T) {
	mux := http.NewServeMux()
	NewReposHandler(nil, "").Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/repos", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp reposResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Repos) != 0 {
		t.Errorf("repos = %+v, want empty", resp.Repos)
	}
	if resp.Repos == nil {
		// JSON shape check: "repos" must be [] not null.
		if body := rec.Body.String(); !json.Valid([]byte(body)) {
			t.Errorf("invalid body: %s", body)
		}
	}
}

func TestRepos_MethodNotAllowed(t *testing.T) {
	mux := reposHarness(t, "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/repos", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
