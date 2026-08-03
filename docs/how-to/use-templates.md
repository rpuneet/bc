# Use Agent Templates

A template is the reusable half of an agent: the system prompt it starts with, the MCP servers it should have, and the guardrails that stop it running away. mycel ships 36 of them, so `mycel agent create` usually means picking one rather than writing one.

## Pick a Template

```bash
mycel template list                          # every template, with its description
mycel template show reviewer                 # the full prompt and settings
mycel agent create critic --template reviewer --tool claude
```

The web UI shows the same set under **Templates**, where the prompt is editable in place.

## What Ships

Templates are grouped by the kind of work rather than by seniority. All of them assume they are working in a real repository with real consequences.

| Area | Templates |
|------|-----------|
| Building | `feature-dev`, `bug-fix`, `refactor`, `test-writer`, `type-tightener`, `backend-service`, `frontend-ui`, `api-designer` |
| Reviewing and shipping | `reviewer`, `pr-shepherd`, `ci-fixer`, `release-manager`, `changelog`, `issue-triage` |
| Performance and cost | `perf-optimizer`, `sql-optimizer`, `cost-optimizer` |
| Data | `db-migration`, `data-pipeline`, `ml-experiment`, `scraper` |
| Operations | `devops-infra`, `containerize`, `observability`, `oncall-responder` |
| Quality | `security-audit`, `accessibility-audit`, `dependency-upgrade`, `i18n` |
| Understanding and planning | `legacy-archaeologist`, `researcher`, `spec-writer`, `docs-writer`, `integration-builder` |
| Coordination | `manager`, `blank` |

`blank` is deliberately empty: use it when you want to write the prompt yourself.

## Edit One

Templates are two files per template in `~/.mycel/templates/` — `<name>.json` for the settings and `<name>.md` for the prompt. Edit either by hand, or through the UI, and the change is picked up on the next agent you create.

Your edits are safe across upgrades. mycel installs a built-in only when it has never installed that one before, so an edited template is never overwritten, and a template you delete stays deleted rather than reappearing at the next daemon start. The bookkeeping lives in `~/.mycel/templates/.builtins.state`.

## Guardrails

Two fields in a template's JSON are enforced while the agent runs:

| Field | Effect |
|-------|--------|
| `max_cost_usd` | The agent is stopped once its cumulative session spend reaches this. Omit it for open-ended work. |
| `stuck_timeout_min` | A working agent that produces no event for this long is flagged as stuck. |

Built-ins set `stuck_timeout_min` — detection is harmless — and set `max_cost_usd` only where the work is naturally bounded, such as `changelog` or `issue-triage`. Nothing is capped by default, because a cap that stops a long refactor halfway is worse than no cap.

## What a Template Does Not Do Yet

`secrets` and `plugins` are accepted in a template's JSON and are **not** applied to agents ([#3550](https://github.com/rpuneet/mycel/issues/3550)). Listing a secret there does not give the agent that credential — use [Manage secrets](manage-secrets.md) instead. The same is true of `tool_policies`, `context_files` and `system_prompt_file`, which the REST API accepts and nothing reads. The UI no longer offers these fields; the JSON still holds any values you saved earlier.

## Create Your Own

```bash
mycel template create my-template            # scaffolds the json/md pair
$EDITOR ~/.mycel/templates/my-template.md    # write the prompt
mycel agent create worker --template my-template
```

A good prompt says what the agent is for, how to decide when the request is ambiguous, and what "done" means. The built-ins are worth reading as examples before writing one — `bug-fix` and `oncall-responder` are the shortest useful ones.
