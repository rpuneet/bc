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

	"github.com/rpuneet/bc/pkg/log"
	"github.com/rpuneet/bc/pkg/workspace"
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

			// Only the active workspace has a live handler tree today; for
			// any other registered workspace return 501 with a hint to
			// activate it first.
			activeID := ""
			if active := mgr.Registry().GetActive(); active != nil {
				activeID = active.ID
				if activeID == "" {
					activeID = workspace.ComputeWorkspaceID(active.Path)
				}
			}
			if entry.ID != activeID {
				log.Info("scoped request for non-active workspace",
					"id", entry.ID, "active", activeID, "path", path)
				http.Error(w, `{"error":"non-active workspace scoped dispatch not implemented; POST /api/workspaces/{id}/activate first"}`, http.StatusNotImplemented)
				return
			}

			// Active workspace: rewrite /api/workspaces/{id}/<rest> → /api/<rest>
			// and propagate the resolved workspace via context so handlers
			// can access it if they wish.
			svc := mgr.Get(entry.ID)
			if svc == nil {
				// Lazy-load (rare: active workspace should already be loaded).
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
		w.Header().Set("Deprecation", "true")
		w.Header().Set("Sunset", deprecationSunset)
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
