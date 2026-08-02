package handlers

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Loopback is not a trust boundary against a web browser. The daemon binds to
// 127.0.0.1 and answers with Access-Control-Allow-Origin: *, so any page the
// user happens to have open can call this API, and the call arrives from
// 127.0.0.1 carrying the user's full authority — the browser acts as a confused
// deputy. Read-only access is a privacy question that CORS already governs;
// a state change is the dangerous one, because storing a tool's install command
// and then triggering an install amounts to remote code execution.
//
// So a mutating request has to show it came from an origin this daemon serves,
// or from something that is not a browser at all.

// safeMethods do not change state, so CORS alone governs them.
var safeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// RejectCrossOriginMutations refuses state-changing requests that a browser
// began from an origin the daemon does not serve. allowedOrigin is the
// configured CORS origin: when it names a specific origin rather than "*", that
// origin may mutate too, which is what keeps a separately hosted UI working.
//
// Applied whether or not CORS headers are enabled. A request that needs no
// preflight is sent regardless of the response's CORS headers, so turning CORS
// off only hides the reply from the attacker — it does not prevent the write.
func RejectCrossOriginMutations(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if safeMethods[r.Method] || mutationAllowed(r, allowedOrigin) {
			next.ServeHTTP(w, r)
			return
		}
		httpError(w, "forbidden: cross-origin request", http.StatusForbidden)
	})
}

// mutationAllowed reports whether r is permitted to change state.
func mutationAllowed(r *http.Request, allowedOrigin string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin means no browser sent this: per the Fetch standard a browser
		// sets Origin on every request whose method is not GET or HEAD, even a
		// same-origin one. So this is the CLI, the SDK, curl, or another server.
		//
		// Sec-Fetch-Site is set by the browser and cannot be forged from page
		// script, so it still catches a browser that omitted Origin.
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}

	// A specific configured origin is trusted. "*" is not: it is the default,
	// so it means "origin unspecified" rather than "anyone may write".
	if allowedOrigin != "" && allowedOrigin != "*" && strings.EqualFold(origin, allowedOrigin) {
		return true
	}

	host := originHost(origin)
	if host == "" {
		// Opaque origin — a sandboxed iframe, or a file:// page — sends
		// "null", which names nothing this daemon serves.
		return false
	}

	// This daemon served the page.
	if strings.EqualFold(host, r.Host) {
		return true
	}

	// Or something else on this machine did. The dev server proxies /api while
	// forwarding the browser's own Origin, and the desktop shell serves its
	// boot page from a separate loopback origin, so both look cross-origin to
	// the strict test while both are in fact local tooling. A page from a
	// remote site can never present a loopback origin, which is the case this
	// exists to stop.
	return isLoopbackHost(host)
}

// originHost returns the host[:port] of an Origin header value, or "" if the
// value does not name a host.
func originHost(origin string) string {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// isLoopbackHost reports whether host[:port] names this machine.
func isLoopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		// No port to split off.
		h = host
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
