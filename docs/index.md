# mycel Documentation

mycel orchestrates teams of AI coding agents across your git repositories. One binary runs the CLI and the server; agents work in isolated sessions with their own git worktrees, and you steer everything from the CLI, the web UI, or the desktop app.

## Start here

| I want to... | Go to |
|--------------|-------|
| Install mycel and bring the server up | [Getting started](tutorials/getting-started.md) |
| Create and talk to my first agent | [Your first agent](tutorials/first-agent.md) |
| Connect Slack, Telegram, or GitHub to agents | [Set up apps](how-to/set-up-apps.md) |
| Understand how the pieces fit together | [Architecture](explanation/architecture.md) |
| Fix something that broke | [Troubleshoot](how-to/troubleshoot.md) |

## Tutorials — learn by doing

Step-by-step lessons that take you from zero to a working agent team.

- [Getting started](tutorials/getting-started.md) — install mycel, run `mycel up`, open the dashboard
- [Your first agent](tutorials/first-agent.md) — create, monitor, and message an agent

## How-to guides — get things done

Focused recipes for specific tasks.

- [Configure mycel](how-to/configure.md) — prefs.json, providers, and runtime backends
- [Set up apps](how-to/set-up-apps.md) — connect platforms and subscribe agents
- [Troubleshoot](how-to/troubleshoot.md) — common errors and their fixes

## Reference — look things up

Exact, verifiable descriptions of every interface.

- [REST API](reference/api-rest.md) — every HTTP endpoint
- [Settings API](reference/api-settings.md) — configuration shape and endpoints
- [CLI reference](reference/cli/mycel.md) — every command, auto-generated

## Explanation — understand the design

How mycel works under the hood and why it is built this way.

- [Architecture](explanation/architecture.md) — the server, clients, and data flow
- [Agents](explanation/agents.md) — lifecycle, repos, worktrees, and roles
- [Database](explanation/database.md) — the global store and cost ledger
- [Notifications](architecture-notifications.md) — apps, subscriptions, delivery
- [Web UI](explanation/web-ui.md) · [MCP](explanation/mcp.md)
- [Networking](explanation/networking.md) · [Security](explanation/security.md) · [Deployment](explanation/deployment.md)
- [Design decisions](explanation/design-decisions.md) — the reasoning behind key choices

## Contributing

- [Testing guide](contributing/testing.md) — run and write tests
- [System overview](overview.md) — a condensed tour of components and flows
