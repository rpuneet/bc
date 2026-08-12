package provider

import (
	"strings"
	"testing"
)

// Verbatim panes from agents mycel was reporting as healthy (#3512). They are
// quoted rather than paraphrased because the whole mechanism rests on matching
// what these CLIs actually print, and a paraphrase would keep passing after the
// real message changed.
const piNoKeyPane = ` Warning: tmux extended-keys is off. Modified Enter keys may not work.
────────────────────────────────────────────────────────────────────────
 Update Available
 New version 0.83.0 is available. Run pi update
────────────────────────────────────────────────────────────────────────
 Error: No API key found for amazon-bedrock.
 Use /login to log into a provider via OAuth or API key. See:
 /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/providers.md
────────────────────────────────────────────────────────────────────────
~/.mycel/agents/calm-gecko/worktree (detached)
0.0%/197k (auto)                    (amazon-bedrock) minimax.minimax-m2.5 • high`

const piNoModelAccessPane = ` Error: 404 The model ` + "`qwen/qwen3-32b`" + ` does not exist or you do not have access to it.
 Model: meta-llama/llama-4-scout-17b-16e-instruct
 Hi
 Error: 404 The model ` + "`meta-llama/llama-4-scout-17b-16e-instruct`" + ` does not exist or you do not have access to it.
~/.mycel/agents/fierce-osprey/worktree (detached)`

const agyQuotaPane = `⚠ Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 156h34m52s.
Error ID: 08a716f9-72a7-45b4-b26f-b2de551b857f-302
────────────────────────────────────────────────────────────
> hi
? for shortcuts                                    Gemini 3.5 Flash · medium`

// Claude / Cursor / Codex panes mirror the CLIs' own fatal lines (#3687).
const claudeNoKeyPane = `────────────────────────────────────────────────────────────────────────
 API Error: 401 Invalid API key · Please run /login
────────────────────────────────────────────────────────────────────────
 > `

const claudeCreditPane = ` Credit balance is too low
 Add credits at https://console.anthropic.com
 > `

const claudeNotLoggedInPane = ` Not logged in · Please run /login
 > `

const claudeUsageLimitPane = `────────────────────────────────────────────────────────────────────────
 API Error: 429 usage limit reached · resets at 3pm
────────────────────────────────────────────────────────────────────────
 > `

const claudeModelUnavailablePane = ` Claude Opus is not available with the Claude Pro plan. Please use Sonnet or upgrade.
 > `

const cursorAuthPane = `Error: Authentication required. Please run 'agent login' first, or set CURSOR_API_KEY environment variable.
`

const cursorBadKeyPane = `Authentication failed: your Cursor credentials or API key are invalid or expired.
If you set CURSOR_API_KEY, check that it is correct, or run ` + "`agent login`" + ` to re-authenticate.
`

const cursorNotLoggedInPane = `Not logged in. Run ` + "`agent login`" + ` first.
`

const cursorModelLoadPane = `Failed to load models: authentication required or model access denied for this account.
`

//nolint:gosec // G101: quoted CLI error text, not a credential
const codexNoCredsPane = `no Codex credentials were found
Run codex login or provide an API key through a supported auth env var.
`

const codexOutOfCreditsPane = `You're out of credits. Your workspace is out of credits. Add credits to continue using Codex.
`

const codexNoAccessPane = `You do not have access to Codex
This account is not currently authorized to use Codex in this workspace.
`

const codexNotSignedInPane = `Not signed in. Please run 'codex login' to sign in with ChatGPT, or provide an API key.
`

const codexUsageLimitPane = `Usage limit reached. You've reached your usage limit. Try again later or raise the limit.
`

const codexUnknownModelPane = "Unknown model `gpt-imaginary` for spawn_agent. Available models: gpt-5, o3.\n"

// A healthy pane, including one that talks about the very things the patterns
// look for. An agent editing provider code must never be reported as broken.
const workingPane = `● Read pkg/provider/pi.go
● The API key handling looks right. No API key found for is the error string
  we match on, so the test should assert that exact text.
● Also discussing markers like "Authentication required", "credit balance is too low",
  and the phrase "no Codex credentials were found" mid-sentence only.
● Bash(go test ./pkg/provider/)
  ok  github.com/rpuneet/mycel/pkg/provider  0.412s
> `

func TestPiDetectFailure(t *testing.T) {
	p := &PiProvider{}
	tests := []struct {
		name       string
		pane       string
		wantIn     string
		wantFailed bool
	}{
		{"no api key", piNoKeyPane, "no API key", true},
		{"model not entitled", piNoModelAccessPane, "model is unavailable", true},
		{"working agent", workingPane, "", false},
		{"empty pane", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, failed := p.DetectFailure(tt.pane)
			if failed != tt.wantFailed {
				t.Fatalf("failed = %v, want %v (reason %q)", failed, tt.wantFailed, reason)
			}
			if tt.wantIn != "" && !strings.Contains(reason, tt.wantIn) {
				t.Errorf("reason = %q, want it to mention %q", reason, tt.wantIn)
			}
			if !tt.wantFailed && reason != "" {
				t.Errorf("reason = %q, want empty when not failed", reason)
			}
		})
	}
}

