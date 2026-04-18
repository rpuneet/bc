# pkg/notify

Notification dispatch and subscription management for bc channels.

## Overview

This package receives inbound events from `pkg/gateway` adapters and routes them to subscribed agents via `tmux send-keys`. It manages subscriptions (which agents receive which channels), delivery logging, and the activity feed for the web UI.

## Key Types

- **`Service`** -- dispatch core: receives notifications, queries subscribers, delivers via tmux, publishes to SSE hub
- **`Store`** -- SQLite/Postgres persistence for subscriptions, delivery log, messages, and gateway state
- **`Notification`** -- JSON payload sent to agents
- **`Subscription`** -- ties an agent to a channel with `mention_only` flag

## Database Tables

| Table | Purpose |
|-------|---------|
| `notify_subscriptions` | Agent-to-channel mappings |
| `notify_delivery_log` | Delivery attempt records |
| `notify_messages` | Inbound messages for activity feed |
| `notify_gateways` | Gateway connection state |
| `notify_channels` | Channel discovery persistence |

## Architecture

See [docs/architecture/notifications.md](../../docs/architecture/notifications.md) for the full notification architecture, including message flow diagrams, filtering logic, and database schema.
