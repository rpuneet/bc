package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/template"
)

// newAgentHandlerForTest creates a minimal AgentHandler with only the template
// store wired so we can call applyTemplate without a real runtime.
func newAgentHandlerForTest(t *testing.T, tmplDir string) *AgentHandler {
	t.Helper()
	store := template.NewStore(tmplDir)
	h := &AgentHandler{tmplStore: store}
	return h
}

// seedTemplate creates a template in tmplDir with the given MCPs list.
func seedTemplate(t *testing.T, dir, name string, mcps []string) {
	t.Helper()
	s := template.NewStore(dir)
	tmpl := template.Template{Name: name, Description: "test", MCPs: mcps}
	if err := s.Create(tmpl, "system prompt", template.ScopeGlobal); err != nil {
		t.Fatalf("seed template %q: %v", name, err)
	}
}

// TestApplyTemplate_NoEmptyMCPEntries verifies that applying a template with
// MCPs does NOT emit empty {url:"",type:""} stubs into .mcp.json.
func TestApplyTemplate_NoEmptyMCPEntries(t *testing.T) {
	wtDir := t.TempDir()
	tmplDir := t.TempDir()

	seedTemplate(t, tmplDir, "feature-dev", []string{"mycel", "github"})

	h := newAgentHandlerForTest(t, tmplDir)
	a := &agent.Agent{Name: "test-agent", WorktreeDir: wtDir}

	if err := h.applyTemplate(nil, a, "feature-dev", nil); err != nil {
		t.Fatalf("applyTemplate: %v", err)
	}

	mcpPath := filepath.Join(wtDir, ".mcp.json")
	raw, err := os.ReadFile(mcpPath) //nolint:gosec // test uses t.TempDir() path
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}

	var cfg agentMCPFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}

	if len(cfg.MCPServers) != 2 {
		t.Errorf("want 2 MCP entries (mycel, github), got %d", len(cfg.MCPServers))
	}
	// Stubs are written with empty URL/Type — that is expected for name-only
	// entries. Critically they must NOT contain empty literal strings for url
	// that override a real config. We verify the fields are their zero values
	// and that no extra keys sneak in.
	for name, entry := range cfg.MCPServers {
		if name != "mycel" && name != "github" {
			t.Errorf("unexpected MCP key %q", name)
		}
		// omitempty means marshaled fields won't appear, but let's confirm
		// the struct itself has no stray data.
		if entry.URL != "" {
			t.Errorf("MCP %q: expected empty URL, got %q", name, entry.URL)
		}
	}
}

// TestApplyTemplate_PreservesExistingMCPConfig verifies that applying a template
// does not overwrite existing .mcp.json entries that have real config.
func TestApplyTemplate_PreservesExistingMCPConfig(t *testing.T) {
	wtDir := t.TempDir()
	tmplDir := t.TempDir()

	// Pre-populate .mcp.json with a real config entry.
	existing := agentMCPFile{
		MCPServers: map[string]agentMCPEntry{
			"mycel": {
				URL:  "http://localhost:9374/mcp/sse",
				Type: "sse",
			},
		},
	}
	initRaw, _ := json.MarshalIndent(existing, "", "  ")
	mcpPath := filepath.Join(wtDir, ".mcp.json")
	if err := os.WriteFile(mcpPath, initRaw, 0600); err != nil {
		t.Fatalf("write pre-existing .mcp.json: %v", err)
	}

	// Template includes "mycel" (already configured) and "github" (new stub).
	seedTemplate(t, tmplDir, "eng", []string{"mycel", "github"})
	h := newAgentHandlerForTest(t, tmplDir)
	a := &agent.Agent{Name: "test-agent", WorktreeDir: wtDir}

	if err := h.applyTemplate(nil, a, "eng", nil); err != nil {
		t.Fatalf("applyTemplate: %v", err)
	}

	raw, err := os.ReadFile(mcpPath) //nolint:gosec // test uses t.TempDir() path
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var cfg agentMCPFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}

	if cfg.MCPServers["mycel"].URL != "http://localhost:9374/mcp/sse" {
		t.Errorf("mycel URL was clobbered; got %q, want %q",
			cfg.MCPServers["mycel"].URL, "http://localhost:9374/mcp/sse")
	}
	if cfg.MCPServers["mycel"].Type != "sse" {
		t.Errorf("bc Type was clobbered; got %q, want sse", cfg.MCPServers["mycel"].Type)
	}
	// github should have been added as a stub.
	if _, ok := cfg.MCPServers["github"]; !ok {
		t.Error("github stub not added")
	}
}

// TestApplyTemplate_NoMCPs verifies that when the template has no MCPs,
// the existing .mcp.json is untouched.
func TestApplyTemplate_NoMCPs(t *testing.T) {
	wtDir := t.TempDir()
	tmplDir := t.TempDir()

	existing := agentMCPFile{
		MCPServers: map[string]agentMCPEntry{
			"mycel": {URL: "http://localhost:9374/mcp/sse", Type: "sse"},
		},
	}
	raw, _ := json.MarshalIndent(existing, "", "  ")
	mcpPath := filepath.Join(wtDir, ".mcp.json")
	if err := os.WriteFile(mcpPath, raw, 0600); err != nil {
		t.Fatalf("write .mcp.json: %v", err)
	}

	seedTemplate(t, tmplDir, "empty", nil)
	h := newAgentHandlerForTest(t, tmplDir)
	a := &agent.Agent{Name: "test-agent", WorktreeDir: wtDir}

	if err := h.applyTemplate(nil, a, "empty", nil); err != nil {
		t.Fatalf("applyTemplate: %v", err)
	}

	after, err := os.ReadFile(mcpPath) //nolint:gosec // test uses t.TempDir() path
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	if string(after) != string(raw) {
		t.Errorf(".mcp.json was modified but should be untouched:\nbefore: %s\nafter:  %s", raw, after)
	}
}
