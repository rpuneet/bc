package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// fakePlugin is a token-auth test app with one secret and one required
// plain field, registered under "fakeapp".
type fakePlugin struct{}

func (fakePlugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "fakeapp",
		Label: "Fake App",
		Auth:  app.AuthToken,
		Multi: true,
		Fields: []app.FieldSpec{
			{Key: "token", Label: "Token", Secret: true, Required: true},
			{Key: "region", Label: "Region", Required: true},
			{Key: "note", Label: "Note"},
		},
		Docs: []string{"test fixture"},
	}
}

func (fakePlugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	if _, err := inst.RequiredSecret("token"); err != nil {
		return nil, err
	}
	return &stubAdapter{name: inst.Name}, nil
}

// fakeQRAdapter implements app.QRPairer on top of stubAdapter.
type fakeQRAdapter struct {
	stubAdapter
}

func (f *fakeQRAdapter) StartPairing(_ context.Context) (app.PairInfo, error) {
	return app.PairInfo{State: "qr_ready", QRDataURL: "data:image/png;base64,x"}, nil
}

func (f *fakeQRAdapter) PairStatus() app.PairInfo {
	return app.PairInfo{State: "qr_ready"}
}

// fakeQRPlugin is a QR-auth test app registered under "fakeqr".
type fakeQRPlugin struct{}

func (fakeQRPlugin) Describe() app.Descriptor {
	return app.Descriptor{ID: "fakeqr", Label: "Fake QR", Auth: app.AuthQR, Docs: []string{"test fixture"}}
}

func (fakeQRPlugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return &fakeQRAdapter{stubAdapter{name: inst.Name}}, nil
}

func init() {
	app.Register(fakePlugin{})
	app.Register(fakeQRPlugin{})
}

// newAppsTestHandler assembles an AppsHandler with a real registry, a
// temp-dir vault, a sandboxed workspace, and a started gateway manager.
func newAppsTestHandler(t *testing.T) (*AppsHandler, *gateway.Manager) {
	t.Helper()
	wks := setupTestWorkspace(t)
	vault := openTestVault(t)

	gw := gateway.NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	gw.SetStartContext(ctx)

	gh := NewGatewayHandler(gw, wks)
	return NewAppsHandler(gh, gw, wks, vault), gw
}

