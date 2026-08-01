# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Start

**Prerequisites**: Go 1.25.4+, tmux, golangci-lint, make. For the web UI / landing: Bun. For the desktop app: the Wails CLI.

**Naming convention**: `make <verb>-<runtime>-<component>`
- **verb** = `build` | `test` | `run` | `release` | `install` | `clean`
- **runtime** = `local` (host machine) | `docker` (container)
- **component** = `mycel` | `web` | `landing` | `desktop`
- `go` | `ts` = language aggregates for CI/CD convenience

**Build**
```bash
make build                         # Build everything (local + docker)
make build-local                   # Build local binaries (go + ts)
make build-local-go                # Build all Go binaries
make build-local-mycel             # Build the mycel binary (embeds web UI, server)
make build-local-ts                # Build all TS packages (web + landing)
make build-local-web               # Build web UI → server/web/dist/
make build-local-landing           # Build landing page
make build-local-desktop           # Build desktop app for the host OS (requires wails CLI)
make build-docker                  # Build Docker images (db, daemon, playwright)
make build-docker-daemon           # Build the daemon Docker image
make build-docker-db               # Build mycel-db (unified TimescaleDB) Docker image
make build-docker-agent            # Build default agent image (claude)
make build-docker-agents           # Build all agent images
make build-docker-agent-base       # Build agent base image
make build-docker-playwright       # Build Playwright MCP Docker image
make release-local-mycel           # Build optimized mycel binary (embeds web UI)
```

**Test**
```bash
make test                                       # Run all tests (go + ts)
make test-go                                    # Run Go tests with race detector
make test-go-fast                               # Go tests excluding slow packages (tmux, secret, doctor, internal/cmd)
go test -race -run TestAgentStart ./pkg/agent/  # Run a specific Go test
make test-ts                                    # Run all TS tests (web + landing)
make test-web                                   # Run web UI tests (vitest)
make test-web-e2e                               # Web e2e tests (Playwright, needs a running daemon)
make test-landing                               # Run landing tests
make coverage-go                                # Go coverage report
make bench-go                                   # Run Go benchmarks
```

**Lint & Check**
```bash
make lint                  # Run all linters (go + ts)
make lint-go               # Run golangci-lint
make lint-ts               # Run all TS linters (web + landing)
make fmt-go                # Format Go code with gofmt
make vet-go                # Run go vet
make check                 # Full quality gate (go + ts)
make check-go              # Go quality gate: vet + lint + test
make check-ts              # TS quality gate: typecheck + lint + test
make ci-local              # Full CI pipeline locally
make ci-docker             # Build all Docker images
```

**Run**
```bash
make run-mycel             # Run mycel CLI from source
make run-web               # Run web UI dev server (hot reload)
make run-landing           # Run landing dev server (hot reload)
```

**Utilities**
```bash
make deps-go               # Download and tidy Go dependencies
make deps-ts               # Install all TS dependencies (bun install)
make scan-go               # Run govulncheck
make install-local-mycel   # Install mycel to $GOPATH/bin
make clean                 # Remove all build artifacts
make clean-local           # Remove build artifacts
make clean-deps            # Remove artifacts + node_modules
```

## Architecture

**mycel** is a CLI-first AI agent orchestration system built in Go with an embedded React web UI — the only rich surface (there is no terminal UI). It coordinates AI agents (Claude Code, Gemini, Cursor, etc.) working in isolated tmux sessions or Docker containers with per-agent git worktrees. The daemon is CWD-free: `mycel up` boots the same from anywhere and all state lives under `~/.mycel`.

### Package Layout

