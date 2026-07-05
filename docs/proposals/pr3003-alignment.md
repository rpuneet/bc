# PR #3003 Alignment

Branch `feat/agents-revamp`, 127 commits ahead `main`. Closes #2999 (122 verification items). Status: OPEN; CI RED (Lint + Web). CodeRabbit auto-skipped (182 files > 150 cap); owner re-triggered, no human review.

---

## 1. What the PR must ship (extracted from proposals)

### `agents-revamp.md` (994 ln) — v2
- Avatar shape, runtime icon, loop pulse, status dot — §3 (ln 220–278), §6.1 (ln 423–433).
- Tabs Attach/Live/Config/Metrics/Code (addendum, ln 963–994 supersedes §6.2).
- Live event stream + 22 hook types + task graph — §8 (ln 516–578).
- Reactive Config (tmux vs docker), system prompt, MCPs, secrets, per-MCP env — §9 (ln 579–662).
- Create-modal: template + avatar + fork — §11 (ln 701–773).
- Templates CRUD page replaces Roles — §2.1 (ln 43–113), §15 (ln 891–922).
- 15 new API endpoints — §12.1 (ln 774–792).

### `multi-workspace-and-code-tab.md` (1395 ln) — extends agents-revamp
- G1–G8 goals (ln 83–102).
- Registry v2 `~/.bc/registry.json` — §4.1 (ln 187–268).
- `bc workspace list|add|remove|rename|default` — §4.6 (ln 403–421).
- Local scan + GitHub OAuth + clone — §4.4 (ln 325–368), §10.3 (ln 1050–1057).
- URL `/w/:wsId/*`, legacy 301s + Deprecation/Sunset — §5.1 (ln 425–465), §9.2–9.3 (ln 957–1005).
- Header + WorkspaceDropdown + AddWorkspaceModal + sidebar collapse — §5.3–5.6 (ln 504–590).
- Code tab top-level + per-agent: FileTree, MonacoReadOnly, Worktree dropdown, Diff, ripgrep — §6 (ln 592–770), §8 (ln 897–940).
- code-server iframe opt-in — §6.7 (ln 761–770).
- `pkg/deps/` + `/api/deps` + SSE logs + Settings UI; bc-db, bc-code-server, bc-browser (deprecated) — §7 (ln 772–895).
- Legacy shim + MCP path migration — §9 (ln 942–1024).
- 122 verification items + perf (10 ws × 20 ag <60s; 30 min eviction; <20 MB/ws) — §12.2 (ln 1184–1349).

### `multi-tenant-bcd.md` (588 ln) — M1–M11
- M1–M7 plumbing: per-request `*Services`, per-ws SSE + MCP, drop 501 branch — §11 (ln 303–352).
- M8 user-global assets (templates, secrets, MCPs, costs with `workspace_id`) — §19 (ln 498–522).
- M9 N ws per project (`ProjectPath/DataDir` split) — ln 523–538.
- M10 `bc workspace archive|restore`, `bc export/import-user-state` — ln 539–555.

### `bc-layout-v2.md` (597 ln, LOCKED 2026-04-17)
- ONE `bc.db` + `secrets.db` + `preferences.json` per ws at `~/.bc/w/<name>-<hash8>/` — §2 (ln 58–112).
- 8-char sha256 hash, `<project>/.bc/data-dir` pointer — §2.1–2.2.
- Registry absorbed into `~/.bc/settings.json` — §2.3, §7.3.
- `pkg/notify` replaces channels — §3.8 (ln 400–456).
- All legacy sidecar DBs dropped — §7 (ln 542–579).

### `bc-layout-v2-import.md` (431 ln)
- `bc workspace import-v1` single-shot, preflight abort if bcd up, `--dry-run/--force/--include-trash/--archive/--json` — §1 (ln 26–75), §2.0 (ln 57–73).

---

## 2. GitHub issues this PR is expected to close

