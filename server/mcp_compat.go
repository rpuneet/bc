// mcp_compat.go — backward-compat shim for pre-M6 MCP SSE URLs.
//
// The canonical MCP endpoint lives at /_mcp/ws/<wsID>/<agent>/{sse,message}
// (see mcp_dispatch.go). Agents spawned before phase M6 were configured
// against the un-scoped /_mcp/<agent>/{sse,message} path, which was mounted
// directly against the launch workspace.
//
// This middleware rewrites those legacy paths to the scoped form using the
// active workspace ID so the /_mcp/ws/ dispatcher serves them, and stamps
// Deprecation / Sunset response headers so anyone tailing logs can see
// which agent configurations still need updating. An internal URL rewrite
// is used rather than an HTTP 308 because streaming SSE clients are not
// reliably redirect-following on the initial GET.
//
// Paths that are already scoped, or that address the MCP discovery /
// .well-known surface, pass through untouched. If there is no active
// workspace in the registry we also pass through — the existing legacy
// mount at /_mcp/<agent>/... continues to target the launch workspace as
// before.
package server

import (
	"net/http"
	"strings"
)

// LegacyMCPCompat wraps next with a middleware that rewrites pre-M6 MCP
// URLs /_mcp/<agent>/{sse,message} to /_mcp/ws/<activeWsID>/<agent>/<action>.
//
// Only two actions are remapped (sse, message) — anything else (including
// an empty agent segment) is forwarded to next so current non-MCP handlers
// and any future /_mcp/<something>/... additions aren't accidentally
// caught.
func LegacyMCPCompat(next http.Handler, mgr *WorkspaceManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/_mcp/") {
			next.ServeHTTP(w, r)
			return
		}

		rest := strings.TrimPrefix(path, "/_mcp/")
		// /_mcp/ws/... is the canonical scoped form — leave it alone.
		if strings.HasPrefix(rest, "ws/") || rest == "ws" {
			next.ServeHTTP(w, r)
			return
		}

		agent, tail, _ := strings.Cut(rest, "/")
		if agent == "" || tail == "" {
			next.ServeHTTP(w, r)
			return
		}
		action, _, _ := strings.Cut(tail, "/")
		if action != "sse" && action != "message" {
			next.ServeHTTP(w, r)
			return
		}

		// Any request that reaches this point hit the legacy
		// /_mcp/<agent>/{sse,message} shape — stamp the deprecation
		// headers regardless of whether we manage to rewrite, so
		// operators grepping logs see every legacy hit (the feature's
		// observability story depends on this even when fallback to
		// the launch-workspace mount is active).
		w.Header().Set("Deprecation", "true")
		w.Header().Set("Sunset", deprecationSunset)

		// Resolve the active workspace; without one we have nowhere to
		// rewrite to, so the existing legacy launch-workspace mount
		// keeps handling the request (still stamped above).
		if mgr == nil {
			next.ServeHTTP(w, r)
			return
		}
		entry := mgr.Registry().GetActive()
		if entry == nil || entry.ID == "" {
			next.ServeHTTP(w, r)
			return
		}

		newPath := "/_mcp/ws/" + entry.ID + "/" + agent + "/" + tail
		r2 := r.Clone(r.Context())
		r2.URL.Path = newPath
		if r.URL.RawPath != "" {
			rawRest := strings.TrimPrefix(r.URL.RawPath, "/_mcp/")
			rawAgent, rawTail, _ := strings.Cut(rawRest, "/")
			if rawAgent != "" && rawTail != "" {
				r2.URL.RawPath = "/_mcp/ws/" + entry.ID + "/" + rawAgent + "/" + rawTail
			}
		}

		next.ServeHTTP(w, r2)
	})
}
