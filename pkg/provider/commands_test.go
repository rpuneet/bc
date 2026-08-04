package provider

import (
	"strings"
	"testing"
)

// TestCuratedCommands asserts every first-party provider surfaces a curated,
// non-empty command list via the CommandLister capability, and that each
// command's invocation starts with the provider's own binary — so the UI never
// shows a dead or mislabelled control.
func TestCuratedCommands(t *testing.T) {
	// binary prefix each provider's commands must start with.
	cases := map[string]string{
		"claude": "claude",
		"codex":  "codex",
		"pi":     "pi",
		"agy":    "agy",
		"cursor": "cursor-agent",
	}

	for name, prefix := range cases {
		t.Run(name, func(t *testing.T) {
			p, ok := DefaultRegistry.Get(name)
			if !ok {
				t.Fatalf("provider %q not registered", name)
			}
			cl, ok := p.(CommandLister)
			if !ok {
				t.Fatalf("provider %q does not implement CommandLister", name)
			}
			cmds := cl.Commands()
			if len(cmds) == 0 {
				t.Fatalf("provider %q returned no commands", name)
			}
			seen := map[string]bool{}
			for _, c := range cmds {
				if c.Name == "" || c.Command == "" || c.Description == "" {
					t.Errorf("provider %q has an incomplete command: %+v", name, c)
				}
				if !strings.HasPrefix(c.Command, prefix) {
					t.Errorf("provider %q command %q does not start with binary %q", name, c.Command, prefix)
				}
				if seen[c.Name] {
					t.Errorf("provider %q has duplicate command name %q", name, c.Name)
				}
				seen[c.Name] = true
			}
		})
	}
}

// TestCuratedCommands_VersionAndHelpRunnable covers PR #3423's fill: every
// first-party provider must expose a "version" and "help" entry, and both
// must be Runnable() — no TTY, no args, no unresolved placeholder — so the
// web UI's guarded POST /api/providers/{name}/run can execute them inline
// instead of only offering "copy + run in your terminal".
func TestCuratedCommands_VersionAndHelpRunnable(t *testing.T) {
	names := []string{"claude", "codex", "pi", "agy", "cursor"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			p, ok := DefaultRegistry.Get(name)
			if !ok {
				t.Fatalf("provider %q not registered", name)
			}
			cl, ok := p.(CommandLister)
			if !ok {
				t.Fatalf("provider %q does not implement CommandLister", name)
			}
			cmds := cl.Commands()

			var version, help *Command
			for i := range cmds {
				switch cmds[i].Name {
				case "version":
					version = &cmds[i]
				case "help":
					help = &cmds[i]
				}
			}
			if version == nil {
				t.Errorf("provider %q has no %q command", name, "version")
			} else if !version.Runnable() {
				t.Errorf("provider %q %q command %+v is not Runnable", name, "version", *version)
			}
			if help == nil {
				t.Errorf("provider %q has no %q command", name, "help")
			} else if !help.Runnable() {
				t.Errorf("provider %q %q command %+v is not Runnable", name, "help", *help)
			}
		})
	}
}
