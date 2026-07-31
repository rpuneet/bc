package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	costpkg "github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/home"
	mcppkg "github.com/rpuneet/mycel/pkg/mcp"
	secretpkg "github.com/rpuneet/mycel/pkg/secret"
	templatepkg "github.com/rpuneet/mycel/pkg/template"
)

// TestM8WiringTemplatesGlobalStore verifies that BuildServices hands
// back the single user-global template store: a Globals-supplied store
// is used as-is, and without one the bundle falls back to
// <MycelHome>/templates.
func TestM8WiringTemplatesGlobalStore(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")

	// Seed one user-global template directly on disk.
	globalDir := filepath.Join(bcHome, "templates")
	if err := os.MkdirAll(globalDir, 0o750); err != nil {
		t.Fatal(err)
	}
	globalTmpl := templatepkg.Template{Name: "user-t", Description: "global"}
	if err := templatepkg.NewStore(globalDir).Create(globalTmpl, "global prompt", templatepkg.ScopeGlobal); err != nil {
		t.Fatalf("seed global template: %v", err)
	}

	// Build a Globals with the user-global template store wired.
	globals := &Globals{
		Templates: templatepkg.NewStore(globalDir),
	}

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), globals, wsDir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer svc.Close() //nolint:errcheck

	if svc.Templates != globals.Templates {
		t.Fatal("svc.Templates is not the Globals-supplied global store")
	}

	// List should see the global template.
	list, err := svc.Templates.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	foundGlobal := false
	for _, tt := range list {
		if tt.Name == "user-t" && tt.Scope == templatepkg.ScopeGlobal {
			foundGlobal = true
		}
	}
	if !foundGlobal {
		t.Errorf("global template not visible via svc.Templates.List(): %+v", list)
	}

	// A bundle built WITHOUT Globals.Templates must fall back to the
	// same <MycelHome>/templates directory (still ONE global store).
	svc2, err := BuildServices(context.Background(), &Globals{}, wsDir)
	if err != nil {
		t.Fatalf("build without globals templates: %v", err)
	}
	defer svc2.Close() //nolint:errcheck
	got, _, err := svc2.Templates.Get("user-t")
	if err != nil {
		t.Fatalf("fallback store Get: %v", err)
	}
	if got.Description != "global" {
		t.Errorf("fallback store description = %q, want global", got.Description)
	}
}

// TestM8WiringSecretsPreferGlobalVault confirms svc.Secrets points at
// the user-global vault supplied via Globals.SecretsVault.
func TestM8WiringSecretsPreferGlobalVault(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")

	vault, err := secretpkg.OpenVaultFile(filepath.Join(bcHome, "secrets.vault"), "unit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close() //nolint:errcheck
	if setErr := vault.Set("API_KEY", "sk-global", ""); setErr != nil {
		t.Fatal(setErr)
	}

	globals := &Globals{SecretsVault: vault}
	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), globals, wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close() //nolint:errcheck

	if svc.Secrets == nil {
		t.Fatal("svc.Secrets nil despite Globals.SecretsVault wired")
	}
	val, err := svc.Secrets.GetValue("API_KEY")
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}
	if val != "sk-global" {
		t.Errorf("got %q, want sk-global", val)
	}
}

// TestM8WiringMCPGlobalView exposes the user-global MCP registry
// through the layered view helper.
func TestM8WiringMCPGlobalView(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")

	mcpPath := filepath.Join(bcHome, "mcps.json")
	gs := mcppkg.NewGlobalStore(mcpPath)
	if err := gs.Add(&mcppkg.ServerConfig{Name: "trusted-gh", Transport: mcppkg.TransportStdio, Command: "npx"}); err != nil {
		t.Fatal(err)
	}

	globals := &Globals{MCPGlobal: gs}
	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), globals, wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close() //nolint:errcheck

	view := svc.MCPLayeredView()
	if view == nil {
		t.Fatal("MCPLayeredView returned nil despite Globals.MCPGlobal set")
	}
	list, err := view.List()
	if err != nil {
		t.Fatal(err)
	}
	foundTrusted := false
	for _, s := range list {
		if s.Name == "trusted-gh" {
			foundTrusted = true
		}
	}
	if !foundTrusted {
		t.Errorf("trusted-gh not visible via layered view: %+v", list)
	}
}

// TestM8WiringCostsSourceDirect confirms BuildServices wires a
// source-direct cost.Service that (a) reads agent session transcripts
// from <MycelHome>/agents/<name>/session/claude/projects and (b)
// persists budget thresholds into the global prefs.json.
func TestM8WiringCostsSourceDirect(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	// Point HOME at an empty dir so the host ~/.claude of the developer
	// running the tests never leaks into the scan.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MYCEL_SECRET_PASSPHRASE", "unit-test")

	// Fabricate one agent-attributed Claude Code transcript.
	repo := filepath.Join(t.TempDir(), "m8-repo")
	sessionFile := filepath.Join(bcHome, "agents", "cost-agent", "session", "claude", "projects", "p", "aaaaaaaa-1111-2222-3333-444444444444.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0o750); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"assistant","sessionId":"s-m8","timestamp":"2026-07-30T10:00:00Z","cwd":%q,"message":{"model":"claude-sonnet-4-20250514","usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`, repo)
	if err := os.WriteFile(sessionFile, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), &Globals{}, wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close() //nolint:errcheck

	if svc.Costs == nil {
		t.Fatal("svc.Costs nil")
	}
	ctx := context.Background()

	// Repo rollup attributes the entry to the session cwd.
	byRepo, err := svc.Costs.SumByRepo(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	wantUSD := 1000*3.0/1e6 + 500*15.0/1e6 // claude-sonnet-4 pricing
	got, ok := byRepo[repo]
	if !ok {
		t.Fatalf("SumByRepo missing seeded repo %q: %+v", repo, byRepo)
	}
	if diff := got - wantUSD; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("SumByRepo[%q] = %v, want %v", repo, got, wantUSD)
	}

	// Agent attribution comes from the agents-dir entity name.
	agentSum, err := svc.Costs.AgentSummary(ctx, "cost-agent")
	if err != nil {
		t.Fatal(err)
	}
	if agentSum.RecordCount != 1 || agentSum.InputTokens != 1000 || agentSum.OutputTokens != 500 {
		t.Errorf("AgentSummary = %+v, want 1 record with 1000/500 tokens", agentSum)
	}

	// Budgets are configuration persisted in ~/.mycel/prefs.json.
	if _, setErr := svc.Costs.SetBudget(ctx, "workspace", costpkg.BudgetPeriodMonthly, 25, 0.8, false); setErr != nil {
		t.Fatalf("SetBudget: %v", setErr)
	}
	budget, err := svc.Costs.GetBudget(ctx, "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if budget == nil || budget.LimitUSD != 25 || budget.Period != costpkg.BudgetPeriodMonthly {
		t.Fatalf("GetBudget = %+v, want monthly $25", budget)
	}

	prefsRaw, err := os.ReadFile(filepath.Join(bcHome, home.PrefsFileName)) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatalf("read prefs.json: %v", err)
	}
	var prefs struct {
		Budgets map[string]costpkg.BudgetConfig `json:"budgets"`
	}
	if err := json.Unmarshal(prefsRaw, &prefs); err != nil {
		t.Fatalf("parse prefs.json: %v", err)
	}
	if cfg, ok := prefs.Budgets["workspace"]; !ok || cfg.LimitUSD != 25 {
		t.Errorf("budget not persisted in prefs.json: %+v", prefs.Budgets)
	}
}
