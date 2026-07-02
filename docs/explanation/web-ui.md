# Web Dashboard Architecture

Design document for the bc web dashboard — one of four equal API clients of the `bcd` daemon.

---

## 1. System Context

The web dashboard is one of four equal API consumers of the `bcd` daemon. It has no special access, no elevated privileges, and no server-side rendering dependency on `bcd` internals. Every operation available in the web UI is available through the same REST + SSE API that the CLI, TUI, and MCP agents use.

```mermaid
graph TB
    subgraph Clients ["API Clients (equal peers)"]
        CLI["mycel CLI<br/><code>internal/cmd/</code><br/>Go binary, direct function calls"]
        TUI["bc TUI<br/><code>tui/src/</code><br/>React/Ink terminal app"]
        Web["bc Web Dashboard<br/><code>web/src/</code><br/>React SPA in browser"]
        MCP["MCP Agents<br/><code>server/mcp/</code><br/>AI tool servers"]
    end

    subgraph Daemon ["bcd Daemon (localhost:9374)"]
        REST["/api/* — REST endpoints<br/>JSON request/response"]
        SSE["/api/events — SSE stream<br/>Real-time push events"]
        Static["/ — Static file server<br/>Embedded web/dist/"]
        Health["/health — Liveness probe"]
    end

    subgraph Storage ["Storage Layer"]
        DB["SQLite<br/>~/.bc/bc.db<br/>agents, subscriptions, costs,<br/>roles, events, cron"]
        FS["Filesystem<br/>~/.bc/<br/>settings.json, logs,<br/>agent worktrees"]
    end

    CLI -->|"HTTP GET/POST"| REST
    TUI -->|"HTTP GET/POST"| REST
    Web -->|"HTTP GET/POST"| REST
    MCP -->|"HTTP GET/POST"| REST

    TUI -->|"EventSource"| SSE
    Web -->|"EventSource"| SSE

    REST --> DB
    REST --> FS
    SSE --> DB

    Static -->|"serves"| Web
```

**Key principle:** The web dashboard is a thin view layer. All business logic, persistence, agent orchestration, and event publishing live in `bcd`. The dashboard only renders state received over HTTP and SSE.

---

## 2. Component Architecture

### 2.1 Component Tree

```mermaid
graph TD
    Root["ReactDOM.createRoot"]
    Root --> StrictMode["React.StrictMode"]
    StrictMode --> TP["ThemeProvider<br/>light/dark mode, Solar Flare tokens"]
    TP --> SSEP["SSEProvider<br/>singleton EventSource"]
    SSEP --> Router["BrowserRouter (react-router-dom)"]
    Router --> Layout["Layout Shell"]

    subgraph Layout ["Layout Shell (AppShell)"]
        Sidebar["Sidebar<br/>@bc/ui NavLink, Badge"]
        Header["Header Bar<br/>team selector, theme toggle"]
        Main["Main Content Area<br/>Outlet / children"]
        Toast["Toast Container<br/>@bc/ui Toast"]
    end

    Layout --> Suspense["React.Suspense<br/>per-route lazy loading"]
    Suspense --> Views["View Components"]

    subgraph Views ["Views (lazy-loaded)"]
        V_Dash["Dashboard"]
        V_Agents["Agents"]
        V_Notifications["Notifications"]
        V_Costs["Costs"]
        V_Teams["Teams"]
        V_Roles["Roles (CRUD)"]
        V_Tools["Tools"]
        V_MCP["MCP"]
        V_Logs["Logs"]
        V_Cron["Cron"]
        V_Secrets["Secrets"]
        V_Doctor["Doctor"]
    end

    subgraph SharedLib ["@bc/ui — Shared Component Library"]
        Button["Button"]
        Input["Input"]
        Badge["Badge / StatusBadge"]
        Table["Table"]
        Card["Card"]
        Panel["Panel"]
        Modal["Modal"]
        ToastComp["Toast"]
        Spinner["Spinner / LoadingIndicator"]
    end

    Views -->|"imports"| SharedLib
```

**Structural decisions:**

- Every view is wrapped in its own `ErrorBoundary` so a crash in one view does not affect the sidebar or other views. A root-level `ErrorBoundary` wraps the entire `BrowserRouter` as a last resort.
- `Layout` (`web/src/components/Layout.tsx`) renders a fixed 192px sidebar with `NavLink` items and an `<Outlet />` for the matched child route.
- Views are lazy-loaded per route via `React.Suspense`.

