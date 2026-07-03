# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- **#3178 phase 2** — per-agent outbound tools (`slack_post`, `telegram_send`, `discord_send`, `whatsapp_send`).
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
