# Proposal: Multi-Tenant bcd

> **Status:** Draft &nbsp;|&nbsp; **Date:** 2026-04-16 &nbsp;|&nbsp; **Related:** #2999, PR #2997
>
> **Supersedes** §4.2 of [multi-workspace-and-code-tab.md](./multi-workspace-and-code-tab.md)
> (the "active workspace only, 501 for others" compromise).
>
> **Not backward compatible.** Delete `--workspace` centrality, delete
> `Services` flat struct, delete legacy `/api/*` single-tenant shim.
> Migration is code-only — per-workspace data already lives in per-workspace
> `.bc/` dirs, nothing to move on disk.

---

## 1. Problem

bcd today is architecturally single-tenant. At boot it:

1. Loads one `*workspace.Workspace` from `--workspace <path>`
2. Builds a flat `server.Services{Agents, Costs, Cron, Templates, ...}`
3. Registers every handler with closures capturing that single Services

The registry at `~/.bc/workspaces.json` and the UI's `/w/<id>/` URLs create
the illusion of multi-workspace. The illusion breaks when you activate a
non-launch workspace — the URL changes but the handlers still close over the
launch workspace. We papered over this with a "501 Not Implemented" branch
in `server/workspace_scope.go` and a confirm dialog in the dropdown.

The user's direction: **delete the single-tenant assumption entirely.**
Make bcd dispatch to any registered workspace at request time.

## 2. Source-of-truth audit

No data migration needed. Each resource already has a clear home:

| Resource    | Source of truth (per workspace unless noted)                  |
|-------------|---------------------------------------------------------------|
| Registry    | `~/.bc/workspaces.json` (GLOBAL)                              |
| Agents      | `<ws>/.bc/agents/<agent-name>/` + `state.db`                  |
| Worktrees   | `<ws>/.bc/agents/<name>/bc-<wsID>-<name>/`                    |
| Channels    | `<ws>/.bc/state.db` (`channels` table)                        |
| Events      | `<ws>/.bc/state.db` (`events` table)                          |
| Settings    | `<ws>/.bc/settings.json`                                      |
| Templates   | `<ws>/.bc/templates/*.{json,md}`                              |
| Secrets     | `<ws>/.bc/secrets.db`                                         |
| Costs       | `<ws>/.bc/costs.db` OR global TimescaleDB                     |
| Cron        | `<ws>/.bc/cron.db`                                            |
| MCP config  | `<ws>/.bc/settings.json` + `<ws>/.mcp.json`                   |
| Stats TSDB  | GLOBAL TimescaleDB when `bc-db` dep runs                      |
| Deps        | GLOBAL — `bc-db`, `bc-code-server`, `bc-browser`              |
| GitHub tok  | `~/.bc/github-token` (GLOBAL)                                 |

Naming confirms isolation: tmux sessions and docker containers use
`bc-<wsID>-<agent>` where `wsID` is `sha256(absPath)[:12]`. Two workspaces
with the same basename already don't collide.

## 3. Architecture

### Before

```
bc up --workspace /path/to/trade
    │
    v
RunServer(…)
    │
    ├── Load workspace "trade"
    ├── Build Services{Agents, Costs, …} from trade's .bc/
    ├── handlers.NewAgentsHandler(svc.Agents)   ← closure over trade's manager
    ├── handlers.NewChannelsHandler(svc.Channels)
    └── srv.Start(ctx)      ← blocks forever, bound to trade
```

### After

```
bc up                         # no --workspace required
    │
    v
RunServer(…)
    │
    ├── Load Registry (~/.bc/workspaces.json)
    ├── WorkspaceManager{ factory: buildWorkspaceServices }
    │       └─ lazy Load(wsID) on first access
    ├── handlers.NewAgentsHandler(wsMgr)   ← takes the manager
    ├── handlers.NewChannelsHandler(wsMgr)
    └── srv.Start(ctx)

Request: GET /api/workspaces/688a38.../agents
    │
    v
WorkspaceScope middleware
    │
    ├── Parse wsID from URL path
    ├── mgr.Load(ctx, wsID)         ← lazy-loads full services
    ├── Stash *WorkspaceServices in r.Context()
    ├── Rewrite URL to /api/agents
    └── next.ServeHTTP(w, r)
                │
                v
          AgentsHandler.ServeHTTP
                │
                ├── svc := WorkspaceServicesFromContext(r.Context())
                ├── agents := svc.Agents.List(ctx)
                └── writeJSON(agents)
```

## 4. `WorkspaceServices` — full struct

