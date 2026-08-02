package tool

import (
	"context"
	"os/exec"
	"testing"
)

// TestCheckAll_PersistsHealthStatus covers the bug the background
// auto-check is meant to fix (#3423): CheckAll must not just return
// ephemeral results, it must persist health_status + last_checked back to
// the store via UpdateHealth so a subsequent Get/List sees fresh data
// without requiring a client to have called checkAll first.
func TestCheckAll_PersistsHealthStatus(t *testing.T) {
	s := NewStore(setupSharedDB(t), "sqlite")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	ctx := context.Background()

	// A CLI tool pointing at a binary that is guaranteed to be on PATH in
	// any test environment ("sh"), so CheckAll deterministically marks it
	// installed rather than depending on what happens to be on the runner.
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found on PATH; cannot exercise a deterministic installed case")
	}
	installedTool := &Tool{Name: "health-check-installed", Type: ToolTypeCLI, Command: shPath, Enabled: true}
	if addErr := s.Add(ctx, installedTool); addErr != nil {
		t.Fatalf("Add installed: %v", addErr)
	}
	missingTool := &Tool{Name: "health-check-missing", Type: ToolTypeCLI, Command: "definitely-not-a-real-binary-xyz", Enabled: true}
	if addErr := s.Add(ctx, missingTool); addErr != nil {
		t.Fatalf("Add missing: %v", addErr)
	}

	// Before CheckAll, freshly-added tools carry the seed-time default,
	// never a live-verified status.
	before, err := s.Get(ctx, installedTool.Name)
	if err != nil {
		t.Fatalf("Get before: %v", err)
	}
	if before.LastChecked != "" {
		t.Fatalf("expected no LastChecked before CheckAll, got %q", before.LastChecked)
	}

	results, err := s.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("CheckAll returned no results")
	}

	installedAfter, err := s.Get(ctx, installedTool.Name)
	if err != nil {
		t.Fatalf("Get installed after: %v", err)
	}
	if installedAfter.HealthStatus != "installed" {
		t.Errorf("installed tool HealthStatus = %q, want %q", installedAfter.HealthStatus, "installed")
	}
	if installedAfter.LastChecked == "" {
		t.Error("installed tool LastChecked was not persisted by CheckAll")
	}

	missingAfter, err := s.Get(ctx, missingTool.Name)
	if err != nil {
		t.Fatalf("Get missing after: %v", err)
	}
	if missingAfter.HealthStatus != "not_installed" {
		t.Errorf("missing tool HealthStatus = %q, want %q", missingAfter.HealthStatus, "not_installed")
	}
	if missingAfter.LastChecked == "" {
		t.Error("missing tool LastChecked was not persisted by CheckAll")
	}
}

// TestUpdateHealth_UnknownTool covers the not-found path so callers (the
// background loop, the manual force-refresh) get a real error rather than a
// silent no-op when a tool disappears mid-batch.
func TestUpdateHealth_UnknownTool(t *testing.T) {
	s := NewStore(setupSharedDB(t), "sqlite")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	if err := s.UpdateHealth(context.Background(), "nonexistent-tool", "installed", "2024-01-01T00:00:00Z"); err == nil {
		t.Error("expected error updating health for an unknown tool")
	}
}

// TestCheckOne_BlankCommand pins the guard that keeps a misconfigured row from
// panicking. strings.Fields collapses whitespace, so a command of " " is
// non-empty yet has no first field: the MCP branch tested only for "" and
// indexed [0] regardless. Both tool types are covered because the two branches
// have already drifted apart once.
func TestCheckOne_BlankCommand(t *testing.T) {
	blanks := map[string]string{
		"empty":     "",
		"space":     " ",
		"tab":       "\t",
		"multiline": "  \n ",
	}

	for _, typ := range []string{ToolTypeMCP, ToolTypeCLI, ToolTypeProvider} {
		for name, cmd := range blanks {
			t.Run(typ+"/"+name, func(t *testing.T) {
				// Transport is only consulted for MCP; harmless on the others.
				r := checkOne(&Tool{Name: "blank", Type: typ, Transport: "stdio", Command: cmd})
				if r.Status == "ok" || r.Status == "installed" {
					t.Errorf("tool with no runnable command reported %q", r.Status)
				}
				if r.Error == "" {
					t.Error("failing status came with no explanation")
				}
			})
		}
	}
}

// TestCheckOne_MCPStdio covers both live outcomes for a stdio server, so the
// blank-command guard above cannot be satisfied by simply failing everything.
func TestCheckOne_MCPStdio(t *testing.T) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found on PATH; cannot exercise a deterministic present case")
	}

	if r := checkOne(&Tool{Name: "present", Type: ToolTypeMCP, Transport: "stdio", Command: shPath}); r.Status != "ok" {
		t.Errorf("stdio server on PATH = %q (%s), want ok", r.Status, r.Error)
	}
	if r := checkOne(&Tool{Name: "absent", Type: ToolTypeMCP, Transport: "stdio", Command: "definitely-not-a-real-binary-xyz"}); r.Status != "error" {
		t.Errorf("stdio server missing from PATH = %q, want error", r.Status)
	}
}

// TestCheckOne_SSEIsNotProbed records that an SSE server is never contacted
// here, so it keeps the default status rather than being reported unreachable.
// That default reads as healthy in the UI without anything having been
// verified, which is tracked separately in #3475 — this test exists to keep the
// stdio guard from quietly changing SSE handling in the meantime.
func TestCheckOne_SSEIsNotProbed(t *testing.T) {
	r := checkOne(&Tool{Name: "sse", Type: ToolTypeMCP, Transport: "sse", URL: "http://127.0.0.1:1/sse"})
	if r.Status != "ok" {
		t.Errorf("SSE server = %q, want the unprobed default ok", r.Status)
	}
}

// TestCheckAll_SurvivesBlankMCPCommand runs a poisoned row through the batch
// path the daemon's background loop uses. A panic here escaped CheckAll and,
// from a bare goroutine, terminated the process — repeatedly, since the row
// that provoked it survives a restart.
func TestCheckAll_SurvivesBlankMCPCommand(t *testing.T) {
	s := NewStore(setupSharedDB(t), "sqlite")
	if err := s.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close() //nolint:errcheck // test cleanup

	ctx := context.Background()
	if err := s.Add(ctx, &Tool{
		Name: "blank-mcp", Type: ToolTypeMCP, Transport: "stdio", Command: " ", Enabled: true,
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	results, err := s.CheckAll(ctx)
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	found := false
	for _, r := range results {
		if r.Name == "blank-mcp" {
			found = true
			if r.Status != "error" {
				t.Errorf("blank stdio command = %q, want error", r.Status)
			}
		}
	}
	if !found {
		t.Fatal("blank-mcp missing from CheckAll results")
	}

	// The batch must also persist the verdict, not just return it.
	got, err := s.Get(ctx, "blank-mcp")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HealthStatus != "error" {
		t.Errorf("persisted health = %q, want error", got.HealthStatus)
	}
}
