# pkg/gateway

Inbound notification adapters for external platforms (Slack, Telegram, Discord, etc.).

## Architecture

Channels are **inbound notification-only gateways**. Each platform adapter implements `NotificationAdapter`:

```go
type NotificationAdapter interface {
    Name() string
    Platform() string
    Start(ctx context.Context, handler func(Notification)) error
    Stop() error
    Channels() []ChannelInfo
}
```

There is no `Send()` method. Agents handle outbound communication using injected credentials.

See [docs/architecture/channels.md](../../docs/architecture/channels.md) for the full architecture.

## Packages

- `gateway.go` -- core interfaces and types
- `manager.go` -- orchestrates adapters, routes inbound notifications to `pkg/notify`
- `slack/` -- Slack Socket Mode adapter
- `telegram/` -- Telegram Bot API long-polling adapter
- `discord/` -- Discord Gateway WebSocket adapter

## Adding a New Adapter

1. Create `pkg/gateway/<platform>/adapter.go`
2. Implement `NotificationAdapter`
3. Register in server startup
4. Store credentials via `bc secret set GATEWAY_<PLATFORM>_TOKEN`

See [How to Add a New Channel](../../docs/architecture/channels.md#how-to-add-a-new-channel) for step-by-step instructions.
