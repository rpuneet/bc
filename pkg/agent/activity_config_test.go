package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/provider"
)

// writeActivityConfig decides what mycel drops into a new agent's worktree so
// the agent can report what it is doing. Getting it wrong is invisible at create
// time and shows up much later as an agent that never leaves "idle", so each
// provider's expected artifact is pinned here.

// hasFile reports whether a worktree-relative path exists under root.
func hasFile(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func TestWriteActivityConfigPerProvider(t *testing.T) {
	cases := []struct {
		tool string
		want string // worktree-relative artifact, "" when nothing should be written
		why  string
	}{
		{
			tool: "claude",
			want: ".claude/settings.json",
			why:  "claude reads hooks from its own settings file",
		},
		{
			tool: "agy",
			want: ".agents/hooks.json",
			why:  "agy reads lifecycle hooks from .agents/hooks.json",
		},
		{
			tool: "cursor",
			want: ".cursor/hooks.json",
			why:  "cursor reads project hooks from .cursor/hooks.json",
		},
		{
			tool: "pi",
			want: "",
			why:  "pi is tailed from its own session log and needs no cooperation",
		},
		{
			tool: "codex",
			want: "",
			why:  "codex is tailed from its rollout transcript and needs no cooperation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			m := NewManager(t.TempDir())
			wt := t.TempDir()

			if err := m.writeActivityConfig(tc.tool, wt, "agent-x"); err != nil {
				t.Fatalf("writeActivityConfig(%s): %v", tc.tool, err)
			}

			if tc.want != "" {
				if !hasFile(t, wt, tc.want) {
					t.Errorf("expected %s (%s)", tc.want, tc.why)
				}
				return
			}

			// Nothing should be written at all — in particular not Claude's
			// settings, which is what used to land in every non-hooks worktree.
			entries, err := os.ReadDir(wt)
			if err != nil {
				t.Fatalf("read worktree: %v", err)
			}
			if len(entries) != 0 {
				var names []string
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Errorf("wrote %v but should write nothing (%s)", names, tc.why)
			}
		})
	}
}

// quietProvider declares no activity signal. Every provider mycel ships reports
// through hooks or a transcript, so this path has no real subject and would
// otherwise go untested — and the thing it must not do is write Claude's hook
// settings into a worktree where nothing reads them.
type quietProvider struct{}

func (quietProvider) Name() string                               { return "quiet" }
func (quietProvider) Description() string                        { return "declares no activity signal" }
func (quietProvider) Command() string                            { return "quiet" }
func (quietProvider) Binary() string                             { return "quiet" }
func (quietProvider) InstallHint() string                        { return "" }
func (quietProvider) BuildCommand(_ provider.CommandOpts) string { return "quiet" }
func (quietProvider) IsInstalled(_ context.Context) bool         { return false }
func (quietProvider) Version(_ context.Context) string           { return "" }
func (quietProvider) ActivityMode() string                       { return provider.ActivityModeNone }
func (quietProvider) WriteHookConfig(_, _, _ string) error       { return nil }
func (quietProvider) TranscriptGlobs(_ string) []string          { return nil }

func TestWriteActivityConfigWritesNothingForANoneModeProvider(t *testing.T) {
	m := NewManager(t.TempDir())
	reg := provider.NewRegistry()
	reg.Register(quietProvider{})
	m.providerRegistry = reg
	wt := t.TempDir()

	if err := m.writeActivityConfig("quiet", wt, "agent-x"); err != nil {
		t.Fatalf("writeActivityConfig: %v", err)
	}
	entries, err := os.ReadDir(wt)
	if err != nil {
		t.Fatalf("read worktree: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("wrote %v for a provider that declares no activity signal", names)
	}
}

// A tool mycel does not know is most often a wrapper around a claude-compatible
// session, so Claude hook settings remain the useful default rather than writing
// nothing and losing all activity.
func TestWriteActivityConfigUnknownToolFallsBackToClaude(t *testing.T) {
	m := NewManager(t.TempDir())
	wt := t.TempDir()

	if err := m.writeActivityConfig("some-unknown-wrapper", wt, "agent-x"); err != nil {
		t.Fatalf("writeActivityConfig: %v", err)
	}
	if !hasFile(t, wt, ".claude/settings.json") {
		t.Error("an unknown tool should still get Claude hook settings")
	}
}

// An agent created before a tool was recorded has no tool name; it must still
// get a working default rather than silently reporting nothing.
func TestWriteActivityConfigEmptyToolFallsBackToClaude(t *testing.T) {
	m := NewManager(t.TempDir())
	wt := t.TempDir()

	if err := m.writeActivityConfig("", wt, "agent-x"); err != nil {
		t.Fatalf("writeActivityConfig: %v", err)
	}
	if !hasFile(t, wt, ".claude/settings.json") {
		t.Error("an empty tool name should fall back to Claude hook settings")
	}
}
