package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinsAreWellFormed(t *testing.T) {
	names := BuiltinNames()
	if len(names) < 30 {
		t.Fatalf("expected a real library of built-ins, got %d", len(names))
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
		// blank is deliberately empty; everything else earns its place by
		// carrying a prompt worth applying.
		if name != "blank" && len(strings.TrimSpace(prompt)) < 200 {
			t.Errorf("built-in %q has a prompt too thin to be production-ready (%d chars)", name, len(prompt))
		}

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

func TestEnsureBuiltinsRespectsADeletedTemplate(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureBuiltins(dir); err != nil {
		t.Fatalf("install: %v", err)
	}

	s := NewStore(dir)
	if err := s.Delete("scraper", ScopeGlobal); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// A deletion is a decision. Reinstalling it on the next daemon start would
	// make it impossible to get rid of a built-in you do not want.
	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	for _, n := range added {
		if n == "scraper" {
			t.Fatal("reinstalled a template the user deleted")
		}
	}
	if _, _, err := s.Get("scraper"); err == nil {
		t.Fatal("deleted template came back")
	}
}

func TestEnsureBuiltinsAddsATemplateShippedLater(t *testing.T) {
	dir := t.TempDir()

	// Simulate a workspace created by an older build: only the four originals,
	// recorded as installed.
	s := NewStore(dir)
	old := []string{"feature-dev", "reviewer", "manager", "blank"}
	for _, n := range old {
		if err := s.Create(Template{Name: n, Description: "old", MCPs: []string{"mycel"}}, "old\n", ScopeGlobal); err != nil {
			t.Fatalf("seed %q: %v", n, err)
		}
	}
	raw, err := json.Marshal(builtinState{Installed: old})
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if writeErr := os.WriteFile(filepath.Join(dir, builtinStateFile), raw, 0o600); writeErr != nil {
		t.Fatalf("write state: %v", writeErr)
	}

	added, err := EnsureBuiltins(dir)
	if err != nil {
		t.Fatalf("EnsureBuiltins: %v", err)
	}
	if len(added) == 0 {
		t.Fatal("an existing workspace received no new built-ins — the old seeding bug")
	}
	for _, n := range added {
		for _, o := range old {
			if n == o {
				t.Errorf("re-added pre-existing template %q", n)
			}
		}
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
		t.Fatalf("SeedDefaults installed %d of %d", len(list), len(BuiltinNames()))
	}
}