Owns **everything per-workspace**, including what used to be global-ish
background loops. Built once per workspace, torn down on eviction.

```go
// server/workspace_services.go

type WorkspaceServices struct {
    Workspace    *workspace.Workspace   // config + paths
    Agents       *agent.Manager          // per-ws agent lifecycle
    AgentSvc     *agent.AgentService     // higher-level wrapper used by handlers
    Channels     *channel.Manager
    Events       events.EventStore       // SQLite (shared DB instance)
    EventWriter  *events.JSONLWriter     // .bc/events.jsonl tail
    Costs        *cost.Store
    CostImporter *cost.Importer
    Cron         *cron.Store
    CronSched    *cron.Scheduler
    Templates    *template.Store
    Secrets      *secret.Store
    MCP          *mcp.Store
    Tools        *tool.Store             // per-workspace tool registry
    Gateway      *gateway.Manager        // if workspace has gateways configured
    Notify       *notify.Service
    Hub          *ws.Hub                 // per-workspace SSE hub

    // Lifecycle: started when Load succeeds, stopped on Evict.
    cancel       context.CancelFunc
    wg           sync.WaitGroup
}

// Close stops all background goroutines and closes stores.
// Safe to call once; idempotent.
func (s *WorkspaceServices) Close() error { … }
```

### Global (not per-workspace)

| Global service | Owner                                          |
|----------------|------------------------------------------------|
| Stats TSDB     | `server.Globals.Stats *stats.Store`            |
| Deps manager   | `server.Globals.Deps *deps.Registry`           |
| GitHub token   | `server.Globals.GithubToken` (from file)       |
| Registry       | `server.Globals.Registry *workspace.Registry`  |
| Discovery      | uses Registry directly                         |

## 5. `WorkspaceManager` contract

```go
type WorkspaceManager struct {
    globals  *Globals
    registry *workspace.Registry
    factory  func(ctx, ws) (*WorkspaceServices, error)
    cache    map[string]*cachedEntry   // wsID -> services + last-used
    mu       sync.RWMutex
}

// Load returns fully-initialized services for wsID. Lazy-loads on first
// access; subsequent calls return the cached instance. Refreshes last-used.
func (m *WorkspaceManager) Load(ctx, wsID) (*WorkspaceServices, error)

// Get returns a loaded instance or nil. Does NOT auto-load.
func (m *WorkspaceManager) Get(wsID) *WorkspaceServices

// Evict stops + closes services for wsID.
func (m *WorkspaceManager) Evict(wsID) error

// List returns currently-loaded workspace IDs.
func (m *WorkspaceManager) List() []string

// StartEvictionLoop evicts services unused for >30min every 5min.
func (m *WorkspaceManager) StartEvictionLoop(ctx)

// Close evicts everything; call on bcd shutdown.
func (m *WorkspaceManager) Close() error
```

**Lifecycle invariants:**

- Background goroutines (tool health loop, cost importer, cron scheduler,
  stats collector, event prune) live inside `WorkspaceServices`. Their
  parent `context.Context` is cancelled on `Evict()`, then `wg.Wait()` ensures
  clean shutdown.
- First request to a workspace triggers `Load()` which performs all the
  heavy init (open DB connections, migrate schemas, start goroutines).
  Subsequent requests hit the cache.
- Eviction after 30min idle is purely a memory optimization. Nothing
  persists in-memory that isn't also on disk.

## 6. Handler refactors

Every handler that closed over Services now takes `*WorkspaceManager` and
resolves services from `r.Context()` via `server.WorkspaceServicesFromContext`.
Patterns are identical; only constructor signature + one line inside
ServeHTTP changes.

Each handler's change:

