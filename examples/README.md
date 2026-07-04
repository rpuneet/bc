# bc Example Configurations

This directory contains example configurations for common bc use cases.

## Workspace Configurations

### solo-developer.toml

Minimal setup for individual developers:
- Single agent workflow
- Low resource usage
- Simple channel structure

```bash
cp examples/solo-developer.toml .bc/settings.json
bc up            # bootstraps the workspace
```

### team-workspace.toml

Full-featured setup for teams:
- Multiple concurrent agents (3 engineers, 1 tech lead, 1 QA, 1 manager)
- Structured communication channels (eng, qa, alerts, standup)
- Budget tracking ($50 default)
- GitHub integration

```bash
cp examples/team-workspace.toml .bc/settings.json
bc up            # bootstraps the workspace
bc agent create eng-01 --role engineer
bc agent create eng-02 --role engineer
```

### ci-cd-integration.toml

Optimized for CI/CD pipelines:
- Headless (non-interactive) operation
- Single agent for automated tasks
- Lower budget ($5 default)
- Aggressive worktree cleanup
- Color output disabled for logs

```bash
# In your CI pipeline:
cp examples/ci-cd-integration.toml .bc/settings.json
bc up --headless # bootstraps the workspace with defaults
bc agent send eng-01 "run tests and report results"
```

## User Configuration

### bcrc.toml

Personal defaults for all workspaces. Copy to `~/.bcrc`:

```bash
cp examples/bcrc.toml ~/.bcrc
```

Settings in `~/.bcrc`:
- Default nickname for channel messages
- Preferred AI tool and model
- Performance tuning for your machine
- Editor preferences

User settings are merged with workspace settings, with workspace taking precedence.

## Quick Start

1. Choose a configuration that matches your use case
2. Copy it to your project's `.bc/settings.json`
3. Start the server with `bc up` — it bootstraps the workspace
4. Start working with `bc up`

## Customization

All example files are heavily commented. Edit them to match your specific needs:

- Adjust `[roster]` for team size
- Configure `[tools]` for your AI providers
- Tune `[performance]` for your hardware
- Set up `[channels]` for your workflow

## See Also

- `bc config show` - View effective configuration
- `bc config edit` - Edit configuration in your editor
- `bc config set <key> <value>` - Set individual values
- `bc help config` - Full configuration documentation
