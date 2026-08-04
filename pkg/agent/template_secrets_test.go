package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/template"
)

// #3550: a template that names secrets and plugins must actually deliver them.

func TestResolveAgentSecretsUnionsTemplate(t *testing.T) {
	seedDir := t.TempDir()
	s := template.NewStore(seedDir)
	if err := s.Create(template.Template{
		Name:    "trader",
		Secrets: []string{"ALPACA_KEY", "TELEGRAM_BOT_TOKEN"},
	}, "trade", template.ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	withTemplatesIn(t, seedDir)

	got := resolveAgentSecrets(t.TempDir(), "", "trader")
	if len(got) != 2 || got[0] != "ALPACA_KEY" || got[1] != "TELEGRAM_BOT_TOKEN" {
		t.Fatalf("got %v, want [ALPACA_KEY TELEGRAM_BOT_TOKEN]", got)
	}

	if got := resolveAgentSecrets(t.TempDir(), "", ""); got != nil {
		t.Fatalf("empty template: got %v", got)
	}
}

func TestTemplatePluginsReachClaudeSessionDir(t *testing.T) {
	seedDir := t.TempDir()
	s := template.NewStore(seedDir)
	if err := s.Create(template.Template{
		Name:    "with-plugin",
		Plugins: []string{"code-review"},
	}, "You review.", template.ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	withTemplatesIn(t, seedDir)

	// AgentSessionDir reads MYCEL_HOME/agents/<name>/session
	mycelHome := os.Getenv("MYCEL_HOME")
	wtDir := t.TempDir()
	if err := SetupAgentFromRoleAndTemplate(t.Context(), t.TempDir(), "plug-1", "", wtDir, "tmux", "claude", "with-plugin"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	manifest := filepath.Join(mycelHome, "agents", "plug-1", "session", "claude", "installed_plugins.json")
	raw, err := os.ReadFile(manifest) //nolint:gosec // test path
	if err != nil {
		t.Fatalf("plugin manifest missing at %s: %v", manifest, err)
	}
	var m struct {
		Plugins map[string]struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if p, ok := m.Plugins["code-review"]; !ok || !p.Enabled {
		t.Fatalf("plugins = %#v, want code-review enabled", m.Plugins)
	}
}

func TestTemplateSecretsInjectIntoAgentEnv(t *testing.T) {
	// End-to-end for the allowlist path: vault has ALPACA_KEY, template
	// declares it, injectVaultSecrets must put it in env (#3550).
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "unit-test-passphrase")

	vaultPath := filepath.Join(mycelHome, "secrets.vault")
	vault, err := secret.OpenVaultFile(vaultPath, "unit-test-passphrase")
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}
	if err := vault.Set("ALPACA_KEY", "sk-live-test", ""); err != nil {
		t.Fatal(err)
	}
	_ = vault.Close()

	tmplDir := filepath.Join(mycelHome, "templates")
	if err := os.MkdirAll(tmplDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := template.NewStore(tmplDir).Create(template.Template{
		Name:    "trader",
		Secrets: []string{"ALPACA_KEY"},
	}, "trade", template.ScopeGlobal); err != nil {
		t.Fatal(err)
	}

	names := resolveAgentSecrets(t.TempDir(), "", "trader")
	env := map[string]string{}
	injected := injectVaultSecrets(env, t.TempDir(), names, nil)
	if env["ALPACA_KEY"] != "sk-live-test" {
		t.Fatalf("env ALPACA_KEY = %q, injected=%v", env["ALPACA_KEY"], injected)
	}
}
