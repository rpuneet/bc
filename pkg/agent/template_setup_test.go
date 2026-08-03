package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/template"
)

// These tests replace server/handlers/apply_template_test.go, which asserted the
// behavior that made templates useless: it required each named MCP server to be
// written as an entry with an empty URL, and read a prompt out of CLAUDE.md no
// matter which provider the agent ran. Both are what a user hit — a Cursor agent
// with a persona it never read and tools it could not reach.

func seedTemplate(t *testing.T, dir, name, prompt string, mcps []string) {
	t.Helper()
	s := template.NewStore(dir)
	if err := s.Create(template.Template{Name: name, Description: "test", MCPs: mcps}, prompt, template.ScopeGlobal); err != nil {
		t.Fatalf("seed template %q: %v", name, err)
	}
}

// withTemplatesIn points the setup path's template lookup at dir by relocating
// MYCEL_HOME, which is where the user-global template store lives.
func withTemplatesIn(t *testing.T, dir string) {
	t.Helper()
	mycelHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mycelHome, "templates"), 0o750); err != nil {
		t.Fatalf("make templates dir: %v", err)
	}
	t.Setenv("MYCEL_HOME", mycelHome)
	seedDir := filepath.Join(mycelHome, "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read seed dir: %v", err)
	}
	for _, e := range entries {
		raw, readErr := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // test temp dir
		if readErr != nil {
			t.Fatalf("read %s: %v", e.Name(), readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(seedDir, e.Name()), raw, 0o600); writeErr != nil {
			t.Fatalf("write %s: %v", e.Name(), writeErr)
		}
	}
}

func readMCPServers(t *testing.T, wtDir string) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wtDir, ".mcp.json")) //nolint:gosec // test temp dir
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var cfg struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	return cfg.MCPServers
}

