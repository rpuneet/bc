# ADR 0001: CLI architecture — API-backed subcommands, single Go binary

- **Status:** Accepted — implemented; superseded in part by later work
- **Date:** 2026-04-17
- **Issue:** #3002
- **Related:** #3000 (bc tunnel), PR #3003 (agents revamp)

> **Outcome note (2026-07):** the API-backed single-binary direction shipped.
> Reality diverged from two details below: the terminal UI was removed
> entirely rather than rebuilt in bubbletea (the embedded web UI is the only
> rich surface), and the workspace registry / `~/.mycel/settings.json` layout was
> replaced by the entity-scoped `~/.mycel` home (`prefs.json`, one `mycel.db`).
> The text below is preserved as the historical record.

## Context

The `mycel` binary currently mixes three responsibilities: CLI command handling, TUI rendering (via an embedded React bundle), and hosting the `the daemon` daemon. Subcommands are inconsistent — some go over HTTP to `the daemon`, some reach into `pkg/` packages directly, some poke the filesystem. As the surface grows this drift becomes painful:

- CLI and web UI can diverge because they don't share code paths.
- Remote access (#3000 bc tunnel) is blocked on any CLI subcommand that doesn't already speak HTTP.
- Tests for CLI behaviour need a full workspace on disk instead of a `httptest.Server`.

This ADR locks in the approach so subsequent PRs can execute it phase-by-phase.

## Decision

### 1. Process lifecycle owns `~/.mycel/settings.json`

`mycel up`, `mycel down`, `mycel restart` are the **only** commands that don't require a running `the daemon`. They read user preferences from `~/.mycel/settings.json`:

- default addr / port (pairs with `~/.mycel/daemon.addr` writer, #43)
- default workspace
- autostart flag
- per-dep toggles (mycel-db, mycel-code-server)

These commands manage the daemon's lifecycle, so talking to its API is a chicken-and-egg problem. Every other subcommand assumes `the daemon` is running.

### 2. All other subcommands go over HTTP

Agent lifecycle, cost queries, templates, secrets, MCP, workspace registry, code browsing, deps status, stats — every non-lifecycle `mycel <subcmd>` routes through `http://127.0.0.1:<port>/api/...` using `pkg/client`. Rationale:

- **Single source of truth.** Handlers are exercised by the web UI, CLI, and (later) `mycel tunnel` against the same code path.
- **Remote-friendly.** Once `mycel tunnel` exists, the same CLI works against a remote the daemon with a single env var or flag.
- **Testable.** CLI tests run against `httptest.Server` instead of constructing workspaces on disk.

### 3. Render in Go, not Node

Stay in pure Go with [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss) + [bubbles](https://github.com/charmbracelet/bubbles) for interactive pieces (streaming agent attach, spinners, progress bars). Reject Ink despite the web-UI component-sharing appeal because:

- One 25 MB static binary keeps the install story (`brew install bc`, `curl | sh`, docker) trivial.
- Node runtime dependency or bundling with `pkg`/`nexe`/`sea` adds install friction and a second language in the release matrix.
- bubbletea is proven (gh, glow, soft-serve) and covers the interactive surface we actually need.

The current React TUI bundle (`internal/cmd/tui-bundle/`) will be **deprecated** once the Go bubbletea TUI reaches feature parity. Not before — it still renders the live dashboard today.

### 4. Single entry point, no `cmd/daemon`

- `cmd/mycel` is the only binary. `mycel daemon run` is the blessed way to launch the server.
- `cmd/daemon` does **not** exist and should not be added.
- Handlers live under `server/handlers/` and are shared by the embedded web UI, CLI HTTP client, and `mycel tunnel`.

### 5. `--json` everywhere; `--help` from a meta endpoint

- Every subcommand accepts `--json` to emit raw API responses unmodified.
- `--help` text is generated from `GET /api/_meta/cli` (served by the daemon) so the CLI help stays in sync with server capabilities. When the daemon is unreachable, fall back to the baked-in help string compiled into the binary.
- `--help --json` emits a JSON Schema-style spec of the subcommand's flags and arguments.

## Open questions — resolved

| # | Question | Decision |
|---|---|---|
| Q1 | Should `mycel up` require running the daemon, or start it? | **Start it.** `mycel up` is a lifecycle command (§1) — it writes `~/.mycel/daemon.{pid,log,addr}` and begins the listener. |
| Q2 | the daemon-unreachable fallback for non-lifecycle commands? | **Clear error + hint.** `mycel agent list` with the daemon down exits non-zero with `the daemon is not running — start it with 'mycel up'`. Do not auto-start (surprising side effect) and do not serve from disk read-only (adds a second code path that can drift). |
| Q3 | Does `--json` apply to `--help`? | **Yes.** `mycel agent send --help --json` emits the JSON Schema for the subcommand. Useful for shell completion generators and documentation pipelines. |
| Q4 | Does `mycel tunnel` (#3000) piggyback on this? | **Yes.** `mycel tunnel` exposes the same `/api/...` surface over the relay. A remote CLI connects by pointing `MYCEL_DAEMON_ADDR` at the relay URL — no new protocol. |
| Q5 | Keep the React TUI bundle? | **Deprecate after parity.** Keep rendering the dashboard via the current bundle until the Go bubbletea path handles the live view; then drop the Node embed and reclaim ~3 MB from the binary. |

## Migration plan

Phased, each phase shippable independently:

1. **Lifecycle + settings.** Wire `mycel up/down/restart` to `~/.mycel/settings.json`. Landed partially by `1e470524` (daemon.addr).
2. **API-backed subcommands.** One family per PR:
   1. `mycel agent *` — largest surface; biggest win.
   2. `mycel cost *`, `mycel template *`, `mycel secret *`.
   3. `mycel mcp *`, `mycel workspace *`.
   4. `mycel code *`, `mycel stats *` (already server-side only).
   Each PR deletes direct `pkg/` imports from `internal/cmd` and replaces them with `pkg/client` calls. Tests move to `httptest`.
3. **Meta endpoint + `--json`.** Implement `GET /api/_meta/cli` and `--help --json`, update every subcommand's help resolver.
4. **Bubbletea interactive.** Prototype on `mycel agent send` with token streaming + tool-call cards. If the result matches or exceeds the current React Attach tab, plan the rest of the interactive surface.
5. **Retire the React TUI bundle** once phase 4's surface covers the live dashboard.

Guardrail: no phase ships without CI green on `feat/agents-revamp`-style lint and the full test suite.

## Non-goals

- **Rewriting the web UI in bubbletea.** Web stays React. Bubbletea is for the terminal.
- **Porting the daemon to a non-Go language.** `the daemon` stays Go; only the *presentation* layer is under discussion.
- **Generic plugin architecture.** If we need scriptable subcommands later, we add them via `mycel exec <script>` reading from `~/.mycel/plugins/`. Not in scope here.

## Consequences

**Positive:**
- CLI and web UI can't drift — they share one HTTP surface.
- `mycel tunnel` for free on every subcommand once the HTTP migration lands.
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
