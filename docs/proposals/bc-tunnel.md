# bc tunnel — remote access via relay

- **Status:** Proposed
- **Issue:** #3000
- **Related:** ADR 0001 (CLI architecture), #3002
- **Last updated:** 2026-04-17

## Summary

Expose the locally-running `bcd` dashboard to the public internet via a persistent WebSocket tunnel to a relay server at `bc-infra.com`. Lets users hit their own `bcd` from a phone, a teammate's laptop, or the hosted landing page without opening a port, running ngrok, or setting up a VPN.

Foundation for a paid `$2–5/mo` plan. The relay is a dumb pipe — zero agent data at rest.

## Architecture

```
User's Mac                                bc-infra.com (relay VPS)
┌───────────────────┐                    ┌───────────────────────┐
│ bcd :8080         │                    │ bc-relay              │
│  ├─ /api          │◄── WSS tunnel ───► │  ├─ Auth (GitHub OAuth)│
│  ├─ tmux/docker   │   (persistent)     │  ├─ Stripe billing    │
│  ├─ web UI        │                    │  ├─ Tunnel registry   │
│  └─ SSE hub       │                    │  ├─ HTTP proxy        │
└───────────────────┘                    │  └─ Push (Web Push)   │
                                         └───────────┬───────────┘
                                                     │ HTTPS
                                            ┌────────▼────────┐
                                            │ Phone / laptop  │
                                            └─────────────────┘
```

Data flow: `Browser → https://bc-infra.com/t/<user>/api/agents → relay → WSS tunnel → local bcd → response back up`.

## Components that live in this repo

| Path | Purpose |
|---|---|
| `internal/cmd/tunnel.go` | `bc tunnel {start,stop,status}` subcommand. |
| `pkg/tunnel/client.go` | WSS client: connect, auto-reconnect, multiplex HTTP + SSE. |
| `pkg/tunnel/protocol.go` | Wire-protocol types shared by client and relay. |
| `pkg/tunnel/config.go` | Credentials at `~/.bc/tunnel.json` (0600). |
| `cmd/bc-relay/` | Relay server binary (Phase 2). |
| `server/handlers/` *(existing)* | **Unchanged.** Relay proxies the same `/api/...` surface the CLI and web UI already use (per ADR 0001 §2). |

## Wire protocol

Single WSS connection per client. Framed JSON, multiplexed by `id`:

```json
// Relay → client: inbound request
{"id": "req-1", "method": "GET", "path": "/api/agents", "headers": {...}, "body": null}

// Client → relay: response
{"id": "req-1", "status": 200, "headers": {...}, "body": "..."}

// SSE / streaming: multiple frames, same id, ending with {"eof": true}
{"id": "req-2", "stream": "data: {\"type\":\"agent.hook\"...}\n\n"}
{"id": "req-2", "eof": true}

// Liveness
{"type": "ping"} / {"type": "pong"}
```

Frames are size-limited (1 MB per frame) and backpressure is honoured — if the tunnel can't keep up with an SSE firehose, excess events are dropped on the client side and a `lag-dropped` counter is sent to the relay so the UI can warn.

## Phases

### Phase 1 — CLI command + WSS client

Shippable without a relay by pointing `--relay` at a local test server. Delivers:

- `bc tunnel start [--relay URL] [--bcd-addr URL]`
- `bc tunnel status` — prints public URL, uptime, connection state.
- `bc tunnel stop`

Client is headless and heartbeats every 30 s. Reconnect with exponential backoff up to 60 s.

### Phase 2 — `cmd/bc-relay/`

Single Go binary, `$5/mo` VPS. In-memory tunnel registry; Redis is **deferred** until multi-instance is actually needed (one VPS comfortably handles thousands of tunnels — this is bytes-shuffling, not compute).

Surface:

```
POST   /auth/github          — OAuth callback
GET    /auth/me
POST   /billing/checkout
POST   /billing/webhook
WS     /tunnel/connect
ANY    /t/<user>/*           — proxy to user's tunnel
GET    /t/<user>/health
```

Deployed from the same monorepo via its own release workflow; landing page at `bc-infra.com` is unchanged.

### Phase 3 — `bc up --tunnel`

Wraps Phase 1 so `bc up --tunnel` means "start bcd and immediately bring up the tunnel" using the `tunnel` section of `~/.bc/settings.json`:

