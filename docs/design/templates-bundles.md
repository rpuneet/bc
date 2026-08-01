# Templates as one-stop bundles

Status: **design — awaiting sign-off** (Phase 2 of the plugin-era program,
board #40). Companion doc: [marketplace.md](marketplace.md).

A template should answer one question completely: *"if I create an agent
from this, what do I get?"* Today it answers a fifth of it. This doc grows
`pkg/template` from a name + prompt + MCP-name-list into a **complete agent
recipe** — provider, prompt, MCP servers, skills, tool policy, app channel
subscriptions, secret references, budgets — applied through one merge
engine with a diff preview, versioned, and shareable via the marketplace.

## Where we are (honest audit)

- `pkg/template.Template`: name, description, prompt file, MCP **names**
  (no specs), secrets, plugins, context files, tool policies, cost/stuck
  budgets. Stored as `<name>.json` + `<name>.md` pair; two-layer store
  (global `~/.mycel/templates/` + repo override) with override-wins.
- Apply exists only at create time: `applyTemplate`
  (`server/handlers/agents.go`) writes `CLAUDE.md` and merges **empty stub
  entries** into `.mcp.json` — the template knows MCP names but not how to
  run them, so it writes `{"github": {}}` and hopes the role config or the
  user fills it in. The stub-clobber bug this caused is patched at that
  one call site.
- Provider, model, runtime, env, app channels are all chosen manually in
  `CreateAgentModal` — the template doesn't carry them, so "template" is
  really "prompt preset".
- Roles (`pkg/home/roles.go`, DB-backed) carry a second, overlapping
  recipe: prompt, MCP server names, secrets, plugins, skills, rules,
  commands, lifecycle prompts, with BFS inheritance. `role_setup` writes
  these into the worktree on spawn. Two systems write the same files.

## The bundle

One template = one complete recipe. Everything is a **reference, never a
value** where secrets or machine-local state are involved.

```yaml
---
name: feature-dev
version: 1.2.0
description: Full-stack feature development
provider: claude          # provider registry ID
model: claude-sonnet-4-6  # optional; provider default when empty
runtime: docker           # optional; prefs default when empty

role: feature-dev         # capability reference (see "Roles" below)

mcps:                     # full specs or marketplace refs — no name-only stubs
  - name: mycel
    builtin: true         # daemon-provided /_mcp/{agent}
  - name: github
    ref: "mcp-registry:io.github.github/github-mcp-server@1.4.0"
  - name: playwright
    command: npx
    args: ["-y", "@playwright/mcp"]

skills:
  - ref: "claude:code-review@2.0.1"   # marketplace item, delegated install

tools:
  allowed: ["Bash(make *)", "Bash(go test *)"]
  denied:  ["Bash(rm -rf *)"]

apps:                     # channel subscriptions wired via pkg/notify
  - instance: slack
    channels: ["#engineering", "#merge"]

env:
  GITHUB_TOKEN: "secret:app:github:token"   # vault reference — never a value
  GOFLAGS: "-mod=mod"                       # plain values allowed for non-secrets

budgets:
  max_cost_usd: 25
  stuck_timeout_min: 30
---

# Feature Developer

You implement features, fix bugs, and write tests in an isolated worktree.
…system prompt is the markdown body…
```

### File format: markdown + YAML frontmatter

One file, `templates/<name>.md`. Chosen over the alternatives deliberately:

- **Roles already use exactly this format** (`ParseRoleFile` in
  `pkg/home/roles.go`) — one parser, one authoring convention, and the
  role→template migration below becomes nearly mechanical.
- The system prompt is the body — no `system_prompt_file` indirection, no
  `.json`+`.md` pair to keep in sync (today's store juggles both).
- It renders on GitHub, which is what "share a template" mostly means.
- TOML/JSON lose on multiline prompts; a separate YAML doc loses the
  prompt-is-body property and adds a second convention beside roles.

The store keeps its two layers (global + repo override) and grows a
loader for `.md` bundles. Existing `.json`+`.md` pairs are folded into
single `.md` files by a one-time local migration (per the no-legacy rule:
the pair format does not survive in tree).

## Apply semantics

All applies — create-time and apply-to-existing — run through the same
`Installer` plan/apply engine from [marketplace.md](marketplace.md).
A template apply is a batch of item applies plus template-owned files.

**Create-time** (the common case): render the full recipe into the fresh
entity dir — `CLAUDE.md` from the body, real `.mcp.json` entries (specs
resolved from the bundle or the marketplace ref — stub entries are gone),
tool policy into settings, env refs resolved at session start (values
never land on disk), app subscriptions registered via `pkg/notify`,
budgets onto the agent record. Marketplace-ref items (`ref:`) resolve
through the marketplace installer and produce receipts.

**Apply-to-existing** — the merge rules, stated once:

- On every apply, write a lock:
  `~/.mycel/agents/<name>/template.lock` — `{template, version,
  content_hash, files: [{path, sha256_after}], keys: {...}}`.
- A later apply (same template new version, or a different template) does
  a **three-way plan**: base = what the lock says the template wrote,
  ours = current on-disk state, theirs = the new bundle.
    - Base == ours (user untouched) → update to theirs silently.
    - Ours != base (user customized) → keep ours, flag in the diff
      preview with an opt-in "replace" per file/key.
    - New keys/files → add.
- The plan (exact diffs + warnings) is always shown before writing;
  `dry_run` is the default first request, apply confirms a plan ID.
  No template apply ever silently clobbers user state — this retires the
  applyTemplate wart class entirely.

**Versioning / upgrade**: `version` is semver, bumped by the author; the
lock pins what an agent got. "Upgrade available" = lock version < store
version. Upgrade is just apply-to-existing with the three-way plan. No
auto-upgrades.

## Relationship to roles: templates absorb the recipe, roles keep capabilities

Three options considered:

1. *Orthogonal* (status quo): both write the worktree; collisions patched
   per call site. This is the bug factory — rejected.
2. *Templates reference roles for everything*: keeps two overlapping
   recipe formats forever — rejected.
3. **Templates absorb the recipe; roles shrink to what only roles do**
   — recommended.

Concretely: prompt content, MCP servers, skills, plugins, secrets, rules,
commands move to the template (the bundle above already holds them). The
role keeps what the runtime genuinely keys on: **capabilities and
hierarchy** (`RoleCapabilities` / `RoleHierarchy`, parent/child agent
permissions) plus lifecycle prompts. A template names its role in one
field (`role: feature-dev`); `role_setup` stops writing prompt/MCP files
and only enforces capabilities.

Migration implications: each built-in role's recipe half is regenerated as
a built-in template (mostly mechanical — same frontmatter format);
`SeedDefaults` seeds bundles instead of stubs; the roles table keeps
name/capabilities/parents/lifecycle and drops the content columns. One-time
local migration, no compatibility layer in tree. Role inheritance for
*content* is not replicated in templates — bundles stay flat; if
composition proves necessary later, a single `extends:` is the escape
hatch (explicit non-goal for now).

## UI

- **Gallery** (`Templates.tsx` grows up): cards with name, description,
  provider badge, version, scope (global/repo), "used by N agents".
- **Detail — the "what you get" manifest**: the rendered plan for a
  hypothetical agent — provider+model, prompt preview, each MCP with its
  real spec and trust tier (marketplace refs show provenance), skills,
  tool policy, channels, env *references*, budgets. This is the same
  `Plan` structure the installer produces — the manifest is not a second
  rendering path.
- **One-click create**: "Create agent" on the detail page opens
  `CreateAgentModal` fully pre-filled (name suggestion + everything from
  the bundle); user can override any field; overrides are recorded in the
  lock so later upgrades respect them.
- **Agent detail**: shows the applied template + version, drift indicator
  (lock hash vs current files), "upgrade available" affordance.

## API sketch

| Route | Purpose |
|---|---|
| `GET  /api/templates` | list (existing), + version, scope, usage count |
| `GET  /api/templates/{name}` | bundle + rendered "what you get" manifest |
| `POST /api/templates` / `PUT /api/templates/{name}` | author (body = the .md bundle) |
| `POST /api/templates/{name}/plan` | `{agent}` → three-way `Plan` (dry run; also serves create preview with `agent: null`) |
| `POST /api/agents` with `template` | create-from-template — applies via the installer, writes the lock |
| `POST /api/agents/{name}/template/apply` | `{plan_id, confirmed_replacements[]}` — apply/upgrade on existing agent |
| `GET  /api/agents/{name}/template` | lock status: template@version, drift, upgrade availability |

Sharing: publishing a template to the marketplace is the marketplace's
verified-manifest PR flow (an entry pointing at the raw `.md`); importing
one is a normal marketplace template install into
`~/.mycel/templates/`. No separate sharing infrastructure.

## Delivery plan

- **P0 — the bundle** (~1.5 wk): `.md` bundle format + parser (reusing the
  role frontmatter parser), store migration off the json/md pair, full
  create-time apply through the installer (real MCP specs, env refs,
  budgets; kills stub entries), `template.lock`, pre-filled
  CreateAgentModal.
- **P1 — apply-to-existing** (~1 wk): three-way plan + diff preview,
  upgrade flow, drift indicator, "what you get" manifest on the detail
  page, app subscriptions + skills refs in the bundle.
- **P2 — roles absorption** (~1 wk): regenerate built-in roles as
  templates, shrink role storage to capabilities+hierarchy+lifecycle,
  `role_setup` stops writing content files. Gated on P0/P1 proving the
  bundle in real use.

## Open questions for Puneet

1. Roles absorption (P2): sign off on shrinking roles to
   capabilities+hierarchy+lifecycle now, or ship P0/P1 and revisit? It
   deletes the role content columns — cheap today, pricier after more
   roles accumulate.
2. Should a template pin `provider` at all, or only recommend it? Pinning
   makes bundles complete but couples shared templates to whichever
   providers the receiving machine has configured.
3. Repo-scoped (override-layer) templates: keep the second layer, or is
   user-global-only enough now that agents bind to repos per-agent? The
   layer exists but the workspace era it served is gone.
4. Create-time overrides in the lock: when a user overrides a bundle field
   at create (say model), should upgrades ever propose reverting to the
   template value, or is an override permanent until manually cleared?
