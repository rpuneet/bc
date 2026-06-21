// workspace_scope.go implements URL-level workspace scoping middleware.
//
// Overview
// ========
//
// The bcd server historically exposes every resource at /api/<resource>/...
// bound to a single workspace. With multi-workspace support the proposal
// introduces two URL schemes:
//
//	/api/workspaces/{id}/<rest>     — scoped (canonical, post-migration)
//	/api/<rest>                     — legacy (active workspace shim)
//
// The middleware in this file performs two jobs:
//
//  1. Scoped rewrite: if the path matches /api/workspaces/{id}/<rest> AND
//     {rest} is NOT empty AND NOT a registry-management route (detail,
//     activate, etc.), rewrite the request's URL.Path to /api/<rest> and
//     annotate the request context with the resolved WorkspaceServices.
//     Existing handlers continue to use their closure-captured services
//     (the active workspace's). If the {id} is the active workspace, this
//     is a no-op rewrite and everything works. If {id} is a different
//     registered workspace, we return 501 Not Implemented — full per-
//     workspace handler dispatch is a future phase; switch via POST
//     /api/workspaces/{id}/activate.
//
//  2. Legacy headers: if the path starts with /api/ but NOT /api/workspaces,
//     the response is annotated with Deprecation: true and a Sunset date so
//     clients can warn when they are still using the old shape. The handler
//     chain is unchanged (handlers serve the active workspace as before).
//
// This split keeps the heavy refactor — per-workspace handler trees — for a
// later phase while delivering the URL surface specified in proposal §4.5.
package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/rpuneet/mycel/pkg/log"
	"github.com/rpuneet/mycel/pkg/workspace"
)

// ctxKey is an unexported type for context keys so external packages can't
// collide.
type ctxKey int

const (
	ctxKeyWorkspaceID       ctxKey = iota // string, resolved workspace id
	ctxKeyWorkspaceServices               // *WorkspaceServices, loaded
)

// deprecationSunset is the date at which legacy /api/<rest> routes will stop
// working. Matches proposal §9.3.
const deprecationSunset = "Sun, 01 Nov 2026 00:00:00 GMT"

// These sub-paths are registry management, not per-workspace dispatch — the
// scoped rewrite must NOT strip the /api/workspaces/{id} prefix.
var workspaceSelfRoutes = map[string]struct{}{
	"":         {},
	"activate": {},
}

// These top-level sub-paths are NOT workspace IDs — they live at
// /api/workspaces/<name>/* and are served directly by discovery/registry
// handlers. The scope middleware must pass them through unchanged.
var workspacesReservedPrefixes = map[string]struct{}{
	"discover": {},
	"clone":    {},
}

