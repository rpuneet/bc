package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

// writeEmptyPromptFile creates a seeded prompt file for append/sync tests.
func writeEmptyPromptFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(p, []byte("# existing prompt\n"), 0o600); err != nil {
		t.Fatalf("seed prompt file: %v", err)
	}
	return p
}

func TestAppendInjectedInstructions_NoSecretValues(t *testing.T) {
	p := writeEmptyPromptFile(t)
	cfg := &home.Config{InjectedInstructions: "Do the thing."}

	const secretValue = "xoxb-super-secret-token-value" //nolint:gosec // fake token used to assert non-leakage

	if err := appendInjectedInstructions(
		context.Background(),
		p,
		cfg,
		nil,
		[]string{"SLACK_BOT_TOKEN"},
	); err != nil {
		t.Fatalf("appendInjectedInstructions: %v", err)
	}

	got := readFile(t, p)
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

func TestAppendInjectedInstructions_EmptyStillWritesManagedShell(t *testing.T) {
	// Empty injected text still syncs the managed block (identity / resources)
	// so spawn always has a stable mycel section (#3648).
	p := writeEmptyPromptFile(t)
	if err := appendInjectedInstructions(context.Background(), p, &home.Config{InjectedInstructions: ""}, []string{"mycel"}, []string{"GH_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	if !strings.Contains(got, managedPromptStart) {
		t.Fatalf("expected managed block even with empty instructions:\n%s", got)
	}
	if !strings.Contains(got, "MCP servers: mycel") {
		t.Fatalf("expected MCP summary:\n%s", got)
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
