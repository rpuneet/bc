# pkg/gateway

External platform adapters for the bc notification system.

## Overview

This package defines the `NotificationAdapter` interface and `Manager` that orchestrate connections to 34 external platforms. Each adapter connects to a platform, receives inbound events, and forwards them to `pkg/notify` for dispatch to subscribed agents.

## Key Types

- **`NotificationAdapter`** — interface each platform implements (Name, Type, Start, Stop, HTTPHandler, Channels, Status)
- **`Notification`** — normalized inbound event (Channel, Platform, Sender, Content, Mentions, Timestamp, Raw json.RawMessage)
- **`Manager`** — registers adapters, discovers channels, routes inbound events
- **`ChannelInfo`** — a channel/group discovered on a platform (ID, Name, Platform)
- **`AdapterStatus`** — connection state (Connected, Error, BotName, LastMessageAt, MessageCount)
- **`AdapterType`** — socket, webhook, or poll

## Adapters

| Platform | Directory | Transport |
|----------|-----------|-----------|
| Slack | `slack/` | Socket Mode (WebSocket) |
| Telegram | `telegram/` | Bot API long-polling |
| Discord | `discord/` | Gateway WebSocket |
| GitHub | `github/` | Webhook |
| GitLab | `gitlab/` | Webhook |
| Sentry | `sentry/` | Webhook |
| PagerDuty | `pagerduty/` | Webhook |
| Datadog | `datadog/` | Webhook |
| Grafana | `grafana/` | Webhook |
| RSS | `rss/` | Poll |
| Notion | `notion/` | Poll |
| WhatsApp | `whatsapp/` | Socket |
| Matrix | `matrix/` | Poll |
| + 21 more | see subdirectories | various |

## Architecture

See [docs/architecture/notifications.md](../../docs/architecture/notifications.md) for the full notification architecture, including message flow diagrams, credential injection, and how to add new adapters.
