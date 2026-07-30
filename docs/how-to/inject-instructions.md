# Inject Instructions

Injected instructions are a block of guidance that mycel appends to every agent's prompt at spawn time — a single place to state conventions that every agent should follow, regardless of role.

## Overview

Each agent's prompt is assembled from its role, then mycel appends two things to the same prompt file:

1. The **injected instructions** text you author.
2. An auto-generated summary of the resources the agent can reach — the names of its available MCP servers and the names of its credential environment variables (names only, never values).

The result is a `## mycel instructions` block at the end of the agent's `CLAUDE.md`, applied to every agent uniformly and refreshed on each spawn and restart.

## Author the Instructions

The instructions are a single text (or markdown) field. Set them from the web UI Settings page, or over the API:

```bash
curl -X PUT http://localhost:9374/api/settings/injected-instructions \
  -H "Content-Type: application/json" \
  -d '{"injected_instructions": "Report status before and after every task. Never merge without a green check."}'
```

The text is stored in the repo's runtime configuration and written to disk, but never contains secret values. Whatever you write is appended verbatim to each agent's prompt.

## What an Agent Sees

Given the instruction above and an agent with the `mycel` and `github` MCP servers plus `GH_TOKEN` and `SLACK_BOT_TOKEN` credentials, the block appended to that agent's prompt reads:

```markdown
## mycel instructions

Report status before and after every task. Never merge without a green check.

### Available resources
MCP servers: mycel, github
Credential env vars: GH_TOKEN, SLACK_BOT_TOKEN
```

The resource summary is generated per agent, so each agent sees the servers and credential names that actually apply to it — the credential values themselves stay in the vault and are only resolved into the agent's environment at spawn.

## Instructions vs. Roles

Injected instructions and role prompts are separate mechanisms that compose:

- **Role prompt** — authored per role, inherited and merged across parent roles, and written first.
- **Injected instructions** — one global text, appended after the role prompt so it applies to every agent.

Use role prompts for what a *kind* of agent should do, and injected instructions for the house rules that hold across your whole fleet.

> Tip: keep injected instructions short and universal. Anything specific to one job belongs in that agent's role prompt, not here.
