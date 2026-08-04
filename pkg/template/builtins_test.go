package template

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinsAreWellFormed(t *testing.T) {
	names := BuiltinNames()
	// #3552: only blank ships; the rest are withdrawn.
	if len(names) != 1 || names[0] != "blank" {
		t.Fatalf("expected only blank to ship, got %v", names)
	}

	for _, name := range names {
		tmpl, prompt, err := Builtin(name)
		if err != nil {
			t.Fatalf("built-in %q: %v", name, err)
		}
		if tmpl.Name != name {
			t.Errorf("built-in %q reports name %q", name, tmpl.Name)
		}
		if strings.TrimSpace(tmpl.Description) == "" {
			t.Errorf("built-in %q has no description — it is the only thing shown in the list", name)
		}
		if len(tmpl.MCPs) == 0 {
			t.Errorf("built-in %q declares no MCPs", name)
		}
		if tmpl.Label != "single-agent" {
			t.Errorf("built-in %q label = %q, want single-agent", name, tmpl.Label)
		}
		_ = prompt // blank may be empty by design

		// Fields the daemon accepts and does not yet apply must stay empty, or a
		// built-in would make exactly the promise the template editor stopped
		// making (#3550).
		if len(tmpl.Secrets) > 0 || len(tmpl.Plugins) > 0 {
			t.Errorf("built-in %q sets secrets/plugins, which are not applied to agents yet", name)
		}
		if tmpl.ToolPolicies != nil || len(tmpl.ContextFiles) > 0 || tmpl.SystemPromptFile != "" {
			t.Errorf("built-in %q sets a field nothing reads", name)
		}
	}
}

func TestWithdrawnBuiltinsListsTheRetiredTaskPrompts(t *testing.T) {
	got := WithdrawnBuiltins()
	if len(got) < 30 {
		t.Fatalf("expected the retired library listed for withdraw, got %d", len(got))
	}
	for _, n := range got {
		if n == "blank" {
			t.Fatal("blank must keep shipping; it is not withdrawn")
		}
	}
	// Spot-check names that CreateAgentModal used to hardcode.
	want := map[string]bool{"feature-dev": true, "reviewer": true, "manager": true}
	for _, n := range got {
		delete(want, n)
	}
	if len(want) > 0 {
		t.Fatalf("withdrawn list missing %v", want)
	}
}

func TestEnsureBuiltinsInstallsIntoAnEmptyDir(t *testing.T) {
	dir := t.TempDir()

	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("EnsureBuiltins: %v", err)
	}
	if len(added) != len(BuiltinNames()) {
		t.Fatalf("added %d of %d built-ins", len(added), len(BuiltinNames()))
	}

	list, err := NewStore(dir).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(BuiltinNames()) {
		t.Fatalf("store lists %d templates, expected %d", len(list), len(BuiltinNames()))
	}

	// The bookkeeping file must not read back as a template.
	for _, tmpl := range list {
		if strings.HasPrefix(tmpl.Name, ".") {
			t.Errorf("state file surfaced as a template: %q", tmpl.Name)
		}
	}
}

func TestEnsureBuiltinsIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("first run: %v", err)
	}

	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("second run added %v", added)
	}
}

func TestEnsureBuiltinsLeavesAnEditedTemplateAlone(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// An older build shipped feature-dev; the user then rewrote its prompt.
	if err := s.Create(Template{Name: "feature-dev", Description: "mine", MCPs: []string{"mycel"}}, "my own prompt\n", ScopeGlobal); err != nil {
		t.Fatalf("seed a pre-existing template: %v", err)
	}

	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("EnsureBuiltins: %v", err)
	}
	for _, n := range added {
		if n == "feature-dev" {
			t.Fatal("overwrote a template that was already on disk")
		}
	}

	got, prompt, err := s.Get("feature-dev")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "mine" || prompt != "my own prompt\n" {
		t.Errorf("edited template was replaced: description=%q prompt=%q", got.Description, prompt)
	}
}

