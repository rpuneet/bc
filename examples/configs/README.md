# Example Workspace Configurations

This directory contains example `settings.json` files for different use cases. Copy the appropriate configuration to your workspace's `.bc/settings.json`.

## Available Configurations

### Solo Developer (`solo-developer.toml`)

Minimal setup for individual developers working alone.

```bash
cp examples/configs/solo-developer.toml .bc/settings.json
bc up            # bootstraps the workspace
```

**Features:**
- Single engineer agent
- Minimal resource usage
- Longer polling intervals to save CPU
- Single `general` channel

**Best for:** Personal projects, learning bc, quick prototypes

### Small Team (`small-team.toml`)

Balanced setup for small development teams (2-5 developers).

```bash
cp examples/configs/small-team.toml .bc/settings.json
bc up            # bootstraps the workspace
```

**Features:**
- 2 engineers + 1 tech lead + 1 QA
- Multiple AI tools enabled (Claude, Gemini)
- Team channels: general, engineering, code-review
- Adaptive polling for active development

**Best for:** Startups, small teams, feature development

### CI/CD Integration (`ci-cd.toml`)

Optimized for automated pipelines and continuous integration.

```bash
cp examples/configs/ci-cd.toml .bc/settings.json
bc up            # bootstraps the workspace with defaults
```

**Features:**
- GitHub CLI integration enabled
- QA-focused roster (1 engineer + 2 QA)
- Fast polling for CI responsiveness
- Specialized channels: ci, alerts, deployments

**Best for:** CI pipelines, automated testing, deployments

## Quick Start

1. Start the server from your repo — it bootstraps the workspace:
   ```bash
   bc up
   ```

2. Or copy a config into the workspace state dir first:

   ```bash
   cp examples/configs/small-team.json ~/.mycel/workspaces/<id>/preferences.json
   mycel up
   ```

## User Defaults (~/.bcrc)

You can set user-level defaults in `~/.bcrc`:

```toml
[user]
nickname = "@alice"

[defaults]
default_role = "engineer"
auto_start_root = true

[tools]
preferred = ["claude"]
```

These settings are merged with workspace config, with workspace taking precedence.

## Creating Custom Configurations

Use these examples as templates. Key sections to customize:

- `[workspace]` - Project name
- `[tools]` - Which AI tools to enable
- `[roster]` - How many agents to spawn with `bc up`
- `[channels]` - Default communication channels
- `[performance]` - Polling intervals based on your needs
