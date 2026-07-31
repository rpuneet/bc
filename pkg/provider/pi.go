package provider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// PiProvider implements the Provider interface for pi CLI.
// pi is an AI coding assistant with read, bash, edit, write tools.
//
// Issue: Add Pi provider support to mycel
// https://github.com/rpuneet/mycel/issues/TODO
//
// pi supports session resumption via --session and --resume flags:
//   - --session <path|id>  Use specific session file or partial UUID
//   - --resume           Select a session to resume
//   - --continue         Continue previous session
//
// Session resumption is tracked through pi's session files in PI_CODING_AGENT_SESSION_DIR.
// Use the full session ID (UUID) to resume a specific session.
type PiProvider struct {
	*GenericAdapter
	name        string
	description string
	command     string
	binary      string
}

func init() { Register(NewPiProvider()) }

// NewPiProvider creates a new pi provider.
func NewPiProvider() *PiProvider {
	return &PiProvider{
		GenericAdapter: NewGenericAdapter("pi"),
		name:           "pi",
		description:    "Pi coding assistant CLI",
		command:        "pi",
		binary:         "pi",
	}
}

// Name returns the provider's unique identifier.
func (p *PiProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *PiProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *PiProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *PiProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *PiProvider) InstallHint() string {
	return "npm install -g @earendil-works/pi-coding-agent"
}

// safePiModelPattern is the charset allowed in a pi model name. pi's model
// identifiers follow the "provider/id" form (e.g. "amazon-bedrock/moonshotai.kimi-k2.5",
// "groq/llama-3.3-70b-versatile"). The charset covers letters, digits, dots,
// hyphens, and slashes — no spaces or shell metacharacters. The first character
// must be alphanumeric to prevent argument injection via a leading dash.
var safePiModelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// SafePiModelName reports whether a pi model name is safe to interpolate
// into a shell command line.
func SafePiModelName(model string) bool {
	return safePiModelPattern.MatchString(model)
}

// BuildCommand returns the full command for a given runtime context.
// When opts.Model is set and passes SafePiModelName, it is appended as flags.
// pi accepts "provider/model" as a single --model value, but also supports
// explicit --provider + --model split, which this function uses when a slash
// is present so the provider routing is unambiguous. Both values are spliced
// into a shell command line — unsafe values are dropped, never escaped.
// When no model is given, pi uses its own configured default — mycel does
// not override that default.
func (p *PiProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.Command()
	if SafePiModelName(opts.Model) {
		if idx := strings.Index(opts.Model, "/"); idx > 0 {
			providerPart := opts.Model[:idx]
			modelPart := opts.Model[idx+1:]
			cmd += " --provider " + providerPart + " --model " + modelPart
		} else {
			cmd += " --model " + opts.Model
		}
	}
	if SafeSessionID(opts.SessionID) {
		cmd += " --session " + opts.SessionID
	}
	if opts.Resume {
		cmd += " --continue"
	}
	return cmd
}

// Models returns an empty static fallback: the live list comes from ListModels
// (DynamicModelLister) which queries `pi --list-models` at runtime. This keeps
// mycel free of baked-in model choices — the user picks from whatever pi reports.
func (p *PiProvider) Models() []string {
	return []string{}
}

// piListModels is overridable in tests.
var piListModels = func(ctx context.Context) (string, error) {
	//nolint:gosec // "pi" is a trusted provider binary name, not user input
	cmd := exec.CommandContext(ctx, "pi", "--list-models")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ListModels queries `pi --list-models` and returns models in "provider/model"
// form. pi prints two-column rows ("provider  model"); this function joins them
// with a slash so the result is a valid BuildCommand input. Only rows whose
// joined form passes SafePiModelName are included — all others are silently
// skipped. When the CLI is unavailable the empty static list is returned.
func (p *PiProvider) ListModels(ctx context.Context) ([]string, error) {
	out, err := piListModels(ctx)
	if err != nil {
		return p.Models(), nil
	}
	var models []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "provider  model" — split on any run of whitespace.
		fields := strings.Fields(line)
		if len(fields) < 2 { //nolint:mnd // 2 is the expected column count, not a magic number
			continue
		}
		combined := fields[0] + "/" + fields[1]
		if !SafePiModelName(combined) {
			continue
		}
		models = append(models, combined)
	}
	// Mirror the error-path fallback: if the CLI exited cleanly but produced no
	// parseable rows, return the static model list rather than nil.
	if len(models) == 0 {
		return p.Models(), nil
	}
	return models, nil
}

// AdjustSessionCommand is a no-op for native tmux sessions: pi runs directly
// inside the mycel-managed tmux pane.
func (p *PiProvider) AdjustSessionCommand(command string) string { return command }

// AdjustContainerCommand wraps the command in a tmux session for Docker so
// mycel can drive it via SendKeys. Double quotes let bash expand
// $MYCEL_WORKTREE_NAME for the session name.
func (p *PiProvider) AdjustContainerCommand(command string) string {
	return fmt.Sprintf(`tmux new-session -s "$MYCEL_WORKTREE_NAME" "%s"`, command)
}

// DockerImage returns empty to use the default image-name convention.
func (p *PiProvider) DockerImage() string { return "" }

// IsInstalled checks if the provider binary is available.
func (p *PiProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// piVersionRe extracts version from pi --version output.
var piVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// Version returns the installed version.
func (p *PiProvider) Version(ctx context.Context) string {
	raw := getBinaryVersion(ctx, p.binary, "--version")
	if m := piVersionRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

// Ensure PiProvider implements all declared interfaces.
var _ Provider = (*PiProvider)(nil)
var _ ModelLister = (*PiProvider)(nil)
var _ DynamicModelLister = (*PiProvider)(nil)
var _ ContainerCustomizer = (*PiProvider)(nil)
var _ SessionCustomizer = (*PiProvider)(nil)
