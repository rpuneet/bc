# Notifications

Notifications are inbound-only gateways that bridge external platforms (Slack, GitHub, Telegram, and 30+ others) to mycel agents. mycel routes platform events to subscribed agents, who respond directly using injected credentials and platform APIs.

## Architecture

mycel acts as a notification router, not a messaging proxy. External platforms push events into mycel through adapter connections. mycel dispatches those events to subscribed agents based on filtering rules. Agents call platform APIs themselves using environment variable credentials.

```mermaid
flowchart LR
    subgraph Platforms["External Platforms"]
        S[Slack]
        T[Telegram]
        G[GitHub]
        W[Webhooks]
        More["...40+ more"]
    end

    subgraph bc["bc (notification router)"]
        GW["Gateway Adapters"]
        NS["Notify Service"]
    end

    subgraph Agents["Agents (tmux/Docker)"]
        A1[agent-1]
        A2[agent-2]
        A3[agent-3]
    end

    S & T & G & W & More -- "events" --> GW
    GW -- "Notification{Raw JSON}" --> NS
    NS -- "dispatch to subscribers" --> A1 & A2 & A3
    A1 & A2 & A3 -. "respond via platform API\n(credentials in env vars)" .-> Platforms
```

## Gateway adapters are inbound-only

Gateway adapters in `pkg/gateway/<platform>/` exist to **receive** messages
from external platforms (Slack, Telegram, Discord, WhatsApp, GitHub, etc.)
and dispatch them into the server's notification pipeline. They are **not** a
generic outbound abstraction.

When an agent needs to **send** a message to an external platform, the
agent talks to the platform's API directly using credentials injected
from `preferences.json` / the workspace secret store. Examples:

- **Telegram**: `curl -H "Authorization: Bearer $TELEGRAM_BOT_TOKEN" https://api.telegram.org/bot.../sendMessage`
- **Slack**: `chat.postMessage` via the Slack Web API using `$SLACK_BOT_TOKEN`
- **WhatsApp**: inbound-only today — the gateway listens via `whatsmeow`; there is no outbound path (see below)

## Outbound cookbook

The four platforms below all follow the same pattern: the agent reads a
bot token from its own `.bc/agents/<name>/env.json` via `${secret:NAME}`
refs, and posts to the platform's official REST API with the agent's
own `Bash` / `WebFetch` tool. No mycel-side wrapper is needed — this
is what agents can already do, formalized as a cookbook so every role
knows the shape.

### env.json — per-agent token slots

Set once per agent. Values are `${secret:NAME}` refs; the actual
secret lives in the workspace secret store (`.bc/secrets.db` or
whichever backend the workspace is configured to use).

```json
{
  "SLACK_BOT_TOKEN":     "${secret:SLACK_BOT_TOKEN}",
  "TELEGRAM_BOT_TOKEN":  "${secret:TELEGRAM_BOT_TOKEN}",
  "DISCORD_BOT_TOKEN":   "${secret:DISCORD_BOT_TOKEN}",
  "WHATSAPP_BOT_TOKEN":  "${secret:WHATSAPP_BOT_TOKEN}"
}
```

Only fill the platforms an agent actually needs — mycel doesn't care
about the ones you leave out.

### Slack — `chat.postMessage`

```bash
curl -s -X POST https://slack.com/api/chat.postMessage \
  -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
  -H "Content-Type: application/json; charset=utf-8" \
  --data '{
    "channel":     "C0123456789",
    "text":        "hello from <agent-name>",
    "username":    "<agent-name>",
    "icon_emoji":  ":robot_face:"
  }'
```

Set `username` + `icon_emoji` on every call — that's how you get the
agent-specific attribution on Slack file uploads and reactions instead
of the gateway bot's default identity. The alternative pattern
(upload-then-post-permalink) is documented in the "Slack screenshot
attribution" reference note.

### Telegram — `sendMessage`

```bash
curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
  --data-urlencode "chat_id=<CHAT_ID>" \
  --data-urlencode "text=hello from <agent-name>"
```

