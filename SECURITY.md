# Security Policy

## Supported Versions

| Version | Supported |
| ------- | --------- |
| 0.4.x   | Yes       |
| < 0.4   | No        |

mycel is pre-1.0; only the latest minor release receives security fixes.

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in mycel, please report it responsibly.

### How to Report

1. **GitHub Security Advisories** (Preferred): Use [GitHub's Security Advisory feature](../../security/advisories/new) to report vulnerabilities privately.

2. **Email**: Send details to the repository maintainers via GitHub.

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fixes (optional)

### What to Expect

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 7 days
- **Resolution Timeline**: Depends on severity
  - Critical: 24-48 hours
  - High: 7 days
  - Medium: 30 days
  - Low: 90 days

### Disclosure Process

1. Report received and acknowledged
2. Vulnerability confirmed and assessed
3. Fix developed and tested
4. Security advisory published with fix
5. Public disclosure after patch is available

### Scope

This policy applies to:

- The **mycel CLI** tool (`cmd/mycel`, `internal/cmd`, `pkg/`)
- The **daemon** (`server/`) including the REST API, SSE hub, and MCP SSE endpoints
- The **web dashboard** (`web/`) and **desktop app** (`desktop/`), both embedding the same web UI
- **Docker agent images** (`docker/`)
- Agent communication protocols (channels, MCP)
- Secret storage and injection (`pkg/secret`)
- Configuration and credential handling (`.mycel/`, `settings.json`)

### Out of Scope

- Issues in third-party dependencies (report to upstream)
- Social engineering attacks
- Physical security issues

## Security Architecture

### Secret Management

mycel stores secrets in an encrypted vault (`~/.mycel/secrets.vault`) under the global `~/.mycel` home, with optional per-repo overrides. Secrets are injected into agent sessions at startup via environment variables and are never written to disk in plaintext. Use `mycel secret set` to manage secrets instead of placing them in `.env`.

Sensitive patterns (API keys, tokens, DSNs) are redacted from WebSocket streams by the daemon before reaching the web dashboard.

### Agent Isolation

Each agent runs in its own tmux session (local) or Docker container (production) with a dedicated git worktree. Agents communicate only through the daemon API and SQLite-backed channels -- never via shared filesystem state.

### Docker Hardening

- All agent images run as a non-root user
- Base images are pinned to specific versions (never `:latest`)
- Containers use bridge networking (not `--net=host`)
- `HEALTHCHECK` directives are included in production images

### Network & API

- The daemon server binds to `localhost:9374` by default
- CORS, request body size limits, and rate limiting are enforced
- MCP SSE endpoints identify the calling agent via query parameter (`?agent=<name>`)

### CI/CD

- `golangci-lint` with strict security linters (`gosec`, `errcheck`, `noctx`)
- `govulncheck` runs on every push to main
- Dependabot monitors Go and npm dependencies

## Security Best Practices

When using mycel:

- Keep mycel updated to the latest version
- Store API keys and tokens via `mycel secret set`, not in `.env` or shell history
- Review agent prompts and role definitions before execution
- Use environment variables for sensitive data (never hardcode)
- Restrict agent capabilities to the minimum required via role definitions
- Monitor agent costs and set appropriate budgets
- Run agents in Docker runtime for production workloads (stronger isolation)
- Audit `.mycel/agents/` directory periodically for stale worktrees
