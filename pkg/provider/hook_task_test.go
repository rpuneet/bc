package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An agent's task line is derived from the prompt it was handed at the start of
// a turn — see pkg/agent/hook_ingest.go. Before that, each hook shipped a
// hand-written label for the event that fired ("Processing prompt...",
// "Thinking...", "Turn complete") in the payload's task field, and the daemon
// stores whatever arrives there: the Live feed then showed `task: Thinking...`
// in the very field that holds the real task line, so a label naming the hook
// occupied the place reserved for what the agent had been asked to do.
//
// The labels are gone from all three hook-writing providers. These tests pin
// that, because the strings are cheap to reintroduce — they read like helpful
// UI text at the call site, and nothing else fails when one comes back.

// writeHookConfig writes one provider's activity configuration into a temporary
// worktree and returns every file it produced there, so a test can assert on
// what the agent's CLI will actually run.
func writeHookConfig(t *testing.T, write func(string) error, files ...string) map[string]string {
	t.Helper()
	root := t.TempDir()
	if err := write(root); err != nil {
		t.Fatalf("write hook config: %v", err)
	}
	out := make(map[string]string, len(files))
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec // test-local temp dir
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		out[rel] = string(raw)
	}
	return out
}

func TestHookConfigsCarryNoHardcodedTask(t *testing.T) {
	tests := []struct {
		name  string
		write func(string) error
		file  string
	}{
		{"claude", WriteClaudeHookSettings, ".claude/settings.json"},
		{"agy", WriteAgyHookSettings, ".agents/hooks.json"},
		{"cursor", WriteCursorHookSettings, cursorReporterRelPath},
	}

	// Every label the three providers used to send. A provider that reports one
	// of these is overwriting a derived task line with the name of a hook.
	labels := []string{
		"Session started", "Session ended", "Processing prompt...", "Running tool",
		"Tool completed", "Tool failed", "Subagent started", "Subagent completed",
		"Compacting context...", "Context compacted", "Turn complete", "Thinking...",
		"Response received", "Task completed", "Creating worktree",
		"Waiting for permission", "MCP input needed", "MCP input received",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeHookConfig(t, tt.write, tt.file)[tt.file]
			for _, label := range labels {
				if strings.Contains(got, label) {
					t.Errorf("%s reports the hardcoded task %q; the task line is derived from the prompt", tt.file, label)
				}
			}
		})
	}
}

// The state field is what carries an event's lifecycle meaning, and removing the
// task labels must not have taken it with them: without state, nothing moves an
// agent between idle and working and the Live feed is all the UI has left.
func TestHookConfigsStillReportState(t *testing.T) {
	claude := writeHookConfig(t, WriteClaudeHookSettings, ".claude/settings.json")[".claude/settings.json"]
	if !strings.Contains(claude, `state:\\\"working\\\"`) {
		t.Errorf("claude settings no longer set state=working:\n%s", claude)
	}

	agy := writeHookConfig(t, WriteAgyHookSettings, ".agents/hooks.json")[".agents/hooks.json"]
	if !strings.Contains(agy, `state:\\\"working\\\"`) {
		t.Errorf("agy hooks no longer set state=working:\n%s", agy)
	}

	// Cursor's reporter takes the state as an argument, so the hooks file is
	// where the value appears.
	cursor := writeHookConfig(t, WriteCursorHookSettings, ".cursor/hooks.json")[".cursor/hooks.json"]
	if !strings.Contains(cursor, "working") {
		t.Errorf("cursor hooks no longer pass a working state:\n%s", cursor)
	}
}
