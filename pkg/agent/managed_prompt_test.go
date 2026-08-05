package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
)

func TestSyncManagedPrompt_Idempotent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(p, []byte("# role prompt\n\nBe helpful.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &home.Config{InjectedInstructions: "Report status before work."}

	for i := 0; i < 3; i++ {
		if err := syncAgentManagedPrompt(context.Background(), p, cfg, nil, "fast-crane", "base",
			[]string{"mycel"}, []string{"SLACK_BOT_TOKEN"}, []string{"slack:general", "slack:*"}); err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
	}

	got := readFile(t, p)
	if strings.Count(got, managedPromptStart) != 1 {
		t.Fatalf("expected one managed start marker, got %d\n%s", strings.Count(got, managedPromptStart), got)
	}
	if strings.Count(got, "Report status before work.") != 1 {
		t.Fatalf("instructions duplicated:\n%s", got)
	}
	if !strings.Contains(got, "# role prompt") {
		t.Fatalf("role prompt lost:\n%s", got)
	}
	for _, want := range []string{
		"Agent: `fast-crane`",
		"Role: `base`",
		"MCP servers: mycel",
		"Credential env vars: SLACK_BOT_TOKEN",
		"`slack:*`",
		"`slack:general`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
}

func TestSyncManagedPrompt_StripsLegacyAppends(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	legacy := "# role\n\n## mycel instructions\n\nold text\n\n## Platform Credentials\n\n- FOO: bar.\n\n## My conventions\n\nKeep me.\n"
	if err := os.WriteFile(p, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syncAgentManagedPrompt(context.Background(), p, &home.Config{InjectedInstructions: "new"}, nil,
		"a", "base", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	if strings.Contains(got, "old text") {
		t.Fatalf("legacy instructions not stripped:\n%s", got)
	}
	if strings.Contains(got, "- FOO: bar.") {
		t.Fatalf("legacy credentials not stripped:\n%s", got)
	}
	if !strings.Contains(got, "## My conventions") || !strings.Contains(got, "Keep me.") {
		t.Fatalf("user content after legacy heading was lost:\n%s", got)
	}
	if !strings.Contains(got, "new") || !strings.Contains(got, managedPromptStart) {
		t.Fatalf("managed block missing:\n%s", got)
	}
}

func TestStripManagedSections_OrphanStart(t *testing.T) {
	in := "# role\n\n" + managedPromptStart + "\npartial\n\n## after orphan\n\nkeep\n"
	got := stripManagedSections(in)
	if strings.Contains(got, managedPromptStart) || strings.Contains(got, "partial") {
		t.Fatalf("orphan start not stripped:\n%s", got)
	}
	// Orphan start drops through EOF (no end marker), so trailing content goes too.
	if strings.Contains(got, "keep") {
		t.Fatalf("expected orphan-to-EOF strip:\n%s", got)
	}
	if !strings.Contains(got, "# role") {
		t.Fatalf("role lost:\n%s", got)
	}
}

func TestAppendInjectedInstructions_UsesManagedBlock(t *testing.T) {
	p := writeEmptyPromptFile(t)
	cfg := &home.Config{InjectedInstructions: "Always report status before and after work."}
	if err := appendInjectedInstructions(context.Background(), p, cfg, []string{"mycel", "github"}, []string{"SLACK_BOT_TOKEN", "GH_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, p)
	for _, want := range []string{
		"# existing prompt",
		managedPromptStart,
		"Always report status before and after work.",
		"MCP servers: github, mycel",
		"Credential env vars: GH_TOKEN, SLACK_BOT_TOKEN",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\n%s", want, got)
		}
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
