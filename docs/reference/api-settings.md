# Settings API

The Settings API reads and updates mycel configuration via the bcd HTTP server. The configuration is a JSON document persisted to `preferences.json` in the per-repo runtime state directory (`~/.mycel/workspaces/<id>/`).

Base URL: `http://localhost:9374`

## Endpoints

### GET /api/settings

Returns the full configuration.

**Response:** `200 OK`

```json
{
  "user": {
    "name": "alice"
  },
  "providers": {
    "default": "claude",
    "providers": {
      "claude": { "command": "claude --dangerously-skip-permissions" },
      "gemini": { "command": "gemini --yolo" }
    }
  },
  "gateways": {},
  "runtime": {
    "default": "docker",
    "docker": {
      "extra_mounts": null,
      "image": "mycel-agent-claude:latest",
      "network": "bc-net",
      "docker_socket_path": "/var/run/docker.sock",
      "memory_mb": 4096,
      "cpus": 2
    },
    "tmux": {
      "session_prefix": "mycel",
      "default_shell": "/bin/bash",
      "history_limit": 10000
    }
  },
  "storage": {
    "default": "sqlite",
    "sqlite": { "path": ".bc" },
    "timescale": {
      "host": "localhost",
      "user": "bc",
      "password": "bc",
      "database": "bc",
      "port": 5432
    }
  },
  "server": {
    "host": "127.0.0.1",
    "cors_origin": "*",
    "port": 9374
  },
  "cron": {
    "poll_interval_seconds": 30,
    "job_timeout_seconds": 300
  },
  "logs": {
    "path": "",
    "max_bytes": 10485760
  },
  "ui": {
    "theme": "dark",
    "mode": "auto",
    "default_view": "dashboard"
  },
  "version": 2
}
```

---

### PATCH /api/settings

Partial update. The request body is a JSON object whose top-level keys are config sections; send only the sections you want to change. Unspecified sections remain unchanged. A `version` key in the body is ignored; any other unknown key is rejected with `400`.

**Supported sections:** `user`, `server`, `runtime`, `providers`, `gateways`, `cron`, `storage`, `logs`, `ui`.

Most sections are replaced wholesale by the patch value. The `gateways` section is deep-merged per platform key, so sending `{"gateways": {"discord": {...}}}` does not remove existing Slack or Telegram entries.

The merged config is validated before saving. On success it is persisted to `preferences.json` and the full updated config is returned.

**Example: change the UI theme**

```bash
curl -X PATCH http://localhost:9374/api/settings \
  -H "Content-Type: application/json" \
  -d '{ "ui": { "theme": "synthwave", "mode": "dark" } }'
```

**Example: switch the runtime backend**

```bash
curl -X PATCH http://localhost:9374/api/settings \
  -H "Content-Type: application/json" \
  -d '{ "runtime": { "default": "tmux" } }'
```

**Response:** `200 OK` with the full updated config (same schema as GET).

**Errors:**

| Status | Description |
|--------|-------------|
| `400`  | Invalid JSON, invalid section data, unknown section, or validation failure |
| `405`  | Method not allowed (only GET and PATCH are supported) |
| `500`  | Failed to save the config |

```json
{
  "error": "validation failed: providers.default is required"
}
```

## Configuration Schema

### `user`

| Field  | Type   | Default | Description |
|--------|--------|---------|-------------|
| `name` | string | `""`    | User display name (max 30 characters) |

### `providers`

| Field       | Type   | Default  | Description |
|-------------|--------|----------|-------------|
| `default`   | string | `claude` | Default provider for new agents; must reference a key in `providers` |
| `providers` | object | claude, gemini | Map of provider name to provider config |

Each entry in the `providers` map:

| Field     | Type   | Description |
|-----------|--------|-------------|
| `command` | string | Command used to launch the provider |

Default commands: `claude` → `claude --dangerously-skip-permissions`, `gemini` → `gemini --yolo`.

### `gateways`

Map of platform key to gateway config. Keys follow a `platform` or `platform:label` convention (e.g. `telegram`, `telegram:alerts`) so a platform can be registered more than once under different labels.

Supported platform keys: `slack`, `telegram`, `discord`, `github`, `gitlab`, `webhook`, `rss`, `jira`, `linear`, `sentry`, `stripe`, `bitbucket`, `pagerduty`, `datadog`, `grafana`, `vercel`, `netlify`, `notion`, `whatsapp`, `signal`, `matrix`, `msteams`, `googlechat`, `line`, `feishu`, `mattermost`, `irc`, `nostr`, `twitch`, `imessage`, `mqtt`, `twitter`, `reddit`, `homeassistant`.

