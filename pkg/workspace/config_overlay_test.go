package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupOverlayWorkspace initializes a workspace under an isolated
// MYCEL_HOME and returns the workspace root and its global state dir.
// The Init-written preferences.json has ui.theme = "dark" (default).
func setupOverlayWorkspace(t *testing.T) (root, stateDir string) {
	t.Helper()
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("MYCEL_STATE_DIR", "")
	t.Setenv("BC_HOME", "")
	t.Setenv("BC_STATE_DIR", "")

	root = t.TempDir()
	gitInitDir(t, root)
	ws, err := Init(root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	return root, ws.StateDir()
}

// writeConfigJSON writes cfg as JSON to path and pins its mtime.
func writeConfigJSON(t *testing.T, path string, cfg Config, mtime time.Time) {
	t.Helper()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// writeRawFile writes raw bytes to path and pins its mtime.
func writeRawFile(t *testing.T, path string, data []byte, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestLoadSettingsOverlayPrecedence(t *testing.T) {
	base := time.Now().Add(-24 * time.Hour).Truncate(time.Second)

	overlayCfg := DefaultConfig()
	overlayCfg.UI.Theme = "light"
	overlayCfg.Server.Port = 9999
	overlayData, err := json.Marshal(overlayCfg)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct { //nolint:govet // test struct, readability over alignment
		overlayData []byte
		name        string
		// overlay file location: "state" = <stateDir>/settings.json,
		// "project" = <root>/.bc/settings.json, "" = no overlay file.
		overlayLoc     string
		wantTheme      string
		wantPrefsTheme string
		overlayOffset  time.Duration // overlay mtime relative to prefs
		wantPort       int
		wantPersisted  bool // merged result written back to preferences.json
	}{
		{
			name:           "prefs only",
			overlayLoc:     "",
			wantTheme:      "dark",
			wantPort:       9374,
			wantPrefsTheme: "dark",
		},
		{
			name:           "settings newer than prefs: overlay wins and persists",
			overlayLoc:     "state",
			overlayData:    overlayData,
			overlayOffset:  2 * time.Hour,
			wantTheme:      "light",
			wantPort:       9999,
			wantPersisted:  true,
			wantPrefsTheme: "light",
		},
		{
			name:           "prefs newer than settings: prefs win",
			overlayLoc:     "state",
			overlayData:    overlayData,
			overlayOffset:  -2 * time.Hour,
			wantTheme:      "dark",
			wantPort:       9374,
			wantPrefsTheme: "dark",
		},
		{
			name:           "equal mtimes: prefs win",
			overlayLoc:     "state",
			overlayData:    overlayData,
			overlayOffset:  0,
			wantTheme:      "dark",
			wantPort:       9374,
			wantPrefsTheme: "dark",
		},
		{
			name:           "malformed overlay: skipped with warning, prefs survive",
			overlayLoc:     "state",
			overlayData:    []byte("{{not json"),
			overlayOffset:  2 * time.Hour,
			wantTheme:      "dark",
			wantPort:       9374,
			wantPrefsTheme: "dark",
		},
		{
			name:           "project .bc/settings.json newer: overlay wins and persists",
			overlayLoc:     "project",
			overlayData:    overlayData,
			overlayOffset:  2 * time.Hour,
			wantTheme:      "light",
			wantPort:       9999,
			wantPersisted:  true,
			wantPrefsTheme: "light",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, stateDir := setupOverlayWorkspace(t)
			prefsPath := filepath.Join(stateDir, PreferencesFileName)

			// Pin the prefs mtime so overlay offsets are deterministic.
			prefsCfg := DefaultConfig()
			writeConfigJSON(t, prefsPath, prefsCfg, base)

			var overlayPath string
			switch tt.overlayLoc {
			case "state":
				overlayPath = filepath.Join(stateDir, LegacySettingsFileName)
			case "project":
				overlayPath = filepath.Join(root, ".bc", LegacySettingsFileName)
			}
			if overlayPath != "" {
				writeRawFile(t, overlayPath, tt.overlayData, base.Add(tt.overlayOffset))
			}

			ws, err := Load(root)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if got := ws.Config.UI.Theme; got != tt.wantTheme {
				t.Errorf("ui.theme = %q, want %q", got, tt.wantTheme)
			}
			if got := ws.Config.Server.Port; got != tt.wantPort {
				t.Errorf("server.port = %d, want %d", got, tt.wantPort)
			}

			// Verify what landed (or didn't) in preferences.json.
			persisted, loadErr := LoadConfig(prefsPath)
			if loadErr != nil {
				t.Fatalf("reload preferences.json: %v", loadErr)
			}
			if persisted.UI.Theme != tt.wantPrefsTheme {
				t.Errorf("persisted ui.theme = %q, want %q", persisted.UI.Theme, tt.wantPrefsTheme)
			}

			info, statErr := os.Stat(prefsPath)
			if statErr != nil {
				t.Fatalf("stat preferences.json: %v", statErr)
			}
			if tt.wantPersisted && !info.ModTime().After(base) {
				t.Error("preferences.json should have been rewritten (mtime unchanged)")
			}
			if !tt.wantPersisted && !info.ModTime().Equal(base) {
				t.Errorf("preferences.json rewritten on a read-only load (mtime %v, want %v)",
					info.ModTime(), base)
			}
		})
	}
}

// TestLoadLegacyOnlyNoPromotion verifies that loading a workspace whose
// only config is a legacy settings.json does NOT write preferences.json
// as a side effect (#3239: no save-on-read promotion).
func TestLoadLegacyOnlyNoPromotion(t *testing.T) {
	t.Setenv("MYCEL_HOME", t.TempDir())
	t.Setenv("MYCEL_STATE_DIR", "")
	t.Setenv("BC_HOME", "")
	t.Setenv("BC_STATE_DIR", "")

	root := t.TempDir()
	gitInitDir(t, root)
	legacyDir := filepath.Join(root, ".bc")
	if err := os.MkdirAll(filepath.Join(legacyDir, "roles"), 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.UI.Theme = "light"
	if err := cfg.Save(filepath.Join(legacyDir, LegacySettingsFileName)); err != nil {
		t.Fatal(err)
	}

	ws, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.UI.Theme != "light" {
		t.Errorf("ui.theme = %q, want %q", ws.Config.UI.Theme, "light")
	}

	if _, statErr := os.Stat(filepath.Join(legacyDir, PreferencesFileName)); !os.IsNotExist(statErr) {
		t.Errorf("preferences.json promoted on read in %s; want no write", legacyDir)
	}
	if gDir, gErr := GlobalStateDir(root); gErr == nil {
		if _, statErr := os.Stat(filepath.Join(gDir, PreferencesFileName)); !os.IsNotExist(statErr) {
			t.Errorf("preferences.json promoted on read in %s; want no write", gDir)
		}
	}
}

func TestApplyOverlaySectionSemantics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gateways.Slack = &SlackGatewayConfig{BotToken: "xoxb-1", Enabled: true}

	// Overlay: only ui + gateways(telegram). Other sections must survive;
	// slack must survive the per-key gateway merge.
	overlay := []byte(`{
		"ui": {"theme": "light", "mode": "dark", "default_view": "agents"},
		"gateways": {"telegram": {"bot_token": "tg-1", "enabled": true}},
		"version": 99,
		"unknown_section": {"x": 1}
	}`)

	if err := cfg.ApplyOverlay(overlay); err != nil {
		t.Fatalf("ApplyOverlay: %v", err)
	}

	if cfg.UI.Theme != "light" || cfg.UI.Mode != "dark" || cfg.UI.DefaultView != "agents" {
		t.Errorf("ui section not replaced: %+v", cfg.UI)
	}
	if cfg.Server.Port != 9374 {
		t.Errorf("untouched server section changed: port = %d", cfg.Server.Port)
	}
	if cfg.Version != ConfigVersion {
		t.Errorf("version overridden to %d; overlay version must be ignored", cfg.Version)
	}
	if cfg.Gateways.Slack == nil || cfg.Gateways.Slack.BotToken != "xoxb-1" {
		t.Errorf("slack gateway wiped by telegram-only overlay: %+v", cfg.Gateways.Slack)
	}
	if cfg.Gateways.Telegram == nil || cfg.Gateways.Telegram.BotToken != "tg-1" {
		t.Errorf("telegram gateway not merged: %+v", cfg.Gateways.Telegram)
	}
}

func TestApplyOverlayMalformed(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.ApplyOverlay([]byte("{{nope")); err == nil {
		t.Fatal("ApplyOverlay(malformed): expected error, got nil")
	}
	if cfg.UI.Theme != "dark" {
		t.Errorf("config mutated by failed overlay: theme = %q", cfg.UI.Theme)
	}
}

func TestConfigDriftSections(t *testing.T) {
	dir := t.TempDir()
	active := DefaultConfig()
	activePath := filepath.Join(dir, PreferencesFileName)
	if err := active.Save(activePath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		other   func() Config
		want    []string
		wantErr bool
	}{
		{
			name:  "identical configs: no drift",
			other: DefaultConfig,
			want:  nil,
		},
		{
			name: "ui and server drift",
			other: func() Config {
				c := DefaultConfig()
				c.UI.Theme = "light"
				c.Server.Port = 9999
				return c
			},
			want: []string{"server", "ui"},
		},
		{
			name: "storage drift (the #3239 outage shape)",
			other: func() Config {
				c := DefaultConfig()
				c.Storage.Default = "timescale"
				return c
			},
			want: []string{"storage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			otherPath := filepath.Join(t.TempDir(), LegacySettingsFileName)
			oc := tt.other()
			if err := oc.Save(otherPath); err != nil {
				t.Fatal(err)
			}
			got, err := ConfigDriftSections(activePath, otherPath)
			if err != nil {
				t.Fatalf("ConfigDriftSections: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("drift = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("drift = %v, want %v", got, tt.want)
				}
			}
		})
	}

	t.Run("malformed other file errors", func(t *testing.T) {
		otherPath := filepath.Join(t.TempDir(), LegacySettingsFileName)
		if err := os.WriteFile(otherPath, []byte("{{bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ConfigDriftSections(activePath, otherPath); err == nil {
			t.Fatal("expected error for malformed settings.json")
		}
	})
}
