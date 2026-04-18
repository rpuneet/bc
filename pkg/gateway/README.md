# pkg/gateway

External platform adapters for the notification channel system.

## Overview

This package defines the `Adapter` interface and `Manager` that orchestrate connections to external platforms (Slack, Telegram, Discord, etc.). Each adapter connects to a platform, receives inbound events, and forwards them to `pkg/notify` for dispatch to subscribed agents.

## Key Types

- **`Adapter`** -- interface that each platform implements (Start, Stop, Channels)
- **`Manager`** -- registers adapters, discovers channels, routes inbound events
- **`InboundMessage`** -- normalized inbound event from a platform
- **`ExternalChannel`** -- a channel/group discovered on a platform

## Adapters

| Platform | Directory | Transport |
|----------|-----------|-----------|
| Slack | `slack/` | Socket Mode (WebSocket) |
| Telegram | `telegram/` | Bot API long-polling |
| Discord | `discord/` | Gateway WebSocket |

## Architecture

See [docs/architecture/channels.md](../../docs/architecture/channels.md) for the full channel architecture, including message flow diagrams, credential injection, and how to add new adapters.
