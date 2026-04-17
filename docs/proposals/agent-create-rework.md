# Agent create rework: `--template` / `--copy` / deprecate `--role`

- **Status:** Design locked, implementation pending
- **Owner:** unassigned
- **Related:** docs/proposals/bc-layout-v2.md (§agent creation API), PR #3003 followups §2.2
- **Last updated:** 2026-04-17

## Goal

Replace the current `bc agent create <name> --role <role>` command with

```
bc agent create <name> [--template <tmpl>] [--copy <agent>] [--tool <tool>] ...
```

so agents are seeded from user-authored markdown templates at `~/.bc/templates/*.md` (or copied wholesale from an existing agent) instead of picking a server-built-in "role" with baked-in permissions.

## Why this is a separate PR

PR #3003 ships the feat/agents-revamp surface with `--role` intact. The rework is **not** in #3003 because:

1. `--role` is a required flag today, validated against a role registry with hierarchy and capabilities (create_agents / assign_work / etc.). Ripping that out mid-PR risks breaking the create flow's security story.
2. Several *other* commands key off roles — `bc agent list --role`, `bc agent send-to-role`, and the role-based capability checks in `pkg/workspace/roles.go`. Deprecating those is bigger than it looks.
3. The v2 layout's "roles table deleted" is separate from the agent-create CLI change. The CLI can switch to `--template`/`--copy` *before* the table is dropped.

## Current `--role` call sites (map)

Grep of `./...` for `role` references in the agent-creation path:

| File | Lines | Usage |
|---|---|---|
| `internal/cmd/agent.go` | 313, 340, 345, 438–445 | `agentCreateRole` var, `--role` flag declaration, `MarkFlagRequired`, validation + `parseRoleStr` |
| `internal/cmd/agent.go` | 67, 89–92 | CLI help examples |
| `internal/cmd/agent.go` | 279–289, 409 | `agent send-to-role` sub-command |
| `internal/cmd/agent.go` | 320, 348 | `agent list --role` filter |
| `internal/cmd/agent_health.go` | — | role in health DTO |
| `internal/cmd/role.go` | — | entire `bc role` sub-command tree (list/show/grant/revoke) |
| `internal/cmd/init_wizard.go` | 246 | `state` param carrying role-seed info |
| `internal/cmd/init.go` | — | `withWorkspace` helper (currently unused) |
| `internal/cmd/mcp.go` | — | role gating on MCP access |
| `internal/cmd/root.go` | — | capability check for root cmd |
| `server/server.go` | — | role in auth middleware (defense-in-depth) |
| `server/mcp/server.go`, `tools.go`, `resources.go` | — | MCP capability table keyed by role |
| `server/handlers/agents_config.go` | — | DTO.Role field |
| `server/handlers/agents.go` | — | POST /api/agents accepts `role` in body |
| `server/handlers/providers.go` | — | role in agent list |
| `server/handlers/stats_agents.go` | — | role grouped in stats |
| `pkg/agent/service.go` | — | `CreateOptions.Role` required field |
| `pkg/agent/agent.go` | — | `Agent.Role` struct field |
| `pkg/workspace/roles.go` | — | `RoleCapabilities`, `RoleHierarchy`, `parseRoleStr`, `ValidateCapability` |

That's ~20 files. A single PR rewriting them all would be unreviewable.

## Phased implementation

### Phase A — additive (can ship any time, small PR)

Add `--template <name>` and `--copy <agent>` flags to `bc agent create` **without** removing `--role`. Precedence: `--copy` > `--template` > `--role`. When `--template` or `--copy` is set, `--role` is no longer required.

- New flag vars in `internal/cmd/agent.go`
- Lookup templates via `~/.bc/templates/<name>.md` (already exists — `pkg/template`).
- Copy via `agent.Service.Clone(srcName, newName)` — the clone API already exists (shipped in `422c07e8`) for the Web UI; expose it on the CLI.
- `CreateOptions` gains `Template string` and `CopyFrom string`. `Role` stays required-or-derived.
- Validation: if `--template=X` is given but no template file exists at `~/.bc/templates/X.md`, error out early.
- Derive a default `--role` value so existing role-keyed code paths keep working: when template flag is set, use role `"base"`; when copy flag is set, inherit the source agent's role.

