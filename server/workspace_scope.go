// workspace_scope.go implements per-request workspace scoping for flat
// /api/* routes (#3079).
//
// Overview
// ========
//
// Every workspace-scoped resource is served at /api/<resource>/... and the
// scope is selected per request via one of:
//
//   - X-BC-Workspace: <id>     header (preferred — SPA sets it automatically)
//   - ?workspace=<id>          query param (curl-friendly fallback)
//   - none → the registry's active workspace
//
// The middleware resolves the requested workspace via the WorkspaceManager
// (lazy-loading if necessary), then annotates the request context with the
// resolved WorkspaceServices so handlers can pull the right per-workspace
// view via WorkspaceServicesFromContext.
//
// The historical /api/workspaces/{id}/<rest> path-scoped rewrite was
// deleted alongside this rewrite — clients must use the header or query
// param. Registry self-routes (/api/workspaces, /api/workspaces/{id},
// /api/workspaces/{id}/activate, /api/workspaces/discover/*, /api/workspaces/clone)
// still work and are served directly by their own handlers.
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

// WorkspaceScope returns a middleware that resolves a per-request
// workspace scope for flat /api/* routes, prefers an explicit
// X-BC-Workspace header, falls back to the ?workspace=<id> query
// param, and finally to the registry's active workspace.
//
// The middleware is a no-op for any path that is not /api/... or for
// the /api/workspaces self-routes (collection, detail, activate,
// discover/*, clone), which are owned by the WorkspacesHandler and
// don't need a per-request scope.
//
// mgr may be nil — in which case the middleware is a pure pass-through.
// If mgr is non-nil but the requested id does not resolve, the request
// gets a 404.
func WorkspaceScope(next http.Handler, mgr *WorkspaceManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Non-API paths pass through.
		if !strings.HasPrefix(path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		// Registry self-routes — owned by WorkspacesHandler, no
		// per-request workspace scope to inject.
		if strings.HasPrefix(path, "/api/workspaces") {
			next.ServeHTTP(w, r)
			return
		}

		if mgr == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Explicit scope hint — header preferred, query param fallback.
		// The SPA sends X-BC-Workspace automatically once it detects
		// /w/<id> in the URL; the query form is the curl-friendly
		// equivalent and the surface we're standardizing on for
		// external callers (#3079).
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
			// Legacy registry entries can lack a persisted ID; mirror the
			// active-fallback path and derive one from the workspace path
			// so mgr.Get/Load receive a stable key.
			resolvedID := entry.ID
			if resolvedID == "" {
				resolvedID = workspace.ComputeWorkspaceID(entry.Path)
			}
			svc := mgr.Get(resolvedID)
			if svc == nil {
				var err error
				svc, err = mgr.Load(r.Context(), resolvedID)
				if err != nil {
					log.Warn("workspace scope: lazy load failed", "id", resolvedID, "error", err)
					http.Error(w, `{"error":"failed to load workspace"}`, http.StatusInternalServerError)
					return
				}
			}
			ctx := context.WithValue(r.Context(), ctxKeyWorkspaceID, resolvedID)
			ctx = context.WithValue(ctx, ctxKeyWorkspaceServices, svc)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fall back to the registry's active workspace.
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
