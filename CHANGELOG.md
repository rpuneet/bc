# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.4] - 2026-07-03

Direction correction. Two decisions surfaced during the v0.3.3
post-release audit:

1. **Workspace is a property on the agent, not a route tenant** —
   every SPA page is served at a flat top-level path. Global pages
   (costs, tools, secrets, gateways, settings) carry no workspace
   concept at all.
2. **Notifications is a one-way inbound stream** routed to subscribed
   agents — not a chat surface with reactions or persistent history.

### Changed
- **Flatten SPA routes** — dropped the `/w/<workspaceId>/…` URL
  segment across the entire web UI. `/live`, `/agents`,
  `/agents/:name`, `/notifications`, `/notifications/:src`,
  `/templates`, `/tools`, `/tools/:provider`, `/cron`, `/secrets`,
  `/stats`, `/metrics`, `/costs`, `/code`, `/settings`, `/about` all
  render at their bare paths. Workspace switching is now a server-side
  `POST /api/workspaces/<id>/activate` triggered by the dropdown; the
  URL never changes. `WorkspaceContext` shed its route guard +
  redirect helpers; server-side `LegacyUIScope` (which used to rewrite
  flat paths → `/w/<id>/…`) deleted.

### Removed
- **`server/legacy_scope.go`** and its test.
- **`web/src/context/WorkspaceContext.test.tsx`**, **`web/src/components/WorkspaceDropdown.test.tsx`**,
  **`web/src/views/AgentDetail.test.tsx`** — all asserted the old
  `/w/<id>/…` behavior.
- **`useWorkspacePath`**, **`ActiveWorkspaceGuard`**,
  **`RedirectToActiveWorkspace`** — no longer needed with flat routes.

