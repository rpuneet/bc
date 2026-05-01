package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/workspace"
)

func newTestWorkspace(t *testing.T) *workspace.Workspace {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".bc")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := &workspace.Config{
		Version: workspace.ConfigVersion,
		Providers: workspace.ProvidersConfig{
			Default:   "claude",
			Providers: map[string]workspace.ProviderConfig{"claude": {Command: "claude"}},
		},
		Runtime: workspace.RuntimeConfig{Default: "tmux"},
		Server:  workspace.ServerConfig{Host: "127.0.0.1", Port: 9374, CORSOrigin: "*"},
		UI:      workspace.UIConfig{Theme: "dark", Mode: "auto"},
	}
	return &workspace.Workspace{
		Config:  cfg,
		RootDir: dir,
	}
}

func TestSettingsPatchSection(t *testing.T) {
	ws := newTestWorkspace(t)
	h := NewSettingsHandler(ws)

	mux := http.NewServeMux()
	h.Register(mux)

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
	ws := newTestWorkspace(t)
	h := NewSettingsHandler(ws)

	mux := http.NewServeMux()
	h.Register(mux)

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

	if ws.Config.User.Name != "bob" {
		t.Errorf("config.User.Name = %q, want %q", ws.Config.User.Name, "bob")
	}
}

func TestSettingsPatchMethodNotAllowed(t *testing.T) {
	ws := newTestWorkspace(t)
	h := NewSettingsHandler(ws)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/settings", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestSettingsPatchAllSections(t *testing.T) {
	ws := newTestWorkspace(t)
	h := NewSettingsHandler(ws)

	mux := http.NewServeMux()
	h.Register(mux)

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

	if ws.Config.User.Name != "test" {
		t.Errorf("User.Name = %q, want %q", ws.Config.User.Name, "test")
	}
	if ws.Config.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", ws.Config.Server.Port, 8080)
	}
	if ws.Config.UI.Theme != "light" {
		t.Errorf("UI.Theme = %q, want %q", ws.Config.UI.Theme, "light")
	}
}
