package handlers

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAvatarProxyPath(t *testing.T) {
	if got := avatarProxyPath(""); got != "" {
		t.Fatalf("empty url should proxy to empty string, got %q", got)
	}
	raw := "https://pps.whatsapp.net/v/t61.0-24/alice.jpg?ccb=11-4&oh=x&oe=y"
	got := avatarProxyPath(raw)
	if !strings.HasPrefix(got, "/api/apps/avatar?u=") {
		t.Fatalf("proxy path = %q, want /api/apps/avatar prefix", got)
	}
	// The raw CDN URL must never appear verbatim in the proxied path.
	if strings.Contains(got, "pps.whatsapp.net") {
		t.Fatalf("proxy path leaks raw host: %q", got)
	}
	// It must round-trip back to the original URL.
	enc := strings.TrimPrefix(got, "/api/apps/avatar?u=")
	dec, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(dec) != raw {
		t.Fatalf("round-trip = %q, want %q", dec, raw)
	}
}

func TestAvatarHostAllowed(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"avatars.slack-edge.com", true},
		{"files.slack.com", true},
		{"secure.gravatar.com", true},
		{"pps.whatsapp.net", true},
		{"media.whatsapp.net", true},
		// SSRF attempts that must be rejected.
		{"evil.com", false},
		{"169.254.169.254", false},
		{"localhost", false},
		{"whatsapp.net.attacker.com", false},
		{"notslack-edge.com", false},
	}
	for _, tt := range tests {
		if got := avatarHostAllowed(tt.host); got != tt.want {
			t.Errorf("avatarHostAllowed(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestAvatarCheckRedirect(t *testing.T) {
	mkReq := func(rawURL string) *http.Request {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse %q: %v", rawURL, err)
		}
		return &http.Request{URL: u}
	}

	// Redirect that stays on an allowlisted host is followed.
	if err := avatarCheckRedirect(mkReq("https://media.whatsapp.net/x.jpg"), nil); err != nil {
		t.Fatalf("allowlisted redirect rejected: %v", err)
	}
	// Redirect to a non-allowlisted host is blocked (SSRF via 30x).
	if err := avatarCheckRedirect(mkReq("https://169.254.169.254/latest/meta-data/"), nil); err == nil {
		t.Fatal("redirect to disallowed host should be blocked")
	}
	if err := avatarCheckRedirect(mkReq("https://evil.com/steal"), nil); err == nil {
		t.Fatal("redirect to evil.com should be blocked")
	}
	// Redirect chains are capped even when every hop is allowlisted.
	via := make([]*http.Request, 5)
	if err := avatarCheckRedirect(mkReq("https://pps.whatsapp.net/x.jpg"), via); err == nil {
		t.Fatal("redirect chain over the cap should be blocked")
	}
}

func TestAvatarProxy_Guards(t *testing.T) {
	h := &GatewayHandler{}

	// Wrong method → 405.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/apps/avatar?u=x", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	h.avatarProxy(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rr.Code)
	}

	// Non-loopback caller → 403 (never fetches).
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/apps/avatar?u=x", nil)
	req.RemoteAddr = "10.0.0.5:5000"
	h.avatarProxy(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status = %d, want 403", rr.Code)
	}

	// Missing u → 400.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/apps/avatar", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	h.avatarProxy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing-u status = %d, want 400", rr.Code)
	}

	// Disallowed host → 403, and the daemon must not attempt the fetch.
	rr = httptest.NewRecorder()
	enc := base64.RawURLEncoding.EncodeToString([]byte("https://169.254.169.254/latest/meta-data/"))
	req = httptest.NewRequest(http.MethodGet, "/api/apps/avatar?u="+enc, nil)
	req.RemoteAddr = "127.0.0.1:5000"
	h.avatarProxy(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("disallowed-host status = %d, want 403", rr.Code)
	}

	// Non-https scheme → 400.
	rr = httptest.NewRecorder()
	enc = base64.RawURLEncoding.EncodeToString([]byte("http://pps.whatsapp.net/x.jpg"))
	req = httptest.NewRequest(http.MethodGet, "/api/apps/avatar?u="+enc, nil)
	req.RemoteAddr = "127.0.0.1:5000"
	h.avatarProxy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("http-scheme status = %d, want 400", rr.Code)
	}
}