### Reverted
- **Emoji reactions in notifications** (#3075, #3226 batch 12) —
  wrong direction. Notifications don't need reactions, thread state,
  or a chat rendering layer. The `reactions` column, `extractReactions()`
  helper, `MessageReaction` type, `SaveMessage`/`GetMessages` reaction
  plumbing, `legacyChannelHistory` reaction emission, `client.ts`
  `ChannelReaction` type, and the (already-dead) `MessageList.tsx`
  reaction rendering are all gone. Half-migrated installs keep an
  orphan SQLite column harmlessly — no destructive drop.

## [0.3.3] - 2026-07-03

Seven-batch continuation of the v0.3.2 design review pass — chart
palette rework + dash-pattern a11y, interactive chart legend, emerald
accent overuse audit, AgentArea extraction with scrim/scrollbar
tokens, agents page grouped by workspace (#3076), emoji reactions
end-to-end (#3075), and #3178 phase 2 landing as a direct-API
outbound cookbook rather than mycel-side wrapper tools.

### Added
- **Emoji reactions** (#3075, #3226 batch 12) — end-to-end pipeline for
  Slack + Discord reactions. New `reactions TEXT` column on
  `notify_messages` stores a JSON-encoded `[{name, count}]` array;
  `extractReactions()` in `pkg/notify/service.go` parses both Slack
  (`reactions[].name`) and Discord (`reactions[].emoji.name`) shapes.
  `legacyChannelHistory` handler surfaces them in the JSON response;
  `MessageList.tsx` renders per-message reaction pills.
- **Agents page grouped by workspace** (#3076, batch 11) — one card per
  workspace, agents grouped underneath with role + status chips.
- **Interactive chart legend** (batch 9) — click a series to solo it,
  shift-click to toggle multiple. Keyboard-accessible.
- **Outbound cookbook** (#3178 phase 2, batch 13) — per-agent Slack /
  Telegram / Discord / WhatsApp outbound documented as a direct-API
  pattern: agents load bot tokens from their own `env.json` (with
  `${secret:NAME}` refs) and call each platform's official REST
  endpoint (`chat.postMessage`, `sendMessage`, webhook, local WhatsApp
  daemon) with per-agent `username` / `icon_emoji`. Full recipe in
  `docs/architecture-notifications.md#outbound-cookbook`. No
  mycel-side wrapper tools — daemon stays out of platform auth /
  rate-limit / retry.

### Changed
- **Chart palette + dash-pattern a11y** (batch 8) — repalette every
  Recharts series to the tokenized 6-swatch mycel palette; add dashed
  stroke patterns so series remain distinguishable in monochrome /
  color-blind settings.
- **Emerald accent overuse pared back** (batch 10) — audit + swap of
  gratuitous emerald usage in favor of neutral tokens; success/status
  emerald reserved for actual success semantics.
- **`AgentArea` extracted** (batch 7) — pulls the agent detail panel
  out of the workspace shell; new `--mycel-scrim` + scrollbar tokens
  cascade across theme variants.

### Deprecated
- **`POST /api/gateways/{platform}/channels/{channel}/send`** — RFC 8594
  `Deprecation` + `Sunset` headers on every response. Will return
  `410 Gone` in v0.4.0. New integrations use the per-agent outbound
  cookbook.

### Docs
- **Notification architecture doc rehabilitated** — renamed
  `docs/_architecture-notifications.md` → `docs/architecture-notifications.md`,
  wired into mkdocs nav under Explanation, and fixed the stale
  `../architecture/notifications.md` cross-links in
  `docs/explanation/networking.md`, `docs/reference/api-rest.md`,
  `docs/README.md`, `docs/index.md`, and the notifications how-to.
  `bc <verb>` → `mycel <verb>` sweep on the how-to (channel names
  like `github:bc` / `slack:all-bc` left alone).

### Fixed
- **Gateway channel double-prefix** (#3219, #3220) — inbound Slack
  messages were being stored under `slack:slack:general` due to a
  double-prefix silently added by the gateway handler. Defensive
  prefix-strip fixes new writes; phantom rows purged from the
  workspace DB. Restores agent subscription routing for existing
  Slack channels.

## [0.3.2] - 2026-07-03

Six-batch UI polish pass on top of v0.3.1 driven by the 5-lead design
review (design / a11y / dataviz / UX / FE-eng) tracked in #3205, plus
one server-side accuracy fix.

### Added
- **Global keyboard focus ring** — WCAG 2.4.7. Two new tokens
  `--mycel-focus-ring` + `--mycel-focus-ring-offset` in `tokens.css`
  cascade from the theme accent; every interactive element gets a
  visible ring on `:focus-visible`. `:focus` still resets to `none`
  so mouse focus stays clean (#3205 batch 5, #3216).
- **`/api/health` returns both `version` and `commit`** so About-page
  update-detection can compare like-with-like (#3212, #3213).
- **About-page `DEV BUILD` chip** for dev builds whose version string
  matches the Makefile `YYYY.MM.DD.<sha>` format. `UPDATE AVAILABLE`
  only fires when both installed and latestTag look like semver AND
  differ (#3212, #3213).

### Changed
- **Theme-aware `agentColor()`** — hash bucket 0 uses `var(--mycel-accent)`
  so the most-prominent chart series follows the theme accent
  (emerald under Dark, tangerine under Solar Flare / Light) instead
  of always the same hashed hue (#3205 batch 5, #3216).
- **`server.BuildInfo.Version`** wired from the same ldflag `main.version`
  the CLI already uses. About INSTALLED reads semver instead of the
  commit hash (#3212, #3213).
- **About release date** reformatted from `03/07/2026, 13:15:41`
  (ambiguous DD/MM/YYYY) to `3 Jul 2026 · 13:15` (#3213).
- **Sidebar footer theme toggle icon** swapped from a sun-with-rays
  (visually collided with the Settings gear at 14px) to a half-shaded
  circle. Trailing `Solar Flare` label dropped — icon + tooltip
  suffice, and the label was overflowing on narrow sidebars (#3213).
- **Chart fill opacity** `0.12 → 0.20` (+ stroke width `1.5 → 1.75`)
  on 9 non-CPU Area series so Network / Disk / Token panels render
  visibly at small-magnitude values (#3213).
- **Live page** — dropped the pulse dot next to the `Live` title;
  the sidebar `Live` pulse + the header `Connected/Reconnecting`
  chip already carry the SSE signal. Third indicator was noise
  (#3205 batch 2, #3213).
- **Metrics range picker** moved from a floating full-width sub-row
  into the page header actions slot. Reclaims ~80px of dead space
  (#3205 batch 2, #3213).
- **Agents-table sort affordance** — every header gets a neutral `▾`
  glyph on hover; active column shows `▲`/`▼` in the accent. Was
  arrow-only on Cost, implying only Cost was sortable
  (#3205 batch 2, #3213).
- **Live filter dropdowns** now have visible `AGENT` / `EVENT` captions
  plus explicit `aria-label`s; `All` disambiguated to `All types`
  (#3205 batch 3, #3214).
- **State column color coding** — `working` → success (green),
  `idle` → warning (amber), `running` → info (cyan), `stuck` →
  warning, `error` → error, else muted. Was one green for three
  meaningfully-different states (#3205 batch 3, #3214).
- **Infra rows show `system`** in the state column instead of an
  empty cell or the literal `unknown` string (#3205 batch 3 + 5,
  #3214 + #3216).
- **Metrics duplicate agent legend** below the table removed —
  swatches + names were already in the Name column one row up.
  Interactive legend deferred to a later batch (#3205 batch 3,
  #3214).
- **Live event row simplification (P1c)** — dropped the bordered
  `h-5 w-5` container around `<ToolDot />` on individual rows;
  tool verb bumped to `font-semibold` and metadata dimmed; on
  aggregated rows `avg` / `total-duration` / token count /
  `<succ>/<total> ok` all move behind the expand chevron so the
  at-rest row reads as a scannable line. Aggregate status dot goes
  amber when there are any failures (#3205 P1c, #3215).
- **Muted-text contrast audit** — every `text-mycel-muted/40` +
  `/50` bumped to full `text-mycel-muted`. `/60` bumped where paired
  with `text-[10px]` or `text-[11px]` on the same span. WCAG AA at
  small sizes (#3205 P2, #3217).
- **Border opacity system** — `border-mycel-border/20` and `/30` (both
  effectively invisible on dark surfaces) consolidated to `/40`.
  Pair is now `border-mycel-border` (strong) + `/40` (soft).
  11 files touched (#3205 batch 6, #3217).

### Fixed
- **About page `UPDATE AVAILABLE` false positive** — comparing the
  daemon's commit hash to the release tag semver never matched.
  Now compares like-with-like via the new `/api/health` `version`
  field, and renders a `DEV BUILD` chip for source builds
  (#3212, #3213).
- **Robust `system` label for infra rows** — batch 3 only rewrote
  `state === "unknown"`; the `server` container comes through with
  a blank state string and rendered as an empty cell. Falsy states
  now fall to the italic `system` label (#3205 batch 5, #3216).

### Deferred to v0.3.3 (or later)
- Interactive chart legend (hover-to-highlight, click-to-toggle).
- Chart palette rework for 6+ distinct series that survive both
  dark and light backgrounds.
- Dash patterns / markers / direct labels on chart lines (a11y —
  color-only encoding still).
- `<AgentArea />` component extraction (fe-eng cleanup).
- `bg-black/50` scrim + `rgba(255,255,255,0.04)` scrollbar tokens.

## [0.3.1] - 2026-07-03

### Renamed
- **Docker image tags**: `bc-agent-*` → `mycel-agent-*` (base/claude/gemini/codex/cursor/infra). Every agent build target dual-tags the produced image so pre-v0.3.1 configs that still reference `bc-agent-*` keep working for one release cycle (#3187 pt 1, #3197).
- **tmux session + Docker container prefix**: `bc-<hash>-<name>` → `mycel-<hash>-<name>`. Reader-side legacy fallback (`Manager.HasSession` / `Manager.ListSessions` / `Backend.ListSessions`) still finds `bc-<hash>-*` sessions and containers, so upgraded workspaces don't lose pre-existing agents (#3187 pt 2, #3198).

### Added
- **RFC 8594 Deprecation headers** on `POST /api/gateways/{platform}/channels/{channel}/send` — signals the endpoint's sunset (target v0.4.0) so callers can migrate to per-agent platform SDKs. See #3178 for the tracking epic (#3201).
- **Theme tokens** — `--mycel-accent-fg`, `--mycel-text-2`, `--mycel-live` per theme, wired through `tailwind.config.ts` so the brand tile stays readable across Solar Flare, Dark, and Light and the pulsing "live" indicator follows the theme (#3202).

### Changed
- **Web: consolidated 6 divergent time-format helpers** into `web/src/utils/time.ts` (`formatRelative` / `formatAbsolute` / `formatDuration`); 16 new colocated tests (#3181, #3199).
- **Web: header + drawer alignment** — both use 48px min-height. Active nav item now uses `bg-mycel-surface-hover`; section labels + workspace caption bumped from 9px @ 40% opacity to 10px muted (#3203, #3202).
- **Web: sidebar flattened** — `GLOBAL`/`SYSTEM` section headers removed. Costs merged into main nav; Settings moved to footer next to About + theme toggle (#3205 P1a, #3208).
- **Web: workspace pill removed from header** on interior pages. `WorkspaceDropdown` stays mounted in a visually-hidden slot so `Cmd/Ctrl+Shift+W` still opens the switcher (#3205 P1b, #3209).
- **Web: `(show) stopped` promoted** to an explicit `☐ Include stopped (N)` toggle pill with `aria-pressed` (#3205 P1b, #3209).
- **Web: KPI row hierarchy** — Working at 26px, other cells 20px; Errored column hidden when 0; zero-value cells dimmed. Semantic tokens replace hard-coded Tailwind hues (#3205 P1d, #3210).
- **Web: chart palette follows the theme accent** — replaced hard-coded solar-flare tangerine `#FF6B35` with `var(--mycel-accent)` in every chart (#3204).
- **Web: `$0.00` cost cells muted** — accent only when `cost > 0` (#3205 P0.1, #3206).
- **Web: CSS namespace sweep** — `Stats.tsx` + `StatsTab.tsx` corrected from incorrect `var(--color-mycel-*)` to canonical `--mycel-*` (#3205 P0.3, #3206).

### Fixed
- **Metrics CPU chart rendered empty despite data.** Two independent bugs: (a) `server/stats_collector.go` still keyed `isAgentContainer` / `isSystemContainer` / `extractAgentName` on the `bc-` prefix so docker-runtime agents stopped being classified post-rename; (b) `pivotAgentMetric` left `undefined` for agents with sample gaps, collapsing the stacked area to the baseline. Both fixed + unit tests (#3182, #3200).
- **CPU chart y-axis + tooltip** — un-stacked, auto-scale with `min 5%`, per-agent tooltip (#3205 P0.2, #3207).
- **npm CD pipeline** — `--allow-same-version` on `npm version` so the tag drives the publish, not the working tree (#3196).
- **`Layout.tsx` hex fallbacks** — 15 inline styles dropped their `#rrggbb` fallbacks; the tokens are always defined so the fallback was silently painting Solar Flare hex under Dark for the first paint (#3205 P0.4, #3206).
- **Stats INFRA filter** — includes `mycel-*` prefixes alongside legacy `bc-*` (#3205, #3206).
- **`Live` SSE indicator** — dot color tokenized (`bg-mycel-success/warning/error`) instead of raw Tailwind hues (#3206).
- **`GatewayFeed` hover states** — five `hover:bg-white/[…]` sites invisible under Light theme; now `hover:bg-mycel-surface-hover` (#3202).

### Deferred to v0.3.2
- **#3178 phase 2** — per-agent outbound integration. Landing in v0.3.3 as a docs-only cookbook (env.json + direct `chat.postMessage` / `sendMessage` / webhook curl recipes), not as mycel-side wrapper tools — keeps the daemon out of the platform auth / rate-limit / retry business and lets agents hit each platform's official REST API directly.
- **#3205 P1c** — Live event row visual simplification (collapse dual status dots, defer avg/ok-ratio behind expand).
- **#3205 P2 + P3** — chart palette rework, interactive legend, global focus-ring token, remaining opacity sprinkle cleanup.

## [0.3.0] - 2026-07-02

### Renamed
- **State root**: `~/.bc/` → `~/.mycel/`. Existing installs are migrated silently on the first invocation of any `mycel` command — no user action required. `~/.bc/` still resolves as a read-only fallback if the migration cannot run (permissions, etc.), so no state is silently lost (#3184).
- **Environment variables**: `MYCEL_HOME` / `MYCEL_STATE_DIR` are canonical. `BC_HOME` / `BC_STATE_DIR` are still honored, with a deprecation warn.
- **User-facing CLI copy**: every help / error / example string that referenced `bc <verb>` is now `mycel <verb>`; `bc CLI` / `bc server` / `bc daemon` prose rebranded (#3185).
- **Web UI + docs**: page title, SetupWizard onboarding steps, GatewayFeed composer chip, MCPServerList description, top-level docs, tutorials, and current-tense explanation pages all say "mycel" (#3186).
- Deferred to v0.3.1: `bc-agent-*` Docker image names and `bc-<hash>-<name>` tmux session prefix — both need coordinated backward-compat migration (#3187).

### Added
- **Pi provider** — new agent tool option, wired end-to-end (backend + web modal) (#3135).
- **About page** (`/about`) — installed version, dist-channel availability, live daemon health checks (#3137).
- **Live page activity hydration** — cards pre-fill from the persisted event store on mount; no longer empty on reload (#3138, #3164).

### Changed
- **Workspace-as-property (RFC #3079)** — every workspace-scoped `/api/*` route now resolves scope via `X-BC-Workspace` header or `?workspace=<id>` query param. The `/api/workspaces/{id}/<rest>` path-rewrite is deleted; only registry self-routes (`/api/workspaces`, `…/{id}`, `…/activate`, `…/discover/*`, `…/clone`) remain (#3147, #3148, #3149, #3150).
- Live page scroll uses pinned anchor (`overflow-anchor: none`, `scrollbar-gutter: stable`) so card updates no longer jump the viewport (#3139, #3140).
- `AgentDetail` no longer stacks two headers — the global `LayoutHeader` is suppressed entirely on that route (#3129).
- Docs site (mkdocs) nav resynced with actual tree; Pygments pinned to 2.17.2 to unblock GitHub Pages build (#3130, #3131, #3132, #3133).
- Landing `/pricing` and `/waitlist` now render `Nav` + `Footer` (#3078, #3167).

### Fixed
- Fresh install + `mycel init` followed by any command errored with "not in a mycel workspace". `workspace.Init` now surfaces registry-save errors instead of swallowing them, and `workspace.Find` self-heals by probing `~/.mycel/workspaces/<id>/preferences.json` for the walked directory (#3173).
- Inter-agent DMs silently no-op'd — `POST /api/agents/{name}/send` now uses `DisallowUnknownFields` and rejects empty message bodies with 400 instead of typing an empty string into the target session (#3174).
- Agent state wedged at `starting` — `AgentService.Send` now reconciles both `stopped` and `starting` when a live session is detected (#3175).
- Agent env vars silently dropped — `EnvFile` column was never persisted to SQLite, so `${secret:NAME}` refs (including AWS Bedrock keys) failed to load. Re-save env from the UI/API after upgrading to recover (#3136).
- Provider command override was dead code — `agent.Start` now actually reads `wsCfg.GetProvider(toolName).Command` (#3141, #3147).
- `go install github.com/rpuneet/mycel/cmd/mycel` was broken because `//go:embed web/dist` had nothing to embed on a fresh checkout. Track `server/web/dist/placeholder.txt` in git (#3036, #3165).

### Removed
- Dead `mycel env` command and hidden `tool check` alias (#3143, #3146).
- Historical `/api/workspaces/{id}/<rest>` path-scoped route family (see workspace-as-property above).

### Deferred to v0.3.1+
- npm Trusted Publishing (OIDC) migration for `cd-npm.yml` — pending Trusted Publisher registration on npmjs.com (#3166).
- Gateway → per-agent platform-tool rearchitecture: outbound Slack / Telegram / Discord / WhatsApp will move from `POST /api/gateways/{platform}/…/send` into per-agent tools using the official platform APIs. Also fixes the recurring "mycel gateway" attribution issue on Slack file uploads (#3178).
- Docker image names `bc-agent-*` → `mycel-agent-*` and tmux session prefix `bc-<hash>-<name>` → `mycel-<hash>-<name>` — deferred so v0.3.0 doesn't break existing agent sessions on upgrade (#3187).
- Major dependency bumps: React 19 (#3163), Vite 8 (#3155), Tailwind 4 (#3161), react-router 7 (#3159), ink 7 (#3152), slack-go 0.27 (#3168), landing eslint 10 (#3169), landing lucide 1.22 (#3170).

## [0.2.3] - 2026-05-02

### Added
- SDK runner skeleton (`packages/mycel-agent-runner`) — Phase 1 of the Claude Agent SDK migration; thin HTTP wrapper exposing one Claude agent over REST + SSE (#2990).
- Mycel-branded landing site at mycel.dev (#3004).

### Changed
- Rebranded `bc` to `mycel` across the binary (`bin/mycel`), release tarballs (`mycel_*`), npm package (`mycel-cli`), Go module (`github.com/rpuneet/mycel`), and install paths (#3053, #3059, #3060, #3062).
- Restored the original particle background animation on the landing site (#3055).
- Backend cleanup: SOLID refactor of server services, dedicated error types, config split, repo_root plumbing, and broad context propagation across notify/cron/tool/doctor/provider/agent stores (#3038, #3046).
- Test isolation: integration tests in `internal/cmd` now use an in-process `httptest.Server` instead of a live bcd, eliminating cross-test interference (#3056).
- CI: relaxed conventional-commits regex to allow `deps(...)` prefix (#3051).
- Dependency bumps across web/tui/landing: TypeScript 6, ESLint 10, react-router-dom 7, GitHub Actions group, @types/node, and lockfile regeneration to unblock CD (#3018, #3020, #3021, #3022, #3023, #3024, #3025, #3028, #3039, #3050).

### Fixed
- MCP sender spoof vulnerability and SSE CORS wildcard tightened (#2967, #2960, #3048).
- MCP tool input fields now have length caps to prevent abuse (#2961, #3045).
- Cron commands run in their own process group so they cannot accidentally signal-kill bcd (#2964, #3052).
- `useEffect` dependency for `selectTab` in `AgentDetail` corrected (#3044).
- Stale `RevealSection` import and vitals TODO removed from landing (#3043).

### Security
- MCP sender spoof patched (#2967).
- SSE CORS wildcard replaced with scoped allowlist (#2960).
- MCP tool input length caps mitigate resource-exhaustion abuse (#2961).
