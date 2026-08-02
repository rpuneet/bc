package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ghadapter "github.com/rpuneet/mycel/pkg/gateway/github"

	"github.com/rpuneet/mycel/pkg/gateway"
)

// TestChannelSend_GitHubIssueRefFallback verifies the real GitHub adapter's
// outbound comment posting through the generic gateway send path: since
// GitHub has no pre-discovered "channel" (its Channels() are webhook event
// types, not repos), Manager.Send's "<platform>:<id>" fallback routes
// channel "github:owner/repo#123" straight to the adapter with id =
// "owner/repo#123" — the same POST /api/channels/send and send_message MCP
// tool other platforms use.
func TestChannelSend_GitHubIssueRefFallback(t *testing.T) {
	var gotPath, gotAuth string
	fakeGitHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer fakeGitHub.Close()

	adapter := ghadapter.NewNamed("github", "", "gh-token")
	adapter.SetAPIBaseForTest(fakeGitHub.URL)

	mgr := gateway.NewManager()
	mgr.Register(adapter)

	h := NewGatewayHandler(mgr, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"channel": "github:rpuneet/mycel#123",
		"message": "posted by an agent",
		"sender":  "zen-zebra",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/channels/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]bool
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp["sent"] {
		t.Error("expected sent=true")
	}
	if gotPath != "/repos/rpuneet/mycel/issues/123/comments" {
		t.Errorf("path = %q, want /repos/rpuneet/mycel/issues/123/comments", gotPath)
	}
	if gotAuth != "Bearer gh-token" {
		t.Errorf("Authorization = %q, want Bearer gh-token", gotAuth)
	}
}

// TestChannelSend_GitHubNoToken verifies the "sign in to GitHub first" error
// surfaces through the HTTP layer when no api_token is configured.
func TestChannelSend_GitHubNoToken(t *testing.T) {
	adapter := ghadapter.NewNamed("github", "", "")

	mgr := gateway.NewManager()
	mgr.Register(adapter)

	h := NewGatewayHandler(mgr, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"channel": "github:rpuneet/mycel#123",
		"message": "posted by an agent",
		"sender":  "zen-zebra",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/channels/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("sign in to GitHub first")) {
		t.Errorf("body = %q, want it to mention signing in", rr.Body.String())
	}
}
