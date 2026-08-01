# Configure mycel

This guide shows how to view and change mycel configuration: the config file, the `mycel config` CLI, the HTTP API, and the web UI.

## Where Configuration Lives

Configuration is JSON in one global file:

```
~/.mycel/prefs.json
```

There is exactly one config — mycel state is global, not per-repo.

You can change configuration four ways:

1. **CLI** — `mycel config` commands (work online via the daemon or offline against the file)
2. **API** — `GET`/`PATCH /api/settings` on the daemon server (see [Settings API](../reference/api-settings.md))
3. **Web UI** — Settings page at `http://localhost:9374`
4. **Editor** — `mycel config edit` opens the file directly

## View the Current Configuration

```bash
mycel config show                    # Show all config
mycel config show providers          # Show one section
mycel config show --json             # Output as JSON
mycel config get providers.default   # Print a single value
mycel config list                    # List all config keys
```

Keys use dot notation matching the JSON structure: `providers.default`, `runtime.docker.image`, `ui.theme`.

## Common Changes

### Change the default provider

The default provider is used for new agents. It must reference a provider defined in `providers.providers`.

```bash
mycel config set providers.default gemini
```

```json
{
  "providers": {
    "default": "gemini",
    "providers": {
      "claude": { "command": "claude --dangerously-skip-permissions" },
      "gemini": { "command": "gemini --yolo" }
    }
  }
}
```

### Add or change a provider command

Provider entries are a map, so edit the file directly:

```bash
mycel config edit
```

```json
{
  "providers": {
    "providers": {
      "claude": { "command": "claude --dangerously-skip-permissions" },
      "cursor": { "command": "cursor-agent" }
    }
  }
}
```

Each provider has a single `command` field — the command used to launch it.

### Switch the runtime backend

Agents run in Docker containers by default; tmux runs them directly on the host.

```bash
mycel config set runtime.default tmux     # or: docker
```

### Tune Docker resources

```bash
mycel config set runtime.docker.image mycel-agent-claude:latest
mycel config set runtime.docker.cpus 4
mycel config set runtime.docker.memory_mb 8192
```

```json
{
  "runtime": {
    "default": "docker",
    "docker": {
      "image": "mycel-agent-claude:latest",
      "network": "mycel-net",
      "cpus": 4,
      "memory_mb": 8192
    }
  }
}
```

### Change the server address

```bash
mycel config set server.host 0.0.0.0
mycel config set server.port 9374
mycel config set server.cors_origin "*"
```

The port must be between 1 and 65535.

### Switch the storage backend

SQLite is the default; TimescaleDB (Postgres) is available for larger deployments.

```bash
mycel config set storage.default timescale
mycel config set storage.timescale.host localhost
mycel config set storage.timescale.port 5432
```

```json
{
  "storage": {
    "default": "timescale",
    "timescale": {
      "host": "localhost",
      "port": 5432,
      "user": "mycel",
      "password": "mycel",
      "database": "mycel"
    }
  }
}
```

### Configure session logs

```bash
mycel config set logs.max_bytes 52428800   # 50 MB
```

Leave `logs.path` empty to keep logs under `<state dir>/logs`.

### Change the UI theme

```bash
mycel config set ui.theme synthwave        # dark, light, matrix, synthwave, high-contrast
mycel config set ui.mode dark              # auto, dark, light
mycel config set ui.default_view dashboard
```

### Set your name

```bash
mycel config set user.name alice
```

Maximum 30 characters.

### Connect apps

Apps (Slack, Telegram, GitHub, and 25+ others) live under the `apps` section, keyed by instance name (`"slack"`, `"telegram:alerts"`). Configure them through the web UI or the `/api/apps` surface rather than by hand — secret fields go to the encrypted vault, never into `prefs.json`. See [Set Up Apps](set-up-apps.md).

## Edit, Validate, Reset

```bash
mycel config edit       # Open the config file in $EDITOR (falls back to nano)
mycel config validate   # Check the config for errors
mycel config reset      # Reset to defaults (prompts; --force to skip)
```

Validation checks the schema version (`2`), that `providers.default` references a defined provider, value ranges for ports, and allowed values for `ui.theme` and `ui.mode`. Any field you leave unset falls back to its default.

## User-Level Configuration

User-level defaults that apply everywhere live in `~/.bcrc`:

```bash
mycel config user init          # Create ~/.bcrc with guided prompts
mycel config user init --quick  # Create with defaults, no prompts
mycel config user show          # Display current user config
mycel config user path          # Show the file path
```

`prefs.json` always takes precedence over user-level defaults.

## Full Example

A complete `prefs.json`:

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
      "image": "mycel-agent-claude:latest",
      "network": "mycel-net",
      "docker_socket_path": "/var/run/docker.sock",
      "extra_mounts": [],
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
      "port": 5432,
      "user": "mycel",
      "password": "mycel",
      "database": "mycel"
    }
  },
  "server": {
    "host": "127.0.0.1",
    "port": 9374,
    "cors_origin": "*"
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
  "version": 3
}
```

For the complete field reference — types, defaults, and validation rules — see [Settings API](../reference/api-settings.md).
