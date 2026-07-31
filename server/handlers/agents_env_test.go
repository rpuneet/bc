package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/server"
)

// TestAgentCreate_InvalidEnvName verifies POST /api/agents rejects env var
// keys that don't match ^[A-Za-z_][A-Za-z0-9_]*$ with a 400 before any
// spawn is attempted.
func TestAgentCreate_InvalidEnvName(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	for _, bad := range []string{"1FOO", "FOO-BAR", "FOO BAR", ""} {
		body, err := json.Marshal(map[string]any{
			"name": "env-api-test",
			"role": "engineer",
			"env":  map[string]string{bad: "x"},
		})
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		resp := post(t, ts.URL+"/api/agents", "application/json", string(body))
		assertStatus(t, resp, http.StatusBadRequest)
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for assertion
		_ = resp.Body.Close()
		if !strings.Contains(string(msg), "invalid env var name") {
			t.Errorf("key %q: expected env validation error, got %s", bad, msg)
		}
	}
}

// TestAgentEnvEndpoint_RoundTrip verifies PUT /api/agents/{name}/env
// replaces the store-backed env map and GET returns it (references
// verbatim), and that invalid keys are rejected with 400.
func TestAgentEnvEndpoint_RoundTrip(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)
	if err := mgr.RegisterStopped(&agent.Agent{
		Name:      "env-edit",
		Role:      agent.Role("engineer"),
		Workspace: dir,
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	url := ts.URL + "/api/agents/env-edit/env"

	// PUT valid entries.
	body := `[{"key":"FOO","value":"bar"},{"key":"API_KEY","value":"${secret:TOKEN}"}]`
	resp := doRequest(t, http.MethodPut, url, "application/json", body)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// GET returns the stored entries, key-sorted, references verbatim.
	resp = get(t, url)
	assertStatus(t, resp, http.StatusOK)
	var got []struct{ Key, Value string }
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode env list: %v", err)
	}
	_ = resp.Body.Close()
	if len(got) != 2 || got[0].Key != "API_KEY" || got[0].Value != "${secret:TOKEN}" || got[1].Key != "FOO" {
		t.Errorf("env round-trip mismatch: %#v", got)
	}

	// Invalid key → 400, env unchanged.
	resp = doRequest(t, http.MethodPut, url, "application/json", `[{"key":"1BAD","value":"x"}]`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()

	// Unknown agent → 404.
	resp = doRequest(t, http.MethodPut, ts.URL+"/api/agents/no-such-agent/env", "application/json", `[]`)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

// TestAgentGet_EnvReturnsReferences verifies the agent DTO returns secret
// references verbatim — never resolved values.
func TestAgentGet_EnvReturnsReferences(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".mycel")

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	if err := mgr.RegisterStopped(&agent.Agent{
		Name:      "env-dto",
		Role:      agent.Role("engineer"),
		Workspace: dir,
		Env: map[string]string{
			"FOO":     "bar",
			"API_KEY": "${secret:MY_TOKEN}",
		},
	}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/env-dto")
	assertStatus(t, resp, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()

	var dto struct {
		Env map[string]string `json:"env"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		t.Fatalf("decode dto: %v", err)
	}
	if dto.Env["FOO"] != "bar" {
		t.Errorf("plain env value missing: %#v", dto.Env)
	}
	if dto.Env["API_KEY"] != "${secret:MY_TOKEN}" {
		t.Errorf("secret reference must be returned verbatim, got %q", dto.Env["API_KEY"])
	}
}