### 2.2 Shared Components

| Component | File | Purpose |
|---|---|---|
| `Layout` | `web/src/components/Layout.tsx` | Shell: sidebar nav + main content area via `<Outlet/>` |
| `ErrorBoundary` | `web/src/components/ErrorBoundary.tsx` | Class component; catches render errors, shows retry UI |
| `StatusBadge` | `web/src/components/StatusBadge.tsx` | Colored pill for agent states (idle, working, done, stuck, error, stopped) |
| `Table<T>` | `web/src/components/Table.tsx` | Generic typed table with columns, row click, empty state |

---

## 3. Shared Component Library Design

> **Note:** The @bc/ui shared component library is a Phase 1 roadmap item. The current implementation uses local components in each package.

### 3.1 Problem Statement

The web dashboard and TUI duplicate every UI primitive. Both have a `StatusBadge`, both have a `Table`, both have a `Panel` -- but with incompatible interfaces and no code sharing. When the Solar Flare design system rolls out, every component must be updated in two places. When a new component is needed, it is built twice.

### 3.2 Architecture: Common Interface, Separate Renderers

The shared library lives in a `packages/ui/` monorepo package. It exports a common props interface for each primitive. Two renderer packages provide platform-specific implementations.

```mermaid
graph TD
    subgraph packages ["packages/ (monorepo)"]
        subgraph ui ["@bc/ui"]
            Types["types.ts<br/>ButtonProps, InputProps,<br/>BadgeProps, TableProps,<br/>CardProps, PanelProps, ..."]
            Tokens["tokens.ts<br/>Solar Flare design tokens<br/>(colors, spacing, radii)"]
        end

        subgraph uiWeb ["@bc/ui-web"]
            WebButton["Button.tsx<br/>renders &lt;button&gt; + Tailwind"]
            WebInput["Input.tsx<br/>renders &lt;input&gt; + Tailwind"]
            WebBadge["Badge.tsx<br/>renders &lt;span&gt; + Tailwind"]
            WebTable["Table.tsx<br/>renders &lt;table&gt; + Tailwind"]
            WebCard["Card.tsx<br/>renders &lt;div&gt; + Tailwind"]
            WebPanel["Panel.tsx<br/>renders &lt;div&gt; with border"]
            WebModal["Modal.tsx<br/>renders &lt;dialog&gt;"]
            WebToast["Toast.tsx<br/>renders floating div"]
            WebSpinner["Spinner.tsx<br/>CSS animation"]
        end

        subgraph uiTui ["@bc/ui-ink"]
            TuiButton["Button.tsx<br/>renders Ink Box + Text"]
            TuiInput["Input.tsx<br/>renders Ink TextInput"]
            TuiBadge["Badge.tsx<br/>renders Ink Text with color"]
            TuiTable["Table.tsx<br/>renders Ink Box grid"]
            TuiCard["Card.tsx<br/>renders Ink Box with border"]
            TuiPanel["Panel.tsx<br/>renders Ink Box borderStyle=single"]
            TuiModal["Modal.tsx<br/>renders Ink overlay Box"]
            TuiSpinner["Spinner.tsx<br/>renders Ink Spinner"]
        end
    end

    subgraph consumers ["Consumer Apps"]
        WebApp["web/ — Web Dashboard<br/>imports from @bc/ui-web"]
        TuiApp["tui/ — Terminal UI<br/>imports from @bc/ui-ink"]
    end

    ui -->|"shared types + tokens"| uiWeb
    ui -->|"shared types + tokens"| uiTui
    uiWeb -->|"used by"| WebApp
    uiTui -->|"used by"| TuiApp
```

### 3.3 Primitive Component Inventory

Each primitive has a single props interface in `@bc/ui` and two renderers.

