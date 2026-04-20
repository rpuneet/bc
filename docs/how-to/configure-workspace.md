# Configuration Guide

This guide covers the bc configuration system: how settings are stored, how to view and modify them, and what each option controls.

## Overview

bc uses a layered configuration system:

1. **Workspace config** (`.bc/settings.json`) — project-specific settings, the primary config file
2. **User config** (`~/.bcrc`) — user-level defaults that apply across all workspaces
3. **CLI** (`bc config` commands) — read and write config from the terminal
4. **API** (`/api/settings` endpoints) — read and write config from the bcd HTTP server
5. **Web UI** (Settings page at `http://localhost:9374`) — visual config editor

Workspace config always takes precedence over user-level defaults.

## Config File Location

The workspace config file lives at:

```
<project-root>/.bc/settings.json
```

It is created automatically when you run `bc init`. The file uses [TOML](https://toml.io/) format.

## Config Sections

### `[workspace]`

Core workspace identity.

| Field     | Type   | Default        | Description                          |
|-----------|--------|----------------|--------------------------------------|
| `name`    | string | (project name) | Workspace display name (required)    |
| `version` | int    | `2`            | Config schema version (must be `2`)  |
| `path`    | string | `""`           | Workspace root path                  |

### `[user]`

User identity settings.

| Field      | Type   | Default | Description                                      |
|------------|--------|---------|--------------------------------------------------|
| `nickname` | string | `@bc`   | Display name for messages (max 15 chars, must start with `@`, alphanumeric and underscores only) |

### `[providers]`

AI agent provider configuration. Each provider has `command`, `enabled`, and optional `env` fields.

| Field     | Type   | Default   | Description                         |
|-----------|--------|-----------|-------------------------------------|
| `default` | string | `gemini`  | Default provider for new agents     |

Built-in providers: `claude`, `gemini`, `cursor`, `codex`, `opencode`, `openclaw`, `aider`.

Each provider section (e.g., `[providers.claude]`):

| Field     | Type              | Default | Description                        |
|-----------|-------------------|---------|------------------------------------|
| `command` | string            | varies  | Command to launch the provider     |
| `enabled` | bool              | `false` | Whether the provider is available  |
| `env`     | map[string]string | `{}`    | Per-provider environment variables |

**Default provider commands:**

| Provider | Default Command                         |
|----------|-----------------------------------------|
| claude   | `claude --dangerously-skip-permissions` |
| gemini   | `gemini --yolo`                         |

Example:

```toml
[providers]
default = "claude"

[providers.claude]
command = "claude --dangerously-skip-permissions"
enabled = true

[providers.gemini]
command = "gemini --yolo"
enabled = true
env = { GEMINI_API_KEY = "your-key" }
```

### `[runtime]`

Agent session backend configuration.

| Field     | Type   | Default  | Description                              |
|-----------|--------|----------|------------------------------------------|
| `backend` | string | `docker` | Runtime backend: `"tmux"` or `"docker"` |

#### `[runtime.docker]`

Docker-specific runtime settings.

| Field          | Type     | Default | Description                           |
|----------------|----------|---------|---------------------------------------|
| `image`        | string   | `""`    | Docker image for agent containers     |
| `network`      | string   | `""`    | Docker network name                   |
| `extra_mounts` | []string | `[]`    | Additional volume mounts              |
| `cpus`         | float64  | `0`     | CPU limit per container               |
| `memory_mb`    | int64    | `0`     | Memory limit in MB per container      |

### `[logs]`

Session log streaming configuration.

| Field       | Type   | Default       | Description                      |
|-------------|--------|---------------|----------------------------------|
| `path`      | string | `.bc/logs`    | Directory for log files          |
| `max_bytes` | int64  | `1048576` (1MB) | Maximum log file size in bytes |

### `[performance]`

TUI polling intervals and cache TTLs. All values are in milliseconds. Minimum poll interval is 500ms.

**Polling intervals:**

| Field                    | Type  | Default | Description                        |
|--------------------------|-------|---------|------------------------------------|
| `poll_interval_agents`   | int64 | `2000`  | Agent status update interval       |
| `poll_interval_channels` | int64 | `3000`  | Notification feed polling interval   |
| `poll_interval_costs`    | int64 | `5000`  | Cost data refresh interval         |
| `poll_interval_status`   | int64 | `2000`  | Dashboard status refresh interval  |
| `poll_interval_logs`     | int64 | `3000`  | Log viewer refresh interval        |
| `poll_interval_teams`    | int64 | `10000` | Team data refresh interval         |
| `poll_interval_daemons`  | int64 | `5000`  | Scheduled tasks refresh interval   |

**Cache TTLs:**

| Field              | Type  | Default | Description                                |
|--------------------|-------|---------|--------------------------------------------|
| `cache_ttl_tmux`   | int64 | `2000`  | Tmux session state cache TTL               |
| `cache_ttl_commands`| int64 | `5000`  | CLI command result cache TTL               |

Cache TTLs must be between 100ms and 60,000ms (1 minute).

**Adaptive polling thresholds:**

| Field                      | Type  | Default | Description                              |
|----------------------------|-------|---------|------------------------------------------|
| `adaptive_fast_interval`   | int64 | `1000`  | Interval when agents are actively working |
| `adaptive_normal_interval` | int64 | `2000`  | Normal operation interval                 |
| `adaptive_slow_interval`   | int64 | `4000`  | Low activity period interval              |
| `adaptive_max_interval`    | int64 | `8000`  | Maximum backoff interval                  |

### `[tui]`

TUI appearance and theming.

| Field   | Type   | Default | Description                                                   |
|---------|--------|---------|---------------------------------------------------------------|
| `theme` | string | `dark`  | Theme: `dark`, `light`, `matrix`, `synthwave`, `high-contrast` |
| `mode`  | string | `auto`  | Color mode: `auto`, `dark`, `light`                           |

### `[env]`

Global environment variables passed to all agents.

```toml
[env]
GITHUB_TOKEN = "ghp_..."
SOME_VAR = "value"
```

### `[roster]`

Team roster: agents that `bc ws up` will start automatically.

```toml
[[roster.agents]]
name = "root"
role = "root"
tool = "claude"

[[roster.agents]]
name = "dev-1"
role = "feature-dev"
tool = "gemini"
runtime = "docker"  # optional per-agent runtime override
```

Each roster entry:

| Field     | Type   | Required | Description                          |
|-----------|--------|----------|--------------------------------------|
| `name`    | string | yes      | Agent name                           |
| `role`    | string | yes      | Role file name (e.g., `feature-dev`) |
| `tool`    | string | yes      | Provider tool (e.g., `claude`)       |
| `runtime` | string | no       | Runtime backend override             |

### `[services]`

External service integrations.

```toml
[services.github]
command = "gh"
enabled = true

[services.gitlab]
command = "glab"
enabled = true

[services.jira]
command = "jira"
enabled = false
```

Each service section:

| Field     | Type   | Default | Description                      |
|-----------|--------|---------|----------------------------------|
| `command` | string | `""`    | CLI command for the service      |
| `enabled` | bool   | `false` | Whether the service is active    |

### `[server]`

bcd HTTP server configuration.

| Field         | Type   | Default          | Description                      |
|---------------|--------|------------------|----------------------------------|
| `addr`        | string | `127.0.0.1:9374` | Listen address for the server    |
| `cors_origin` | string | `*`              | Allowed CORS origin              |

### `[scheduler]`

Cron/job scheduler configuration.

| Field           | Type | Default | Description                                    |
|-----------------|------|---------|------------------------------------------------|
| `tick_interval` | int  | `60`    | Seconds between scheduler ticks                |
| `job_timeout`   | int  | `300`   | Seconds before a job is considered timed out   |

### `[storage]`

Persistent storage paths.

| Field         | Type   | Default      | Description                  |
|---------------|--------|--------------|------------------------------|
| `sqlite_path` | string | `.bc/bc.db`  | Path to the SQLite database  |

## CLI Commands

The `bc config` command group provides full config management from the terminal.

### `bc config show [key]`

Display the current workspace configuration. Optionally filter by section key.

```bash
bc config show                    # Show all config
bc config show providers          # Show providers section
bc config show providers.claude   # Show specific provider
bc config show --json             # Output as JSON
```

### `bc config get <key>`

Get a single configuration value using dot notation.

```bash
bc config get workspace.name        # "my-project"
bc config get providers.default     # "gemini"
bc config get runtime.backend       # "docker"
bc config get tui.theme             # "dark"
```

### `bc config set <key> <value>`

Set a configuration value. The value type is automatically inferred (string, number, boolean).

```bash
bc config set providers.default claude
bc config set runtime.backend tmux
bc config set tui.theme synthwave
bc config set performance.poll_interval_agents 5000
bc config set user.nickname "@alice"
```

### `bc config list`

List all available configuration keys.

```bash
bc config list          # Print all keys, one per line
bc config list --json   # Output as JSON array
```

### `bc config edit`

Open the config file in your default editor (`$EDITOR`, falls back to `nano`).

```bash
bc config edit
```

### `bc config validate`

Validate the config file for errors. Checks TOML syntax, required fields, valid values, and provider references.

```bash
bc config validate
```

### `bc config reset`

Reset the config to default values. Prompts for confirmation unless `--force` is used.

```bash
bc config reset           # Prompts for confirmation
bc config reset --force   # Skip confirmation
```

## User-Level Config (`~/.bcrc`)

The user-level config provides defaults that apply across all bc workspaces. Workspace config takes precedence.

### Structure

```toml
[user]
nickname = "@alice"

[defaults]
default_role = "engineer"
auto_start_root = true

[tools]
preferred = ["claude", "gemini"]
```

| Section              | Field            | Type     | Description                              |
|----------------------|------------------|----------|------------------------------------------|
| `[user]`             | `nickname`       | string   | Default nickname across workspaces       |
| `[defaults]`         | `default_role`   | string   | Default role for new agents              |
| `[defaults]`         | `auto_start_root`| bool     | Auto-start root agent with `bc up`       |
| `[tools]`            | `preferred`      | []string | Preferred tools in priority order        |

### User Config Commands

```bash
bc config user init           # Interactive setup wizard
bc config user init --quick   # Create with defaults (no prompts)
bc config user show           # Display current user config
bc config user path           # Show path to ~/.bcrc
```

## Validation Rules

The config is validated on save (both CLI and API). Key rules:

- `workspace.name` is required
- `workspace.version` must be `2`
- `providers.default` is required and must reference a defined provider or service
- Poll intervals must be at least 500ms (if set; zero uses defaults)
- Cache TTLs must be between 100ms and 60,000ms (if set)
- `tui.theme` must be one of: `dark`, `light`, `matrix`, `synthwave`, `high-contrast`
- `tui.mode` must be one of: `auto`, `dark`, `light`
- `user.nickname` must start with `@`, be 15 chars or less, and contain only letters, numbers, and underscores

## Example Config

A complete `settings.json` example:

```toml
[workspace]
name = "my-project"
version = 2

[user]
nickname = "@alice"

[providers]
default = "claude"

[providers.claude]
command = "claude --dangerously-skip-permissions"
enabled = true

[providers.gemini]
command = "gemini --yolo"
enabled = true

[runtime]
backend = "docker"

[runtime.docker]
image = "bc-agent:latest"
cpus = 2.0
memory_mb = 4096

[logs]
path = ".bc/logs"
max_bytes = 1048576

[server]
addr = "127.0.0.1:9374"
cors_origin = "*"

[scheduler]
tick_interval = 60
job_timeout = 300

[storage]
sqlite_path = ".bc/bc.db"

[tui]
theme = "dark"
mode = "auto"

[performance]
poll_interval_agents = 2000
poll_interval_channels = 3000
poll_interval_costs = 5000
poll_interval_status = 2000

[env]
GITHUB_TOKEN = "ghp_..."

[[roster.agents]]
name = "root"
role = "root"
tool = "claude"

[[roster.agents]]
name = "dev-1"
role = "feature-dev"
tool = "gemini"

[services.github]
command = "gh"
enabled = true
```
