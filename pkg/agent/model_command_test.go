package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/home"
	"github.com/rpuneet/mycel/pkg/provider"
)

// TestGetAgentCommand_PiIsolatesHomeAgentsMD is the #3678 regression:
// mycel pi spawns must disable context-file discovery (so ~/AGENTS.md
// beads instructions cannot leak) and still append the mycel-managed
// Pi.md prompt written on spawn/restart (#3648/#3652).
func TestGetAgentCommand_PiIsolatesHomeAgentsMD(t *testing.T) {
	m := &Manager{providerRegistry: provider.DefaultRegistry}
	cmd, ok := m.getAgentCommand("pi", "beads-agents-leak", false, "", "")
	if !ok {
		t.Fatal("getAgentCommand(pi) not ok")
	}
	if !strings.Contains(cmd, "--no-context-files") {
		t.Errorf("pi spawn missing --no-context-files: %q", cmd)
	}
	if !strings.Contains(cmd, "--append-system-prompt Pi.md") {
		t.Errorf("pi spawn missing mycel-managed append: %q", cmd)
	}
	// Must not shell-out to delete or rewrite the user's ~/AGENTS.md.
	for _, bad := range []string{"rm ", "unlink ", "AGENTS.md"} {
		if strings.Contains(cmd, bad) {
			t.Errorf("pi spawn must not touch user AGENTS.md (%q in %q)", bad, cmd)
		}
	}
}

// TestGetAgentCommandModel verifies the model reaches the provider's
// BuildCommand and that unsafe values are dropped before the command
// line is assembled.
func TestGetAgentCommandModel(t *testing.T) {
	m := &Manager{providerRegistry: provider.DefaultRegistry}

	tests := []struct {
		name       string
		tool       string
		model      string
		wantFlag   string
		wantAbsent string
	}{
		{"claude model injected", "claude", "fable", " --model fable", ""},
		{"agy model injected", "agy", "Gemini 3 Flash", " --model 'Gemini 3 Flash'", ""},
		{"empty model no flag", "claude", "", "", "--model"},
		{"unsafe model dropped", "claude", "$(id)", "", "id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := m.getAgentCommand(tt.tool, "test-agent", false, "", tt.model)
			if !ok {
				t.Fatalf("getAgentCommand(%q) not ok", tt.tool)
			}
			if tt.wantFlag != "" && !strings.Contains(cmd, tt.wantFlag) {
				t.Errorf("command %q missing %q", cmd, tt.wantFlag)
			}
			if tt.wantFlag == "" && tt.wantAbsent != "" && strings.Contains(cmd, tt.wantAbsent) {
				t.Errorf("command %q must not contain %q", cmd, tt.wantAbsent)
			}
		})
	}
}

// TestGetAgentCommand_CursorResume is the #3713 regression: stop→start must
// pass --continue (or --resume <id>) so Cursor does not open a fresh chat.
func TestGetAgentCommand_CursorResume(t *testing.T) {
	m := &Manager{providerRegistry: provider.DefaultRegistry}
	const sid = "ec4fb8e4-e6ee-4bf5-9e36-17a8c3ea122f"

	cmd, ok := m.getAgentCommand("cursor", "fast-crane", true, "", "")
	if !ok {
		t.Fatal("getAgentCommand(cursor) not ok")
	}
	if !strings.Contains(cmd, "--continue") {
		t.Errorf("resume without id missing --continue: %q", cmd)
	}

	cmd, ok = m.getAgentCommand("cursor", "fast-crane", true, sid, "")
	if !ok {
		t.Fatal("getAgentCommand(cursor+id) not ok")
	}
	if !strings.Contains(cmd, "--resume "+sid) {
		t.Errorf("resume with id missing --resume: %q", cmd)
	}
	if strings.Contains(cmd, "--continue") {
		t.Errorf("explicit id must not also pass --continue: %q", cmd)
	}
}

// TestGetAgentCommandModelWithOverride verifies the global command
// override path still injects a generic --model flag (gated on
// SafeModelName) via appendSessionFlags.
func TestGetAgentCommandModelWithOverride(t *testing.T) {
	m := &Manager{
		providerRegistry: provider.DefaultRegistry,
		providersConfig: &home.ProvidersConfig{
			Providers: map[string]home.ProviderConfig{
				"pi": {Command: "pi --provider amazon-bedrock"},
			},
		},
	}

	cmd, ok := m.getAgentCommand("pi", "test-agent", false, "", "anthropic/claude-sonnet-4-6")
	if !ok {
		t.Fatal("getAgentCommand(pi) not ok")
	}
	if !strings.HasPrefix(cmd, "pi --provider amazon-bedrock") {
		t.Errorf("override base lost: %q", cmd)
	}
	if !strings.Contains(cmd, " --model anthropic/claude-sonnet-4-6") {
		t.Errorf("command %q missing generic --model flag", cmd)
	}
	// Override path skips BuildCommand — isolation must still apply (#3678).
	if !strings.Contains(cmd, "--no-context-files") {
		t.Errorf("override command missing --no-context-files: %q", cmd)
	}
	if !strings.Contains(cmd, "--append-system-prompt Pi.md") {
		t.Errorf("override command missing managed prompt append: %q", cmd)
	}

	// Unsafe model must be dropped on the override path too.
	cmd, _ = m.getAgentCommand("pi", "test-agent", false, "", "a b; rm")
	if strings.Contains(cmd, "rm") {
		t.Errorf("unsafe model leaked into override command: %q", cmd)
	}
	if strings.Contains(cmd, "--model a b") || strings.Contains(cmd, "--model a") {
		t.Errorf("unsafe model flag leaked into override command: %q", cmd)
	}
}

// TestRestartCommandUsesStoredModel verifies the model survives a store
// round-trip (simulating a daemon restart) and that the command built
// from the reloaded agent — the same call startAgent makes — carries
// the stored model.
func TestRestartCommandUsesStoredModel(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	a := &Agent{Name: "model-keeper", ID: "model-keeper", Role: Role("engineer"),
		State: StateStopped, Tool: "claude", Model: "opusplan"}
	if saveErr := store.Save(context.Background(), a); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}
	if closeErr := store.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}

	// Fresh store = fresh daemon. The reloaded agent must still know
	// its model, and the restart command must include it.
	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore (reopen): %v", err)
	}
	defer func() { _ = store2.Close() }()

	loaded, err := store2.Load(context.Background(), "model-keeper")
	if err != nil || loaded == nil {
		t.Fatalf("Load after reopen: agent=%v err=%v", loaded, err)
	}
	if loaded.Model != "opusplan" {
		t.Fatalf("Model after reopen = %q, want opusplan", loaded.Model)
	}

	m := &Manager{providerRegistry: provider.DefaultRegistry}
	cmd, ok := m.getAgentCommand(loaded.Tool, loaded.Name, false, "", loaded.Model)
	if !ok {
		t.Fatal("getAgentCommand not ok")
	}
	if !strings.Contains(cmd, " --model opusplan") {
		t.Errorf("restart command %q missing stored model", cmd)
	}
}
