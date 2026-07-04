# Set Up Notifications

This guide walks through connecting external platforms to mycel and routing notifications to agents.

## Overview

mycel routes inbound events from external platforms (Slack, Telegram, GitHub, and others) to subscribed agents. Agents receive notifications as JSON payloads in their tmux or Docker sessions and respond using platform APIs directly.

## 1. Add a Notification Source

### Slack

1. Create a Slack app at [api.slack.com/apps](https://api.slack.com/apps).
2. Enable **Socket Mode** under the app settings.
3. Add OAuth scopes: `channels:read`, `chat:write`, `connections:write`.
4. Install the app to your workspace and copy the **Bot Token** (`xoxb-...`) and **App Token** (`xapp-...`).
5. Invite the bot to the channels you want to monitor.
6. Connect the gateway:

```bash
# Store credentials
mycel secret set SLACK_BOT_TOKEN "xoxb-..."
mycel secret set SLACK_APP_TOKEN "xapp-..."
```

Use the web UI at `http://localhost:9374` to connect the Slack gateway, or call the API directly:

```bash
curl -X POST http://localhost:9374/api/gateways \
  -H "Content-Type: application/json" \
  -d '{"platform": "slack", "tokens": {"bot_token": "xoxb-...", "app_token": "xapp-..."}}'
```

### Telegram

1. Message [@BotFather](https://t.me/BotFather) on Telegram and create a new bot with `/newbot`.
2. Copy the bot token.
3. Add the bot to your Telegram groups.
4. Optionally disable privacy mode so the bot can read all group messages (not just commands).

```bash
mycel secret set TELEGRAM_BOT_TOKEN "123456:ABC-..."
```

Connect via the web UI or API:

```bash
curl -X POST http://localhost:9374/api/gateways \
  -H "Content-Type: application/json" \
  -d '{"platform": "telegram", "tokens": {"bot_token": "123456:ABC-..."}}'
```

### GitHub

1. Create a GitHub App or configure a repository webhook at your repo's Settings > Webhooks.
2. Select the events you want to receive (pull request comments, reviews, issues, pushes).
3. Set the webhook URL to your bcd instance's hook endpoint (requires a tunnel for local development).
4. Copy the token and webhook secret.

```bash
mycel secret set GITHUB_TOKEN "ghp_..."
mycel secret set GITHUB_WEBHOOK_SECRET "whsec_..."
```

## 2. Subscribe Agents

Once a gateway is connected and sources are discovered, subscribe agents to receive notifications.

```bash
# List available notification sources
mycel notify list

# Subscribe an agent to all messages on a source
mycel notify subscribe slack:engineering eng-01

# Subscribe with mention-only filtering (noisy channels)
mycel notify subscribe --mention-only slack:engineering lead-01

# Subscribe to GitHub events
mycel notify subscribe github:bc eng-02
```

### Mention-Only Filtering

For high-traffic channels, use `--mention-only` so the agent only receives notifications where it is explicitly @mentioned:

```bash
mycel notify subscribe --mention-only slack:all-bc eng-01
```

The agent receives messages containing `@eng-01` and ignores all other traffic.

## 3. Verify Delivery

After subscribing, verify that notifications are flowing:

```bash
# Check adapter connection health
mycel notify status

# View recent delivery activity for a source
mycel notify activity slack:engineering --limit 10

# Check agent output for received notifications
mycel agent peek eng-01
```

The web dashboard at `http://localhost:9374` provides a real-time activity feed showing inbound notifications and delivery status.

## 4. Troubleshoot Common Issues

### Adapter shows "disconnected"

```bash
mycel notify status
```

If an adapter shows disconnected:

- Verify the credentials are correct: `mycel secret list`
- Check that the bot is still invited to the platform channels.
- For Slack: ensure Socket Mode is enabled and the app token is valid.
- For Telegram: verify the bot token with `curl https://api.telegram.org/bot<TOKEN>/getMe`.
- Restart bcd: `mycel down && mycel up`.

### Agent not receiving notifications

1. Confirm the subscription exists:

   ```bash
   mycel notify list
   ```

2. Confirm the agent is running:

   ```bash
   mycel status
   ```
3. Check the delivery log in the web UI for failed deliveries.
4. If using `--mention-only`, ensure the sender is using the correct @mention format (`@agent-name`).

### Self-skip filtering

Agents do not receive notifications they themselves sent. If an agent posts a message to Slack via the Slack API, that message is filtered out when it echoes back through the gateway. This is expected behavior.

### Duplicate notifications

If an agent receives the same notification multiple times, check for duplicate subscriptions:

```bash
mycel notify list
```

Remove duplicates with `mycel notify unsubscribe`.

## Sending back to a platform

Notifications are one-way in — mycel does not proxy agent replies back
to the platform. When an agent needs to post to Slack / Telegram /
Discord / WhatsApp, it calls the platform's official REST API
directly with a bot token loaded from its own `env.json`. See the
[**Outbound cookbook**](../architecture-notifications.md#outbound-cookbook)
in the Notification architecture doc for the exact curl invocations
and the `env.json` template.

## Next Steps

- Read the [Notifications architecture](../architecture-notifications.md) for the full system design + outbound cookbook.
- See the [REST API reference](../reference/api-rest.md) for gateway and subscription endpoints.