| File (server/handlers/) | Current constructor                                    | New constructor                  |
|-------------------------|--------------------------------------------------------|----------------------------------|
| `agents.go`             | `NewAgentHandler(svc.Agents, svc.Costs, svc.WS, hub)`  | `NewAgentHandler(mgr, globals)`  |
| `channels.go`           | `NewChannelsHandler(ch, svc.Agents, hub)`              | `NewChannelsHandler(mgr)`        |
| `cost.go`               | `NewCostHandler(svc.Costs, svc.CostImporter)`          | `NewCostHandler(mgr)`            |
| `cron.go`               | `NewCronHandler(svc.Cron, svc.CronSched)`              | `NewCronHandler(mgr)`            |
| `events.go`             | `NewEventsHandler(svc.Events)`                         | `NewEventsHandler(mgr)`          |
| `files.go`              | (uses workspace path)                                   | takes mgr, path from services    |
| `mcp.go`                | `NewMCPHandler(svc.MCP)`                               | `NewMCPHandler(mgr)`             |
| `notify.go`             | `NewNotifyHandler(svc.Notify)`                         | `NewNotifyHandler(mgr)`          |
| `providers.go`          | `NewProvidersHandler(cfg)`                             | unchanged — global               |
| `roles.go`              | DELETE                                                 | DELETE (replaced by templates)   |
| `secrets.go`            | `NewSecretsHandler(svc.Secrets)`                       | `NewSecretsHandler(mgr)`         |
| `settings.go`           | `NewSettingsHandler(svc.WS)`                           | `NewSettingsHandler(mgr)`        |
| `stats.go`              | `NewStatsHandler(svc.Stats)`                           | takes mgr + globals.Stats        |
| `templates.go`          | `NewTemplateHandler(svc.Templates, svc.Agents)`        | `NewTemplateHandler(mgr)`        |
| `tools.go`              | `NewToolHandler(svc.Tools)`                            | `NewToolHandler(mgr)`            |
| `workspace.go`          | active-workspace handler                                | keep but pulls from mgr.Active() |
| `workspaces.go`         | registry management                                     | no changes; already uses mgr     |
| `discovery.go`          | local/github/clone                                      | no changes; uses registry        |
| `deps.go`               | uses globals.Deps                                       | no changes; already global       |
| `code.go`               | workspace-resolver based                                | small tweak for context-based    |

**Inside every ServeHTTP:**

```go
func (h *Foo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    svc := server.WorkspaceServicesFromContext(r.Context())
    if svc == nil {
        http.Error(w, "workspace not resolved", http.StatusBadRequest)
        return
    }
    // ... use svc.Agents / svc.Channels / etc ...
}
```

## 7. SSE hub: per-workspace with global fan-in

**Before:** one global hub.
**After:**

- Each `WorkspaceServices` owns its own `*ws.Hub`.
- Agent hook events publish to their workspace's hub.
- `GET /api/workspaces/{id}/events` subscribes to that hub.
- `GET /api/events` (global) subscribes to a fan-in hub that forwards from
  all loaded workspaces' hubs, annotating each event with `workspace_id`.

This ensures:
- Scoped dashboards don't leak events between workspaces
- The global Live view still works for users who want to see everything
- Eviction cleanly closes the per-workspace hub + any fan-in subscription

## 8. MCP routing

**Before:** `/_mcp/<agent>/sse` and `/message` — single namespace.

**After:** `/_mcp/<wsID>/<agent>/sse` and `/_mcp/<wsID>/<agent>/message`.

- `BC_AGENT_MCP_URL` env set on the agent at spawn uses the wsID-prefixed path
- Agents inside workspaces post MCP messages to their scoped endpoint
- No collision between same-named agents in different workspaces

Migration note: running agents were spawned with the old path in their
claude settings.json. The `updateAgentHookPorts` function already rewrites
that file on bcd startup; extend it to also rewrite MCP paths.

## 9. Hook endpoint

**Current:** `POST /api/agents/{name}/hook` at a flat path.

**New:**
- Primary path: `POST /api/workspaces/{wsID}/agents/{name}/hook`
- Agent's `.claude/settings.json` hook commands include `BC_WORKSPACE` env
  so the body also carries the wsID for cross-check
- Legacy `/api/agents/{name}/hook` returns 410 Gone with a pointer to the
  scoped URL (not 404 so users can see the error)

The `updateAgentHookPorts` rewriter in `serve.go` rewrites existing agent
settings.json files to point at the scoped URL on startup.

## 10. What gets deleted

| Path                                                      | Why                                            |
|-----------------------------------------------------------|------------------------------------------------|
| `server.Services` struct                                  | single-tenant artifact                         |
| `server.Services` field in `server.New`                   | replaced by `*WorkspaceManager`                |
| `server/workspace_scope.go` 501 branch                    | all workspaces now dispatch                    |
| `server/workspace_scope.go` legacy deprecation headers    | legacy `/api/*` routes removed entirely        |
| `server.WorkspacesHandler.activeRefresh` hook             | no hot-swap needed                             |
| `server/handlers/roles.go`                                | templates replaced roles                       |
| `internal/cmd/serve.go` stub WorkspaceFactory             | replaced with real factory                     |
| `internal/cmd/up.go` mandatory `--workspace` check        | now optional (registers path if provided)      |
| `web/src/components/WorkspaceDropdown.tsx` confirm dialog | true switching works, no warning needed       |
| `web/src/views/Workspace.tsx` (already deleted)           | folded into Settings                           |
| Legacy redirect routes in `web/src/App.tsx`               | keep for one release; remove in follow-up     |

