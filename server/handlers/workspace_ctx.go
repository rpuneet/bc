// workspace_ctx.go — handler-side shim for per-request workspace resolution.
//
// Phase M3 wires the server's WorkspaceServicesFromContext lookup into the
// handlers package without creating an import cycle. Handlers read their
// per-workspace dependencies via WorkspaceFromContext(r.Context()) at the
// top of each method; if the lookup returns nil they fall back to the
// closure fields captured at construction time (legacy single-workspace
// code path).
//
// Phase M4 will delete the closure fields; every handler will then rely
// solely on context.
package handlers

import (
	"context"
	"sync/atomic"

	"github.com/rpuneet/bc/pkg/agent"
	"github.com/rpuneet/bc/pkg/cost"
	"github.com/rpuneet/bc/pkg/cron"
	"github.com/rpuneet/bc/pkg/events"
	"github.com/rpuneet/bc/pkg/gateway"
	"github.com/rpuneet/bc/pkg/mcp"
	"github.com/rpuneet/bc/pkg/notify"
	"github.com/rpuneet/bc/pkg/secret"
	"github.com/rpuneet/bc/pkg/stats"
	"github.com/rpuneet/bc/pkg/template"
	"github.com/rpuneet/bc/pkg/tool"
	"github.com/rpuneet/bc/pkg/workspace"
	"github.com/rpuneet/bc/server/ws"
)

// WorkspaceView is the subset of per-workspace dependencies handlers need.
// The server populates this in-process and exposes a lookup via
// SetWorkspaceFromContext.
type WorkspaceView struct {
	Workspace    *workspace.Workspace
	Agents       *agent.AgentService
	Events       events.EventStore
	EventWriter  *events.JSONLWriter
	Costs        *cost.Store
	CostImporter *cost.Importer
	Cron         *cron.Store
	CronSched    *cron.Scheduler
	Templates    *template.Store
	Secrets      *secret.Store
	MCP          *mcp.Store
	Tools        *tool.Store
	Gateway      *gateway.Manager
	Notify       *notify.Service
	Stats        *stats.Store
	Hub          *ws.Hub
}

// lookup holds the server-installed WorkspaceView resolver. Atomic.Pointer
// avoids locking on every request.
var workspaceLookup atomic.Pointer[func(context.Context) *WorkspaceView]

// SetWorkspaceFromContext installs the server's resolver. Passing nil
// clears it (useful for tests).
func SetWorkspaceFromContext(fn func(context.Context) *WorkspaceView) {
	if fn == nil {
		workspaceLookup.Store(nil)
		return
	}
	workspaceLookup.Store(&fn)
}

// WorkspaceFromContext returns the per-request workspace view installed by
// the scoping middleware, or nil if no resolver is wired or the request
// was not scoped.
func WorkspaceFromContext(ctx context.Context) *WorkspaceView {
	fn := workspaceLookup.Load()
	if fn == nil {
		return nil
	}
	return (*fn)(ctx)
}
