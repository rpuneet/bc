package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// CodexProvider implements the Provider interface for OpenAI Codex CLI.
// Codex is OpenAI's code generation model.
//
// Issue #1479: Codex CLI Provider Integration
type CodexProvider struct {
	*GenericAdapter
	name        string
	description string
	command     string
	binary      string
}

func init() { Register(NewCodexProvider()) }

// NewCodexProvider creates a new Codex provider.
func NewCodexProvider() *CodexProvider {
	return &CodexProvider{
		GenericAdapter: NewGenericAdapter("codex"),
		name:           "codex",
		description:    "OpenAI Codex CLI",
		command:        "codex --full-auto",
		binary:         "codex",
	}
}

// Name returns the provider's unique identifier.
func (p *CodexProvider) Name() string {
	return p.name
}

// Description returns a human-readable description.
func (p *CodexProvider) Description() string {
	return p.description
}

// Command returns the shell command to start this provider.
func (p *CodexProvider) Command() string {
	return p.command
}

// Binary returns the executable name for LookPath/version checks.
func (p *CodexProvider) Binary() string {
	return p.binary
}

// InstallHint returns a human-readable install instruction.
func (p *CodexProvider) InstallHint() string {
	return "npm install -g @openai/codex"
}

// BuildCommand returns the full command for a given runtime context.
// Supports --model <m> for model selection; the value is spliced into a
// shell command line, so unsafe values are dropped.
//
// Resume is intentionally NOT wired: `codex --full-auto` starts a fresh
// autonomous session and exposes no verified session-id flag for this exec
// mode (resume is a separate interactive `codex resume` subcommand). Rather
// than emit an unverified flag, CodexProvider does not implement
// SessionResumer, and opts.SessionID/opts.Resume are ignored here.
func (p *CodexProvider) BuildCommand(opts CommandOpts) string {
	cmd := p.command
	if SafeModelName(opts.Model) {
		cmd += " --model " + opts.Model
	}
	return cmd
}

// AdjustSessionCommand is a no-op for native tmux sessions: codex runs directly
// inside the mycel-managed tmux pane.
func (p *CodexProvider) AdjustSessionCommand(command string) string { return command }

// AdjustContainerCommand wraps the command in a tmux session for Docker so
// mycel can drive it via SendKeys. Double quotes let bash expand
// $MYCEL_WORKTREE_NAME for the session name.
func (p *CodexProvider) AdjustContainerCommand(command string) string {
	return fmt.Sprintf(`tmux new-session -s "$MYCEL_WORKTREE_NAME" "%s"`, command)
}

// DockerImage returns empty to use the default image-name convention.
func (p *CodexProvider) DockerImage() string { return "" }

// Models returns a small static fallback for the Codex CLI. Live catalogs
// come from ListModels (`codex debug models`).
func (p *CodexProvider) Models() []string {
	return []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.2"}
}

// codexListModels is overridable in tests.
var codexListModels = func(ctx context.Context) (string, error) {
	return runProviderCommand(ctx, "codex", "debug", "models")
}

// ListModels returns slugs from `codex debug models` (JSON catalog). Prefers
// visibility=list entries; falls back to the static Models() list.
func (p *CodexProvider) ListModels(ctx context.Context) ([]string, error) {
	out, err := codexListModels(ctx)
	if err != nil {
		return p.Models(), nil
	}
	models := parseCodexDebugModels(out)
	if len(models) == 0 {
		return p.Models(), nil
	}
	return models, nil
}

// parseCodexDebugModels extracts usable --model slugs from the JSON blob
// printed by `codex debug models`.
func parseCodexDebugModels(out string) []string {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	// Tolerate leading/trailing log noise by locating the JSON object.
	start := strings.Index(out, "{")
	end := strings.LastIndex(out, "}")
	if start < 0 || end <= start {
		return nil
	}
	var catalog struct {
		Models []struct {
			Slug       string `json:"slug"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if err := json.Unmarshal([]byte(out[start:end+1]), &catalog); err != nil {
		return nil
	}
	var listed, all []string
	seen := map[string]bool{}
	for _, m := range catalog.Models {
		slug := strings.TrimSpace(m.Slug)
		if slug == "" || !SafeModelName(slug) || seen[slug] {
			continue
		}
		seen[slug] = true
		all = append(all, slug)
		if m.Visibility == "" || m.Visibility == "list" {
			listed = append(listed, slug)
		}
	}
	if len(listed) > 0 {
		return listed
	}
	return all
}

// IsInstalled checks if the provider binary is available.
func (p *CodexProvider) IsInstalled(ctx context.Context) bool {
	return checkBinaryExists(ctx, p.binary)
}

// codexVersionRe extracts a semver-like version from codex --version output
// which may look like "codex-cli 0.111.0" or "v0.111.0".
var codexVersionRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// Version returns the installed version, stripped of any prefix like "codex-cli".
func (p *CodexProvider) Version(ctx context.Context) string {
	raw := getBinaryVersion(ctx, p.binary, "--version")
	if m := codexVersionRe.FindString(raw); m != "" {
		return m
	}
	return raw
}

// Ensure CodexProvider implements Provider interface.
var _ Provider = (*CodexProvider)(nil)
var _ ModelLister = (*CodexProvider)(nil)
var _ DynamicModelLister = (*CodexProvider)(nil)
var _ ContainerCustomizer = (*CodexProvider)(nil)
var _ SessionCustomizer = (*CodexProvider)(nil)
