package agent

import (
	"testing"
)

func TestStateForHookEvent(t *testing.T) {
	tests := []struct {
		event HookEvent
		want  State
		ok    bool
	}{
		{HookSessionStart, StateIdle, true},
		{HookSessionEnd, StateStopped, true},
		{HookUserPromptSubmit, StateWorking, true},
		{HookTaskCompleted, StateDone, true},
		{HookPermissionRequest, StateStuck, true},
		{HookStop, StateIdle, true},
		// Tool-level events are informational, not state-changing
		{HookPreToolUse, "", false},
		{HookPostToolUse, "", false},
		{HookEvent("unknown"), "", false},
		{HookEvent(""), "", false},
	}
	for _, tc := range tests {
		got, ok := StateForHookEvent(tc.event)
		if ok != tc.ok {
			t.Errorf("StateForHookEvent(%q) ok=%v, want %v", tc.event, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Errorf("StateForHookEvent(%q) = %q, want %q", tc.event, got, tc.want)
		}
	}
}

func TestIsKnownEvent(t *testing.T) {
	// State-changing events
	if !IsKnownEvent(HookPreToolUse) {
		t.Error("PreToolUse should be known")
	}
	// Informational events
	if !IsKnownEvent(HookNotification) {
		t.Error("Notification should be known")
	}
	if !IsKnownEvent(HookChannelMessage) {
		t.Error("ChannelMessage should be known")
	}
	// Unknown
	if IsKnownEvent(HookEvent("bogus")) {
		t.Error("bogus should not be known")
	}
}
