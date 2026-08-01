package handlers_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/server"
)

// setupAgentForMCPAdd registers a stopped agent with a real worktree dir
// (so addAgentMCP has somewhere to write .mcp.json) and returns the test
// server plus the worktree path.
func setupAgentForMCPAdd(t *testing.T) (ts string, wtDir string, closeFn func()) {
	t.Helper()
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")
	if err := os.MkdirAll(filepath.Join(stateDir, "agents"), 0750); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}

	wt := filepath.Join(dir, "wt", "zen-zebra")
	if err := os.MkdirAll(wt, 0750); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)
	if err := mgr.RegisterStopped(&agent.Agent{
		Name:           "zen-zebra",
		Role:           agent.Role("engineer"),
		Workspace:      dir,
		Tool:           "claude",
		RuntimeBackend: "docker", // avoid the tmux "claude mcp add" side-effect goroutine
		WorktreeDir:    wt,
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	server := buildTestServerWithServices(t, server.Services{Agents: svc})
	return server.URL, wt, server.Close
}

// seedGlobalMCP writes a real, connectable definition into the user-global
// MCP registry (~/.mycel/mcps.json) so addAgentMCP has something to resolve
// req.Name against.
func seedGlobalMCP(t *testing.T, cfg *mcp.ServerConfig) {
	t.Helper()
	path, err := home.GlobalMCPConfig()
	if err != nil {
		t.Fatalf("resolve global mcp config path: %v", err)
	}
	if err := mcp.NewGlobalStore(path).Add(cfg); err != nil {
		t.Fatalf("seed global mcp registry: %v", err)
	}
}

// TestAddAgentMCP_ResolvesRealDefinitionFromRegistry is a regression test
// for the bug where "Add MCP" wrote an empty {} stanza into .mcp.json: the
// web UI only sends a bare name, so the server must resolve the real
// command/url/env from the global MCP registry before persisting.
func TestAddAgentMCP_ResolvesRealDefinitionFromRegistry(t *testing.T) {
	tsURL, wtDir, closeFn := setupAgentForMCPAdd(t)
	defer closeFn()

	seedGlobalMCP(t, &mcp.ServerConfig{
		Name:      "github",
		Transport: mcp.TransportStdio,
		Command:   "npx -y @modelcontextprotocol/server-github",
		Env:       map[string]string{"GITHUB_TOKEN": "${secret:github_token}"},
		Enabled:   true,
	})

	body, err := json.Marshal(map[string]string{"name": "github"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp := post(t, tsURL+"/api/agents/zen-zebra/mcps", "application/json", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusCreated)
	got := readJSON(t, resp)

	if got["command"] != "npx -y @modelcontextprotocol/server-github" {
		t.Errorf("command = %v, want the resolved registry command", got["command"])
	}
	env, _ := got["env"].(map[string]any)
	if env["GITHUB_TOKEN"] != "${secret:github_token}" {
		t.Errorf("env = %v, want GITHUB_TOKEN carried over from the registry", got["env"])
	}

	// The persisted .mcp.json must carry the same real entry — not an
	// empty stanza.
	raw, err := os.ReadFile(filepath.Join(wtDir, ".mcp.json")) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var onDisk struct {
		MCPServers map[string]struct {
			Env     map[string]string `json:"env"`
			Command string            `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	entry, ok := onDisk.MCPServers["github"]
	if !ok {
		t.Fatalf(".mcp.json missing github entry: %s", raw)
	}
	if entry.Command == "" {
		t.Errorf(".mcp.json github entry has empty command — wrote a non-functional stanza: %s", raw)
	}
	if entry.Command != "npx -y @modelcontextprotocol/server-github" {
		t.Errorf(".mcp.json command = %q, want resolved registry command", entry.Command)
	}
}

// TestAddAgentMCP_RejectsUnknownName verifies that a name with no
// definition in the global registry is rejected rather than silently
// written as an empty, non-functional stanza.
func TestAddAgentMCP_RejectsUnknownName(t *testing.T) {
	tsURL, wtDir, closeFn := setupAgentForMCPAdd(t)
	defer closeFn()

	body, err := json.Marshal(map[string]string{"name": "totally-unknown-mcp"})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	resp := post(t, tsURL+"/api/agents/zen-zebra/mcps", "application/json", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	// Nothing should have been written to .mcp.json for the rejected name.
	raw, readErr := os.ReadFile(filepath.Join(wtDir, ".mcp.json")) //nolint:gosec // test fixture path
	if readErr == nil {
		var onDisk struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(raw, &onDisk); err == nil {
			if _, ok := onDisk.MCPServers["totally-unknown-mcp"]; ok {
				t.Errorf(".mcp.json should not contain an entry for the rejected name: %s", raw)
			}
		}
	}
}