| # | title | state | accept-criteria-status |
|---|---|---|---|
| 2999 | Agents page: complete implementation tracker | OPEN | 122-item checklist; all unchecked; partial per §5 |
| 3012 | Multi-workspace support | unverified | referenced in proposal metadata only |
| 3013 | URL + header refactor | unverified | referenced only |
| 3014 | Code tab | unverified | referenced only |
| 3015 | Optional dependencies manager | unverified | referenced only |
| 2979 | Agents Revamp: Fleet Command Center | CLOSED | parent epic, superseded by #2999 |

PR body only asserts `Closes #2999`. #3012–#3015 not explicitly closed.

---

## 3. Unresolved reviewer feedback / CI status

CI run 24530144415:

**Lint: FAIL** (30+ violations)
- `gofmt` — `migrate_runtime.go:54`, `pkg/agent/agent.go:343`, `pkg/agent/service.go:38`, `pkg/stats/tmux_sampler_test.go:15`, `pkg/workspace/migrate_v3.go:52`, `server/handlers/global_costs.go:17`.
- `errcheck` — `migrate_runtime.go:707–719` (7 unchecked `fmt.Fprintf`).
- `gosec` — G304 in `migrate_runtime_test.go:80,101`, `serve.go:274`, `agents.go:1488`; G301 `agents.go:1516`; G306 `agents.go:1526`; G204 `tmux_sampler.go:202`.
- `govet/shadow` — `init_test.go:76`; `migrate_runtime_test.go:89,94,97`; `workspace.go:188`; `cors_scope_test.go:75,78,92,194`.
- `govet/fieldalignment` — `migrate_runtime.go:53`, `tmux_sampler_test.go:13`, `agents.go:1471`.
- `noctx` — `migrate_runtime.go:384`, `migrate_runtime_test.go:201,255`.
- `misspell` — `container.go:364` ("honour").
- `staticcheck` S1009 — `agents_config_test.go:91`.

**Web: FAIL** — `WorkspaceDropdown.test.tsx` "opens on Cmd+K" — placeholder "search workspaces" not found (regression from `5399fc41` moving ⌘K → ⌘⇧W).

**PASS**: TUI, Landing, Test (race), PR-Quality. **Skipping**: Build, Container Scan, Security, Test-full (main-only).

**Reviewers**: CodeRabbit skipped (file count). Owner re-triggered but no review body. Zero human reviews.

---

## 4. Decisions made in recent conversations (memory)

- **2026-04-16** — branch must NOT merge until Puneet explicitly says ready; rebase/force OK (`feedback_agents_branch_until_approved`).
- **2026-04-10** — `pkg/channel` deleted (~8k LoC); replaced by `pkg/notify`; PR #2946 merged (`project_channels_revamp`).
- **2026-04-17** — bc-layout-v2 LOCKED: `~/.bc/settings.json` absorbs registry; `~/.bc/w/<name>-<hash8>/`; 8-char sha256; per-ws secrets.db only; `daemons`/`roles` tables dropped; `.migrated-*` markers deleted.
- **2026-04-17** — bc-layout-v2-import LOCKED: `bc workspace import-v1` is the only transformer; one-shot; `--dry-run` default.
- **v3 vision** — MERGE & ENHANCE: Activity = /live with filter; Stats = /metrics with filter; loop icon borderless.
- **Review before merge** (`feedback_coderabbit_review`). Tests: integration/e2e > coverage-padding units.

---

## 5. Gap table

Status: DONE | PARTIAL | TODO | BROKEN.

