package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/server"
)

// TestAgentHandler_GetConfig verifies GET /api/agents/{name}/config returns
// 200 + an AgentConfig DTO for a known agent. This is a regression test for
// the bug where getAgentConfig used the stale h.svc closure instead of the
// per-request resolved svc, causing 404s on non-launch repos.
func TestAgentHandler_GetConfig(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")
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

// TestPatchAgentConfig verifies PATCH /api/agents/{name}/config writes the
// system prompt to the correct file based on the agent's tool. An agy agent
// must land in AGENTS.md (via the provider ConfigAdapter), NOT the
// previously-hardcoded CLAUDE.md; a claude agent still writes CLAUDE.md.
func TestPatchAgentConfig(t *testing.T) {
	cases := []struct {
		name      string
		tool      string
		wantFile  string
		wrongFile string
	}{
		{"claude", "claude", "CLAUDE.md", "AGENTS.md"},
		{"agy", "agy", "AGENTS.md", "CLAUDE.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupHome(t)
			stateDir := filepath.Join(dir, ".mycel")
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

// TestPatchAgentConfig_Resources verifies PATCH /api/agents/{name}/config
// persists the per-agent Docker CPU/memory caps and the model override,
// echoes them back, and leaves an unrelated field (system prompt) untouched
// when it is omitted from the request.
func TestPatchAgentConfig_Resources(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	wtDir := filepath.Join(dir, "wt", "zen-zebra")
	if err := os.MkdirAll(wtDir, 0750); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	const prompt = "keep me"
	if err := os.WriteFile(filepath.Join(wtDir, "CLAUDE.md"), []byte(prompt), 0600); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)
	if err := mgr.RegisterStopped(&agent.Agent{
		Name:           "zen-zebra",
		Role:           agent.Role("engineer"),
		Workspace:      dir,
		Tool:           "claude",
		RuntimeBackend: "docker",
		WorktreeDir:    wtDir,
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	// PATCH only cpus/memory_mb/model — system_prompt omitted.
	payload := `{"cpus":1.5,"memory_mb":3072,"model":"fable"}`
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

	var dto struct {
		Model    string  `json:"model"`
		CPUs     float64 `json:"cpus"`
		MemoryMB int64   `json:"memory_mb"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&dto); decErr != nil {
		t.Fatalf("decode config dto: %v", decErr)
	}
	if dto.CPUs != 1.5 || dto.MemoryMB != 3072 || dto.Model != "fable" {
		t.Fatalf("response = %+v, want cpus=1.5 mem=3072 model=fable", dto)
	}

	// Persisted on the in-memory record.
	a := mgr.GetAgent("zen-zebra")
	if a == nil {
		t.Fatal("agent gone after patch")
	}
	if a.CPUs != 1.5 || a.MemoryMB != 3072 || a.Model != "fable" {
		t.Fatalf("record = {cpus:%v mem:%v model:%q}, want {1.5 3072 fable}", a.CPUs, a.MemoryMB, a.Model)
	}

	// Omitted system_prompt must be left untouched on disk.
	got, err := os.ReadFile(filepath.Join(wtDir, "CLAUDE.md")) //nolint:gosec // trusted test path
	if err != nil || string(got) != prompt {
		t.Fatalf("CLAUDE.md = %q (err=%v), want it unchanged (%q)", string(got), err, prompt)
	}

	// A negative cap is rejected.
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch,
		ts.URL+"/api/agents/zen-zebra/config", strings.NewReader(`{"cpus":-1}`))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do negative: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	assertStatus(t, resp2, http.StatusBadRequest)
}

// TestPatchAgentConfig_ConcurrentPartial verifies that two concurrent
// partial PATCHes — one setting only cpus, the other only memory_mb —
// both survive. This is the regression guard for the lost-update race:
// the merge must happen under the manager lock, not against a snapshot
// read before the write.
func TestPatchAgentConfig_ConcurrentPartial(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")
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
		Tool:           "claude",
		RuntimeBackend: "docker",
		WorktreeDir:    wtDir,
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	patch := func(body string) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch,
			ts.URL+"/api/agents/zen-zebra/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("patch %s: %v", body, err)
			return
		}
		_ = resp.Body.Close()
	}

	// Hammer cpus-only and memory-only PATCHes concurrently. Under the old
	// snapshot-merge, the later writer overwrote the other field back to its
	// stale value; under the locked partial merge, both must stick.
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); patch(`{"cpus":3}`) }()
		go func() { defer wg.Done(); patch(`{"memory_mb":5120}`) }()
	}
	wg.Wait()

	a := mgr.GetAgent("zen-zebra")
	if a == nil {
		t.Fatal("agent gone after concurrent patch")
	}
	if a.CPUs != 3 {
		t.Errorf("cpus = %v, want 3 (lost update)", a.CPUs)
	}
	if a.MemoryMB != 5120 {
		t.Errorf("memory_mb = %v, want 5120 (lost update)", a.MemoryMB)
	}
}

// TestAgentHandler_GetConfigNotFound verifies /config returns 404 for an
// unknown agent (not a generic handler 404 leak).
func TestAgentHandler_GetConfigNotFound(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")
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
