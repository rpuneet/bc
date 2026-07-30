package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/provider"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/server"
	"github.com/rpuneet/mycel/server/ws"
)

// --- helpers for building test servers with real services ---

// sandboxBCHome points MYCEL_HOME (and HOME) at a per-test tempdir so
// global state (prefs.json, mycel.db, agents/) lands in a disposable
// sandbox instead of the caller's real ~/.mycel.
func sandboxBCHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MYCEL_HOME", filepath.Join(home, ".mycel"))
}

func setupHome(t *testing.T) string {
	t.Helper()
	sandboxBCHome(t)
	dir := t.TempDir()
	// home.Open requires a non-empty root to be a git repo
	if out, err := exec.CommandContext(context.Background(), "git", "init", dir).CombinedOutput(); err != nil { //nolint:gosec // dir is a t.TempDir(), not user input
		t.Fatalf("git init: %v\n%s", err, out)
	}
	wks, err := home.Open(dir)
	if err != nil {
		t.Fatalf("open home: %v", err)
	}
	_ = wks
	return dir
}

// --- source-direct cost helpers ---

// memBudgets is a tiny in-memory cost.BudgetStore for handler tests.
type memBudgets struct {
	m map[string]cost.BudgetConfig
}

func newMemBudgets() *memBudgets { return &memBudgets{m: map[string]cost.BudgetConfig{}} }

func (b *memBudgets) All() (map[string]cost.BudgetConfig, error) {
	out := make(map[string]cost.BudgetConfig, len(b.m))
	for k, v := range b.m {
		out[k] = v
	}
	return out, nil
}

func (b *memBudgets) Set(scope string, cfg cost.BudgetConfig) error {
	b.m[scope] = cfg
	return nil
}

func (b *memBudgets) Delete(scope string) error {
	if _, ok := b.m[scope]; !ok {
		return fmt.Errorf("budget not found for scope %q", scope)
	}
	delete(b.m, scope)
	return nil
}

// newCostService builds a source-direct cost.Service over throwaway
// source dirs (home + agents) backed by an in-memory budget store.
// Returns the service plus the home dir so tests can drop Claude Code
// JSONL fixtures under <home>/.claude/projects/.
func newCostServiceAt(t *testing.T) (*cost.Service, string) {
	t.Helper()
	home := t.TempDir()
	svc := cost.NewService(provider.DefaultRegistry, cost.Options{
		Home:      home,
		AgentsDir: filepath.Join(home, "agents"),
	}, newMemBudgets())
	return svc, home
}

func newCostService(t *testing.T) *cost.Service {
	t.Helper()
	svc, _ := newCostServiceAt(t)
	return svc
}

