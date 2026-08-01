package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/home"
)

func newTestHome(t *testing.T) *home.Home {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".mycel")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := &home.Config{
		Version: home.ConfigVersion,
		Providers: home.ProvidersConfig{
			Default:   "claude",
			Providers: map[string]home.ProviderConfig{"claude": {Command: "claude"}},
		},
		Runtime: home.RuntimeConfig{Default: "tmux"},
		Server:  home.ServerConfig{Host: "127.0.0.1", Port: 9374, CORSOrigin: "*"},
		UI:      home.UIConfig{Theme: "dark", Mode: "auto"},
	}
	return &home.Home{
		Config:  cfg,
		RootDir: dir,
	}
}

func TestSettingsPatchSection(t *testing.T) {
	h := newTestHome(t)
	sh := NewSettingsHandler(h)

	mux := http.NewServeMux()
	sh.Register(mux)

	tests := []struct {
		body       string
		wantErr    string
		name       string
		wantStatus int
	}{
		{
			name:       "patch user section",
			body:       `{"user":{"name":"alice"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "patch runtime section",
			body:       `{"runtime":{"default":"docker"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "patch logs section",
			body:       `{"logs":{"path":"custom/logs"}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown section returns 400",
			body:       `{"bogus":{}}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "unknown section: bogus",
		},
		{
			name:       "gateways section is gone",
			body:       `{"gateways":{"slack":{"bot_token":"x"}}}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "unknown section: gateways",
		},
		{
			name:       "patch onboarding section",
			body:       `{"onboarding":{"step":"runtime","completed":["welcome","system"]}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "patch apps section",
			body:       `{"apps":{"fakeapp":{"app":"fakeapp","enabled":true,"config":{"region":"eu"}}}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "patch notifications section",
			body:       `{"notifications":{"default_channel":"slack:general","enabled":true}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "apps patch with unknown app returns 400",
			body:       `{"apps":{"ghost":{"app":"ghost","enabled":true}}}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "unknown app",
		},
		{
			name:       "apps patch with secret field is rejected",
			body:       `{"apps":{"fakeapp":{"app":"fakeapp","enabled":true,"config":{"region":"eu","token":"leak"}}}}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "must be stored in the vault",
		},
		{
			name:       "invalid JSON returns 400",
			body:       `{not json}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantErr != "" && !strings.Contains(rec.Body.String(), tt.wantErr) {
				t.Errorf("body = %s, want containing %q", rec.Body.String(), tt.wantErr)
			}
		})
	}
}

func TestSettingsPatchUpdatesConfig(t *testing.T) {
	h := newTestHome(t)
	sh := NewSettingsHandler(h)

	mux := http.NewServeMux()
	sh.Register(mux)

	body := `{"user":{"name":"bob"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if h.Config.User.Name != "bob" {
		t.Errorf("config.User.Name = %q, want %q", h.Config.User.Name, "bob")
	}
}

// TestSettingsAppsPatchMerges verifies the per-instance merge: patching
// one instance never wipes the others.
func TestSettingsAppsPatchMerges(t *testing.T) {
	h := newTestHome(t)
	h.Config.Apps = map[string]app.InstanceConfig{
		"fakeqr": {App: "fakeqr", Enabled: true},
	}
	sh := NewSettingsHandler(h)
	mux := http.NewServeMux()
	sh.Register(mux)

	body := `{"apps":{"fakeapp:ci":{"app":"fakeapp","enabled":true,"config":{"region":"eu"}}}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := h.Config.Apps["fakeqr"]; !ok {
		t.Error("existing instance wiped by unrelated patch")
	}
	if ic := h.Config.Apps["fakeapp:ci"]; ic.App != "fakeapp" || ic.Config["region"] != "eu" {
		t.Errorf("patched instance = %+v", ic)
	}
}

func TestSettingsPatchMethodNotAllowed(t *testing.T) {
	h := newTestHome(t)
	sh := NewSettingsHandler(h)

	mux := http.NewServeMux()
	sh.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/settings", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestSettingsPatchAllSections(t *testing.T) {
	h := newTestHome(t)
	sh := NewSettingsHandler(h)

	mux := http.NewServeMux()
	sh.Register(mux)

	body := `{
		"user": {"name": "test"},
		"server": {"host": "0.0.0.0", "port": 8080, "cors_origin": "*"},
		"runtime": {"default": "docker"},
		"ui": {"theme": "light", "mode": "dark"}
	}`
	req := httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if h.Config.User.Name != "test" {
		t.Errorf("User.Name = %q, want %q", h.Config.User.Name, "test")
	}
	if h.Config.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", h.Config.Server.Port, 8080)
	}
	if h.Config.UI.Theme != "light" {
		t.Errorf("UI.Theme = %q, want %q", h.Config.UI.Theme, "light")
	}
}
