package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/rpuneet/mycel/pkg/app"
)

// testProvider builds a flow whose endpoints point at a stub server and whose
// credentials resolve from a fixed pair.
func testFlow(tokenURL string, creds CredsFunc) *LoopbackFlow {
	return NewLoopbackFlow(Provider{
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://auth.example.test/authorize",
			TokenURL: tokenURL,
		},
		Scopes:          []string{"scope.read", "scope.send"},
		AuthCodeOptions: []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("prompt", "consent")},
		Secrets: func(clientID, clientSecret string, tok *oauth2.Token) map[string]string {
			return map[string]string{
				"client_id":     clientID,
				"client_secret": clientSecret,
				"refresh_token": tok.RefreshToken,
				"access_token":  tok.AccessToken,
			}
		},
	}, creds)
}

func fixedCreds(inst app.Instance) (string, string, error) {
	return "the-client", "the-secret", nil
}

// callbackURL derives the loopback callback URL for a begun session from its
// consent URL's redirect_uri.
func callbackURL(t *testing.T, authURL, query string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	redirect := u.Query().Get("redirect_uri")
	if redirect == "" {
		t.Fatal("consent URL missing redirect_uri")
	}
	return redirect + "?" + query
}

// TestLoopbackHappyPath drives the full flow: begin → browser redirect with a
// valid code → poll exchanges it against a stub token endpoint → secrets.
func TestLoopbackHappyPath(t *testing.T) {
	// Stub token endpoint returns a token bundle for the exchange.
	var gotVerifier, gotCode, gotRedirect string
	stub := http.NewServeMux()
	stub.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotVerifier = r.FormValue("code_verifier")
		gotCode = r.FormValue("code")
		gotRedirect = r.FormValue("redirect_uri")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at-123","refresh_token":"rt-456","token_type":"Bearer","expires_in":3600}`)
	})
	ts := startTestServer(t, stub)

	f := testFlow(ts+"/token", fixedCreds)
	sess, err := f.BeginAuth(context.Background(), app.Instance{Name: "x"})
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	if sess.Kind != app.AuthKindCallback || sess.AuthURL == "" {
		t.Fatalf("unexpected session %+v", sess)
	}

	// Poll before the code arrives → pending.
	res, err := f.PollAuth(context.Background(), app.AuthSession{ID: sess.ID})
	if err != nil || res.State != app.AuthStatePending {
		t.Fatalf("pre-code poll = %+v, %v; want pending", res, err)
	}

	// Simulate the browser hitting the loopback redirect with the code.
	cb := callbackURL(t, sess.AuthURL, "code=auth-code-789&state="+stateOf(t, sess.AuthURL))
	resp, err := http.Get(cb) //nolint:noctx,gosec // test loopback URL
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "complete") {
		t.Errorf("callback page = %q, want success", body)
	}

	// Poll now completes with the exchanged secrets.
	res, err = pollUntil(t, f, sess.ID, app.AuthStateComplete)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if res.Secrets["refresh_token"] != "rt-456" || res.Secrets["access_token"] != "at-123" {
		t.Errorf("secrets = %+v, want rt-456/at-123", res.Secrets)
	}
	if res.Secrets["client_id"] != "the-client" || res.Secrets["client_secret"] != "the-secret" {
		t.Errorf("client creds not propagated: %+v", res.Secrets)
	}
	// PKCE verifier and loopback redirect were sent in the exchange.
	if gotVerifier == "" {
		t.Error("token exchange sent no code_verifier (PKCE)")
	}
	if gotCode != "auth-code-789" {
		t.Errorf("exchange code = %q, want auth-code-789", gotCode)
	}
	if !strings.HasPrefix(gotRedirect, "http://127.0.0.1:") {
		t.Errorf("exchange redirect_uri = %q, want loopback", gotRedirect)
	}
}

// TestLoopbackStateMismatch rejects a callback whose state does not match.
func TestLoopbackStateMismatch(t *testing.T) {
	f := testFlow("https://unused.test/token", fixedCreds)
	sess, err := f.BeginAuth(context.Background(), app.Instance{Name: "x"})
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	cb := callbackURL(t, sess.AuthURL, "code=c&state=wrong")
	resp, err := http.Get(cb) //nolint:noctx,gosec // test loopback URL
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	_ = resp.Body.Close()

	res, _ := pollUntil(t, f, sess.ID, app.AuthStateError)
	if !strings.Contains(res.Error, "state") {
		t.Errorf("error = %q, want state mismatch", res.Error)
	}
}

