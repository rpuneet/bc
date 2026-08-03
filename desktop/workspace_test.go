package main

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

// Launched from Finder, the app's working directory is `/`, so it has nothing to
// infer a workspace from. It used to serve none — and because tmux session names
// carry a hash of the workspace, its daemon then looked for sessions under a
// prefix no running agent used: every agent listed as running, none attachable
// (#3569). The workspace the CLI daemon published is the answer.
func TestTheAppAdoptsTheWorkspaceTheLastDaemonServed(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("MYCEL_WORKSPACE", "")
	t.Chdir(t.TempDir()) // outside any adopted repo, as a GUI launch is

	ws := t.TempDir()
	if err := home.PublishDaemonWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	if got := resolveRepoRoot(); got != ws {
		t.Errorf("resolveRepoRoot() = %q, want the published workspace %q", got, ws)
	}
}

// An explicit workspace still wins: adopting a previous daemon's is a fallback,
// not an override.
func TestAnExplicitWorkspaceBeatsThePublishedOne(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	published, asked := t.TempDir(), t.TempDir()
	if err := home.PublishDaemonWorkspace(published); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYCEL_WORKSPACE", asked)

	if got := resolveRepoRoot(); got != asked {
		t.Errorf("resolveRepoRoot() = %q, want the requested %q", got, asked)
	}
}

// With no record at all the app boots without an anchor repo, as before.
func TestWithNothingPublishedTheAppServesNoWorkspace(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("MYCEL_WORKSPACE", "")
	t.Chdir(t.TempDir())

	if got := resolveRepoRoot(); got != "" {
		t.Errorf("resolveRepoRoot() = %q, want empty when no daemon has published a workspace", got)
	}
}
