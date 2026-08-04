# Use Agent Templates

A template is the reusable half of an agent: the system prompt it starts with,
the MCP servers it should have, and the guardrails that stop it running away.
A template can optionally carry a **label** — `single-agent` or `multi-agent` —
so the UI can tell a persona from a composed system (#3552).

mycel ships a thin `blank` starting point. Richer blueprints (trader,
engineering-team, and the rest) land with the blueprint work in #3558.

## Pick a Template

```bash
mycel template list                          # every template, with its description
mycel template show blank                    # the full prompt and settings
mycel agent create critic --template blank --tool claude
```

The web UI shows the same set under **Templates**, where the prompt is editable in place.

## What Ships

| Template | Label | Purpose |
|----------|-------|---------|
| `blank` | `single-agent` | Empty prompt — write your own |

The older library of 35 task-prompt templates (`feature-dev`, `reviewer`, …)
is withdrawn on upgrade: unedited copies are removed; anything you changed
is left alone.

## Edit One

Templates are two files per template in `~/.mycel/templates/` — `<name>.json`
for the settings and `<name>.md` for the prompt. Edit either by hand, or
through the UI, and the change is picked up on the next agent you create.

Your edits are safe across upgrades. mycel installs or upgrades a built-in
only when the on-disk content still matches what mycel last wrote (content
hash in `.builtins.state`). An edited template is never overwritten, a
template you delete stays deleted, and withdrawn names never reappear.

## Guardrails

Two fields in a template's JSON are enforced while the agent runs:

- `max_cost_usd` — stop the agent when estimated spend crosses the cap
- `stuck_timeout_min` — mark the agent stuck when it has been idle too long

These have CLI and API support today; the web UI for them is #3574.

## Create Your Own

```bash
mycel template create my-persona \
  --description "What this agent does" \
  --prompt-file ./persona.md
```

Or use **Templates → New** in the UI. Set `"label": "single-agent"` (or
`"multi-agent"`) in the JSON when you know which kind it is.
