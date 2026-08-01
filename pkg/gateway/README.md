# pkg/gateway

External platform adapters for the mycel notification system.

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

## Inbound-only by design

Gateway adapters are **inbound-only**. They receive messages from
external platforms and forward them to `pkg/notify` for dispatch. They
are not a generic outbound abstraction.

**Interface contract**: the `NotificationAdapter` interface itself is
inbound-only — it does **not** require a `Send` method. The required
surface is `Name`, `Type`, `Start`, `Stop`, `HTTPHandler`, `Channels`,
and `Status`. Outbound message sending is performed directly by
agents and handlers using credentials injected from the workspace
secret store (e.g. `$SLACK_BOT_TOKEN`, `$TELEGRAM_BOT_TOKEN`). New
adapters should **not** implement `Send`.

The `Send` methods on the Slack, Telegram, and Discord adapters are a
legacy exception, retained as a convenience for the daemon's internal notify
dispatch path. They are not part of the `NotificationAdapter` contract.
Most adapters (WhatsApp, Matrix, IRC, Signal, etc.) deliberately have
no `Send` method — that is correct, not a missing feature.

See [docs/explanation/notifications.md](../../docs/explanation/notifications.md#gateway-adapters-are-inbound-only)
for the full rationale, message flow diagrams, credential injection,
and how to add new adapters.

## Architecture

See [docs/explanation/notifications.md](../../docs/explanation/notifications.md) for the full notification architecture, including message flow diagrams, credential injection, and how to add new adapters.
