# mycel Documentation

mycel is a CLI-first orchestration system for coordinating teams of AI coding agents across multiple repositories.

This documentation is organized following the [Diataxis framework](https://diataxis.fr) into four categories:

## [Tutorials](tutorials/index.md) -- Learning-oriented

Step-by-step guides for getting started with bc.

- [Getting Started](tutorials/getting-started.md) -- Install and run your first workspace
- [Your First Agent](tutorials/first-agent.md) -- Create, monitor, and communicate with an agent

## [How-To Guides](how-to/index.md) -- Task-oriented

Practical guides for accomplishing specific tasks.

- [Configure your workspace](how-to/configure-workspace.md) -- Settings, providers, and runtime backends
- [Notification Architecture](architecture-notifications.md) -- Notification gateway, platform integrations, and subscriptions
- [Set Up Notifications](how-to/set-up-notifications.md) -- Connect platforms and subscribe agents
- [Troubleshoot issues](how-to/troubleshoot.md) -- Common errors and fixes

## [Reference](reference/index.md) -- Information-oriented

Technical reference material for APIs, CLI commands, and configuration.

- [REST API](reference/api-rest.md) -- All HTTP endpoints
- [Settings API](reference/api-settings.md) -- Configuration endpoints
- [CLI Reference](reference/cli/bc.md) -- Auto-generated command documentation

## [Explanation](explanation/index.md) -- Understanding-oriented

Deep dives into architecture, design decisions, and subsystem internals.

- [Architecture](explanation/architecture.md) -- Component diagram, data flow, package dependencies
- [Design Decisions](explanation/design-decisions.md) -- ADRs for key technical choices
- [Agents](explanation/agents.md) -- State machine, runtimes, worktrees, roles
- [MCP Server](explanation/mcp.md) -- Resources, tools, transports, notifications
- [Database](explanation/database.md) -- Schema, ER diagram, migrations
- [Web Dashboard](explanation/web-ui.md) -- React SPA architecture
- [TUI](explanation/tui.md) -- Terminal UI architecture
- [Design System](explanation/design-system.md) -- Solar Flare palette and tokens
- [Networking](explanation/networking.md) -- Protocols, SSE, CORS
- [CI/CD](explanation/ci-cd.md) -- Pipelines and release workflow
- [Deployment](explanation/deployment.md) -- Docker, networking, volumes
- [Security](explanation/security.md) -- Threat model, encryption, isolation

## Other

- [Contributing: Testing](contributing/testing.md) -- How to run and write tests
- [Screenshots](screenshots/) -- Dashboard and landing page screenshots
- [System Overview](overview.md) -- Architecture layers, components, data flow diagrams
