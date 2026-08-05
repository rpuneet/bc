package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AgyProvider implements the Provider interface for the Antigravity CLI
// (`agy`), Google's terminal agent running the Gemini 3.x family. It matches
// the Claude provider's integration depth: model selection, resume by
// conversation ID or bare continue, dynamic + static model listing,
// resumable-session detection, and activity reporting via session transcript.
type AgyProvider struct {
	AgyConfigAdapter // embeds ConfigAdapter implementation (.agents layout)
	name             string
	description      string
	command          string
	binary           string
}

func init() { Register(NewAgyProvider()) }

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

// agyDefaultModel is the model mycel selects for a new agy agent when none is
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
// '\' idiom. Callers must still gate the value through SafeAgyModelName; the
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
// in the mycel-managed tmux pane.
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
// so mycel must not emit the flag (mirroring the Claude provider's gate). agy
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

// ActivityMode reports that agy exposes activity via an on-disk session
// transcript the daemon tails. This is ActivityModeTranscript (not hooks)
// because agy's hooks do not pipe JSON payload on stdin as documented.
//
// Fixes #3531: agy's hooks fire for state transitions but provide no payload,
// resulting in agents showing state but no prompt/tool details. The transcript
// approach allows full activity reporting including prompts and tool usage.
func (p *AgyProvider) ActivityMode() string { return ActivityModeTranscript }

// WriteHookConfig is a no-op for agy: activity is sourced by tailing its
// session transcript, not via hooks.
func (p *AgyProvider) WriteHookConfig(_, _, _ string) error { return nil }

// agyTranscriptsRoot resolves agy's transcript root directory. agy stores
// session transcripts under ~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/
var agyTranscriptsRoot = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "antigravity-cli", "brain")
}

// TranscriptGlobs returns glob patterns matching agy's session transcript files.
// agy writes transcripts to ~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/transcript.jsonl
// The cwd parameter is ignored because agy stores transcripts in a global location,
// not per-worktree. Returns nil if cwd is empty.
func (p *AgyProvider) TranscriptGlobs(cwd string) []string {
	if cwd == "" {
		return nil
	}
	root := agyTranscriptsRoot()
	if root == "" {
		return nil
	}
	return []string{filepath.Join(root, "*", ".system_generated", "logs", "transcript.jsonl")}
}

// agyTranscriptLine is one JSONL entry in an agy transcript file.
type agyTranscriptLine struct {
	StepIndex int           `json:"step_index"`
	Source    string        `json:"source"`
	Type      string        `json:"type"`
	Status    string        `json:"status"`
	CreatedAt string        `json:"created_at"`
	Content   string        `json:"content,omitempty"`
	Thinking  string        `json:"thinking,omitempty"`
	ToolCalls []agyToolCall `json:"tool_calls,omitempty"`
	Error     string        `json:"error,omitempty"`
	ErrorCode int           `json:"error_code,omitempty"`
}

// agyToolCall represents a tool call in agy's transcript.
type agyToolCall struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

// ParseTranscriptLine turns one agy JSONL line into zero or more activity
// events. Unrecognized or malformed lines yield (nil, nil).
func (p *AgyProvider) ParseTranscriptLine(line []byte) ([]TranscriptActivity, error) {
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return nil, nil
	}

	var entry agyTranscriptLine
	if err := json.Unmarshal(line, &entry); err != nil {
		return nil, nil // tolerate malformed lines mid-stream
	}

	ts, _ := time.Parse(time.RFC3339, entry.CreatedAt)

	switch entry.Type {
	case "USER_INPUT":
		// Extract prompt from content - strip USER_REQUEST wrapper
		prompt := extractAgyPrompt(entry.Content)
		if prompt == "" {
			return nil, nil
		}
		return []TranscriptActivity{{
			Event:     "UserPromptSubmit",
			Prompt:    prompt,
			Timestamp: ts,
		}}, nil

	case "PLANNER_RESPONSE":
		// Model planning response may include tool calls
		if len(entry.ToolCalls) == 0 {
			return nil, nil
		}
		var activities []TranscriptActivity
		for _, tc := range entry.ToolCalls {
			if tc.Name != "" {
				activities = append(activities, TranscriptActivity{
					Event:     "PreToolUse",
					ToolName:  tc.Name,
					ToolInput: tc.Args,
					Timestamp: ts,
				})
			}
		}
		return activities, nil

	case "RUN_COMMAND":
		// Tool execution results - skip as we can't accurately map PostToolUse
		// without state tracking
		return nil, nil

	default:
		return nil, nil
	}
}

// extractAgyPrompt extracts the user prompt from agy's USER_INPUT content.
// agy wraps prompts in <USER_REQUEST> tags, so we extract the inner content.
func extractAgyPrompt(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	// Extract content between <USER_REQUEST> and </USER_REQUEST>
	if start := strings.Index(content, "<USER_REQUEST>"); start != -1 {
		if end := strings.Index(content[start:], "</USER_REQUEST>"); end != -1 {
			content = content[start+len("<USER_REQUEST>") : start+end]
		}
	}

	// Strip <ADDITIONAL_METADATA> if present
	if metaStart := strings.Index(content, "<ADDITIONAL_METADATA>"); metaStart != -1 {
		content = strings.TrimSpace(content[:metaStart])
	}

	return strings.TrimSpace(content)
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
var _ TranscriptParser = (*AgyProvider)(nil)
