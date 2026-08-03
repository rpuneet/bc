# Contributing to mycel

Thank you for your interest in contributing to mycel! This document provides guidelines and instructions for contributing.

## Development Setup

### Prerequisites

- Go 1.25.4+
- tmux
- golangci-lint
- make
- Bun (for the web UI and landing page)
- the Wails CLI (only needed for the desktop app)

### Getting Started

```bash
# Clone the repository
git clone https://github.com/rpuneet/mycel.git
cd mycel

# Install dependencies
make deps

# Build the project
make build

# Run tests
make test

# Install locally
make install-local-mycel
```

## Build Commands

Naming convention: `make <verb>-<runtime>-<component>` where `verb` = `build` | `test` | `run` | `release` | `install` | `clean`, `runtime` = `local` (host) | `docker` (container), `component` = `mycel` | `web` | `landing` | `desktop`. `go` and `ts` are language aggregates for CI/CD convenience.

### Build (local)

| Command | Description |
|---------|-------------|
| `make build` | Build everything (local + docker) |
| `make build-local` | Build local binaries (go + ts) |
| `make build-local-go` | Build all Go binaries |
| `make build-local-mycel` | Build the mycel binary (embeds web UI, server) |
| `make build-local-ts` | Build all TS packages (web + landing) |
| `make build-local-web` | Build the web UI → `server/web/dist/` |
| `make build-local-landing` | Build the landing page |
| `make build-local-desktop` | Build the desktop app for the host OS (requires the Wails CLI) |
| `make release` | Build optimized release binaries (stripped symbols) |
| `make release-local-mycel` | Build an optimized mycel binary (embeds web UI) |
| `make install-local-mycel` | Install mycel to `$GOPATH/bin` |

### Build (Docker)

| Command | Description |
|---------|-------------|
| `make build-docker` | Build all Docker images (db, daemon, playwright) |
| `make build-docker-daemon` | Build the daemon Docker image |
| `make build-docker-db` | Build mycel-db (unified TimescaleDB) Docker image |
| `make build-docker-agent` | Build the default agent image (claude) |
| `make build-docker-agents` | Build all agent images |
| `make build-docker-agent-base` | Build the agent base image |
| `make build-docker-playwright` | Build the Playwright MCP Docker image |

### Optional: embedding the Google OAuth client for Gmail

`make build-local-mycel`, `make release-local-mycel`, and `make build-local-desktop` all read `GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` from the environment and inject them into the binary via `-ldflags -X` so that Gmail's "Sign in with Google" works with zero setup. Nothing is hardcoded in source — if the variables are unset the build is unaffected and Gmail falls back to the manual client-id/secret/refresh-token paste, exactly as before.

- **Official releases**: the maintainer sets `GOOGLE_OAUTH_CLIENT_ID` and `GOOGLE_OAUTH_CLIENT_SECRET` as repo Actions secrets (used by `release.yml`, `cd-main.yml`, and `.goreleaser.yml`).
- **Local dev**: `export GOOGLE_OAUTH_CLIENT_ID=... GOOGLE_OAUTH_CLIENT_SECRET=...` before running the build commands above to test one-click sign-in locally.

This is safe to embed because the client is registered as a Google "Desktop app" (installed application) OAuth client — Google's own docs treat the client secret for that client type as non-confidential (it can't be kept secret in a distributed binary), so baking it in at build time is the standard, supported pattern rather than a workaround. See `pkg/gateway/gmail/oauth.go` for the resolution order (env var override → build-injected default → unconfigured).

### Test

| Command | Description |
|---------|-------------|
| `make test-go` | Run Go tests with race detector |
| `make test-go-fast` | Go tests excluding slow packages (tmux, secret, doctor, internal/cmd) |
| `make test-ts` | Run all TS tests (web + landing) |
| `make test-web` | Run web UI tests (vitest) |
| `make test-web-e2e` | Web e2e tests (Playwright, needs a running daemon) |
| `make test-landing` | Run landing tests |
| `make coverage-go` | Go test coverage report |
| `make bench-go` | Run Go benchmarks |

### Lint & Quality

| Command | Description |
|---------|-------------|
| `make lint` | Run all linters (go + ts) |
| `make lint-go` | Run golangci-lint on Go code |
| `make lint-ts` | Run all TS linters (web + landing) |
| `make fmt-go` | Format Go code with gofmt |
| `make vet-go` | Run go vet |
| `make check` | Full quality gate (go + ts) |
| `make check-go` | Go quality gate: vet + lint + test |
| `make check-ts` | TS quality gate: typecheck + lint + test |
| `make ci-local` | Full CI pipeline locally |
| `make ci-docker` | Build all Docker images |

