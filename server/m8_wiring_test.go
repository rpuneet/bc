package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bccost "github.com/rpuneet/mycel/pkg/cost"
	bcmcp "github.com/rpuneet/mycel/pkg/mcp"
	bcsecret "github.com/rpuneet/mycel/pkg/secret"
	bctemplate "github.com/rpuneet/mycel/pkg/template"
)

// TestM8WiringTemplatesGlobalOverride verifies that BuildServices
// hands back a Templates store whose List() sees a user-global template
// unioned with a per-workspace override.
func TestM8WiringTemplatesGlobalOverride(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	// Seed one user-global template directly on disk.
	globalDir := filepath.Join(bcHome, "templates")
	if err := os.MkdirAll(globalDir, 0o750); err != nil {
		t.Fatal(err)
	}
	globalTmpl := bctemplate.Template{Name: "user-t", Description: "global"}
	if err := bctemplate.NewStore(globalDir).Create(globalTmpl, "global prompt", bctemplate.ScopeGlobal); err != nil {
		t.Fatalf("seed global template: %v", err)
	}

	// Build a Globals with the user-global template store wired.
	globals := &Globals{
		Templates: bctemplate.NewStore(globalDir),
	}

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), globals, wsDir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer svc.Close() //nolint:errcheck

	if svc.Templates == nil {
		t.Fatal("svc.Templates nil")
	}

	// List should see the global template.
	list, err := svc.Templates.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	foundGlobal := false
	for _, tt := range list {
		if tt.Name == "user-t" && tt.Scope == bctemplate.ScopeGlobal {
			foundGlobal = true
		}
	}
	if !foundGlobal {
		t.Errorf("global template not visible via svc.Templates.List(): %+v", list)
	}

	// Add a workspace override and confirm it wins.
	if createErr := svc.Templates.Create(
		bctemplate.Template{Name: "user-t", Description: "workspace override"},
		"ws prompt",
		bctemplate.ScopeWorkspace,
	); createErr != nil {
		t.Fatalf("create ws override: %v", createErr)
	}
	got, _, err := svc.Templates.Get("user-t")
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != bctemplate.ScopeWorkspace {
		t.Errorf("scope = %q, want workspace", got.Scope)
	}
	if got.Description != "workspace override" {
		t.Errorf("description = %q (override lost)", got.Description)
	}
}

// TestM8WiringSecretsPreferGlobalVault confirms svc.Secrets points at
// the user-global vault supplied via Globals.SecretsVault.
func TestM8WiringSecretsPreferGlobalVault(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	vault, err := bcsecret.OpenVaultFile(filepath.Join(bcHome, "secrets.vault"), "unit-test")
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
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	mcpPath := filepath.Join(bcHome, "mcps.json")
	gs := bcmcp.NewGlobalStore(mcpPath)
	if err := gs.Add(&bcmcp.ServerConfig{Name: "trusted-gh", Transport: bcmcp.TransportStdio, Command: "npx"}); err != nil {
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

// TestM8WiringCostsGlobalLedger confirms svc.Costs points at the
// user-global ledger when Globals.CostsGlobal is supplied.
func TestM8WiringCostsGlobalLedger(t *testing.T) {
	bcHome := t.TempDir()
	t.Setenv("MYCEL_HOME", bcHome)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	costs, err := bccost.OpenGlobalStore(filepath.Join(bcHome, "costs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer costs.Close() //nolint:errcheck

	globals := &Globals{CostsGlobal: costs}
	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	svc, err := BuildServices(context.Background(), globals, wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close() //nolint:errcheck

	// svc.Costs is the same pointer as the global ledger.
	if svc.Costs != costs {
		t.Errorf("svc.Costs does not reference Globals.CostsGlobal")
	}
	// The importer should have picked up the repo path.
	if svc.CostImporter == nil {
		t.Fatal("CostImporter nil")
	}
	// We can't reach imp.repo directly; indirect verification via
	// inserting a scoped record and rolling up.
	scoped := svc.Costs.ScopedTo(wsDir)
	if _, recErr := scoped.Record(context.Background(), "agent", "", "model", 1, 1, 0.50); recErr != nil {
		t.Fatalf("Record: %v", recErr)
	}
	byRepo, err := svc.Costs.SumByRepo(context.Background(), timeZero())
	if err != nil {
		t.Fatal(err)
	}
	// The importer may sweep in unrelated JSONL files from the developer's
	// host ~/.claude directory — we only assert that the scoped record
	// landed under the repo path, not the exact total (host import
	// amplifies it).
	if _, ok := byRepo[wsDir]; !ok {
		t.Errorf("ledger did not attribute scoped record: %+v", byRepo)
	}
}

func timeZero() interface{ Format(string) string } {
	// A tiny helper so the test can pass an always-before time to
	// SumByRepo without importing "time" at the test call site.
	return zeroTime{}
}

type zeroTime struct{}

func (zeroTime) Format(string) string { return "0000-01-01T00:00:00Z" }
