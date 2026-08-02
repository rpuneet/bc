# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- **Cross-origin writes are refused.** Any page the user had open in another
  tab could call the daemon's API — loopback keeps other machines out, not
  other websites — and `POST /api/tools` accepted an arbitrary `install_cmd`
  that `POST /api/deps/install` then handed to `sh -c`. State-changing
  requests now have to come from an origin the daemon serves, from a loopback
  origin, or from a non-browser client such as the CLI. Reads are unchanged
  (#3470).

### Fixed

- **`mycel channel send` works again.** Every send failed with
  `405 method not allowed`, and so did `mycel agent health`'s alert delivery:
  the Go client posted to `/api/channels/{name}/messages`, a path no route
  serves. `/api/channels/` is handled by the history endpoint, which accepts GET
  only. Both now use `POST /api/channels/send` — the endpoint the web UI and the
  `send_message` MCP tool have been using all along, which is why sending
  appeared to work everywhere except the CLI. An unroutable channel is also no
  longer reported as sent: the endpoint answers `{"sent": false}` without an
  error, and both commands now say so instead of printing success over a message
  that went nowhere (#3487).
- **A blank MCP command no longer takes the daemon down.** The tool health
  check indexed the first field of a stored command without checking there was
  one, so a whitespace-only entry panicked — on a background goroutine, where
  no HTTP recovery middleware can help, and again on every 30-second tick
  because the offending row was still there. The check now reports the tool as
  misconfigured, and the health pass recovers rather than propagating (#3471).
- **Agent-spawned children are no longer exempt from guardrails.** `spawn_agent`
  over MCP had no `template` field, and the guardrail loop skips any agent
  without one, so every child an agent spawned ran with no cost cap and no stuck
  detection — the unattended case the guardrails exist for. It now accepts a
  template and, when none is given, inherits the caller's, so omitting the field
  cannot silently mean "unguarded" (#3472).
- **Optional services are manageable again.** The dependency manager UI existed
  but nothing imported it, so the whole `/api/deps` lifecycle — list, start,
  stop, stream logs — had no entry point, and the Code tab's "Edit in VS Code"
  button could never appear, because it only renders while
  `mycel-code-server` is running. It now sits under Settings → Providers &
  Tools → Optional Services. The code-server URL also derives its host from the
  page instead of hardcoding `localhost`, which broke whenever the UI was opened
  from another machine (#3473).
- **The CLI no longer contacts WhatsApp on every command.** The WhatsApp adapter
  negotiated its protocol version in package `init`, and the package is linked
  into the CLI, so every invocation — `mycel --version` and `mycel --help`
  included — made an HTTP request to WhatsApp's servers and printed a WhatsApp log
  line before doing what was asked. The lookup now happens when the adapter
  actually connects, once per process (#3455).
- **Tool details report real paths and owners.** An expanded CLI tool row
  showed "Path: git" — the configured command name under a Path label — and a
  "Version cmd" box that was just the tool name plus `--version`, both styled
  like inputs waiting to be filled in. The API now separates `path` (resolved,
  absolute) from `command` (configured) and infers which package manager owns
  each tool, following the symlink so a Homebrew binary is attributable. With
  the owner known, the row names the update command that manager would use
  instead of saying "copy the command above" with no command above it, and it
  no longer offers to uninstall OS-provided binaries the backend refuses to
  touch. The Setup card now appears only while setup is unfinished, rather
  than permanently restating that the re-run icon exists (#3482).

## [0.4.4] - 2026-08-02

App readiness. The surfaces that looked finished in 0.4.3 became real:
Providers and Tools genuinely install, update, and report true versions;
setup is Settings revealing itself section by section; external sign-in
links work inside the desktop window; and the stubs that presented
invented data as real are gone.

### Added

- **Progressive-disclosure setup.** Setup is the Settings page revealing
  itself section by section as each one is satisfied — no separate
  `/welcome` wizard, and a re-run icon replays the reveal without
  blanking anything already configured (#3437).
- **Agent orchestration over MCP.** Agents can `spawn_agent`,
  `send_to_agent`, `stop_agent`, and `list_children`, each
  permission-gated and role-checked (#3441).
- **Agent guardrails.** `MaxCostUSD` and `StuckTimeoutMin` are enforced
  for real — runaway agents auto-stop, stuck ones get flagged — instead
  of being config that was only stored and displayed (#3439).
- **Agent activity Timeline** tab, sourced from persisted lifecycle
  events (#3443).
- **GitHub two-way.** Outbound PR/issue comments and commit statuses via
  the OAuth token, plus `GH_TOKEN`/`GITHUB_TOKEN` injection so `gh` and
  `git` authenticate inside agents with no per-agent setup (#3445,
  #3436).
- **One-click sign-in.** A default GitHub OAuth client ships with the
  binary (device flow, no secret) and Gmail's Google client is embedded
  at build time, so both connect with zero setup (#3429, #3438).
- **`mycel app scaffold`** generates an app/gateway plugin skeleton
  (#3444, docs #3446, #3447).
- **MCP health checks in `doctor`** (#3440).
- **Unified header back/forward** control, replacing per-page back
  buttons (#3434).

### Changed

- **Providers & Tools live in Settings** as a list-only section; the
  standalone `/tools` page is gone and per-provider drill-down stays at
  `/settings/providers/:name`. tmux is the recommended and default
  runtime (#3431).
- **Providers surface revamp** — real brand logos, a logo hero on the
  detail page, descriptions, and a cleaner hierarchy in both themes
  (#3450), on top of a comprehensive `ProviderDetail` with sign-in,
  uninstall, models, and version/help commands (#3433).
- **Docs** restructured for accuracy, with a PR docs-freshness check so
  they can't silently drift again (#3430, #3432).

### Fixed

- **Providers/Tools actually work.** Install and update run for real via
  streamed NDJSON instead of echoing a hint; versions render clean
  (`v2.1.205`); CLI-tool detection no longer reports "0 CLI tools" (a
  dropped `type` column); Claude installs with `npm -g`, not `npx`
  (#3424, #3427, #3449).
- **Desktop external links.** Wails never injects `BrowserOpenURL` into
  the daemon's HTTP origin, so GitHub/Gmail sign-in links were dead in
  the app window. External links now route through a loopback-only
  `POST /api/system/open-url` that rejects non-http(s) schemes (#3448,
  #3435).
- **Apps showed only half your connected apps.** A stale hardcoded
  prefix list hid messages from 14 of 28 apps; senders now resolve to
  real identities (#3420).
- **Honest numbers.** Insights no longer invents an 80/20 token split,
  reports live resource usage, and the unrouted `CostsGlobal` view is
  gone (#3421).
- **Live feed.** Pause works for real, event coverage is complete,
  permission rows resolve, and rows title themselves with the tool-call
  description (#3419, #3426).
- **Add MCP** resolves a real server definition instead of writing an
  empty stanza that could never connect (#3422).
- Browser back/forward behaves across the app, with no phantom pages
  (#3425); Templates/Marketplace tell the truth about each other and the
  boot splash has a stall fallback (#3418).
- Accessibility sweep: touch targets, focus rings, label associations,
  and keyboard-reachable controls (#3442).

## [0.4.3] - 2026-08-02

Craft and truth pass over the 0.4.0 platform: the Tools/Providers
manager became real, Live started showing what agents are actually
doing, and outbound messages started looking like the agent that sent
them.

### Added

- **Agent identity everywhere.** A `whoami` MCP tool, a server-rendered
  public AgentCharacter avatar, and real per-agent icons on outbound
  Slack messages (#3408, #3395).
- **Live activity for more providers** — codex and pi via transcript
  tailing (#3404, #3400) — plus pinned running rows, expandable
  history, and unified row controls (#3407).
- **First-run setup wizard** and a branded daemon boot sequence with a
  live readiness stream (#3384, #3396).
- **Gmail app** (inbound + send via the Gmail API) and one-click
  loopback OAuth for Gmail and GitHub connect (#3383, #3392).
- **Real profile photos** for channels and senders (#3401), and
  identity avatars across Apps (#3385).
- **Agent Settings tab** with per-agent CPU/memory budgeting (#3391),
  and a fleet default provider + model manager (#3389).
- **Tools manager**: package-manager autodetect, per-provider
  subcommands, working uninstall, streamed install (#3394, #3403).
- **Direct messaging** to any contact via `<platform>:<id>` (#3380).

### Changed

- Settings slimmed and mirrored to the wizard, with setup folded in
  (#3386); craft passes over Settings, Apps, and Notifications (#3387,
  #3388, #3382).
- Landing heroes replaced with live-recorded product motion (#3402).
- Phase 2 design docs for the marketplace installer and templates as
  bundles (#3399).

### Fixed

- Agent lifecycle events persist, so new agents have a timeline (#3397),
  without double-persisting hook events (#3415).
- The agent raw stream shows tool output and responses again (#3410).
- `cpus`/`memory_mb` columns migrate onto existing databases — a
  release blocker that put running agents at risk (#3393).
- The agent Metrics "By model" panel reads real data (#3398).
- `/settings/tools` remount loop, in-surface CLI install, and wizard
  provider parity (#3390).
- Stale `bc` MCP-server naming in user-facing surfaces (#3409); dev-only
  `preferences.json · v3` badge removed from Settings (#3406).
- Repo hygiene: stray binaries gitignored, dead component directories
  dropped (#3417).

### Security

- `golang.org/x/image` bumped to v0.44.0, clearing 12 CodeQL alerts
  (#3412).

## [0.4.2] - 2026-07-31

### Fixed

- CD injects the release version into the desktop build, so packaged
  apps no longer report a dev version (#3381).

## [0.4.1] - 2026-07-31

### Added

- Message any contact directly via `<platform>:<id>`, without a
  pre-existing channel (#3380).

## [0.4.0] - 2026-07-31

The plugin era. mycel becomes a platform: every integration — the AI
providers that power agents and the apps they talk through — is now a
self-describing plugin, credentials live in the encrypted vault, the
web UI is the product's single rich surface, and a native desktop app
ships for macOS, Linux, and Windows. The fleet got faces.

### Added

- **Apps platform.** Notifications becomes Apps: 28 real integrations,
  each a self-registering plugin (descriptor + build factory) with
  auth-kind-aware connect flows — token, webhook-secret, QR pairing
  (WhatsApp), and OAuth. Credentials never touch config files; they
  live in the vault as `app:<name>:<key>` and inject into agent env.
- **Sign in with GitHub.** RFC 8628 device-flow OAuth in the connect
  wizard — user code, verification link, live polling; the minted token
  vaults as `api_token` and reaches agents as `GITHUB_API_TOKEN`.
- **Living agent identity.** Every agent gets a deterministic
  mycelium-inspired character (form, hue, eyes, marks derived from its
  name) that breathes, blinks, works, droops, and errors with its
  state and pulses on live events — adopted across every surface, the
  drawer agents tree, and the New Agent flow's morphing preview.
- **Desktop app.** A Wails shell that runs the mycel server in-process
  (or attaches to a running daemon), with the web UI in a native
  window and on localhost — packaged for macOS, Linux, and Windows.
- **Code tab.** The agent detail Code tab is a real embedded browser —
  tree, Monaco editor, and diff view pinned to the agent's worktree.
- **Insights.** Rebuilt around four questions — spend trend, where it
  goes, activity, cache efficiency — period-scoped end-to-end, with
  drill-downs: live system vitals (CPU/memory/network/disk), token
  composition, and per-agent/model/repo detail.
- **Per-agent apps.** Choose apps and channel subscriptions in the New
  Agent flow and manage them from the agent's Config tab.

### Changed

- **Entity-scoped home.** `~/.mycel` flattens: one `prefs.json`, one
  `mycel.db`, one vault; each agent owns `agents/<name>/{worktree,
  session,logs,tmp}`; stateful apps own `apps/<name>/`. `mycel up`
  works from any directory. One-shot migration:
  `scripts/migrate-mycel-home.sh`.
- **Source-direct costs.** The cost ledger is gone; costs compute on
  read from provider transcripts via the `CostReader` capability
  (claude implements it), cached in-process. Honest numbers, no drift.
- **Providers self-register** with capability interfaces (commands,
  MCP config, cost reading) — no hardcoded provider switches anywhere.
- **Coffee & cream brand.** Mushroom mark, espresso/porcelain themes,
  Fraunces display accents, and a product-first landing with real app
  screenshots and a proximity-linked mycelial network background.
- `pkg/workspace` is now `pkg/home`; the workspace vocabulary is gone
  from code and docs.

### Removed

- **The TUI.** The web UI is the only rich surface; `mycel` with no
  arguments starts the daemon and opens the dashboard.
- **Cron.** Agents schedule their own work.
- **Placeholder integrations** (nostr, homeassistant) and adapters that
  could not ship honestly (msteams, googlechat, feishu without inbound
  signature validation; twitter behind a paywalled API).
- The standalone Secrets page (now Apps → Custom Keys), the legacy
  `/api/gateways` surface, 45MB of tracked binaries and orphaned
  screenshots, and the `costs.db` ledger.

### Security

- govulncheck clean in called code (Go 1.25.12, x/text 0.39); landing
  and web dependency audits at zero known-vulnerable resolutions
  (one documented N/A: react-router RSC-server CSRF — client-only SPA).

## [0.3.12] - 2026-07-05

One header, real numbers. The web UI converges on a single shared header
every page drives, the analytics page becomes a real dashboard wired to
the cost ledger, and agents can now talk back over WhatsApp.

### Added

- **Outbound WhatsApp.** Agents can reply on WhatsApp — the gateway
  adapter implements `Send()` over the paired whatsmeow session, routed
  by native JID with a bounded send timeout (#3314).
- **Notifications home.** A full notifications hub — connected apps,
  channels, and recent activity in one place (#3311), with WhatsApp
  channels resolved to real display names instead of raw ids (#3312).
- **Resizable drawer.** Drag the drawer's right edge to widen it (bounds
  remembered; double-click resets) so long agent and channel names stop
  clipping (#3317).

### Changed

- **One shared header.** The mycel brand + drawer toggle move into the
  header as a column that resizes with the drawer and collapses to an
  icon rail; every page — agent detail, Tools, Code, Secrets, Insights —
  feeds its title and controls into that single header instead of
  rendering its own bar (#3317, #3319).
- **Insights is a single-page dashboard.** Tabs are gone; a KPI strip
  (spend, tokens, active agents, burn rate, top cost driver) sits above
  grouped Cost / Usage / System / Activity sections with a sticky
  anchor-nav and drill-downs into agents and channels (#3313, #3321).
- **Cost and token analytics read the ledger.** Every cost/token view —
  the dashboard charts, the agents table columns, and the KPIs — now
  sources from the cost ledger (`/api/costs/*`), which carries real
  per-agent, per-model, and daily dollars and tokens, including a cache
  hit-ratio readout (#3321).
- **`BC_*` environment variables are now `MYCEL_*`.** Agent env injection
  and the reserved-prefix guard use the `MYCEL_` namespace (#3308).
- **Provider capabilities.** Hook-event ingestion is extracted behind a
  transport-agnostic seam and providers declare capabilities through
  optional interfaces; agent state is derived from ingested events
  rather than provider-side detection (#3309).

### Removed

- **Dead stats-store token/cost path.** The per-agent token/cost
  timeseries (`/api/agents/stats/{tokens,cost}`, the misrouted collector
  block, and the `token_metrics` table with its query/record code) is
  deleted — it never recorded cost and always returned empty. The cost
  ledger is the single source of truth (#3321).

## [0.3.11] - 2026-07-05

The visual release: the entire web UI rebuilt on a real design system,
plus model and environment as first-class agent parameters.

### Changed

- **Two themes, one palette.** Dark (default) and light, every color an
  exact Radix scale step (Sand neutrals + Orange accent); Solar Flare
  retired with stored preferences migrating. Three-step layered
  elevation, semantic tint tokens for badges — replacing ~200 alpha
  classes that silently compiled to nothing.
- **Typography and component systems.** Real type hierarchy, one radius
  system, primary/secondary/danger buttons, monospace reserved for
  identifiers.
- **Chrome.** Full-width header owning the brand (new mycelium
  connected-nodes mark), per-view controls (summary, wide search,
  Filters chip, primary action), a utility menu (theme/Settings/About),
  and a pure-nav drawer with smooth collapse. Page titles and the
  duplicate drawer toggle are gone.
- **Information-rich agents table.** Icon actions, merged provider·model
  runtime cell, live Activity and real Cost columns (the dead MCP column
  removed), compact repo bands with group cost, activity-feed peek (eye
  toggle) replacing the raw-terminal peek.
- **Live.** Back-to-latest is a chat-style top-center pill matching the
  newest-first feed.

### Added

- **Model as an agent parameter.** Per-provider dropdown in the create
  modal (lists verified against each live CLI), stored on the agent,
  injected with the right flag per provider, preserved across restarts,
  shown in the table and detail.
- **Per-agent environment variables** with `${secret:NAME}` vault
  references — autocomplete in a key/value editor at create and detail;
  references stored, resolved only at spawn; `BC_*` system variables
  protected from both config and env files.

### Fixed

- **Per-agent cost was broken end-to-end**: the client read a field the
  server never sent, and the server looked up the ledger by bare agent
  name while entries key by session name. Costs are real everywhere now
  (table, peek, stats), with exact-candidate matching.
- The Live overflow menu opened invisibly behind the page (stacking
  context); mobile search regressions; agent-detail environment editor
  previously wrote a file nothing read — now store-backed and injected
  at restart.

## [0.3.10] - 2026-07-04

The single-tenant release: the workspace concept is gone. One global
`~/.mycel/mycel.db`, agents bound to a repo instead of a workspace,
flat name-keyed state, and an agent create path that survives every
failure mode we could find.

### Removed

- **The workspace concept.** Per-workspace databases, registries,
  service bundles, idle eviction, `/w/` URLs, and workspace-scoped
  API surfaces are all deleted. bcd is single-tenant: one `Services`
  bundle for the process lifetime, one global database, and `repo` is
  just a column on the agent. Workspace-scoped issues #3241, #3242,
  and #3243 closed as designed-away.
- **The Ralph loop**, replaced by Start/Stop/Restart lifecycle
  controls in the agent header.

### Added

- **Repo as the unit of work.** The create-agent modal grows a
  required Repo field (known-repos dropdown + local discovery), the
  agents list groups by repo, and `GET /api/repos` lists every repo
  with agent counts. Worktrees are checked out from the agent's own
  repo — tmux and docker both.
- **Flat state layout.** New worktrees land at
  `~/.mycel/worktrees/<agent>/` and agent state (claude config, logs)
  at `~/.mycel/agents/<agent>/` — no hash-keyed directories. Existing
  agents keep working from their recorded paths;
  `scripts/migrate-worktree-layout.sh` moves them (dry-run first,
  skips running agents, repairs git worktree links).
- **Trust pre-seeding.** Fresh claude agents no longer hang at the
  interactive "trust this folder" prompt: the worktree is pre-trusted
  in the agent's claude config for both runtimes.
- `GET /api/stats/channels` backs the Metrics notification panel with
  real per-channel message, member, and top-sender data.
- `mycel doctor` flags missing `mycel-agent-*` docker images per
  provider, with the build command.

### Fixed

- **Agents survive daemon restarts regardless of repo.** The manager
  loaded only boot-repo agents at startup, orphaning everything else
  (row and tmux session alive, API blind). It now loads the whole
  global table, and restarts always bind to the agent's own repo.
- **Failed creates roll back completely.** A failed docker create
  could leave a phantom `starting` row that reserved the agent name
  forever; creation now rolls back both the in-memory entry and the
  store row.
- **The bc MCP endpoint is derived, not stored.** A static
  `mcp_servers` row can't be right for tmux and docker at once and
  pinned a stale port — agents now get the live daemon address per
  runtime, and the phantom `tool validation` warning is gone (roles
  also resolve from the global store on every path).
- **Shutdown hygiene.** Gateway adapters get a graceful `Stop` before
  stores close, and in-flight notify dispatches drain instead of
  racing teardown.
- The CLI `--tool` help now derives from the provider registry (the
  honest five), `mycel mcp register` no longer looks up the Unix `bc`
  calculator, and the last `bc` → `mycel` help strings are fixed.

### Documentation

- Every page realigned with the repo-era architecture and verified
  against source — tutorials, how-to, reference, and explanation.
  `mkdocs build --strict` passes clean.

## [0.3.9] - 2026-07-04

The clean-architecture release: `mycel init` is gone (install → `mycel up`
is the whole story), every legacy fallback and in-tree migration is
removed, the security backlog is cleared, and the Live page event stream
was rebuilt around the agent-detail hook-stream UI.

### Removed

- **`mycel init`.** `mycel up` bootstraps everything: run it inside a git
  repo and the repo is adopted automatically; run it anywhere else and
  the daemon serves the web UI with an add-repo flow. The wizard,
  presets, and every "run mycel init" message are gone.
- **All legacy compatibility code.** Home resolution is `MYCEL_HOME` →
  `~/.mycel`, period; `preferences.json` is the only config file
  (settings.json readers, overlay merging, and save-on-read promotion
  deleted); legacy daemon.addr/socket/secret-key/cost-ledger fallbacks,
  tmux/container `bc-` prefixes, the `bc-agent-*` image fallback, MCP
  legacy-URL shims, the `"sql"` storage alias, v1-workspace detection,
  and completed one-shot store migrations are all removed.

### Security

- Resolved 86 CodeQL alerts with real taint barriers: path-injection
  guards across agent/worktree/template/attachment/file handling, three
  command-injection fixes (unvalidated worktree param reaching `git -C`;
  clone URL scheme allowlist with `--` separation), separator-aware MCP
  path containment, `edwards25519` CVE bump, and a bcd base-image
  upgrade clearing ~1,700 stale container-scan CVEs.

### Changed

- **Live event stream v2.** Flat newest-first hook-event rows shared
  with the agent-detail page: rich one-line summaries extracted from
  event JSON (commands, file paths, subagent descriptions, durations),
  monochrome glyphs instead of emoji, no aggregation buckets, stable
  ordering with no reflow, and stale running-timers finalize when an
  agent stops.
- **Notifications drawer revamp.** Apps group their channels with real
  brand icons, connection dots with human-readable failure reasons and
  reconnect actions, discord guild sub-grouping, per-app activity times,
  unread accents, and a channel filter.

### Fixed

- **Multi-workspace data bleed.** Every workspace now has its own database
  connection (per-workspace registry with explicit store handles) — one
  workspace's subscriptions, cron jobs, and events can no longer land in
  another's database, and repos added at runtime come up fully online.
- TUI test suite stabilized: tmux e2e tests run on an isolated socket,
  config tests no longer race a spawned CLI, and the CI job fails fast
  instead of hanging for hours.
- `GithubTokenPath` was the last path still reading `~/.bc`.

## [0.3.8] - 2026-07-04

The "nothing fails silently" release: the storage/config failure class
behind the notifications outage is fixed end-to-end, lifetime costs were
corrected (~5x overstated), the Live page was redesigned, and the entire
documentation set was rewritten from source and now deploys reliably to
bc-infra.com.

### Fixed

- **Storage resilience.** The daemon falls back to SQLite (with a loud
  warning) when the configured TimescaleDB is unreachable, instead of
  silently disabling notify/cron/MCP/tools/events.
- **Config precedence.** A newer `settings.json` now overlays
  `preferences.json` at load (section-replace, per-platform gateway
  deep-merge); the save-on-read promotion that froze stale config is
  gone. `mycel doctor` flags config drift.
- **Cost accounting.** Corrected pricing tiers (Opus 4.5+ was billed at
  the legacy 3x rate; Fable 5 was missing), deduplicated compaction
  sidechain imports (29.7% of records), and separated cache tokens from
  totals — lifetime cost corrected from $109k to ~$22k.
- **Agent state consistency.** Lifecycle hooks no longer overwrite the
  agent's reported task; the Agents table applies state-change events
  live; "Last Active" no longer freezes on busy agents (events query read
  the oldest window); agent-detail tab URLs no longer self-append.
- **Discord channels deduplicated.** Canonical `discord:<guild>:<channel>`
  keys with a one-time migration merging the three legacy naming schemes.
- **Web routing.** Legacy `/w/<hash>/` URLs redirect back to flat routes
  (fixes cached-redirect 404s on refresh/bookmarks, open-redirect safe);
  unknown `/api/*` paths return JSON 404 instead of the SPA shell.
- **IRC adapter** could never connect (integer timeouts read as
  nanoseconds).
- **Security.** Go 1.25.11 clears GO-2026-5039/5037/4971/4918.

### Added

- **Degraded-services surfacing.** `/api/health` reports failed stores
  with reasons, 503s carry the cause, `mycel doctor` gains a daemon
  check, and the web UI shows a degraded banner.
- **Cloudflare Pages deploys from CI** (wrangler) — bc-infra.com tracks
  main again after weeks of stale builds.
- CI guard that fails when the generated CLI reference drifts from the
  Cobra command tree.

### Changed

- **Live page redesigned** around a single presence line, search, and a
  ⋯ menu — the stats grid, dual filter dropdowns, and five-button toolbar
  are gone.
- **Notifications are strictly stream-only**: the message composer, the
  deprecated gateway send endpoint, and the reaction/pin hover
  affordances are removed.
- **Docs rewritten from source**: regenerated CLI reference (133 files),
  REST API reference rebuilt from route registration (196 endpoints),
  config/MCP/tutorials/troubleshoot and 12 architecture docs corrected;
  104MB of internal screenshots removed from the published site.

## [0.3.7] - 2026-07-03

Course-correct on the v0.3.6 notifications rewrite.

### Reverted

- **Restore `GatewayFeed.tsx` + `AgentPeekPanel.tsx`.** The v0.3.6
  `ChannelStream` rewrite over-corrected the notifications-are-a-stream
  arc. The header "N/N agents" dropdown (Listening / Available with
  the per-agent @-mentions vs all-msgs toggle + remove/add) and the
  per-message delivered / not-delivered labels ARE routing
  observability, not chat framing. Only reactions + a
  proxy-back-out composer would violate stream-only, and reactions
  stayed out from v0.3.4. `web/src/views/Notifications.tsx` renders
  `GatewayFeed` again; `ChannelStream.tsx` deleted.

### Added

- **Slack mrkdwn client-side normalization** in `MessageContent`.
  Slack sends messages with raw angle-bracket tokens that rendered
  broken in the UI (`<@U0AP1U92T3K>` for @-mentions,
  `<#C0BAJV8UXLL|general>` for channel refs, `<https://url|label>`
  for links). Rewrite them before the existing tokenizer runs:

      <@USERID>              -> @user
      <@USERID|name>         -> @name
      <#CHANNELID|name>      -> #name
      <#CHANNELID>           -> #channel
      <https://url|label>    -> label (https://url)
      <https://url>          -> https://url

  Full user-ID -> display-name resolution needs a Slack `users.info`
  round trip and belongs on the gateway adapter; this client-side
  pass at least strips the noise.

## [0.3.6] - 2026-07-03

Post-v0.3.5 cleanup pass.

### Changed

- **Delivery log — quiet offline-agent skips.** Sending to a stopped
  subscriber used to be logged as `StatusFailed`, so a channel with
  seven subscribers and six stopped agents rendered an alarming
  `86 failed / 14 delivered`. The routing decision was correct; the
  agent was just offline. Dispatch now detects the pkg/agent
  "agent not running" / "is stopped" error text and skips the
  `LogDelivery` call entirely (debug log only). Failed count on the
  notifications page reflects genuine send errors again. No schema
  change.

### Removed

- **`web/src/components/notifications/GatewayFeed.tsx`** — replaced
  by `ChannelStream` in v0.3.5, no callers remained.
- **`web/src/components/AgentPeekPanel.tsx`** — its only caller was
  GatewayFeed's message-history surface.

## [0.3.5] - 2026-07-03

Follow-through on the workspace-as-property and
notifications-are-a-stream arcs started in v0.3.4.

### Removed

- **WorkspaceDropdown / WorkspacePicker / activate flow** — workspace
  is a property on the agent, not something the user "switches to",
  so a switcher was a UX contradiction. Sidebar chip drops its
  `workspace` subtitle. `Cmd/Ctrl+Shift+W` shortcut retired.
- **`WorkspaceContext.activate()`** — the dropdown was its only
  caller. Server-side `POST /api/workspaces/<id>/activate` remains
  for external callers but is no longer invoked by the SPA.
- **Client `X-BC-Workspace` header** — routing to a workspace uses
  `?workspace=<id>` explicitly when needed (currently only for
  `POST /api/agents` from the new-agent modal).

### Changed

- **Agents page — default group = workspace** (was: repo). Toggle
  reads/writes `mycel-agents-group-by-workspace` in localStorage and
  disables when every visible agent shares one workspace. Group
  header caption is the workspace path (home-collapsed to `~/…`).
- **New-agent modal — required Workspace select.** Populated from
  `GET /api/workspaces`; defaults to the current active workspace;
  submit hits `POST /api/agents?workspace=<selectedId>`. Can't
  create an agent without picking a workspace.
- **Notifications page — replaced `GatewayFeed` chat surface with
  `ChannelStream`.** New page shows:
  1. **Subscriptions** table — subscribed agents, `all messages` vs
     `@-mentions only`, since-when, per-row unsubscribe.
  2. **Delivery log** — most recent inbound-delivery attempts with
     status dot (delivered/failed/pending), agent, error, content
     preview, timestamp. Delivered / failed counts in the header.
  3. **Outbound note** — link to the outbound cookbook, since mycel
     does not proxy replies back to the platform.
  No composer, no message-history threading, no reactions, no
  avatars — this is a routing + observability surface.

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
