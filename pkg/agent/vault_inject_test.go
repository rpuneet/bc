package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rpuneet/mycel/pkg/secret"
)

// seedWorkspaceVault writes a secret into the workspace vault under wsPath/.bc/secrets.db.
func seedWorkspaceVault(t *testing.T, wsPath, name, value string) {
	t.Helper()
	ss, err := secret.NewStore(wsPath, "test-passphrase")
	if err != nil {
		t.Fatalf("seedWorkspaceVault: NewStore: %v", err)
	}
	defer func() { _ = ss.Close() }()
	if err := ss.Set(name, value, ""); err != nil {
		t.Fatalf("seedWorkspaceVault: Set %q: %v", name, err)
	}
}

// seedGlobalVault writes a secret into the global vault at globalVaultPath.
func seedGlobalVault(t *testing.T, globalVaultPath, name, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(globalVaultPath), 0700); err != nil {
		t.Fatalf("seedGlobalVault: MkdirAll: %v", err)
	}
	gs, err := secret.OpenVaultFile(globalVaultPath, "test-passphrase")
	if err != nil {
		t.Fatalf("seedGlobalVault: OpenVaultFile: %v", err)
	}
	defer func() { _ = gs.Close() }()
	if err := gs.Set(name, value, ""); err != nil {
		t.Fatalf("seedGlobalVault: Set %q: %v", name, err)
	}
}

// TestInjectVaultSecrets covers the vault→env injection helper.
func TestInjectVaultSecrets(t *testing.T) {
	// Pin the passphrase and home dir so all sub-tests share the same key.
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "test-passphrase")

	wsPath := t.TempDir()
	globalVaultPath := filepath.Join(mycelHome, "secrets.vault")

	seedWorkspaceVault(t, wsPath, "WS_SECRET", "ws-value")
	seedWorkspaceVault(t, wsPath, "SLACK_BOT_TOKEN", "slack-ws-token")
	seedWorkspaceVault(t, wsPath, "GITHUB_PERSONAL_ACCESS_TOKEN", "ghp-token-123")
	seedGlobalVault(t, globalVaultPath, "GLOBAL_SECRET", "global-value")
	seedGlobalVault(t, globalVaultPath, "SHARED_SECRET", "global-shared")
	seedWorkspaceVault(t, wsPath, "SHARED_SECRET", "ws-wins")

	tests := []struct {
		name        string
		preEnv      map[string]string // env already set before injection
		roleSecrets []string
		wantKey     string
		wantValue   string
		wantAbsent  []string
	}{
		{
			name:        "role-declared secret injected from workspace vault",
			preEnv:      map[string]string{},
			roleSecrets: []string{"WS_SECRET"},
			wantKey:     "WS_SECRET",
			wantValue:   "ws-value",
		},
		{
			name:        "role-declared secret injected from global vault",
			preEnv:      map[string]string{},
			roleSecrets: []string{"GLOBAL_SECRET"},
			wantKey:     "GLOBAL_SECRET",
			wantValue:   "global-value",
		},
		{
			name:        "workspace vault wins over global for same name",
			preEnv:      map[string]string{},
			roleSecrets: []string{"SHARED_SECRET"},
			wantKey:     "SHARED_SECRET",
			wantValue:   "ws-wins",
		},
		{
			name:   "well-known gateway token auto-injected without role declaration",
			preEnv: map[string]string{},
			// SLACK_BOT_TOKEN not in roleSecrets — should still inject via wellKnownVaultTokens
			roleSecrets: nil,
			wantKey:     "SLACK_BOT_TOKEN",
			wantValue:   "slack-ws-token",
		},
		{
			name: "existing env wins over vault (precedence)",
			preEnv: map[string]string{
				"SLACK_BOT_TOKEN": "gateway-token", // set by injectGatewayEnv
			},
			roleSecrets: []string{"SLACK_BOT_TOKEN"},
			wantKey:     "SLACK_BOT_TOKEN",
			wantValue:   "gateway-token", // vault must NOT overwrite
		},
		{
			name:        "missing secret skipped — no key inserted",
			preEnv:      map[string]string{},
			roleSecrets: []string{"DOES_NOT_EXIST"},
			wantAbsent:  []string{"DOES_NOT_EXIST"},
		},
		{
			name:        "GITHUB_PAT aliased to GITHUB_TOKEN and GH_TOKEN",
			preEnv:      map[string]string{},
			roleSecrets: []string{"GITHUB_PERSONAL_ACCESS_TOKEN"},
			wantKey:     "GITHUB_TOKEN",
			wantValue:   "ghp-token-123",
		},
		{
			name:        "GH_TOKEN also aliased from GITHUB_PAT",
			preEnv:      map[string]string{},
			roleSecrets: []string{"GITHUB_PERSONAL_ACCESS_TOKEN"},
			wantKey:     "GH_TOKEN",
			wantValue:   "ghp-token-123",
		},
		{
			name: "existing GITHUB_TOKEN wins over vault alias",
			preEnv: map[string]string{
				"GITHUB_TOKEN": "existing-gh-token",
			},
			roleSecrets: []string{"GITHUB_PERSONAL_ACCESS_TOKEN"},
			wantKey:     "GITHUB_TOKEN",
			wantValue:   "existing-gh-token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := make(map[string]string, len(tc.preEnv))
			for k, v := range tc.preEnv {
				env[k] = v
			}

			injectVaultSecrets(env, wsPath, tc.roleSecrets)

			if tc.wantKey != "" {
				if got := env[tc.wantKey]; got != tc.wantValue {
					t.Errorf("env[%q] = %q, want %q", tc.wantKey, got, tc.wantValue)
				}
			}
			for _, absent := range tc.wantAbsent {
				if _, ok := env[absent]; ok {
					t.Errorf("env[%q] should be absent but is present", absent)
				}
			}
		})
	}
}