func TestAgyDetectFailure(t *testing.T) {
	p := &AgyProvider{}

	reason, failed := p.DetectFailure(agyQuotaPane)
	if !failed {
		t.Fatal("a pane reporting an exhausted quota must be reported as failed")
	}
	if !strings.Contains(reason, "quota") {
		t.Errorf("reason = %q, want it to mention the quota", reason)
	}

	if _, failed := p.DetectFailure(workingPane); failed {
		t.Error("a working agent must not be reported as failed")
	}
}

func TestClaudeDetectFailure(t *testing.T) {
	p := &ClaudeProvider{}
	tests := []struct {
		name       string
		pane       string
		wantIn     string
		wantFailed bool
	}{
		{"invalid api key", claudeNoKeyPane, "API key", true},
		{"credit balance", claudeCreditPane, "credit", true},
		{"not logged in", claudeNotLoggedInPane, "not logged in", true},
		{"usage limit", claudeUsageLimitPane, "usage limit", true},
		{"model unavailable", claudeModelUnavailablePane, "model is unavailable", true},
		{"working agent", workingPane, "", false},
		{"empty pane", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, failed := p.DetectFailure(tt.pane)
			if failed != tt.wantFailed {
				t.Fatalf("failed = %v, want %v (reason %q)", failed, tt.wantFailed, reason)
			}
			if tt.wantIn != "" && !strings.Contains(reason, tt.wantIn) {
				t.Errorf("reason = %q, want it to mention %q", reason, tt.wantIn)
			}
			if !tt.wantFailed && reason != "" {
				t.Errorf("reason = %q, want empty when not failed", reason)
			}
		})
	}
}

func TestCursorDetectFailure(t *testing.T) {
	p := &CursorProvider{}
	tests := []struct {
		name       string
		pane       string
		wantIn     string
		wantFailed bool
	}{
		{"auth required", cursorAuthPane, "not authenticated", true},
		{"bad api key", cursorBadKeyPane, "API key", true},
		{"not logged in", cursorNotLoggedInPane, "not logged in", true},
		{"failed to load models", cursorModelLoadPane, "could not load models", true},
		{"working agent", workingPane, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, failed := p.DetectFailure(tt.pane)
			if failed != tt.wantFailed {
				t.Fatalf("failed = %v, want %v (reason %q)", failed, tt.wantFailed, reason)
			}
			if tt.wantIn != "" && !strings.Contains(reason, tt.wantIn) {
				t.Errorf("reason = %q, want it to mention %q", reason, tt.wantIn)
			}
		})
	}
}

func TestCodexDetectFailure(t *testing.T) {
	p := &CodexProvider{}
	tests := []struct {
		name       string
		pane       string
		wantIn     string
		wantFailed bool
	}{
		{"no credentials", codexNoCredsPane, "credentials", true},
		{"out of credits", codexOutOfCreditsPane, "credits", true},
		{"no access", codexNoAccessPane, "cannot use Codex", true},
		{"not signed in", codexNotSignedInPane, "not signed in", true},
		{"usage limit", codexUsageLimitPane, "usage limit", true},
		{"unknown model", codexUnknownModelPane, "model is unavailable", true},
		{"working agent", workingPane, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, failed := p.DetectFailure(tt.pane)
			if failed != tt.wantFailed {
				t.Fatalf("failed = %v, want %v (reason %q)", failed, tt.wantFailed, reason)
			}
			if tt.wantIn != "" && !strings.Contains(reason, tt.wantIn) {
				t.Errorf("reason = %q, want it to mention %q", reason, tt.wantIn)
			}
		})
	}
}

func TestMatchFailureReadsOnlyTheTail(t *testing.T) {
	// A pane holds scrollback. An error from an earlier session that has since
	// recovered must scroll out of reach, or a healthy agent is reported broken
	// for the rest of its life.
	patterns := []FailurePattern{{Contains: "no api key found for", Reason: "no key"}}
	old := "Error: No API key found for amazon-bedrock.\n" + strings.Repeat("● working fine\n", 40)

	if _, failed := MatchFailure(old, 25, patterns); failed {
		t.Error("an error above the tail window must not count as a current failure")
	}
	if _, failed := MatchFailure(old, 0, patterns); !failed {
		t.Error("tailLines <= 0 must search the whole pane")
	}
}

