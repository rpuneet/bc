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

// avatarHostSuffixes is the SSRF allowlist for the avatar proxy: the only
// hosts it will ever fetch. Platform avatar CDNs serve public, tokenless URLs,
// so proxying them leaks nothing — but the daemon must never be tricked into
// fetching an arbitrary URL slipped into ?u=, so every hop is checked against
// this list. Leading dots make these strict subdomain suffixes.
var avatarHostSuffixes = []string{
	".slack-edge.com", // Slack profile images (avatars-*.slack-edge.com)
	".slack.com",
	".gravatar.com", // Slack's default gravatar fallbacks (secure.gravatar.com)
	".whatsapp.net", // WhatsApp profile pics (pps./media./mmg.whatsapp.net)
}

// avatarHostAllowed reports whether host is one of the platform avatar CDNs we
// proxy. Matching is exact-suffix on a leading dot so "evil-whatsapp.net" and
// "whatsapp.net.attacker.com" are both rejected. It is the barrier guard the
// avatarProxy flow relies on: nothing is dialed unless the host clears it.
func avatarHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, s := range avatarHostSuffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
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

	// SSRF barrier: the request below may only ever target an allowlisted avatar
	// CDN. The check is written as inline strings.HasSuffix guards applied
	// *directly* to parsed.Hostname() — no intervening ToLower/Trim, which would
	// sever the value from the URL — because that is the exact sanitizer shape
	// CodeQL's request-forgery analysis recognizes as a barrier for the request
	// on `parsed` below. A helper/loop (avatarHostAllowed, used by the redirect
	// guard) is not tracked across the call boundary. Keep these literals in
	// sync with avatarHostSuffixes; the leading dots make them strict subdomain
	// suffixes, so "whatsapp.net.attacker.com"/"evil-whatsapp.net" are rejected.
	// Hostnames are effectively lowercase from these CDNs; a mixed-case host
	// simply fails closed here (403), which is safe.
	hostAllowed := strings.HasSuffix(parsed.Hostname(), ".slack-edge.com") ||
		strings.HasSuffix(parsed.Hostname(), ".slack.com") ||
		strings.HasSuffix(parsed.Hostname(), ".gravatar.com") ||
		strings.HasSuffix(parsed.Hostname(), ".whatsapp.net")
	if !hostAllowed {
		httpError(w, "avatar host not allowed", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Even an allowlisted name must not resolve into an internal IP range
	// (DNS-rebinding / internal-service SSRF).
	if !avatarHostResolvesPublic(ctx, parsed.Hostname()) {
		httpError(w, "avatar host not allowed", http.StatusForbidden)
		return
	}

	// The URL handed to the client is parsed.String() — the same *url.URL whose
	// host the guard above validated — so the fetch destination is provably
	// constrained to the allowlist.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
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