func TestTemplatePromptGoesToTheFileTheProviderReads(t *testing.T) {
	// The whole point of a persona is that the agent reads it. Cursor reads
	// .cursorrules and never CLAUDE.md, so writing one file for every provider
	// meant the default provider ignored every template.
	for _, tc := range []struct {
		tool     string
		wantFile string
	}{
		{"cursor", ".cursorrules"},
		{"claude", "CLAUDE.md"},
		{"agy", "AGENTS.md"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			seedDir := t.TempDir()
			seedTemplate(t, seedDir, "trader", "You trade.", nil)
			withTemplatesIn(t, seedDir)

			wtDir := t.TempDir()
			if err := SetupAgentFromRoleAndTemplate(t.Context(), t.TempDir(), "a1", "", wtDir, "tmux", tc.tool, "trader"); err != nil {
				t.Fatalf("setup: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(wtDir, tc.wantFile)) //nolint:gosec // test temp dir
			if err != nil {
				t.Fatalf("%s: %v", tc.wantFile, err)
			}
			if strings.TrimSpace(string(got)) != "You trade." {
				t.Errorf("%s = %q, want the template's prompt", tc.wantFile, got)
			}
		})
	}
}

func TestTemplateWritesNoMCPServerItCannotDescribe(t *testing.T) {
	// An entry naming a server without saying how to reach one is not a
	// configuration, it is a claim. The old writer emitted `{}` per name.
	seedDir := t.TempDir()
	seedTemplate(t, seedDir, "trader", "You trade.", []string{"alpaca-not-installed"})
	withTemplatesIn(t, seedDir)

	wtDir := t.TempDir()
	if err := SetupAgentFromRoleAndTemplate(t.Context(), t.TempDir(), "a1", "", wtDir, "tmux", "cursor", "trader"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	servers := readMCPServers(t, wtDir)
	if entry, ok := servers["alpaca-not-installed"]; ok {
		t.Errorf("wrote an entry for a server with no definition: %v", entry)
	}
	for name, entry := range servers {
		if len(entry) == 0 {
			t.Errorf("MCP %q is an empty object, which configures nothing", name)
		}
	}
}

func TestEveryAgentGetsTheMycelServer(t *testing.T) {
	// It is how an agent messages anyone or reports a cost, and it needs no
	// store to describe, so nothing should be able to cost an agent its own.
	seedDir := t.TempDir()
	seedTemplate(t, seedDir, "trader", "You trade.", nil)
	withTemplatesIn(t, seedDir)

	wtDir := t.TempDir()
	if err := SetupAgentFromRoleAndTemplate(t.Context(), t.TempDir(), "a1", "", wtDir, "tmux", "cursor", "trader"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	servers := readMCPServers(t, wtDir)
	self, ok := servers["mycel"]
	if !ok {
		t.Fatalf("no mycel server; got %v", servers)
	}
	if url, _ := self["url"].(string); url == "" {
		t.Errorf("mycel server has no url: %v", self)
	}
}

func TestAMissingTemplateIsNotFatal(t *testing.T) {
	// The agent exists and is running by the time this is called, so a template
	// that cannot be read is worth a warning and not a failure — and nothing
	// should be invented in its place.
	withTemplatesIn(t, t.TempDir())

	wtDir := t.TempDir()
	if err := SetupAgentFromRoleAndTemplate(t.Context(), t.TempDir(), "a1", "", wtDir, "tmux", "cursor", "no-such-template"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, name := range []string{".cursorrules", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(wtDir, name)); err == nil {
			t.Errorf("wrote %s for a template that does not exist", name)
		}
	}
}

func TestAClaudeAgentIsTheOnlyOneSetUpWithTheClaudeCLI(t *testing.T) {
	// Running `claude mcp add` for a Cursor agent registered its servers with a
	// CLI it does not use, and skipped writing the .mcp.json it does read.
	for _, tool := range []string{"cursor", "agy", "codex"} {
		t.Run(tool, func(t *testing.T) {
			if isClaudeTool(tool) {
				t.Fatalf("%s should not be treated as claude", tool)
			}
		})
	}
	for _, tool := range []string{"claude", ""} {
		t.Run("claude/"+tool, func(t *testing.T) {
			if !isClaudeTool(tool) {
				t.Fatalf("%q should be treated as claude (empty is the default)", tool)
			}
		})
	}
}

func TestOverlayTemplate(t *testing.T) {
	role := &home.ResolvedRole{
		Name:       "engineer",
		Prompt:     "role prompt",
		MCPServers: []string{"mycel", "github"},
		Secrets:    []string{"GITHUB_TOKEN"},
		Plugins:    []string{"role-plugin"},
	}

	t.Run("the prompt is replaced, because that is what a template is for", func(t *testing.T) {
		out := overlayTemplate(role, &template.Template{Name: "trader"}, "template prompt")
		if out.Prompt != "template prompt" {
			t.Errorf("prompt = %q, want the template's", out.Prompt)
		}
	})

	t.Run("an empty template prompt does not blank the role's", func(t *testing.T) {
		out := overlayTemplate(role, &template.Template{Name: "trader"}, "")
		if out.Prompt != "role prompt" {
			t.Errorf("prompt = %q, want the role's kept", out.Prompt)
		}
	})

	t.Run("lists are unions — asking for one server is not asking to lose the rest", func(t *testing.T) {
		out := overlayTemplate(role, &template.Template{
			Name:    "trader",
			MCPs:    []string{"alpaca", "github"},
			Secrets: []string{"ALPACA_KEY"},
			Plugins: []string{"tmpl-plugin"},
		}, "p")

		want := []string{"mycel", "github", "alpaca"}
		if len(out.MCPServers) != len(want) {
			t.Fatalf("MCPServers = %v, want %v", out.MCPServers, want)
		}
		for i, name := range want {
			if out.MCPServers[i] != name {
				t.Errorf("MCPServers[%d] = %q, want %q", i, out.MCPServers[i], name)
			}
		}
		if len(out.Secrets) != 2 || len(out.Plugins) != 2 {
			t.Errorf("secrets = %v, plugins = %v; want both unioned", out.Secrets, out.Plugins)
		}
	})

	t.Run("the role is left untouched", func(t *testing.T) {
		before := len(role.MCPServers)
		overlayTemplate(role, &template.Template{Name: "trader", MCPs: []string{"alpaca"}}, "p")
		if len(role.MCPServers) != before {
			t.Errorf("overlay mutated the role: %v", role.MCPServers)
		}
	})

	t.Run("a template with no role behind it still provisions", func(t *testing.T) {
		out := overlayTemplate(nil, &template.Template{Name: "trader", MCPs: []string{"alpaca"}}, "p")
		if out == nil || out.Prompt != "p" || len(out.MCPServers) != 1 {
			t.Errorf("overlay onto nil role = %+v", out)
		}
	})
}
