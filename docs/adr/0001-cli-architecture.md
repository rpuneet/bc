# ADR 0001: CLI architecture — API-backed subcommands, single Go binary

- **Status:** Accepted
- **Date:** 2026-04-17
- **Issue:** #3002
- **Related:** #3000 (bc tunnel), PR #3003 (agents revamp)

## Context

The `bc` binary currently mixes three responsibilities: CLI command handling, TUI rendering (via an embedded React bundle), and hosting the `bcd` daemon. Subcommands are inconsistent — some go over HTTP to `bcd`, some reach into `pkg/` packages directly, some poke the filesystem. As the surface grows this drift becomes painful:

- CLI and web UI can diverge because they don't share code paths.
- Remote access (#3000 bc tunnel) is blocked on any CLI subcommand that doesn't already speak HTTP.
- Tests for CLI behaviour need a full workspace on disk instead of a `httptest.Server`.

This ADR locks in the approach so subsequent PRs can execute it phase-by-phase.

## Decision

### 1. Process lifecycle owns `~/.bc/settings.json`

`bc up`, `bc down`, `bc restart` are the **only** commands that don't require a running `bcd`. They read user preferences from `~/.bc/settings.json`:

- default addr / port (pairs with `~/.bc/daemon.addr` writer, #43)
- default workspace
- autostart flag
- per-dep toggles (bc-db, bc-code-server)

These commands manage the daemon's lifecycle, so talking to its API is a chicken-and-egg problem. Every other subcommand assumes `bcd` is running.

### 2. All other subcommands go over HTTP

Agent lifecycle, cost queries, templates, secrets, MCP, workspace registry, code browsing, deps status, stats — every non-lifecycle `bc <subcmd>` routes through `http://127.0.0.1:<port>/api/...` using `pkg/client`. Rationale:

- **Single source of truth.** Handlers are exercised by the web UI, CLI, and (later) `bc tunnel` against the same code path.
- **Remote-friendly.** Once `bc tunnel` exists, the same CLI works against a remote bcd with a single env var or flag.
- **Testable.** CLI tests run against `httptest.Server` instead of constructing workspaces on disk.

### 3. Render in Go, not Node

Stay in pure Go with [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss) + [bubbles](https://github.com/charmbracelet/bubbles) for interactive pieces (streaming agent attach, spinners, progress bars). Reject Ink despite the web-UI component-sharing appeal because:

- One 25 MB static binary keeps the install story (`brew install bc`, `curl | sh`, docker) trivial.
- Node runtime dependency or bundling with `pkg`/`nexe`/`sea` adds install friction and a second language in the release matrix.
- bubbletea is proven (gh, glow, soft-serve) and covers the interactive surface we actually need.

The current React TUI bundle (`internal/cmd/tui-bundle/`) will be **deprecated** once the Go bubbletea TUI reaches feature parity. Not before — it still renders the live dashboard today.

### 4. Single entry point, no `cmd/bcd`

- `cmd/mycel` is the only binary. `mycel daemon run` is the blessed way to launch the server.
- `cmd/bcd` does **not** exist and should not be added.
- Handlers live under `server/handlers/` and are shared by the embedded web UI, CLI HTTP client, and `mycel tunnel`.

### 5. `--json` everywhere; `--help` from a meta endpoint

- Every subcommand accepts `--json` to emit raw API responses unmodified.
- `--help` text is generated from `GET /api/_meta/cli` (served by bcd) so the CLI help stays in sync with server capabilities. When bcd is unreachable, fall back to the baked-in help string compiled into the binary.
- `--help --json` emits a JSON Schema-style spec of the subcommand's flags and arguments.

## Open questions — resolved

| # | Question | Decision |
|---|---|---|
| Q1 | Should `bc up` require running bcd, or start it? | **Start it.** `bc up` is a lifecycle command (§1) — it writes `~/.bc/daemon.{pid,log,addr}` and begins the listener. |
| Q2 | bcd-unreachable fallback for non-lifecycle commands? | **Clear error + hint.** `bc agent list` with bcd down exits non-zero with `bcd is not running — start it with 'bc up'`. Do not auto-start (surprising side effect) and do not serve from disk read-only (adds a second code path that can drift). |
| Q3 | Does `--json` apply to `--help`? | **Yes.** `bc agent send --help --json` emits the JSON Schema for the subcommand. Useful for shell completion generators and documentation pipelines. |
| Q4 | Does `bc tunnel` (#3000) piggyback on this? | **Yes.** `bc tunnel` exposes the same `/api/...` surface over the relay. A remote CLI connects by pointing `MYCEL_DAEMON_ADDR` at the relay URL — no new protocol. |
| Q5 | Keep the React TUI bundle? | **Deprecate after parity.** Keep rendering the dashboard via the current bundle until the Go bubbletea path handles the live view; then drop the Node embed and reclaim ~3 MB from the binary. |

## Migration plan

Phased, each phase shippable independently:

1. **Lifecycle + settings.** Wire `bc up/down/restart` to `~/.bc/settings.json`. Landed partially by `1e470524` (daemon.addr).
2. **API-backed subcommands.** One family per PR:
   1. `bc agent *` — largest surface; biggest win.
   2. `bc cost *`, `bc template *`, `bc secret *`.
   3. `bc mcp *`, `bc workspace *`.
   4. `bc code *`, `bc stats *` (already server-side only).
   Each PR deletes direct `pkg/` imports from `internal/cmd` and replaces them with `pkg/client` calls. Tests move to `httptest`.
3. **Meta endpoint + `--json`.** Implement `GET /api/_meta/cli` and `--help --json`, update every subcommand's help resolver.
4. **Bubbletea interactive.** Prototype on `bc agent send` with token streaming + tool-call cards. If the result matches or exceeds the current React Attach tab, plan the rest of the interactive surface.
5. **Retire the React TUI bundle** once phase 4's surface covers the live dashboard.

Guardrail: no phase ships without CI green on `feat/agents-revamp`-style lint and the full test suite.

## Non-goals

- **Rewriting the web UI in bubbletea.** Web stays React. Bubbletea is for the terminal.
- **Porting the daemon to a non-Go language.** `bcd` stays Go; only the *presentation* layer is under discussion.
- **Generic plugin architecture.** If we need scriptable subcommands later, we add them via `bc exec <script>` reading from `~/.bc/plugins/`. Not in scope here.

## Consequences

**Positive:**
- CLI and web UI can't drift — they share one HTTP surface.
- `bc tunnel` for free on every subcommand once the HTTP migration lands.
- Single static Go binary — no Node runtime in the install path.
- Tests run against `httptest`, no filesystem setup needed.

**Negative:**
- Phase 2 is a lot of mechanical PR churn — ~12 CLI families to migrate.
- Loss of `pkg/channel`-style direct Go integration for speed (the HTTP round-trip adds ~1 ms per call on localhost — acceptable for a human-scale CLI).
- Retiring the React TUI bundle means rebuilding the dashboard in Go. Plan at least one dedicated PR for that.

## References

- Issue #3002 — CLI architecture discussion
- Issue #3000 — bc tunnel
- PR #3003 — agents revamp (ships #43 daemon.addr, precondition for §1)
- `docs/proposals/bc-layout-v2.md` — on-disk layout (interacts with settings.json absorption)