// WorkspaceScope returns a middleware that:
//   - rewrites /api/workspaces/{id}/<rest> → /api/<rest> when <rest> is a
//     scoped resource (not a self-route), after resolving {id} to a
//     loaded *WorkspaceServices.
//   - annotates legacy /api/<rest> responses with Deprecation / Sunset.
//
// The middleware is a no-op for any path that is not /api/... .
//
// mgr may be nil — in which case scoped rewrites are disabled (registry
// CRUD still works since it's served by handlers registered directly on the
// mux). If mgr is non-nil but the {id} does not resolve, the request gets a
// 404.
//
// Matching rules for scoped routes (see §4.5):
//
//	GET    /api/workspaces/{id}          -> registry detail (self route)
//	POST   /api/workspaces/{id}/activate -> registry action (self route)
//	* /api/workspaces/{id}/agents        -> scoped (rewrite)
//	* /api/workspaces/{id}/channels      -> scoped (rewrite)
//	* /api/workspaces/{id}/events        -> scoped (rewrite)
//	etc.
func WorkspaceScope(next http.Handler, mgr *WorkspaceManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Non-API paths pass through.
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Scoped: /api/workspaces/{id}/...
		if strings.HasPrefix(path, "/api/workspaces/") {
			rest := strings.TrimPrefix(path, "/api/workspaces/")
			id, tail, _ := strings.Cut(rest, "/")
			if id == "" {
				next.ServeHTTP(w, r)
				return
			}
			// Reserved top-level names (discover, clone, etc.) are NOT
			// workspace IDs — forward without touching the URL.
			if _, ok := workspacesReservedPrefixes[id]; ok {
				next.ServeHTTP(w, r)
				return
			}
			// Registry self-routes pass straight through to the
			// WorkspacesHandler registered on the mux.
			if _, ok := workspaceSelfRoutes[tail]; ok {
				next.ServeHTTP(w, r)
				return
			}

			// Scoped dispatch — resolve the workspace.
			if mgr == nil {
				http.Error(w, `{"error":"workspace manager not configured"}`, http.StatusNotImplemented)
				return
			}

			// Must be a registered workspace.
			entry := mgr.Registry().FindByID(id)
			if entry == nil {
				entry = mgr.Registry().Resolve(id)
			}
			if entry == nil {
				http.Error(w, `{"error":"workspace not found"}`, http.StatusNotFound)
				return
			}

			// Phase M5: any registered workspace dispatches via the
			// manager. First access lazy-loads the services; subsequent
			// requests hit the cache.
			svc := mgr.Get(entry.ID)
			if svc == nil {
				var err error
				svc, err = mgr.Load(r.Context(), entry.ID)
				if err != nil {
					log.Warn("workspace scope: lazy load failed", "id", entry.ID, "error", err)
					http.Error(w, `{"error":"failed to load workspace"}`, http.StatusInternalServerError)
					return
				}
			}
			ctx := context.WithValue(r.Context(), ctxKeyWorkspaceID, entry.ID)
			ctx = context.WithValue(ctx, ctxKeyWorkspaceServices, svc)

			r2 := r.Clone(ctx)
			r2.URL.Path = "/api/" + tail
			if r.URL.RawPath != "" {
				// Preserve escaping if the original had one.
				rawRest := strings.TrimPrefix(r.URL.RawPath, "/api/workspaces/")
				_, rawTail, _ := strings.Cut(rawRest, "/")
				r2.URL.RawPath = "/api/" + rawTail
			}
			next.ServeHTTP(w, r2)
			return
		}

		// Legacy: /api/<rest> (not /api/workspaces/...)
		// Mark as deprecated and stash the active workspace's services in
		// context so handlers can transition to context-based resolution
		// without needing scoped URLs. Phase M3.
		w.Header().Set("Deprecation", "true")
		w.Header().Set("Sunset", deprecationSunset)

		if mgr != nil {
			// X-BC-Workspace header OR ?workspace= query param overrides
			// the active workspace, allowing clients to scope flat /api/
			// routes to a specific workspace without using the
			// /api/workspaces/{id}/... URL form. The header is preferred
			// (browsers send it automatically once the SPA detects a
			// /w/<id> path), but the query param is convenient for
			// curl-style ad-hoc calls and is the form we're standardising
			// on as we delete the /api/workspaces/{id} surface (#3079).
			hdrID := r.Header.Get("X-BC-Workspace")
			if hdrID == "" {
				hdrID = r.URL.Query().Get("workspace")
			}
			if hdrID != "" {
				entry := mgr.Registry().FindByID(hdrID)
				if entry == nil {
					entry = mgr.Registry().Resolve(hdrID)
				}
				if entry == nil {
					http.Error(w, `{"error":"workspace not found"}`, http.StatusNotFound)
					return
				}
				svc := mgr.Get(entry.ID)
				if svc == nil {
					var err error
					svc, err = mgr.Load(r.Context(), entry.ID)
					if err != nil {
						log.Warn("workspace scope: lazy load failed", "id", entry.ID, "error", err)
						http.Error(w, `{"error":"failed to load workspace"}`, http.StatusInternalServerError)
						return
					}
				}
				ctx := context.WithValue(r.Context(), ctxKeyWorkspaceID, entry.ID)
				ctx = context.WithValue(ctx, ctxKeyWorkspaceServices, svc)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if active := mgr.Active(); active != nil {
				activeID := ""
				if e := mgr.Registry().GetActive(); e != nil {
					activeID = e.ID
					if activeID == "" {
						activeID = workspace.ComputeWorkspaceID(e.Path)
					}
				}
				ctx := context.WithValue(r.Context(), ctxKeyWorkspaceID, activeID)
				ctx = context.WithValue(ctx, ctxKeyWorkspaceServices, active)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// WorkspaceServicesFromContext extracts the per-request *WorkspaceServices
// that WorkspaceScope stored under ctxKeyWorkspaceServices. Returns nil if
// the middleware did not fire (legacy route) or the scope was for the
// active workspace and no services handle was stored.
func WorkspaceServicesFromContext(ctx context.Context) *WorkspaceServices {
	if v, ok := ctx.Value(ctxKeyWorkspaceServices).(*WorkspaceServices); ok {
		return v
	}
	return nil
}

// WorkspaceIDFromContext returns the scoped workspace ID set by the
// WorkspaceScope middleware, or an empty string if unset.
func WorkspaceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyWorkspaceID).(string); ok {
		return v
	}
	return ""
}
