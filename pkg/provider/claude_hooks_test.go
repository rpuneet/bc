package provider

// Tests for the Claude hook-settings writer, moved from pkg/agent alongside
// WriteClaudeHookSettings.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteClaudeHookSettings_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteClaudeHookSettings(dir); err != nil {
		t.Fatalf("WriteClaudeHookSettings: %v", err)
	}
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}
	content := string(data)
	// Should have all 21 valid Claude Code hook events (StopFailure excluded)
	for _, event := range []string{
		"SessionStart", "SessionEnd", "UserPromptSubmit",
		"PreToolUse", "PostToolUse", "PostToolUseFailure",
		"PermissionRequest", "Stop",
		"SubagentStart", "SubagentStop", "TaskCompleted",
		"WorktreeCreate", "WorktreeRemove",
		"PreCompact", "PostCompact",
		"Elicitation", "ElicitationResult",
	} {
		if !strings.Contains(content, `"`+event+`"`) {
			t.Errorf("settings.json missing hook event %q", event)
		}
	}
	// Should use HTTP POST, not file-based
	if !strings.Contains(content, "/api/agents/") {
		t.Error("settings.json should contain HTTP hook URL")
	}
}

func TestWriteClaudeHookSettings_Idempotent(t *testing.T) {
	dir := t.TempDir()
	for i := range 3 {
		if err := WriteClaudeHookSettings(dir); err != nil {
			t.Fatalf("call %d: WriteClaudeHookSettings: %v", i, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json")) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	// Each hook event should appear exactly once as a key
	count := strings.Count(string(data), `"PreToolUse"`)
	if count != 1 {
		t.Errorf("PreToolUse appears %d times, want 1", count)
	}
}

func TestWriteClaudeHookSettings_MergesExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		t.Fatal(err)
	}
	// Use a non-bc-managed custom hook key — bc hooks overwrite, custom hooks are preserved.
	existing := `{"hooks":{"CustomHook":[{"hooks":[{"type":"command","command":"echo hi"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteClaudeHookSettings(dir); err != nil {
		t.Fatalf("WriteClaudeHookSettings: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json")) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	content := string(data)
	// Custom user hook preserved
	if !strings.Contains(content, "echo hi") {
		t.Error("existing CustomHook was removed during merge")
	}
	if !strings.Contains(content, "PreToolUse") {
		t.Error("PreToolUse hook not added during merge")
	}
}

func TestWriteClaudeHookSettings_PreservesUserCustomizedHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		t.Fatal(err)
	}
	// User has customized PreToolUse with their own command (not bc-managed).
	existing := `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"echo my-custom-hook"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteClaudeHookSettings(dir); err != nil {
		t.Fatalf("WriteClaudeHookSettings: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json")) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	content := string(data)
	// User's custom hook should be preserved, not overwritten by bc
	if !strings.Contains(content, "my-custom-hook") {
		t.Error("user-customized PreToolUse hook was overwritten by bc")
	}
}

func TestWriteClaudeHookSettings_RemovesInvalidKeys(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0750); err != nil {
		t.Fatal(err)
	}
	// Simulate old settings with StopFailure (invalid Claude Code hook key)
	// PreToolUse has a bc-managed hook (contains /api/agents/) that should be overwritten.
	existing := `{"hooks":{"StopFailure":[{"hooks":[{"type":"command","command":"old"}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"curl /api/agents/old/hook"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteClaudeHookSettings(dir); err != nil {
		t.Fatalf("WriteClaudeHookSettings: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json")) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "StopFailure") {
		t.Error("StopFailure should have been removed from settings")
	}
	// PreToolUse should be overwritten with new bc hook, not preserved as "old"
	if strings.Contains(content, `"old"`) {
		t.Error("old hook commands should have been overwritten")
	}
}

// The hook merge must round-trip unknown top-level settings keys —
// permissions, env, model etc. survive a rewrite untouched.
func TestWriteClaudeHookSettingsPreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	existing := `{
  "permissions": {"allow": ["Bash(ls:*)"]},
  "model": "opus",
  "env": {"FOO": "bar"},
  "hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "echo user-hook"}]}]}
}`
	path := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteClaudeHookSettings(dir); err != nil {
		t.Fatalf("WriteClaudeHookSettings: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // t.TempDir()-derived test path
	if err != nil {
		t.Fatal(err)
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(data, &full); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	for _, key := range []string{"permissions", "model", "env", "hooks"} {
		if _, ok := full[key]; !ok {
			t.Errorf("top-level key %q was dropped by the hook merge", key)
		}
	}
	if got := string(full["model"]); got != `"opus"` {
		t.Errorf("model = %s, want \"opus\"", got)
	}
	// The user's own SessionStart hook survives alongside the bc hook.
	var hooks map[string][]claudeHookMatcher
	if err := json.Unmarshal(full["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	foundUser := false
	for _, m := range hooks["SessionStart"] {
		for _, h := range m.Hooks {
			if h.Command == "echo user-hook" {
				foundUser = true
			}
		}
	}
	if !foundUser {
		t.Error("user SessionStart hook was dropped by the merge")
	}
}
