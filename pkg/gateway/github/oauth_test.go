package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rpuneet/mycel/pkg/app"
)

// stubGitHub fakes the two device-flow endpoints. tokenResponses is
// consumed in order, one per upstream token poll.
type stubGitHub struct {
	t              *testing.T
	deviceResponse map[string]any
	tokenResponses []map[string]any
	tokenCalls     atomic.Int64
	deviceCalls    atomic.Int64
}

func (s *stubGitHub) start() (*httptest.Server, *deviceFlow) {
	s.t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		s.deviceCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			s.t.Errorf("parse device form: %v", err)
		}
		if got := r.PostForm.Get("client_id"); got == "" {
			s.t.Error("device code request missing client_id")
		}
		writeStubJSON(s.t, w, s.deviceResponse)
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := s.tokenCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			s.t.Errorf("parse token form: %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
			s.t.Errorf("grant_type = %q", got)
		}
		if got := r.PostForm.Get("device_code"); got == "" {
			s.t.Error("token request missing device_code")
		}
		idx := int(n) - 1
		if idx >= len(s.tokenResponses) {
			idx = len(s.tokenResponses) - 1
		}
		writeStubJSON(s.t, w, s.tokenResponses[idx])
	})
	srv := httptest.NewServer(mux)
	s.t.Cleanup(srv.Close)

	f := newDeviceFlow()
	f.deviceCodeURL = srv.URL + "/login/device/code"
	f.tokenURL = srv.URL + "/login/oauth/access_token"
	return srv, f
}

func writeStubJSON(t *testing.T, w http.ResponseWriter, body map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Errorf("encode stub response: %v", err)
	}
}

func testInstance() app.Instance {
	return app.Instance{
		App:    "github",
		Name:   "github",
		Config: map[string]string{"oauth_client_id": "Ov23liTESTID"},
	}
}

func deviceOK() map[string]any {
	return map[string]any{
		"device_code":      "dev-code-1",
		"user_code":        "ABCD-1234",
		"verification_uri": "https://github.com/login/device",
		"expires_in":       900,
		"interval":         0, // exercise the 5s default; tests bypass it via lastPoll
	}
}

func TestBeginAuthDefaultsClientIDWhenUnset(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: deviceOK()}
	_, f := stub.start()

	// No oauth_client_id in the instance config: BeginAuth must fall back
	// to mycel's built-in client ID rather than erroring — this is what
	// gives users "Sign in with GitHub" with zero setup.
	_, err := f.BeginAuth(context.Background(), app.Instance{App: "github", Name: "github"})
	if err != nil {
		t.Fatalf("BeginAuth: %v, want no error (should default to DefaultOAuthClientID)", err)
	}

	f.mu.Lock()
	var gotClientID string
	for _, s := range f.sessions {
		gotClientID = s.clientID
	}
	f.mu.Unlock()
	if gotClientID != DefaultOAuthClientID {
		t.Errorf("session clientID = %q, want %q", gotClientID, DefaultOAuthClientID)
	}
}

func TestBeginAuthOverridesDefaultClientID(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: deviceOK()}
	_, f := stub.start()

	sess, err := f.BeginAuth(context.Background(), testInstance())
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}

	f.mu.Lock()
	gotClientID := f.sessions[sess.ID].clientID
	f.mu.Unlock()
	const pastedClientID = "Ov23liTESTID"
	if gotClientID != pastedClientID {
		t.Errorf("session clientID = %q, want pasted %q (override the default)", gotClientID, pastedClientID)
	}
	if gotClientID == DefaultOAuthClientID {
		t.Error("pasted client ID was overridden by the default — override should win")
	}
}

func TestBeginAuthReturnsDeviceSession(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: deviceOK()}
	_, f := stub.start()

	sess, err := f.BeginAuth(context.Background(), testInstance())
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	if sess.Kind != app.AuthKindDevice {
		t.Errorf("Kind = %q, want device", sess.Kind)
	}
	if sess.UserCode != "ABCD-1234" || sess.VerificationURL != "https://github.com/login/device" {
		t.Errorf("session = %+v", sess)
	}
	if sess.ID == "" || sess.Interval <= 0 || !sess.ExpiresAt.After(time.Now()) {
		t.Errorf("session bookkeeping = %+v", sess)
	}
	if stub.deviceCalls.Load() != 1 {
		t.Errorf("device endpoint calls = %d, want 1", stub.deviceCalls.Load())
	}
}

func TestBeginAuthSurfacesUpstreamError(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: map[string]any{
		"error": "unauthorized_client", "error_description": "client is suspended",
	}}
	_, f := stub.start()

	_, err := f.BeginAuth(context.Background(), testInstance())
	if err == nil || !strings.Contains(err.Error(), "unauthorized_client") {
		t.Fatalf("err = %v, want unauthorized_client", err)
	}
}