| Component | `@bc/ui` Props | `@bc/ui-web` Renders As | `@bc/ui-ink` Renders As |
|---|---|---|---|
| **Button** | `variant`, `size`, `disabled`, `onClick`, `children` | `<button>` with Tailwind classes | Ink `<Box>` with border + `<Text>` |
| **Input** | `value`, `onChange`, `placeholder`, `disabled` | `<input>` with Tailwind | Ink `TextInput` component |
| **Badge** | `variant` (status/role/info), `children` | `<span>` pill with bg/text colors | Ink `<Text>` with ANSI color |
| **StatusBadge** | `state` (idle/working/done/error/...) | `<span>` with semantic color classes | Ink `<Text>` with symbol + color |
| **Table** | `columns`, `data`, `keyFn`, `onRowClick`, `emptyMessage` | HTML `<table>` | Ink `<Box>` column layout |
| **Card** | `title`, `children`, `variant` | `<div>` with border + padding | Ink `<Box>` with borderStyle |
| **Panel** | `title`, `children`, `focused`, `borderColor` | `<div>` with header + border | Ink `<Box borderStyle="single">` |
| **Modal** | `open`, `onClose`, `title`, `children` | `<dialog>` or portal overlay | Ink `<Box>` absolute overlay |
| **Toast** | `message`, `variant` (success/error/info), `duration` | Floating `<div>` with auto-dismiss | Ink `<Box>` at bottom of screen |
| **Spinner** | `label`, `size` | CSS keyframe animation | Ink `<Spinner>` component |
| **ProgressBar** | `value`, `max`, `label` | `<div>` with width percentage | Ink `<Box>` with filled chars |

### 3.4 Design Token Flow

```mermaid
graph LR
    subgraph Source ["@bc/ui/tokens.ts"]
        SF["Solar Flare palette<br/>Named colors + semantic mappings<br/>Dark + Light mode variants"]
    end

    subgraph WebTokens ["Web Token Pipeline"]
        CSS["tokens.css<br/>CSS custom properties<br/>--bc-bg, --bc-surface, ..."]
        TW["tailwind.config.ts<br/>bc-* color aliases<br/>→ var(--bc-*)"]
        Classes["Tailwind classes<br/>bg-bc-surface, text-bc-accent, ..."]
    end

    subgraph TuiTokens ["TUI Token Pipeline"]
        ThemeCtx["ThemeContext<br/>ThemeColors object"]
        ANSI["ANSI color mapping<br/>256-color with fallbacks"]
        InkColor["Ink color prop<br/>color={theme.colors.accent}"]
    end

    Source -->|"JS export"| CSS
    Source -->|"JS export"| ThemeCtx
    CSS --> TW --> Classes
    ThemeCtx --> ANSI --> InkColor
```

### 3.5 Implementation Strategy

**Phase 1 -- Extract types.** Create `packages/ui/` with shared `Props` interfaces and `tokens.ts`. No renderer changes. Both apps continue using their existing components but the interfaces converge.

**Phase 2 -- Web renderers.** Create `packages/ui-web/` implementing all primitives against the shared interfaces. Migrate `web/src/components/` to re-export from `@bc/ui-web`. The web dashboard's existing components become thin wrappers.

**Phase 3 -- TUI renderers.** Create `packages/ui-tui/` implementing the same interfaces with Ink primitives. Migrate `tui/src/components/` to re-export from `@bc/ui-ink`.

**Phase 4 -- Delete duplicates.** Remove the original component files from both `web/src/components/` and `tui/src/components/`. All imports resolve to `@bc/ui-web` or `@bc/ui-ink`.

---

## 4. Routing and Navigation

### 4.1 Route Map

```mermaid
graph LR
    subgraph Routes ["Routes"]
        I["/ — Dashboard"]
        A["/agents — Agent list"]
        AD["/agents/:name — Agent detail<br/>(terminal peek, cost, logs)"]
        CH["/notifications — Notification sources"]
        CHD["/notifications/:sourceName — Notification source detail"]
        CO["/costs — Cost overview"]
        T_List["/teams — Team list"]
        T_Detail["/teams/:name — Team detail<br/>(members, config, metrics)"]
        RO_List["/roles — Role list"]
        RO_Detail["/roles/:name — Role detail"]
        RO_New["/roles/new — Create role"]
        RO_Edit["/roles/:name/edit — Edit role"]
        T["/tools — Tools"]
        M["/mcp — MCP servers"]
        CR["/cron — Cron jobs"]
        SE["/secrets — Secrets"]
        LO["/logs — Event log"]
        DR["/doctor — Health check"]
    end
```