// TestLoopbackProviderError surfaces an error= param from the redirect.
func TestLoopbackProviderError(t *testing.T) {
	f := testFlow("https://unused.test/token", fixedCreds)
	sess, err := f.BeginAuth(context.Background(), app.Instance{Name: "x"})
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	cb := callbackURL(t, sess.AuthURL, "error=access_denied&error_description=user+said+no&state="+stateOf(t, sess.AuthURL))
	resp, err := http.Get(cb) //nolint:noctx,gosec // test loopback URL
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	_ = resp.Body.Close()

	res, _ := pollUntil(t, f, sess.ID, app.AuthStateError)
	if !strings.Contains(res.Error, "access_denied") {
		t.Errorf("error = %q, want access_denied", res.Error)
	}
}

// TestLoopbackCallbackNoReflectedInput proves the browser-facing callback
// page never reflects attacker-controlled query params (reflected XSS): a
// crafted error/error_description must not appear in the HTML, while the
// error stays available on the trusted daemon-side poll path.
func TestLoopbackCallbackNoReflectedInput(t *testing.T) {
	f := testFlow("https://unused.test/token", fixedCreds)
	sess, err := f.BeginAuth(context.Background(), app.Instance{Name: "x"})
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}

	const marker = "XSSMARKER_31337"
	inject := url.Values{
		"error":             {"<script>alert('" + marker + "')</script>"},
		"error_description": {"<img src=x onerror=alert('" + marker + "')>"},
		"state":             {stateOf(t, sess.AuthURL)},
	}.Encode()
	cb := callbackURL(t, sess.AuthURL, inject)
	resp, err := http.Get(cb) //nolint:noctx,gosec // test loopback URL
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	page := string(body)
	if strings.Contains(page, marker) {
		t.Errorf("callback page reflected injected input:\n%s", page)
	}
	if strings.Contains(page, "<script>alert") || strings.Contains(page, "onerror=") {
		t.Errorf("callback page contains an unescaped injected tag:\n%s", page)
	}

	// The real error is still surfaced on the trusted poll path.
	res, _ := pollUntil(t, f, sess.ID, app.AuthStateError)
	if res.State != app.AuthStateError || !strings.Contains(res.Error, marker) {
		t.Errorf("poll error = %+v, want the raw provider error daemon-side", res)
	}
}

// TestLoopbackUnknownSession reports an error for an unknown session ID.
func TestLoopbackUnknownSession(t *testing.T) {
	f := testFlow("https://unused.test/token", fixedCreds)
	res, err := f.PollAuth(context.Background(), app.AuthSession{ID: "nope"})
	if err != nil {
		t.Fatalf("PollAuth: %v", err)
	}
	if res.State != app.AuthStateError {
		t.Errorf("state = %q, want error", res.State)
	}
}

// TestLoopbackCredsError propagates the resolver's fallback error verbatim.
func TestLoopbackCredsError(t *testing.T) {
	f := testFlow("https://unused.test/token", func(app.Instance) (string, string, error) {
		return "", "", errNotConfigured
	})
	_, err := f.BeginAuth(context.Background(), app.Instance{Name: "x"})
	if err != errNotConfigured { //nolint:errorlint // sentinel identity is the assertion
		t.Fatalf("BeginAuth err = %v, want errNotConfigured", err)
	}
}

var errNotConfigured = errString("client not configured")

type errString string

func (e errString) Error() string { return string(e) }

// --- helpers ---

func startTestServer(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

// stateOf extracts the state param from a consent URL.
func stateOf(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	return u.Query().Get("state")
}

// pollUntil polls up to ~2s for the target terminal state.
func pollUntil(t *testing.T, f *LoopbackFlow, id, want string) (app.AuthResult, error) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		res, err := f.PollAuth(context.Background(), app.AuthSession{ID: id})
		if err != nil {
			return res, err
		}
		if res.State == want || res.State == app.AuthStateError {
			return res, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return app.AuthResult{}, nil
}