func TestAppsCatalog(t *testing.T) {
	h, _ := newAppsTestHandler(t)
	h.ws.Config.Apps = map[string]app.InstanceConfig{
		"fakeapp:ci": {App: "fakeapp", Enabled: true, Config: map[string]string{"region": "eu"}},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	rr := httptest.NewRecorder()
	h.catalog(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Catalog []struct {
			ID     string `json:"id"`
			Auth   string `json:"auth"`
			Fields []struct {
				Key    string `json:"key"`
				Secret bool   `json:"secret"`
			} `json:"fields"`
		} `json:"catalog"`
		Instances []struct { //nolint:govet // test-only struct, field order mirrors JSON
			Name     string         `json:"name"`
			App      string         `json:"app"`
			Enabled  bool           `json:"enabled"`
			Config   map[string]any `json:"config"`
			Channels []string       `json:"channels"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	foundFake := false
	for _, d := range resp.Catalog {
		if d.ID == "fakeapp" {
			foundFake = true
			if d.Auth != "token" {
				t.Errorf("fakeapp auth = %q, want token", d.Auth)
			}
		}
	}
	if !foundFake {
		t.Error("catalog missing fakeapp descriptor")
	}
	if len(resp.Instances) != 1 || resp.Instances[0].Name != "fakeapp:ci" || !resp.Instances[0].Enabled {
		t.Fatalf("instances = %+v, want one enabled fakeapp:ci", resp.Instances)
	}

	inst := resp.Instances[0]
	// Channels is always an array, never null.
	if inst.Channels == nil {
		t.Error("instance channels should be [], not null")
	}
	// Secret fields surface as has_<field> booleans; the vault is empty
	// here so the token reads as not configured.
	if got, ok := inst.Config["has_token"]; !ok || got != false {
		t.Errorf("config has_token = %v (present %v), want false", got, ok)
	}
	if inst.Config["region"] != "eu" {
		t.Errorf("config region = %v, want eu", inst.Config["region"])
	}
}

// TestAppsCatalogConfiguredSecretAndChannels verifies has_<field> flips
// once the vault holds a value and that discovered adapter channels are
// attached to their instance.
func TestAppsCatalogConfiguredSecretAndChannels(t *testing.T) {
	h, gw := newAppsTestHandler(t)
	h.ws.Config.Apps = map[string]app.InstanceConfig{
		"fakeapp:ci": {App: "fakeapp", Enabled: true, Config: map[string]string{"region": "eu"}},
	}
	if err := h.vault.Set("app:fakeapp:ci:token", "sekret", "app credential"); err != nil {
		t.Fatalf("seed vault: %v", err)
	}
	// Seed a discovered channel for the instance via an inbound notification.
	gw.Register(&stubAdapter{name: "fakeapp:ci"})
	gw.HandleNotification("fakeapp:ci", gateway.Notification{
		Channel: "ci-alerts", ChannelID: "C1", Platform: "fakeapp", Sender: "bot", Content: "hi",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	rr := httptest.NewRecorder()
	h.catalog(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Instances []struct {
			Name     string         `json:"name"`
			Config   map[string]any `json:"config"`
			Channels []string       `json:"channels"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Instances) != 1 {
		t.Fatalf("instances = %+v, want one", resp.Instances)
	}
	inst := resp.Instances[0]
	if got := inst.Config["has_token"]; got != true {
		t.Errorf("has_token = %v, want true", got)
	}
	found := false
	for _, ch := range inst.Channels {
		if strings.HasPrefix(ch, "fakeapp:ci:") {
			found = true
		}
	}
	if !found {
		t.Errorf("channels = %v, want one with fakeapp:ci: prefix", inst.Channels)
	}
	// The raw secret value never appears in the catalog payload.
	if strings.Contains(rr.Body.String(), "sekret") {
		t.Error("secret value leaked into catalog response")
	}
}

func TestAppsUpdateSplitsSecretsFromConfig(t *testing.T) {
	h, gw := newAppsTestHandler(t)

	body := `{"app":"fakeapp","enabled":true,"config":{"token":"sekret-token","region":"eu","note":"hi"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/apps/fakeapp:ci", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.update(rr, req, "fakeapp:ci")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	// Secret went to the vault under app:<instance>:<key>.
	if got, err := h.vault.GetValue("app:fakeapp:ci:token"); err != nil || got != "sekret-token" {
		t.Errorf("vault token = %q (err %v), want sekret-token", got, err)
	}

	// Plain fields persisted in config; the secret did not.
	ic, ok := h.ws.Config.Apps["fakeapp:ci"]
	if !ok {
		t.Fatal("instance not persisted in config")
	}
	if ic.App != "fakeapp" || !ic.Enabled {
		t.Errorf("instance = %+v, want enabled fakeapp", ic)
	}
	if ic.Config["region"] != "eu" || ic.Config["note"] != "hi" {
		t.Errorf("plain config = %v", ic.Config)
	}
	if _, present := ic.Config["token"]; present {
		t.Error("secret field leaked into plain config")
	}

	// The saved preferences file must never contain the secret value.
	data, err := os.ReadFile(h.ws.SettingsFile())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(data), "sekret-token") {
		t.Error("secret value leaked into preferences.json")
	}

	// Adapter was hot-started.
	if gw.GetAdapter("fakeapp:ci") == nil {
		t.Error("adapter not hot-started after update")
	}
}

func TestAppsUpdateRejectsUnknownAppAndField(t *testing.T) {
	h, _ := newAppsTestHandler(t)

	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{"unknown app", "nope", `{"app":"nope","config":{}}`, http.StatusBadRequest},
		{"unknown field", "fakeapp", `{"config":{"bogus":"x","region":"eu"}}`, http.StatusBadRequest},
		{"missing required plain field", "fakeapp", `{"config":{"token":"t"}}`, http.StatusBadRequest},
		{"label on non-multi app", "fakeqr:two", `{"config":{}}`, http.StatusBadRequest},
		{"name/app mismatch", "fakeapp:ci", `{"app":"fakeqr","config":{}}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/apps/"+tt.path, strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			h.update(rr, req, tt.path)
			if rr.Code != tt.want {
				t.Errorf("status = %d, want %d; body = %s", rr.Code, tt.want, rr.Body.String())
			}
		})
	}
}

func TestAppsDeleteRemovesConfigVaultAndState(t *testing.T) {
	h, _ := newAppsTestHandler(t)

	// Seed: connected instance with vault secret and a state dir.
	h.ws.Config.Apps = map[string]app.InstanceConfig{
		"fakeapp:ci": {App: "fakeapp", Enabled: true, Config: map[string]string{"region": "eu"}},
	}
	if err := h.vault.Set("app:fakeapp:ci:token", "sekret", "app credential"); err != nil {
		t.Fatalf("seed vault: %v", err)
	}
	stateDir := h.stateDir("fakeapp:ci")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("seed state dir: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/apps/fakeapp:ci", nil)
	rr := httptest.NewRecorder()
	h.delete(rr, req, "fakeapp:ci")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if _, ok := h.ws.Config.Apps["fakeapp:ci"]; ok {
		t.Error("instance still in config after delete")
	}
	if _, err := h.vault.GetValue("app:fakeapp:ci:token"); err == nil {
		t.Error("vault secret still present after delete")
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Errorf("state dir still present after delete: %v", err)
	}
}

