// build_services_db_test.go — single global database.
//
// BuildServices resolves the ONE global mycel.db: bundles built for two
// repos in the same process share the same database file, and isolation
// comes from data keys (agent name, repo path) rather than from separate
// files.
package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bcagent "github.com/rpuneet/mycel/pkg/agent"
	bcdb "github.com/rpuneet/mycel/pkg/db"
)

// TestBuildServices_SharedGlobalDB asserts the single-database
// semantics: two workspaces in one process share mycel.db (channel
// subscriptions are global), agents are isolated by repo key, and a
// duplicate agent name across repos is rejected.
func TestBuildServices_SharedGlobalDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })
	ctx := context.Background()

	wsA, wsB := t.TempDir(), t.TempDir()
	gitInitDir(t, wsA)
	gitInitDir(t, wsB)

	svcA, err := BuildServices(ctx, &Globals{}, wsA)
	if err != nil {
		t.Fatalf("build A: %v", err)
	}
	defer svcA.Close() //nolint:errcheck
	svcB, err := BuildServices(ctx, &Globals{}, wsB)
	if err != nil {
		t.Fatalf("build B: %v", err)
	}
	defer svcB.Close() //nolint:errcheck

	if svcA.Notify == nil || svcB.Notify == nil {
		t.Fatalf("notify services must be online (degraded A=%v B=%v)",
			svcA.Degraded, svcB.Degraded)
	}

	// Channel subscriptions live in the ONE shared database: a
	// subscription made through workspace A's services is visible
	// through workspace B's — that is the single-DB contract.
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
	if len(subsB) != 1 || subsB[0].Agent != "agent-a" {
		t.Errorf("shared DB: workspace B must see the same channel subscription, got %+v", subsB)
	}

	// Agents are isolated by repo key: an agent registered in A does
	// not appear in B's manager.
	if regErr := svcA.AgentMgr.RegisterStopped(&bcagent.Agent{
		Name:      "shared-db-agent",
		Role:      bcagent.Role("engineer"),
		Workspace: wsA,
		Repo:      wsA,
	}); regErr != nil {
		t.Fatalf("register agent in A: %v", regErr)
	}
	if got := svcB.AgentMgr.GetAgent("shared-db-agent"); got != nil {
		t.Error("agent registered in workspace A must not be visible in workspace B's manager")
	}

	// Agent names are globally unique: reusing the name from another
	// repo must be rejected with a helpful error.
	dupErr := svcB.AgentMgr.RegisterStopped(&bcagent.Agent{
		Name:      "shared-db-agent",
		Role:      bcagent.Role("engineer"),
		Workspace: wsB,
		Repo:      wsB,
	})
	if dupErr == nil {
		t.Fatal("duplicate agent name across repos must be rejected")
	}
	if msg := dupErr.Error(); !strings.Contains(msg, "shared-db-agent") || !strings.Contains(msg, "already in use") {
		t.Errorf("duplicate-name error should identify the conflict, got: %v", dupErr)
	}

	// One database file at MYCEL_HOME — no per-workspace bc.db files.
	if _, statErr := os.Stat(filepath.Join(home, "mycel.db")); statErr != nil {
		t.Errorf("global mycel.db missing: %v", statErr)
	}
	for _, dir := range []string{wsA, wsB} {
		if _, statErr := os.Stat(filepath.Join(dir, ".bc", "bc.db")); statErr == nil {
			t.Errorf("per-workspace bc.db must not be created anymore (%s)", dir)
		}
	}
}

// TestBuildServices_LazyGlobalDB simulates a fresh boot: no database
// exists anywhere until BuildServices runs, at which point the global
// mycel.db is opened lazily and the stores come up online rather than
// degraded.
func TestBuildServices_LazyGlobalDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MYCEL_HOME", home)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")
	t.Cleanup(func() { _ = bcdb.CloseGlobal() })
	ctx := context.Background()

	wsDir := t.TempDir()
	gitInitDir(t, wsDir)

	// Nothing pre-opened: the global db file must not exist yet.
	if _, err := os.Stat(filepath.Join(home, "mycel.db")); err == nil {
		t.Fatal("mycel.db must not exist before services are built")
	}

	svc, err := BuildServices(ctx, &Globals{}, wsDir)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer svc.Close() //nolint:errcheck

	for name, present := range map[string]bool{
		"notify": svc.Notify != nil,
		"cron":   svc.Cron != nil,
		"mcp":    svc.MCP != nil,
		"tools":  svc.Tools != nil,
		"events": svc.EventLog != nil,
	} {
		if !present {
			t.Errorf("%s store not online after lazy add (degraded: %v)", name, svc.Degraded)
		}
	}
	if reason, degraded := svc.Degraded["storage"]; degraded {
		t.Errorf("storage degraded on lazy add: %s", reason)
	}
	if _, err := os.Stat(filepath.Join(home, "mycel.db")); err != nil {
		t.Errorf("global mycel.db not created lazily: %v", err)
	}
}
