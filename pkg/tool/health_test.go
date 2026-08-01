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
