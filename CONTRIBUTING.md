# Contributing to bc

Thank you for your interest in contributing to bc! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.25.4+
- Bun (for TUI development)
- tmux
- golangci-lint
- make

### Getting Started

```bash
# Clone the repository
git clone https://github.com/rpuneet/mycel.git
cd bc

# Install dependencies
make deps

# Build the project
make build

# Run tests
make test

# Install locally
make install
```

## Build Commands

Naming convention: `make <verb>[-<runtime>]-<component>` where `runtime` = `local` (host) | `docker` (container), `component` = `mycel` | `tui` | `web` | `landing`. `go` and `ts` are language aggregates for CI/CD convenience.

### Build (local)

| Command | Description |
|---------|-------------|
| `make build` | Build everything (local + docker) |
| `make build-local` | Build all local binaries (go + ts) |
| `make build-local-go` | Build all Go binaries (mycel + the daemon) |
| `make build-local-mycel` | Build mycel CLI binary to `bin/mycel` |
| `make build-local-mycel` | Build the daemon binary (embeds web UI) |
| `make build-local-ts` | Build all TS packages (tui + web + landing) |
| `make build-local-tui` | Build TUI package |
| `make build-local-web` | Build React web UI → `server/web/dist/` |
| `make build-local-landing` | Build Next.js landing page |
| `make release` | Build optimized release binaries (stripped symbols) |
| `make install-local-mycel` | Install mycel to `$GOPATH/bin` |

### Build (Docker)

| Command | Description |
|---------|-------------|
| `make build-docker` | Build all Docker images (db, daemon, playwright) |
| `make build-docker-daemon` | Build daemon Docker image |
| `make build-docker-db` | Build mycel-db (unified TimescaleDB) Docker image |
| `make build-docker-db` | Build mycel-db (TimescaleDB) Docker image |
| `make build-docker-agent` | Build default agent Docker image (claude) |
| `make build-docker-agents` | Build all agent Docker images |

### Test

| Command | Description |
|---------|-------------|
| `make test` | Run all tests (go + ts) |
| `make test-go` | Run Go tests with race detector |
| `make test-ts` | Run all TS tests (tui + web + landing) |
| `make test-tui` | Run TUI tests |
| `make test-web` | Run web UI tests (vitest) |
| `make test-landing` | Run landing page tests |
| `make coverage-go` | Run Go tests with coverage report (60% threshold) |
| `make bench-go` | Run Go benchmarks |

### Lint & Quality

| Command | Description |
|---------|-------------|
| `make lint` | Run all linters (go + ts) |
| `make lint-go` | Run golangci-lint on Go code |
| `make lint-ts` | Run all TS linters (tui + web + landing) |
| `make fmt-go` | Format Go code with gofmt |
| `make fmt-ts` | Format all TS code |
| `make vet-go` | Run go vet |
| `make vet-ts` | Typecheck all TS |
| `make check` | Full quality gate (go + ts) |
| `make check-go` | Go quality gate (gen + fmt + vet + lint + test) |
| `make check-ts` | TS quality gate (lint + test) |
| `make ci-local` | Full CI pipeline locally |

### Run & Deploy

| Command | Description |
|---------|-------------|
| `make run-mycel` | Run mycel CLI from source (`go run`) |
| `make run-web` | Run web UI dev server (hot reload) |
| `make run-landing` | Run landing dev server (hot reload) |
| `make run-tui` | Run TUI in dev mode |

### Utilities

| Command | Description |
|---------|-------------|
| `make deps` | Install all dependencies (go + ts) |
| `make deps-go` | Download and tidy Go dependencies |
| `make deps-ts` | Install all TS dependencies (bun install) |
| `make scan-go` | Run govulncheck for Go vulnerabilities |
| `make scan-ts` | Run TS dependency audit |
| `make install-local-mycel` | Install mycel to `$GOPATH/bin` |
| `make clean` | Remove all build artifacts |
| `make clean-deps` | Remove build artifacts + node_modules |