// TestLoadSecrets_RealPassphrase proves loadSecrets resolves values correctly
// when the vault is encrypted with a real passphrase (BUG 1 regression test).
// Previously loadSecrets opened with empty passphrase "" which silently returned
// empty values on encrypted vaults.
func TestLoadSecrets_RealPassphrase(t *testing.T) {
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "strong-passphrase")

	wsPath := t.TempDir()
	// Seed with the REAL passphrase.
	ss, err := secret.NewStore(wsPath, "strong-passphrase")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := ss.Set("MCP_API_KEY", "the-real-value", ""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_ = ss.Close()

	got := loadSecrets(wsPath, []string{"MCP_API_KEY"})

	if got["MCP_API_KEY"] != "the-real-value" {
		t.Errorf("loadSecrets with real passphrase: MCP_API_KEY = %q, want %q",
			got["MCP_API_KEY"], "the-real-value")
	}
}

// TestResolveSecretRefs_LayeredStore ensures resolveSecretRefs resolves refs
// from the layered (global+workspace) vault, not just the workspace vault.
func TestResolveSecretRefs_LayeredStore(t *testing.T) {
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "test-passphrase")

	wsPath := t.TempDir()
	globalVaultPath := filepath.Join(mycelHome, "secrets.vault")

	// Secret lives ONLY in the global vault.
	seedGlobalVault(t, globalVaultPath, "GLOBAL_API_KEY", "global-resolved-value")

	env := map[string]string{
		"MYCEL_AGENT_ID": "agent1",
		"API_KEY":        "${secret:GLOBAL_API_KEY}",
	}
	resolveSecretRefs(env, wsPath)

	if env["API_KEY"] != "global-resolved-value" {
		t.Errorf("resolveSecretRefs from global vault: API_KEY = %q, want %q",
			env["API_KEY"], "global-resolved-value")
	}
	// System vars must be untouched.
	if env["MYCEL_AGENT_ID"] != "agent1" {
		t.Errorf("MYCEL_AGENT_ID clobbered: %q", env["MYCEL_AGENT_ID"])
	}
}
