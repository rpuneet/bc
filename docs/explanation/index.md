# Explanation

Understanding-oriented documentation that explains how and why mycel works the way it does.

## Architecture

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | Component diagram, data flow, MCP integration, and package dependencies |
| [Design Decisions](design-decisions.md) | Architecture Decision Records (ADRs) for key technical choices |

## Subsystems

| Document | Description |
|----------|-------------|
| [Agents](agents.md) | Agent state machine, runtime backends, worktree management, and roles |
| [Apps](apps.md) | The plugin platform for external integrations (Slack, GitHub, WhatsApp, …) |
| [Notifications](notifications.md) | Notification routing, subscriptions, and delivery |
| [MCP Server](mcp.md) | Resources, tools, transports, and notifications |
| [Database](database.md) | Storage backends, schema management, encryption, and filesystem layout |
| [Networking](networking.md) | Client-server communication protocols, SSE events, MCP transports |

## Frontend

| Document | Description |
|----------|-------------|
| [Web Dashboard](web-ui.md) | React SPA architecture, component tree, routing, and state management |
| [Design System](design-system.md) | Solar Flare palette, design tokens, and shared component library |

## Infrastructure

| Document | Description |
|----------|-------------|
| [CI/CD](ci-cd.md) | GitHub Actions pipelines, test strategy, and release workflow |
| [Deployment](deployment.md) | Docker containers, networking, volumes, and resource management |
| [Security](security.md) | Threat model, secret encryption, agent isolation, and hardening |