Or directly with Bun (from `tui/`, `web/`, or `landing/`):

```bash
cd tui
bun install        # Install dependencies
bun run build      # Build to dist/
bun test           # Run tests
bun run lint       # Lint code
```

### TUI Testing

The TUI uses `bun:test` for testing. Key patterns:

**Testing Hooks Without DOM**

React hooks in Ink/terminal environment don't have DOM access. Test exported helper functions and type interfaces instead of hook behavior:

```typescript
// Test helper functions directly
import { getSeverityColor } from '../useLogs';
expect(getSeverityColor('error')).toBe('red');

// Validate type exports
import type { UseStatusOptions } from '../useStatus';
const options: UseStatusOptions = { pollInterval: 5000 };
expect(options.pollInterval).toBe(5000);
```

**Test File Location**

- Hooks: `tui/src/hooks/__tests__/*.test.tsx`
- Views: `tui/src/views/__tests__/*.test.tsx`
- Components: `tui/src/__tests__/components/*.test.tsx`

**Running Specific Tests**

```bash
bun test src/hooks/__tests__/useStatus.test.tsx
bun test --watch  # Watch mode
```

## Code Style

### Linting

We use `golangci-lint` with strict settings. All code must pass linting before merge.

```bash
# Run linter
make lint

# Configuration is in .golangci.yml
```

### Key Lint Rules

- **errcheck**: All errors must be handled
- **gosec**: Security issues must be addressed
- **govet**: No shadowed variables
- **noctx**: Context must be propagated
- **fieldalignment**: Struct fields optimally aligned

### Code Guidelines

1. **Error Handling**: Never ignore errors. Use explicit handling or `//nolint:errcheck` with justification.

2. **Context Propagation**: Pass `context.Context` through all call chains.

3. **Testing**: Write tests for new functionality. Use table-driven tests where appropriate.

4. **Documentation**: Document exported functions and types.

5. **Naming Conventions**:
   - Short receiver names: `w` for workspace, `a` for agent, `c` for channel
   - Avoid package-level variables except for cobra commands
   - Use descriptive but concise variable names

6. **Struct Alignment**: Run `make lint` to catch fieldalignment issues.

## Project Structure

```
bc/
├── cmd/mycel/           # CLI entry point (main.go)
├── config/              # Generated config code (cfgx)
├── internal/
│   └── cmd/             # Cobra command implementations
├── pkg/                 # Reusable packages
│   ├── agent/           # Agent lifecycle, roles, tmux sessions
│   ├── attachment/      # File attachment handling
│   ├── channel/         # SQLite-backed communication
│   ├── client/          # API client
│   ├── container/       # Docker container management
│   ├── cost/            # Cost tracking and budgets
│   ├── cron/            # Scheduled task management
│   ├── db/              # Database abstraction
│   ├── doctor/          # System health diagnostics
│   ├── events/          # Event logging
│   ├── gateway/         # External gateway integrations
│   ├── log/             # Structured logging
│   ├── mcp/             # MCP protocol support
│   ├── names/           # Agent name generation
│   ├── provider/        # AI provider registry
│   ├── runtime/         # Runtime backends (tmux, docker)
│   ├── secret/          # Secret management
│   ├── stats/           # Workspace statistics
│   ├── tmux/            # tmux session control
│   ├── token/           # Token management
│   ├── tool/            # Tool management
│   ├── ui/              # CLI output formatting (colors, tables)
│   ├── workspace/       # Workspace config (settings.json v2)
│   └── worktree/        # Git worktree operations
├── server/              # the daemon (API, web UI, MCP)
│   └── web/             # Embedded web UI (React)
│       └── dist/        # Built web assets
├── prompts/             # Default role prompt templates
├── web/                 # Web UI source (React/Vite)
├── landing/             # Landing page (Next.js)
└── tui/                 # TypeScript/React TUI (Ink)
    ├── src/
    │   ├── __tests__/   # Component and integration tests
    │   ├── components/  # Reusable UI components
    │   ├── hooks/       # React hooks (useAgents, useChannels, etc.)
    │   │   └── __tests__/ # Hook tests
    │   ├── navigation/  # Tab bar, keyboard navigation
    │   ├── services/    # mycel CLI wrapper (mycel.ts)
    │   ├── views/       # Full-screen views (14 views)
    │   │   └── __tests__/ # View tests
    │   └── app.tsx      # Main TUI application
    └── dist/            # Compiled output (CommonJS)
```

