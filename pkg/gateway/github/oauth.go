package github

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rpuneet/mycel/pkg/app"
)

// GitHub device-flow endpoints (RFC 8628). Any GitHub OAuth app's client
// ID works — the device flow needs no client secret and no redirect URL,
// so the whole flow runs locally against the daemon.
const (
	defaultDeviceCodeURL = "https://github.com/login/device/code"
	defaultTokenURL      = "https://github.com/login/oauth/access_token" //nolint:gosec // endpoint URL, not a credential
	// deviceScope grants API access for repo operations (comments,
	// statuses) — the outbound counterpart to the inbound webhooks.
	deviceScope = "repo"
	// pollTimeout bounds a single upstream poll request.
	pollTimeout = 15 * time.Second
	// DefaultOAuthClientID is mycel's own registered GitHub OAuth App
	// (device flow enabled). Device-flow client IDs are public — like the
	// gh CLI's own client ID, there is no secret to protect — so shipping
	// it here gives every user one-click "Sign in with GitHub" with zero
	// setup. Users may still override it with their own OAuth app's
	// client ID (advanced: their own org, higher rate limits).
	DefaultOAuthClientID = "Ov23liP7jwMEwOZtM2b5" // gitleaks:allow -- public GitHub device-flow client ID, not a secret
)

// deviceFlow implements app.OAuthFlow via the GitHub device flow.
// Sessions live in memory only: they expire within minutes, so a daemon
// restart aborts pending auths and the user starts over.
type deviceFlow struct {
	sessions      map[string]*deviceSession
	httpc         *http.Client
	deviceCodeURL string
	tokenURL      string
	mu            sync.Mutex
}

// deviceSession is one pending device authorization.
type deviceSession struct {
	expiresAt  time.Time
	lastPoll   time.Time
	clientID   string
	deviceCode string
	interval   time.Duration
}

func newDeviceFlow() *deviceFlow {
	return &deviceFlow{
		sessions:      make(map[string]*deviceSession),
		httpc:         &http.Client{Timeout: pollTimeout},
		deviceCodeURL: defaultDeviceCodeURL,
		tokenURL:      defaultTokenURL,
	}
}

// BeginAuth requests a device+user code pair from GitHub and returns the
// session the user completes in a browser.
func (f *deviceFlow) BeginAuth(ctx context.Context, inst app.Instance) (app.AuthSession, error) {
	clientID := strings.TrimSpace(inst.Config["oauth_client_id"])
	if clientID == "" {
		// No override pasted: fall back to mycel's own public OAuth app so
		// sign-in works out of the box.
		clientID = DefaultOAuthClientID
	}

	form := url.Values{"client_id": {clientID}, "scope": {deviceScope}}
	var resp struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := f.postForm(ctx, f.deviceCodeURL, form, &resp); err != nil {
		return app.AuthSession{}, fmt.Errorf("github: request device code: %w", err)
	}
	if resp.Error != "" {
		return app.AuthSession{}, fmt.Errorf("github: device code: %s", oauthErrString(resp.Error, resp.ErrorDesc))
	}
	if resp.DeviceCode == "" || resp.UserCode == "" {
		return app.AuthSession{}, fmt.Errorf("github: device code response missing codes")
	}

	id, err := newSessionID()
	if err != nil {
		return app.AuthSession{}, fmt.Errorf("github: session id: %w", err)
	}
	interval := time.Duration(resp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	if resp.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(15 * time.Minute)
	}

	f.mu.Lock()
	f.pruneLocked()
	f.sessions[id] = &deviceSession{
		clientID:   clientID,
		deviceCode: resp.DeviceCode,
		interval:   interval,
		expiresAt:  expiresAt,
	}
	f.mu.Unlock()

	verificationURL := resp.VerificationURI
	if verificationURL == "" {
		verificationURL = "https://github.com/login/device"
	}
	return app.AuthSession{
		ID:              id,
		Kind:            app.AuthKindDevice,
		VerificationURL: verificationURL,
		UserCode:        resp.UserCode,
		ExpiresAt:       expiresAt,
		Interval:        interval,
	}, nil
}

// PollAuth asks GitHub whether the user has authorized the device code.
// Upstream polling is rate-limited to the session interval — callers may
// poll the daemon as often as they like.
func (f *deviceFlow) PollAuth(ctx context.Context, session app.AuthSession) (app.AuthResult, error) {
	f.mu.Lock()
	s, ok := f.sessions[session.ID]
	if !ok {
		f.mu.Unlock()
		return app.AuthResult{State: app.AuthStateError, Error: "unknown or expired auth session — start the sign-in again (a daemon restart aborts pending auths)"}, nil
	}
	if time.Now().After(s.expiresAt) {
		delete(f.sessions, session.ID)
		f.mu.Unlock()
		return app.AuthResult{State: app.AuthStateError, Error: "the sign-in code expired — start the sign-in again"}, nil
	}
	if time.Since(s.lastPoll) < s.interval {
		f.mu.Unlock()
		return app.AuthResult{State: app.AuthStatePending}, nil
	}
	s.lastPoll = time.Now()
	clientID, deviceCode := s.clientID, s.deviceCode
	f.mu.Unlock()

	form := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
		Interval    int    `json:"interval"`
	}
	if err := f.postForm(ctx, f.tokenURL, form, &resp); err != nil {
		// Transient network failure: keep the session, report pending.
		return app.AuthResult{State: app.AuthStatePending}, nil //nolint:nilerr // pending-on-error keeps the poll loop alive
	}

	switch resp.Error {
	case "":
		// fallthrough to token handling below
	case "authorization_pending":
		return app.AuthResult{State: app.AuthStatePending}, nil
	case "slow_down":
		f.mu.Lock()
		if slow, live := f.sessions[session.ID]; live {
			if resp.Interval > 0 {
				slow.interval = time.Duration(resp.Interval) * time.Second
			} else {
				slow.interval += 5 * time.Second
			}
		}
		f.mu.Unlock()
		return app.AuthResult{State: app.AuthStatePending}, nil
	default:
		f.dropSession(session.ID)
		return app.AuthResult{State: app.AuthStateError, Error: oauthErrString(resp.Error, resp.ErrorDesc)}, nil
	}

	if resp.AccessToken == "" {
		f.dropSession(session.ID)
		return app.AuthResult{State: app.AuthStateError, Error: "github returned no access token"}, nil
	}
	f.dropSession(session.ID)
	return app.AuthResult{
		State:   app.AuthStateComplete,
		Secrets: map[string]string{"api_token": resp.AccessToken},
	}, nil
}

// postForm POSTs a form and decodes the JSON response into out.
func (f *deviceFlow) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := f.httpc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close() //nolint:errcheck // read-only body
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 500 {
		return fmt.Errorf("github responded %d", res.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// dropSession removes a finished or failed session.
func (f *deviceFlow) dropSession(id string) {
	f.mu.Lock()
	delete(f.sessions, id)
	f.mu.Unlock()
}

// pruneLocked drops expired sessions. Callers hold f.mu.
func (f *deviceFlow) pruneLocked() {
	now := time.Now()
	for id, s := range f.sessions {
		if now.After(s.expiresAt) {
			delete(f.sessions, id)
		}
	}
}

// newSessionID returns a random 128-bit hex session ID.
func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// oauthErrString formats an OAuth error code + optional description.
func oauthErrString(code, desc string) string {
	if desc != "" {
		return code + ": " + desc
	}
	return code
}
