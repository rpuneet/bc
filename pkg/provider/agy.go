package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AgyProvider implements the Provider interface for the Antigravity CLI
// (`agy`), Google's terminal agent running the Gemini 3.x family. It matches
// the Claude provider's integration depth: model selection, resume by
// conversation ID or bare continue, dynamic + static model listing,
// resumable-session detection, and hook-based activity reporting.
type AgyProvider struct {
	AgyConfigAdapter // embeds ConfigAdapter implementation (.agents layout)
	name             string
	description      string
	command          string
	binary           string
}

// NewAgyProvider creates a new Antigravity CLI provider.
func NewAgyProvider() *AgyProvider {
	return &AgyProvider{
		name:        "agy",
		description: "Google Antigravity CLI (Gemini)",
		command:     "agy --dangerously-skip-permissions",
		binary:      "agy",
	}
}

// Name returns the provider's unique identifier.
func (p *AgyProvider) Name() string { return p.name }

// Description returns a human-readable description.
func (p *AgyProvider) Description() string { return p.description }

// Command returns the shell command to start this provider.
func (p *AgyProvider) Command() string { return p.command }

// Binary returns the executable name for LookPath/version checks.
func (p *AgyProvider) Binary() string { return p.binary }

// InstallHint returns a human-readable install instruction.
func (p *AgyProvider) InstallHint() string {
	return "curl -fsSL https://antigravity.google/install.sh | sh"
}

// agyDefaultModel is the model bc selects for a new agy agent when none is
// specified. It is a valid `agy models` entry.
const agyDefaultModel = "Gemini 3 Flash"

// agySessionIDPattern is the full-string UUID shape of an agy conversation
// ID (agy stores conversations as <uuid>.db). The ID is spliced into a shell
// command line, so anything else — including an empty string — is rejected
// rather than quoted.
var agySessionIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// safeAgyModelPattern is the charset allowed in an agy model name. Unlike
// SafeModelName (which rejects spaces and parentheses), agy's real model
// identifiers contain both — e.g. "Gemini 3.5 Flash (High)". The charset is
// deliberately limited to letters, digits, spaces, dots, parentheses and
// dashes: none of these are shell metacharacters that survive single-quoting,
// so BuildCommand can splice a single-quoted model value in safely. The first
// character must be alphanumeric so the value can never be parsed as a flag
// (argument injection). Note: the single quote itself is NOT in the set, so a
// value can never break out of the surrounding quotes.
var safeAgyModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .()-]*$`)

// SafeAgyModelName reports whether an agy model name is safe to single-quote
// into a shell command line.
func SafeAgyModelName(model string) bool {
	return safeAgyModelPattern.MatchString(model)
}

// shellSingleQuote wraps s in single quotes for safe interpolation into a
// shell command line, escaping any embedded single quote via the standard
// '\” idiom. Callers must still gate the value through SafeAgyModelName; the
// escape here is defense in depth.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildCommand returns the full command for a given runtime context.
// --dangerously-skip-permissions makes agy approve tool use autonomously.
// Model priority: opts.Model is injected as `--model '<m>'` when it passes
// SafeAgyModelName; the value is single-quoted (agy models contain spaces and
// parentheses). Unsafe values are dropped, never partially escaped.
// Resume priority mirrors Claude: SessionID (--conversation <uuid>) > Resume
// flag (--continue).
func (p *AgyProvider) BuildCommand(opts CommandOpts) string {
	cmd := "agy --dangerously-skip-permissions"
	if SafeAgyModelName(opts.Model) {
		cmd += " --model " + shellSingleQuote(opts.Model)
	}
	switch {
	case agySessionIDPattern.MatchString(opts.SessionID):
		cmd += " --conversation " + opts.SessionID
	case opts.Resume:
		cmd += " --continue"
	}
	return cmd
}

// Models returns the curated static model list for the Antigravity CLI. This
// is the fallback used when `agy models` cannot be queried at runtime; the
// live list is preferred via ListModels (DynamicModelLister).
func (p *AgyProvider) Models() []string {
	return []string{
		"Gemini 3.5 Flash (Medium)",
		"Gemini 3.5 Flash (High)",
		"Gemini 3.5 Flash (Low)",
		"Gemini 3.1 Pro (Low)",
		"Gemini 3.1 Pro (High)",
		"Gemini 3 Flash",
	}
}

// agyListModels is overridable in tests.
var agyListModels = func(ctx context.Context) (string, error) {
	out, err := runProviderCommand(ctx, "agy", "models")
	return out, err
}

// ListModels enumerates agy's models at runtime by shelling `agy models` and
// parsing one model per line. Only lines that pass SafeAgyModelName are kept
// (so the UI never offers an unusable value). Falls back to the static
// Models() list when the CLI is unavailable or returns nothing usable.
func (p *AgyProvider) ListModels(ctx context.Context) ([]string, error) {
	out, err := agyListModels(ctx)
	if err != nil {
		return p.Models(), nil
	}
	var models []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !SafeAgyModelName(line) {
			continue
		}
		models = append(models, line)
	}
	if len(models) == 0 {
		return p.Models(), nil
	}
	return models, nil
}

// AdjustSessionCommand is a no-op for native tmux sessions; agy runs directly
// in the bc-managed tmux pane.
func (p *AgyProvider) AdjustSessionCommand(command string) string { return command }

// AdjustContainerCommand wraps the command in a tmux session for Docker so
// SendKeys works, matching the Claude provider. Double quotes let bash expand
// $MYCEL_WORKTREE_NAME for the session name; the agy command's own single
// quotes (around --model) survive as literals and are honored by the inner
// shell.
func (p *AgyProvider) AdjustContainerCommand(command string) string {
	return fmt.Sprintf(`tmux new-session -s "$MYCEL_WORKTREE_NAME" "%s"`, command)
}

// DockerImage returns empty to use the default image convention.
func (p *AgyProvider) DockerImage() string { return "" }

// IsInstalled checks if the provider binary is available.
func (p *AgyProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// Version returns the installed version.
func (p *AgyProvider) Version(ctx context.Context) string {
	return getBinaryVersion(ctx, p.binary, "--version")
}

// SupportsResume reports that agy can resume a specific conversation by ID
// (agy --conversation <uuid>).
func (p *AgyProvider) SupportsResume() bool { return true }

// agyResumePattern matches an agy resume hint of the form
// "agy --conversation <uuid>" that agy may print on graceful exit.
var agyResumePattern = regexp.MustCompile(`agy --conversation ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// ParseSessionID scans tool output for agy's resume hint and returns the
// conversation UUID, or "" if none is found.
func (p *AgyProvider) ParseSessionID(output string) string {
	m := agyResumePattern.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// agyHomeDir is overridable in tests.
var agyHomeDir = os.UserHomeDir

// agyConversationsDir returns the directory where the agy CLI stores its
// conversation databases.
func agyConversationsDir(home string) string {
	return filepath.Join(home, ".gemini", "antigravity", "conversations")
}

// HasResumableSession reports whether agy has at least one stored conversation
// that `agy --continue` could pick up. agy's --continue resumes the most
// recent conversation; when no conversation exists it has nothing to continue,
// so bc must not emit the flag (mirroring the Claude provider's gate). agy
// keeps conversations globally as ~/.gemini/antigravity/conversations/<uuid>.db
// (not keyed by working directory), so this is a global has-any check.
func (p *AgyProvider) HasResumableSession(_ string) bool {
	home, err := agyHomeDir()
	if err != nil {
		return false
	}
	entries, err := os.ReadDir(agyConversationsDir(home))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
			return true
		}
	}
	return false
}

