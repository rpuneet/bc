# Browse the Marketplace

The Marketplace is a live catalog of skills, MCP servers, and templates that you can hand to any running agent to install for itself.

## Overview

The Marketplace aggregates entries from eight sources into a single searchable catalog:

| Source | What it provides |
|--------|------------------|
| MCP Registry | Servers listed on `registry.modelcontextprotocol.io` |
| GitHub | Repositories tagged `mcp-server` and `claude-skill`, gated by star count |
| mycel | Templates from the local mycel template store |
| Claude | Skills from `anthropics/claude-plugins-official` |
| openclaw | Skills from the ClawHub catalog |
| Google | Extensions from the `gemini-cli-extensions` org |
| Glama | MCP servers listed on `glama.ai` |
| Smithery | MCP servers listed on `registry.smithery.ai` |

Each entry is one of three types — **MCP server**, **skill**, or **template**. The catalog is fetched live from the remote sources and cached on the server for one hour, so browsing is fast while listings stay current. The web UI refreshes its view every minute.

## Browse and Filter

Open the Marketplace from the web UI at `http://localhost:9374`. The header shows how many sources are currently active. Three controls narrow the list:

- **Search** — matches an entry's name and description.
- **Type** — filter to MCP Servers, Skills, or Templates.
- **Source** — filter to a single source (MCP Registry, Glama, Smithery, Claude skills, openclaw, Google, GitHub, or mycel).

Each card shows the entry's name, a type badge, a colour-coded source badge, its description, a star count for GitHub-sourced entries, and a link to the upstream repository or listing.

## Install to an Agent

The Marketplace does not install anything on the host. Instead, **Add** composes an install instruction and sends it as a message to the agents you pick — each agent then runs the install itself inside its own session.

1. Click **Add** on an entry.
2. Choose one or more target agents from the picker.
3. mycel composes the right install steps for that entry's type and source and dispatches them to each agent as a message.

The exact command an agent runs depends on the entry:

| Entry | Command the agent runs |
|-------|------------------------|
| MCP server | `claude mcp add "<name>" "<source-url>"` |
| Skill (openclaw) | `openclaw skills install "<slug>"` |
| Skill (Claude, GitHub) | `claude plugin marketplace add "<repo-url>"` then `claude plugin install "<name>@<marketplace>"` |
| Template | `mycel agent create <agent-name> --template "<name>"` |

Templates are the exception: the `mycel` source lists the templates already on
this machine (`~/.mycel/templates/`), so there is nothing to fetch. Adding one
tells the agent how to put it to use — creating an agent from it — rather than
installing anything.

For a skill sourced from a plugin marketplace, the agent first registers the marketplace and then installs the plugin from it:

```bash
# Step 1 — register the marketplace (first time only)
claude plugin marketplace add "https://github.com/owner/repo"

# Step 2 — install the plugin
claude plugin install "my-skill@repo"
```

Some Glama listings do not expose a runnable endpoint through their catalog API. For those, the install message contains manual steps instead of a ready-made command: open the listing, find its `npx`/`uvx` invocation or HTTP/SSE URL, and run `claude mcp add "<name>" <command-or-url>` with it.

## API

The same catalog is available over HTTP:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/marketplace` | List catalog entries; filter with `?type=`, `?source=`, and `?q=` |
| POST | `/api/marketplace/install` | Dispatch install instructions to named agents |

A GET request accepts a type filter, a source filter, and a free-text query. The install request names the entry and a list of agents; the response reports how many agents the instruction was dispatched to, along with any per-agent errors.

> Tip: install to a single agent first, confirm it picked up the skill or server, then roll the same entry out to the rest of your fleet.