Navigation is a static `NAV_ITEMS` array in `Layout.tsx`. Each entry has a `to` path, `label`, and single-character `icon`. `NavLink` provides active styling. The index route (`/`) uses the `end` prop.

---

## 5. Data Layer

### 5.1 API Client

`web/src/api/client.ts` exports a typed `api` object with methods for all REST endpoints. It uses a `request<T>()` helper that prepends `/api`, sets `Content-Type: application/json`, throws on non-2xx, and returns `res.json()` cast to `T`.

**API surface:**

| Method | Endpoint | Used By |
|---|---|---|
| `api.listAgents()` | `GET /api/agents` | Dashboard, Agents |
| `api.getAgent(name)` | `GET /api/agents/:name` | Agent detail |
| `api.startAgent(name)` | `POST /api/agents/:name/start` | Agents |
| `api.stopAgent(name)` | `POST /api/agents/:name/stop` | Agents |
| `api.sendToAgent(name, msg)` | `POST /api/agents/:name/send` | Agents |
| `api.listGateways()` | `GET /api/gateways` | Dashboard, Notifications |
| `api.listNotificationSources()` | `GET /api/channels` | Notifications |
| `api.getChannelHistory(name, limit, before)` | `GET /api/channels/:name/history` | Notifications |
| `api.getChannelSubscriptions(channel)` | `GET /api/gateways/:gw/channels/:ch/agents` or `/api/notify/subscriptions/:channel` | Notifications |
| `api.getChannelActivity(channel, limit)` | `GET /api/gateways/:gw/channels/:ch/activity` or `/api/notify/activity/:channel` | Notifications |
| `api.getCostSummary()` | `GET /api/costs` | Dashboard, Costs |
| `api.getCostByAgent()` | `GET /api/costs/agents` | Costs |
| `api.listRoles()` | `GET /api/roles` | Roles |
| `api.getRole(name)` | `GET /api/roles/:name` | Role detail |
| `api.createRole(data)` | `POST /api/roles` | Role create form |
| `api.updateRole(name, data)` | `PUT /api/roles/:name` | Role edit form |
| `api.deleteRole(name)` | `DELETE /api/roles/:name` | Role delete action |
| `api.listTools()` | `GET /api/tools` | Tools |
| `api.listMCP()` | `GET /api/mcp` | MCP |
| `api.getLogs(tail)` | `GET /api/logs` | Logs |
| `api.getDoctor()` | `GET /api/doctor` | Doctor |
| `api.listCron()` | `GET /api/cron` | Cron |
| `api.listSecrets()` | `GET /api/secrets` | Secrets |
| `api.listTeams()` | `GET /api/teams` | Teams |
| `api.getTeam(name)` | `GET /api/teams/:name` | Team detail |

**Design requirements:**

- `AbortController` on every `fetch()`, cleaned up in `useEffect` return.
- Request deduplication via a shared API client instance (singleton module or context).
- Exponential backoff retry for transient failures (503, network errors).
- No auth headers -- assumes same-origin (localhost only).

### 5.2 Shared API Client

Both the web dashboard and TUI use the same typed HTTP client, extracted to `packages/api/`:

```mermaid
graph TD
    subgraph pkg ["packages/api/ — @bc/api"]
        Client["client.ts<br/>request&lt;T&gt;() wrapper<br/>AbortController, retry, dedup"]
        Types["types.ts<br/>Agent, Channel, CostSummary,<br/>Role, Team, etc."]
        Methods["methods.ts<br/>api.listAgents(), api.listTeams(),<br/>api.createRole(), etc."]
        SSEClient["sse.ts<br/>createSSEConnection()<br/>subscribe(type, callback)"]
    end

    subgraph web ["web/"]
        WebHooks["hooks/<br/>usePolling, useWebSocket"]
    end

    subgraph tui ["tui/"]
        TuiHooks["hooks/<br/>useAgents, useNotifications, usePolling"]
    end

    pkg -->|"import @bc/api"| WebHooks
    pkg -->|"import @bc/api"| TuiHooks
```

### 5.3 Data Flow: Typical View Load