## Pull Request Process

1. **Branch Naming**: Use descriptive branch names
   - `feat/description` for features
   - `fix/description` for bug fixes
   - `docs/description` for documentation

2. **Commits**: Write clear commit messages
   - Use conventional commits format
   - Reference issues where applicable

3. **Testing**: Ensure all tests pass
   ```bash
   make check
   ```

4. **PR Description**: Include
   - Summary of changes
   - Related issue numbers
   - Test plan

5. **Review**: Address all review feedback

## Architecture Overview

Key concepts to understand before contributing:

- **Agents**: AI assistants running in isolated tmux sessions, each with its own git worktree
- **Workspace**: Project directory with `.mycel/` containing config, state, and per-agent data
- **Channels**: SQLite-backed inter-agent communication with persistent history
- **Roles**: Agent capabilities defined in `.mycel/roles/*.md` (engineer, manager, etc.)
- **Memory**: Per-agent persistent knowledge (experiences, learnings)

See `CLAUDE.md` for detailed architecture patterns and package documentation.

## Reporting Issues

Use GitHub Issues for:
- Bug reports
- Feature requests
- Documentation improvements

Include:
- Clear description
- Steps to reproduce (for bugs)
- Expected vs actual behavior
- Environment details

## Questions?

Open an issue or discussion on GitHub.

## Releasing

Releases are cut manually via GitHub Actions. CI/CD is fully automated from tag onwards.

### Steps

1. Ensure `main` is green. Check https://github.com/rpuneet/mycel/actions/workflows/ci.yml
2. Go to **Actions → Release → Run workflow**
3. Enter version in semver format: `vMAJOR.MINOR.PATCH` (e.g. `v0.1.0`)
   - Alpha/RC allowed: `v0.2.0-alpha`, `v1.0.0-rc.1`
4. Click **Run workflow**

### What happens

The release workflow:

1. **Prepare** — validates version format, creates and pushes git tag
2. **CI** — full test suite (lint, test, TUI, web, landing, build gate, security, container scan)
3. **Release Linux** — GoReleaser builds `linux/amd64`, creates archive + checksums, publishes GitHub release
4. **Release macOS** — Native CGO builds for `darwin/amd64` and `darwin/arm64`, uploads to release
5. **Release Docker** — Pushes `ghcr.io/rpuneet/mycel:<version>` and `:latest` to GHCR
6. **SBOM** — Generates and uploads `sbom.spdx.json` to release

### Homebrew tap publish

Requires `HOMEBREW_TAP_TOKEN` repo secret (GitHub PAT with repo scope for `rpuneet/homebrew-bc`). If unset, Homebrew publish is skipped automatically.

### Continuous deployment

Every merge to `main` also publishes `ghcr.io/rpuneet/mycel:main` via `.github/workflows/cd-main.yml`. No tagging required — users can pull the bleeding edge.

### Version strategy

- `v0.x.y` — pre-1.0, any breaking changes allowed, document in release notes
- `v1.0.0+` — semver discipline: breaking → major, features → minor, fixes → patch
- Pre-releases: `-alpha`, `-beta`, `-rc.N` suffixes

### Rollback

If a release is broken:

1. Delete the GitHub release (keeps the tag)
2. Or delete the tag: `git push origin :refs/tags/vX.Y.Z`
3. Fix, re-tag, re-run the workflow

Docker images are immutable — pull a prior tag instead.
