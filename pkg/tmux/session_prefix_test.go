package tmux

import (
	"slices"
	"strings"
	"testing"
)

func TestNormalizeSessionPrefix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "mycel-"},
		{"mycel", "mycel-"},
		{"mycel-", "mycel-"},
		{"custom", "custom-"},
		{"custom-", "custom-"},
		{"  mycel  ", "mycel-"},
	}
	for _, tt := range tests {
		if got := NormalizeSessionPrefix(tt.in); got != tt.want {
			t.Errorf("NormalizeSessionPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSessionShell(t *testing.T) {
	m := &Manager{}
	if got := m.sessionShell(); got != "/bin/bash" {
		t.Errorf("default shell = %q", got)
	}
	m.DefaultShell = "/bin/zsh"
	if got := m.sessionShell(); got != "/bin/zsh" {
		t.Errorf("zsh = %q", got)
	}
	m.DefaultShell = "zsh" // relative — rejected
	if got := m.sessionShell(); got != "/bin/bash" {
		t.Errorf("unsafe shell fell through: %q", got)
	}
	m.DefaultShell = "/bin/bash; rm -rf /"
	if got := m.sessionShell(); got != "/bin/bash" {
		t.Errorf("metachar shell fell through: %q", got)
	}
}

func TestCreateSessionWithEnv_HistoryLimitAndShell(t *testing.T) {
	mock, records := recordingMock("")
	m := newTestManager("mycel-", mock)
	m.DefaultShell = "/bin/zsh"
	m.HistoryLimit = 5000

	if err := m.CreateSessionWithEnv(testCtx(), "agent1", "/repo", "echo hi", nil); err != nil {
		t.Fatalf("CreateSessionWithEnv: %v", err)
	}
	if len(*records) != 2 {
		t.Fatalf("want 2 tmux calls (new-session + set-option), got %d", len(*records))
	}
	args0 := (*records)[0].args
	if !slices.Contains(args0, "/bin/zsh") {
		t.Errorf("shell missing: %v", args0)
	}
	args1 := (*records)[1].args
	joined := strings.Join(args1, " ")
	if !strings.Contains(joined, "history-limit") || !strings.Contains(joined, "5000") {
		t.Errorf("history-limit missing: %v", args1)
	}
}
