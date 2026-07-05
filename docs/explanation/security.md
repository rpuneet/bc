# Security

This document describes the security model, threat boundaries, and hardening
measures in mycel and its server.

## Threat Model

mycel is a **local development tool**. The server binds to `127.0.0.1:9374`
by default and is only reachable from the local machine. Security relies
primarily on the localhost trust boundary, with an optional API-key
authentication layer (see [API Key Authentication](#api-key-authentication))
for anything beyond that.

**In scope:**

- Protecting secrets at rest (API keys, tokens stored via `mycel secret set`).
- Isolating Docker-based agents from each other and from the host filesystem.
- Preventing information leakage through HTTP error responses.
- Rate-limiting the API to mitigate local denial-of-service.

**Out of scope (by design):**

- TLS — the server is not designed to be exposed to a network directly. If
  you need remote access, put it behind a TLS-terminating reverse proxy and
  enable API-key authentication.
- Multi-tenant isolation — the server is single-tenant, used by one
  developer (or one CI job) at a time.

## Secret Management

Secrets are stored in an SQLite vault at `~/.mycel/secrets.vault` and encrypted
with **AES-256-GCM**. The encryption key is derived from a master passphrase
using **PBKDF2-SHA256** with 600,000 iterations (per OWASP 2023 guidance) and
a random 16-byte salt.

### Passphrase Resolution

The passphrase is resolved in priority order:

1. **`MYCEL_SECRET_PASSPHRASE` environment variable** — set this in CI or when
   you want explicit control.
2. **Auto-generated key file at `~/.mycel/secret-key`** — created on first use
   with 32 random bytes (hex-encoded), file permissions `0600`, directory
   permissions `0700`.

### Encryption Details

| Parameter       | Value                        |
|-----------------|------------------------------|
| Algorithm       | AES-256-GCM                  |
| Key derivation  | PBKDF2-SHA256, 600k rounds   |
| Salt            | 16 bytes, random, per-store  |
| Nonce           | 12 bytes, random, per-value  |
| Storage format  | Base64(nonce ‖ ciphertext)   |

Secrets are resolved at runtime via `${secret:NAME}` references in agent
environment variables. The `ResolveEnv` method substitutes these references
with decrypted values just before the agent process starts.

## Docker Agent Isolation

When the runtime is set to `docker`, each agent runs in its own container
with the following isolation measures.

### Volume Mounts

Containers receive exactly two mounts by default:

1. **Agent's repo** → `/workspace` (project source code).
2. **Persistent Claude state** → `/home/agent/.claude` (auth, plugins,
   sessions). Stored at `~/.mycel/workspaces/<id>/agents/<name>/claude/` on
   the host.

### Mount Validation

Extra mounts (configured via `runtime.docker.extra_mounts`) are validated
by `validateMount()` before being passed to `docker run`:

- **Format check**: must be `src:dst` or `src:dst:opts`.
- **Path traversal rejection**: source paths containing `..` are rejected.
- **Absolute path requirement**: source must be an absolute path.
- **Symlink resolution**: source is resolved via `filepath.EvalSymlinks` to
  prevent symlink-based escapes (e.g., a symlink inside the repo pointing
  to `/etc`).
- **Repo containment**: the resolved source path must be within or equal
  to the repo root directory.

### Network

The default Docker network is `bc-net` (`runtime.docker.network` in
`preferences.json`; the backend falls back to `bridge` when unset). To fully
isolate agents from the network, set the network to `none`.

### Resource Limits

Containers are created with configurable CPU and memory limits (defaults:
2 CPUs, 2048 MB). These prevent runaway agents from starving the host.

### Environment Variable Validation

Environment variable names passed to `docker run -e` are validated against
the POSIX pattern `^[A-Za-z_][A-Za-z0-9_]*$` to prevent injection through
crafted key names.

## HTTP Security

### Middleware Chain

The mycel server applies middleware in this order (outermost runs first):

```
RateLimit → APIKeyAuth → RequestID → RequestLogger → Recovery → Gzip → MaxBodySize → CORS → Router
```

### API Key Authentication

Authentication is **optional** and disabled by default. Starting the server
with `mycel up --api-key <key>` (or setting the `MYCEL_API_KEY` environment
variable) enables the `APIKeyAuth` middleware, which requires every request
to carry either:

- `Authorization: Bearer <key>`, or
- an `X-API-Key: <key>` header.

Requests without a valid key receive `401 Unauthorized`. When no key is
configured, the middleware is a pass-through and the localhost trust
boundary is the only protection.

### Rate Limiting

A token-bucket rate limiter is applied globally:

- **Rate**: 100 requests per second
- **Burst**: 200 tokens

Requests that exceed the limit receive `429 Too Many Requests`.

### Request Body Limit

All requests are limited to **1 MB** (`1 << 20` bytes) via the `MaxBodySize`
middleware. Requests exceeding this limit are rejected before the handler
runs.

### Error Wrapping

Internal errors are never leaked to clients. The `httpInternalError` helper
logs the full error server-side and returns a generic JSON response:

```json
{"error": "internal server error"}
```

The `Recovery` middleware catches panics and returns the same generic error
instead of crashing the server or exposing stack traces.

### CORS

CORS is enabled by default with origin `*` (safe because the server only
listens on localhost). The origin can be restricted via `Config.CORSOrigin`.

### Request IDs

Every request is assigned a unique ID (via `X-Request-ID` header). If the
client provides one, it is reused; otherwise a random hex ID is generated.

## MCP Security

The MCP (Model Context Protocol) server is mounted at `/_mcp/sse` and
`/_mcp/message` on the same HTTP server. It inherits the same localhost
trust model and the same optional API-key middleware as the REST API.

MCP endpoints are protected by the same middleware chain (rate limiting,
body size limit, recovery) as the REST API.

## Recommendations for Production-Like Deployments

If you need to expose the mycel server beyond localhost (e.g., for remote
agent coordination):

1. Enable API-key authentication (`mycel up --api-key` or `MYCEL_API_KEY`).
2. Place the server behind a reverse proxy (nginx, Caddy, etc.) with TLS
   termination.
3. Restrict CORS origin to your specific domain.
4. Set `MYCEL_SECRET_PASSPHRASE` explicitly rather than relying on the
   auto-generated key file.
5. Use `network = "none"` for Docker agents that do not need outbound access.
