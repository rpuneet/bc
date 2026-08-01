# Marketplace: from catalog to installer

Status: **design — awaiting sign-off** (Phase 2 of the plugin-era program,
board #39). Companion doc: [templates-bundles.md](templates-bundles.md).

The marketplace today is a good catalog and a weak installer. `pkg/marketplace`
aggregates eight live sources into one browsable list; "install" composes a
free-text message and fires it at an agent, hoping the agent runs the right
CLI command. This doc turns install into a first-class, deterministic,
reversible daemon operation — with trust tiers, consent previews, and receipts.

## Where we are (honest audit)

What ships today:

- **Catalog** (`pkg/marketplace/aggregator.go`): concurrent fan-out over
  MCP registry, GitHub (stars ≥ 1000, topics `mcp-server`/`claude-skill`),
  local mycel templates, Anthropic claude-plugins-official, ClawHub
  (openclaw skills), gemini-cli-extensions, Glama, Smithery. In-memory
  cache, 1 h TTL, stale-on-error, dedupe by ID. Three item types:
  `mcp`, `skill`, `template`.
- **Install** (`server/handlers/marketplace.go`): `POST
  /api/marketplace/install` sends a formatted instruction message to each
  selected agent via `AgentSender` — the same path as a chat message. The
  agent is asked to run `claude mcp add …`, `claude plugin install …`,
  `openclaw skills install …`, or `mycel template import …`.

The warts, named:

1. **Install is fire-and-forget prose.** No record of what was installed,
   where, or whether it succeeded. Uninstall does not exist.
2. **`mycel template import` does not exist** — `internal/cmd/template.go`
   registers only list/show/create/delete. Template installs instruct the
   agent to run a command that fails.
3. **Config clobbering has already bitten us.** `applyTemplate` in
   `server/handlers/agents.go` used to write `.mcp.json` stubs that wiped
   the role-generated MCP config; the fix was a hand-rolled read-merge-write.
   That fix is local to one call site. Nothing prevents the next writer from
   clobbering again — merge semantics must live in one place.
4. **No trust signal.** A 12-star repo from GitHub search renders the same
   card as the official MCP registry entry. `InstallSpec` is whatever the
   source gave us.
5. **Cache is memory-only** — cold daemon boot blocks the marketplace view
   on eight network calls.

## Item taxonomy (what mycel can actually install)

| Type | Installable today? | Install target | P |
|---|---|---|---|
| `mcp` — MCP server | Yes (deterministic) | agent worktree `.mcp.json`, or user-global `~/.mycel/mcps.json` | P0 |
| `template` — agent bundle | Yes (deterministic) | `~/.mycel/templates/` (then create-agent-from-template) | P0 |
| `skill` — Claude skill / plugin | Partially — only via the agent's own CLI (`claude plugin install`) inside its session | agent worktree `.claude/` | P1 |
| `app-preset` — pre-filled Apps connect config | Link-out only. Apps are **compiled-in Go plugins** (`pkg/app/builtin`); the marketplace cannot install code into the binary. A card deep-links to the Apps connect flow with config pre-filled — never secrets. | `prefs.json` `apps` via existing `/api/apps` | P2 |
| `provider-preset` — model/params defaults | Not an install at all; it is a settings PATCH. Deferred until a real need appears. | — | — |

Non-goals: installing Go app plugins at runtime (no dynamic loading — an app
is a PR), paid items, and any hosted mycel-run registry service.

## Registry model

Three source classes, surfaced as a `tier` on every item:

- **`verified`** — the first-party mycel index: a curated
  `marketplace.json` manifest in a mycel-owned GitHub repo (same pattern as
  `anthropics/claude-plugins-official`). Entries are hand-reviewed, carry a
  full install spec (exact command/URL/transport — no guessing), a pinned
  version, and a content hash where the artifact is fetchable. Small by
  design: tens of items, not thousands.
- **`community`** — the existing aggregated registries (MCP registry,
  Glama, Smithery, ClawHub, claude-plugins, gemini extensions, GitHub
  stars-gated). Known indexes, unreviewed content. Provenance shown:
  source registry, repo URL, stars, publisher.
- **`unlisted`** — a raw URL or name the user pastes ("install from URL").
  Rendered with an explicit warning; requires typing the agent name to
  confirm. Exists because power users will do this anyway — better in a
  flow with receipts than in a terminal we can't see.

Indexing and freshness:

- Keep the fan-out aggregator; add an **on-disk cache**
  `~/.mycel/marketplace/cache.json` written after each successful
  aggregate. Boot serves disk cache instantly, refreshes in the background.
  TTL stays 1 h; `?refresh=1` forces.
- The verified manifest is fetched like any other source but never
  stale-dropped: last good copy is always kept on disk.
- Dedupe across tiers prefers the higher tier: a server present in both
  the verified manifest and Glama renders once, as verified.

## Trust and consent: the install plan

The core fix for warts 1–3 is architectural, not per-callsite: **every
install flows through one `Installer` in a new `pkg/marketplace/install`,
and every install is two-phase** — plan, then apply.

```go
// Plan computes exactly what an install would change, without writing.
type Plan struct {
    Item     Item
    Targets  []TargetPlan // one per selected agent, or one fleet-wide target
}

type TargetPlan struct {
    Agent   string       // "" for fleet-wide (user-global) installs
    Changes []FileChange // every file/keys touched, with before/after
    Warns   []string     // "key 'github' already exists with a different command — will keep yours"
}

type FileChange struct {
    Path    string // absolute: ~/.mycel/agents/<name>/worktree/.mcp.json, ~/.mycel/mcps.json, …
    Op      string // "create" | "merge" | "none"
    Diff    string // unified diff of the exact bytes that would be written
}
```

Merge semantics (the applyTemplate wart, fixed once, centrally):

- JSON maps (`.mcp.json` `mcpServers`, env maps): **add-only by default**.
  An existing key is never overwritten; a colliding key with different
  content becomes a `Warn` in the plan, and the UI offers an explicit
  per-key "replace" checkbox. No silent clobber, ever.
- Whole files the item owns (a skill directory, a template file): written
  only if absent, or if the on-disk content still hash-matches what a
  previous install wrote (recorded in the receipt) — i.e. user-edited
  files are never overwritten without the plan flagging it.
- `Plan` is also the consent UI: the install modal renders the diffs and
  warns per agent before the user confirms. `dry_run` is not a flag —
  planning **is** the first request; apply references the plan.

## Install-to-agent, receipts, uninstall

Scope choice at install time: **selected agent(s)** or **fleet-wide**.

- Per-agent installs write only inside the agent's entity dir
  (`~/.mycel/agents/<name>/`) — worktree `.mcp.json`, `.claude/skills/…`.
  Deleting the agent deletes the install, consistent with the entity-dir
  contract.
- Fleet-wide installs write user-global state (`~/.mycel/mcps.json`,
  `~/.mycel/templates/`) and apply to future agents; existing agents get
  it via the same plan/apply flow per agent.

Every apply writes a **receipt** — a new `installs` table in the global DB
(`db.Global`, `IF NOT EXISTS`, same pattern as every store):

```sql
CREATE TABLE IF NOT EXISTS installs (
  id         TEXT PRIMARY KEY,   -- ulid
  item_id    TEXT NOT NULL,      -- "mcp-registry:io.github.foo/bar"
  version    TEXT NOT NULL,      -- pinned version or content hash at install time
  tier       TEXT NOT NULL,      -- verified | community | unlisted
  agent      TEXT NOT NULL,      -- "" = fleet-wide
  changes    TEXT NOT NULL,      -- JSON: [{path, op, keys_added, sha256_after}]
  created_at TIMESTAMP NOT NULL
);
```

Uninstall inverts the receipt: remove only the keys/files the receipt
added, and only where the current hash still matches `sha256_after`
(user-modified → plan warns, asks). Rollback of a bad install is just
uninstall; no snapshotting beyond the receipt.

Agent-native installs (skills via `claude plugin install`) can't be
daemon-written — the provider CLI owns that state. They keep the
message-dispatch path, but the message becomes structured, the dispatch is
recorded as a receipt with `op: "delegated"`, and the agent is asked to
confirm via its MCP (`report_status`) so the receipt can be marked
succeeded/failed. Honest limitation: delegated installs have weaker
uninstall (a delegated uninstall message).

## Integration with Apps and Templates

- **Apps**: marketplace never installs app code. An `app-preset` card
  resolves to "open the Apps connect modal for descriptor X with these
  non-secret config fields pre-filled". Secrets always go through the
  normal vault flow.
- **Templates** ([templates-bundles.md](templates-bundles.md)): a template
  bundle references marketplace items by `id@version`
  (`mcp-registry:io.github.foo/bar@1.2.0`). Creating an agent from a
  template drives the same `Installer` plan/apply per referenced item —
  one merge engine, one receipt trail, whether the trigger was a
  marketplace card or a template.

## API sketch

| Route | Purpose |
|---|---|
| `GET  /api/marketplace?type=&source=&tier=&q=&refresh=` | catalog (existing, + tier filter) |
| `GET  /api/marketplace/items/{id}` | item detail: provenance, versions, install spec |
| `POST /api/marketplace/plan` | `{item_id, agents[], fleet_wide, overrides{}}` → `Plan` (no writes) |
| `POST /api/marketplace/install` | `{plan_id, confirmed_replacements[]}` → receipts; 409 if state changed since plan |
| `GET  /api/marketplace/installs?agent=` | receipts (what's installed where) |
| `DELETE /api/marketplace/installs/{id}` | uninstall via receipt inversion (plan/confirm for dirty files) |

The existing `POST /api/marketplace/install` free-text body is replaced,
not kept alongside — one install path (per the no-legacy rule); delegated
dispatch survives as an internal strategy of the Installer, not a
separate API.

## Delivery plan

- **P0 — deterministic installs + receipts** (~1.5 wk): `pkg/marketplace/install`
  (Plan/Apply/merge engine + tests), `installs` table, MCP + template
  install paths daemon-side, plan-diff modal in the web UI, on-disk
  catalog cache. Kill the fake `mycel template import` instruction.
- **P1 — trust** (~1 wk): first-party verified manifest repo + fetcher,
  tier badges + provenance panel, install-from-URL (unlisted) with
  confirm, uninstall UI, delegated-skill receipts with agent confirmation.
- **P2 — breadth** (~1 wk, after templates ship): app-preset deep links,
  per-item version pinning + upgrade check against the verified manifest,
  fleet-wide → per-agent backfill flow.

## Open questions for Puneet

1. Verified manifest location: a public `rpuneet/mycel-marketplace` repo
   (community PRs possible), or a `marketplace/` dir inside the main repo
   (simpler, but a catalog change means a mycel commit)?
2. Community tier default: show all eight sources by default, or default
   the catalog view to verified+MCP-registry with community behind a
   filter toggle? (Signal vs. breadth on first impression.)
3. Delegated skill installs (P1): acceptable that their uninstall is
   best-effort via agent message, or should skills be deferred entirely
   until a daemon-side skill writer exists?
4. Is fleet-wide install worth having in P0, or is per-agent-only enough
   for the 2-agent fleets we actually run today?
