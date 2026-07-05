# Proposal: Multi-Tenant bcd

> **Status:** Implemented &nbsp;|&nbsp; **Date:** 2026-04-16 &nbsp;|&nbsp; **Related:** #2999, PR #2997
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

- `MYCEL_AGENT_MCP_URL` env set on the agent at spawn uses the wsID-prefixed path
- Agents inside workspaces post MCP messages to their scoped endpoint
- No collision between same-named agents in different workspaces

Migration note: running agents were spawned with the old path in their
claude settings.json. The `updateAgentHookPorts` function already rewrites
that file on bcd startup; extend it to also rewrite MCP paths.

## 9. Hook endpoint

**Current:** `POST /api/agents/{name}/hook` at a flat path.

**New:**
- Primary path: `POST /api/workspaces/{wsID}/agents/{name}/hook`
- Agent's `.claude/settings.json` hook commands include `MYCEL_WORKSPACE` env
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

- [x] Switching workspaces in the dropdown shows the **new** workspace's
      agents/channels/events without restarting bcd (phase M5 — scoped
      URL routing via WorkspaceManager; WorkspaceDropdown confirm
      dialog removed)
- [x] Creating an agent in workspace A does not appear in workspace B
      (server/multi_tenant_test.go:TestMultiTenant_AgentsIsolatedPerWorkspace)
- [x] Cost records scope correctly per workspace (each
      WorkspaceServices opens its own cost.Store against the workspace's
      .bc/ dir via BuildWorkspaceServices)
- [x] SSE `/api/events` shows events from all loaded workspaces with
      `workspace_id` annotations (phase M6 — Hub.ForwardTo fan-in)
- [x] `/api/workspaces/<id>/events` only shows that workspace's events
      (scope middleware stashes per-workspace Hub in context)
- [x] Evicting an idle workspace stops its goroutines
      (WorkspaceServices.Close cancels child context and waits on wg)
- [x] `bc up` without `--workspace` boots successfully with empty registry
- [x] `bc up --workspace <path>` registers that path and marks it active

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

---

# Part II — Decoupled Product Architecture (Phases M8-M10)

M1-M7 made bcd multi-tenant over the existing fat `.bc/` model. That unblocks
workspace switching but leaves the *product* fundamentally the same shape. If
we want each workspace to be a cheap, disposable thing rather than a silo
holding user-scoped state, we need to split layers.

## 16. Five natural layers

```
L1  MACHINE        docker engine, tmux, Go runtime, bc-db/code-server       bc just uses
L2  USER           ~/.bc/  templates, secrets vault, GitHub tok,            follows the user
                    tool registry, MCP trust, cost ledger, workspace index
L3  PROJECT        a git repo — code + remote + branch (no BC state)        owned by repo
L4  WORKSPACE      <ws>/.bc/  thin state.db (channels, events, cron),       one per "session
                    settings overrides, agent runtime, pointers upward       of work"
L5  AGENT          live tmux/docker session, worktree, hook stream          ephemeral
```

Today L2, L3 partially, and L4 are all fused inside `<ws>/.bc/`. That is
the root cause of:

- Reseeding templates per workspace (L2 stuff leaking into L4)
- Retyping ANTHROPIC_API_KEY per workspace (L2 in L4)
- Not being able to see "cost across projects" (L2 in L4)
- Not being able to have N workspaces on the same repo (L3 == L4 assumption)
- Not being able to archive a workspace and keep your templates/keys (L2 in L4)

## 17. Proposed split

### Move to `~/.bc/` (user-scoped, machine-portable with sync)

| File/dir                   | What                                        | Per-ws override? |
|----------------------------|---------------------------------------------|------------------|
| `~/.bc/templates/*.md,json`| Agent recipes                                | yes, via `<ws>/.bc/templates/` |
| `~/.bc/secrets.vault`      | Encrypted KV (API keys, tokens)              | yes, `<ws>/.bc/secrets.db` for ws-only keys |
| `~/.bc/mcps.json`          | Trusted MCP servers + default env            | yes |
| `~/.bc/tools.json`         | Installed CLI tools registry                 | no (machine-level) |
| `~/.bc/costs.db`           | All cost records with `workspace_id` column  | no (global ledger) |
| `~/.bc/github-token`       | already here; unchanged                      | no |
| `~/.bc/registry.json`      | list of workspaces (already here)            | no |
| `~/.bc/prefs.json`         | NEW: theme, locale, default runtime          | no |

### Stays in `<ws>/.bc/` (workspace-scoped, disposable)

| File/dir            | What                                              |
|---------------------|---------------------------------------------------|
| `state.db`          | channels, events, agent state, task graph         |
| `cron.db`           | jobs that run against THIS workspace's agents     |
| `agents/<name>/`    | live runtime (tmux session dir, worktree)         |
| `settings.json`     | overrides on top of user prefs — typically tiny   |
| `env.json`          | workspace-scoped env vars (today: per-agent)      |

### New `<project>/<empty>` — just a git repo

No `.bc/` required at the project level unless the user explicitly does
`bc workspace init <path>`. A "workspace" can live outside any repo (scratch
mode) or reference a repo by path.

## 18. Registry entry shape change

