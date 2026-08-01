# pkg/notify

Notification dispatch and subscription management for mycel.

## Overview

This package receives inbound events from `pkg/gateway` adapters and routes them to subscribed agents via `tmux send-keys`. Gateway adapters forward raw platform payloads as `Notification{Raw JSON}` -- this package handles dispatch, filtering, and delivery logging. Agents respond to platforms through the gateway API proxy, not through this package.

## Key Types

- **`Service`** -- dispatch core: receives notifications, queries subscribers, delivers via tmux, publishes to SSE hub
- **`Store`** -- SQLite/Postgres persistence for subscriptions, delivery log, messages, and gateway state
- **`Notification`** -- JSON payload sent to agents (includes raw platform payload)
- **`Subscription`** -- ties an agent to a channel with `mention_only` flag

## Dispatch Pipeline

1. Adapter calls `Service.Dispatch()` with a `Notification` containing the raw platform JSON
2. Service saves the message to `notify_messages` for the activity feed
3. Service queries `notify_subscriptions` for matching agents
4. For each subscriber: self-skip filter (sender == agent?), then mention filter
5. Deliver via `tmux send-keys` with the full JSON payload
6. Log delivery result to `notify_delivery_log`
7. Publish `gateway.message` event to SSE hub for web UI

## Database Tables

| Table | Purpose |
|-------|---------|
| `notify_subscriptions` | Agent-to-channel mappings |
| `notify_delivery_log` | Delivery attempt records |
| `notify_messages` | Inbound messages for activity feed |
| `notify_gateways` | Gateway connection state |
| `notify_channels` | Channel discovery persistence |

## Architecture

See [docs/architecture-notifications.md](../../docs/architecture-notifications.md) for the full notification architecture, including the 3-part adapter pattern, filtering logic, and database schema.