func TestMatchFailureRequiresTheProvidersOwnErrorMarker(t *testing.T) {
	// This is what keeps an agent that talks about failures apart from a
	// provider that has one. Both lines contain the fragment; only the
	// provider's starts with its error marker.
	patterns := []FailurePattern{{
		LineStartsWith: "error:",
		Contains:       "no api key found for",
		Reason:         "no key",
	}}

	agentProse := "● The message is \"No API key found for amazon-bedrock\", which we match on."
	if reason, failed := MatchFailure(agentProse, 25, patterns); failed {
		t.Errorf("agent prose reported as a provider failure, reason = %q", reason)
	}

	providerError := " Error: No API key found for amazon-bedrock."
	if _, failed := MatchFailure(providerError, 25, patterns); !failed {
		t.Error("the provider's own error line must match despite its indentation")
	}
}

func TestMatchFailureNeedsMarkerAndFragmentOnOneLine(t *testing.T) {
	// A marker on one line and the fragment on another is two unrelated pieces
	// of output, not a failure report.
	patterns := []FailurePattern{{
		LineStartsWith: "error:",
		Contains:       "quota reached",
		Reason:         "spent",
	}}
	split := "Error: something else entirely\n● the quota reached its limit last week"
	if reason, failed := MatchFailure(split, 25, patterns); failed {
		t.Errorf("marker and fragment on separate lines matched, reason = %q", reason)
	}
}

func TestDetectFailureSeesThroughTerminalColour(t *testing.T) {
	// Captured verbatim from GET /api/agents/calm-gecko/peek, escape sequences
	// and all. This is what a detector is actually handed — the first attempt at
	// this feature matched nothing in production while every unit test passed,
	// because the fixtures were hand-typed plain text and the real error line
	// starts with a color sequence rather than "Error:".
	pane := " \x1b[38;2;204;102;102mError: No API key found for amazon-bedrock." +
		"                                               \n" +
		"\x1b[2K \x1b[38;2;204;102;102mUse /login to log into a provider via OAuth or API key. See:\x1b[0m\x1b]8;;file:///docs\x07\n" +
		"\x1b[2K\x1b[38;2;102;102;102m0.0%/197k (auto)\x1b[39m (amazon-bedrock) minimax.minimax-m2.5\n"

	reason, failed := (&PiProvider{}).DetectFailure(pane)
	if !failed {
		t.Fatal("a colored provider error must be detected — panes are never plain text")
	}
	if !strings.Contains(reason, "no API key") {
		t.Errorf("reason = %q, want it to mention the missing key", reason)
	}
}

func TestStripANSIKeepsTextAndLines(t *testing.T) {
	in := "\x1b[2K \x1b[38;2;204;102;102mError: boom\x1b[0m\n\x1b[38;5;9msecond line\x1b[m"
	want := " Error: boom\nsecond line"
	if got := StripANSI(in); got != want {
		t.Errorf("StripANSI = %q, want %q", got, want)
	}
}

func TestStripANSILeavesPlainTextUntouched(t *testing.T) {
	in := "Error: No API key found for amazon-bedrock.\n> "
	if got := StripANSI(in); got != in {
		t.Errorf("StripANSI altered plain text: %q", got)
	}
}

func TestStripANSIKeepsLineCountWithUnterminatedSequence(t *testing.T) {
	// A pane is a fixed-width window, so it can cut a sequence in half. Losing a
	// newline there would join two lines and let a marker on one match a
	// fragment on the next.
	in := "Error: boom\n\x1b]8;;http://truncated"
	if got := strings.Count(StripANSI(in), "\n"); got != 1 {
		t.Errorf("newline count = %d, want 1", got)
	}
}

func TestMatchFailureIsCaseInsensitive(t *testing.T) {
	patterns := []FailurePattern{{Contains: "Quota Reached", Reason: "spent"}}
	if _, failed := MatchFailure("⚠ individual QUOTA reached.", 25, patterns); !failed {
		t.Error("matching must not depend on the casing a provider happens to use")
	}
}

func TestMatchFailureSkipsEmptyPatterns(t *testing.T) {
	// An empty Contains would match every pane and report every agent broken.
	patterns := []FailurePattern{{Contains: "", Reason: "should never fire"}}
	if reason, failed := MatchFailure(workingPane, 25, patterns); failed {
		t.Errorf("empty pattern matched, reason = %q", reason)
	}
}

// The detectors must be reachable through the capability interface, since that
// is how the daemon finds them.
func TestProvidersImplementFailureDetector(t *testing.T) {
	for _, tt := range []struct {
		p    Provider
		name string
	}{
		{&PiProvider{}, "pi"},
		{&AgyProvider{}, "agy"},
		{&ClaudeProvider{}, "claude"},
		{&CursorProvider{}, "cursor"},
		{&CodexProvider{}, "codex"},
	} {
		if _, ok := tt.p.(FailureDetector); !ok {
			t.Errorf("%s does not implement FailureDetector", tt.name)
		}
	}
}