### Run

| Command | Description |
|---------|-------------|
| `make run-mycel` | Run the mycel CLI from source (`go run`) |
| `make run-web` | Run the web UI dev server (hot reload) |
| `make run-landing` | Run the landing dev server (hot reload) |

### Utilities

| Command | Description |
|---------|-------------|
| `make deps` | Install all dependencies (go + ts) |
| `make deps-go` | Download and tidy Go dependencies |
| `make deps-ts` | Install all TS dependencies (bun install) |
| `make scan-go` | Run govulncheck for Go vulnerabilities |
| `make scan-ts` | Run TS dependency audit |
| `make clean` | Remove all build artifacts |
| `make clean-local` | Remove build artifacts |
| `make clean-deps` | Remove build artifacts + node_modules |

Or directly with Bun (from `web/` or `landing/`):

```bash
cd web
bun install        # Install dependencies
bun run build      # Build to dist/
bun test           # Run tests
bun run lint       # Lint code
```

## Code Style

### Linting

We use `golangci-lint` with strict settings. All code must pass linting before merge.

```bash
# Run linter
make lint-go

# Configuration is in .golangci.yml
```

### Key Lint Rules

- **errcheck**: All errors must be handled (type assertions too)
- **gosec**: Security issues must be addressed (G104 excluded)
- **govet**: `enable-all` — no shadowed variables, struct field alignment enforced
- **noctx**: Context must be propagated through all call chains
- **staticcheck, bodyclose, prealloc, unconvert, misspell, ineffassign, unused**: also enforced

### Code Guidelines

1. **Error Handling**: Never ignore errors. Use explicit handling or `//nolint:errcheck` with justification.
2. **Context Propagation**: Pass `context.Context` through all call chains.
3. **Testing**: Write tests for new functionality. Use table-driven tests where appropriate.
4. **Documentation**: Document exported functions and types.
5. **Naming Conventions**:
   - Short receiver names: `h` for Home and handlers, `a` for agent, `m` for manager
   - Avoid package-level variables except for Cobra commands
   - gofmt with `-s` (simplify); goimports with local prefix `github.com/rpuneet/mycel`
6. **Struct Alignment**: Run `make lint-go` to catch fieldalignment issues.

## Project Structure

```
mycel/
├── cmd/
│   ├── mycel/           # CLI entry point (main.go)
│   └── gendocs/         # Regenerates docs/reference/cli from the Cobra tree
├── internal/
│   └── cmd/             # Cobra command implementations (agent, channel, config, cost, …)
├── pkg/                 # Reusable packages, self-contained (never imported by cmd in reverse)
│   ├── agent/           # Agent lifecycle, Manager, SpawnOptions, role setup
│   ├── app/             # Apps plugin platform (28 built-in integrations)
│   ├── attachment/      # File attachment handling
│   ├── client/          # HTTP client for the daemon API
│   ├── container/       # Docker runtime backend
│   ├── cost/            # Cost analytics computed from provider session files
│   ├── db/              # The single global database (~/.mycel/mycel.db)
│   ├── deps/             # Managed external dependencies (code-server, db containers)
│   ├── doctor/          # Health diagnostics (`mycel doctor`)
│   ├── events/          # Event log store
│   ├── gateway/         # Per-platform adapters (slack, telegram, discord, whatsapp, …)
│   ├── home/            # The ~/.mycel home: prefs.json, layout paths, roles, repo discovery
│   ├── log/             # Structured logging
│   ├── marketplace/     # Template/plugin marketplace
│   ├── mcp/             # MCP registry and client plumbing
│   ├── names/           # Agent name generation
│   ├── notify/          # Notification fan-out: subscriptions, delivery, history
│   ├── provider/        # AI provider registry (claude, gemini, cursor, …)
│   ├── runtime/         # Runtime backend interface (tmux, docker)
│   ├── secret/          # Encrypted vault (~/.mycel/secrets.vault)
│   ├── stats/           # Usage statistics and metrics
│   ├── template/        # Agent templates
│   ├── tmux/            # tmux session management
│   ├── token/           # Token counting
│   ├── tool/            # Tool registry and execution
│   ├── ui/              # Terminal output utilities for the CLI
│   └── worktree/        # Git worktree management
├── server/              # The daemon: HTTP server, handlers, agent-facing MCP, SSE hub
│   └── web/dist/        # Embedded, built web assets
├── web/                 # Web UI source (React/Vite) — the only rich surface
├── desktop/             # Wails desktop app wrapping the same web UI
├── landing/             # Landing page
├── packages/            # Shared TS packages (mycel-cli npm wrapper)
└── docker/              # Per-provider agent Dockerfiles plus daemon/db/playwright images
```