Every gateway config has an `enabled` (bool) field plus platform-specific credentials, for example:

| Platform   | Fields |
|------------|--------|
| `slack`    | `bot_token`, `app_token`, `mode`, `enabled` |
| `telegram` | `bot_token`, `mode`, `enabled` |
| `discord`  | `bot_token`, `enabled` |
| `github`   | `secret`, `enabled` |
| `webhook`  | `secret`, `enabled` |
| `rss`      | `url`, `interval` (seconds), `enabled` |

See [Set Up Notifications](../how-to/set-up-notifications.md) for connecting gateways.

### `runtime`

| Field     | Type   | Default  | Description |
|-----------|--------|----------|-------------|
| `default` | string | `docker` | Agent session backend: `tmux` or `docker` |

`runtime.docker`:

| Field                | Type     | Default                     | Description |
|----------------------|----------|-----------------------------|-------------|
| `image`              | string   | `mycel-agent-claude:latest` | Docker image for agent containers |
| `network`            | string   | `bc-net`                    | Docker network name |
| `docker_socket_path` | string   | `/var/run/docker.sock`      | Docker socket mounted into containers |
| `extra_mounts`       | []string | `[]`                        | Additional volume mounts |
| `memory_mb`          | int      | `4096`                      | Memory limit per container (MB) |
| `cpus`               | float    | `2`                         | CPU limit per container |

`runtime.tmux`:

| Field            | Type   | Default     | Description |
|------------------|--------|-------------|-------------|
| `session_prefix` | string | `mycel`     | Prefix for tmux session names |
| `default_shell`  | string | `/bin/bash` | Shell used inside sessions |
| `history_limit`  | int    | `10000`     | tmux scrollback history limit |

### `storage`

| Field     | Type   | Default  | Description |
|-----------|--------|----------|-------------|
| `default` | string | `sqlite` | Storage backend: `sqlite` or `timescale` |

`storage.sqlite`:

| Field  | Type   | Default | Description |
|--------|--------|---------|-------------|
| `path` | string | `.bc`   | Directory for SQLite database files |

`storage.timescale`:

| Field      | Type   | Default     | Description |
|------------|--------|-------------|-------------|
| `host`     | string | `localhost` | TimescaleDB (Postgres) host |
| `port`     | int    | `5432`      | Port (1–65535) |
| `user`     | string | `bc`        | Database user |
| `password` | string | `bc`        | Database password |
| `database` | string | `bc`        | Database name |

### `server`

| Field         | Type   | Default     | Description |
|---------------|--------|-------------|-------------|
| `host`        | string | `127.0.0.1` | Listen address for bcd |
| `port`        | int    | `9374`      | Listen port (1–65535) |
| `cors_origin` | string | `*`         | Allowed CORS origin |

### `cron`

| Field                   | Type | Default | Description |
|-------------------------|------|---------|-------------|
| `poll_interval_seconds` | int  | `30`    | Seconds between scheduler polls |
| `job_timeout_seconds`   | int  | `300`   | Seconds before a job is considered timed out |

### `logs`

| Field       | Type   | Default            | Description |
|-------------|--------|--------------------|-------------|
| `path`      | string | `""`               | Log directory; empty means `<state dir>/logs` |
| `max_bytes` | int    | `10485760` (10 MB) | Maximum log file size in bytes |

### `ui`

| Field          | Type   | Default     | Description |
|----------------|--------|-------------|-------------|
| `theme`        | string | `dark`      | Theme: `dark`, `light`, `matrix`, `synthwave`, `high-contrast` |
| `mode`         | string | `auto`      | Color mode: `auto`, `dark`, `light` |
| `default_view` | string | `dashboard` | View shown on startup |

### `version`

| Field     | Type | Default | Description |
|-----------|------|---------|-------------|
| `version` | int  | `2`     | Config schema version (must be `2`) |

## Validation

The merged configuration is validated before every save. If validation fails, no changes are persisted and a `400` error is returned with the validation message.

- `version` must be `2`
- `providers.default` is required and must reference a defined provider
- `user.name` must be at most 30 characters
- `server.port` must be between 1 and 65535 (if set)
- `storage.default` must be `sqlite` or `timescale`
- `storage.timescale.port` must be between 1 and 65535 (if set)
- `ui.theme` must be one of: `dark`, `light`, `matrix`, `synthwave`, `high-contrast`
- `ui.mode` must be one of: `auto`, `dark`, `light`

Fields left unset (zero-valued) are filled in from defaults when the config is loaded.

## Error Format

All error responses use a consistent JSON format:

```json
{
  "error": "description of the error"
}
```
