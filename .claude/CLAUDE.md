# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Start

**Prerequisites**: Go 1.25.4+, tmux, golangci-lint, make. For TUI: Bun.

**Naming convention**: `make <verb>-<runtime>-<component>`
- **verb** = `build` | `test` | `run` | `release` | `install` | `clean`
- **runtime** = `local` (host machine) | `docker` (container)
- **component** = `bc` | `bcd` | `tui` | `web` | `landing`
- `go` | `ts` = language aggregates for CI/CD convenience

**Build**
```bash
make build                         # Build everything (local + docker)
make build-local                   # Build local binaries (go + ts)
make build-local-go                # Build all Go binaries (bc + bcd)
make build-local-bc                # Build bc CLI
make build-local-bcd               # Build bcd server (embeds web UI)
make build-local-ts                # Build all TS packages (tui + web + landing)
make build-local-tui               # Build TUI
make build-local-web               # Build web UI → server/web/dist/
make build-local-landing           # Build landing page
make build-docker                  # Build Docker images (db, bcd, playwright)
make build-docker-daemon           # Build bcd Docker image
make build-docker-db               # Build bc-db (unified TimescaleDB) Docker image
make build-docker-agent            # Build default agent image (claude)
make build-docker-agents           # Build all agent images
make build-docker-agent-base       # Build agent base image
make build-docker-playwright       # Build Playwright MCP Docker image
make release                       # Build release binaries (stripped)
```

**Test**
```bash
make test                                       # Run all tests (go + ts)
make test-go                                    # Run Go tests with race detector
make test-go-fast                               # Run Go tests excluding slow packages
go test -race -run TestAgentStart ./pkg/agent/  # Run a specific Go test
make test-ts                                    # Run all TS tests (tui + web + landing)
make test-tui                                   # Run TUI tests
make test-web                                   # Run web UI tests (vitest)
make test-landing                               # Run landing tests
make coverage-go                                # Go coverage report (60% threshold)
make bench-go                                   # Run Go benchmarks
```

**Lint & Check**
```bash
make lint                  # Run all linters (go + ts)
make lint-go               # Run golangci-lint
make lint-ts               # Run all TS linters (tui + web + landing)
make fmt-go                # Format Go code with gofmt
make vet-go                # Run go vet
make check                 # Full quality gate (go + ts)
make check-go              # Go quality gate: gen + fmt + vet + lint + test
make check-ts              # TS quality gate: lint + test
make ci-local              # Full CI pipeline locally
make ci-docker             # Build all Docker images
```

**Run**
```bash
make run-bc                        # Run bc CLI from source
make run-web                       # Run web UI dev server (hot reload)
make run-landing                   # Run landing dev server (hot reload)
make run-tui                       # Run TUI dev mode
```

**Utilities**
```bash
make deps-go               # Download and tidy Go dependencies
make deps-ts               # Install all TS dependencies (bun install)
make scan-go               # Run govulncheck
make install-local-bc      # Install bc to $GOPATH/bin
make clean                 # Remove all build artifacts
make clean-local           # Remove build artifacts
make clean-deps            # Remove artifacts + node_modules
```

## Architecture

**bc** is a CLI-first AI agent orchestration system built in Go with a TypeScript/React TUI. It coordinates teams of AI agents (Claude Code, Gemini, Cursor, etc.) working in isolated tmux sessions with per-agent git worktrees.

### Package Layout

