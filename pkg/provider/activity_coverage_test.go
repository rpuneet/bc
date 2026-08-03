package provider

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configFilesUnder returns every regular file below root, keyed by its
// root-relative path, with its contents. Used to assert on what a provider's
// WriteHookConfig actually produced.
func configFilesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	found := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, readErr := os.ReadFile(path) //nolint:gosec // test-local temp dir
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		found[rel] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return found
}

// Activity coverage across the whole registry.
//
// The Live feed only works for a provider that declares how mycel can observe
// it. When a provider declares nothing, mycel silently falls back to writing
// Claude hook settings the provider never reads, so the agent looks permanently
// idle and the UI has no way to explain why. That is exactly how cursor shipped
// without a working Live tab.
//
// These tests make the gap impossible to reintroduce: every registered provider
// must state its mode, and each mode carries obligations the provider must meet
// for the daemon to actually collect anything.

// TestEveryProviderDeclaresActivityMode fails for a provider that implements no
// ActivitySource at all. "none" is a valid, honest answer — silence is not.
func TestEveryProviderDeclaresActivityMode(t *testing.T) {
	for _, p := range DefaultRegistry.List() {
		src, ok := p.(ActivitySource)
		if !ok {
			t.Errorf("provider %q implements no ActivitySource: the Live tab cannot tell "+
				"'no events yet' from 'capture impossible'. Declare ActivityModeNone if it has no signal.", p.Name())
			continue
		}
		switch mode := src.ActivityMode(); mode {
		case ActivityModeHooks, ActivityModeTranscript, ActivityModeNone:
		default:
			t.Errorf("provider %q reports unknown activity mode %q", p.Name(), mode)
		}
	}
}

// TestHooksProvidersWriteAConfig checks that a provider claiming hooks mode
// actually writes something into the worktree. A hooks-mode provider whose
// WriteHookConfig is a no-op reports nothing at runtime, and nothing else in the
// system would notice.
func TestHooksProvidersWriteAConfig(t *testing.T) {
	for _, p := range DefaultRegistry.List() {
		src, ok := p.(ActivitySource)
		if !ok || src.ActivityMode() != ActivityModeHooks {
			continue
		}
		t.Run(p.Name(), func(t *testing.T) {
			dir := t.TempDir()
			if err := src.WriteHookConfig(dir, "", "agent-x"); err != nil {
				t.Fatalf("WriteHookConfig: %v", err)
			}
			files := configFilesUnder(t, dir)
			if len(files) == 0 {
				t.Fatal("hooks mode but WriteHookConfig wrote nothing — the agent will never report activity")
			}
			// The generated config must point at the daemon's hook endpoint,
			// resolving the agent from the environment rather than baking in a
			// value that goes stale the moment the agent is renamed or the
			// daemon moves port.
			var referencesEndpoint, resolvesAgent bool
			for _, body := range files {
				if strings.Contains(body, "/api/agents/") {
					referencesEndpoint = true
				}
				if strings.Contains(body, "MYCEL_AGENT_ID") {
					resolvesAgent = true
				}
			}
			if !referencesEndpoint {
				t.Error("no generated file references /api/agents/ — nothing will reach the daemon")
			}
			if !resolvesAgent {
				t.Error("no generated file references MYCEL_AGENT_ID — the agent identity is not resolved at runtime")
			}
		})
	}
}

// TestTranscriptProvidersCanBeParsed checks the pull-side obligation: a provider
// in transcript mode is useless to the tailer unless it can both locate its
// session file and parse a line of it.
func TestTranscriptProvidersCanBeParsed(t *testing.T) {
	for _, p := range DefaultRegistry.List() {
		src, ok := p.(ActivitySource)
		if !ok || src.ActivityMode() != ActivityModeTranscript {
			continue
		}
		t.Run(p.Name(), func(t *testing.T) {
			_, stateless := p.(TranscriptParser)
			_, session := p.(TranscriptSessionParser)
			if !stateless && !session {
				t.Error("transcript mode but no TranscriptParser or TranscriptSessionParser — the tailer skips this provider entirely")
			}
			if stateless && session {
				t.Error("implements both parser interfaces; a provider must pick one")
			}
			// The tailer finds the file either by glob or by an explicit
			// selector. With neither there is nothing to tail.
			_, selector := p.(TranscriptFileSelector)
			if !selector && len(src.TranscriptGlobs("/tmp/wt")) == 0 {
				t.Error("no TranscriptGlobs and no TranscriptFileSelector — the tailer cannot locate a session file")
			}
		})
	}
}

// TestNoneProvidersWriteNothing checks that a provider with no activity signal
// does not litter the worktree with config nothing reads.
func TestNoneProvidersWriteNothing(t *testing.T) {
	for _, p := range DefaultRegistry.List() {
		src, ok := p.(ActivitySource)
		if !ok || src.ActivityMode() != ActivityModeNone {
			continue
		}
		t.Run(p.Name(), func(t *testing.T) {
			dir := t.TempDir()
			if err := src.WriteHookConfig(dir, "", "agent-x"); err != nil {
				t.Fatalf("WriteHookConfig: %v", err)
			}
			if files := configFilesUnder(t, dir); len(files) != 0 {
				t.Errorf("wrote %d file(s) despite having no activity signal: %v", len(files), keys(files))
			}
		})
	}
}

// keys returns a map's keys, for readable failure output.
func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
