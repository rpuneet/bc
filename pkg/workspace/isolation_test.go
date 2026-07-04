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
