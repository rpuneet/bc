package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/secret"
)

// seedRepoVault writes a secret into the repo vault under repoPath/.mycel/secrets.db.
func seedRepoVault(t *testing.T, repoPath, name, value string) {
	t.Helper()
	ss, err := secret.NewStore(repoPath, "test-passphrase")
	if err != nil {
		t.Fatalf("seedRepoVault: NewStore: %v", err)
	}
	defer func() { _ = ss.Close() }()
	if err := ss.Set(name, value, ""); err != nil {
		t.Fatalf("seedRepoVault: Set %q: %v", name, err)
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

	repoPath := t.TempDir()
	globalVaultPath := filepath.Join(mycelHome, "secrets.vault")

	seedRepoVault(t, repoPath, "WS_SECRET", "h-value")
	seedRepoVault(t, repoPath, "SLACK_BOT_TOKEN", "slack-h-token")
	seedRepoVault(t, repoPath, "GITHUB_PERSONAL_ACCESS_TOKEN", "ghp-token-123")
	seedGlobalVault(t, globalVaultPath, "GLOBAL_SECRET", "global-value")
	seedGlobalVault(t, globalVaultPath, "SHARED_SECRET", "global-shared")
	seedRepoVault(t, repoPath, "SHARED_SECRET", "h-wins")

	tests := []struct {
		name        string
		preEnv      map[string]string // env already set before injection
		roleSecrets []string
		channels    []string
		wantKey     string
		wantValue   string
		wantAbsent  []string
	}{
		{
			name:        "role-declared secret injected from repo vault",
			preEnv:      map[string]string{},
			roleSecrets: []string{"WS_SECRET"},
			wantKey:     "WS_SECRET",
			wantValue:   "h-value",
		},
		{
			name:        "role-declared secret injected from global vault",
			preEnv:      map[string]string{},
			roleSecrets: []string{"GLOBAL_SECRET"},
			wantKey:     "GLOBAL_SECRET",
			wantValue:   "global-value",
		},
		{
			name:        "repo vault wins over global for same name",
			preEnv:      map[string]string{},
			roleSecrets: []string{"SHARED_SECRET"},
			wantKey:     "SHARED_SECRET",
			wantValue:   "h-wins",
		},
		{
			name:   "well-known gateway token auto-injected for slack subscriber",
			preEnv: map[string]string{},
			// SLACK_BOT_TOKEN not in roleSecrets — inject via wellKnownVaultTokens
			// only when subscribed to a slack channel (#3686).
			roleSecrets: nil,
			channels:    []string{"slack:general"},
			wantKey:     "SLACK_BOT_TOKEN",
			wantValue:   "slack-h-token",
		},
		{
			name:        "well-known token skipped for non-subscriber",
			preEnv:      map[string]string{},
			roleSecrets: nil,
			channels:    []string{"telegram:ops"},
			wantAbsent:  []string{"SLACK_BOT_TOKEN"},
		},
		{
			name: "existing env wins over vault (precedence)",
			preEnv: map[string]string{
				"SLACK_BOT_TOKEN": "gateway-token", // set by injectAppEnv
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

			injectVaultSecrets(env, repoPath, tc.roleSecrets, nil, tc.channels)

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

// TestInjectVaultSecretsAppCredentials proves connected-app credentials
// stored under app:<instance>:<key> are exported under their conventional
// env names only for agents subscribed to that instance (#3686).
func TestInjectVaultSecretsAppCredentials(t *testing.T) {
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "test-passphrase")

	repoPath := t.TempDir()
	seedRepoVault(t, repoPath, "app:slack:bot_token", "xoxb-app-vault")
	seedRepoVault(t, repoPath, "app:telegram:alerts:bot_token", "tg-app-vault")
	seedRepoVault(t, repoPath, "app:rss:blog:url", "should-not-inject") // rss url is not a Secret field

	apps := map[string]app.InstanceConfig{
		"slack":           {App: "slack", Enabled: true},
		"telegram:alerts": {App: "telegram", Enabled: true},
		"telegram:off":    {App: "telegram", Enabled: false},
		"rss:blog":        {App: "rss", Enabled: true, Config: map[string]string{"url": "https://x/feed"}},
	}

	t.Run("subscriber receives matching instance secrets", func(t *testing.T) {
		env := map[string]string{}
		injected := injectVaultSecrets(env, repoPath, nil, apps, []string{
			"slack:general",
			"telegram:alerts:ops",
		})

		if env["SLACK_BOT_TOKEN"] != "xoxb-app-vault" {
			t.Errorf("SLACK_BOT_TOKEN = %q, want xoxb-app-vault", env["SLACK_BOT_TOKEN"])
		}
		if env["TELEGRAM_BOT_TOKEN_ALERTS"] != "tg-app-vault" {
			t.Errorf("TELEGRAM_BOT_TOKEN_ALERTS = %q, want tg-app-vault", env["TELEGRAM_BOT_TOKEN_ALERTS"])
		}
		if _, ok := env["TELEGRAM_BOT_TOKEN_OFF"]; ok {
			t.Error("disabled instance credential must not be injected")
		}
		if _, ok := env["RSS_URL_BLOG"]; ok {
			t.Error("non-secret field must not be injected from the vault")
		}
		if len(injected) == 0 {
			t.Error("injected key names should be reported")
		}
	})

	t.Run("non-subscriber excluded from other instances", func(t *testing.T) {
		env := map[string]string{}
		injectVaultSecrets(env, repoPath, nil, apps, []string{"slack:*"})

		if env["SLACK_BOT_TOKEN"] != "xoxb-app-vault" {
			t.Errorf("SLACK_BOT_TOKEN = %q, want xoxb-app-vault for slack subscriber", env["SLACK_BOT_TOKEN"])
		}
		if _, ok := env["TELEGRAM_BOT_TOKEN_ALERTS"]; ok {
			t.Error("telegram:alerts secret must not inject for a slack-only subscriber")
		}
	})

	t.Run("no subscriptions yields no connected-app secrets", func(t *testing.T) {
		env := map[string]string{}
		injectVaultSecrets(env, repoPath, nil, apps, nil)

		if _, ok := env["SLACK_BOT_TOKEN"]; ok {
			t.Error("SLACK_BOT_TOKEN must not inject without a slack subscription")
		}
		if _, ok := env["TELEGRAM_BOT_TOKEN_ALERTS"]; ok {
			t.Error("TELEGRAM_BOT_TOKEN_ALERTS must not inject without a telegram:alerts subscription")
		}
	})
}

// TestInjectVaultSecrets_GitHubAppToken proves a connected GitHub app
// instance's api_token (as populated by the device-flow "Sign in with
// GitHub", or pasted manually) is made available to subscribed agents as
// both GH_TOKEN and GITHUB_TOKEN — so `gh` and git's credential helper
// authenticate with zero per-agent setup — while an agent with no
// github subscription and no role allowlist gets neither key (#3686).
func TestInjectVaultSecrets_GitHubAppToken(t *testing.T) {
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "test-passphrase")

	repoPath := t.TempDir()
	seedRepoVault(t, repoPath, "app:github:api_token", "gho_devflow_token")

	t.Run("subscribed agent yields GH_TOKEN and GITHUB_TOKEN", func(t *testing.T) {
		apps := map[string]app.InstanceConfig{
			"github": {App: "github", Enabled: true},
		}
		env := map[string]string{}
		injected := injectVaultSecrets(env, repoPath, nil, apps, []string{"github:*"})

		if env["GH_TOKEN"] != "gho_devflow_token" {
			t.Errorf("GH_TOKEN = %q, want gho_devflow_token", env["GH_TOKEN"])
		}
		if env["GITHUB_TOKEN"] != "gho_devflow_token" {
			t.Errorf("GITHUB_TOKEN = %q, want gho_devflow_token", env["GITHUB_TOKEN"])
		}
		// The generic connected-app env name is still populated too.
		if env["GITHUB_API_TOKEN"] != "gho_devflow_token" {
			t.Errorf("GITHUB_API_TOKEN = %q, want gho_devflow_token", env["GITHUB_API_TOKEN"])
		}
		found := false
		for _, k := range injected {
			if k == "GH_TOKEN" {
				found = true
			}
		}
		if !found {
			t.Error("GH_TOKEN should be reported in the injected key names")
		}
	})

	t.Run("role allowlist aliases without github subscription", func(t *testing.T) {
		apps := map[string]app.InstanceConfig{
			"github": {App: "github", Enabled: true},
		}
		env := map[string]string{}
		injectVaultSecrets(env, repoPath, []string{"GH_TOKEN"}, apps, nil)

		if env["GH_TOKEN"] != "gho_devflow_token" {
			t.Errorf("GH_TOKEN = %q, want gho_devflow_token via role allowlist", env["GH_TOKEN"])
		}
		if env["GITHUB_TOKEN"] != "gho_devflow_token" {
			t.Errorf("GITHUB_TOKEN = %q, want gho_devflow_token via role allowlist", env["GITHUB_TOKEN"])
		}
	})

	t.Run("non-subscriber without role allowlist yields neither key", func(t *testing.T) {
		apps := map[string]app.InstanceConfig{
			"github": {App: "github", Enabled: true},
		}
		env := map[string]string{}
		injectVaultSecrets(env, repoPath, nil, apps, []string{"slack:general"})

		if _, ok := env["GH_TOKEN"]; ok {
			t.Error("GH_TOKEN should be absent without github scope")
		}
		if _, ok := env["GITHUB_TOKEN"]; ok {
			t.Error("GITHUB_TOKEN should be absent without github scope")
		}
	})

	t.Run("disabled instance yields neither key", func(t *testing.T) {
		apps := map[string]app.InstanceConfig{
			"github": {App: "github", Enabled: false},
		}
		env := map[string]string{}
		injectVaultSecrets(env, repoPath, nil, apps, []string{"github:*"})

		if _, ok := env["GH_TOKEN"]; ok {
			t.Error("GH_TOKEN should be absent when no github app instance is enabled")
		}
		if _, ok := env["GITHUB_TOKEN"]; ok {
			t.Error("GITHUB_TOKEN should be absent when no github app instance is enabled")
		}
	})

	t.Run("no github app configured yields neither key", func(t *testing.T) {
		env := map[string]string{}
		injectVaultSecrets(env, repoPath, nil, nil, nil)

		if _, ok := env["GH_TOKEN"]; ok {
			t.Error("GH_TOKEN should be absent with no apps configured")
		}
		if _, ok := env["GITHUB_TOKEN"]; ok {
			t.Error("GITHUB_TOKEN should be absent with no apps configured")
		}
	})
}

func TestSubscribedToAppAndPlatform(t *testing.T) {
	channels := []string{"slack:general", "slack:*", "telegram:alerts:ops", "telegram:alerts:*"}
	if !subscribedToApp(channels, "slack") {
		t.Error("want subscribedToApp(slack)")
	}
	if !subscribedToApp(channels, "telegram:alerts") {
		t.Error("want subscribedToApp(telegram:alerts)")
	}
	if subscribedToApp(channels, "discord") {
		t.Error("discord must not match")
	}
	if !subscribedToPlatform(channels, "slack") {
		t.Error("want subscribedToPlatform(slack)")
	}
	if !subscribedToPlatform(channels, "telegram") {
		t.Error("labeled telegram:alerts:* must still match platform telegram")
	}
	if subscribedToPlatform(channels, "discord") {
		t.Error("discord platform must not match")
	}
}

// TestAppEnvAndPromptInstructions covers plain-field env injection and the
// descriptor-driven prompt documentation.
func TestAppEnvAndPromptInstructions(t *testing.T) {
	apps := map[string]app.InstanceConfig{
		"rss:blog": {App: "rss", Enabled: true, Config: map[string]string{"url": "https://example.com/feed.xml", "interval": "60"}},
		"slack":    {App: "slack", Enabled: true, Config: map[string]string{"mode": "socket"}},
		"rss:off":  {App: "rss", Enabled: false, Config: map[string]string{"url": "https://off/feed"}},
	}

	env := map[string]string{}
	injectAppEnv(env, apps)
	if env["RSS_URL_BLOG"] != "https://example.com/feed.xml" {
		t.Errorf("RSS_URL_BLOG = %q, want feed url", env["RSS_URL_BLOG"])
	}
	if _, ok := env["RSS_INTERVAL_BLOG"]; ok {
		t.Error("optional plain field must not be injected")
	}
	if _, ok := env["RSS_URL_OFF"]; ok {
		t.Error("disabled instance must not be injected")
	}

	doc := appPromptInstructions(apps)
	if !strings.Contains(doc, "SLACK_BOT_TOKEN") {
		t.Errorf("prompt docs missing SLACK_BOT_TOKEN:\n%s", doc)
	}
	if !strings.Contains(doc, "RSS_URL_BLOG") {
		t.Errorf("prompt docs missing RSS_URL_BLOG:\n%s", doc)
	}
	if strings.Contains(doc, "RSS_URL_OFF") {
		t.Errorf("prompt docs must skip disabled instances:\n%s", doc)
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

	repoPath := t.TempDir()
	// Seed with the REAL passphrase.
	ss, err := secret.NewStore(repoPath, "strong-passphrase")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := ss.Set("MCP_API_KEY", "the-real-value", ""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_ = ss.Close()

	got := loadSecrets(repoPath, []string{"MCP_API_KEY"})

	if got["MCP_API_KEY"] != "the-real-value" {
		t.Errorf("loadSecrets with real passphrase: MCP_API_KEY = %q, want %q",
			got["MCP_API_KEY"], "the-real-value")
	}
}

// TestResolveSecretRefs_LayeredStore ensures resolveSecretRefs resolves refs
// from the layered (global+repo) vault, not just the repo vault.
func TestResolveSecretRefs_LayeredStore(t *testing.T) {
	mycelHome := t.TempDir()
	t.Setenv("MYCEL_HOME", mycelHome)
	t.Setenv(secret.PassphraseEnvVar, "test-passphrase")

	repoPath := t.TempDir()
	globalVaultPath := filepath.Join(mycelHome, "secrets.vault")

	// Secret lives ONLY in the global vault.
	seedGlobalVault(t, globalVaultPath, "GLOBAL_API_KEY", "global-resolved-value")

	env := map[string]string{
		"MYCEL_AGENT_ID": "agent1",
		"API_KEY":        "${secret:GLOBAL_API_KEY}",
	}
	resolveSecretRefs(env, repoPath)

	if env["API_KEY"] != "global-resolved-value" {
		t.Errorf("resolveSecretRefs from global vault: API_KEY = %q, want %q",
			env["API_KEY"], "global-resolved-value")
	}
	// System vars must be untouched.
	if env["MYCEL_AGENT_ID"] != "agent1" {
		t.Errorf("MYCEL_AGENT_ID clobbered: %q", env["MYCEL_AGENT_ID"])
	}
}