```mermaid
sequenceDiagram
    participant User
    participant Router as React Router
    participant View as View Component
    participant Hook as usePolling
    participant API as api client (fetch)
    participant BCD as bcd daemon
    participant SSE as SSE Provider

    User->>Router: Navigate to /agents
    Router->>View: Render Agents component
    View->>Hook: usePolling(fetchAgents, 5000)
    Hook->>API: api.listAgents()
    API->>BCD: GET /api/agents
    BCD-->>API: 200 JSON [Agent, Agent, ...]
    API-->>Hook: Agent[]
    Hook-->>View: { data: Agent[], loading: false, error: null }
    View-->>User: Render agent table

    Note over Hook: setInterval(5000) starts

    SSE-->>View: agent.state_changed { name: "eng-01" }
    View->>Hook: refresh()
    Hook->>API: api.listAgents()
    API->>BCD: GET /api/agents
    BCD-->>API: 200 JSON (updated)
    API-->>Hook: Agent[] (fresh)
    Hook-->>View: { data: Agent[] (updated) }
    View-->>User: Re-render with new state

    User->>Router: Navigate away from /agents
    Router->>View: Unmount
    View->>Hook: cleanup (clearInterval, abort in-flight fetch)
```

---

## 6. State Management

### 6.1 Approach

There is no global state store. Each view manages its own data via `usePolling` and local `useState`. SSE events are delivered through a singleton `SSEProvider` context.

```mermaid
graph TD
    subgraph ServerState ["Server State (via bcd API)"]
        Agents["Agent list + states"]
        Channels["Notification sources + activity"]
        Costs["Cost aggregates"]
        Roles["Role definitions"]
        Tools["Tool/MCP/Secret config"]
    end

    subgraph UIState ["UI State (local)"]
        Selected["Selected row/item"]
        Expanded["Expanded/collapsed sections"]
        FormInput["Form input values"]
        ChatDraft["Filter/search state"]
    end

    subgraph InfraState ["Infrastructure State"]
        Theme["Theme mode<br/>(localStorage + context)"]
        SSEConn["SSE connection status<br/>(context)"]
        RouteState["Current route<br/>(react-router)"]
    end
```

### 6.2 State Categories and Where They Live

| Category | Location | Rationale |
|---|---|---|
| **Agent list** | Per-view `usePolling` + SSE refresh | Server is source of truth; no client cache needed beyond current fetch |
| **Notification feed** | Per-view local state + SSE append | Events appended via SSE, full refetch for consistency |
| **Cost data** | Per-view `usePolling` | Aggregated, changes slowly |
| **Roles** | Per-view fetch + invalidate on mutation | Refetch after create/update/delete |
| **Theme** | `ThemeProvider` context, persisted to `localStorage` | User preference survives page reload |
| **SSE connection** | Singleton `SSEProvider` context | Single `EventSource` shared across all views |
| **Selected row** | Local `useState` | Ephemeral, resets on navigation |
| **Form state** | Local `useState` (or `useActionState` in React 19) | Ephemeral, tied to form lifecycle |
| **Route params** | `react-router-dom` `useParams` | Framework-managed |

### 6.3 Polling and SSE by View

| View | Polling Interval | SSE Events | API Calls |
|---|---|---|---|
| Dashboard | 5s | -- | `listAgents`, `listGateways`, `getCostSummary` |
| Agents | 5s | `agent.state_changed` | `listAgents`, `startAgent`, `stopAgent`, `sendToAgent` |
| Notifications | 10s (list) | `gateway.message` | `listGateways`, `listNotificationSources`, `getChannelHistory`, `getChannelActivity` |
| Costs | 10s | -- | `getCostSummary`, `getCostByAgent` |
| Roles | 30s | -- | `listRoles` + full CRUD |
| Tools | 30s | -- | `listTools` |
| MCP | 30s | -- | `listMCP` |
| Logs | 5s | all events | `getLogs` |
| Doctor | 30s | -- | `getDoctor` |
| Cron | 10s | -- | `listCron` |
| Secrets | 30s | -- | `listSecrets` |
| Teams | 10s | -- | `listTeams` |

**Known duplication concerns:**

- Dashboard, Agents, and Costs all independently poll `listAgents` and/or cost endpoints. Multiple `setInterval` timers hit the same endpoints on overlapping schedules.
- `useWebSocket` should be a singleton SSE provider rather than instantiated per component.
- Logs view polls every 5s but should use SSE, since the event log is exactly what SSE was designed for.

