// Package oauth provides a generic loopback (localhost redirect) OAuth 2.0
// authorization-code + PKCE flow that runs entirely on the local machine.
//
// The daemon opens a short-lived listener on http://127.0.0.1:<port>/oauth/callback,
// the user authorizes in their system browser (the web UI opens the consent
// URL), and the daemon exchanges the returned code for access + refresh
// tokens which it hands back to the caller to persist in the vault. No hosted
// redirect and no cloud are involved — this is the flow "Desktop app" OAuth
// clients (e.g. Google) permit via loopback redirects.
//
// It implements app.OAuthFlow (AuthKindCallback), so it slots into the same
// /api/apps/{name}/auth begin+poll endpoints as the GitHub device flow.
// Sessions live in memory only and expire within minutes: a daemon restart
// aborts a pending sign-in and the user simply starts over.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/rpuneet/mycel/pkg/app"
)

const (
	// callbackPath is the loopback redirect path. The full redirect URI is
	// http://127.0.0.1:<port>/oauth/callback with a per-session ephemeral port.
	callbackPath = "/oauth/callback"
	// sessionTTL bounds how long a begun flow stays pollable.
	sessionTTL = 5 * time.Minute
	// pollInterval is the minimum client poll spacing reported to the UI.
	pollInterval = 2 * time.Second
)

// Provider is the static description of one provider's loopback flow.
type Provider struct {
	// Endpoint is the provider's authorization + token endpoints.
	Endpoint oauth2.Endpoint
	// Secrets maps a granted token into the vault secret keys the app's
	// Build expects (e.g. {"refresh_token": ..., "client_id": ...}).
	Secrets func(clientID, clientSecret string, tok *oauth2.Token) map[string]string
	// Scopes requested on the consent screen.
	Scopes []string
	// AuthCodeOptions are appended to the consent URL — e.g. offline access
	// and a consent prompt to force a refresh token on Google.
	AuthCodeOptions []oauth2.AuthCodeOption
}

// CredsFunc resolves the OAuth client_id + client_secret for an instance.
// It returns a clear, actionable error when the server-side client is not
// configured, so the caller surfaces the token-paste fallback.
type CredsFunc func(inst app.Instance) (clientID, clientSecret string, err error)

// LoopbackFlow implements app.OAuthFlow for a single provider using loopback
// redirects. It is safe for concurrent use.
type LoopbackFlow struct {
	creds    CredsFunc
	sessions map[string]*loopbackSession
	provider Provider
	mu       sync.Mutex
}

// loopbackSession is one in-flight authorization. The callback listener runs
// on its own goroutine and records the code (or error) under the flow mutex;
// PollAuth reads it and performs the token exchange.
type loopbackSession struct {
	conf       *oauth2.Config
	srv        *http.Server
	expiresAt  time.Time
	verifier   string
	state      string
	code       string
	errMsg     string
	exchanging bool
	closed     bool // listener already shut down
}

// NewLoopbackFlow builds a flow for a provider with a client-credential
// resolver.
func NewLoopbackFlow(provider Provider, creds CredsFunc) *LoopbackFlow {
	return &LoopbackFlow{
		provider: provider,
		creds:    creds,
		sessions: make(map[string]*loopbackSession),
	}
}

// BeginAuth resolves the client credentials, starts a loopback listener, and
// returns the consent URL for the user to open in a browser.
func (f *LoopbackFlow) BeginAuth(ctx context.Context, inst app.Instance) (app.AuthSession, error) {
	clientID, clientSecret, err := f.creds(inst)
	if err != nil {
		return app.AuthSession{}, err
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return app.AuthSession{}, fmt.Errorf("start loopback listener: %w", err)
	}
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close() //nolint:errcheck // best-effort cleanup on error
		return app.AuthSession{}, fmt.Errorf("loopback listener has no TCP address")
	}
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d%s", tcpAddr.Port, callbackPath)

	id, err := randToken()
	if err != nil {
		_ = ln.Close() //nolint:errcheck // best-effort cleanup on error
		return app.AuthSession{}, fmt.Errorf("session id: %w", err)
	}
	state, err := randToken()
	if err != nil {
		_ = ln.Close() //nolint:errcheck // best-effort cleanup on error
		return app.AuthSession{}, fmt.Errorf("state: %w", err)
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     f.provider.Endpoint,
		Scopes:       f.provider.Scopes,
		RedirectURL:  redirectURL,
	}
	verifier := oauth2.GenerateVerifier()

	sess := &loopbackSession{
		conf:      conf,
		verifier:  verifier,
		state:     state,
		expiresAt: time.Now().Add(sessionTTL),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		f.handleCallback(w, r, id)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	sess.srv = srv

	f.mu.Lock()
	f.pruneLocked()
	f.sessions[id] = sess
	f.mu.Unlock()

	go func() { _ = srv.Serve(ln) }() //nolint:errcheck // Serve returns on Shutdown

	opts := make([]oauth2.AuthCodeOption, 0, len(f.provider.AuthCodeOptions)+2)
	opts = append(opts, oauth2.S256ChallengeOption(verifier), oauth2.AccessTypeOffline)
	opts = append(opts, f.provider.AuthCodeOptions...)
	authURL := conf.AuthCodeURL(state, opts...)

	return app.AuthSession{
		ID:        id,
		Kind:      app.AuthKindCallback,
		AuthURL:   authURL,
		ExpiresAt: sess.expiresAt,
		Interval:  pollInterval,
	}, nil
}

