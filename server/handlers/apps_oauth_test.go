package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/app"
	"github.com/rpuneet/mycel/pkg/gateway"
)

// fakeOAuthPlugin is an OAuth-capable test app registered under
// "fakeoauth". Poll results are scripted per test via setPollResult.
type fakeOAuthPlugin struct {
	beginConfig map[string]string
	pollResult  app.AuthResult
	mu          sync.Mutex
}

const fakeOAuthSessionID = "oauth-sess-1"

func (p *fakeOAuthPlugin) Describe() app.Descriptor {
	return app.Descriptor{
		ID:    "fakeoauth",
		Label: "Fake OAuth",
		Auth:  app.AuthToken,
		Fields: []app.FieldSpec{
			{Key: "oauth_client_id", Label: "OAuth Client ID"},
			{Key: "api_token", Label: "API Token", Secret: true},
		},
		Docs: []string{"test fixture"},
	}
}

func (p *fakeOAuthPlugin) Build(inst app.Instance, _ app.Env) (gateway.NotificationAdapter, error) {
	return &stubAdapter{name: inst.Name}, nil
}

func (p *fakeOAuthPlugin) BeginAuth(_ context.Context, inst app.Instance) (app.AuthSession, error) {
	if inst.Config["oauth_client_id"] == "" {
		return app.AuthSession{}, errors.New("oauth_client_id is not set")
	}
	p.mu.Lock()
	p.beginConfig = inst.Config
	p.mu.Unlock()
	return app.AuthSession{
		ID:              fakeOAuthSessionID,
		Kind:            app.AuthKindDevice,
		VerificationURL: "https://example.com/login/device",
		UserCode:        "WXYZ-9876",
		ExpiresAt:       time.Now().Add(10 * time.Minute),
		Interval:        5 * time.Second,
	}, nil
}

func (p *fakeOAuthPlugin) PollAuth(_ context.Context, session app.AuthSession) (app.AuthResult, error) {
	if session.ID != fakeOAuthSessionID {
		return app.AuthResult{State: app.AuthStateError, Error: "unknown session"}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pollResult.State == "" {
		return app.AuthResult{State: app.AuthStatePending}, nil
	}
	return p.pollResult, nil
}

func (p *fakeOAuthPlugin) setPollResult(res app.AuthResult) {
	p.mu.Lock()
	p.pollResult = res
	p.mu.Unlock()
}

var oauthFake = &fakeOAuthPlugin{}

func init() {
	app.Register(oauthFake)
}

func TestAppsCatalogReportsOAuthAvailability(t *testing.T) {
	h, _ := newAppsTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	rr := httptest.NewRecorder()
	h.catalog(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Catalog []struct {
			ID             string `json:"id"`
			OAuthAvailable bool   `json:"oauth_available"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]bool{}
	for _, d := range resp.Catalog {
		got[d.ID] = d.OAuthAvailable
	}
	if !got["fakeoauth"] {
		t.Error("fakeoauth oauth_available = false, want true")
	}
	if got["fakeapp"] {
		t.Error("fakeapp oauth_available = true, want false")
	}
	if avail, ok := got["github"]; ok && !avail {
		t.Error("github oauth_available = false, want true (device flow)")
	}
}

func TestAppsOAuthBeginPollPersist(t *testing.T) {
	h, gw := newAppsTestHandler(t)
	oauthFake.setPollResult(app.AuthResult{})

	// Begin: plain config in the body persists with the instance and the
	// session comes back with the device-flow user code.
	body := `{"config":{"oauth_client_id":"Ov23liTEST"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/apps/fakeoauth/auth", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.auth(rr, req, "fakeoauth")
	if rr.Code != http.StatusOK {
		t.Fatalf("begin status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	var sess struct {
		ID              string `json:"id"`
		Kind            string `json:"kind"`
		State           string `json:"state"`
		VerificationURL string `json:"verification_url"`
		UserCode        string `json:"user_code"`
		IntervalSeconds int    `json:"interval_seconds"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if sess.ID != fakeOAuthSessionID || sess.Kind != app.AuthKindDevice || sess.State != app.AuthStatePending {
		t.Errorf("session = %+v", sess)
	}
	if sess.UserCode != "WXYZ-9876" || sess.VerificationURL == "" || sess.IntervalSeconds != 5 {
		t.Errorf("session UX fields = %+v", sess)
	}
	ic, ok := h.ws.Config.Apps["fakeoauth"]
	if !ok || ic.Config["oauth_client_id"] != "Ov23liTEST" {
		t.Fatalf("instance = %+v (ok %v), want persisted oauth_client_id", ic, ok)
	}

	// Pending poll.
	req2 := httptest.NewRequest(http.MethodGet, "/api/apps/fakeoauth/auth/status?session="+sess.ID, nil)
	rr2 := httptest.NewRecorder()
	h.authStatus(rr2, req2, "fakeoauth")
	if rr2.Code != http.StatusOK || !strings.Contains(rr2.Body.String(), app.AuthStatePending) {
		t.Fatalf("pending poll = %d %s", rr2.Code, rr2.Body.String())
	}

	// Complete: secrets persist to the vault, the instance is enabled,
	// the adapter hot-starts, and the token never crosses the wire.
	oauthFake.setPollResult(app.AuthResult{
		State:   app.AuthStateComplete,
		Secrets: map[string]string{"api_token": "gho_oauth_token"},
	})
	req3 := httptest.NewRequest(http.MethodGet, "/api/apps/fakeoauth/auth/status?session="+sess.ID, nil)
	rr3 := httptest.NewRecorder()
	h.authStatus(rr3, req3, "fakeoauth")
	if rr3.Code != http.StatusOK || !strings.Contains(rr3.Body.String(), app.AuthStateComplete) {
		t.Fatalf("complete poll = %d %s", rr3.Code, rr3.Body.String())
	}
	if strings.Contains(rr3.Body.String(), "gho_oauth_token") {
		t.Error("token leaked into the auth/status response")
	}
	if got, err := h.vault.GetValue("app:fakeoauth:api_token"); err != nil || got != "gho_oauth_token" {
		t.Errorf("vault api_token = %q (err %v), want gho_oauth_token", got, err)
	}
	if ic := h.ws.Config.Apps["fakeoauth"]; !ic.Enabled {
		t.Error("instance not enabled after completed auth")
	}
	if gw.GetAdapter("fakeoauth") == nil {
		t.Error("adapter not hot-started after completed auth")
	}
}

func TestAppsOAuthBeginValidatesConfig(t *testing.T) {
	h, _ := newAppsTestHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{"secret field in config", `{"config":{"api_token":"x","oauth_client_id":"id"}}`},
		{"unknown field", `{"config":{"bogus":"x"}}`},
		{"begin error surfaces (missing client id)", `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/apps/fakeoauth/auth", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			h.auth(rr, req, "fakeoauth")
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAppsOAuthStatusRequiresSessionParam(t *testing.T) {
	h, _ := newAppsTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/fakeoauth/auth/status", nil)
	rr := httptest.NewRecorder()
	h.authStatus(rr, req, "fakeoauth")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body = %s", rr.Code, rr.Body.String())
	}
}

func TestAppsOAuthUnknownSessionPolls(t *testing.T) {
	h, _ := newAppsTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/apps/fakeoauth/auth/status?session=ghost", nil)
	rr := httptest.NewRecorder()
	h.authStatus(rr, req, "fakeoauth")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), app.AuthStateError) {
		t.Errorf("body = %s, want error state", rr.Body.String())
	}
}
