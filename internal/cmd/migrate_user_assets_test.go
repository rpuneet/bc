package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bcmcp "github.com/rpuneet/bc/pkg/mcp"
	bcsecret "github.com/rpuneet/bc/pkg/secret"
	bctemplate "github.com/rpuneet/bc/pkg/template"
	bcworkspace "github.com/rpuneet/bc/pkg/workspace"
)

// setupBCHome returns an isolated ~/.bc replacement. The caller is
// responsible for also isolating the passphrase via
// BC_SECRET_PASSPHRASE if secret migration paths are exercised.
func setupBCHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BC_HOME", dir)
	// Ensure a clean BC_STATE_DIR so tests don't pick up a user's real state.
	t.Setenv("BC_STATE_DIR", filepath.Join(dir, "state"))
	return dir
}

func TestMigrationMarkerShortCircuits(t *testing.T) {
	home := setupBCHome(t)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")
	// Pre-write the marker: migration should be a no-op.
	if err := os.WriteFile(filepath.Join(home, migrationMarkerName), []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrateUserAssets(context.Background(), false); err != nil {
		t.Fatalf("RunMigrateUserAssets: %v", err)
	}
	// The marker must still exist.
	if _, err := os.Stat(filepath.Join(home, migrationMarkerName)); err != nil {
		t.Errorf("marker disappeared: %v", err)
	}
}

func TestMigrateTemplates(t *testing.T) {
	_ = setupBCHome(t)

	// Synthesize a legacy workspace with a templates/ dir.
	wsRoot := t.TempDir()
	wsLegacy := filepath.Join(wsRoot, ".bc")
	legacyTmpl := filepath.Join(wsLegacy, "templates")
	if err := os.MkdirAll(legacyTmpl, 0o750); err != nil {
		t.Fatal(err)
	}
	// Write a minimal template JSON + prompt.
	jsonBytes, _ := json.Marshal(bctemplate.Template{Name: "legacy-t", Description: "from ws"})
	if err := os.WriteFile(filepath.Join(legacyTmpl, "legacy-t.json"), jsonBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyTmpl, "legacy-t.md"), []byte("prompt"), 0o600); err != nil {
		t.Fatal(err)
	}

	migratedDir := filepath.Join(wsLegacy, ".migrated")
	if err := os.MkdirAll(migratedDir, 0o750); err != nil {
		t.Fatal(err)
	}

	n, err := migrateTemplates(wsLegacy, migratedDir)
	if err != nil {
		t.Fatalf("migrateTemplates: %v", err)
	}
	if n != 1 {
		t.Errorf("migrated %d templates, want 1", n)
	}

	// ~/.bc/templates/legacy-t.json must exist.
	globalDir, _ := bcworkspace.GlobalTemplatesDir()
	if _, err := os.Stat(filepath.Join(globalDir, "legacy-t.json")); err != nil {
		t.Errorf("global template missing: %v", err)
	}
	// Legacy file must be moved into .migrated.
	if _, err := os.Stat(filepath.Join(migratedDir, "templates", "legacy-t.json")); err != nil {
		t.Errorf("legacy file not moved to .migrated: %v", err)
	}
	// And the legacy copy removed.
	if _, err := os.Stat(filepath.Join(legacyTmpl, "legacy-t.json")); err == nil {
		t.Errorf("legacy template still present")
	}
}

func TestMigrateSecrets(t *testing.T) {
	_ = setupBCHome(t)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	wsRoot := t.TempDir()
	wsLegacy := filepath.Join(wsRoot, ".bc")
	if err := os.MkdirAll(wsLegacy, 0o750); err != nil {
		t.Fatal(err)
	}
	migratedDir := filepath.Join(wsLegacy, ".migrated")
	if err := os.MkdirAll(migratedDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Seed a legacy secrets.db with a single key.
	src, err := bcsecret.NewStore(wsRoot, "unit-test")
	if err != nil {
		t.Fatal(err)
	}
	if setErr := src.Set("LEGACY_KEY", "sk-legacy", "from ws"); setErr != nil {
		t.Fatal(setErr)
	}
	_ = src.Close()

	n, err := migrateSecrets(wsLegacy, migratedDir)
	if err != nil {
		t.Fatalf("migrateSecrets: %v", err)
	}
	if n != 1 {
		t.Errorf("migrated %d secrets, want 1", n)
	}

	// Global vault now holds the secret.
	vaultPath, _ := bcworkspace.GlobalSecretsVault()
	dst, err := bcsecret.OpenVaultFile(vaultPath, "unit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close() //nolint:errcheck
	got, err := dst.GetValue("LEGACY_KEY")
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}
	if got != "sk-legacy" {
		t.Errorf("got %q, want sk-legacy", got)
	}

	// Legacy file moved.
	if _, err := os.Stat(filepath.Join(migratedDir, "secrets.db")); err != nil {
		t.Errorf("legacy secrets not moved: %v", err)
	}
}

func TestMigrateMCPs(t *testing.T) {
	_ = setupBCHome(t)

	wsRoot := t.TempDir()
	wsLegacy := filepath.Join(wsRoot, ".bc")
	if err := os.MkdirAll(wsLegacy, 0o750); err != nil {
		t.Fatal(err)
	}
	migratedDir := filepath.Join(wsLegacy, ".migrated")
	if err := os.MkdirAll(migratedDir, 0o750); err != nil {
		t.Fatal(err)
	}

	// Legacy .mcp.json with one server entry.
	legacy := map[string]any{
		"mcpServers": map[string]any{
			"gh": map[string]any{
				"command": "npx",
				"args":    []string{"-y", "@github/mcp"},
				"env":     map[string]string{"GITHUB_TOKEN": "${secret:GITHUB_TOKEN}"},
			},
		},
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(wsLegacy, ".mcp.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := migrateMCPs(wsLegacy, migratedDir)
	if err != nil {
		t.Fatalf("migrateMCPs: %v", err)
	}
	if n != 1 {
		t.Errorf("migrated %d MCPs, want 1", n)
	}

	globalPath, _ := bcworkspace.GlobalMCPConfig()
	gs := bcmcp.NewGlobalStore(globalPath)
	got, _ := gs.Get("gh")
	if got == nil {
		t.Fatal("mcp 'gh' missing from global registry")
	}
	if got.Command != "npx" {
		t.Errorf("command lost: %+v", got)
	}
	if got.Env["GITHUB_TOKEN"] == "" {
		t.Errorf("env lost: %+v", got.Env)
	}

	if _, err := os.Stat(filepath.Join(migratedDir, ".mcp.json")); err != nil {
		t.Errorf("legacy .mcp.json not moved: %v", err)
	}
}

func TestRunMigrateUserAssetsWritesMarker(t *testing.T) {
	home := setupBCHome(t)
	t.Setenv("BC_SECRET_PASSPHRASE", "unit-test")

	// No workspaces registered → still writes marker.
	if err := RunMigrateUserAssets(context.Background(), false); err != nil {
		t.Fatalf("RunMigrateUserAssets: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, migrationMarkerName)); err != nil {
		t.Errorf("marker missing: %v", err)
	}
}