// handleCallback validates state, records the authorization code (or error),
// renders a browser page, and tears down the listener.
func (f *LoopbackFlow) handleCallback(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()

	f.mu.Lock()
	sess, ok := f.sessions[id]
	if !ok {
		f.mu.Unlock()
		writePage(w, pageExpired)
		return
	}

	switch {
	case q.Get("error") != "":
		// The provider error/description are attacker-influenced query params.
		// Keep them daemon-side only (surfaced via PollAuth → auth/status,
		// a trusted JSON path); never reflect them into the browser HTML.
		sess.errMsg = oauthErr(q.Get("error"), q.Get("error_description"))
	case q.Get("state") != sess.state:
		sess.errMsg = "state mismatch — the sign-in could not be verified; start again"
	case q.Get("code") == "":
		sess.errMsg = "no authorization code returned"
	default:
		sess.code = q.Get("code")
	}
	failed := sess.errMsg != ""
	f.mu.Unlock()

	// The listener has done its job either way; shut it down in the
	// background so the response flushes first. The session stays in the map
	// until PollAuth reads the code (or error) and reaches a terminal state.
	go f.closeListener(id)

	// The browser page is always one of a fixed set of static strings — no
	// query input is ever echoed (guards against reflected XSS on the
	// OAuth callback).
	if failed {
		writePage(w, pageFailed)
		return
	}
	writePage(w, pageComplete)
}

// PollAuth reports progress and, once the code has arrived, exchanges it for
// tokens. The exchange runs at most once per session.
func (f *LoopbackFlow) PollAuth(ctx context.Context, session app.AuthSession) (app.AuthResult, error) {
	f.mu.Lock()
	sess, ok := f.sessions[session.ID]
	if !ok {
		f.mu.Unlock()
		return app.AuthResult{State: app.AuthStateError, Error: "unknown or expired sign-in session — start the sign-in again (a daemon restart aborts pending sign-ins)"}, nil
	}
	if time.Now().After(sess.expiresAt) {
		f.mu.Unlock()
		f.drop(session.ID)
		return app.AuthResult{State: app.AuthStateError, Error: "the sign-in timed out — start the sign-in again"}, nil
	}
	if sess.errMsg != "" {
		msg := sess.errMsg
		f.mu.Unlock()
		f.drop(session.ID)
		return app.AuthResult{State: app.AuthStateError, Error: msg}, nil
	}
	if sess.code == "" || sess.exchanging {
		f.mu.Unlock()
		return app.AuthResult{State: app.AuthStatePending}, nil
	}
	sess.exchanging = true
	conf, verifier, code := sess.conf, sess.verifier, sess.code
	f.mu.Unlock()

	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		f.drop(session.ID)
		return app.AuthResult{State: app.AuthStateError, Error: "exchange authorization code: " + err.Error()}, nil
	}
	secrets := f.provider.Secrets(conf.ClientID, conf.ClientSecret, tok)
	f.drop(session.ID)
	return app.AuthResult{State: app.AuthStateComplete, Secrets: secrets}, nil
}

// closeListener shuts a session's listener down but keeps the session in the
// map so PollAuth can still read the recorded code or error.
func (f *LoopbackFlow) closeListener(id string) {
	f.mu.Lock()
	sess, ok := f.sessions[id]
	if !ok || sess.closed {
		f.mu.Unlock()
		return
	}
	sess.closed = true
	srv := sess.srv
	f.mu.Unlock()
	shutdownServer(srv)
}

// drop stops a session's listener and removes it from the map.
func (f *LoopbackFlow) drop(id string) {
	f.mu.Lock()
	sess, ok := f.sessions[id]
	delete(f.sessions, id)
	f.mu.Unlock()
	if ok && !sess.closed {
		shutdownServer(sess.srv)
	}
}

// shutdownServer gracefully stops an HTTP server, ignoring teardown errors.
func shutdownServer(srv *http.Server) {
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx) //nolint:errcheck // best-effort teardown
}

// pruneLocked shuts down and drops expired sessions. Callers hold f.mu.
func (f *LoopbackFlow) pruneLocked() {
	now := time.Now()
	for id, sess := range f.sessions {
		if now.After(sess.expiresAt) {
			srv := sess.srv
			closed := sess.closed
			delete(f.sessions, id)
			if !closed {
				go shutdownServer(srv)
			}
		}
	}
}

// Static browser-facing messages for the callback page. These are the only
// strings writePage ever renders — no request input is echoed.
const (
	pageComplete = "Sign-in complete. You can close this window and return to mycel."
	pageFailed   = "Sign-in failed. Return to mycel for details, then start the sign-in again."
	pageExpired  = "This sign-in session has expired. Return to mycel and start again."
)

// writePage renders a minimal HTML page shown in the user's browser after the
// redirect. The message is HTML-escaped as defense-in-depth so no caller can
// ever reflect untrusted input into the response.
func writePage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, //nolint:errcheck // browser-facing page, nothing to do on write error
		"<!doctype html><html><head><meta charset=utf-8><title>mycel</title></head>"+
			"<body style=\"font-family:system-ui,sans-serif;background:#f5efe6;color:#3a2f28;"+
			"display:flex;align-items:center;justify-content:center;height:100vh;margin:0\">"+
			"<p style=\"max-width:28rem;text-align:center;font-size:1rem;line-height:1.5\">%s</p>"+
			"</body></html>", html.EscapeString(msg))
}

// randToken returns a random 128-bit hex token for session/state IDs.
func randToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// oauthErr formats an OAuth error code with its optional description.
func oauthErr(code, desc string) string {
	if desc != "" {
		return code + ": " + desc
	}
	return code
}
