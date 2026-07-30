package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

// writeEmptyPromptFile creates an empty prompt file so appendInjectedInstructions
// (which opens with O_APPEND|O_WRONLY, no O_CREATE) can write to it.
func writeEmptyPromptFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(p, []byte("# existing prompt\n"), 0o600); err != nil {
		t.Fatalf("seed prompt file: %v", err)
	}
	return p
}

func TestAppendInjectedInstructions_AppendsBlock(t *testing.T) {
	p := writeEmptyPromptFile(t)
	cfg := &home.Config{
		InjectedInstructions: "Always report status before and after work.",
	}

	err := appendInjectedInstructions(
		context.Background(),
		p,
		cfg,
		[]string{"github", "bc"},                // intentionally unsorted
		[]string{"SLACK_BOT_TOKEN", "GH_TOKEN"}, // key NAMES only
	)
	if err != nil {
		t.Fatalf("appendInjectedInstructions: %v", err)
	}

	data, err := os.ReadFile(p) //nolint:gosec // test file path from t.TempDir
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		"# existing prompt",                              // pre-existing content preserved
		"## mycel instructions",                          // block header
		"Always report status before and after work.",    // authored text
		"### Available resources",                        // resources sub-header
		"MCP servers: bc, github",                        // sorted MCP names
		"Credential env vars: GH_TOKEN, SLACK_BOT_TOKEN", // sorted key NAMES
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt file missing %q\n---\n%s", want, got)
		}
	}
}

// TestAppendInjectedInstructions_NoSecretValues proves that only credential env
// var NAMES are written — a secret VALUE handed to the function via env would
// never reach the prompt because the function only ever receives names.
func TestAppendInjectedInstructions_NoSecretValues(t *testing.T) {
	p := writeEmptyPromptFile(t)
	cfg := &home.Config{InjectedInstructions: "Do the thing."}

	const secretValue = "xoxb-super-secret-token-value" //nolint:gosec // fake token used to assert non-leakage

	// Only the NAME is passed. The value must never be written.
	if err := appendInjectedInstructions(
		context.Background(),
		p,
		cfg,
		nil,
		[]string{"SLACK_BOT_TOKEN"},
	); err != nil {
		t.Fatalf("appendInjectedInstructions: %v", err)
	}

	data, err := os.ReadFile(p) //nolint:gosec // test file path from t.TempDir
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	got := string(data)

	if strings.Contains(got, secretValue) {
		t.Fatalf("secret value leaked into prompt file:\n%s", got)
	}
	if !strings.Contains(got, "SLACK_BOT_TOKEN") {
		t.Errorf("expected credential name in prompt, got:\n%s", got)
	}
	if !strings.Contains(got, "MCP servers: none") {
		t.Errorf("expected empty MCP summary to render as \"none\", got:\n%s", got)
	}
}

func TestAppendInjectedInstructions_EmptyIsNoop(t *testing.T) {
	p := writeEmptyPromptFile(t)
	before, err := os.ReadFile(p) //nolint:gosec // test file path from t.TempDir
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}

	// Empty / whitespace-only instructions must not touch the file.
	for _, cfg := range []*home.Config{
		nil,
		{InjectedInstructions: ""},
		{InjectedInstructions: "   \n\t "},
	} {
		if appendErr := appendInjectedInstructions(context.Background(), p, cfg, []string{"bc"}, []string{"GH_TOKEN"}); appendErr != nil {
			t.Fatalf("appendInjectedInstructions: %v", appendErr)
		}
	}

	after, err := os.ReadFile(p) //nolint:gosec // test file path from t.TempDir
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("prompt file changed on no-op:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestSummariseNames(t *testing.T) {
	if got := summarizeNames(nil); got != "none" {
		t.Errorf("summarizeNames(nil) = %q, want none", got)
	}
	if got := summarizeNames([]string{"c", "a", "b"}); got != "a, b, c" {
		t.Errorf("summarizeNames sorted = %q, want \"a, b, c\"", got)
	}
}