- **cmd/mycel/main.go** → entry point, injects version via ldflags, delegates to internal/cmd
- **cmd/gendocs/main.go** → regenerates docs/reference/cli from the Cobra tree
- **internal/cmd/** → all Cobra CLI commands in a single package. Commands are `*Cmd` variables registered via `init()`. Repo-scoped commands resolve state via `getRepo()`/`requireRepo()`; daemon-first commands use `newDaemonClient()`.
- **pkg/** → reusable packages:
  - **agent/** → agent lifecycle, Manager, SpawnOptions, role setup
  - **app/** → the Apps plugin platform: descriptors, registry, instance resolution, vault-backed secrets; `app/builtin` imports the 28 built-in plugins
  - **attachment/** → file attachment handling
  - **client/** → HTTP client for the daemon API
  - **container/** → Docker runtime backend
  - **cost/** → cost analytics computed directly from provider session files (no ledger, no import)
  - **db/** → the single global database (`~/.mycel/mycel.db`, SQLite WAL or TimescaleDB)
  - **deps/** → managed external dependencies (code-server, db containers)
  - **doctor/** → health diagnostics (`mycel doctor`)
  - **events/** → event log store
  - **gateway/** → per-platform adapters (slack, telegram, discord, whatsapp, …) built by app plugins
  - **home/** → the ~/.mycel home: prefs.json config, layout paths, roles, repo discovery
  - **log/** → structured logging
  - **marketplace/** → template/plugin marketplace
  - **mcp/** → MCP registry (user-global mcps.json + DB layer) and client plumbing
  - **names/** → agent name generation
  - **notify/** → notification fan-out: subscriptions, delivery, history
  - **provider/** → AI provider registry (claude, gemini, cursor, …) with capabilities incl. `CostReader`
  - **runtime/** → backend interface (tmux, docker)
  - **secret/** → encrypted vault (`~/.mycel/secrets.vault`), layered repo overrides
  - **stats/** → usage statistics and metrics
  - **template/** → agent templates (global dir + optional override layer)
  - **tmux/** → tmux session management
  - **token/** → token counting
  - **tool/** → tool registry and execution
  - **ui/** → terminal output utilities for the CLI
  - **worktree/** → git worktree management
- **server/** → the daemon's HTTP server, handlers, agent-facing MCP (`/_mcp/{agent}`), SSE hub (`server/ws`), embedded web UI
- **web/** → web UI (React/Vite), embedded into the binary via `server/web/dist/`
- **desktop/** → Wails desktop app wrapping the same web UI
- **landing/** → landing page
- **packages/** → shared TS packages (`packages/mycel-cli` — the npm-published `mycel-cli` wrapper)
- **docker/** → per-provider agent Dockerfiles plus daemon/db/playwright images

### Key Concepts

- **Agents**: Isolated AI assistants with globally unique names. Everything one agent owns lives in its entity dir `~/.mycel/agents/<name>/` — `worktree/` (git worktree), `session/` (provider state), `logs/`, `tmp/`. Deleting the dir deletes the agent's filesystem state. Agents have roles, optional templates, and an avatar/character shown in the web UI.
- **Home**: The single global state root `~/.mycel` (override with `MYCEL_HOME`): `prefs.json` (the one config), `mycel.db` (the one database), `secrets.vault`, `mcps.json`, `tools.json`, `templates/`, `apps/<instance>/`, `logs/`, `run/`. Repos stay pristine — mycel never writes runtime state into them. `pkg/home.Open` bootstraps-or-loads it.
- **Apps**: Plugin integrations with external platforms (28 built-ins). One descriptor per app drives the connect UI and config validation; instances live in `prefs.json` under `apps`, secret fields in the vault as `app:<instance>:<key>`. Served at `/api/apps`.
- **Costs**: Computed on demand from provider sources — providers implementing the `CostReader` capability scan their own session logs. There is no cost database.
- **Runtime backends**: Agents run in Docker containers (default) or tmux sessions, configured via `runtime` in prefs.json.
- **Roles**: DB-backed (roles table) with capabilities and inheritance; role prompts and MCP servers are written into the agent worktree on spawn.

## Implementation Details

### Command Structure
- Single `internal/cmd` package with one file per command group (agent.go, config.go, cost.go, …)
- Cobra framework with `*Cmd` variables and `init()` registration
- Registered top-level commands: agent, channel, completion, config, cost, doctor, down, logs, mcp, notify, secret, stats, status, template, tool, up, version
- Bare `mycel` boots the daemon (if needed) and opens the web UI; non-interactive invocations print help
- Global flags: `-v/--verbose`, `--json`

### Database
- One global database for all stores: `~/.mycel/mycel.db` (SQLite WAL) or TimescaleDB via `DATABASE_URL` / `storage.default`
- Shared handle via `db.Global(cfg)`; stores never open the file twice
- Tables created with `IF NOT EXISTS` per store — no migration framework
- JSON-encoded TEXT columns for list/map fields (roles, provider settings)

### Testing Patterns
- Table-driven tests preferred
- `TestMain()` in `internal/cmd/` and `pkg/agent/` sets up global `RoleCapabilities` and `RoleHierarchy` maps
- Integration tests use `setupIntegrationHome()` (points `MYCEL_HOME` at a temp dir and bootstraps a git repo) and `seedAgents()` helpers
- E2E tests use live tmux sessions (agent_e2e_test.go, channel_e2e_test.go); server e2e tests wire real stores against a temp home (`server/e2e_test.go`)
- Web: vitest for units, Playwright for e2e (`make test-web-e2e` needs a running daemon)

### Error Handling
- Never ignore errors — use explicit handling or `//nolint:errcheck` with justification
- `noctx` linter enforces context.Context propagation through all call chains

## Code Style

- gofmt with -s (simplify)
- goimports with local prefix `github.com/rpuneet/mycel` (import grouping: stdlib, external, local)
- Short receiver names: `h` for Home and handlers, `a` for agent, `m` for manager
- Avoid package-level variables except for cobra commands
- Struct field alignment matters for memory efficiency (govet fieldalignment)

## Linting

Strict golangci-lint config in `.golangci.yml`. Key linters:
- **errcheck**: all errors handled (type assertions too)
- **govet**: enable-all (includes fieldalignment, shadow, etc.)
- **gosec**: security issues (G104 excluded)
- **noctx**: context propagation
- **staticcheck, bodyclose, prealloc, unconvert, misspell, ineffassign, unused**

Exclusions: test file magic numbers, main.go globals.

## Git Conventions

- Branch naming: `feat/`, `fix/`, `docs/` prefixes
- Conventional commits format
- Run `make check` before committing

## Docker Agent Images

```bash
make build-docker-agent            # Build default agent Docker image (claude)
make build-docker-agents           # Build all agent Docker images
make build-docker-agent-base       # Build agent base image
```

## Architecture Patterns

- cmd imports pkg, never vice versa; pkg packages are self-contained
- Home access: `home.Open(rootDir)` / `home.Load(rootDir)` / `home.Find(dir)`
- Agent operations: `agent.NewManagerWithRepo(agentsDir, repoRoot)`, `mgr.SpawnAgentWithOptions(ctx, opts)`
- App plugins: `app.Register(plugin)` in the plugin package's `init()`, one import line in `pkg/app/builtin`
- Use interfaces for loose coupling between packages; optional adapter capabilities are runtime-asserted (`QRPairer`, `messageSender`, …)
