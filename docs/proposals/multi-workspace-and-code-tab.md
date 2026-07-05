# Proposal: Multi-Workspace Support, URL + Header Refactor, Code Tab, and Optional Dependencies

> **Status:** Superseded by RFC #3079 (workspace-as-property, shipped in v0.3.0) &nbsp;|&nbsp; **Original Author:** zen-zebra &nbsp;|&nbsp; **Date:** 2026-04-16
>
> **Historical note (v0.3.0, 2026-07-02):** the `/api/workspaces/{ws}/…` path-scoped API surface described in §5 and §9 of this document was **replaced** in v0.3.0 by a flat `/api/*` surface where workspace scope is expressed as an `X-BC-Workspace: <id>` header or a `?workspace=<id>` query parameter. Only registry self-routes remain under `/api/workspaces/…`. See RFC #3079 and PRs #3147, #3148, #3149, #3150. The rest of this document is kept for historical context; do not implement it as written.
>
> **Extends:** [`docs/proposals/agents-revamp.md`](./agents-revamp.md) (v2)
>
> **Related issues / PRs:**
> - Issue #2979 — Agents Revamp v2 (parent)
> - Issue #2999 — Agents revamp verification checklist
> - Issue #3012 — Multi-workspace support (this proposal)
> - Issue #3013 — URL + header refactor (this proposal)
> - Issue #3014 — Code tab (this proposal)
> - Issue #3015 — Optional dependencies manager (this proposal)
> - PR #2946 — Channels Revamp (context)

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals and Non-Goals](#2-goals-and-non-goals)
3. [High-Level Architecture](#3-high-level-architecture)
4. [Detailed Design — Multi-Workspace Support](#4-detailed-design--multi-workspace-support)
5. [Detailed Design — URL + Header Refactor](#5-detailed-design--url--header-refactor)
6. [Detailed Design — Code Tab](#6-detailed-design--code-tab)
7. [Detailed Design — Optional Dependencies Manager](#7-detailed-design--optional-dependencies-manager)
8. [Detailed Design — Agent Page Tab Reorder](#8-detailed-design--agent-page-tab-reorder)
9. [Migration Strategy](#9-migration-strategy)
10. [Security Considerations](#10-security-considerations)
11. [Phased Delivery Plan](#11-phased-delivery-plan)
12. [Testing Strategy + Verification Checklist](#12-testing-strategy--verification-checklist)
13. [Alignment with agents-revamp.md](#13-alignment-with-agents-revampmd)

---

## 1. Problem Statement

Today, a running `bcd` instance is bound to a single workspace (one project
folder with a `.bc/` subdir). Users who operate across multiple projects are
forced to:

- Run one `bcd` per workspace, each on its own port
- Switch browser tabs or shells to change context
- Manually juggle `MYCEL_WORKSPACE` / `cd` into the right folder

This friction compounds with every new project. It also blocks several features
that naturally cross the workspace boundary, such as:

- A global cost/metrics dashboard across all user projects
- Cross-workspace agent comparisons
- A single login / OAuth handshake shared by all projects
- Hosting `bcd` as a team service rather than a personal daemon

In parallel, the web UI has accumulated inconsistencies:

- Each page invents its own header layout
- Routes are flat (`/live`, `/agents/:name`) with no namespacing; adding a
  workspace prefix would be a large refactor
- The `Workspace` nav item duplicates what a proper settings page should contain
- Users cannot inspect or edit files in the UI; they must drop to a shell

Finally, the workspace accumulates **optional runtime dependencies**
(TimescaleDB, Playwright browser, code-server) that are started and managed
ad-hoc via Makefile targets with no UI surface. Users discover them by reading
docs rather than through the product.

This proposal unifies these four threads into a coherent redesign of the bcd
server, HTTP API, and web UI:

1. **Multi-workspace** — one bcd serves N workspaces simultaneously
2. **URL + header refactor** — `/w/<wsId>/…` routing with a shared HUD header
3. **Code tab** — VS Code-like file tree + Monaco viewer with diff mode
4. **Optional dependencies** — typed manager + settings UI for bc-db, code-server, etc.
5. **Agent page tab reorder** — Attach becomes the default, Code tab is added

This document extends the existing [Agents Revamp v2](./agents-revamp.md)
proposal. Where this document conflicts with agents-revamp.md, this one wins.

---

## 2. Goals and Non-Goals

### 2.1 Goals

- **G1** One `bcd` instance serves an unbounded number of workspaces with
  fully-isolated state (tmux/Docker sessions, worktrees, channels, events,
  stats, costs).
- **G2** Workspaces are registered in a global registry at `~/.bc/registry.json`
  and discoverable via local filesystem scan, GitHub repo list, or manual add.
- **G3** Every web UI route is namespaced under `/w/<wsId>/…`; the active
  workspace is a first-class URL concept, not a hidden cookie.
- **G4** A single shared `Header` component provides a consistent HUD across
  every tab, with slotted left/center/right regions.
- **G5** A new `Code` tab (top-level and per-agent) provides a VS Code-like
  experience with read-only Monaco by default, optional code-server iframe,
  and diff mode for agent worktrees.
- **G6** Optional runtime dependencies are managed through a typed
  `pkg/deps/` package with a Settings UI section (toggle, status, logs).
- **G7** Agent detail tabs reorder to Attach / Live / Config / Metrics / Code,
  with Attach as the default, and Code scoped to the agent's worktree.
- **G8** Full backwards compatibility for existing installs via URL redirects
  and a legacy-routes shim that scopes to the first / last-used workspace.

### 2.2 Non-Goals

- **NG1** Multi-tenant identity (multiple users per bcd). Auth remains
  single-user; a future proposal may revisit.
- **NG2** Remote-hosted workspaces (serving bcd from a cluster with
  per-workspace pods). The design leaves room for it but does not implement it.
- **NG3** Replacing agents-revamp.md. Templates, avatars, and the live-events
  pipeline are still authoritative there.
- **NG4** A full IDE in-browser. The Code tab is a viewer + optional
  code-server iframe, not a rebuild of VS Code.
- **NG5** A plugin marketplace for dependencies. `pkg/deps/` is a closed
  typed list; adding a dependency means a code change.
- **NG6** Built-in browser automation. Claude Code already ships a browser
  plugin — we document `bc-browser` but leave it disabled.

---

## 3. High-Level Architecture

```
                        ┌──────────────────────────────────────────┐
                        │                 bcd (:9374)              │
                        │                                          │
  Browser (web UI)      │   ┌─────────────────────────────────┐    │
  /w/<wsId>/*       ────┼──▶│ Router                          │    │
                        │   │  - /api/workspaces              │    │
                        │   │  - /api/workspaces/{ws}/…       │    │
                        │   │  - /api/deps                    │    │
                        │   │  - /api/auth/github             │    │
                        │   │  - legacy: /api/* (→ active ws) │    │
                        │   └──────────────┬──────────────────┘    │
                        │                  │                       │
                        │   ┌──────────────▼──────────────────┐    │
                        │   │ WorkspaceRegistry (singleton)   │    │
                        │   │  ~/.bc/registry.json            │    │
                        │   │  aliases, last_used_at, github  │    │
                        │   └──────────────┬──────────────────┘    │
                        │                  │                       │
                        │                  │ LoadServices(wsId)    │
                        │                  ▼                       │
                        │   ┌──────────────────────────────────────┴────┐
                        │   │  Services (per-workspace, lazy-loaded)    │
                        │   │  id: abc123   name: monorepo              │
                        │   │  path: ~/Projects/monorepo                │
                        │   │   ├── AgentManager (tmux/docker)          │
                        │   │   ├── ChannelManager (SQLite scoped)      │
                        │   │   ├── EventStore (SQLite scoped)          │
                        │   │   ├── StatsStore (Timescale scoped)       │
                        │   │   ├── WorktreeManager                     │
                        │   │   ├── FileBrowser (constrained)           │
                        │   │   └── DepsController                      │
                        │   └───────────────────────────────────────────┘
                        │                                               │
                        │   ┌───────────────────────────────────────────┐
                        │   │ DepsManager (global singleton)            │
                        │   │   ├── bc-db        (TimescaleDB)          │
                        │   │   ├── bc-code-server (optional, per-ws)   │
                        │   │   └── bc-browser  (documented, disabled)  │
                        │   └───────────────────────────────────────────┘
                        └────────────────────────────────────────────────┘

    Host FS                              Containers / tmux
    ~/.bc/                               bc-<wsHash>-<agent>   (tmux session)
      registry.json                      bc-db                 (shared)
      github-token                       bc-code-server-<wsHash> (per ws, opt)
      workspaces.json (legacy alias)
    ~/Projects/monorepo/.bc/
      agents/<name>/worktree/
      channels.db, events.db, stats.db
    ~/Projects/site/.bc/
      ...
```

The key insight: **workspaces are discovered and addressed globally, but all
per-workspace state still lives inside each workspace's own `.bc/` directory.**
`~/.bc/` contains only the registry and cross-workspace user data
(GitHub token, theme, alias map). This preserves `bc`'s current "workspace is
portable" property — you can still `tar czf` a project folder and move it.

---

## 4. Detailed Design — Multi-Workspace Support

### 4.1 Registry (extends existing `pkg/workspace/registry.go`)

#### 4.1.1 On-disk format: `~/.bc/registry.json`

```json
{
  "version": 2,
  "default_workspace": "abc123",
  "workspaces": [
    {
      "id": "abc123",
      "name": "monorepo",
      "path": "/Users/puneet/Projects/monorepo",
      "github_url": "https://github.com/rpuneet/monorepo",
      "github_full_name": "rpuneet/monorepo",
      "last_used_at": "2026-04-15T11:03:22Z",
      "created_at": "2026-03-01T08:10:00Z"
    },
    {
      "id": "d41d8c",
      "name": "site",
      "path": "/Users/puneet/Projects/site",
      "github_url": null,
      "github_full_name": null,
      "last_used_at": "2026-04-14T17:45:12Z",
      "created_at": "2026-04-09T09:00:00Z"
    }
  ],
  "aliases": {
    "mono": "abc123",
    "blog": "d41d8c"
  }
}
```

- `id` is a stable 6–12 char hex derived from `sha256(abs_path)[:6]`. It is
  used in URLs (`/w/abc123/…`), container/tmux names (`bc-abc123-<agent>`),
  and internal maps.
- `name` is user-visible (defaults to `basename(path)`, editable).
- `aliases` supports the existing `bc -w mono agents ls` short-form.
- `default_workspace` is the redirect target when the user visits `/` with no
  session hint.

The existing `pkg/workspace/registry.go` already supports aliases and a
`workspaces.json` file at `~/.bc/workspaces.json`. This proposal renames the
file to `registry.json` (with a read-compat shim for the old name for one
minor version), adds the `id`, `github_*`, and timestamp fields, and bumps
`version` to `2`.

#### 4.1.2 Go API

```go
// pkg/workspace/registry.go (additions)

type Workspace struct {
    ID             string    `json:"id"`
    Name           string    `json:"name"`
    Path           string    `json:"path"`
    GithubURL      string    `json:"github_url,omitempty"`
    GithubFullName string    `json:"github_full_name,omitempty"`
    LastUsedAt     time.Time `json:"last_used_at"`
    CreatedAt      time.Time `json:"created_at"`
}

type Registry struct {
    Version          int                  `json:"version"`
    DefaultWorkspace string               `json:"default_workspace"`
    Workspaces       []Workspace          `json:"workspaces"`
    Aliases          map[string]string    `json:"aliases"`
}

func LoadRegistry() (*Registry, error)
func (r *Registry) Save() error
func (r *Registry) Add(path string) (*Workspace, error)   // dedups by path
func (r *Registry) Remove(id string) error                // leaves .bc/ intact
func (r *Registry) Get(idOrAlias string) (*Workspace, error)
func (r *Registry) Touch(id string) error                 // updates last_used_at
func (r *Registry) Resolve(urlSegment string) (*Workspace, error)  // id|alias|name
```

`Registry` is protected by a `sync.RWMutex`; writes fsync the file.

### 4.2 Per-workspace `Services`

```go
// server/services.go (new)

type Services struct {
    Workspace *workspace.Workspace
    Agents    *agent.Manager
    Channels  *channel.Manager
    Events    *events.Store
    Stats     *stats.Store
    Costs     *cost.Store
    Worktrees *worktree.Manager
    Files     *files.Browser    // new
    Deps      *deps.Controller  // new
}

type ServicesRegistry struct {
    mu       sync.RWMutex
    byID     map[string]*Services
    registry *workspace.Registry
    // ...
}

func (sr *ServicesRegistry) Get(ctx context.Context, id string) (*Services, error) {
    sr.mu.RLock()
    if s, ok := sr.byID[id]; ok {
        sr.mu.RUnlock()
        return s, nil
    }
    sr.mu.RUnlock()
    return sr.loadLocked(ctx, id)
}
```

Services are **lazy-loaded** on first access, cached in the map, and evicted
on `DELETE /api/workspaces/{ws}`. A background goroutine evicts Services whose
`Workspace.LastAccess` is older than 30 minutes to cap memory.

### 4.3 Isolation rules

| Resource | Isolation scheme |
|----------|------------------|
| tmux sessions | `bc-<wsID>-<agent>` (e.g., `bc-abc123-clever-urial`) |
| Docker containers | `bc-<wsID>-<agent>`, volumes `bc-<wsID>-<agent>-home` |
| Worktrees | `<ws.Path>/.bc/agents/<agent>/worktree/` |
| Channels DB | `<ws.Path>/.bc/channels.db` |
| Events DB | `<ws.Path>/.bc/events.db` |
| Stats / cost | TimescaleDB tables keyed by `workspace_id` column |
| Cron jobs | Stored in workspace DB, keyed by `workspace_id` |
| File browser root | `<ws.Path>/` (path-constrained) |

The existing tmux naming uses `bc-<workspace-basename>-<agent>`, which collides
when two workspaces share a basename. The new scheme uses the registry `id`,
which is a hash of the absolute path and therefore collision-free.

### 4.4 Discovery

#### 4.4.1 Local scan

```
POST /api/discovery/local/scan
Body: { "root": "/Users/puneet/Projects", "depth": 3 }
→ 200 { "candidates": [
    { "path": "/Users/puneet/Projects/monorepo", "has_bc": true,
      "git_remote": "git@github.com:rpuneet/monorepo.git",
      "already_registered": true, "id": "abc123" },
    { "path": "/Users/puneet/Projects/site", "has_bc": false,
      "git_remote": "", "already_registered": false },
    ...
  ] }
```

Implementation: `filepath.WalkDir` capped at `depth`, skip `node_modules`,
`.git`, `dist`, `build`. Emits any folder containing `.git`.

#### 4.4.2 GitHub repo list

```
POST /api/auth/github/start          → 302 to github.com/login/oauth
GET  /api/auth/github/callback       ← stores token at ~/.bc/github-token (0600)
GET  /api/discovery/github           → { "repos": [{full_name, clone_url, ssh_url, private, default_branch}...] }
POST /api/discovery/github/clone     Body: { "full_name": "rpuneet/monorepo", "target": "/Users/puneet/Projects" }
                                     → 200 { "id": "abc123", "path": "..." }
```

The token file uses the existing format; we add the file at `~/.bc/github-token`
with `0600` perms. OAuth scope: `repo` (read) + `read:user`.

#### 4.4.3 Manual add

```
POST /api/workspaces
Body: { "path": "/Users/puneet/Projects/site", "name": "site" }
→ 201 { "id": "d41d8c", "name": "site", ... }
```

If `<path>/.bc/` does not exist, server returns `409 Conflict` with
`{ "code": "not_initialized", "hint": "run `bc init`" }`. The frontend offers
an "Initialize" button that calls `POST /api/workspaces/{id}/init`.

### 4.5 Workspace HTTP API

All new routes live under `/api/workspaces/…`. Legacy `/api/…` routes are
preserved for one major version via a shim that scopes to the active workspace
(see §9 Migration Strategy).

```
GET    /api/workspaces                         → list { workspaces, default, active }
POST   /api/workspaces                         → add existing path
GET    /api/workspaces/{ws}                    → detail + health
PATCH  /api/workspaces/{ws}                    → rename, set github_url, etc.
DELETE /api/workspaces/{ws}                    → unregister (keeps .bc/ on disk)
POST   /api/workspaces/{ws}/init               → run bc init on an uninitialised path
POST   /api/workspaces/{ws}/activate           → sets active ws cookie/header

GET    /api/workspaces/{ws}/agents             → scoped existing handlers
POST   /api/workspaces/{ws}/agents             ...
GET    /api/workspaces/{ws}/channels
GET    /api/workspaces/{ws}/events
GET    /api/workspaces/{ws}/stats
GET    /api/workspaces/{ws}/costs
GET    /api/workspaces/{ws}/worktrees
GET    /api/workspaces/{ws}/code/tree          → §6
GET    /api/workspaces/{ws}/code/file          → §6
GET    /api/workspaces/{ws}/code/diff          → §6
GET    /api/workspaces/{ws}/code/search        → §6
GET    /api/workspaces/{ws}/deps               → §7
```

`{ws}` accepts `id`, `alias`, or a URL-encoded `name`. Middleware resolves it
via `Registry.Resolve()` and injects a `*Services` into the request context
under the key `ctxKeyServices`.

### 4.6 CLI changes

`bc` already supports `bc -w <alias> agents ls`. We add:

```
bc workspace list
bc workspace add <path> [--name <n>]
bc workspace remove <id|alias>
bc workspace rename <id|alias> <new-name>
bc workspace default <id|alias>
bc workspace github login       # opens browser for OAuth
bc workspace github list        # prints repos
bc workspace github clone <full_name> [--target <dir>]
```

All of these delegate to the bcd HTTP API when bcd is running, and to
`pkg/workspace/registry.go` directly when it is not.

---

## 5. Detailed Design — URL + Header Refactor

### 5.1 URL scheme

**Before (current):**

```
/                       → Live (default)
/live                   → Live events
/agents                 → Agents list
/agents/:name           → Agent detail
/channels               → Channels
/channels/:name         → Channel
/workspace              → Workspace settings
/metrics                → Metrics
/tools                  → Tools
```

**After (this proposal):**

```
/                                    → redirects to /w/<default_id>/live
/w                                   → workspace picker
/w/:wsId                             → redirects to /w/:wsId/live
/w/:wsId/live                        → Live events
/w/:wsId/agents                      → Agents list
/w/:wsId/agents/:name                → Agent detail (default tab = attach, see §8)
/w/:wsId/agents/:name/live
/w/:wsId/agents/:name/config
/w/:wsId/agents/:name/metrics
/w/:wsId/agents/:name/code
/w/:wsId/channels
/w/:wsId/channels/:name
/w/:wsId/code                        → top-level code tab (§6)
/w/:wsId/metrics
/w/:wsId/tools
/w/:wsId/settings                    → was /workspace
/w/:wsId/settings/deps               → Dependencies section (§7)
/w/:wsId/settings/general
/w/:wsId/settings/roles
/w/:wsId/settings/secrets
/settings                            → global settings (theme, account)
```

### 5.2 React Router refactor

File: `web/src/App.tsx`

```tsx
// Before (flat):
<Routes>
  <Route path="/" element={<Live />} />
  <Route path="/agents" element={<Agents />} />
  <Route path="/agents/:name" element={<AgentDetail />} />
  ...
</Routes>

// After:
<Routes>
  <Route path="/" element={<Navigate to={`/w/${defaultWs}/live`} replace />} />
  <Route path="/w" element={<WorkspacePicker />} />
  <Route path="/w/:wsId" element={<WorkspaceShell />}>
    <Route index element={<Navigate to="live" replace />} />
    <Route path="live" element={<Live />} />
    <Route path="agents" element={<Agents />} />
    <Route path="agents/:name/*" element={<AgentDetail />} />
    <Route path="channels" element={<Channels />} />
    <Route path="channels/:name" element={<ChannelDetail />} />
    <Route path="code/*" element={<CodeTab scope="workspace" />} />
    <Route path="metrics" element={<Metrics />} />
    <Route path="tools" element={<Tools />} />
    <Route path="settings/*" element={<Settings />} />
  </Route>
  <Route path="/settings/*" element={<GlobalSettings />} />
</Routes>
```

`WorkspaceShell` reads `:wsId` from params, fetches the workspace metadata via
`useWorkspace(wsId)`, and provides it via a `WorkspaceContext` to all children.
It renders the sidebar, the shared `Header`, and an `<Outlet />`.

### 5.3 Shared `Header.tsx`

File: `web/src/components/Header.tsx` (new)

```tsx
type HeaderProps = {
  /** Center region: title / breadcrumb / tabs. */
  center?: React.ReactNode;
  /** Right region: primary CTA, filter pills, dropdowns, etc. */
  actions?: React.ReactNode;
  /** Override workspace switcher (e.g., hide on the workspace picker page). */
  hideWorkspaceSwitcher?: boolean;
};

export function Header({ center, actions, hideWorkspaceSwitcher }: HeaderProps) {
  const { collapsed, toggle } = useSidebar();
  const { workspace, switchTo, list } = useWorkspace();

  return (
    <header className="bc-header">
      <div className="bc-header__left">
        <IconButton icon={collapsed ? "chevron-right" : "chevron-left"} onClick={toggle} />
        {!hideWorkspaceSwitcher && (
          <WorkspaceDropdown value={workspace} options={list} onChange={switchTo} />
        )}
      </div>
      <div className="bc-header__center">{center}</div>
      <div className="bc-header__right">{actions}</div>
    </header>
  );
}
```

Styling: monospace HUD aesthetic matching the existing Live page. Tokens go in
`web/src/theme/header.css`. Each page sets its `center` and `actions` via:

```tsx
// e.g., web/src/pages/Live.tsx
return (
  <>
    <Header
      center={<span className="bc-header__title">LIVE EVENTS</span>}
      actions={
        <>
          <FilterPills filters={filters} onChange={setFilters} />
          <Button icon="download" onClick={exportCSV}>EXPORT</Button>
        </>
      }
    />
    <main>…events list…</main>
  </>
);
```

### 5.4 Layout changes

File: `web/src/components/Layout.tsx`

- Remove the `Workspace` nav item; its content moves under `Settings`.
- Add `Code` as a top-level nav item between `Tools` and `Channels`.
- New order: `Live`, `Agents`, `Code`, `Channels`, `Tools`, `Metrics`, `Settings`.
- Sidebar collapse state persists in `localStorage` under `bc.sidebar.collapsed`.
- Hide sidebar entirely on the workspace picker (`/w`).

### 5.5 `WorkspaceDropdown.tsx`

File: `web/src/components/WorkspaceDropdown.tsx` (new)

- Displays current workspace `name` with a `[id]` suffix.
- Dropdown shows all workspaces sorted by `last_used_at desc`.
- Search input at the top (fuzzy match over `name`, `alias`, `path`).
- Footer: "Add workspace…" button → opens `AddWorkspaceModal`.
- Keyboard shortcut: `⌘K` opens it (consistent with Live page filter).

### 5.6 `AddWorkspaceModal.tsx`

File: `web/src/components/AddWorkspaceModal.tsx` (new)

Three tabs:

1. **Scan local** — picks a root dir, hits `POST /api/discovery/local/scan`,
   shows results with checkboxes.
2. **From GitHub** — if no token, shows "Connect GitHub" → `/api/auth/github/start`.
   Otherwise lists repos from `/api/discovery/github`, each with a `Clone` button.
3. **Manual path** — text input + `Add` button → `POST /api/workspaces`.

---

## 6. Detailed Design — Code Tab

### 6.1 Scope

The Code tab provides a **read-only** file browsing experience by default, with
an optional **write-capable** mode that embeds `code-server` via iframe.

Two entry points:

- **Top-level Code tab** (`/w/:wsId/code`) — defaults to the workspace root
  repo (main worktree).
- **Per-agent Code tab** (`/w/:wsId/agents/:name/code`) — defaults to that
  agent's worktree, shown in diff mode vs the main worktree's HEAD.

### 6.2 Component hierarchy

```
<CodeTab scope="workspace" | "agent">
  <Header
    center={<BreadcrumbPath /> <ViewModeTabs />}
    actions={<WorktreeDropdown /> <SearchButton /> <CodeServerToggle />}
  />
  <CodeTabBody>
    <FileTree />                       (left, resizable pane, ~280px default)
    <CodeViewer>                       (right, fills remainder)
      <MonacoReadOnly />               if mode === "view"
      <MonacoDiffEditor />             if mode === "diff"
      <CodeServerIframe />             if mode === "edit" && deps.code-server running
    </CodeViewer>
  </CodeTabBody>
</CodeTab>
```

Files:

- `web/src/pages/CodeTab.tsx`
- `web/src/components/code/FileTree.tsx`
- `web/src/components/code/CodeViewer.tsx`
- `web/src/components/code/MonacoReadOnly.tsx`
- `web/src/components/code/MonacoDiffEditor.tsx`
- `web/src/components/code/CodeServerIframe.tsx`
- `web/src/components/code/WorktreeDropdown.tsx`
- `web/src/components/code/BreadcrumbPath.tsx`
- `web/src/hooks/useFileTree.ts`
- `web/src/hooks/useFileContent.ts`
- `web/src/hooks/useDiff.ts`

### 6.3 Worktree dropdown

Options:

- `main repo` — `<ws.Path>/` (read-only, no diff)
- `<agent-name> worktree` — `<ws.Path>/.bc/agents/<name>/worktree/`
  - Automatically renders in **diff mode** against `main` by default.
  - Can toggle to plain view.

The top-level Code tab defaults to `main repo`. The per-agent Code tab defaults
to that agent's worktree (diff mode).

### 6.4 API

#### 6.4.1 Tree

```
GET /api/workspaces/{ws}/code/tree?path=src&worktree=main&depth=1
→ 200 {
  "path": "src",
  "entries": [
    { "name": "App.tsx", "type": "file", "size": 4321, "mtime": "..." },
    { "name": "components", "type": "dir", "entries_count": 12 },
    ...
  ]
}
```

- `path` is relative to the worktree root; defaults to `""`.
- `worktree=main` or `worktree=<agent-name>`.
- `depth` default = 1 (lazy-loaded expansion).
- Excludes `.git/`, respects an opt-in allowlist of hidden dirs
  (`.bc/` is hidden by default, shown via `show_hidden=1`).

#### 6.4.2 File content

```
GET /api/workspaces/{ws}/code/file?path=src/App.tsx&worktree=main
→ 200 {
  "path": "src/App.tsx",
  "worktree": "main",
  "size": 4321,
  "mtime": "...",
  "content": "…",       // utf-8; omitted if binary or > 2MB
  "encoding": "utf-8",
  "language": "typescript",
  "truncated": false
}
```

Binary files return `{"binary": true, "size": ..., "download_url": "..."}` and
the UI shows a "download" link.

#### 6.4.3 Diff

```
GET /api/workspaces/{ws}/code/diff?worktree=clever-urial&path=src/App.tsx
→ 200 {
  "path": "src/App.tsx",
  "base": { "ref": "main-HEAD", "sha": "abc...", "content": "…" },
  "head": { "ref": "worktree", "sha": "wip",    "content": "…" },
  "stats": { "additions": 12, "deletions": 3 }
}

GET /api/workspaces/{ws}/code/diff?worktree=clever-urial
→ 200 {
  "base": { "ref": "main", "sha": "..." },
  "head": { "ref": "worktree" },
  "files": [
    { "path": "src/App.tsx", "additions": 12, "deletions": 3, "status": "modified" },
    { "path": "src/new.ts",  "additions": 42, "deletions": 0, "status": "added" },
    ...
  ]
}
```

Uses `git diff` under the hood, bounded by a 10-second timeout per request.

#### 6.4.4 Search

```
GET /api/workspaces/{ws}/code/search?q=TODO&worktree=main&limit=100
→ 200 { "matches": [ { "path":"...", "line":42, "text":"..." }, ... ] }
```

Shell out to `rg --json`, require `ripgrep` on host, fall back to a Go
implementation via `doublestar` + line-scan if unavailable.

### 6.5 Path constraint

All four endpoints resolve paths with:

```go
func safePath(root, rel string) (string, error) {
    abs := filepath.Join(root, rel)
    clean := filepath.Clean(abs)
    if !strings.HasPrefix(clean, filepath.Clean(root)+string(filepath.Separator)) &&
       clean != filepath.Clean(root) {
        return "", ErrPathEscape
    }
    // also reject symlinks pointing outside root
    resolved, err := filepath.EvalSymlinks(clean)
    if err == nil && !strings.HasPrefix(resolved, ...) {
        return "", ErrPathEscape
    }
    return clean, nil
}
```

A shared `pkg/files/safepath.go` houses this helper. All code tab handlers must
use it. Unit tests cover `..` segments, absolute paths, symlinks, and NUL bytes.

### 6.6 Monaco integration

Library: `@monaco-editor/react` (peer dep: `monaco-editor`).

- `MonacoReadOnly` sets `options.readOnly = true`.
- `MonacoDiffEditor` uses Monaco's built-in `DiffEditor` with `renderSideBySide`
  configurable via a toggle (default: side-by-side on ≥1280px, inline below).
- Language inferred from file extension via a small `lang.ts` map.
- No external CDN; Monaco is bundled via Vite.

### 6.7 code-server mode (opt-in)

If the `bc-code-server` dependency is running (§7), the Code tab shows an
"Edit in VS Code" toggle that swaps the `CodeViewer` for an iframe to
`http://localhost:<port>/?folder=<ws.Path>`. The iframe URL is produced by
`pkg/deps/code_server.go`. A warning banner explains that code-server has full
write access to the workspace. The toggle only appears when
`status === "running"`.

---

## 7. Detailed Design — Optional Dependencies Manager

### 7.1 `pkg/deps/`

```go
// pkg/deps/deps.go

type Status string

const (
    StatusUnknown  Status = "unknown"
    StatusStopped  Status = "stopped"
    StatusStarting Status = "starting"
    StatusRunning  Status = "running"
    StatusError    Status = "error"
)

type Info struct {
    Name         string            `json:"name"`
    Description  string            `json:"description"`
    Status       Status            `json:"status"`
    Enabled      bool              `json:"enabled"`
    Endpoint     string            `json:"endpoint,omitempty"`
    Version      string            `json:"version,omitempty"`
    LastError    string            `json:"last_error,omitempty"`
    Metadata     map[string]string `json:"metadata,omitempty"`
}

type Dependency interface {
    Name() string
    Info(ctx context.Context) (Info, error)
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Logs(ctx context.Context, follow bool, tail int) (io.ReadCloser, error)
}

type Controller struct {
    mu    sync.RWMutex
    deps  map[string]Dependency
    // ...
}

func (c *Controller) Register(d Dependency)
func (c *Controller) List(ctx context.Context) ([]Info, error)
func (c *Controller) Get(name string) (Dependency, error)
```

### 7.2 Implementations

#### 7.2.1 `pkg/deps/db.go` — `bc-db` (TimescaleDB)

- Wraps the existing `docker/bc-db` image.
- Singleton: one per host (not per-workspace); all workspaces share the same
  TimescaleDB instance with `workspace_id` column scoping.
- `Start()` runs `docker run -d --name bc-db -p 5432:5432 …` with a named
  volume `bc-db-data`.
- `Stop()` stops but does not remove the container.
- `Logs()` proxies `docker logs -f bc-db`.

#### 7.2.2 `pkg/deps/code_server.go` — `bc-code-server` (new)

- Per-workspace: `bc-code-server-<wsID>` container.
- Image: `codercom/code-server:latest` (documented; pull on demand).
- Port: allocated dynamically in the 8200–8299 range; stored in `Info.Endpoint`.
- Volume mount: `-v <ws.Path>:/home/coder/workspace`.
- `Start(ctx)` takes a `workspaceID` — the controller passes it through.
- The UI is embedded via iframe in the Code tab (§6.7).

#### 7.2.3 `pkg/deps/browser.go` — `bc-browser` (documented, disabled)

- Stub implementation that always returns `StatusStopped` and refuses to
  `Start()` with `ErrDisabled("built-in Claude Code browser plugin supersedes this")`.
- Kept in the registry for discoverability; listed in the Settings UI as
  "Deprecated — use Claude Code browser plugin".

### 7.3 HTTP API

```
GET  /api/deps                            → [] Info
GET  /api/deps/{name}                     → Info
POST /api/deps/{name}/start               → 202 then poll /status
POST /api/deps/{name}/stop                → 202
GET  /api/deps/{name}/status              → Info
GET  /api/deps/{name}/logs?tail=200&follow=1   → text/event-stream
```

Per-workspace dependencies (code-server) use:

```
POST /api/workspaces/{ws}/deps/bc-code-server/start
GET  /api/workspaces/{ws}/deps/bc-code-server/status
```

### 7.4 Settings UI

File: `web/src/pages/Settings/Dependencies.tsx`

Renders a card per dep:

```
┌────────────────────────────────────────────────┐
│ bc-db · TimescaleDB                            │
│ Status: ● Running    Endpoint: localhost:5432  │
│ [Stop] [View logs]                             │
│ Used by: metrics, costs, stats                 │
└────────────────────────────────────────────────┘
┌────────────────────────────────────────────────┐
│ bc-code-server · VS Code in the browser        │
│ Status: ○ Stopped                              │
│ [Start] [Configure]                            │
│ Used by: Code tab (Edit mode)                  │
│ Workspace: monorepo                            │
└────────────────────────────────────────────────┘
┌────────────────────────────────────────────────┐
│ bc-browser · Playwright browser           DEPR │
│ Superseded by Claude Code browser plugin.      │
│ [Learn more]                                   │
└────────────────────────────────────────────────┘
```

Clicking `View logs` opens a modal with a `<pre>` that streams from the SSE
endpoint, with pause/resume and a `Copy all` button.

---

## 8. Detailed Design — Agent Page Tab Reorder

### 8.1 New tab order

| # | Tab | URL suffix | Notes |
|---|-----|-----------|-------|
| 1 | Attach | `/agents/:name` (default) or `/agents/:name/attach` | tmux terminal, default tab |
| 2 | Live | `/agents/:name/live` | live events for this agent |
| 3 | Config | `/agents/:name/config` | system prompt, MCPs, secrets |
| 4 | Metrics | `/agents/:name/metrics` | graphs + timeframes |
| 5 | Code | `/agents/:name/code` | **new** — agent's worktree in diff mode |

`Attach` being the default is a behavior change from agents-revamp.md, which
opens `Live` by default. Rationale: the most common operator action is to look
at or interact with the tmux session; `Live` is read-only.

### 8.2 Implementation

File: `web/src/pages/AgentDetail.tsx`

```tsx
const tabs = [
  { key: "attach",  label: "Attach",  path: "attach"  },
  { key: "live",    label: "Live",    path: "live"    },
  { key: "config",  label: "Config",  path: "config"  },
  { key: "metrics", label: "Metrics", path: "metrics" },
  { key: "code",    label: "Code",    path: "code"    },
];

<Routes>
  <Route index element={<Navigate to="attach" replace />} />
  <Route path="attach"  element={<AttachTab  agent={name} />} />
  <Route path="live"    element={<LiveTab    agent={name} />} />
  <Route path="config"  element={<ConfigTab  agent={name} />} />
  <Route path="metrics" element={<MetricsTab agent={name} />} />
  <Route path="code"    element={<CodeTab    scope="agent" agent={name} />} />
</Routes>
```

The agent Code tab reuses the same components as the top-level Code tab
(`FileTree`, `CodeViewer`, etc.), pre-setting the `WorktreeDropdown` to the
agent's worktree and mode to `diff`.

---

## 9. Migration Strategy

### 9.1 Existing installs

- On first bcd boot after upgrade:
  1. If `~/.bc/workspaces.json` exists, migrate it to `~/.bc/registry.json`
     (keep the old file as `.legacy` for one minor version).
  2. If the current working directory is a workspace and not in the registry,
     auto-register it and set it as `default_workspace`.
  3. Print a one-time migration log line: `migrated N workspaces to registry`.

- If bcd was started without any registered workspace:
  - Web UI redirects `/` → `/w` (picker) with an empty state + "Add workspace"
    button.

### 9.2 URL redirects

A middleware runs before the React app:

```
GET /live                → 301 /w/<active>/live
GET /agents              → 301 /w/<active>/agents
GET /agents/<name>       → 301 /w/<active>/agents/<name>
GET /channels            → 301 /w/<active>/channels
GET /channels/<name>     → 301 /w/<active>/channels/<name>
GET /metrics             → 301 /w/<active>/metrics
GET /tools               → 301 /w/<active>/tools
GET /workspace           → 301 /w/<active>/settings
```

`<active>` resolution order:
1. `X-BC-Workspace` request header (used by CLI / tests)
2. `bc_active_ws` cookie
3. Registry's `default_workspace`
4. First workspace in registry sorted by `last_used_at desc`

### 9.3 Legacy API shim

For one major version, every `/api/<rest>` route (not under `/api/workspaces/`)
is delegated to `/api/workspaces/<active>/<rest>`. The shim lives in
`server/middleware/legacy_scope.go`:

```go
func LegacyScope(next http.Handler, sr *ServicesRegistry) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/api/workspaces/") ||
           !strings.HasPrefix(r.URL.Path, "/api/") {
            next.ServeHTTP(w, r); return
        }
        ws := resolveActive(r)
        if ws == "" { http.Error(w, "no active workspace", 400); return }
        r2 := r.Clone(r.Context())
        r2.URL.Path = "/api/workspaces/" + ws + strings.TrimPrefix(r.URL.Path, "/api")
        w.Header().Set("Deprecation", "true")
        w.Header().Set("Sunset", "Sun, 01 Nov 2026 00:00:00 GMT")
        next.ServeHTTP(w, r2)
    }
}
```

Responses include `Deprecation: true` + `Sunset:` headers so clients can log
warnings. The CLI is updated to use the new routes immediately; the shim is
mainly for third-party scripts and in-flight websocket connections.

### 9.4 tmux / Docker naming migration

Existing sessions named `bc-<basename>-<agent>` are migrated on first boot:

1. For each session matching `bc-<anything-not-6-hex>-*`, look up the matching
   workspace by basename.
2. If found and only one match, rename to `bc-<wsID>-<agent>` via
   `tmux rename-session` (Docker: rename the container if stopped; otherwise
   record the new name in state and rename on next restart).
3. If ambiguous, log a warning and skip — operator must manually reattach.

### 9.5 MCP endpoint

The MCP SSE endpoint moves from `/_mcp/<agent>/sse` to
`/_mcp/<wsID>/<agent>/sse`. The old path is preserved with the legacy shim
resolving `<active>`. All generated `.mcp.json` files are updated on next
workspace boot.

---

## 10. Security Considerations

### 10.1 Path traversal

All Code tab handlers (`tree`, `file`, `diff`, `search`) route through
`pkg/files.SafePath(root, rel)`. Rejected inputs:

- Any `rel` containing `..` after clean
- Absolute paths (must be relative)
- Symlinks that resolve outside `root`
- NUL bytes
- Paths exceeding `MAX_PATH` (4096 bytes)

Unit tests in `pkg/files/safepath_test.go` cover every case with
`fstest.MapFS` fixtures.

### 10.2 Workspace isolation

- Per-workspace `Services` cannot be accessed by another workspace's request:
  the middleware injects a specific `*Services` into the context and handlers
  read only from that.
- Cross-workspace APIs (`/api/workspaces`, `/api/deps`) are guarded by an
  explicit allowlist in the router.

### 10.3 GitHub OAuth token

- Stored at `~/.bc/github-token` with mode `0600`, written via `os.OpenFile`
  with `O_WRONLY|O_CREATE|O_TRUNC`.
- Never logged. Never returned in API responses (`/api/auth/github/status`
  returns only `connected: true|false` and the authenticated username).
- Revocation: `DELETE /api/auth/github` removes the file.
- Scope: `repo` + `read:user`. No write scopes.

### 10.4 code-server iframe

- iframe `sandbox="allow-scripts allow-same-origin allow-forms allow-downloads"`.
- Communication via `postMessage` restricted to the code-server origin.
- CSP on bcd: `frame-src 'self' http://localhost:8200-8299`.
- User must explicitly `Start` the dep; a warning banner reminds that it has
  full write access to the workspace.

### 10.5 Dependencies manager

- `Start` and `Stop` endpoints require a local-only check (bcd refuses on
  non-loopback origins unless `MYCEL_REMOTE=1` is set).
- Docker socket access is gated; a missing Docker returns `503` with a clear
  message instead of crashing.

### 10.6 Legacy shim

- The `X-BC-Workspace` header takes precedence over cookies. This prevents
  CSRF attacks from switching workspaces silently via attacker-controlled
  cookies. When no header is present, the cookie is used only on GET.
- The shim writes a single audit log line per request with the resolved
  `wsID`.

---

## 11. Phased Delivery Plan

Eight verticals (A–H). Each vertical produces a mergeable PR with its own
tests and docs. Order below is the recommended merge order; some can parallelise.

### Phase A — Workspace registry (week 1)

- `pkg/workspace/registry.go` extends to v2 schema (id, github_*, timestamps)
- `~/.bc/workspaces.json` → `~/.bc/registry.json` migration
- `server/handlers/workspaces.go` with CRUD + activate endpoints
- `bc workspace list/add/remove/rename/default` CLI commands
- Migration tests + unit tests for `Registry.Resolve`
- No UI changes yet

### Phase B — Discovery + GitHub OAuth (week 1–2)

- `pkg/discovery/local.go` (filesystem scan)
- `pkg/discovery/github.go` (OAuth + repo list + clone)
- `server/handlers/discovery.go`, `server/handlers/auth_github.go`
- Token storage at `~/.bc/github-token` (0600)
- Tests with `httptest` GitHub mock

### Phase C — URL + header refactor (week 2–3)

- `web/src/components/Header.tsx` (new)
- `web/src/components/WorkspaceDropdown.tsx` (new)
- `web/src/components/AddWorkspaceModal.tsx` (new)
- Refactor `App.tsx` routes to `/w/:wsId/*`
- `WorkspaceShell.tsx` with context
- Remove `Workspace` nav item; fold into `Settings`
- Legacy redirects in `server/middleware/legacy_scope.go`
- Legacy response shim with `Deprecation` headers
- Playwright e2e: legacy URLs redirect correctly

### Phase D — Code tab (week 3–4)

- `pkg/files/safepath.go` (with thorough tests)
- `pkg/files/browser.go` (tree/file/diff readers)
- `server/handlers/code.go` (four endpoints)
- `web/src/pages/CodeTab.tsx` + subcomponents
- Monaco integration via `@monaco-editor/react`
- `web/src/components/code/*` subtree
- Worktree dropdown + diff mode
- Top-level `/w/:wsId/code` route

### Phase E — Optional dependencies manager (week 4–5)

- `pkg/deps/deps.go` interface + Controller
- `pkg/deps/db.go` (bc-db)
- `pkg/deps/code_server.go` (bc-code-server)
- `pkg/deps/browser.go` (bc-browser stub)
- `server/handlers/deps.go`
- `web/src/pages/Settings/Dependencies.tsx` + modal for logs
- Docs for adding a new dependency

### Phase F — Agent detail tab reorder + Code tab inside agent (week 5)

- Update `web/src/pages/AgentDetail.tsx` tab order + default = `attach`
- Add the Code tab inside agent detail, wired to the agent's worktree in
  diff mode
- Update sidebar nav order
- Update playwright e2e fixtures

### Phase G — Documentation (week 5–6)

- README updates
- `docs/how-to/multi-workspace.md`
- `docs/how-to/code-tab.md`
- `docs/how-to/dependencies.md`
- `docs/reference/api.md` regenerated
- Blog post / changelog entry

### Phase H — Verification + polish (week 6)

- Run the verification checklist (§12) end-to-end on macOS + Linux
- Load test: 10 concurrent workspaces, 20 agents each
- Accessibility pass on the new Header + Code tab
- Final review with user (Puneet)
- Merge to `main`

---

## 12. Testing Strategy + Verification Checklist

### 12.1 Testing strategy

- **Unit tests**: per-package in `pkg/…`, using `testify/require`, table-driven
  where applicable.
- **Integration tests**: `server/` handlers with `httptest`, real SQLite,
  mocked Docker client (`moby/moby/client` interface).
- **E2E tests**: Playwright under `web/e2e/` covering registration,
  workspace switching, code tab navigation, diff rendering, deps start/stop.
- **Load test**: bash + `hey` against `/api/workspaces/*/events` with 10
  workspaces × 50 rps.
- **Migration test**: a fixture at `testdata/legacy-workspaces.json` is loaded
  by a dedicated test that asserts the v2 registry is correct.

Meaningful integration and e2e tests are preferred over coverage-padding unit
tests — consistent with the team's testing rule.

### 12.2 Verification checklist (100+ items)

Tick each item during Phase H on **both** macOS and Linux. Checklist is
grouped by feature area. Items marked `[carry]` are carried forward from the
agents-revamp issue #2999 verification set; re-running them catches
regressions introduced by this proposal.

#### 12.2.1 Registry + migration (12 items)

- [ ] 1. Fresh install: `~/.bc/registry.json` is created on first bcd boot
- [ ] 2. `version` field is `2`
- [ ] 3. An initialised cwd is auto-registered as `default_workspace`
- [ ] 4. Legacy `~/.bc/workspaces.json` is migrated, original kept as `.legacy`
- [ ] 5. Migration is idempotent (running twice is a no-op)
- [ ] 6. `bc workspace list` prints all workspaces with ids and aliases
- [ ] 7. `bc workspace add <path>` with missing `.bc/` prompts to init
- [ ] 8. `bc workspace remove` unregisters but leaves `.bc/` intact on disk
- [ ] 9. `bc workspace rename` updates `name` and refreshes the cache
- [ ] 10. `bc workspace default <id>` persists and is reflected in the UI
- [ ] 11. Duplicate `path` add returns the existing record (no dupes)
- [ ] 12. Registry writes are atomic (kill -9 mid-write leaves valid JSON)

#### 12.2.2 Discovery (10 items)

- [ ] 13. Local scan at `~/Projects` with `depth=3` returns all `.git` repos
- [ ] 14. Scan skips `node_modules`, `dist`, `build`, `.git`
- [ ] 15. Already-registered repos are flagged with `already_registered: true`
- [ ] 16. GitHub OAuth login opens a browser and stores the token (0600)
- [ ] 17. GitHub repo list paginates beyond 30 repos
- [ ] 18. Clone from GitHub creates the workspace and registers it
- [ ] 19. Clone handles SSH URLs correctly when SSH key is available
- [ ] 20. `DELETE /api/auth/github` removes the token and returns `connected: false`
- [ ] 21. API never returns the raw token in any response
- [ ] 22. Cloning into a non-empty target fails gracefully

#### 12.2.3 URL + routing (14 items)

- [ ] 23. `/` redirects to `/w/<default>/live`
- [ ] 24. `/live` redirects to `/w/<active>/live` with 301
- [ ] 25. `/agents/<name>` redirects to `/w/<active>/agents/<name>/attach`
- [ ] 26. `/workspace` redirects to `/w/<active>/settings`
- [ ] 27. Visiting `/w` with zero workspaces shows empty state
- [ ] 28. Visiting `/w` with workspaces shows the picker
- [ ] 29. Switching workspace via dropdown updates `last_used_at`
- [ ] 30. Switching workspace preserves the current tab (e.g., on agents)
- [ ] 31. Direct deep link `/w/<id>/agents/foo/config` loads the Config tab
- [ ] 32. Invalid `wsId` returns a 404 page with a "Go to picker" button
- [ ] 33. `X-BC-Workspace` header overrides cookie on legacy routes
- [ ] 34. Legacy API responses include `Deprecation: true` header
- [ ] 35. Legacy API responses include `Sunset:` header
- [ ] 36. MCP SSE at `/_mcp/<agent>/sse` redirects to wsId-scoped URL

#### 12.2.4 Header + layout (12 items)

- [ ] 37. Sidebar collapse persists across reloads
- [ ] 38. Sidebar toggle button appears in the Header left slot
- [ ] 39. WorkspaceDropdown shows `name` + `[id]` suffix
- [ ] 40. WorkspaceDropdown search filters by name, alias, and path
- [ ] 41. `⌘K` opens the dropdown
- [ ] 42. "Add workspace" footer opens AddWorkspaceModal
- [ ] 43. AddWorkspaceModal Scan tab lists candidates with checkboxes
- [ ] 44. AddWorkspaceModal GitHub tab prompts OAuth when no token
- [ ] 45. AddWorkspaceModal Manual tab validates path
- [ ] 46. Header `center` slot renders page-specific title/breadcrumb
- [ ] 47. Header `actions` slot renders page-specific CTAs
- [ ] 48. Header uses the monospace HUD aesthetic consistently

#### 12.2.5 Code tab — top-level (18 items)

- [ ] 49. `/w/<id>/code` opens with main repo selected and file tree shown
- [ ] 50. File tree lazy-loads children on expand (depth=1 per click)
- [ ] 51. File tree hides `.git/` by default
- [ ] 52. File tree hides `.bc/` by default; `show_hidden=1` shows it
- [ ] 53. Clicking a file opens it in `MonacoReadOnly`
- [ ] 54. Binary file shows a "download" link
- [ ] 55. File >2MB is truncated with a clear indicator
- [ ] 56. Language is inferred from extension (e.g., `.tsx` → TypeScript)
- [ ] 57. Worktree dropdown lists main repo + each agent's worktree
- [ ] 58. Selecting an agent worktree switches to DiffEditor with base = main
- [ ] 59. DiffEditor renders side-by-side on ≥1280px, inline below
- [ ] 60. Diff stats (additions/deletions) display correctly
- [ ] 61. Search (ripgrep) returns results with file+line+snippet
- [ ] 62. Search debounces at 300ms
- [ ] 63. Path traversal `../../../etc/passwd` returns 400
- [ ] 64. Symlink-out-of-root returns 400
- [ ] 65. Breadcrumb path is clickable; each segment scopes the tree
- [ ] 66. Monaco loads without external CDN calls

#### 12.2.6 Code tab — per-agent (8 items)

- [ ] 67. `/w/<id>/agents/foo/code` opens with agent worktree in diff mode
- [ ] 68. The tab is #5 in the agent detail tab bar
- [ ] 69. Diff view matches `git diff main...worktree` output
- [ ] 70. Toggling "plain view" switches to MonacoReadOnly on the worktree
- [ ] 71. If worktree doesn't exist, shows a clear empty state
- [ ] 72. Updating a file on disk refreshes the diff on next click
- [ ] 73. File selection persists when switching between diff/plain
- [ ] 74. Deep link `/w/<id>/agents/foo/code?path=src/a.ts` works

#### 12.2.7 code-server mode (6 items)

- [ ] 75. "Edit in VS Code" toggle is hidden when dep is stopped
- [ ] 76. Starting `bc-code-server` shows the toggle within 10s
- [ ] 77. Toggle renders an iframe to the allocated port
- [ ] 78. iframe uses `sandbox` attribute with limited permissions
- [ ] 79. Warning banner about write access is dismissible but remembered
- [ ] 80. Stopping the dep removes the toggle and returns to MonacoReadOnly

#### 12.2.8 Dependencies manager (10 items)

- [ ] 81. `/api/deps` lists bc-db, bc-code-server, bc-browser
- [ ] 82. Starting bc-db shows `starting` then `running` within 15s
- [ ] 83. Stopping bc-db transitions to `stopped`
- [ ] 84. Logs endpoint streams via SSE and can be paused/resumed
- [ ] 85. bc-browser is visible but marked "Deprecated"
- [ ] 86. bc-browser start returns 409 with a clear reason
- [ ] 87. Missing Docker returns 503 from `/api/deps/bc-db/start`
- [ ] 88. Non-loopback request to `/start` is refused unless `MYCEL_REMOTE=1`
- [ ] 89. Per-workspace code-server endpoint reports the correct port
- [ ] 90. Settings → Dependencies page reflects live status via polling

#### 12.2.9 Agent detail tab reorder (8 items)

- [ ] 91. Tab order is Attach, Live, Config, Metrics, Code
- [ ] 92. Default tab is Attach
- [ ] 93. Deep link `/…/agents/foo` renders Attach
- [ ] 94. Deep link `/…/agents/foo/live` renders Live
- [ ] 95. `carry`: Live event stream renders without regressions
- [ ] 96. `carry`: Config tab MCP list + add/delete still work
- [ ] 97. `carry`: Metrics timeframe selector still works
- [ ] 98. Code tab defaults to the agent's worktree

#### 12.2.10 Isolation (6 items)

- [ ] 99. Two workspaces with same basename produce distinct tmux session names
- [ ] 100. Events from ws A never appear in ws B's event stream
- [ ] 101. Channels in ws A are invisible in ws B
- [ ] 102. `carry`: Starting an agent in ws A does not affect ws B agents
- [ ] 103. Stats queries scope correctly by `workspace_id`
- [ ] 104. `carry`: Cost import does not leak across workspaces

#### 12.2.11 Agents revamp regressions (12 `carry` items from #2999)

- [ ] 105. `carry`: Template selection renders a default system prompt
- [ ] 106. `carry`: Avatar animation plays on `hook.tool_use`
- [ ] 107. `carry`: Task graph renders parent/child relationships
- [ ] 108. `carry`: `waiting` state shows approve/deny UI
- [ ] 109. `carry`: 22 event types all render in Live
- [ ] 110. `carry`: Filter pills combine correctly (AND semantics)
- [ ] 111. `carry`: Fork flow creates a new agent with shared template
- [ ] 112. `carry`: Auto-discovery reconciles on bcd restart
- [ ] 113. `carry`: Stats graphs don't crash with zero data
- [ ] 114. `carry`: MCP add/delete is reflected without page reload
- [ ] 115. `carry`: Secrets are masked in the UI
- [ ] 116. `carry`: Event store TTL pruning runs on schedule

#### 12.2.12 Performance + stability (6 items)

- [ ] 117. 10 workspaces × 20 agents boot within 60s
- [ ] 118. Memory overhead per idle workspace < 20MB
- [ ] 119. Services eviction kicks in after 30min idle
- [ ] 120. No goroutine leaks (runtime.NumGoroutine stable after 1h run)
- [ ] 121. Restarting bcd preserves registry and worktree naming
- [ ] 122. Killed bcd mid-write leaves registry.json valid JSON

---

## 13. Alignment with `agents-revamp.md`

### 13.1 What stays in `agents-revamp.md`

- Templates (prompt packs) as the primary agent creation axis
- Avatar system + animations
- Live events pipeline, 22 event types, task graph
- Config editor (system prompt, MCPs, secrets)
- Runtime-gated edits
- Dedicated metrics tab with timeframes
- Removal of the parent/child hierarchy

### 13.2 What evolves in this proposal

- **Agent detail tabs**: reordered to Attach / Live / Config / Metrics / Code.
  The agents-revamp doc opened `Live` by default; this proposal makes
  `Attach` the default and adds `Code` as tab #5.
- **Routing**: all agent routes are now namespaced under
  `/w/:wsId/agents/:name/*`. The agents-revamp doc assumed a flat `/agents/:name`.
- **Workspace nav item**: removed and folded into `Settings`. Agents-revamp
  listed a `Workspace` nav item.
- **MCP SSE endpoint**: moves from `/_mcp/<agent>/sse` to
  `/_mcp/<wsID>/<agent>/sse`. Legacy path redirects for one major version.

### 13.3 What moves here

- All multi-workspace content.
- URL shape + shared Header.
- Code tab (new surface; not mentioned in agents-revamp.md).
- Optional dependencies manager (new).

### 13.4 Document updates required

`docs/proposals/agents-revamp.md` is updated by this proposal with:

- A note at the top linking to this doc as an extension.
- A new section at the bottom documenting the tab reorder and the addition
  of the Code tab.

No other content in `agents-revamp.md` is rewritten; its v2 design remains
authoritative for templates/avatars/live.

---

*End of proposal.*