See `CLAUDE.md` (or `.claude/CLAUDE.md`) for detailed architecture patterns and package documentation.

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

5. **Review**: Address all review feedback. If your PR changes `internal/cmd/**`, `pkg/**` public API, or `server/**` without a matching update under `docs/**` or the relevant README, the docs-freshness CI check will leave an advisory comment — update the docs or explain why none are needed.

### Changelog entries

**Most PRs should not touch `CHANGELOG.md`.** Release notes are generated from
commit history by GoReleaser, and because PRs are squash-merged, each PR already
becomes one grouped line carrying its title and number. Restating every fix in
the changelog produces a hand-written second copy of that list — and since every
entry goes at the top of the same file, it is a reliable source of merge
conflicts between PRs in flight.

Add an entry only when a reader has to *do* something or would be surprised:

- **Security fixes** — what was exposed and what now blocks it.
- **Breaking changes** — mark the commit `type(scope)!:` as well.
- **Behavior changes with an upgrade action** — a removed tool or flag, a renamed
  path, a changed default, a keybinding that moved. State the action explicitly.

Ordinary bug fixes, features, refactors and internal changes need no entry: your
commit subject is the release note. Make the subject read as a statement to a
user rather than a note to yourself.

## Architecture Overview

Key concepts to understand before contributing:

- **Agents**: AI assistants running in isolated tmux sessions or Docker containers, each with its own git worktree
- **Home**: The single global state root `~/.mycel` — `prefs.json` (config), `mycel.db` (database), `secrets.vault`, `mcps.json`, `templates/`
- **Apps**: Plugin integrations with external platforms (Slack, GitHub, etc.), served at `/api/apps`
- **Roles**: DB-backed capabilities and inheritance; role prompts and MCP servers are written into the agent worktree on spawn
- **Providers**: AI agent CLIs (Claude, Gemini, Cursor, …) registered behind a common interface in `pkg/provider`

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

Releases are cut manually by pushing a tag. Everything from the tag onwards —
CI, builds for every platform, the GitHub release, Homebrew and the SBOM — is
automated.

### Steps

1. Ensure `main` is green. Check https://github.com/rpuneet/mycel/actions/workflows/ci.yml
2. Tag the commit you want to ship and push the tag:

   ```bash
   git tag -a v0.4.4 -m "v0.4.4"
   git push origin v0.4.4
   ```

   Use semver: `vMAJOR.MINOR.PATCH`. Alpha/RC allowed: `v0.5.0-alpha`, `v1.0.0-rc.1`.

Pushing the tag is what triggers the release. **Actions → Release → Run workflow**
also works and tags for you, but push the tag by hand when you want the released
artifacts to come from a specific commit you have already verified.

> **If you use Run workflow and it fails at the Security gate:** the `Prepare` job
> creates and pushes the tag *before* CI runs, so a later failure leaves a tag with
> no release attached. Because that tag was pushed by `GITHUB_TOKEN` it does not
> trigger a workflow run of its own. Delete and re-push it from your own account to
> start a real release from the same commit:
>
> ```bash
> git push origin :refs/tags/v0.4.4
> git tag -a v0.4.4 <sha> -m "v0.4.4" && git push origin v0.4.4
> ```

### What happens

The release workflow (`.github/workflows/release.yml`):

1. **Prepare** — validates version format; creates and pushes the git tag when
   started via **Run workflow** (on a tag push the tag already exists)
2. **CI** — full suite: lint, test, test (full), web, landing, build gate, security
3. **Release Linux + Windows** — GoReleaser builds `linux/{amd64,arm64}` and
   `windows/{amd64,arm64}`, plus `.deb` and `.rpm` packages, and publishes the
   GitHub release with `checksums.txt`
4. **Release macOS** — native CGO builds for `darwin/amd64` and `darwin/arm64`,
   uploaded with `checksums-macos.txt`
5. **Release Desktop** — Wails desktop app for macOS, Linux and Windows
6. **Update Homebrew Formula** — see below
7. **SBOM** — generates and uploads `sbom.spdx.json`

Docker images are **not** built here. `cd-main.yml` publishes
`ghcr.io/rpuneet/mycel:main` and `:main-<sha>` on every merge to `main`, so there
is no `:<version>` image per release — see
[Continuous deployment](#continuous-deployment).

### Homebrew tap publish

Requires the `HOMEBREW_TAP_TOKEN` repo secret (GitHub PAT with repo scope for `rpuneet/homebrew-mycel`). If unset, Homebrew publish is skipped automatically.

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
