# pkg/gateway

External platform adapters for the notification channel system.

## Overview

This package defines the `Adapter` interface and `Manager` that orchestrate connections to external platforms (Slack, Telegram, Discord, etc.). Each adapter connects to a platform, receives inbound events, and forwards them to `pkg/notify` for dispatch to subscribed agents.

## Key Types

- **`NotificationAdapter`** -- current interface that each platform implements (Name, Type, Start, Stop, HTTPHandler, Channels, Status)
- **`MessageSender`** -- optional interface for adapters that support outbound messaging (Send)
- **`Manager`** -- registers adapters, discovers channels, routes inbound events
- **`Notification`** -- normalized inbound event from a platform (Channel, Platform, Sender, Mentions, Timestamp, Raw)
- **`ChannelInfo`** -- a channel/group discovered on a platform (ID, Name, Platform)
- **`AdapterStatus`** -- connection state reported to the web UI (Connected, Error, BotName, LastMessageAt, MessageCount)
- **`Adapter`** -- legacy interface (deprecated, kept during migration)
- **`InboundMessage`** -- legacy normalized message (deprecated, use Notification)
- **`ExternalChannel`** -- legacy channel type (deprecated, use ChannelInfo)

## Adapters

| Platform | Directory | Transport |
|----------|-----------|-----------|
| Slack | `slack/` | Socket Mode (WebSocket) |
| Telegram | `telegram/` | Bot API long-polling |
| Discord | `discord/` | Gateway WebSocket |

## Architecture

See [docs/architecture/notifications.md](../../docs/architecture/notifications.md) for the full notification architecture, including message flow diagrams, credential injection, and how to add new adapters.