---

## 7. Styling and Theme

### 7.1 Token Layer

Styling uses Tailwind CSS utility classes with a custom color palette mapped to CSS custom properties.

**Tokens** (`web/src/theme/tokens.css`):

```css
:root {
  --bc-bg: #0C0A08;          /* Obsidian Warm */
  --bc-surface: #151210;     /* Ember Dark */
  --bc-border: #2A2420;      /* Bark */
  --bc-text: #F5F0EB;        /* Warm White */
  --bc-muted: #8C7E72;       /* Sandstone Dark */
  --bc-accent: #EA580C;      /* Tangerine */
  --bc-success: #22C55E;     /* Success Green Bright */
  --bc-warning: #FB923C;     /* Warning Amber */
  --bc-error: #EF4444;       /* Error Red Bright */
}

[data-theme="light"] {
  --bc-bg: #FBF7F2;          /* Parchment */
  --bc-surface: #FFFFFF;     /* White */
  --bc-border: #E5DDD4;      /* Linen */
  --bc-text: #1E1A16;        /* Umber */
  --bc-muted: #78706A;       /* Sandstone */
  --bc-accent: #EA580C;      /* Tangerine */
  --bc-success: #16A34A;     /* Success Green */
  --bc-warning: #EA580C;     /* Warning Orange */
  --bc-error: #DC2626;       /* Error Red */
}
```

**Tailwind bridge** (`web/tailwind.config.ts`): maps `bc-*` color names to the CSS variables, so classes like `bg-bc-surface`, `text-bc-accent`, `border-bc-border` resolve to the custom properties.

The font stack is `ui-monospace, SFMono-Regular, 'SF Mono', Menlo, monospace`. Theme preference is stored in `localStorage('bc-theme')`, defaulting to system preference via `prefers-color-scheme`. Toggle is managed by `ThemeProvider` context, applied as `data-theme` attribute on `<html>`.

### 7.2 Hardcoded Color Audit

Several views use Tailwind color literals directly instead of `bc-*` tokens. These break in light mode because they have no corresponding light-mode override:

- `Roles.tsx`: `bg-purple-500/20 text-purple-400`, `bg-cyan-500/20 text-cyan-400`, `bg-orange-500/20 text-orange-400`, `bg-blue-500/20 text-blue-400`, `bg-yellow-500/20 text-yellow-400`, `bg-green-500/20 text-green-400`, `bg-emerald-500/20 text-emerald-400`
- `Agents.tsx`, `Tools.tsx`, `MCP.tsx`, `Cron.tsx`, `Secrets.tsx`, `Doctor.tsx`: various hardcoded color literals

All hardcoded colors must migrate to semantic token classes (e.g., `bg-bc-accent/20 text-bc-accent`) or to new categorical tokens (e.g., `--bc-tag-mcp`, `--bc-tag-secret`).

---

## 8. Future Considerations

The web dashboard currently uses **Vite 6 + react-router-dom 6** as its build and routing stack. This section captures potential future directions.

### 8.1 Framework Evolution

The current stack (Vite + react-router) serves well for a localhost SPA embedded in bcd. If future requirements demand server-side rendering (e.g., shareable report pages, print-friendly cost summaries), a framework migration could be evaluated. Key trade-offs to consider:

- **Automatic code splitting** vs. current manual `React.lazy` per route
- **File-based routing** vs. declarative `<Route>` registration in `App.tsx`
- **Build complexity** -- the current Vite build produces a flat `dist/` that bcd embeds via `//go:embed`; alternative frameworks may require different embedding strategies
- **Dev server performance** -- Vite's cold start is significantly faster than heavier frameworks

### 8.2 Shared Component Library

Extracting shared UI primitives into a `packages/ui/` monorepo package would allow the web dashboard, TUI, and landing page to share types, tokens, and component interfaces. See section 3 for the proposed architecture.

---

## 9. Teams Replace Workspaces

### 9.1 Design

Teams are the primary organizational unit. Agents belong to teams, and the agent naming convention encodes team membership: `bc-<session-id-last6>-<team>-<agent>`.

**API:**

