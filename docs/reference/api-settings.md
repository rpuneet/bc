# Settings API

The Settings API reads and updates mycel configuration via the the daemon HTTP server. The configuration is a JSON document persisted to `~/.mycel/prefs.json`.

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
  "apps": {},
  "runtime": {
    "default": "docker",
    "docker": {
      "extra_mounts": null,
      "image": "mycel-agent-claude:latest",
      "network": "mycel-net",
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
    "sqlite": { "path": ".mycel" },
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

**Supported sections:** `user`, `server`, `runtime`, `providers`, `apps`, `storage`, `logs`, `ui`, `injected_instructions`.

Most sections are replaced wholesale by the patch value. The `apps` section is merged per instance key, so sending `{"apps": {"discord": {...}}}` does not remove existing Slack or Telegram instances. Each submitted instance is validated against its app descriptor — unknown apps, unknown config keys, and secret-typed fields are all rejected (secrets go through `POST /api/apps/{instance}`).

The merged config is validated before saving. On success it is persisted to `prefs.json` and the full updated config is returned.

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

### `apps`

Map of instance name to app instance config. Keys follow an `app` or `app:label` convention (e.g. `telegram`, `telegram:alerts`) so an app can be connected more than once under different labels.

Registered app IDs (28 built-in plugins): `bitbucket`, `datadog`, `discord`, `github`, `gitlab`, `grafana`, `imessage`, `irc`, `jira`, `line`, `linear`, `matrix`, `mattermost`, `mqtt`, `netlify`, `notion`, `pagerduty`, `reddit`, `rss`, `sentry`, `signal`, `slack`, `stripe`, `telegram`, `twitch`, `vercel`, `webhook`, `whatsapp`.

Every instance has one generic shape:

```json
{
  "apps": {
    "slack":           { "app": "slack",    "enabled": true, "config": { "mode": "socket" } },
    "telegram:alerts": { "app": "telegram", "enabled": true, "config": { "mode": "poll" } }
  }
}
```

Config keys are validated against the app's descriptor (`GET /api/apps` lists every field). **Secret-typed fields never appear here** — they live in the encrypted vault as `app:<instance>:<key>` and are written via `POST /api/apps/{instance}`.

See [Set Up Apps](../how-to/set-up-apps.md) for connecting apps.

### `runtime`

| Field     | Type   | Default  | Description |
|-----------|--------|----------|-------------|
| `default` | string | `docker` | Agent session backend: `tmux` or `docker` |

`runtime.docker`:

| Field                | Type     | Default                     | Description |
|----------------------|----------|-----------------------------|-------------|
| `image`              | string   | `mycel-agent-claude:latest` | Docker image for agent containers |
| `network`            | string   | `mycel-net`                    | Docker network name |
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
| `path` | string | `.mycel`   | Directory for SQLite database files |

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
| `host`        | string | `127.0.0.1` | Listen address for the daemon |
| `port`        | int    | `9374`      | Listen port (1–65535) |
| `cors_origin` | string | `*`         | Allowed CORS origin |

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
