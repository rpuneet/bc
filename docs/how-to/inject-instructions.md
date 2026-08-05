# Inject Instructions

Injected instructions are guidance mycel writes into every agent's prompt —
a single place for conventions that every agent should follow, regardless of role.

## Overview

Each agent's prompt file (`CLAUDE.md`, `AGENTS.md`, `.cursorrules`, … depending
on the provider) is assembled in layers:

1. **Role / template prompt** — authored persona, written first by spawn setup.
2. **Mycel-managed section** — rewritten **idempotently** on every spawn and
   restart (markers `<!-- mycel-managed:start -->` … `<!-- mycel-managed:end -->`).
   This block includes:
   - Your injected instructions text
   - Identity (agent name + role)
   - MCP server names + credential env var **names** (never values)
   - Connected apps + platform credential env docs
   - This agent's notification subscriptions (`slack:*`, `gmail:…`, …)

Edits to subscriptions or apps show up on the **next spawn/restart** of the
agent (the managed section is not live-rewritten while the session is running).

## Author the Instructions

The instructions are a single text (or markdown) field. Set them from the web UI Settings page, or over the API:

```bash
curl -X PUT http://localhost:9374/api/settings/injected-instructions \
  -H "Content-Type: application/json" \
  -d '{"injected_instructions": "Report status before and after every task. Never merge without a green check."}'
```

The text is stored in the repo's runtime configuration and written to disk, but never contains secret values. Whatever you write is included verbatim inside the managed section.

## What an Agent Sees

Given the instruction above and an agent with the `mycel` and `github` MCP servers plus `GH_TOKEN` and `SLACK_BOT_TOKEN` credentials, the managed block looks like:

```markdown
<!-- mycel-managed:start -->
## mycel context

_This section is managed by mycel — rewritten on every spawn/restart. Do not edit by hand._

### Identity
- Agent: `fast-crane` (also `MYCEL_AGENT_ID`)
- Role: `base`

### Instructions

Report status before and after every task. Never merge without a green check.

### Available resources
MCP servers: github, mycel
Credential env vars: GH_TOKEN, SLACK_BOT_TOKEN

### Connected apps
- `slack` (Slack)

## Platform Credentials

You have access to these platform credentials via environment variables:

- SLACK_BOT_TOKEN: Slack Bot Token.

Your agent name is available as the `MYCEL_AGENT_ID` environment variable.

### Notification subscriptions
- `slack:*`
- `slack:general`

<!-- mycel-managed:end -->
```

Resource and subscription lists are generated **per agent**. Credential values stay in the vault and are only resolved into the agent's environment at spawn.

## Instructions vs. Roles

Injected instructions and role prompts are separate mechanisms that compose:

- **Role prompt** — authored per role, inherited and merged across parent roles, and written first.
- **Mycel-managed section** — global injected text plus live workspace context, rewritten after the role prompt so it applies to every agent without stacking duplicate appends.

Use role prompts for what a *kind* of agent should do, and injected instructions for the house rules that hold across your whole fleet.

> Tip: keep injected instructions short and universal. Anything specific to one job belongs in that agent's role prompt, not here.

## Prompt stack (operators)

| Layer | Owner | When written |
|-------|--------|----------------|
| Role / template `system_prompt` | you / template | spawn setup |
| Mycel-managed block (markers) | mycel | every spawn + restart |
| Repo / user `AGENTS.md` conventions | tool / user | outside mycel (Claude/Cursor may also load `~/AGENTS.md`) |

Do not hand-edit inside the managed markers — the next restart will overwrite that region.