```go
// before
type RegistryEntry struct {
    ID string          // sha256(abs Path)[:12]
    Path string        // /Users/puneet/Projects/trade  ← project AND ws
    Name string        // "trade"
    ...
}

// after
type RegistryEntry struct {
    ID          string  // sha256(abs path)[:12]
    Name        string  // "trade-prod" (workspace identity)
    ProjectPath string  // /Users/puneet/Projects/trade — the git repo (optional for scratch)
    GitHubURL   string  // cached from remote
    DataDir     string  // <project>/.bc or ~/.bc/workspaces/<name>/ for scratch
    ...
}
```

This lets two workspaces (`trade-prod`, `trade-paper`) share `ProjectPath`
`/Users/puneet/Projects/trade` while having separate `DataDir`s.

## 19. Phases

### Phase M8 — Promote user assets

1. On bcd startup, if `~/.bc/templates/` is empty, migrate from the first
   registered workspace's `.bc/templates/` (one-time merge). Same for
   secrets (`secrets.db` → `~/.bc/secrets.vault`), MCP config, costs.
2. `WorkspaceServices` now reads from user-global stores by default:
   ```go
   svc.Templates = globalTemplates.WithOverride(ws.TemplateDirIfExists())
   svc.Secrets   = globalSecrets.WithOverride(ws.SecretsDirIfExists())
   svc.MCP       = globalMCP.WithOverride(ws.MCPPathIfExists())
   svc.Costs     = globalCosts.ScopedTo(ws.ID)
   ```
3. Delete per-workspace `templates/`, `secrets.db`, `.mcp.json`,
   `costs.db` on next open (archive to `.bc/.migrated/`).
4. New commands:
   - `bc template edit <name>` → `~/.bc/templates/<name>.md`
   - `bc secret add KEY=VALUE` → user vault
   - `bc secret add KEY=VALUE --workspace trade` → ws override
   - `bc cost report --by project` → SELECT SUM GROUP BY project

**Acceptance:**
- Tuning a template in one workspace affects ALL workspaces
- Adding a secret once makes it available in every workspace
- `bc cost report` shows totals across all projects

### Phase M9 — N workspaces per project

1. Change `RegistryEntry` to separate `Name`, `ProjectPath`, `DataDir`.
2. `bc workspace new <name> --project <path>` creates a new workspace with
   its own `DataDir` (either `<path>/.bc-<name>` or
   `~/.bc/workspaces/<name>`). Multiple can share `<path>`.
3. Update tmux/docker naming to use workspace `Name+ID` (already uses ID).
4. Update UI: group workspace dropdown by project, show workspace name as
   primary label, project path as secondary.

**Acceptance:**
- Two workspaces pointing at `/Projects/trade` work independently
- Agent `alice` in `trade-prod` and agent `alice` in `trade-paper` don't
  collide at the tmux/docker layer
- GET /api/workspaces returns distinct entries for both

### Phase M10 — Archive & portability

1. `bc workspace archive <name>` — stops agents, bundles
   `<ws>/.bc/state.db + cron.db + agent logs` into `~/.bc/archives/<name>-<date>.tar.gz`,
   removes from registry. User assets (templates/secrets/MCPs) untouched.
2. `bc workspace restore <file>` — unpacks, re-registers.
3. `bc export-user-state` — tars `~/.bc/templates/ + secrets.vault + mcps.json + tools.json + prefs.json` (without GitHub token by default).
4. `bc import-user-state <file>` — unpacks on a new machine.

**Acceptance:**
- Fresh mac → install bc → `bc import-user-state`-from-backup → every
  template/secret/MCP is there, no workspace data (correct: that's on the
  previous machine).
- Archive a workspace → delete the repo directory → restore → workspace data
  back, repo needs to be re-cloned separately (that's L3 code, bc doesn't own
  it).

## 20. Migration risks & mitigations

- **Concurrent secrets access across workspaces** — user vault needs a single
  file lock. Use a simple BoltDB or SQLite for `secrets.vault`, not a naive
  JSON file.
- **Template edits while a workspace is generating an agent** — either the
  workspace reads once at agent-create time (safe) or watches the file
  (requires debounce). Start with read-once.
- **Cost-store column addition** — `workspace_id` is a simple ADD COLUMN.
  Backfill from path prefix during migration. Old rows get NULL, report as
  "unattributed".
- **GitHub token sync** — it's a secret. Default `bc export-user-state` to
  exclude it unless `--include-auth` is passed.

## 21. What this means for the product

- "bc is the control plane for your AI agents." Your keys, templates, and
  tool trust are yours and follow you. Workspaces are places you do work.
- "Spin up a new workspace in a second." Because it's just a `state.db`, not
  a reseeding ritual.
- "See how you're spending across everything." Cost analytics go multi-project.
- "Experiment without fear." Archive the workspace, experiment is gone,
  learnings preserved via the shared user vault / templates.

## 22. Relationship to M1-M7

M1-M7 is the **plumbing**: bcd can dispatch to any workspace at request
time. That's necessary but not sufficient for the product.

M8-M10 is the **shape**: users store L2 things in their user dir, workspaces
become cheap, projects become just paths.

The two efforts compose cleanly. M8 can begin as soon as M7 lands.
