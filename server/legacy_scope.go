// legacy_scope.go — 301 redirects for un-scoped UI paths.
//
// The multi-workspace UI lives under /w/<wsId>/<page>. Older bookmarks
// hit /<page> directly and the SPA used to resolve them against the
// active workspace. After the /w/<wsId>/... canonicalization (see
// multi-workspace-and-code-tab.md §5.1 and §9.2–9.3) those un-scoped
// URLs should issue a permanent redirect to the active workspace's
// canonical URL, with Deprecation / Sunset headers so link-following
// clients can log the rename.
//
// Scope: this middleware only rewrites the top-level SPA pages, not
// their subpaths that are served by /api/ or /_mcp/, which are handled
// by WorkspaceScope and the MCP dispatcher. If there is no active
// workspace in the registry we pass through — the SPA will show its
// "pick a workspace" CTA.
package server

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/rpuneet/bc/pkg/log"
)

// emptyIDWarnFired guards against spamming the log when the active
// workspace entry is present but has an empty ID. That state is a
// registry invariant violation (ID is the 8-char sha256), so it
// deserves visibility — but exactly once per process, not per request.
var emptyIDWarnFired atomic.Bool

// legacyUIPages is the set of top-level SPA pages that used to live at
// /<page> and now live at /w/<wsId>/<page>. Kept as a simple map to make
// membership checks O(1) and additions obvious.
var legacyUIPages = map[string]struct{}{
	"live":      {},
	"agents":    {},
	"channels":  {},
	"metrics":   {},
	"tools":     {},
	"workspace": {},
}

// LegacyUIScope returns a middleware that 301-redirects legacy top-level
// SPA pages to their /w/<activeWsId>/<page> form. Paths the SPA already
// scopes (/w/...), API routes (/api/...), MCP routes (/_mcp/...), health
// probes, and static asset lookups pass through untouched.
//
// If mgr is nil or there is no active workspace, the middleware is a
// no-op — the catch-all SPA handler will serve index.html as before.
func LegacyUIScope(next http.Handler, mgr *WorkspaceManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Only GET / HEAD ever issue redirects; POST/PUT/DELETE go
		// through untouched and will 404 if the target doesn't exist.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		// Fast bail-outs: already scoped, API, MCP, asset, health.
		if path == "/" ||
			strings.HasPrefix(path, "/w/") ||
			strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/_mcp/") ||
			strings.HasPrefix(path, "/assets/") ||
			strings.HasPrefix(path, "/static/") ||
			path == "/health" || path == "/healthz" ||
			path == "/favicon.ico" || strings.HasSuffix(path, ".js") ||
			strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".map") ||
			strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".woff2") ||
			strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".svg") ||
			strings.HasSuffix(path, ".ico") {
			next.ServeHTTP(w, r)
			return
		}

		// Extract the first path segment and check the legacy page set.
		trimmed := strings.TrimPrefix(path, "/")
		head, tail, _ := strings.Cut(trimmed, "/")
		if _, ok := legacyUIPages[head]; !ok {
			next.ServeHTTP(w, r)
			return
		}

		// Resolve the active workspace. Without one, fall through and
		// let the SPA handle the (now-ambiguous) URL.
		if mgr == nil {
			next.ServeHTTP(w, r)
			return
		}
		entry := mgr.Registry().GetActive()
		if entry == nil {
			next.ServeHTTP(w, r)
			return
		}
		if entry.ID == "" {
			// Registry invariant says ID is an 8-char sha256 — an
			// empty ID means the entry was written without going
			// through RegisterWithAlias, usually a test leak. Warn
			// once per process so operators notice, then pass through.
			if emptyIDWarnFired.CompareAndSwap(false, true) {
				log.Warn("legacy_scope: active workspace has empty ID — registry invariant violated",
					"path", entry.Path, "name", entry.Name)
			}
			next.ServeHTTP(w, r)
			return
		}

		// Build the canonical URL. Preserve subpath + query so deep
		// links keep working.
		target := "/w/" + entry.ID + "/" + head
		if tail != "" {
			target += "/" + tail
		}
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}

		w.Header().Set("Deprecation", "true")
		w.Header().Set("Sunset", deprecationSunset)
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusMovedPermanently)
	})
}