// ActivityMode reports that agy emits activity via lifecycle hooks
// (.agents/hooks.json) that POST to bcd's hook endpoint, exactly like Claude.
func (p *AgyProvider) ActivityMode() string { return ActivityModeHooks }

// WriteHookConfig writes agy lifecycle-hook settings into the agent worktree.
// daemonAddr and agentID are unused: the generated hook commands resolve the
// daemon address and agent identity at runtime via the MYCEL_DAEMON_ADDR and
// MYCEL_AGENT_ID environment variables set on the agent session.
func (p *AgyProvider) WriteHookConfig(worktreeDir, _, _ string) error {
	return WriteAgyHookSettings(worktreeDir)
}

// TranscriptGlobs returns glob patterns matching the agy CLI's session
// transcript for an agent working in cwd. The CLI writes its transcript to
// <cwd>/.gemini/antigravity-cli/transcript.jsonl.
func (p *AgyProvider) TranscriptGlobs(cwd string) []string {
	if cwd == "" {
		return nil
	}
	return []string{filepath.Join(cwd, ".gemini", "antigravity-cli", "transcript.jsonl")}
}

// Ensure AgyProvider implements all declared interfaces.
var _ Provider = (*AgyProvider)(nil)
var _ ModelLister = (*AgyProvider)(nil)
var _ DynamicModelLister = (*AgyProvider)(nil)
var _ ContainerCustomizer = (*AgyProvider)(nil)
var _ SessionCustomizer = (*AgyProvider)(nil)
var _ SessionResumer = (*AgyProvider)(nil)
var _ ResumableSessionDetector = (*AgyProvider)(nil)
var _ ActivitySource = (*AgyProvider)(nil)