func TestEnsureBuiltinsUpgradesWhenHashStillMatches(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Simulate a prior ship: record the current on-disk hash, then rewrite
	// the files to a stale body while leaving the recorded hash pointing at
	// those "old" bytes — the next EnsureBuiltins must treat them as ours
	// and replace with the embed.
	name := "blank"
	staleJSON := []byte(`{"name":"blank","description":"stale","mcps":["bc"]}` + "\n")
	staleMD := []byte("old prompt that referenced bc\n")
	staleHash := builtinContentHash(staleJSON, string(staleMD))
	if err := os.WriteFile(filepath.Join(dir, name+".json"), staleJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), staleMD, 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := readBuiltinState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Hashes == nil {
		state.Hashes = map[string]string{}
	}
	state.Hashes[name] = staleHash
	if werr := writeBuiltinState(dir, state); werr != nil {
		t.Fatal(werr)
	}

	if _, uerr := EnsureBuiltins(dir); uerr != nil {
		t.Fatalf("upgrade run: %v", uerr)
	}

	raw, rerr := os.ReadFile(filepath.Join(dir, name+".json")) //nolint:gosec // test path under temp dir
	if rerr != nil {
		t.Fatal(rerr)
	}
	if strings.Contains(string(raw), `"bc"`) {
		t.Fatalf("stale builtin was not upgraded: %s", raw)
	}
	shipHash, err := shippedBuiltinHash(name)
	if err != nil {
		t.Fatal(err)
	}
	diskHash, err := diskBuiltinHash(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if diskHash != shipHash {
		t.Fatalf("after upgrade disk hash %s != ship hash %s", diskHash, shipHash)
	}
	state, err = readBuiltinState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Hashes[name] != shipHash {
		t.Fatalf("state hash = %q, want %q", state.Hashes[name], shipHash)
	}
}

func TestEnsureBuiltinsDoesNotUpgradeAnEditEvenWithARecordedHash(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("install: %v", err)
	}

	name := "blank"
	state, err := readBuiltinState(dir)
	if err != nil {
		t.Fatal(err)
	}
	// User edits after install: disk hash diverges from recorded.
	edit := []byte(`{"name":"blank","description":"my edit","mcps":["mycel"]}` + "\n")
	if werr := os.WriteFile(filepath.Join(dir, name+".json"), edit, 0o600); werr != nil {
		t.Fatal(werr)
	}
	if werr := os.WriteFile(filepath.Join(dir, name+".md"), []byte("edited prompt\n"), 0o600); werr != nil {
		t.Fatal(werr)
	}

	if _, uerr := EnsureBuiltins(dir); uerr != nil {
		t.Fatalf("second run: %v", uerr)
	}
	raw, rerr := os.ReadFile(filepath.Join(dir, name+".json")) //nolint:gosec // test path under temp dir
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !strings.Contains(string(raw), "my edit") {
		t.Fatalf("edit was overwritten: %s", raw)
	}
	// Recorded hash must still be the pre-edit value (we do not adopt edits).
	if state.Hashes[name] == "" {
		t.Fatal("expected a recorded hash from install")
	}
	state2, _ := readBuiltinState(dir)
	if state2.Hashes[name] != state.Hashes[name] {
		t.Fatalf("hash was rewritten to follow the edit")
	}
}

func TestEnsureBuiltinsWithdrawsAnOwnedBuiltin(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	name := "blank"
	state, err := readBuiltinState(dir)
	if err != nil {
		t.Fatal(err)
	}
	state.Withdrawn = append(state.Withdrawn, name)
	if err := writeBuiltinState(dir, state); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("withdraw run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, name+".json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("owned withdrawn builtin still on disk: %v", err)
	}
	// Must not come back on a further run.
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, name+".json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("withdrawn builtin was reinstalled")
	}
}

func TestEnsureBuiltinsWithdrawsRetiredTaskPrompts(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Simulate a v0.4.5 workspace that still has an unedited feature-dev
	// with a recorded hash matching the on-disk bytes.
	jsonBytes := []byte(`{"description":"Implement a feature","mcps":["mycel"]}` + "\n")
	md := "old feature-dev prompt\n"
	hash := builtinContentHash(jsonBytes, md)
	if err := os.WriteFile(filepath.Join(dir, "feature-dev.json"), jsonBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "feature-dev.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
	state := builtinState{
		Installed: []string{"feature-dev"},
		Hashes:    map[string]string{"feature-dev": hash},
	}
	if err := writeBuiltinState(dir, state); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("EnsureBuiltins: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feature-dev.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("retired feature-dev should have been withdrawn")
	}
	// blank should still install for this workspace.
	if _, _, err := s.Get("blank"); err != nil {
		t.Fatalf("blank should still ship: %v", err)
	}
}

func TestEnsureBuiltinsRespectsADeletedTemplate(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("install: %v", err)
	}

	s := NewStore(dir)
	if err := s.Delete("blank", ScopeGlobal); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// A deletion is a decision. Reinstalling it on the next daemon start would
	// make it impossible to get rid of a built-in you do not want.
	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	for _, n := range added {
		if n == "blank" {
			t.Fatal("reinstalled a template the user deleted")
		}
	}
	if _, _, err := s.Get("blank"); err == nil {
		t.Fatal("deleted template came back")
	}
}

func TestEnsureBuiltinsSurvivesACorruptStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, builtinStateFile), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("EnsureBuiltins: %v", err)
	}
	if len(added) != len(BuiltinNames()) {
		t.Fatalf("added %d built-ins past a corrupt state file", len(added))
	}
}

func TestSeedDefaultsInstallsTheBuiltins(t *testing.T) {
	dir := t.TempDir()
	if err := SeedDefaults(dir); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	list, err := NewStore(dir).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(BuiltinNames()) {
		t.Fatalf("SeedDefaults left %d templates, want %d", len(list), len(BuiltinNames()))
	}
}