```json
{
  "tunnel": {
    "enabled": true,
    "relay": "https://relay.bc-infra.com",
    "auto_connect": true
  }
}
```

### Phase 4 — PWA + push

Web UI gains `manifest.json` + a tiny service worker so Safari/Chrome can "Add to Home Screen". Push via the Web Push API through the relay when:

- Agent state changes to `stuck` (PermissionRequest hook).
- Budget ceiling crossed.
- Ralph-loop iteration finishes.
- bcd goes unreachable for more than 5 minutes (monitored tier only).

## Pricing (sketch, not a commitment)

| Feature | Free | Paid ($5/mo) |
|---|---|---|
| Local dashboard | ✅ | ✅ |
| Remote tunnel | ✅ (100 req/min, 1 device) | ✅ (1000 req/min, unlimited devices) |
| Push notifications | ❌ | ✅ |
| Team read-only viewers | ❌ | ✅ (up to 5) |
| Custom subdomain | ❌ | `<name>.bc-infra.com` |
| Tunnel uptime monitoring | ❌ | ✅ |

## Open questions — resolved

| # | Question | Decision |
|---|---|---|
| Q1 | Relay in this repo or separate? | **This repo under `cmd/bc-relay/`.** Keeps one source of truth for the wire protocol; sibling release pipeline. We can split into a `bc-infra/` repo later if the relay outgrows the CLI's release cadence. |
| Q2 | Free tier? | **Yes, with a rate limit.** 100 req/min is enough for a phone-checking-dashboard use case; the 1000 req/min jump on paid covers teams and long SSE streams. Free tier gates out push, team viewers, and monitoring. |
| Q3 | Should `bc tunnel` tunnel arbitrary local ports? | **Yes — `--bcd-addr` accepts any URL.** The default targets the local bcd, but the plumbing is generic (HTTP+WS proxy) and a user wanting to tunnel another local service (e.g. Grafana) shouldn't be blocked. Security implications flagged in the docs but trust model is "your machine, your choice." |
| Q4 | Team viewers — read-only or interactive? | **Read-only by default; interactive via an explicit `--interactive` share flag.** Read-only covers "show my dashboard to a teammate" safely; interactive is for pair-debugging and requires a positive opt-in so nobody accidentally grants agent-stop authority to every viewer. |

## Security model

- Tunnel transport: WSS (TLS) — relay sees only encrypted bytes at the socket level.
- Auth: GitHub OAuth → JWT; tunnel open requires a valid JWT. Tokens on disk at `~/.bc/tunnel.json` with 0600.
- API-key passthrough: if `bcd --api-key` is set, the relay forwards the Authorization header unchanged; relay does not see or store the key.
- Rate limits enforced at the relay, per-user.
- Shared links: short-lived signed URLs with configurable TTL (1 h, 24 h, 7 d); revocable from the dashboard.
- Optional IP allowlist per tunnel (paid tier).
- No agent state, logs, secrets, or costs are persisted on the relay.

## Non-goals

- **Replacing SSH** for interactive shell access. `bc tunnel` is HTTP+WS only.
- **Multi-region relay.** Single-region is fine at $2–5/mo scale; global anycast is a later problem.
- **Offline queueing.** If the tunnel drops, requests fail with 504; no store-and-forward.
- **Self-hosted relay UX.** The CLI takes `--relay URL` so someone can point at their own relay, but we don't promise packaging / support for third-party installs.

## Milestones

- [ ] Phase 1 scaffolding (client + test harness) — 1 PR
- [ ] Phase 1 CLI + protocol + `bc tunnel start` against a local test relay — 1 PR
- [ ] Phase 2 `cmd/bc-relay/` minimum viable (auth + proxy + in-memory registry) — 1 PR
- [ ] Phase 2 Stripe billing — 1 PR
- [ ] Phase 3 `bc up --tunnel` + settings plumbing — 1 PR
- [ ] Phase 4 PWA manifest + service worker — 1 PR
- [ ] Phase 4 Web Push integration — 1 PR
- [ ] Landing page pricing section — 1 PR

## References

- Issue #3000 — feature request
- ADR 0001 — CLI architecture (tunnel rides the `/api/...` surface, no new protocol needed between CLI and local bcd)
- `docs/proposals/multi-workspace-and-code-tab.md` — URL scoping (tunnel transparently carries `/w/<wsId>/...`)