func TestAppsDeleteUnknownInstance(t *testing.T) {
	h, _ := newAppsTestHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/apps/ghost", nil)
	rr := httptest.NewRecorder()
	h.delete(rr, req, "ghost")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestAppsAuthQRFlow(t *testing.T) {
	h, gw := newAppsTestHandler(t)

	// POST /api/apps/fakeqr/auth — pair-first: instance is created,
	// adapter built and started, QRPairer dispatched.
	req := httptest.NewRequest(http.MethodPost, "/api/apps/fakeqr/auth", nil)
	rr := httptest.NewRecorder()
	h.auth(rr, req, "fakeqr")

	if rr.Code != http.StatusOK {
		t.Fatalf("auth status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var info app.PairInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode pair info: %v", err)
	}
	if info.State != "qr_ready" || info.QRDataURL == "" {
		t.Errorf("pair info = %+v, want qr_ready with QR data", info)
	}
	if _, ok := h.ws.Config.Apps["fakeqr"]; !ok {
		t.Error("pair-first flow did not persist the instance")
	}
	if gw.GetAdapter("fakeqr") == nil {
		t.Error("adapter not started by auth flow")
	}

	// GET /api/apps/fakeqr/auth/status polls the running adapter.
	req2 := httptest.NewRequest(http.MethodGet, "/api/apps/fakeqr/auth/status", nil)
	rr2 := httptest.NewRecorder()
	h.authStatus(rr2, req2, "fakeqr")
	if rr2.Code != http.StatusOK {
		t.Fatalf("auth/status = %d, want 200; body = %s", rr2.Code, rr2.Body.String())
	}
	if !strings.Contains(rr2.Body.String(), "qr_ready") {
		t.Errorf("auth/status body = %s, want qr_ready", rr2.Body.String())
	}
}

func TestAppsAuthNotSupported(t *testing.T) {
	h, _ := newAppsTestHandler(t)

	// Configure a token app whose adapter has no QRPairer capability.
	if err := h.vault.Set("app:fakeapp:token", "tok", "app credential"); err != nil {
		t.Fatalf("seed vault: %v", err)
	}
	h.ws.Config.Apps = map[string]app.InstanceConfig{
		"fakeapp": {App: "fakeapp", Enabled: true, Config: map[string]string{"region": "eu"}},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/apps/fakeapp/auth", nil)
	rr := httptest.NewRecorder()
	h.auth(rr, req, "fakeapp")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

// TestAppsRoutes verifies the /api/apps surface end to end through the
// mux — auth flows, delegated per-instance routes, the channel surface
// under /api/apps/channels, and that the removed /api/gateways aliases
// really are gone.
func TestAppsRoutes(t *testing.T) {
	h, _ := newAppsTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	h.gh.Register(mux)

	// Auth flow through the mux.
	req := httptest.NewRequest(http.MethodPost, "/api/apps/fakeqr/auth", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "qr_ready") {
		t.Errorf("auth body = %s, want qr_ready", rr.Body.String())
	}

	// Auth status poll.
	req2 := httptest.NewRequest(http.MethodGet, "/api/apps/fakeqr/auth/status", nil)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("auth/status = %d, want 200", rr2.Code)
	}

	// /api/apps/{name}/health delegates to the shared gateway route.
	req3 := httptest.NewRequest(http.MethodGet, "/api/apps/fakeqr/health", nil)
	rr3 := httptest.NewRecorder()
	mux.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("apps health = %d, want 200; body = %s", rr3.Code, rr3.Body.String())
	}

	// The channel surface lives under /api/apps/channels — the longest
	// pattern wins over the /api/apps/{name} instance router.
	req4 := httptest.NewRequest(http.MethodGet, "/api/apps/channels", nil)
	rr4 := httptest.NewRecorder()
	mux.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("apps channels = %d, want 200; body = %s", rr4.Code, rr4.Body.String())
	}
	req5 := httptest.NewRequest(http.MethodGet, "/api/apps/channels/fakeqr:x/history", nil)
	rr5 := httptest.NewRecorder()
	mux.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusOK {
		t.Fatalf("apps channel history = %d, want 200; body = %s", rr5.Code, rr5.Body.String())
	}

	// The transitional /api/gateways/* aliases are gone.
	for _, alias := range []string{
		"/api/gateways",
		"/api/gateways/fakeqr/pair",
		"/api/gateways/fakeqr/health",
		"/api/gateways/activity",
	} {
		reqA := httptest.NewRequest(http.MethodGet, alias, nil)
		rrA := httptest.NewRecorder()
		mux.ServeHTTP(rrA, reqA)
		if rrA.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404 (alias removed)", alias, rrA.Code)
		}
	}

	// Filesystem cleanup for the pair-first instance state.
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(h.ws.StateDir(), "apps")) })
}
