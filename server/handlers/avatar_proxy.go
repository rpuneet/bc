package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// avatarMaxBytes caps the proxied image size so a hostile or oversized
// upstream response can't exhaust daemon memory.
const avatarMaxBytes = 5 << 20 // 5 MiB

// avatarAllowedHosts is the exact-match SSRF allowlist for the avatar proxy:
// the only hosts it will ever fetch. Platform avatar CDNs serve public,
// tokenless URLs, so proxying them leaks nothing — but the daemon must never be
// tricked into fetching an arbitrary URL slipped into ?u=. Exact hostnames
// (not suffixes) are used so a lookalike like "whatsapp.net.attacker.com" can
// never match. Keep in sync with the inline == guard in avatarProxy, which must
// stay literal for CodeQL's request-forgery barrier to recognize it.
var avatarAllowedHosts = map[string]bool{
	"pps.whatsapp.net":       true, // WhatsApp profile pictures
	"media.whatsapp.net":     true, // WhatsApp media (thumbnails)
	"avatars.slack-edge.com": true, // Slack uploaded avatars
	"a.slack-edge.com":       true, // Slack alt avatar host
	"secure.gravatar.com":    true, // Slack default (gravatar) avatars
}

// avatarHostAllowed reports whether host is an allowlisted avatar CDN. Used by
// the redirect guard and tests; the primary request path uses an inline literal
// == check (see avatarProxy) because CodeQL only recognizes equality-against-
// constant on url.Hostname() as a request-forgery barrier.
func avatarHostAllowed(host string) bool {
	return avatarAllowedHosts[strings.ToLower(host)]
}

// avatarHostResolvesPublic reports whether an already-allowlisted host resolves
// only to public unicast IPs. It blocks an allowlisted (or attacker-controlled)
// name that points into loopback/private/link-local ranges — the DNS-rebinding
// / internal-service SSRF vector that a name-only allowlist can't catch.
func avatarHostResolvesPublic(ctx context.Context, host string) bool {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return false
		}
	}
	return true
}

// avatarCheckRedirect follows redirects only while each hop stays on an
// allowlisted host, closing the redirect-based SSRF hole, and caps the chain.
func avatarCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("avatar proxy: too many redirects")
	}
	if !avatarHostAllowed(req.URL.Hostname()) {
		return fmt.Errorf("avatar proxy: redirect to disallowed host %q", req.URL.Hostname())
	}
	return nil
}

// avatarHTTPClient fetches upstream avatars with the redirect guard installed.
var avatarHTTPClient = &http.Client{
	Timeout:       12 * time.Second,
	CheckRedirect: avatarCheckRedirect,
}

// avatarProxyPath wraps a raw platform avatar URL in the loopback-guarded proxy
// path the web UI loads directly. Returns "" for empty input so DTO callers can
// omit the field. The raw URL is base64url-encoded — not because it is secret
// (these CDN URLs are public and tokenless) but so its own query string
// survives the round trip intact.
func avatarProxyPath(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	return "/api/apps/avatar?u=" + base64.RawURLEncoding.EncodeToString([]byte(rawURL))
}

// avatarProxy handles GET /api/apps/avatar?u=<base64url> — it fetches an
// allowlisted platform avatar server-side and streams the image bytes to the
// local web UI. This keeps expiring/CDN URLs server-mediated and guarantees the
// image is reachable from the browser without embedding any platform tokens.
func (h *GatewayHandler) avatarProxy(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	// The proxy fetches (allowlisted) hosts on the daemon's behalf, so only
	// local web-UI clients may drive it.
	if !checkLoopback(w, r) {
		return
	}

	enc := r.URL.Query().Get("u")
	if enc == "" {
		httpError(w, "missing u parameter", http.StatusBadRequest)
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		httpError(w, "invalid u parameter", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(string(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		httpError(w, "invalid avatar url", http.StatusBadRequest)
		return
	}

	// SSRF barrier: map the requested host to a *constant* scheme://host/ base.
	// The switch is an exact-equality allowlist (unlisted host → 403), and the
	// outbound URL is then rebuilt as base + path + query where `base` is a
	// string literal. Because the authority comes entirely from that constant
	// prefix and the attacker-influenced data is confined to the path/query
	// after the first "/", the fetch destination can never be steered off an
	// allowlisted avatar CDN — this is the "sanitizing prefix" shape CodeQL's
	// request-forgery analysis recognizes as a barrier. Keep in sync with
	// avatarAllowedHosts. Hosts are lowercase from these CDNs; a mixed-case host
	// falls through to 403, which is safe.
	host := parsed.Hostname()
	var base string
	switch host {
	case "pps.whatsapp.net":
		base = "https://pps.whatsapp.net/"
	case "media.whatsapp.net":
		base = "https://media.whatsapp.net/"
	case "avatars.slack-edge.com":
		base = "https://avatars.slack-edge.com/"
	case "a.slack-edge.com":
		base = "https://a.slack-edge.com/"
	case "secure.gravatar.com":
		base = "https://secure.gravatar.com/"
	default:
		httpError(w, "avatar host not allowed", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Even an allowlisted name must not resolve into an internal IP range
	// (DNS-rebinding / internal-service SSRF).
	if !avatarHostResolvesPublic(ctx, host) {
		httpError(w, "avatar host not allowed", http.StatusForbidden)
		return
	}

	// Rebuild from the constant base: only url.Parse-separated path/query (never
	// the raw string, and never anything that could carry an authority) follow
	// the constant "https://<host>/" prefix.
	fetchURL := base + strings.TrimPrefix(parsed.EscapedPath(), "/")
	if parsed.RawQuery != "" {
		fetchURL += "?" + parsed.RawQuery
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		httpError(w, "avatar request failed", http.StatusBadGateway)
		return
	}
	resp, err := avatarHTTPClient.Do(req)
	if err != nil {
		httpError(w, "avatar fetch failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		httpError(w, "avatar unavailable", http.StatusBadGateway)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		httpError(w, "not an image", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, avatarMaxBytes)) //nolint:errcheck // client may disconnect mid-stream
}
