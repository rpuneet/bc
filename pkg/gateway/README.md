# pkg/gateway

External platform adapters for the bc notification system.

## Overview

This package defines the `NotificationAdapter` interface and `Manager` that orchestrate connections to 34 external platforms. Each adapter follows a 3-part pattern:

1. **Connect** -- Authenticate with the platform (token, QR code, OAuth). Store only credentials + session state.
2. **Listen** -- Receive real-time events and forward them as `Notification{Raw JSON}` to `pkg/notify` for dispatch.
3. **API Proxy** -- Expose the platform's native API via `/api/gateways/{platform}/api/*` so agents can send messages, upload files, etc.

There are no `Send`/`SendFile`/`React` methods on the adapter interface. Agents interact with platforms through the API proxy.

## Key Types

- **`NotificationAdapter`** -- interface each platform implements (Name, Type, Start, Stop, HTTPHandler, Channels, Status)
- **`Notification`** -- inbound event with raw JSON payload (Channel, Platform, Sender, Content, Mentions, Timestamp, Raw)
- **`Manager`** -- registers adapters, discovers channels, routes inbound events
- **`ChannelInfo`** -- a channel/group from the platform API (ID, Name, Platform)
- **`AdapterStatus`** -- connection state (Connected, Error, BotName, LastMessageAt, MessageCount)
- **`AdapterType`** -- socket, webhook, or poll

## Working Adapters

| Platform | Directory | Transport | SDK / Library |
|----------|-----------|-----------|---------------|
| Slack | `slack/` | Socket Mode (WebSocket) | `slack-go/slack` |
| Telegram | `telegram/` | Bot API long-polling | `go-telegram-bot-api` |
| Discord | `discord/` | Gateway WebSocket | `bwmarrin/discordgo` |
| WhatsApp | `whatsapp/` | Multi-device protocol | `whatsmeow` (QR pairing) |
| IRC | `irc/` | IRC protocol | `ergochat/irc-go` |
| MQTT | `mqtt/` | MQTT protocol | `eclipse/paho.mqtt.golang` |
| Mattermost | `mattermost/` | WebSocket | `gorilla/websocket` |
| GitHub | `github/` | Webhook | Webhook handler |
| Generic Webhook | `webhook/` | HTTP POST | Webhook receiver |
| RSS | `rss/` | HTTP polling | HTTP client |
| Matrix | `matrix/` | HTTP polling | HTTP client |
| Reddit | `reddit/` | HTTP polling | HTTP client |
| Twitter | `twitter/` | HTTP polling | HTTP client |

## Stub Adapters (Coming Soon)

Signal, Line, Feishu, Google Chat, MS Teams, GitLab, Bitbucket, Jira, Linear, Sentry, PagerDuty, Datadog, Grafana, Stripe, Vercel, Netlify, Notion, Twitch, Home Assistant, Nostr, iMessage.

## Architecture

See [docs/architecture/notifications.md](../../docs/architecture/notifications.md) for the full notification architecture, including the 3-part adapter pattern, message flow diagrams, and how to add new adapters.
