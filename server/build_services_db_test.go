// build_services_db_test.go — issue #3238: per-workspace databases.
//
// BuildWorkspaceServices must resolve each workspace's OWN database from
// the pkg/db registry so stores for workspace B never write into
// workspace A's bc.db, and a workspace-less daemon boot must be able to
// add a repo later and get a fully-online service bundle (lazy open).
package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bcdb "github.com/rpuneet/mycel/pkg/db"
)

// TestBuildWorkspaceServices_NotifyIsolation is the multi-workspace
// bleed regression test at the factory level: two workspaces built in
// the same process must keep notify subscriptions (and everything else
// in bc.db) fully isolated.
func TestBuildWorkspaceServices_NotifyIsolation(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")
	ctx := context.Background()

	wsA, wsB := t.TempDir(), t.TempDir()
	gitInitDir(t, wsA)
	gitInitDir(t, wsB)
	t.Cleanup(func() {
		_ = bcdb.CloseWorkspaceDB(wsA)
		_ = bcdb.CloseWorkspaceDB(wsB)
	})

	svcA, err := BuildWorkspaceServices(ctx, &Globals{}, wsA)
	if err != nil {
		t.Fatalf("build A: %v", err)
	}
	defer svcA.Close() //nolint:errcheck
	svcB, err := BuildWorkspaceServices(ctx, &Globals{}, wsB)
	if err != nil {
		t.Fatalf("build B: %v", err)
	}
	defer svcB.Close() //nolint:errcheck

	if svcA.Notify == nil || svcB.Notify == nil {
		t.Fatalf("notify services must be online (degraded A=%v B=%v)",
			svcA.Degraded, svcB.Degraded)
	}

	if subErr := svcA.Notify.Store().Subscribe(ctx, "#engineering", "agent-a", false); subErr != nil {
		t.Fatalf("subscribe in A: %v", subErr)
	}

	subsA, err := svcA.Notify.Store().Subscribers(ctx, "#engineering")
	if err != nil {
		t.Fatalf("subscribers A: %v", err)
	}
	if len(subsA) != 1 || subsA[0].Agent != "agent-a" {
		t.Fatalf("workspace A subscribers = %+v, want [agent-a]", subsA)
	}

	subsB, err := svcB.Notify.Store().Subscribers(ctx, "#engineering")
	if err != nil {
		t.Fatalf("subscribers B: %v", err)
	}
	if len(subsB) != 0 {
		t.Errorf("workspace A's subscription bled into workspace B: %+v", subsB)
	}

	// Each workspace must have its own database file.
	for _, dir := range []string{wsA, wsB} {
		if _, statErr := os.Stat(filepath.Join(dir, ".bc", "bc.db")); statErr != nil {
			t.Errorf("bc.db missing for %s: %v", dir, statErr)
		}
	}
}

// TestBuildWorkspaceServices_LazyWorkspaceDB simulates the
// workspace-less boot path: no database exists anywhere until a repo is
// added (BuildWorkspaceServices is what POST /api/workspaces invokes),
// at which point the registry opens that workspace's DB lazily and the
// stores come up online rather than degraded.
func TestBuildWorkspaceServices_LazyWorkspaceDB(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")
	ctx := context.Background()

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)
	t.Cleanup(func() { _ = bcdb.CloseWorkspaceDB(wsDir) })

	// Nothing pre-opened: the db file must not exist yet.
	if _, err := os.Stat(filepath.Join(wsDir, ".bc", "bc.db")); err == nil {
		t.Fatal("bc.db must not exist before services are built")
	}

	svc, err := BuildWorkspaceServices(ctx, &Globals{}, wsDir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer svc.Close() //nolint:errcheck

	for name, present := range map[string]bool{
		"notify": svc.Notify != nil,
		"cron":   svc.Cron != nil,
		"mcp":    svc.MCP != nil,
		"tools":  svc.Tools != nil,
		"events": svc.Events != nil,
	} {
		if !present {
			t.Errorf("%s store not online after lazy add (degraded: %v)", name, svc.Degraded)
		}
	}
	if reason, degraded := svc.Degraded["storage"]; degraded {
		t.Errorf("storage degraded on lazy add: %s", reason)
	}
	if _, err := os.Stat(filepath.Join(wsDir, ".bc", "bc.db")); err != nil {
		t.Errorf("bc.db not created lazily: %v", err)
	}
}