| # | requirement | source | commit | status | notes |
|---|---|---|---|---|---|
| 1 | Registry v2 schema (id, github_url, last_used_at, v=2) | mws §4.1 ln 189–235 | `7ec66b52` | DONE | |
| 2 | Registry migration + `.legacy` backup | mws §9.1 ln 944–955 | `7ec66b52`, `1abbedad` | DONE | |
| 3 | `bc workspace list/add/remove/rename/default` | mws §4.6 ln 403–421 | `39187cdc` | PARTIAL | all 5 subcommands not verified |
| 4 | `/api/workspaces` CRUD + activate | mws §4.5 ln 370–401 | `39187cdc` | DONE | |
| 5 | WorkspaceManager lazy load | mtd §5 ln 155–197 | `d2094174`,`89f2da40` | DONE | |
| 6 | M1 WorkspaceServices struct | mtd M1 | `24f1fc5e` | DONE | |
| 7 | M2 BuildWorkspaceServices factory | mtd M2 | `eafcfcf3` | DONE | |
| 8 | M3 handlers resolve from context | mtd M3 | `24da3592`,`d7e278bc`,`add6c7ca`,`86d0c161`,`b389739d` | DONE | Track-A closure bug fixed |
| 9 | M4 NewWithManager ctor | mtd M4 | `ddc264b3` | DONE | |
| 10 | M5 multi-ws dispatch | mtd M5 | `89f2da40` | DONE | |
| 11 | M6 per-ws SSE + scoped MCP | mtd M6 | `1cd385a0` | DONE | |
| 12 | M7 drop 501 branch | mtd M7 | `c3445d0c` | DONE | |
| 13 | M8a–g user-global assets | mtd §19 | `aafc8ede`..`c252f2b1` | DONE | |
| 14 | M11a DataDir helper | lv2 §2 | `a23167b4` | DONE | |
| 15 | M11b Workspace carries DataDir | lv2 §2 | `afa1d3f4` | DONE | |
| 16 | M11c preferences.json rename | lv2 §2.4 ln 138–159 | `821c8727` | DONE | |
| 17 | M11e workspace-runtime migration | mws §9.4 ln 1006–1015 | `1db9aa72`,`af61ea24` | DONE | |
| 18 | Byte-integrity migration + rollback | lv2 §6 ln 505–540 | `1abbedad` | DONE | |
| 19 | Local filesystem scan | mws §4.4.1 ln 327–343 | `c2ee0230` | DONE | |
| 20 | GitHub OAuth + repo list + clone | mws §4.4.2 ln 345–356, §10.3 | `c2ee0230`, `discover_github.go` | PARTIAL | clone+list present; OAuth flow, token storage, DELETE endpoint missing — see #54 |
| 21 | URL refactor `/w/:wsId/*` | mws §5.1–5.2 ln 425–503 | `cdf62068` | DONE | |
| 22 | Shared Header + Dropdown + SidebarToggle | mws §5.3 ln 504–557 | `f9a7df88`,`02303506` | DONE | Cmd+K test broken — #46 |
| 23 | AddWorkspaceModal (Scan/GitHub/Manual) | mws §5.6 ln 578–590 | `cdf62068` | PARTIAL | picker exists; 3-tab modal UX unverified |
| 24 | Legacy URL 301 + Deprecation/Sunset | mws §9.2–9.3 ln 957–1005 | — | **TODO** | no `legacy_scope.go` in server/ |
| 25 | Code tab: tree + Monaco + Diff + worktree | mws §6 ln 592–770 | `6338cac9`,`cdf62068`,`2f6bc485`,`522c306d` | DONE | |
| 26 | `pkg/files.SafeJoin` | mws §6.5 ln 727–749 | `9e76869c` | DONE | |
| 27 | Code handlers (tree/file/diff/search) | mws §6.4 ln 651–725 | `code.go`,`code_test.go` | PARTIAL | ripgrep search unverified |
| 28 | code-server iframe opt-in | mws §6.7 ln 761–770 | `48b5cb2c` | DONE | |
| 29 | Dependencies manager + UI | mws §7 ln 772–895 | `0013c2a6`,`522c306d`,`ff72dd2f` | DONE | |
| 30 | Agent tab reorder Attach/Live/Config/Metrics/Code | addendum ln 963–994 | `2869c4e4`,`c506807e` | DONE | |
| 31 | Clone → CreateAgentModal | ar §11.5 ln 766–773 | `422c07e8` | DONE | |
| 32 | Archive/unarchive lifecycle | ar §6.3 | `0713c4ed`,`4dcd78ae` | DONE | refuses running |
| 33 | Per-MCP env editor | ar §9.1 + v3 vision | `63edd8a0`,`07d8f585` | DONE | transactional |
| 34 | Cross-ws cost report `/costs` | mtd §19.M8 ln 519–522 | `fcf6a1e3` | DONE | |
| 35 | Settings Deps live status + logs | mws §7.4 ln 865–895 | `522c306d` | DONE | |
| 36 | `/api/health` DB probe | import §2.0 | `124f89eb` | DONE | |
| 37 | `~/.bc/daemon.{pid,log}` flat paths | lv2 §2 | `85fccdb5` | DONE | |
| 38 | Stats dedupe via TmuxSampler | v3 vision | `b97b9f0d`,`c0d4b193`,`6028c82b`..`d8795309` | DONE | |
| 39 | Live hides stopped by default | v3 + audit | `822697c6`,`5a58590e` | DONE | |
| 40 | Live stream + 22 hook types + task graph | ar §4, §8 | `e6399938`,`aabc9c4f`,`b04cedfb`,`bdf3aa46` | DONE | |
| 41 | Ralph loop server-side | ar §3.3 | `c1b0b90b`,`ea70b4f2`,`c5fe70c5` | DONE | |
| 42 | Templates CRUD + API | ar §2.1 ln 43–113 | earlier + M8b | DONE | |
| 43 | fmtCost consolidation + sub-cent | audit | `5399fc41`,`ff948ae4`,`047fc860` | DONE | |
| 44 | Top-bar chip canonical h1 | audit | `bb6e2d67`,`1d4e974f` | DONE | |
| 45 | Sidebar scoped `/w/:wsId/` | mws §5.4 | `3625022b` | DONE | |
| 46 | Cmd+K opens WorkspaceDropdown | mws item 41 | `5399fc41` (→Cmd+Shift+W) | **BROKEN** | test fails; decide acceptance or restore |
| 47 | Concurrent multi-ws dispatch test | mtd §12 | `af407df0`,`ae9e6046`,`ed10cc2e` | DONE | |
| 48 | 30-min idle eviction | mws item 119 | `e3c74c51` | DONE | |
| 49 | CORS + WorkspaceScope interaction | mtd §6 | `e48b670a`,`0e2bed15` | DONE | |
| 50 | Playwright scoped-workspace e2e | mws §12.1 | `8d877021` | DONE | |
| 51 | Tests sandbox `~/.bc/workspaces.json` | hardening | `edcf2420` | DONE | |
| 52 | MCP SSE `/_mcp/{agent}` → `/_mcp/{wsId}/{agent}` compat | mws §9.5 ln 1017–1024 | `1cd385a0` | PARTIAL | scoping done; compat redirect + vectors missing |
| 53 | Workspace isolation (tmux names) | mws item 99 | `37fe403f` | DONE | |
| 54 | GitHub OAuth handler + `~/.bc/github-token` (0600) + DELETE | mws §10.3 ln 1050–1057 | — | **TODO** | blocks #2999 items 16–21 |
| 55 | Lint green | CI | — | **BROKEN** | 30+ violations — see §3 |
| 56 | Web vitest green | CI | — | **BROKEN** | Dropdown Cmd+K test |
| 57 | `bc workspace archive/restore` | mtd §19.M10 | — | TODO | post-PR candidate |
| 58 | `bc export/import-user-state` | mtd §19.M10 | — | TODO | post-PR candidate |
| 59 | `bc workspace import-v1` | lv2-import §1 | — | TODO | post-PR (v2 layout) |
| 60 | Layout v2 `~/.bc/w/<name>-<hash8>/` | lv2 §2 | — | TODO | current is `~/.bc/workspaces/<12-id>` |
| 61 | Registry absorbed into `~/.bc/settings.json` | lv2 §2.3 | — | TODO | still `registry.json` |
| 62 | Single `bc.db` per workspace | lv2 §3 | — | TODO | sidecar DBs remain |
| 63 | `<project>/.bc/data-dir` pointer | lv2 §2.2 | — | TODO | |
| 64 | ripgrep search endpoint | mws §6.4.4, item 61 | unverified | TODO | |
| 65 | File >2 MB + binary download link | mws items 54,55 | code.go | PARTIAL | UX unverified |
| 66 | Monaco no external CDN | mws item 66 | `6338cac9` | PARTIAL | runtime load must be confirmed |
| 67 | Services eviction 30 min | mws item 119 | `e3c74c51` | DONE | |
| 68 | 1 h goroutine soak | mws item 120 | — | TODO | |
| 69 | 10 ws × 20 ag <60 s boot | mws item 117 | — | TODO | |
| 70 | <20 MB per idle ws | mws item 118 | — | TODO | |
| 71 | VS Code iframe sandbox/CSP | mws §10.4 ln 1059–1065 | `48b5cb2c` | PARTIAL | attrs/frame-src unverified (items 77,78) |
| 72 | Deps loopback guard + MYCEL_REMOTE bypass | mws §10.5, item 88 | — | TODO | |
| 73 | Deps SSE logs pause/resume | mws item 84 | `522c306d` | PARTIAL | polling exists; SSE unverified |
| 74 | bc-browser 409 + Deprecated badge | mws items 85,86 | `bc_browser.go` | PARTIAL | 409 + badge unverified |
| 75 | Registry atomic write on SIGKILL | mws item 12 | `7ec66b52` | PARTIAL | test fixture missing |
| 76 | Invalid wsId 404 + picker CTA | mws item 32 | `803dfa1e` | PARTIAL | CTA button unverified |

