package provider

import (
	"path/filepath"
	"testing"
)

func TestPiEncodeCWD(t *testing.T) {
	// Verified against real directories under ~/.pi/agent/sessions.
	cases := map[string]string{
		"/Users/puneetrai/.mycel/worktrees/amber-heron":              "--Users-puneetrai-.mycel-worktrees-amber-heron--",
		"/Users/puneetrai/Projects/bc/.bc/agents/zen-zebra/bc-bc-zz": "--Users-puneetrai-Projects-bc-.bc-agents-zen-zebra-bc-bc-zz--",
	}
	for cwd, want := range cases {
		if got := encodePiCWD(cwd); got != want {
			t.Errorf("encodePiCWD(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestPiTranscriptGlobs(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "/tmp/pisess")
	p := NewPiProvider()
	globs := p.TranscriptGlobs("/Users/x/wt")
	want := filepath.Join("/tmp/pisess", "--Users-x-wt--", "*.jsonl")
	if len(globs) != 1 || globs[0] != want {
		t.Fatalf("TranscriptGlobs = %v, want [%s]", globs, want)
	}
	if got := p.TranscriptGlobs(""); got != nil {
		t.Errorf("TranscriptGlobs(\"\") = %v, want nil", got)
	}
}

func TestPiActivityMode(t *testing.T) {
	p := NewPiProvider()
	if p.ActivityMode() != ActivityModeTranscript {
		t.Errorf("ActivityMode = %q, want %q", p.ActivityMode(), ActivityModeTranscript)
	}
	// WriteHookConfig is a no-op and must not error.
	if err := p.WriteHookConfig("/tmp", "", ""); err != nil {
		t.Errorf("WriteHookConfig returned %v, want nil", err)
	}
}

func TestPiParseTranscriptLine(t *testing.T) {
	p := NewPiProvider()

	tests := []struct {
		check   func(t *testing.T, acts []TranscriptActivity)
		name    string
		line    string
		wantLen int
	}{
		{
			name:    "session start",
			line:    `{"type":"session","cwd":"/Users/x/wt","timestamp":"2026-07-06T13:36:41.507Z"}`,
			wantLen: 1,
			check: func(t *testing.T, acts []TranscriptActivity) {
				if acts[0].Event != "SessionStart" {
					t.Errorf("event = %q, want SessionStart", acts[0].Event)
				}
			},
		},
		{
			name:    "user prompt",
			line:    `{"type":"message","timestamp":"2026-07-06T13:36:41.600Z","message":{"role":"user","content":[{"type":"text","text":"explore the mcp setup"}]}}`,
			wantLen: 1,
			check: func(t *testing.T, acts []TranscriptActivity) {
				if acts[0].Event != "UserPromptSubmit" {
					t.Errorf("event = %q, want UserPromptSubmit", acts[0].Event)
				}
				if acts[0].Prompt != "explore the mcp setup" {
					t.Errorf("prompt = %q", acts[0].Prompt)
				}
			},
		},
		{
			name:    "assistant with two tool calls",
			line:    `{"type":"message","timestamp":"2026-07-06T13:36:42Z","message":{"role":"assistant","content":[{"type":"text","text":"I'll look."},{"type":"toolCall","id":"functions.bash:0","name":"bash","arguments":{"command":"ls -la"}},{"type":"toolCall","id":"functions.bash:1","name":"read_file","arguments":{"path":"a.txt"}}]}}`,
			wantLen: 2,
			check: func(t *testing.T, acts []TranscriptActivity) {
				if acts[0].Event != "PreToolUse" || acts[0].ToolName != "bash" {
					t.Errorf("act0 = %+v, want PreToolUse/bash", acts[0])
				}
				input, ok := acts[0].ToolInput.(map[string]any)
				if !ok || input["command"] != "ls -la" {
					t.Errorf("act0 tool_input = %v", acts[0].ToolInput)
				}
				if acts[1].ToolName != "read_file" {
					t.Errorf("act1 tool = %q, want read_file", acts[1].ToolName)
				}
			},
		},
		{
			name:    "assistant with only text yields nothing",
			line:    `{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
			wantLen: 0,
		},
		{
			name:    "tool result success",
			line:    `{"type":"message","timestamp":"2026-07-06T13:36:43Z","message":{"role":"toolResult","toolCallId":"functions.bash:0","toolName":"bash","isError":false,"content":[{"type":"text","text":"./.mcp.json\n"}]}}`,
			wantLen: 1,
			check: func(t *testing.T, acts []TranscriptActivity) {
				if acts[0].Event != "PostToolUse" || acts[0].ToolName != "bash" {
					t.Errorf("act = %+v, want PostToolUse/bash", acts[0])
				}
				if acts[0].ToolResponse != "./.mcp.json\n" {
					t.Errorf("tool_response = %q", acts[0].ToolResponse)
				}
			},
		},
		{
			name:    "tool result error",
			line:    `{"type":"message","message":{"role":"toolResult","toolName":"bash","isError":true,"content":[{"type":"text","text":"boom"}]}}`,
			wantLen: 1,
			check: func(t *testing.T, acts []TranscriptActivity) {
				if acts[0].Event != "PostToolUseFailure" {
					t.Errorf("event = %q, want PostToolUseFailure", acts[0].Event)
				}
				if acts[0].Error != "boom" {
					t.Errorf("error = %q, want boom", acts[0].Error)
				}
			},
		},
		{name: "malformed json", line: `{not json`, wantLen: 0},
		{name: "blank line", line: `   `, wantLen: 0},
		{name: "unknown type", line: `{"type":"model_change","provider":"x"}`, wantLen: 0},
		{name: "message with no payload", line: `{"type":"message"}`, wantLen: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			acts, err := p.ParseTranscriptLine([]byte(tc.line))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(acts) != tc.wantLen {
				t.Fatalf("got %d activities, want %d: %+v", len(acts), tc.wantLen, acts)
			}
			if tc.check != nil {
				tc.check(t, acts)
			}
		})
	}
}