Tests: one new test per flag path + one that pins precedence.

**PR size:** ~200 LOC Go + ~40 LOC tests.

### Phase B — deprecate `--role` (small PR, after Phase A bakes)

- Remove `MarkFlagRequired("role")` from `agentCreateCmd`.
- When no `--template`/`--copy`/`--role` is given, default to `--template=base` (ship a `~/.bc/templates/base.md` via an install-time step or via `pkg/template` embed).
- When `--role` is given, emit `--role is deprecated; use --template instead (see docs/proposals/agent-create-rework.md)`.
- Update CLI help/examples.

Tests: deprecation warning appears; absence of any flag defaults to `base` template.

**PR size:** ~60 LOC Go.

### Phase C — drop `--role` and the `roles` table (bigger PR, layout-v2 scope)

This is layout-v2 territory (docs/proposals/bc-layout-v2.md):

- Delete `--role` flag, `agentCreateRole` var, `parseRoleStr`.
- Delete `bc agent list --role`, `bc agent send-to-role` (or rewrite against templates).
- Delete `bc role` command tree entirely.
- Delete `pkg/workspace/roles.go` (RoleCapabilities, RoleHierarchy, ValidateCapability).
- Drop the `roles` table from `bc.db` (migration: SQL `DROP TABLE roles`).
- Remove `Role` from `Agent`, `CreateOptions`, DTOs; replace with `Template` where it adds value.
- Rewrite MCP capability checks to key off template name or a new explicit capability grant map (design discussion needed).
- Rewrite the auth middleware's role gate to key off something else.

**PR size:** ~1,000+ LOC Go + tests. Needs its own proposal doc for the capability replacement.

**Gate:** Phase C ships with the layout-v2 migration, not before. The v2 import tool (bc-layout-v2-import.md) is the piece that transitions existing users; a pre-v2 rollout of Phase C would leave old installs stranded.

## Non-goals for Phase A

- No changes to MCP capability checks.
- No changes to `bc role` command tree.
- No schema changes — `roles` table stays.
- No hierarchy/parent changes — `--parent` flag keeps its current semantics.
- No change to how agent prompts are authored; templates already exist and are used by the Web UI's CreateAgentModal.

## Test plan per phase

- **A:** unit tests for flag precedence, template-missing error, copy-source-missing error, both-set-error; integration test that spins up `httptest` against `bc agent create --template foo`.
- **B:** CLI test that `bc agent create --help` shows `--role` with "deprecated" prefix; e2e test for the default-to-`base` path.
- **C:** grep sweep that `--role` is gone from all .go files; schema migration tested against a seeded v2 DB.

## Risks

- **Phase A risk:** "Two ways to create an agent" — surface area doubles while `--role` still works. Mitigate by documenting which flag wins and by emitting a once-per-session notice when multiple are set.
- **Phase B risk:** users running scripted `bc agent create --role engineer` see a deprecation warning on every run. Deprecation TTL should match the usual 6-month window.
- **Phase C risk:** MCP capability breakage. Mitigate by designing the replacement capability system as part of Phase C's proposal, not reused from the existing role table.

## Open decisions

- **Default template name.** Proposed: `"base"`. Alternative: whatever `CurrentTemplate` flag points at. Prefer `"base"` because it matches the v2 layout's single-default story.
- **`--copy` and the source agent's state.** Does clone copy memory? MCP config? Working set? Current `agent.Service.Clone` copies prompt + tool + role; Phase A stays with that contract.
- **Non-interactive vs interactive fallback.** If no flag is given and stdin is a TTY, prompt for a template pick. Phase A skips this — just default to `base`.

## References

- `docs/proposals/bc-layout-v2.md` §agent-creation-api — spec source
- `docs/proposals/pr3003-followups.md` §2.2 — work-queue entry
- Issue #2999 — original checklist; items 87–95 cover agent-create flow
- `pkg/agent/service.go:CreateOptions` — where the new fields land
- `pkg/template/` — existing template storage, seeded by `M8b` (user-global templates)
