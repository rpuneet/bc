package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseCodexDebugModels(t *testing.T) {
	out := `{"models":[
		{"slug":"gpt-5.6-sol","visibility":"list"},
		{"slug":"gpt-5.4","visibility":"hide"},
		{"slug":"gpt-5.2","visibility":"list"},
		{"slug":"bad;slug","visibility":"list"},
		{"slug":"gpt-5.6-sol","visibility":"list"}
	]}`
	got := parseCodexDebugModels(out)
	want := []string{"gpt-5.6-sol", "gpt-5.2"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("parseCodexDebugModels() = %v, want %v", got, want)
	}
}

func TestCodexListModels(t *testing.T) {
	p := NewCodexProvider()
	orig := codexListModels
	t.Cleanup(func() { codexListModels = orig })

	codexListModels = func(_ context.Context) (string, error) {
		return `{"models":[{"slug":"gpt-5.6-luna","visibility":"list"}]}`, nil
	}
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "gpt-5.6-luna" {
		t.Fatalf("ListModels() = %v", got)
	}

	codexListModels = func(_ context.Context) (string, error) {
		return "", errors.New("not installed")
	}
	got, err = p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(p.Models()) {
		t.Fatalf("fallback ListModels() = %v, want static %v", got, p.Models())
	}
}

func TestParseCursorListModels(t *testing.T) {
	out := "Available models\n\nauto - Auto (current, default)\ngpt-5.3-codex - Codex 5.3\nbad;id - nope\n"
	got := parseCursorListModels(out)
	want := []string{"auto", "gpt-5.3-codex"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("parseCursorListModels() = %v, want %v", got, want)
	}
}

func TestCursorListModels(t *testing.T) {
	p := NewCursorProvider()
	orig := cursorListModels
	t.Cleanup(func() { cursorListModels = orig })

	cursorListModels = func(_ context.Context) (string, error) {
		return "composer-2.5 - Composer 2.5\n", nil
	}
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "composer-2.5" {
		t.Fatalf("ListModels() = %v", got)
	}
}

func TestClaudeListModelsMarksReachable(t *testing.T) {
	p := NewClaudeProvider()
	orig := claudeListModels
	t.Cleanup(func() { claudeListModels = orig })

	claudeListModels = func(_ context.Context) (string, error) { return "ok", nil }
	got, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(p.Models()) {
		t.Fatalf("ListModels() = %v", got)
	}

	claudeListModels = func(_ context.Context) (string, error) { return "", errors.New("missing") }
	if _, err := p.ListModels(context.Background()); err == nil {
		t.Fatal("expected error when CLI probe fails")
	}

	if !SafeClaudeModelName("opus[1m]") {
		t.Fatal("opus[1m] should be a safe Claude model")
	}
	cmd := p.BuildCommand(CommandOpts{Model: "opus[1m]"})
	if !strings.Contains(cmd, "--model 'opus[1m]'") {
		t.Fatalf("BuildCommand = %q, want quoted bracket alias", cmd)
	}
}
