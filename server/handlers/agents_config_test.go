package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/bc/pkg/agent"
	"github.com/rpuneet/bc/server"
	"github.com/rpuneet/bc/server/handlers"
	"github.com/rpuneet/bc/server/ws"
)

// TestAgentHandler_GetConfig verifies GET /api/agents/{name}/config returns
// 200 + an AgentConfig DTO for a known agent. This is a regression test for
// the bug where getAgentConfig used the stale h.svc closure instead of the
// per-request resolved svc, causing 404s on non-launch workspaces.
func TestAgentHandler_GetConfig(t *testing.T) {
	dir := setupWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}

	// Prepare a fake worktree with a CLAUDE.md for system_prompt reading.
	wtDir := filepath.Join(dir, "wt", "zen-zebra")
	if err := os.MkdirAll(wtDir, 0750); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	const prompt = "you are zen-zebra, an agent for testing."
	if err := os.WriteFile(filepath.Join(wtDir, "CLAUDE.md"), []byte(prompt), 0600); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	// Seed an agent record directly via RegisterStopped (no runtime needed).
	if err := mgr.RegisterStopped(&agent.Agent{
		Name:           "zen-zebra",
		Role:           agent.Role("engineer"),
		Workspace:      dir,
		Tool:           "claude",
		RuntimeBackend: "tmux",
		WorktreeDir:    wtDir,
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	// Sanity: /api/agents/{name} resolves (mirrors the bug report: no-suffix
	// worked; /config did not).
	base := get(t, ts.URL+"/api/agents/zen-zebra")
	assertStatus(t, base, http.StatusOK)
	_ = base.Body.Close()

	// The actual regression check: /config sub-route must also return 200.
	resp := get(t, ts.URL+"/api/agents/zen-zebra/config")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)

	var dto struct {
		WorktreePath   string   `json:"worktree_path"`
		SystemPrompt   string   `json:"system_prompt"`
		RuntimeBackend string   `json:"runtime_backend"`
		Tool           string   `json:"tool"`
		MCPServers     []string `json:"mcp_servers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		t.Fatalf("decode config dto: %v", err)
	}
	if dto.WorktreePath != wtDir {
		t.Errorf("worktree_path = %q, want %q", dto.WorktreePath, wtDir)
	}
	if dto.SystemPrompt != prompt {
		t.Errorf("system_prompt = %q, want %q", dto.SystemPrompt, prompt)
	}
	if dto.Tool != "claude" {
		t.Errorf("tool = %q, want %q", dto.Tool, "claude")
	}
	if dto.RuntimeBackend != "tmux" {
		t.Errorf("runtime_backend = %q, want %q", dto.RuntimeBackend, "tmux")
	}
	// mcp_servers is omitempty + empty slice on DTO, so nil on decode is fine.
	// We just assert the field type is consistent (i.e. decoded as array, not scalar).
	if len(dto.MCPServers) != 0 {
		t.Errorf("mcp_servers unexpected content: %v", dto.MCPServers)
	}
}

// TestAgentHandler_GetConfigReadsFromContext verifies the scoped-workspace
// path: getAgentConfig must resolve the agent through the per-request svc
// from the context WorkspaceView, NOT the launch-time closure. This is the
// regression that caused /api/agents/zen-zebra/config to 404 in prod when
// the launch workspace had no such agent but the scoped one did.
//
// Mirrors the pattern in context_read_test.go for template/secret/cost
// handlers. Without the fix this test returns 404 (launch svc doesn't
// know about zen-zebra); with the fix it returns 200 using ctx svc.
func TestAgentHandler_GetConfigReadsFromContext(t *testing.T) {
	// Launch workspace: empty (no agents).
	launchDir := t.TempDir()
	launchStateDir := filepath.Join(launchDir, ".bc")
	if err := os.MkdirAll(filepath.Join(launchStateDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir launch agents: %v", err)
	}
	launchMgr := agent.NewManager(launchStateDir)
	launchSvc := agent.NewAgentService(launchMgr, nil, nil)

	// Ctx workspace: has zen-zebra.
	ctxDir := t.TempDir()
	ctxStateDir := filepath.Join(ctxDir, ".bc")
	if err := os.MkdirAll(filepath.Join(ctxStateDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir ctx agents: %v", err)
	}
	wtDir := filepath.Join(ctxDir, "wt", "zen-zebra")
	if err := os.MkdirAll(wtDir, 0750); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	const prompt = "ctx-only zen-zebra prompt"
	if err := os.WriteFile(filepath.Join(wtDir, "CLAUDE.md"), []byte(prompt), 0600); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	ctxMgr := agent.NewManager(ctxStateDir)
	ctxSvc := agent.NewAgentService(ctxMgr, nil, nil)
	if err := ctxMgr.RegisterStopped(&agent.Agent{
		Name:           "zen-zebra",
		Role:           agent.Role("engineer"),
		Workspace:      ctxDir,
		Tool:           "claude",
		RuntimeBackend: "tmux",
		WorktreeDir:    wtDir,
	}); err != nil {
		t.Fatalf("register ctx agent: %v", err)
	}

	// Wire handler with the *launch* svc, but install a ctx resolver that
	// returns the ctx svc. A correct handler must use ctx svc.
	hub := ws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	h := handlers.NewAgentHandler(launchSvc, nil, nil, hub)
	mux := http.NewServeMux()
	h.Register(mux)

	withCtxView(t, &handlers.WorkspaceView{Agents: ctxSvc})

	status, body := doJSON(t, mux, http.MethodGet, "/api/agents/zen-zebra/config", nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s (handler likely using stale launch closure)", status, body)
	}
	if !strings.Contains(string(body), prompt) {
		t.Errorf("response missing ctx prompt %q, body=%s", prompt, body)
	}
}

// TestPatchAgentConfig verifies PATCH /api/agents/{name}/config writes the
// system prompt to the correct file based on the agent's tool. A gemini
// agent must land in GEMINI.md (via provider ConfigAdapter fallback), NOT
// the previously-hardcoded CLAUDE.md; a claude agent still writes CLAUDE.md.
func TestPatchAgentConfig(t *testing.T) {
	cases := []struct {
		name      string
		tool      string
		wantFile  string
		wrongFile string
	}{
		{"claude", "claude", "CLAUDE.md", "GEMINI.md"},
		{"gemini", "gemini", "Gemini.md", "CLAUDE.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupWorkspace(t)
			stateDir := filepath.Join(dir, ".bc")
			if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
				t.Fatalf("mkdir agents: %v", err)
			}
			wtDir := filepath.Join(dir, "wt", "zen-zebra")
			if err := os.MkdirAll(wtDir, 0750); err != nil {
				t.Fatalf("mkdir worktree: %v", err)
			}

			mgr := agent.NewManager(stateDir)
			svc := agent.NewAgentService(mgr, nil, nil)
			if err := mgr.RegisterStopped(&agent.Agent{
				Name:           "zen-zebra",
				Role:           agent.Role("engineer"),
				Workspace:      dir,
				Tool:           tc.tool,
				RuntimeBackend: "tmux",
				WorktreeDir:    wtDir,
			}); err != nil {
				t.Fatalf("register agent: %v", err)
			}

			ts := buildTestServerWithServices(t, server.Services{Agents: svc})
			defer ts.Close()

			prompt := "prompt-for-" + tc.tool
			payload := `{"system_prompt":"` + prompt + `"}`
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch,
				ts.URL+"/api/agents/zen-zebra/config", strings.NewReader(payload))
			if err != nil {
				t.Fatalf("new req: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			assertStatus(t, resp, http.StatusOK)

			// The correct prompt file must exist with the written prompt.
			got, err := os.ReadFile(filepath.Join(wtDir, tc.wantFile)) //nolint:gosec // trusted test path
			if err != nil {
				t.Fatalf("expected %s to exist: %v", tc.wantFile, err)
			}
			if string(got) != prompt {
				t.Errorf("%s body = %q, want %q", tc.wantFile, string(got), prompt)
			}
			// The wrong file must NOT have been created (regression guard for
			// the hardcoded-CLAUDE.md bug).
			if _, err := os.Stat(filepath.Join(wtDir, tc.wrongFile)); !os.IsNotExist(err) {
				t.Errorf("%s unexpectedly present (err=%v); PATCH wrote to wrong file", tc.wrongFile, err)
			}
		})
	}
}

// TestAgentHandler_GetConfigNotFound verifies /config returns 404 for an
// unknown agent (not a generic handler 404 leak).
func TestAgentHandler_GetConfigNotFound(t *testing.T) {
	dir := setupWorkspace(t)
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/does-not-exist/config")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusNotFound)
}
