package provider

import "testing"

// TestDockerDrivableProviders verifies that every provider shipped with a
// Docker agent image implements SessionCustomizer and tmux-wraps its command
// for container execution — without which mycel cannot drive it via SendKeys.
func TestDockerDrivableProviders(t *testing.T) {
	providers := []struct {
		p    Provider
		base string
	}{
		{NewCodexProvider(), "codex --full-auto"},
		{NewCursorProvider(), "cursor-agent"},
		{NewPiProvider(), "pi"},
	}
	for _, tc := range providers {
		t.Run(tc.p.Name(), func(t *testing.T) {
			sc, ok := tc.p.(SessionCustomizer)
			if !ok {
				t.Fatalf("%s must implement SessionCustomizer to be drivable in Docker", tc.p.Name())
			}
			// Native tmux: command passes through unchanged.
			if got := sc.AdjustSessionCommand(tc.base); got != tc.base {
				t.Errorf("AdjustSessionCommand() = %q, want unchanged %q", got, tc.base)
			}
			// Docker: command is wrapped in a named tmux session.
			want := `tmux new-session -s "$MYCEL_WORKTREE_NAME" "` + tc.base + `"`
			if got := sc.AdjustContainerCommand(tc.base); got != want {
				t.Errorf("AdjustContainerCommand() = %q, want %q", got, want)
			}
		})
	}
}
