# Channel Architecture

> **Status:** Active | **Issue:** [#3006](https://github.com/gh-curious-otter/bc/issues/3006) | **Supersedes:** [channels-revamp proposal](../proposals/channels-revamp.md)

## Overview

Channels are **inbound notification-only gateways** that connect external platforms to bc agents. bc receives platform events and forwards the raw JSON payload to subscribed agents. Agents handle all outbound communication themselves using injected credentials.

**Key principles:**
1. bc handles **inbound only** -- no Send/outbound implementation
2. **Raw JSON passthrough** -- platform payloads forwarded as-is, agents parse them
3. **Agent handles outbound** using platform credentials injected as environment variables
4. **Identity via instructions** -- agent system prompt tells it how to identify itself per platform

```
                        INBOUND ONLY
                        ============

  +----------+    +----------+    +----------+    +----------+
  |  Slack   |    | Telegram |    |  GitHub  |    | Discord  |
  |  events  |    |  events  |    |  events  |    |  events  |
  +----+-----+    +----+-----+    +----+-----+    +----+-----+
       |               |               |               |
       v               v               v               v
  +----+-----+    +----+-----+    +----+-----+    +----+-----+
  |  Slack   |    | Telegram |    |  GitHub  |    | Discord  |
  | Adapter  |    | Adapter  |    | Adapter  |    | Adapter  |
  +----+-----+    +----+-----+    +----+-----+    +----+-----+
       |               |               |               |
       +-------+-------+-------+-------+
               |
               v
       +-------+--------+
       | notify.Dispatch |-----> SSE Hub (Web UI)
       +-------+--------+
               |
       +-------+--------+
       | Subscription   |
       | Filter         |
       | (self-skip,    |
       |  mention_only) |
       +-------+--------+
               |
       +---+---+---+
       |   |       |
       v   v       v
     eng-01 eng-02 lead-01
     (tmux send-keys)

  Agent outbound: agent uses env var credentials
  (SLACK_BOT_TOKEN, GITHUB_TOKEN, etc.) to call
  platform APIs directly.
```

## NotificationAdapter Interface

Each platform adapter implements a minimal interface with no outbound methods:

```go
type NotificationAdapter interface {
    // Name returns the platform identifier ("slack", "telegram", "github").
    Name() string

    // Platform returns the platform identifier (may differ from Name for
    // multi-instance adapters, e.g., "telegram" for "telegram:my-bot").
    Platform() string

    // Start connects to the platform and begins receiving events.
    // Calls handler for each inbound notification. Blocks until ctx is canceled.
    Start(ctx context.Context, handler func(Notification)) error

    // Stop gracefully disconnects from the platform.
    Stop() error

    // Channels returns all channels/groups the bot has access to.
    Channels() []ChannelInfo
}
```

There is no `Send()`, `SendFile()`, or `React()` method. Outbound is the agent's responsibility.

Each adapter is ~50-100 lines: connect to platform, extract sender field, forward raw JSON.

## Notification Data Model

A minimal envelope wrapping the raw platform payload:

```go
type Notification struct {
    Channel   string          // "slack:eng", "github:bc", "telegram:chat"
    Platform  string          // "slack", "github", "telegram"
    Sender    string          // extracted for self-skip only
    Mentions  []string        // extracted via regex for mention_only filter
    Timestamp time.Time
    Raw       json.RawMessage // entire platform payload, no parsing
}
```

No `Content`, `Event`, `ThreadID`, `MessageID`, or `Metadata` fields. The agent reads the raw JSON directly and decides how to act.

## Message Flow (Inbound Only)

```
1. Platform event arrives (WebSocket, webhook, long-poll)
2. Adapter extracts: sender (for self-skip), mentions (regex @name)
3. Adapter wraps entire payload in Notification{Raw: <original JSON>}
4. Adapter calls handler(notification)
5. notify.Dispatch() receives the notification
6. Load subscribers for this channel from DB
7. For each subscriber:
   a. Self-skip: if sub.Agent == sender, skip
   b. Mention filter: if mention_only=ON and agent not @mentioned, skip
   c. Deliver: tmux send-keys with JSON notification
   d. Log delivery result to DB
8. Publish to SSE Hub for Web UI live updates
```

## Credential Injection

When an agent starts, bc injects platform credentials from workspace secrets as environment variables:

```
BC_AGENT_ID=zen-zebra
SLACK_BOT_TOKEN=xoxb-...
TELEGRAM_BOT_TOKEN=123456:ABC...
GITHUB_TOKEN=ghp_...
DISCORD_BOT_TOKEN=...
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
```

Credentials are stored encrypted via `pkg/secret` (AES-256-GCM). No plaintext tokens in settings.json.

## Agent Identity Per Platform

bc injects identity instructions into the agent's system prompt / CLAUDE.md:

```markdown
## Platform Credentials
You have access to these platform credentials via environment variables:
- SLACK_BOT_TOKEN: Use Slack API. Set `username` param to your agent name (BC_AGENT_ID).
- TELEGRAM_BOT_TOKEN: Use Telegram Bot API. Prefix messages with [your-agent-name].
- DISCORD_WEBHOOK_URL: Use Discord webhook API. Set `username` param to your agent name.
- GITHUB_TOKEN: Use GitHub API. Comments appear as the token owner.
```

Per-platform identity support:

| Platform | Per-message identity? | Approach |
|----------|----------------------|----------|
| Slack | Yes | `username` param in chat.postMessage |
| Discord | Yes | `username` param in webhook execute |
| Mattermost | Yes | Bot API username param |
| Telegram | No (fixed bot name) | Prefix `[agent-name]: message` |
| GitHub | App-level | One bot identity (natural for PR comments) |
| Gmail | No (account holder) | Sends as authenticated user |

## Self-Skip and Mention Filtering

**Self-skip:** Each adapter extracts `Sender` (one field lookup per platform). `notify.Dispatch()` skips delivery when `sub.Agent == sender`.

**Mention filter:** Regex `@[a-zA-Z][a-zA-Z0-9_-]*` applied across raw JSON bytes. Used for the `mention_only` subscription toggle.

**Bot filtering:** Adapter skips messages from its own bot ID before forwarding:
- Slack: check `BotID` field
- Telegram: check `is_bot` field
- Discord: check `author.bot` field

## Subscription Model

Agents subscribe to channels. Each subscription has a `mention_only` toggle:

| Setting | Behavior | Use Case |
|---------|----------|----------|
| OFF (default) | Agent receives all messages in the channel | Small/focused channels |
| ON | Agent only receives when `@<agent-name>` appears | Noisy channels |

Per-agent, per-channel. Example: `eng-01` has mention_only=ON for `slack:all-bc` but OFF for `slack:engineering`.

### Database Schema

```sql
CREATE TABLE IF NOT EXISTS notify_subscriptions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    channel      TEXT NOT NULL,          -- "slack:engineering"
    agent        TEXT NOT NULL,
    mention_only INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    UNIQUE(channel, agent)
);

CREATE TABLE IF NOT EXISTS notify_delivery_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    logged_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    channel   TEXT NOT NULL,
    agent     TEXT NOT NULL,
    status    TEXT NOT NULL CHECK(status IN ('delivered', 'failed', 'pending')),
    error     TEXT,
    preview   TEXT  -- first 120 chars for debugging
);

CREATE TABLE IF NOT EXISTS notify_gateways (
    name         TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    connected    INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT,
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
```

Delivery log pruned to last 1000 entries per channel.

## Web UI Design

### Channels Page Layout

```
+------------------+------------------------------------------+----------------------+
| GATEWAYS         |  slack:engineering                        | AGENTS               |
|                  |                                          |                      |
| > Slack     (3)  |  10:32  alice                            | * eng-01  (engineer) |
|   #engineering <-|  Can someone review PR #428?             |   [@] mention only   |
|   #all-bc        |                                          |                      |
|   #infra         |  10:32  bob                              | * eng-02  (engineer) |
|                  |  @eng-01 take a look                     |   [ ] all messages   |
| > Telegram  (1)  |  -> delivered to eng-01                  |                      |
|   bc-dev         |                                          | o lead-01 (lead)     |
|                  |  10:35  eng-01                            |   [Add]              |
| > Discord   (1)  |  Looking now, will review                |                      |
|   #general       |  -> relayed to eng-02                    | * = online  o = off  |
|                  |                                          |                      |
| > GitHub    (0)  |                                          |                      |
|   [Setup ->]     |                                          |                      |
|                  |                                          |                      |
| + Connect app    |                                          |                      |
+------------------+------------------------------------------+----------------------+
```

| Panel | Content |
|-------|---------|
| Left sidebar | Gateway dropdowns with channel lists. Unconnected gateways show Setup link. |
| Main area | Activity feed -- notifications with delivery status badges. |
| Right panel | Agent subscriptions -- add/remove agents, mention_only toggle. |

### SSE Events

| Event | Trigger |
|-------|---------|
| `gateway.message` | New notification received |
| `gateway.delivery` | Delivery status update |
| `gateway.connected` | Gateway connected |
| `gateway.disconnected` | Gateway lost connection |

## Integration List

47 platforms across 4 tiers. Full details in [issue #3006](https://github.com/gh-curious-otter/bc/issues/3006).

### Tier 1 -- Chat Channels (19 platforms)

Bidirectional messaging. Agents receive chat messages and respond via platform API.

| # | Platform | Transport | In bc? |
|---|----------|-----------|--------|
| 1 | Slack | Socket Mode (WebSocket) | Yes |
| 2 | Telegram | Bot API long-polling | Yes |
| 3 | Discord | Gateway WebSocket | Stub |
| 4 | WhatsApp | Cloud API webhooks | No |
| 5 | Signal | signal-cli bridge | No |
| 6 | iMessage | BlueBubbles server | No |
| 7 | Matrix | Client-Server API | No |
| 8 | Microsoft Teams | Bot Framework + Graph API | No |
| 9 | Google Chat | Chat API + Pub/Sub | No |
| 10 | LINE | Messaging API | No |
| 11 | Feishu/Lark | Event Subscription API | No |
| 12 | Mattermost | WebSocket + REST API | No |
| 13 | IRC | IRC protocol | No |
| 14 | Nostr | NIP-04 encrypted DMs | No |
| 15 | QQ | QQ Bot API | No |
| 16 | Zalo | Zalo OA API | No |
| 17 | Twitch | IRC + EventSub | No |
| 18 | Synology Chat | Incoming/Outgoing webhooks | No |
| 19 | Nextcloud Talk | Nextcloud OCS API | No |

### Tier 2 -- Event/Webhook Channels (20 platforms)

Inbound notifications from platform events (not chat). Agents respond via platform API.

| # | Platform | Event Types | Transport |
|---|----------|-------------|-----------|
| 20 | GitHub | PR, issue, push, CI, release, deployment | Webhooks |
| 21 | GitLab | MR, pipeline, issue, push | Webhooks |
| 22 | Bitbucket | PR, push, pipeline | Webhooks |
| 23 | Gmail | Email received, label change | Google Pub/Sub |
| 24 | Outlook | Email received | Microsoft Graph webhooks |
| 25 | Jira | Issue created/updated/transitioned | Webhooks |
| 26 | Linear | Issue created/updated, cycle changes | Webhooks |
| 27 | Sentry | Error/exception alerts | Webhooks |
| 28 | PagerDuty | Incident triggered/resolved | Webhooks |
| 29 | Datadog | Alert triggered | Webhooks |
| 30 | Grafana | Alert firing/resolved | Webhooks |
| 31 | Vercel | Deployment created/ready/error | Webhooks |
| 32 | Netlify | Deploy succeeded/failed | Webhooks |
| 33 | AWS SNS | Any SNS topic notification | HTTP subscription |
| 34 | Stripe | Payment succeeded/failed, subscription events | Webhooks |
| 35 | Shopify | Order created, product updated | Webhooks |
| 36 | Notion | Page updated, database changed | Polling |
| 37 | Obsidian | Vault file changed | Filesystem watcher |
| 38 | Generic Webhook | Any HTTP POST with JSON payload | HTTP endpoint |
| 39 | Cron | Scheduled time triggers | Internal timer |

### Tier 3 -- IoT & Smart Home (3 platforms)

| # | Platform | Events | Transport |
|---|----------|--------|-----------|
| 40 | Home Assistant | Device state changes, automations | WebSocket + REST |
| 41 | Philips Hue | Light state, motion sensors | Hue Bridge API |
| 42 | MQTT | Any IoT topic message | MQTT protocol |

### Tier 4 -- Social & Media (5 platforms)

| # | Platform | Events | Transport |
|---|----------|--------|-----------|
| 43 | Twitter/X | Mentions, DMs, timeline | Twitter API v2 |
| 44 | Reddit | Post/comment in subreddit | Reddit API |
| 45 | YouTube | New video, comment, live chat | YouTube Data API |
| 46 | Spotify | Playback state changes | Spotify Web API |
| 47 | RSS/Atom | New feed entries | HTTP polling |

## How to Add a New Channel

Step-by-step guide for implementing a new platform adapter.

### 1. Create the adapter package

```
pkg/gateway/<platform>/
    adapter.go      -- NotificationAdapter implementation
    adapter_test.go -- unit tests
```

### 2. Implement NotificationAdapter

```go
package myplatform

import (
    "context"
    "encoding/json"

    "github.com/rpuneet/bc/pkg/gateway"
    "github.com/rpuneet/bc/pkg/notify"
)

type Adapter struct {
    token    string
    channels []notify.ChannelInfo
}

func New(token string) *Adapter {
    return &Adapter{token: token}
}

func (a *Adapter) Name() string     { return "myplatform" }
func (a *Adapter) Platform() string { return "myplatform" }

func (a *Adapter) Start(ctx context.Context, handler func(notify.Notification)) error {
    // 1. Connect to platform (WebSocket, long-poll, webhook listener)
    // 2. For each incoming event:
    //    - Extract sender name (for self-skip)
    //    - Extract @mentions via regex (for mention_only filter)
    //    - Skip messages from own bot ID
    //    - Marshal entire platform payload to json.RawMessage
    //    - Call handler(Notification{...})
    // 3. Block until ctx.Done()
    return nil
}

func (a *Adapter) Stop() error {
    // Disconnect from platform
    return nil
}

func (a *Adapter) Channels() []notify.ChannelInfo {
    return a.channels
}
```

### 3. Register the adapter

In `server/` startup code, register the adapter with the gateway manager:

```go
if token := secrets.Get("GATEWAY_MYPLATFORM_TOKEN"); token != "" {
    adapter := myplatform.New(token)
    manager.Register(adapter)
}
```

### 4. Store credentials

Add the platform token to workspace secrets:

```bash
bc secret set GATEWAY_MYPLATFORM_TOKEN <token>
```

### 5. Test

- Verify the adapter connects and discovers channels
- Send a test message on the platform and confirm notification delivery
- Subscribe an agent and verify tmux send-keys delivery
- Test self-skip (agent's own messages not echoed back)
- Test mention_only filtering

## REST API

```
GET    /api/gateways                                              -- list + status
POST   /api/gateways                                              -- connect
PATCH  /api/gateways/{gateway}                                    -- update settings
DELETE /api/gateways/{gateway}                                    -- disconnect
GET    /api/gateways/{gateway}/health                             -- live probe
GET    /api/gateways/{gateway}/channels                           -- discovered channels
GET    /api/gateways/{gateway}/channels/{channel}                 -- detail + agents
POST   /api/gateways/{gateway}/channels/{channel}/agents          -- subscribe agent
DELETE /api/gateways/{gateway}/channels/{channel}/agents/{agent}  -- unsubscribe
PATCH  /api/gateways/{gateway}/channels/{channel}/agents/{agent}  -- toggle mention_only
GET    /api/gateways/{gateway}/channels/{channel}/activity        -- delivery log
```

## What Changed from the Previous Design

The [channels-revamp proposal](../proposals/channels-revamp.md) described a **bidirectional** gateway with `Adapter.Send()` and `FileSender` interfaces. The new architecture removes all outbound responsibility from bc:

| Previous Design | New Architecture |
|----------------|------------------|
| `Adapter.Send()` routes outbound messages | No outbound -- agent calls platform API directly |
| `FileSender.SendFile()` for file uploads | No file upload -- agent uses platform file API |
| `InboundMessage` with parsed fields | `Notification` with `Raw json.RawMessage` |
| bc parses message content, extracts types | Raw passthrough -- agent parses the payload |
| Gateway Manager routes both directions | Manager handles inbound dispatch only |
| Complex channel mapping and persistence | Simple subscription-based routing |