| Method | Endpoint | Purpose |
|---|---|---|
| `GET /api/teams` | List all teams | List view |
| `GET /api/teams/:name` | Team detail: members, agents, config, metrics | Detail view |
| `GET /api/roles` | List roles (standalone, not nested under workspace) | Roles view |

**UI:**

| Element | Design |
|---|---|
| Sidebar label | "Teams" |
| Routes | `/teams` (list) + `/teams/:name` (detail) |
| Dashboard summary | Per-team agent counts, team status indicators |
| Agent table | Grouped by team, with team column and filter |
| Agent name display | Parsed: highlight team segment, linkable to team detail |

### 9.2 Agent Naming Convention

Agent names follow `bc-<session-id-last6>-<team>-<agent>`. The web dashboard parses this format to extract and display the team and agent segments separately.

```
bc-a1b2c3-frontend-eng-01
   |       |         |
   |       |         +-- Agent name within team
   |       +-- Team name
   +-- Session ID (last 6 chars)
```

The Agents view displays this as columns: Session, Team, Agent, Role, Status.

---

## 10. Roles: Database-Backed CRUD

### 10.1 API

Roles are stored in SQLite with a full CRUD API:

| Method | Endpoint | Purpose |
|---|---|---|
| `GET /api/roles` | List all roles | List view |
| `GET /api/roles/:name` | Get single role | Detail view |
| `POST /api/roles` | Create role | Create form |
| `PUT /api/roles/:name` | Update role | Edit form |
| `DELETE /api/roles/:name` | Delete role | Delete confirmation |

### 10.2 UI Design

**List view** (`/roles`): Card layout with "Create Role" button and per-card edit/delete actions.

**Create/Edit form** (`/roles/new`, `/roles/:name/edit`): Form with fields for:
- Name (text input, required, immutable on edit)
- Prompt (multiline textarea)
- MCP Servers (tag input)
- Secrets (tag input)
- Plugins (tag input)
- Lifecycle prompts: on-create, on-start, on-stop, on-delete (collapsible section)
- Commands (key-value editor)
- Rules (key-value editor)
- Skills (key-value editor)
- Settings (JSON editor)

**Delete flow**: Confirmation modal ("Delete role `engineer`? This cannot be undone.") before sending `DELETE /api/roles/:name`.

---

## 11. Global Config Directory

The config directory lives at `~/.bc/` as a global location. The web dashboard references this in several places:

- `Roles.tsx` displays labels for role field names from the DB, not filesystem paths.
- The `Doctor` view surfaces file-path-based diagnostics that reference `~/.bc/` paths.
- The `Teams` view shows the workspace config path as `~/.bc/settings.json`.

---

## 12. File Reference

| File | Role |
|---|---|
| `web/src/main.tsx` | Entry point: mounts `<App />` in `React.StrictMode` |
| `web/src/App.tsx` | Route definitions, root `ErrorBoundary` |
| `web/src/components/Layout.tsx` | App shell: sidebar nav + content outlet |
| `web/src/components/ErrorBoundary.tsx` | React error boundary with retry UI |
| `web/src/components/StatusBadge.tsx` | Agent state pill component |
| `web/src/components/Table.tsx` | Generic typed table component |
| `web/src/api/client.ts` | REST API client with typed methods |
| `web/src/api/types.ts` | SSE event type definitions |
| `web/src/hooks/usePolling.ts` | Generic polling hook (fetch + setInterval) |
| `web/src/hooks/useWebSocket.ts` | SSE connection hook (misnamed, uses EventSource) |
| `web/src/views/*.tsx` | 12 view components (one per route) |
| `web/src/theme/tokens.css` | CSS custom properties + Tailwind base |
| `web/tailwind.config.ts` | Tailwind config mapping `bc-*` to CSS vars |
| `web/vite.config.ts` | Vite config: dev proxy to bcd, build output |
| `web/package.json` | `@bc/web` package definition |
| `server/server.go` | bcd server: route registration, static file serving |
| `server/embed.go` | `//go:embed web/dist` for production serving |
| `server/ws/hub.go` | SSE hub: subscriber management, event broadcast |
| `server/handlers/*.go` | REST endpoint handlers (one file per resource) |
| `docs/explanation/design-system.md` | Solar Flare design system specification |
| `docs/explanation/tui.md` | TUI architecture (parallel reference) |
