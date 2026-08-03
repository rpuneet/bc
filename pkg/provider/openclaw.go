package provider

import (
	"context"
	"fmt"
	"regexp"
)

// OpenclawProvider implements the Provider interface for the OpenClaw CLI.
// OpenClaw is an open-source personal AI assistant that exposes an
// interactive terminal agent via `openclaw tui`.
//
// Interactive invocation: mycel starts OpenClaw in local mode
//
//	openclaw tui --local
//
// which runs against the embedded agent runtime instead of a Gateway, so an
// agent works on the same machine without requiring a running OpenClaw
// gateway/daemon. See https://docs.openclaw.ai/cli/tui and
// https://docs.openclaw.ai/web/tui.
//
// Session resumption uses OpenClaw's `--session <key>` flag: when a concrete
// session key is supplied the TUI reattaches to that session. OpenClaw has no
// separate "continue last session" flag for local mode (a bare `openclaw tui`
// auto-resumes the last session for the same scope), so a Resume request
// without a session key emits no extra flag — the default behavior already
// resumes.
//
// Model selection is intentionally NOT exposed as a command flag: `openclaw
// tui` has no `--model` flag. OpenClaw selects the model via its in-TUI model
// picker (Ctrl+L / `/model <provider/model>`) or per-agent routing configured
// with `openclaw agents add --model`. mycel does not invent a model flag, so
// OpenclawProvider deliberately does not implement ModelLister.
type OpenclawProvider struct {
	*GenericAdapter
	name        string
	description string
	command     string
	binary      string
}

func init() { Register(NewOpenclawProvider()) }

// NewOpenclawProvider creates a new OpenClaw provider.
func NewOpenclawProvider() *OpenclawProvider {
	return &OpenclawProvider{
		GenericAdapter: NewGenericAdapter("openclaw"),
		name:           "openclaw",
		description:    "OpenClaw personal AI assistant CLI",
		command:        "openclaw tui --local",
		binary:         "openclaw",
	}
}

// Name returns the provider's unique identifier.
func (p *OpenclawProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *OpenclawProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *OpenclawProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *OpenclawProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *OpenclawProvider) InstallHint() string {
	return "npm install -g openclaw"
}

// BuildCommand returns the full command for a given runtime context.
// Base command is `openclaw tui --local` (embedded agent runtime). When a
// concrete session ID is supplied and passes SafeSessionID, it is appended as
// `--session <id>` to reattach to that session. The value is spliced into a
// shell command line, so unsafe IDs are dropped, never escaped.
//
// opts.Model is ignored: `openclaw tui` has no model flag (model is chosen in
// the TUI or via per-agent routing), so mycel does not fabricate one.
//
// opts.Resume without a session ID emits no flag: a bare `openclaw tui`
// already auto-resumes the last session for the same scope.
func (p *OpenclawProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeSessionID(opts.SessionID) {
		cmd += " --session " + opts.SessionID
	}
	return cmd
}

// AdjustSessionCommand is a no-op for native tmux sessions: OpenClaw's TUI runs
// directly inside the mycel-managed tmux session.
func (p *OpenclawProvider) AdjustSessionCommand(command string) string {
	return command
}

// AdjustContainerCommand wraps the command in a tmux session for Docker so
// mycel can drive the TUI via SendKeys. Double quotes let bash expand
// $MYCEL_WORKTREE_NAME for the session name.
func (p *OpenclawProvider) AdjustContainerCommand(command string) string {
	return fmt.Sprintf(`tmux new-session -s "$MYCEL_WORKTREE_NAME" "%s"`, command)
}

// DockerImage returns empty to use the default image-name convention
// (mycel-agent-openclaw:latest).
func (p *OpenclawProvider) DockerImage() string { return "" }

// IsInstalled checks if the provider binary is available.
func (p *OpenclawProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// openclawVersionRe extracts a semver-like version from `openclaw --version`
// output. OpenClaw uses date-based versions (e.g. "2026.6.11"), which this
// three-segment pattern matches.
var openclawVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// Version returns the installed version.
func (p *OpenclawProvider) Version(ctx context.Context) string {
	raw := getBinaryVersion(ctx, p.binary, "--version")
	if m := openclawVersionRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

// ActivityMode reports that OpenClaw exposes no activity signal mycel can
// attribute to an agent, so agent state is never derived from its output.
//
// Neither of the two mechanisms mycel supports is available. OpenClaw's `hooks`
// command manages plugin "hook packs" that extend the assistant, not
// per-invocation lifecycle callbacks a host can register, so there is nothing to
// push. And its transcripts are not per-worktree: sessions live in one global
// store (~/.openclaw/agents/<agent>/sessions) keyed by OpenClaw's own agent and
// chat identity rather than the working directory, so two mycel agents running
// `openclaw tui` would read each other's session file. Tailing it would
// attribute activity to the wrong agent, which is worse than showing none.
//
// Declaring the mode explicitly is what makes the UI honest: it tells the Live
// tab to say capture is unavailable instead of waiting for events that will
// never arrive, and it stops mycel writing Claude hook settings into an OpenClaw
// worktree where nothing would ever read them.
func (p *OpenclawProvider) ActivityMode() string { return ActivityModeNone }

// WriteHookConfig is a no-op: OpenClaw has no lifecycle hook mechanism
// (ActivityModeNone).
func (p *OpenclawProvider) WriteHookConfig(_, _, _ string) error { return nil }

// TranscriptGlobs returns nil: OpenClaw's session store is global rather than
// per-worktree, so no glob can identify one mycel agent's transcript.
func (p *OpenclawProvider) TranscriptGlobs(_ string) []string { return nil }

// Ensure OpenclawProvider implements all declared interfaces.
var _ Provider = (*OpenclawProvider)(nil)
var _ ActivitySource = (*OpenclawProvider)(nil)
var _ ContainerCustomizer = (*OpenclawProvider)(nil)
var _ SessionCustomizer = (*OpenclawProvider)(nil)
