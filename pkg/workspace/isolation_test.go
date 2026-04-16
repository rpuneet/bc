package workspace

import (
	"path/filepath"
	"testing"
)

// TestMultiWorkspaceIsolation confirms the invariants the proposal §4.3 relies
// on for keeping multiple workspaces' runtime artifacts (tmux sessions, worktree
// dirs, docker containers) from colliding:
//
//  1. Two workspaces with the same basename produce distinct IDs.
//  2. ComputeWorkspaceID is deterministic — the same path always hashes the
//     same way, across process restarts.
//  3. Distinct paths hash to distinct IDs, even when they only differ in a
//     single trailing segment.
//  4. IDs are 12 chars of hex so naming schemes like `bc-<id>-<agent>` keep a
//     predictable budget.
//
// These invariants are what lets us put two workspaces named "bc" side-by-side
// without one's agents shadowing the other's tmux sessions.
func TestMultiWorkspaceIsolation(t *testing.T) {
	t.Parallel()

	// 1) Same basename, different parents -> different IDs.
	// Two real workspaces on disk: ~/Projects/bc and ~/Projects/bc-deploy/bc
	idA := ComputeWorkspaceID("/Users/test/Projects/bc")
	idB := ComputeWorkspaceID("/Users/test/Projects/bc-deploy/bc")
	if idA == idB {
		t.Fatalf("same basename should not collide: both %q produced %q", "bc", idA)
	}

	// 2) Deterministic.
	idAgain := ComputeWorkspaceID("/Users/test/Projects/bc")
	if idA != idAgain {
		t.Fatalf("ComputeWorkspaceID not deterministic: first=%q second=%q", idA, idAgain)
	}

	// 3) Distinct neighbors -> distinct IDs.
	pathC := filepath.Join("/Users/test/Projects", "trade")
	pathD := filepath.Join("/Users/test/Projects", "trader")
	if ComputeWorkspaceID(pathC) == ComputeWorkspaceID(pathD) {
		t.Fatalf("adjacent paths should not collide: %q and %q", pathC, pathD)
	}

	// 4) 12 chars of lowercase hex.
	if got := len(idA); got != 12 {
		t.Fatalf("ID length: want 12, got %d (%q)", got, idA)
	}
	for _, c := range idA {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("ID contains non-hex char %q in %q", c, idA)
		}
	}
}

// TestRegistryIsolation confirms that registering two workspaces with the same
// basename yields two distinct entries addressable by their stable IDs, so
// downstream code (worktree manager, tmux session names, docker container
// names) can use the ID as the disambiguating prefix.
func TestRegistryIsolation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	r := &Registry{path: filepath.Join(tmp, "registry.json")}

	const pathA = "/Users/test/Projects/bc"
	const pathB = "/Users/test/Projects/bc-deploy/bc"

	if err := r.RegisterWithAlias(pathA, "bc", ""); err != nil {
		t.Fatalf("register A: %v", err)
	}
	if err := r.RegisterWithAlias(pathB, "bc", ""); err != nil {
		t.Fatalf("register B: %v", err)
	}

	if got := len(r.Workspaces); got != 2 {
		t.Fatalf("registry should hold 2 entries with shared basename, got %d", got)
	}

	a := r.FindByID(ComputeWorkspaceID(pathA))
	b := r.FindByID(ComputeWorkspaceID(pathB))
	if a == nil || b == nil {
		t.Fatalf("FindByID missed one: a=%v b=%v", a, b)
	}
	if a.Path == b.Path {
		t.Fatalf("both entries resolved to the same path %q", a.Path)
	}
	if a.ID == b.ID {
		t.Fatalf("both entries share ID %q — hash collision", a.ID)
	}
}
