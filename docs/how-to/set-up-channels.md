# Set Up Channels

Channels connect external platforms (Slack, Telegram, Discord, etc.) to your bc agents as **inbound notification gateways**. Agents receive raw platform events and respond using injected credentials.

For the full architecture, see [Channel Architecture](../architecture/channels.md).

## Connect a Platform

### Slack

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps)
2. Enable Socket Mode
3. Add scopes: `channels:read`, `connections:write`
4. Copy the bot token and app-level token
5. Store credentials:
   ```bash
   bc secret set GATEWAY_SLACK_BOT_TOKEN xoxb-...
   bc secret set GATEWAY_SLACK_APP_TOKEN xapp-...
   ```
6. Invite the bot to the channels you want to monitor

### Telegram

1. Message [@BotFather](https://t.me/BotFather) with `/newbot`
2. Copy the bot token
3. Store credential:
   ```bash
   bc secret set GATEWAY_TELEGRAM_BOT_TOKEN 123456:ABC...
   ```
4. Add the bot to your groups
5. Optionally disable privacy mode for full message access

### Discord

1. Create an app at the [Discord Developer Portal](https://discord.com/developers/applications)
2. Enable the `MESSAGE_CONTENT` intent
3. Copy the bot token
4. Store credential:
   ```bash
   bc secret set GATEWAY_DISCORD_BOT_TOKEN ...
   ```
5. Generate an invite URL and add the bot to your server

## Subscribe Agents

```bash
bc channel subscribe --channel slack:engineering --agent eng-01
bc channel subscribe --channel telegram:bc-dev --agent eng-02 --mention-only
```

## Unsubscribe Agents

```bash
bc channel unsubscribe --channel slack:engineering --agent eng-01
```

## Check Status

```bash
bc channel list       # all channels across gateways with subscriber counts
bc channel status     # gateway connection status + health
```

## How Agents Respond

bc does **not** send outbound messages. Agents use injected platform credentials (environment variables) to call platform APIs directly. Identity instructions in the agent's system prompt tell it how to identify itself on each platform.

See [Channel Architecture -- Credential Injection](../architecture/channels.md#credential-injection) for details.