- **cmd/bc/main.go** → entry point, injects version via ldflags, delegates to internal/cmd
- **cmd/bcd/main.go** → daemon entry point
- **internal/cmd/** → all Cobra CLI commands in a single package. Commands are `*Cmd` variables registered via `init()`. Access workspace via `getWorkspace(cmd)` helper.
- **pkg/** → reusable packages:
  - **agent/** → agent lifecycle, Manager, SpawnOptions, role setup
  - **attachment/** → file attachment handling
  - **channel/** → (legacy, being removed) SQLite-backed inter-agent communication
  - **client/** → HTTP client for bcd API
  - **container/** → Docker runtime backend
  - **cost/** → cost tracking, budgets, import from Claude
  - **cron/** → scheduled task execution
  - **db/** → database utilities and connection management
  - **doctor/** → workspace health diagnostics
  - **events/** → event log (SQLite)
  - **gateway/** → inbound notification adapters for external platforms (Slack, Telegram, Discord, etc.)
  - **log/** → structured logging
  - **mcp/** → Model Context Protocol client/server
  - **names/** → agent name generation
  - **provider/** → AI provider registry (claude, gemini, cursor, etc.)
  - **runtime/** → backend interface (tmux, docker)
  - **secret/** → secret management
  - **stats/** → usage statistics and metrics
  - **tmux/** → tmux session management
  - **token/** → token counting and management
  - **tool/** → tool registry and execution
  - **ui/** → terminal UI utilities
  - **workspace/** → workspace config, roles, state
  - **worktree/** → git worktree management
- **config/** → configuration constants
- **server/** → bcd HTTP server, handlers, MCP, WebSocket/SSE hub
- **tui/src/** → React/Ink terminal UI, compiled to CommonJS in tui/dist/
- **web/** → web UI (React/Vite)
- **landing/** → landing page
- **prompts/** → default role prompt templates
- **docker/** → per-provider Dockerfiles (claude, gemini, codex, aider, opencode, openclaw, cursor)

### Key Concepts

- **Agents**: Isolated AI assistants in tmux sessions, each with own git worktree. Have roles (root, engineer, manager) with capabilities. State in `.bc/agents/<name>/`.
- **Workspace**: Project dir with `.bc/` subdirectory for config, state, logs. Uses settings.json (v2) config format.
- **Channels**: Inbound notification-only gateways connecting external platforms (Slack, Telegram, etc.) to agents. See [docs/architecture/channels.md](docs/architecture/channels.md).
- **Memory**: Per-agent persistent knowledge (experiences, learnings).
- **Runtime backends**: Agents run in either tmux sessions or Docker containers, configured via `[runtime]` in settings.json.
- **Roles**: Defined in `.bc/roles/*.md` with capabilities (create_agents, assign_work, implement_tasks, etc.) and hierarchy.

### Config Generation

Configuration is stored in `settings.json` (JSON format). The `make gen-go` target is currently a no-op.

## Implementation Details

### Command Structure
- Single `internal/cmd` package with one file per command group (agent.go, channel.go, etc.)
- Cobra framework with `*Cmd` variables and `init()` registration
- Root command: opens TUI if workspace exists, prompts init if not, shows help in non-interactive mode
- Global flags: `-v/--verbose`, `--json`

### Database
- SQLite for persistent storage (channels, cost, events) in `.bc/` directory
- Tables created with `IF NOT EXISTS` for idempotency
- JSON encoding for complex data types

### Testing Patterns
- Table-driven tests preferred
- `TestMain()` in `internal/cmd/` and `pkg/agent/` sets up global `RoleCapabilities` and `RoleHierarchy` maps
- Integration tests use `setupIntegrationWorkspace()` and `seedAgents()` helpers
- E2E tests use live tmux sessions (agent_e2e_test.go, channel_e2e_test.go)
- TUI: test exported helper functions and type interfaces, not hooks directly (hooks can't be tested without DOM in Ink)

### Error Handling
- Never ignore errors — use explicit handling or `//nolint:errcheck` with justification
- `noctx` linter enforces context.Context propagation through all call chains

## Code Style

- gofmt with -s (simplify)
- goimports with local prefix `github.com/rpuneet/bc` (import grouping: stdlib, external, local)
- Short receiver names: `w` for workspace, `a` for agent, `c` for channel
- Avoid package-level variables except for cobra commands
- Struct field alignment matters for memory efficiency (govet fieldalignment)

## Linting

Strict golangci-lint config in `.golangci.yml`. Key linters:
- **errcheck**: all errors handled (type assertions too)
- **govet**: enable-all (includes fieldalignment, shadow, etc.)
- **gosec**: security issues (G104 excluded)
- **noctx**: context propagation
- **staticcheck, bodyclose, prealloc, unconvert, misspell, ineffassign, unused**

Exclusions: deprecated queue/beads migration, test file magic numbers, main.go globals.

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
- Workspace access: `workspace.Load(rootDir)` / `workspace.Init(rootDir)`
- Agent operations: `ws.Agents(ctx)`, `agent.Start(ctx, ws, name, role)`
- Channel communication: notification-only inbound via `pkg/gateway` + `pkg/notify`
- Use interfaces for loose coupling between packages