Telegram uses one bot token per bot; agents share a bot but distinguish
themselves in the `text` prefix.

### Discord — `webhooks` or bot REST

Preferred: per-agent Discord webhooks (each agent has its own webhook
URL, so username/avatar are fully agent-controlled).

```bash
curl -s -X POST "$DISCORD_WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  --data '{
    "content":     "hello from <agent-name>",
    "username":    "<agent-name>",
    "avatar_url":  "https://…"
  }'
```

Falls back to `POST /channels/<id>/messages` with a bot token when the
agent needs to post to a channel that doesn't have a webhook.

### WhatsApp — inbound-only (no outbound path today)

WhatsApp is the one platform without an outbound recipe. The
`pkg/gateway/whatsapp` adapter connects via the `whatsmeow` library and
only **listens**: the persisted device session lives inside the server
process, the adapter exposes no HTTP handler, and no send endpoint
exists (the old daemon-side send route has been removed — unknown
gateway sub-routes return `404`). Agents cannot call the WhatsApp
personal API directly either, because the device session is not theirs
to share. Outbound WhatsApp is tracked in
[#3178](https://github.com/rpuneet/mycel/issues/3178).

### Why not wrap these as mycel-side tools?

The tempting shape is `pkg/tool/platform/slack.go` exposing
`slack_post` as a first-class MCP tool. We deliberately don't:

1. Every agent already has `Bash` / `WebFetch` — curl on `chat.postMessage`
   is a one-liner, and the docs above are the whole spec.
2. Wrappers would need a corresponding schema, versioning, and would
   have to be updated for every new endpoint the platforms add
   (uploads, reactions, threading). curl doesn't.
3. Per-agent token isolation is already handled by `env.json` —
   nothing about a wrapper would change that boundary.
4. Removing the wrapper layer keeps the mycel surface smaller and the
   platform API surface fully discoverable to the agent.

The only exception is WhatsApp, where the persistent session lives
inside the server and no outbound path currently exists.

The existing `Send` methods on the Slack, Telegram, and Discord adapters
are an **internal convenience** used by the MCP `send_message` tool when
an agent posts to a gateway channel through the server. Do **not** add
`Send` to new adapters. WhatsApp, Matrix, IRC, Signal, and the other
30+ adapters deliberately have no `Send` method — that is correct per
this design, **not** a missing feature or a bug.

### `POST /api/gateways/{platform}/channels/{channel}/send` is removed

**Status: Removed.** (It was deprecated in v0.3.1 with RFC 8594
Deprecation/Sunset headers and has since been deleted.)

This endpoint violated the "notifications strictly inbound" principle
above. The server no longer registers it — unknown gateway sub-routes,
including `send`, return `404 Not Found`.

The replacement pattern is documented above: each agent calls the
platform's official API directly with credentials from its own env, so
posts, file uploads, and reactions attribute correctly to the agent's own
bot identity. Tracking issue: [#3178](https://github.com/rpuneet/mycel/issues/3178).

If you find a code-review tool flagging "adapter X is missing Send",
that finding is incorrect: outbound is the agent's responsibility, and
the inbound adapter is complete without it.

## Connection patterns

All 37+ platform adapters follow one of three connection patterns:

| Pattern | Examples | Mechanism |
|---------|----------|-----------|
| **Socket** | Slack, Discord, Telegram, Matrix, IRC, Mattermost, Twitch, MQTT | Long-lived connection. `Start()` blocks, events stream in. |
| **Webhook** | GitHub, GitLab, Stripe, Sentry, PagerDuty, Datadog, Generic | bc exposes an HTTP endpoint. The platform POSTs events. |
| **Poll** | RSS, Notion, Reddit, Gmail | Timer-based fetch. `Start()` polls on interval, forwards new items. |

## NotificationAdapter Interface

Every platform adapter implements this interface. Adapters are thin wrappers (~50-100 lines) that connect to a platform and forward raw events.

```go
// Located in: pkg/gateway/gateway.go

// AdapterType identifies the connection pattern.
type AdapterType string

const (
    AdapterSocket  AdapterType = "socket"   // long-lived connection (WebSocket, polling loop)
    AdapterWebhook AdapterType = "webhook"  // HTTP endpoint — platform POSTs events to bc
    AdapterPoll    AdapterType = "poll"     // timer-based polling — bc fetches new events
)

// NotificationAdapter handles the platform connection lifecycle.
type NotificationAdapter interface {
    // Name returns the adapter identifier ("slack", "github", "telegram").
    Name() string

    // Type returns the connection pattern. Determines how bc wires the adapter:
    //   socket  → goroutine running Start()
    //   webhook → HTTPHandler() mounted on the server HTTP mux at /hooks/{name}
    //   poll    → goroutine running Start() with internal ticker
    Type() AdapterType

    // Start connects to the platform and begins receiving notifications.
    // Calls handler for each inbound event with raw JSON payload.
    // Blocks until ctx is canceled. For webhook adapters, this is a no-op.
    Start(ctx context.Context, handler func(Notification)) error

    // Stop gracefully disconnects from the platform.
    Stop() error

    // HTTPHandler returns an http.Handler for webhook-based adapters.
    // Socket and poll adapters return nil.
    // The handler is mounted at /hooks/{name} on the mycel HTTP server.
    HTTPHandler() http.Handler

    // Channels returns discovered channels/groups the bot has access to.
    Channels() []ChannelInfo

    // Status returns the adapter's connection state for the web UI.
    Status() AdapterStatus
}

// AdapterStatus is reported to the web UI for observability.
type AdapterStatus struct {
    Connected     bool      `json:"connected"`
    Error         string    `json:"error,omitempty"`
    BotName       string    `json:"bot_name,omitempty"`
    LastMessageAt time.Time `json:"last_message_at,omitempty"`
    MessageCount  int64     `json:"message_count"`
}

// ChannelInfo represents a discovered channel on a platform.
type ChannelInfo struct {
    ID       string `json:"id"`       // platform channel ID
    Name     string `json:"name"`     // human-readable name
    Platform string `json:"platform"` // adapter name
}
```

## Notification Data Model

Each notification wraps the complete platform payload as raw JSON. Agents parse what they need. This avoids maintaining platform-specific data models and gives agents full context (files, reactions, threads, metadata).

```go
// Located in: pkg/gateway/gateway.go

type Notification struct {
    Timestamp time.Time       `json:"timestamp"` // when bc received the event
    Raw       json.RawMessage `json:"raw"`       // ENTIRE platform payload — no parsing
    Channel   string          `json:"channel"`   // "engineering", "bc-repo", "general"
    Platform  string          `json:"platform"`  // "slack", "github", "telegram"
    Sender    string          `json:"sender"`    // extracted for self-skip filtering
    Content   string          `json:"content"`   // human-readable text for display/storage
    Mentions  []string        `json:"mentions"`  // extracted @mentions for mention_only filter
}
```

| Field | Purpose |
|-------|---------|
| `Channel` | Channel name within the platform (e.g., `"engineering"`). Combined with `Platform` for subscription lookup as `platform:channel`. |
| `Platform` | Platform identifier. Matches adapter `Name()`. |
| `Sender` | Extracted from raw payload (one field per adapter). Used for self-skip filtering. |
| `Content` | Human-readable text extracted by the adapter for display and storage. Falls back to raw JSON. |
| `Mentions` | Extracted via regex `@[a-zA-Z][a-zA-Z0-9_-]*` across the raw JSON bytes. Used for `mention_only` filtering. |
| `Timestamp` | When bc received the event. |
| `Raw` | Complete platform payload, unmodified. The agent receives the full JSON and parses what it needs. |

Example notification delivered to an agent:

```json
{
  "timestamp": "2026-04-18T10:32:15Z",
  "channel": "engineering",
  "platform": "slack",
  "sender": "bob",
  "mentions": ["eng-01"],
  "raw": {
    "type": "message",
    "channel": "C0ABC123",
    "user": "U0DEF456",
    "text": "@eng-01 take a look at PR #428",
    "ts": "1712657535.000200",
    "team": "T0GHI789"
  }
}
```

## Multi-Bot Support

A single platform supports multiple adapter instances, each with its own credentials and channels. The adapter `Name()` uses a `platform:label` convention:

```
telegram              → default Telegram bot
telegram:trade        → trade research bot
slack                 → main workspace bot
slack:personal        → personal workspace bot
github:bc             → bc repo webhooks
github:trade          → trade repo webhooks
```

The gateway manager registers each as a separate adapter. Subscriptions reference the full name (e.g., `telegram:trade:chat_group`), so agents subscribe to specific bots. Credentials are stored per-instance in workspace secrets.

## Platform Integrations

### Tier 1: Chat Platforms

Bidirectional messaging platforms where agents receive and respond.

| # | Platform | Transport | Credential Env Vars | Identity Method | Status |
|---|----------|-----------|---------------------|-----------------|--------|
| 1 | **Slack** | Socket Mode (WebSocket) | `SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` | `username` param in `chat.postMessage` | Active |
| 2 | **Telegram** | Bot API long-polling | `TELEGRAM_BOT_TOKEN` | Prefix `[agent-name]: message` | Active |
| 3 | **Discord** | Gateway WebSocket | `DISCORD_BOT_TOKEN` or `DISCORD_WEBHOOK_URL` | `username` param in webhook execute | Active |
| 4 | **WhatsApp** | Cloud API webhooks | `WHATSAPP_TOKEN` | Prefix text (fixed number) | Active |
| 5 | **Signal** | signal-cli bridge | `SIGNAL_CLI_URL` | Prefix text (fixed number) | Active |
| 6 | **iMessage** | BlueBubbles server | `BLUEBUBBLES_URL`, `BLUEBUBBLES_PASSWORD` | Prefix text (fixed Apple ID) | Active |
| 7 | **Matrix** | Client-Server API | `MATRIX_HOMESERVER`, `MATRIX_TOKEN` | Bot display name | Active |
| 8 | Microsoft Teams | Bot Framework | `TEAMS_APP_ID`, `TEAMS_APP_SECRET` | Bot identity | Planned |
| 9 | Google Chat | Chat API + Pub/Sub | `GOOGLE_CHAT_SA_KEY` | Bot identity | Planned |
| 10 | LINE | Messaging API | `LINE_CHANNEL_TOKEN` | Bot identity | Planned |
| 11 | Feishu/Lark | Event Subscription | `FEISHU_APP_ID`, `FEISHU_APP_SECRET` | Bot identity | Planned |
| 12 | **Mattermost** | WebSocket + REST | `MATTERMOST_URL`, `MATTERMOST_TOKEN` | `username` param | Active |
| 13 | **IRC** | IRC protocol | `IRC_SERVER`, `IRC_NICK`, `IRC_PASSWORD` | Nick per agent | Active |
| 14 | Nostr | NIP-04 DMs | `NOSTR_PRIVATE_KEY` | Public key identity | Planned |
| 15 | Twitch | IRC + EventSub | `TWITCH_OAUTH_TOKEN` | Bot username | Planned |

### Tier 2: Event and Webhook Platforms

Inbound-only platforms that push structured events (CI results, issue updates, deployments).

| # | Platform | Event Types | Transport | Credential Env Vars | Status |
|---|----------|-------------|-----------|---------------------|--------|
| 16 | **GitHub** | PR, issue, push, CI, review, release, deployment | Webhooks | `GITHUB_TOKEN`, `GITHUB_WEBHOOK_SECRET` | Active |
| 17 | GitLab | MR, pipeline, issue, push | Webhooks | `GITLAB_TOKEN`, `GITLAB_WEBHOOK_SECRET` | Planned |
| 18 | Bitbucket | PR, push, pipeline | Webhooks | `BITBUCKET_TOKEN` | Planned |
| 19 | Gmail | Email received | Google Pub/Sub | `GMAIL_SA_KEY` | Planned |
| 20 | Outlook | Email received | Graph API webhooks | `OUTLOOK_CLIENT_ID`, `OUTLOOK_CLIENT_SECRET` | Planned |
| 21 | Jira | Issue CRUD, transitions | Webhooks | `JIRA_TOKEN`, `JIRA_WEBHOOK_SECRET` | Planned |
| 22 | Linear | Issue CRUD, cycle changes | Webhooks | `LINEAR_API_KEY`, `LINEAR_WEBHOOK_SECRET` | Planned |
| 23 | Sentry | Error/exception alerts | Webhooks | `SENTRY_DSN` | Planned |
| 24 | PagerDuty | Incident triggered/resolved | Webhooks | `PAGERDUTY_TOKEN` | Planned |
| 25 | Datadog | Alert triggered | Webhooks | `DATADOG_API_KEY` | Planned |
| 26 | Grafana | Alert firing/resolved | Webhooks | `GRAFANA_API_KEY` | Planned |
| 27 | Vercel | Deployment events | Webhooks | `VERCEL_TOKEN` | Planned |
| 28 | Netlify | Deploy events | Webhooks | `NETLIFY_TOKEN` | Planned |
| 29 | AWS SNS | Any SNS topic | HTTP subscription | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | Planned |
| 30 | Stripe | Payment, subscription events | Webhooks | `STRIPE_WEBHOOK_SECRET` | Planned |
| 31 | **Notion** | Page/database changes | Polling | `NOTION_TOKEN` | Active |
| 32 | **Generic Webhook** | Any HTTP POST with JSON | HTTP endpoint | None (public endpoint) | Active |

### Tier 3: IoT and Smart Home

| # | Platform | Events | Transport | Credential Env Vars | Status |
|---|----------|--------|-----------|---------------------|--------|
| 33 | Home Assistant | Device state changes | WebSocket + REST | `HASS_URL`, `HASS_TOKEN` | Planned |
| 34 | **MQTT** | IoT topic messages | MQTT protocol | `MQTT_BROKER`, `MQTT_USERNAME`, `MQTT_PASSWORD` | Active |

### Tier 4: Social and Media

| # | Platform | Events | Transport | Credential Env Vars | Status |
|---|----------|--------|-----------|---------------------|--------|
| 35 | **Twitter/X** | Mentions, DMs | Twitter API v2 | `TWITTER_BEARER_TOKEN` | Active |
| 36 | **Reddit** | Posts/comments | Reddit API | `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET` | Active |
| 37 | **RSS/Atom** | New feed entries | HTTP polling | None | Active |

## Credential Management

Credentials flow from the web UI through encrypted storage into agent environment variables.

```mermaid
flowchart LR
    UI["Web UI<br/>Setup Wizard"] -->|"POST /api/secrets"| SEC["pkg/secret<br/>AES-256-GCM"]
    SEC -->|"agent start"| ENV["Agent env vars"]
    ENV --> AGENT["Agent process<br/>(tmux/Docker)"]
    AGENT -->|"uses credentials"| PLATFORM["Platform API"]

    style SEC fill:#2d5016,stroke:#4ade80
```

1. The user enters platform credentials in the web UI setup wizard.
2. `POST /api/secrets` encrypts them with AES-256-GCM via `pkg/secret`.
3. When an agent starts, bc reads workspace secrets and injects them as environment variables.
4. The agent's system prompt includes instructions on which env vars are available.
5. The agent calls platform APIs directly using those credentials.

Secrets are never stored in `settings.json`, never transmitted via SSE events, and never exposed in API responses.

| Platform | Setup Steps |
|----------|-------------|
| **Slack** | Create app at api.slack.com > Enable Socket Mode > Add scopes (`channels:read`, `chat:write`, `connections:write`) > Copy bot token + app token > Invite bot to channels |
| **Telegram** | Message @BotFather `/newbot` > Copy bot token > Add bot to groups > Disable privacy mode (optional, for reading all group messages) |
| **Discord** | Create app at discord.com/developers > Enable `MESSAGE_CONTENT` intent > Copy bot token > Generate invite URL with required permissions > Add bot to server |
| **GitHub** | Create GitHub App or configure repository webhook > Select events (PR comments, reviews, issues, pushes) > Copy token and webhook secret |

## Agent Identity

Each platform has different support for per-message identity. Some allow setting a username per API call; others require the agent to prefix its name in the message body.

| Platform | Per-Message Identity? | Mechanism |
|----------|----------------------|-----------|
| Slack | Yes | `username` parameter in `chat.postMessage` |
| Discord | Yes | `username` parameter in webhook execute |
| Mattermost | Yes | Bot API username parameter |
| Telegram | No (fixed bot name) | Agent prefixes message with `[agent-name]: ` |
| WhatsApp | No (fixed number) | Agent prefixes message text |
| Signal | No (fixed number) | Agent prefixes message text |
| GitHub | App-level | Comments appear as the GitHub App / token owner |
| Gmail | No (account holder) | Sends as authenticated user |

Identity instructions are injected into the agent's system prompt at startup:

```markdown
## Platform Credentials
You have access to these platform credentials via environment variables:
- SLACK_BOT_TOKEN: Use Slack API. Set `username` param to your agent name
  (available as BC_AGENT_ID env var) for identity.
- TELEGRAM_BOT_TOKEN: Use Telegram Bot API. Prefix messages with your agent name.
```

## Subscriptions

### Database Schema

Five tables track subscriptions, delivery, gateway state, and channel mappings.

```mermaid
erDiagram
    notify_subscriptions {
        INTEGER id PK
        TEXT channel
        TEXT agent
        INTEGER mention_only
        TEXT created_at
    }

    notify_messages {
        INTEGER id PK
        TEXT channel
        TEXT sender
        TEXT content
        TEXT created_at
    }

    notify_delivery_log {
        INTEGER id PK
        TEXT logged_at
        TEXT channel
        TEXT agent
        TEXT status
        TEXT error
        TEXT preview
    }

    notify_subscriptions ||--o{ notify_delivery_log : "deliveries per subscription"

    notify_gateways {
        TEXT name PK
        INTEGER enabled
        INTEGER connected
        TEXT last_seen_at
        TEXT updated_at
    }

    notify_channels {
        TEXT bc_channel PK
        TEXT platform
        TEXT platform_id
        TEXT updated_at
    }
```

### SQL DDL

```sql
CREATE TABLE IF NOT EXISTS notify_subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    channel      TEXT NOT NULL,          -- "slack:engineering", "telegram:bc-dev"
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(channel, agent)
);

CREATE TABLE IF NOT EXISTS notify_messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    channel    TEXT NOT NULL,
    sender     TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS notify_delivery_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    channel   TEXT NOT NULL,
    agent     TEXT NOT NULL,
    status    TEXT NOT NULL CHECK(status IN ('delivered', 'failed', 'pending')),
    error     TEXT,
    preview   TEXT
);

CREATE TABLE IF NOT EXISTS notify_gateways (
    name         TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    connected    INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS notify_channels (
    bc_channel   TEXT PRIMARY KEY,
    platform     TEXT NOT NULL,
    platform_id  TEXT NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
```

| Table | Purpose |
|-------|---------|
| `notify_subscriptions` | Maps agents to notification channels. `UNIQUE(channel, agent)`. |
| `notify_messages` | Stores inbound messages for the web UI activity feed. |
| `notify_delivery_log` | Records every delivery attempt per agent (delivered/failed/pending). |
| `notify_gateways` | Persists gateway adapter state (enabled, connected) across restarts. |
| `notify_channels` | Maps mycel channel names to platform channel IDs for routing. |

### Retention Policy

| Rule | Value |
|------|-------|
| Max entries per source | 1,000 (oldest pruned on insert) |
| TTL | 7 days (entries older than 7d pruned on schedule) |
| `delivery_log` retention | 1,000 per source, same TTL |

Pruning runs on a periodic timer (every hour). Both limits apply -- whichever triggers first.

## Dispatch

The dispatch pipeline handles self-skip filtering, mention filtering, and delivery logging in a single pass.

```mermaid
flowchart TD
    A[Inbound event from platform] --> B[Adapter calls onMessage handler]
    B --> C[Manager dispatches in goroutine]
    C --> D["notify.Service.Dispatch()"]
    D --> E[Save message to notification_log]
    D --> F[Load subscribers from subscriptions]
    D --> G["Extract @mentions from content"]

    F --> H{For each subscriber}
    H --> I{Self-skip: sender == agent?}
    I -->|Yes| J[Skip]
    I -->|No| K{mention_only enabled?}
    K -->|Yes| L{Agent @mentioned?}
    L -->|No| J
    L -->|Yes| M[Deliver]
    K -->|No| M

    M --> N[tmux send-keys JSON payload]
    M --> O[Log delivery to delivery_log]

    D --> P[Publish gateway.message to SSE hub]
    D --> Q[Prune old delivery log entries]
```

**Self-skip** prevents agents from receiving their own messages echoed back by the platform. Each adapter extracts the sender with a single field lookup (`event.User` for Slack, `message.From.UserName` for Telegram, `message.Author.ID` for Discord). The `[platform] ` prefix is stripped before comparison.

**Mention filtering** controls whether an agent receives all messages or only those containing `@agent-name`:

| Setting | Behavior | Use Case |
|---------|----------|----------|
| `mention_only = false` (default) | Agent receives all messages in the channel | Small or focused channels |
| `mention_only = true` | Agent receives messages containing `@<agent-name>` | Noisy channels |

Settings are per-agent, per-channel.

## Web UI

### Notifications Settings Page

```
+-------------------+------------------------------------------+----------------------+
| GATEWAYS          |  slack:engineering                        | AGENTS               |
|                   |                                          |                      |
| > Slack       (3) |  10:32  alice                            | * eng-01  (engineer) |
|   #engineering  <-|  Can someone review PR #428?             |   [x] @mention only  |
|   #all-bc         |                                          |   [Remove]           |
|   #infra          |  10:32  bob                              |                      |
|                   |  @eng-01 take a look                     | * eng-02  (engineer) |
| > Telegram    (1) |  -> delivered to eng-01                  |   [ ] all messages   |
|   bc-dev          |                                          |   [Remove]           |
|                   |  10:35  eng-01                            |                      |
| > Discord     (1) |  Looking now, will review                | o lead-01 (lead)     |
|   #general        |  -> relayed to eng-02                    |   [Add]              |
|                   |                                          |                      |
| > GitHub      (0) |                                          | o root    (manager)  |
|   [Setup ->]      |                                          |   [Add]              |
|                   |                                          |                      |
| + Connect app     |                                          | * = online  o = off  |
+-------------------+------------------------------------------+----------------------+
```

| Panel | Content |
|-------|---------|
| **Left sidebar** | Gateway dropdowns with channel lists. Unconnected gateways show a "Setup" link. |
| **Main area** | Activity feed with delivery status badges. Polls every 5s and receives live SSE updates via `/api/events`. |
| **Right panel** | Agent list with online indicators, role badges, and `@mention` toggle. |

### SSE Events

| Event | Trigger |
|-------|---------|
| `gateway.message` | New inbound message -- appends to activity feed |

### Component Tree

```mermaid
graph TD
    NS[NotificationSidebar.tsx] --> GF[GatewayFeed.tsx]
    NS --> SP[SubscriptionPanel.tsx]
    NS --> SW[SetupWizard.tsx]

    GF --> ML[MessageList.tsx]
    GF --> AA[AgentAvatar.tsx]
    GF --> MU[messageUtils.ts]
```

## Adding a New Adapter

1. Create the adapter file at `pkg/gateway/<platform>/<platform>.go`.

2. Implement `NotificationAdapter`:

```go
package myplatform

import (
    "context"
    "github.com/rpuneet/mycel/pkg/gateway"
)

type Adapter struct {
    token string
}

func New(token string) *Adapter {
    return &Adapter{token: token}
}

func (a *Adapter) Name() string { return "myplatform" }

func (a *Adapter) Start(ctx context.Context, handler func(gateway.Notification)) error {
    for {
        select {
        case <-ctx.Done():
            return nil
        case event := <-a.events:
            raw, _ := json.Marshal(event)
            handler(gateway.Notification{
                Channel:   extractChannel(event),
                Platform:  "myplatform",
                Sender:    extractSender(event),
                Timestamp: time.Now(),
                Raw:       raw,
            })
        }
    }
}

func (a *Adapter) Stop() error              { return a.conn.Close() }
func (a *Adapter) HTTPHandler() http.Handler { return nil }
func (a *Adapter) Channels() []gateway.ChannelInfo { return a.discovered }
func (a *Adapter) Status() gateway.AdapterStatus {
    return gateway.AdapterStatus{Connected: a.connected, MessageCount: a.count}
}
```

3. Register the adapter in `server/build_services.go` (the `buildGatewayManager` function):

```go
adapter := myplatform.New(token)
gatewayMgr.Register(adapter)
```

4. Add credential env vars to the [Platform Integrations](#platform-integrations) table and the setup wizard.

5. Update the agent system prompt template with identity instructions for the new platform.

## API Reference

All endpoints are served by the mycel server at `http://127.0.0.1:9374`. Localhost-only by default; Bearer auth applies when the server is started with `--api-key`/`BC_API_KEY`.

### Gateway Management

```
GET    /api/gateways                                              -- list all gateways + status
PATCH  /api/gateways/{platform}                                   -- update tokens/settings
GET    /api/gateways/{platform}/health                            -- live connection probe
GET    /api/gateways/{platform}/channels                          -- discovered channels
GET    /api/gateways/{platform}/channels/{channel}                -- channel detail + subscribers
```

There is no outbound send endpoint — `POST .../channels/{channel}/send`
has been removed (unknown sub-routes return `404`). Outbound messages are
sent by agents directly against each platform's API (see the
[Outbound cookbook](#outbound-cookbook)).

### Agent Subscription Management

Gateway-scoped routes:

```text
GET    /api/notify/subscriptions                         -- list all subscriptions
POST   /api/notify/subscriptions                         -- subscribe agent {channel, agent, mention_only}
GET    /api/notify/subscriptions/{channel}               -- list subscribers for channel
DELETE /api/notify/subscriptions/{channel}?agent={agent} -- unsubscribe agent
PATCH  /api/notify/subscriptions/{channel}               -- update subscription (toggle mention_only)
GET    /api/notify/activity/{channel}                    -- delivery log entries
GET    /api/gateways/activity                            -- aggregated activity across all gateways
```

### Activity Feed

```
GET    /api/gateways/activity                                     -- aggregate activity across all gateway channels
GET    /api/channels                                              -- legacy channel list (gateway + subscribed)
GET    /api/channels/{name}/history                               -- legacy message history
```

## Package Reference

| Package | Purpose |
|---------|---------|
| [`pkg/gateway/`](https://github.com/rpuneet/mycel/tree/main/pkg/gateway) | Adapter interface, Manager, platform adapters (Slack, Telegram, Discord) |
| [`pkg/notify/`](https://github.com/rpuneet/mycel/tree/main/pkg/notify) | Notification types, Store (SQLite/Postgres), Service (dispatch + subscription management) |
| `pkg/secret/` | AES-256-GCM encrypted credential storage |
| `server/handlers/` | REST API handlers for gateway and subscription management |

## What's Next

- [Agent Lifecycle](explanation/agents.md) -- how agents start, receive credentials, and run
- [MCP Protocol](explanation/mcp.md) -- Model Context Protocol integration