Legend: mws=multi-workspace-and-code-tab.md · mtd=multi-tenant-bcd.md · lv2=bc-layout-v2.md · ar=agents-revamp.md.

---

## 6. Recommended execution order to close PR #3003

Sizes: XS ≤ 30 min, S 1–2 h, M half-day, L day+.

1. **(XS)** `gofmt -s -w` on 6 files listed in §3; commit.
2. **(S)** Fix 30+ lint violations: errcheck `migrate_runtime.go:707–719`, gosec G304/G301/G306/G204, shadow, fieldalignment, noctx, misspell, staticcheck S1009. One commit per linter category.
3. **(XS)** Fix `WorkspaceDropdown.test.tsx` — update placeholder OR restore Cmd+K (decision: gap #46).
4. **(S)** GitHub OAuth handler + `~/.bc/github-token` (0600), `POST/DELETE/GET /api/auth/github` (gap #54, blocks #2999 items 16–21).
5. **(S)** Legacy URL shim `server/middleware/legacy_scope.go` with 301s + Deprecation/Sunset for `/live`, `/agents`, `/channels`, `/metrics`, `/tools`, `/workspace` (gap #24, covers #2999 items 23–36).
6. **(S)** MCP SSE compat redirect `/_mcp/{agent}/sse` → `/_mcp/{wsId}/{agent}/sse` (gap #52, #2999 item 36).
7. **(S)** Ripgrep search endpoint + debounce (gap #64, items 61–62).
8. **(XS)** Deps Start/Stop loopback guard with `MYCEL_REMOTE=1` bypass (gap #72).
9. **(XS)** bc-browser 409 on start + Deprecated badge (gap #74).
10. **(M)** Run 122-item #2999 verification on macOS; post results as PR comment with screenshots.
11. **(M)** Load/soak: 10 ws × 20 agents boot time; 1 h goroutine soak; memory (gaps #68–70). Defer if approved.
12. **(M)** Defer Layout-v2 items (#57–#63) to follow-up issues; note in PR body.
13. **(XS)** Rebase/squash to reduce file count, re-request CodeRabbit.
14. **(XS)** Await Puneet's explicit "merge it" (memory rule).

Items likely missing from current TaskList #1–#43: #4 GitHub OAuth, #5 legacy URL shim, #6 MCP compat redirect, #8 MYCEL_REMOTE guard, #9 bc-browser 409+badge, #10 full verification run, #11 load/soak, #12 Layout-v2 deferral note.

---

## Status count
- DONE: 47
- PARTIAL: 13
- TODO: 15 (of which 7 — #57–#63 — are Layout-v2 / M10 items likely post-PR)
- BROKEN: 3 (#46 Cmd+K, #55 Lint, #56 Web vitest)