// claudeUsageLine fabricates one Claude Code JSONL transcript line with
// token usage the claude provider's CostReader parses and prices.
func claudeUsageLine(session, ts, cwd, model string, in, out int64) string {
	return fmt.Sprintf(`{"type":"assistant","sessionId":%q,"timestamp":%q,"cwd":%q,"message":{"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		session, ts, cwd, model, in, out)
}

// writeClaudeSession writes a JSONL session transcript at path.
func writeClaudeSession(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// setupHomeWithDB sets up a home whose database is opened
// lazily via the db registry (see openWSDB).
func setupHomeWithDB(t *testing.T) string {
	t.Helper()
	return setupHome(t)
}

// openWSDB returns the database handle + driver for the
// given repo root, mirroring how BuildServices resolves it.
func openWSDB(t *testing.T, dir string) (*db.DB, string) {
	t.Helper()
	d, driver, err := db.Global(nil)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.CloseGlobal() })
	return d, driver
}

// openWSSQLite returns just the SQLite handle for stores that take a
// bare *db.DB (events).
func openWSSQLite(t *testing.T, dir string) *db.DB {
	t.Helper()
	d, _ := openWSDB(t, dir)
	return d
}

func buildTestServerWithServices(t *testing.T, svc server.Services) *httptest.Server {
	t.Helper()
	hub := ws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	cfg := server.Config{Addr: "127.0.0.1:0", CORS: true, CORSOrigin: "*"}
	srv := server.New(cfg, svc, hub, nil)
	return httptest.NewServer(srv.Handler())
}

func readJSONArray(t *testing.T, resp *http.Response) []any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var arr []any
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		t.Fatalf("decode json array: %v", err)
	}
	return arr
}

// --- Cost handler tests ---

func TestCostHandler_Summary(t *testing.T) {
	svc, home := newCostServiceAt(t)
	writeClaudeSession(t,
		filepath.Join(home, ".claude", "projects", "p1", "11111111-aaaa-1111-1111-111111111111.jsonl"),
		claudeUsageLine("s1", "2026-07-30T10:00:00Z", "/repos/proj", "claude-sonnet-4-20250514", 100, 50),
	)

	ts := buildTestServerWithServices(t, server.Services{Costs: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/costs")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	// claude-sonnet-4: $3/M input + $15/M output.
	wantUSD := 100*3.0/1e6 + 50*15.0/1e6
	gotUSD, _ := body["total_cost_usd"].(float64)
	if diff := gotUSD - wantUSD; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("total_cost_usd = %v, want %v", gotUSD, wantUSD)
	}
	if body["record_count"] != float64(1) {
		t.Fatalf("record_count = %v, want 1", body["record_count"])
	}
}

func TestCostHandler_SummaryMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_ByResource(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	tests := []struct {
		name string
		path string
		want int
	}{
		{"agents", "/api/costs/agents", http.StatusOK},
		{"teams", "/api/costs/teams", http.StatusOK},
		{"models", "/api/costs/models", http.StatusOK},
		{"daily", "/api/costs/daily", http.StatusOK},
		{"daily with days", "/api/costs/daily?days=7", http.StatusOK},
		{"project", "/api/costs/project", http.StatusOK},
		{"project with params", "/api/costs/project?lookback_days=7&project_days=14", http.StatusOK},
		{"budgets list", "/api/costs/budgets", http.StatusOK},
		{"agent detail missing name", "/api/costs/agent", http.StatusBadRequest},
		{"unknown resource", "/api/costs/unknown", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := get(t, ts.URL+tt.path)
			assertStatus(t, resp, tt.want)
			_ = resp.Body.Close()
		})
	}
}

func TestCostHandler_AgentDetail(t *testing.T) {
	svc, home := newCostServiceAt(t)
	// Agent-attributed source: <AgentsDir>/<name>/session/claude/projects.
	writeClaudeSession(t,
		filepath.Join(home, "agents", "test-agent", "session", "claude", "projects", "p", "22222222-aaaa-2222-2222-222222222222.jsonl"),
		claudeUsageLine("s-agent", "2026-07-30T09:00:00Z", "/workspace", "claude-sonnet-4-20250514", 1000, 500),
	)

	ts := buildTestServerWithServices(t, server.Services{Costs: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/costs/agent/test-agent")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	summary, ok := body["summary"].(map[string]any)
	if !ok {
		t.Fatal("expected summary field in agent detail response")
	}
	if summary["agent_id"] != "test-agent" {
		t.Fatalf("summary agent_id = %v, want test-agent", summary["agent_id"])
	}
	wantUSD := 1000*3.0/1e6 + 500*15.0/1e6
	gotUSD, _ := summary["total_cost_usd"].(float64)
	if diff := gotUSD - wantUSD; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("summary total_cost_usd = %v, want %v", gotUSD, wantUSD)
	}
	if _, ok := body["daily"]; !ok {
		t.Fatal("expected daily field in agent detail response")
	}
}

func TestCostHandler_Budgets_Create(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	// Create budget
	resp := post(t, ts.URL+"/api/costs/budgets", "application/json",
		`{"scope":"workspace","period":"monthly","limit_usd":100.0}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if body["scope"] != "workspace" {
		t.Fatalf("expected scope workspace, got %v", body["scope"])
	}

	// Check budget by scope
	resp = get(t, ts.URL+"/api/costs/budgets/workspace")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Delete budget
	resp = doRequest(t, http.MethodDelete, ts.URL+"/api/costs/budgets/workspace", "", "")
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
}

func TestCostHandler_Budgets_Validation(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing scope", `{"period":"monthly","limit_usd":100}`, http.StatusBadRequest},
		{"zero limit", `{"scope":"ws","period":"monthly","limit_usd":0}`, http.StatusBadRequest},
		{"invalid period", `{"scope":"ws","period":"yearly","limit_usd":100}`, http.StatusBadRequest},
		{"invalid JSON", `{invalid}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := post(t, ts.URL+"/api/costs/budgets", "application/json", tt.body)
			assertStatus(t, resp, tt.want)
			_ = resp.Body.Close()
		})
	}
}

func TestCostHandler_Budgets_DeleteNoScope(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := doRequest(t, http.MethodDelete, ts.URL+"/api/costs/budgets", "", "")
	// budgets route with empty scope and DELETE
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestCostHandler_Budgets_MethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/costs/budgets", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// TestCostHandler_Sync verifies POST /api/costs/sync re-scans the
// provider session files and reports the merged entry count.
func TestCostHandler_Sync(t *testing.T) {
	svc, home := newCostServiceAt(t)

	ts := buildTestServerWithServices(t, server.Services{Costs: svc})
	defer ts.Close()

	// No sources yet — sync succeeds with zero entries.
	resp := post(t, ts.URL+"/api/costs/sync", "application/json", `{}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if body["imported"] != float64(0) {
		t.Fatalf("imported = %v, want 0", body["imported"])
	}

	// Drop a transcript on disk; sync must pick it up immediately
	// (bypassing the 60s cache).
	writeClaudeSession(t,
		filepath.Join(home, ".claude", "projects", "p", "33333333-aaaa-3333-3333-333333333333.jsonl"),
		claudeUsageLine("s-sync", "2026-07-30T11:00:00Z", "/repos/sync", "claude-sonnet-4-20250514", 10, 5),
	)
	resp = post(t, ts.URL+"/api/costs/sync", "application/json", `{}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["imported"] != float64(1) {
		t.Fatalf("imported = %v, want 1 after writing a transcript", body["imported"])
	}
}

func TestCostHandler_SyncMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/costs/sync")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_AgentsMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs/agents", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_BudgetNotFound(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/costs/budgets/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

// --- Secret handler tests ---

func TestSecretHandler_ListEmpty(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/secrets")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty secrets, got %d", len(arr))
	}
}

func TestSecretHandler_CRUD(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	// Create
	resp := post(t, ts.URL+"/api/secrets", "application/json",
		`{"name":"MY_KEY","value":"secret123","description":"test key"}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusCreated)
	body := readJSON(t, resp)
	if body["name"] != "MY_KEY" {
		t.Fatalf("expected name MY_KEY, got %v", body["name"])
	}

	// Get metadata (should not contain value)
	resp = get(t, ts.URL+"/api/secrets/MY_KEY")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["name"] != "MY_KEY" {
		t.Fatalf("expected name MY_KEY, got %v", body["name"])
	}

	// Update
	resp = doRequest(t, http.MethodPut, ts.URL+"/api/secrets/MY_KEY", "application/json",
		`{"value":"updated","description":"updated key"}`)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Delete
	resp = doRequest(t, http.MethodDelete, ts.URL+"/api/secrets/MY_KEY", "", "")
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()

	// Verify deleted
	resp = get(t, ts.URL+"/api/secrets/MY_KEY")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestSecretHandler_GetNotFound(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/secrets/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestSecretHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/secrets", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestSecretHandler_EmptyName(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/secrets/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestSecretHandler_CreateInvalidBody(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/secrets", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestSecretHandler_UpdateInvalidBody(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPut, ts.URL+"/api/secrets/test", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestSecretHandler_ByNameMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	store, err := secret.NewStore(dir, "test-passphrase")
	if err != nil {
		t.Fatalf("create secret store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Secrets: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/secrets/test", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// --- MCP handler tests ---

func TestMCPHandler_ListEmpty(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/mcp")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty mcp servers, got %d", len(arr))
	}
}

func TestMCPHandler_CRUD(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	// Create (transport must be "stdio" or "sse")
	resp := post(t, ts.URL+"/api/mcp", "application/json",
		`{"name":"test-server","transport":"stdio","command":"npx test-server","enabled":true}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusCreated)
	body := readJSON(t, resp)
	if body["name"] != "test-server" {
		t.Fatalf("expected name test-server, got %v", body["name"])
	}

	// Get
	resp = get(t, ts.URL+"/api/mcp/test-server")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["name"] != "test-server" {
		t.Fatalf("expected name test-server, got %v", body["name"])
	}

	// Enable/Disable
	resp = post(t, ts.URL+"/api/mcp/test-server/disable", "application/json", ``)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", body["enabled"])
	}

	resp = post(t, ts.URL+"/api/mcp/test-server/enable", "application/json", ``)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", body["enabled"])
	}

	// Delete
	resp = doRequest(t, http.MethodDelete, ts.URL+"/api/mcp/test-server", "", "")
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
}

func TestMCPHandler_GetNotFound(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/mcp/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestMCPHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/mcp", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestMCPHandler_EmptyName(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/mcp/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestMCPHandler_UnknownSub(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/mcp", "application/json",
		`{"name":"sub-srv","transport":"stdio","command":"echo test"}`)
	_ = resp.Body.Close()

	resp = get(t, ts.URL+"/api/mcp/sub-srv/unknown")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestMCPHandler_CreateInvalidBody(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/mcp", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestMCPHandler_ServerMethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/mcp", "application/json",
		`{"name":"mna-srv","transport":"stdio","command":"echo test"}`)
	_ = resp.Body.Close()

	// PUT is not a supported method on /api/mcp/{name}. PATCH is allowed
	// (env editor) — covered by dedicated tests in mcp_test.go.
	resp = doRequest(t, http.MethodPut, ts.URL+"/api/mcp/mna-srv", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestMCPHandler_EnableMethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, err := mcp.NewStore(openWSDB(t, dir))
	if err != nil {
		t.Fatalf("create mcp store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{MCP: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/mcp/test/enable")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// --- Tool handler tests ---

func TestToolHandler_List(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/tools")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	// Tool store may pre-populate with default tools, so just check array format
	_ = readJSONArray(t, resp)
}

func TestToolHandler_GetNotFound(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/tools/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestToolHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	// POST is now valid (creates tools), test PATCH instead
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, ts.URL+"/api/tools", nil)
	resp, _ := http.DefaultClient.Do(req)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestToolHandler_EmptyName(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/tools/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestToolHandler_UnknownSub(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/tools/test/unknown")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestToolHandler_EnableDisableMethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/tools/test/enable")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestToolHandler_PutInvalidBody(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPut, ts.URL+"/api/tools/test", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestToolHandler_ToolMethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/tools/test", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// --- Event handler tests --- (team handler tests removed)

// --- Event handler tests ---

func TestEventHandler_ListEmpty(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/logs")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty events, got %d", len(arr))
	}
}

func TestEventHandler_AppendAndList(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	// Append event
	resp := post(t, ts.URL+"/api/logs", "application/json",
		`{"agent":"alice","type":"started","message":"agent started"}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}

	// List events with tail
	resp = get(t, ts.URL+"/api/logs?tail=10")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 1 {
		t.Fatalf("expected 1 event, got %d", len(arr))
	}
}

func TestEventHandler_ByAgent(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	// Append events for different agents
	resp := post(t, ts.URL+"/api/logs", "application/json",
		`{"agent":"alice","type":"started","message":"alice started"}`)
	_ = resp.Body.Close()

	resp = post(t, ts.URL+"/api/logs", "application/json",
		`{"agent":"bob","type":"started","message":"bob started"}`)
	_ = resp.Body.Close()

	// Filter by agent
	resp = get(t, ts.URL+"/api/logs/alice")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 1 {
		t.Fatalf("expected 1 event for alice, got %d", len(arr))
	}
}

func TestEventHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	resp := doRequest(t, http.MethodPut, ts.URL+"/api/logs", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestEventHandler_AppendInvalidBody(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/logs", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestEventHandler_EmptyAgentName(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/logs/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestEventHandler_ByAgentMethodNotAllowed(t *testing.T) {
	dir := setupHomeWithDB(t)
	store, _ := events.NewSQLiteLog(openWSSQLite(t, dir))
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{EventLog: store})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/logs/alice", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// --- Doctor handler tests ---

func TestDoctorHandler_RunAll(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/doctor")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if _, ok := body["categories"]; !ok {
		t.Fatal("expected categories field in doctor response")
	}
}

func TestDoctorHandler_ByCategory(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/doctor/home")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestDoctorHandler_UnknownCategory(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/doctor/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestDoctorHandler_EmptyCategory(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/doctor/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestDoctorHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/doctor", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestDoctorHandler_ByCategoryMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/doctor/home", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// --- Roles handler tests ---

func TestRolesHandler_ListRoles(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/roles")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestRolesHandler_CreateAndGetRole(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	// Create
	resp := post(t, ts.URL+"/api/roles", "application/json",
		`{"name":"test-role","description":"a test role","prompt":"Be helpful"}`)
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	// Get
	resp = get(t, ts.URL+"/api/roles/test-role")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Update
	resp = doRequest(t, http.MethodPut, ts.URL+"/api/roles/test-role", "application/json",
		`{"description":"updated role","prompt":"Be very helpful"}`)
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()

	// Delete
	resp = doRequest(t, http.MethodDelete, ts.URL+"/api/roles/test-role", "", "")
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
}

func TestRolesHandler_CreateDuplicate(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/roles", "application/json",
		`{"name":"dup-role","prompt":"test"}`)
	assertStatus(t, resp, http.StatusCreated)
	_ = resp.Body.Close()

	resp = post(t, ts.URL+"/api/roles", "application/json",
		`{"name":"dup-role","prompt":"test"}`)
	assertStatus(t, resp, http.StatusConflict)
	_ = resp.Body.Close()
}

func TestRolesHandler_CreateMissingName(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/roles", "application/json",
		`{"prompt":"test"}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestRolesHandler_GetNotFound(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/roles/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestRolesHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/roles", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestRolesHandler_EmptyName(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/roles/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestRolesHandler_CreateInvalidBody(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/roles", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestRolesHandler_PutInvalidBody(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	// Create the role first
	resp := post(t, ts.URL+"/api/roles", "application/json",
		`{"name":"put-inv-role","prompt":"test"}`)
	_ = resp.Body.Close()

	resp = doRequest(t, http.MethodPut, ts.URL+"/api/roles/put-inv-role", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestRolesHandler_ByNameMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := doRequest(t, http.MethodPatch, ts.URL+"/api/roles/test", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

// --- Settings handler tests ---

// --- Stats handler tests ---

func TestStatsHandler_System(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/stats/system")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)

	expectedFields := []string{"hostname", "os", "arch", "cpus", "go_version", "uptime_seconds", "goroutines"}
	for _, field := range expectedFields {
		if _, ok := body[field]; !ok {
			t.Fatalf("missing field %q in system stats", field)
		}
	}
}

func TestStatsHandler_SystemMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/stats/system", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestStatsHandler_SummaryEmpty(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/stats/summary")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)

	expectedFields := []string{
		"agents_total", "agents_running", "agents_stopped",
		"channels_total", "messages_total", "total_cost_usd",
		"roles_total", "tools_total", "uptime_seconds",
	}
	for _, field := range expectedFields {
		if _, ok := body[field]; !ok {
			t.Fatalf("missing field %q in summary stats", field)
		}
	}
}

func TestStatsHandler_SummaryMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/stats/summary", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestStatsHandler_SummaryWithServices(t *testing.T) {
	dir := setupHomeWithDB(t)

	// Set up costs (source-direct service over empty sources)
	costStore := newCostService(t)

	// Set up tools
	toolStore := tool.NewStore(openWSDB(t, dir))
	if err := toolStore.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = toolStore.Close() })

	// Set up home
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{
		Costs: costStore,
		Tools: toolStore,
		Home:  wks,
	})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/stats/summary")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	// All counts should be 0 for a fresh home
	if body["agents_total"] != float64(0) {
		t.Fatalf("expected agents_total=0, got %v", body["agents_total"])
	}
}

func TestStatsHandler_SystemWithRepo(t *testing.T) {
	dir := setupHome(t)
	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	ts := buildTestServerWithServices(t, server.Services{Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/stats/system")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if _, ok := body["hostname"]; !ok {
		t.Fatal("missing hostname in system stats")
	}
}

// --- Agent handler tests (limited, since AgentService needs real tmux) ---

func TestAgentHandler_ListEmpty(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty agents, got %d", len(arr))
	}
}

func TestAgentHandler_MethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := doRequest(t, http.MethodPut, ts.URL+"/api/agents", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_CreateInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_GetNotFound(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/nonexistent")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_EmptyName(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/")
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_UnknownAction(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/test/unknown-action")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_GenerateName(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/generate-name")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if _, ok := body["name"]; !ok {
		t.Fatal("expected name field in generate-name response")
	}
}

func TestAgentHandler_GenerateNameMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/generate-name", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_BroadcastMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/broadcast")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_BroadcastInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/broadcast", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_SendRoleMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/send-role")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_SendRoleInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/send-role", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_SendPatternMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/send-pattern")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_SendPatternInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/send-pattern", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_StopAllMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/stop-all")
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_StopAll(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/stop-all", "application/json", ``)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if _, ok := body["stopped"]; !ok {
		t.Fatal("expected stopped field")
	}
}

func TestAgentHandler_Broadcast(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/broadcast", "application/json", `{"message":"hello all"}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if _, ok := body["sent"]; !ok {
		t.Fatal("expected sent field")
	}
}

func TestAgentHandler_SendOnNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/nonexist/send", "application/json", `{"message":"hello"}`)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_SendInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/test/send", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_HealthMethodNotAllowed(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/health", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestAgentHandler_HealthEmpty(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/health")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty health, got %d", len(arr))
	}
}

func TestAgentHandler_HealthWithTimeout(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/health?timeout=30s")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestAgentHandler_StartNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/nonexist/start", "application/json", ``)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_StopNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/nonexist/stop", "application/json", ``)
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_DeleteNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := doRequest(t, http.MethodDelete, ts.URL+"/api/agents/nonexist", "", "")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_PeekNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/nonexist/peek")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_SessionsNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/nonexist/sessions")
	assertStatus(t, resp, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestAgentHandler_RenameInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/test/rename", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_HookInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/test/hook", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_HookUnknownEvent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/test/hook", "application/json", `{"event":"unknown_event_xyz"}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_ReportInvalidBody(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/test/report", "application/json", `{invalid}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

func TestAgentHandler_ReportInvalidState(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/agents/test/report", "application/json", `{"state":"invalid_state_xyz"}`)
	assertStatus(t, resp, http.StatusBadRequest)
	_ = resp.Body.Close()
}

// --- Pagination tests ---

// --- Settings PUT with sections ---

// --- Settings PUT covering all section branches ---

// --- Agent handler with cost enrichment ---

func TestAgentHandler_ListWithCosts(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	costStore := newCostService(t)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc, Costs: costStore})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestAgentHandler_ListWithRepo(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	wks, err := home.Load(dir)
	if err != nil {
		t.Fatalf("load home: %v", err)
	}

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc, Home: wks})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents")
	assertStatus(t, resp, http.StatusOK)
	_ = resp.Body.Close()
}

func TestAgentHandler_ListPagination(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents?limit=10&offset=100")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty result with large offset, got %d", len(arr))
	}
}

// --- Tool CRUD via API ---

func TestToolHandler_CRUD(t *testing.T) {
	dir := setupHomeWithDB(t)
	store := tool.NewStore(openWSDB(t, dir))
	if err := store.Open(); err != nil {
		t.Fatalf("open tool store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := buildTestServerWithServices(t, server.Services{Tools: store})
	defer ts.Close()

	// PUT to update/create a tool
	resp := doRequest(t, http.MethodPut, ts.URL+"/api/tools/claude", "application/json",
		`{"name":"claude","command":"claude --skip-permissions","enabled":true}`)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body := readJSON(t, resp)
	if body["name"] != "claude" {
		t.Fatalf("expected name claude, got %v", body["name"])
	}

	// GET tool
	resp = get(t, ts.URL+"/api/tools/claude")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["name"] != "claude" {
		t.Fatalf("expected name claude, got %v", body["name"])
	}

	// Enable
	resp = post(t, ts.URL+"/api/tools/claude/enable", "application/json", ``)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["enabled"] != true {
		t.Fatalf("expected enabled=true, got %v", body["enabled"])
	}

	// Disable
	resp = post(t, ts.URL+"/api/tools/claude/disable", "application/json", ``)
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	body = readJSON(t, resp)
	if body["enabled"] != false {
		t.Fatalf("expected enabled=false, got %v", body["enabled"])
	}

	// Delete
	resp = doRequest(t, http.MethodDelete, ts.URL+"/api/tools/claude", "", "")
	assertStatus(t, resp, http.StatusNoContent)
	_ = resp.Body.Close()
}

// --- Cost handler: budget valid periods ---

func TestCostHandler_Budgets_AllPeriods(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	for _, period := range []string{"daily", "weekly", "monthly"} {
		t.Run(period, func(t *testing.T) {
			resp := post(t, ts.URL+"/api/costs/budgets", "application/json",
				`{"scope":"test-`+period+`","period":"`+period+`","limit_usd":50.0}`)
			assertStatus(t, resp, http.StatusOK)
			_ = resp.Body.Close()
		})
	}
}

// --- Agent handler: stats endpoint ---

func TestAgentHandler_StatsNonexistent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/nonexist/stats")
	// stats returns empty array or error for nonexistent agent
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAgentHandler_StatsWithLimit(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/test/stats?limit=5")
	// OK to get error or empty result for nonexistent agent
	_ = resp.Body.Close()
}

// --- Agent health with agent filter ---

func TestAgentHandler_HealthWithFilter(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	mgr := agent.NewManager(stateDir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	resp := get(t, ts.URL+"/api/agents/health?agent=nonexist")
	defer func() { _ = resp.Body.Close() }()
	assertStatus(t, resp, http.StatusOK)
	arr := readJSONArray(t, resp)
	if len(arr) != 0 {
		t.Fatalf("expected empty health with filter, got %d", len(arr))
	}
}

// --- Additional coverage for CORS helper ---

func TestCORSMiddlewareDefault(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	t.Cleanup(hub.Stop)

	// Build server without CORSOrigin set to exercise CORS default
	cfg := server.Config{Addr: "127.0.0.1:0", CORS: true}
	srv := server.New(cfg, server.Services{}, hub, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := get(t, ts.URL+"/health")
	assertStatus(t, resp, http.StatusOK)
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("want CORS origin *, got %q", got)
	}
	_ = resp.Body.Close()
}

// --- Agent handler: create agent (exercises success paths) ---

func TestAgentHandler_CreateAgent(t *testing.T) {
	dir := setupHome(t)
	stateDir := filepath.Join(dir, ".bc")
	_ = os.MkdirAll(filepath.Join(stateDir, "agents"), 0750)

	// Use NewManagerWithRepo so worktreeMgr is initialized and doesn't panic.
	mgr := agent.NewManagerWithRepo(stateDir, dir)
	svc := agent.NewAgentService(mgr, nil, nil)

	ts := buildTestServerWithServices(t, server.Services{Agents: svc})
	defer ts.Close()

	// Create agent - may fail without tmux/real runtime or if role doesn't exist,
	// but must not panic (500). Accept 201 (created) or 400 (validation failure).
	resp := post(t, ts.URL+"/api/agents", "application/json",
		`{"name":"test-agent","role":"engineer","tool":"claude"}`)
	// Accept 201 (created) or 400 (if runtime not available / role missing)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 201 or 400, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// --- Settings PUT with invalid section content triggers specific validation ---

// --- Cost handler: by-resource method not allowed on various sub-resources ---

func TestCostHandler_DailyMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs/daily", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_ProjectMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs/project", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_AgentDetailMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs/agent/test", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_TeamsMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs/teams", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}

func TestCostHandler_ModelsMethodNotAllowed(t *testing.T) {
	ts := buildTestServerWithServices(t, server.Services{Costs: newCostService(t)})
	defer ts.Close()

	resp := post(t, ts.URL+"/api/costs/models", "application/json", `{}`)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	_ = resp.Body.Close()
}
