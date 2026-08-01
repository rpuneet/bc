package app

import (
	"context"
	"time"
)

// Auth flow states reported by AuthResult.State.
const (
	// AuthStatePending means the user has not completed the browser flow yet.
	AuthStatePending = "pending"
	// AuthStateComplete means the flow finished and secrets are available.
	AuthStateComplete = "complete"
	// AuthStateError means the flow failed or expired.
	AuthStateError = "error"
)

// Auth flow kinds reported by AuthSession.Kind.
const (
	// AuthKindDevice is an OAuth device flow: the user enters UserCode at
	// VerificationURL while the daemon polls for completion.
	AuthKindDevice = "device"
	// AuthKindCallback is an authorization-code flow: the user opens
	// AuthURL and a local listener handles the redirect.
	AuthKindCallback = "callback"
)

// AuthSession is one in-flight browser auth attempt. Device flow returns
// VerificationURL+UserCode; callback flow returns AuthURL and the local
// listener handles the redirect.
type AuthSession struct {
	// ExpiresAt is when the session stops being pollable.
	ExpiresAt time.Time
	// ID identifies the session for PollAuth / the auth/status endpoint.
	ID string
	// Kind is AuthKindDevice or AuthKindCallback.
	Kind string
	// AuthURL is the browser URL to open (callback flow).
	AuthURL string
	// VerificationURL is where the user enters UserCode (device flow).
	VerificationURL string
	// UserCode is the short code the user types in (device flow).
	UserCode string
	// Interval is the minimum wait between polls.
	Interval time.Duration
}

// AuthResult reports auth progress. Secrets are never serialized — the
// server persists them to the vault and only the state crosses the wire.
type AuthResult struct {
	// Secrets, on completion, holds the credentials to persist, keyed by
	// the descriptor's Secret field keys (e.g. {"api_token": "..."}).
	Secrets map[string]string `json:"-"`
	// State is AuthStatePending, AuthStateComplete, or AuthStateError.
	State string `json:"state"`
	// Error describes the failure when State is AuthStateError.
	Error string `json:"error,omitempty"`
}

// OAuthFlow is implemented by plugins whose app can authenticate via a
// browser flow. It is a plugin-level capability — auth happens before an
// adapter exists (unlike QRPairer, which lives on the built adapter).
//
// Session state is held in-memory by the plugin, keyed by session ID:
// sessions are short-lived (minutes), so a daemon restart aborts any
// pending auth and the user simply starts over.
type OAuthFlow interface {
	// BeginAuth starts the flow for an instance and returns the session
	// the user drives from a browser.
	BeginAuth(ctx context.Context, inst Instance) (AuthSession, error)
	// PollAuth reports progress for a previously begun session (only
	// session.ID needs to be set); on success it returns the secrets to
	// persist. Implementations must rate-limit upstream polling to the
	// session's interval themselves.
	PollAuth(ctx context.Context, session AuthSession) (AuthResult, error)
}

// OAuthConfigured is an optional capability on an OAuthFlow plugin. It
// reports whether the browser flow is actually usable right now — e.g. the
// server-side OAuth client credentials are present. When it returns false,
// the catalog reports oauth_available=false so the connect UI shows the
// honest token-paste fallback instead of a "Connect" button that would fail.
//
// A plugin whose flow needs only per-instance config the user pastes (like
// the GitHub device flow's client ID) does not implement this — its flow is
// always "available" and BeginAuth surfaces any missing field.
type OAuthConfigured interface {
	// OAuthConfigured reports whether the browser sign-in can run now.
	OAuthConfigured() bool
}
