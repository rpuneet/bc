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

	"github.com/rpuneet/mycel/pkg/agent"
	"github.com/rpuneet/mycel/pkg/cost"
	"github.com/rpuneet/mycel/pkg/cron"
	"github.com/rpuneet/mycel/pkg/events"
	"github.com/rpuneet/mycel/pkg/gateway"
	"github.com/rpuneet/mycel/pkg/mcp"
	"github.com/rpuneet/mycel/pkg/notify"
	"github.com/rpuneet/mycel/pkg/secret"
	"github.com/rpuneet/mycel/pkg/stats"
	"github.com/rpuneet/mycel/pkg/template"
	"github.com/rpuneet/mycel/pkg/tool"
	"github.com/rpuneet/mycel/pkg/workspace"
	"github.com/rpuneet/mycel/server/ws"
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
	// Degraded maps service name → reason for services that failed to
	// initialize (populated by server/build_services.go). Lets 503
	// responses explain WHY a service is missing.
	Degraded map[string]string
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