## 11. Phased rollout (7 phases)

Each phase leaves `bcd` building, testing, and servable.

### Phase M1 — `WorkspaceServices` inventory

Widen `WorkspaceServices` struct to cover every field. Still only one is
built (the launch workspace). `Services` stays alongside. No behavior
change. **Goal:** give ourselves a typed seam.

### Phase M2 — Real factory

Extract the giant initialization block in `serve.go` into
`buildWorkspaceServices(ctx, globals, wsRoot)`. Called once at boot for the
launch workspace. Still the only call-site. **Goal:** factoring without
semantics change.

### Phase M3 — Handlers read context

Each handler grows `WorkspaceServicesFromContext(r.Context())` at the top of
`ServeHTTP`. The middleware stashes the launch workspace's services on
every request. Handlers still compile fine with their old fields too
(belt + suspenders). **Goal:** prove the dispatch works with one workspace.

### Phase M4 — Remove closure deps

Drop each handler's old store fields; handlers rely on context. Now
`NewAgentsHandler(mgr)` is the only constructor. `Services` struct deleted.
**Goal:** single source of services per request.

### Phase M5 — Multi-workspace dispatch

Make `WorkspaceManager.Load(ctx, wsID)` actually build services for non-launch
workspaces. Delete the 501 branch. Eviction loop activates. **Goal:**
switching workspaces via the dropdown just works.

### Phase M6 — Per-workspace SSE + MCP

Move the SSE hub into `WorkspaceServices`. Add global fan-in hub. Scope MCP
paths. Rewrite agent settings.json on startup. **Goal:** no cross-workspace
event or MCP bleed.

### Phase M7 — Cleanup

- Delete `server.Services` struct entirely
- Delete legacy `/api/*` backend routes (keep web-side redirects to `/w/...`)
- Delete the `WorkspaceDropdown` confirm dialog
- Update proposal docs (this one supersedes the relevant §§ of the earlier one)
- Add integration test: spin 2 workspaces with 2 agents each, confirm
  zero cross-contamination

## 12. Testing

**Unit**
- `WorkspaceManager_Load_caches` — second load is instant
- `WorkspaceManager_Evict_closes_services` — goroutines stop
- `WorkspaceScope_context_populated` — middleware stashes services
- `WorkspaceScope_unknown_workspace_404`

**Integration** (new file: `server/multi_tenant_test.go`)
- Spin up an in-process `bcd` with two tmpdir workspaces
- Create an agent `alice` in workspace A, `bob` in workspace B
- `GET /api/workspaces/<A>/agents` returns only `alice`
- `GET /api/workspaces/<B>/agents` returns only `bob`
- Fire a hook event to A's alice; confirm B's event log is empty
- Fire a channel message in A; confirm B's channels are unchanged
- Evict A; reload; data intact

## 13. Verification checklist (for #2999 test runs)

- [ ] Switching workspaces in the dropdown shows the **new** workspace's
      agents/channels/events without restarting bcd
- [ ] Creating an agent in workspace A does not appear in workspace B
- [ ] Cost records scope correctly per workspace
- [ ] SSE `/api/events` shows events from all loaded workspaces with
      `workspace_id` annotations
- [ ] `/api/workspaces/<id>/events` only shows that workspace's events
- [ ] Evicting an idle workspace stops its goroutines (goroutine count
      stable after an hour)
- [ ] `bc up` without `--workspace` boots successfully with empty registry
- [ ] `bc up --workspace <path>` registers that path and marks it active

## 14. Risks & mitigations

- **Goroutine leaks** — every goroutine started inside
  `buildWorkspaceServices` must use the services' cancel ctx. Audit
  with `runtime.NumGoroutine()` before/after eviction in tests.
- **Open file handles** — SQLite connections held by evicted services must
  be closed. The `Close()` on each store is called.
- **Agent tmux sessions survive eviction** — evicting a workspace does NOT
  kill agent sessions; they keep running and reattach when the workspace is
  re-loaded. The `agent.Manager` re-scans on next Load.
- **In-flight SSE subscribers on evicted workspace** — they get disconnected
  cleanly when the hub closes. Browser auto-reconnects via EventSource
  retry.

## 15. Rollback

Nothing to roll back — the data is untouched throughout. Reverting the code
commits reverts the behavior. No on-disk migration to undo.