// forcePollable rewinds the session's lastPoll so PollAuth hits upstream
// instead of returning the rate-limited cached pending.
func forcePollable(f *deviceFlow, id string) {
	f.mu.Lock()
	if s, ok := f.sessions[id]; ok {
		s.lastPoll = time.Time{}
	}
	f.mu.Unlock()
}

func TestPollAuthPendingThenComplete(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: deviceOK(), tokenResponses: []map[string]any{
		{"error": "authorization_pending"},
		{"access_token": "gho_test_token", "token_type": "bearer", "scope": "repo"},
	}}
	_, f := stub.start()

	sess, err := f.BeginAuth(context.Background(), testInstance())
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}

	// First poll: user has not authorized yet.
	res, err := f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil {
		t.Fatalf("PollAuth: %v", err)
	}
	if res.State != app.AuthStatePending {
		t.Fatalf("state = %q, want pending", res.State)
	}

	// Immediate re-poll is rate-limited: no upstream call.
	res, err = f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil || res.State != app.AuthStatePending {
		t.Fatalf("rate-limited poll = %+v, %v; want pending", res, err)
	}
	if got := stub.tokenCalls.Load(); got != 1 {
		t.Fatalf("token endpoint calls = %d, want 1 (rate limit)", got)
	}

	// After the interval the user has authorized: token comes back.
	forcePollable(f, sess.ID)
	res, err = f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil {
		t.Fatalf("PollAuth: %v", err)
	}
	if res.State != app.AuthStateComplete {
		t.Fatalf("state = %q (err %q), want complete", res.State, res.Error)
	}
	if res.Secrets["api_token"] != "gho_test_token" {
		t.Errorf("secrets = %v, want api_token", res.Secrets)
	}

	// The session is consumed: polling again reports an unknown session.
	res, err = f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil || res.State != app.AuthStateError {
		t.Fatalf("post-complete poll = %+v, %v; want error state", res, err)
	}
}

func TestPollAuthTerminalErrors(t *testing.T) {
	tests := []struct {
		name    string
		errCode string
	}{
		{"access denied", "access_denied"},
		{"expired token", "expired_token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubGitHub{t: t, deviceResponse: deviceOK(), tokenResponses: []map[string]any{
				{"error": tt.errCode},
			}}
			_, f := stub.start()
			sess, err := f.BeginAuth(context.Background(), testInstance())
			if err != nil {
				t.Fatalf("BeginAuth: %v", err)
			}
			forcePollable(f, sess.ID)
			res, err := f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
			if err != nil {
				t.Fatalf("PollAuth: %v", err)
			}
			if res.State != app.AuthStateError || !strings.Contains(res.Error, tt.errCode) {
				t.Errorf("result = %+v, want error state mentioning %s", res, tt.errCode)
			}
			// Terminal errors consume the session.
			res, _ = f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
			if !strings.Contains(res.Error, "unknown or expired") {
				t.Errorf("post-error poll = %+v, want unknown session", res)
			}
		})
	}
}

func TestPollAuthSlowDownStretchesInterval(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: deviceOK(), tokenResponses: []map[string]any{
		{"error": "slow_down", "interval": 10},
	}}
	_, f := stub.start()
	sess, err := f.BeginAuth(context.Background(), testInstance())
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	forcePollable(f, sess.ID)
	res, err := f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil || res.State != app.AuthStatePending {
		t.Fatalf("slow_down poll = %+v, %v; want pending", res, err)
	}
	f.mu.Lock()
	got := f.sessions[sess.ID].interval
	f.mu.Unlock()
	if got != 10*time.Second {
		t.Errorf("interval = %v, want 10s after slow_down", got)
	}
}

func TestPollAuthUnknownSession(t *testing.T) {
	f := newDeviceFlow()
	res, err := f.PollAuth(context.Background(), app.AuthSession{ID: "nope"})
	if err != nil {
		t.Fatalf("PollAuth: %v", err)
	}
	if res.State != app.AuthStateError || !strings.Contains(res.Error, "unknown or expired") {
		t.Errorf("result = %+v, want unknown-session error", res)
	}
}

func TestPollAuthExpiredSession(t *testing.T) {
	stub := &stubGitHub{t: t, deviceResponse: deviceOK()}
	_, f := stub.start()
	sess, err := f.BeginAuth(context.Background(), testInstance())
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	f.mu.Lock()
	f.sessions[sess.ID].expiresAt = time.Now().Add(-time.Minute)
	f.mu.Unlock()

	res, err := f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil {
		t.Fatalf("PollAuth: %v", err)
	}
	if res.State != app.AuthStateError || !strings.Contains(res.Error, "expired") {
		t.Errorf("result = %+v, want expiry error", res)
	}
}
