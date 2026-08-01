package handlers

import (
	"testing"

	"github.com/rpuneet/mycel/pkg/events"
)

// TestToActivityItem_PrefersDescriptionOverCommand covers the row-title gap
// from #3423: a Bash tool call carries a human-written `description` in its
// tool_input (e.g. "Check rebuild completion") alongside the raw `command`.
// The activity timeline's Message field used to key off tool_name+command
// only, making rows read as cryptic shell one-liners. It must now prefer
// the description when present.
func TestToActivityItem_PrefersDescriptionOverCommand(t *testing.T) {
	e := events.Event{
		Type: "hook.PreToolUse",
		Data: map[string]any{
			"tool_name": "Bash",
			"tool_input": map[string]any{
				"command":     "cd /repo && grep -rn foo .",
				"description": "Check rebuild completion",
			},
		},
	}

	item := toActivityItem(e, false)

	want := "Bash: Check rebuild completion"
	if item.Message != want {
		t.Fatalf("Message = %q, want %q", item.Message, want)
	}
}

// TestToActivityItem_FallsBackToCommandWithoutDescription ensures tools (or
// Bash calls) that don't supply a description keep the previous behavior of
// surfacing the raw command rather than going blank.
func TestToActivityItem_FallsBackToCommandWithoutDescription(t *testing.T) {
	e := events.Event{
		Type: "hook.PreToolUse",
		Data: map[string]any{
			"tool_name": "Bash",
			"tool_input": map[string]any{
				"command": "ls -la",
			},
		},
	}

	item := toActivityItem(e, false)

	want := "Bash: ls -la"
	if item.Message != want {
		t.Fatalf("Message = %q, want %q", item.Message, want)
	}
}

// TestToActivityItem_ToolWithoutInputFallsBackToToolName covers a tool_name
// with no tool_input at all (e.g. a lifecycle-ish hook) — the row still
// gets a title rather than an empty Message.
func TestToActivityItem_ToolWithoutInputFallsBackToToolName(t *testing.T) {
	e := events.Event{
		Type: "hook.PreToolUse",
		Data: map[string]any{
			"tool_name": "Read",
		},
	}

	item := toActivityItem(e, false)

	if item.Message != "Read" {
		t.Fatalf("Message = %q, want %q", item.Message, "Read")
	}
}
