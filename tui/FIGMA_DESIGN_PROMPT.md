# Figma Design Prompt: "bc" TUI — Complete Redesign

## What This Is

**bc** is mission control for AI coding agents. You spawn a fleet of AI engineers, managers, QA — each working in isolated git worktrees on your codebase. This TUI is the cockpit: monitor agent status, read their conversations, track costs, inspect their memory, stream their logs. The user is a senior engineer running 5–20 agents simultaneously.

**The current UI is broken.** These are the real problems to solve:

1. **70% of the screen is empty** — most views use a fraction of available space, leaving vast blank regions below the content
2. **No persistent navigation** — there's no visible tab bar; the user relies on invisible number keys and a bottom footer they have to memorize
3. **Double footer redundancy** — every view has a view-specific hint bar AND a global hint bar at the bottom, wasting 2-3 rows and creating confusion about which shortcuts work where
4. **Chat is unreadable** — message bubbles overflow their borders, text bleeds past container edges, all messages look identical with no sender differentiation
5. **Flat visual hierarchy** — everything is the same visual weight; your eye has nowhere to land
6. **Primitive metric cards** — bordered boxes with plain numbers, no color, no context, no spark of life
7. **Command bar is at the bottom** — not a floating overlay; it just appears under all the content, easily missed
8. **Inconsistent borders** — some views use Panel borders, some don't, some use full-width horizontal rules, some have nothing
9. **No empty state design** — when there's no data (common for new workspaces), you just get a blank screen

**Design goal**: Make it feel like **Linear dark mode crossed with btop** — a dense, professional, information-rich cockpit that power users love and newcomers can navigate. Every pixel (character cell) earns its place.

---

## Design Canvas

Everything is a **monospace character grid**. 1 cell = 1 character. Use **Berkeley Mono**, **JetBrains Mono**, or **SF Mono**.

Design at **3 viewport sizes**:

| Viewport | Grid | Role |
|----------|------|------|
| **Standard** | 120 cols × 30 rows | Primary design target |
| **Compact** | 80 cols × 24 rows | Must fully work, nothing broken |
| **Wide** | 160 cols × 40 rows | Luxury spacing, side panels |

---

## Color System — "Deep Space"

The current theme uses basic ANSI names (`cyan`, `gray`, `blue`) which renders differently on every terminal. Design with a specific palette that will be approximated in 256-color mode.

**Philosophy**: The background should be deep enough that colored text feels like it *glows*. Like a radar screen in a dark room. Colors are surgical — never decorative, always semantic.

### Backgrounds (4 elevation tiers)

| Token | Hex | Usage |
|-------|-----|-------|
| `void` | `#07090F` | Absolute deepest — behind the app, visible as thin gaps between panels |
| `base` | `#0D1117` | Primary canvas — the default background of all content areas |
| `surface` | `#151B23` | Elevated panels — cards, bordered regions, table containers |
| `overlay` | `#1B2230` | Highest elevation — command bar, modals, dropdowns, tooltips |

These 4 tiers create depth without shadows or gradients. The difference between each is subtle (4-6 HSL lightness units) but perceptible. Panels "float" above the base by being slightly brighter.

### Borders (3 levels)

| Token | Hex | Usage |
|-------|-----|-------|
| `border-ghost` | `#1B2230` | Barely-visible structural borders. You feel them more than see them. Separators between areas |
| `border-subtle` | `#2D333B` | Standard panel/card borders. Visible but quiet |
| `border-focus` | `#4A9EFF` | Active/focused element. The only border that *pops* |

**Key insight from btop/lazygit**: The focused panel should have a bright border; everything else should have borders so subtle they almost disappear. This creates instant visual hierarchy.

### Text (4 levels)

| Token | Hex | Usage |
|-------|-----|-------|
| `text-bright` | `#F0F3F6` | Primary text, data values, active content |
| `text-normal` | `#C9D1D9` | Standard body text, table data |
| `text-muted` | `#768390` | Labels, timestamps, metadata, column headers |
| `text-ghost` | `#3D444D` | Disabled items, placeholder text, decorative borders |

### Semantic Colors (5 hues, each with 2 shades)

| Token | Bright | Muted | Meaning |
|-------|--------|-------|---------|
| `blue` | `#4A9EFF` | `#1A3A5C` | Primary interactive: selection, focus, links, working state |
| `green` | `#3FB950` | `#1A3B2A` | Success, healthy, done, engineers |
| `amber` | `#D4A72C` | `#3D3117` | Warning, stuck, caution, managers |
| `red` | `#F85149` | `#3D1B1B` | Error, destructive, critical |
| `violet` | `#A371F7` | `#2D1F4E` | AI/memory, badges, accent |

The "muted" shade is used for backgrounds of status badges — e.g., a green status badge has `green-muted` background with `green-bright` text and symbol. This creates pill-like badges that pop without being garish.

### Role Colors

| Role | Color | Glyph |
|------|-------|-------|
| Root | `violet` | `◆` |
| Engineer | `green` | `●` |
| Manager | `amber` | `▲` |
| Tech Lead | `blue` | `◇` |
| QA | `red` | `■` |
| UX | `blue` | `○` |
| System | `text-ghost` | `·` |

Use **geometric glyphs** (not emoji) for role indicators. They render consistently across all terminals and look cleaner at small sizes.

---

## Status Symbol System

Every status combines a **symbol** + **color** + **text label** (never color alone):

| State | Symbol | Color | Rendering |
|-------|--------|-------|-----------|
| Working | `◆` | `blue` | `◆ working` on blue-muted background |
| Idle | `○` | `text-muted` | `○ idle` no background |
| Done | `✓` | `green` | `✓ done` on green-muted background |
| Error | `✗` | `red` | `✗ error` on red-muted background |
| Stuck | `▲` | `amber` | `▲ stuck` on amber-muted background |
| Stopped | `·` | `text-ghost` | `· stopped` no background |
| Pending | `–` | `text-ghost` | `– pending` no background |

The muted background creates a **pill badge** effect. This is a key design upgrade — status indicators have visible bounding boxes, making them scannable in dense tables.

---

## Layout Architecture — The New Shell

The biggest structural change: **a persistent top navigation bar** that replaces the invisible number-key system and the redundant double-footer.

```
 bc ◆ myproject    Dashboard  Agents  Channels  Costs  Logs  Roles  Memory  Worktrees  Help     ← Top Bar (always visible)
─────────────────────────────────────────────────────────────────────────────────────────────────


                                    VIEW CONTENT AREA
                              (fills ALL remaining space)


─────────────────────────────────────────────────────────────────────────────────────────────────
 [:] command  [/] filter  [?] help                                              3 agents working  ← Status Bar
```

### Top Bar

- `bc` wordmark in bold blue, followed by `◆` role glyph and workspace name in `text-muted`
- Tab labels are plain text. **Active tab**: `text-bright` + bold + underline character below (Unicode `▔` or just color). **Inactive tabs**: `text-muted`
- At 80 cols: collapse to `1·Dash 2·Agt 3·Ch 4·Cost 5·Log` (number + dot + abbreviated label)
- At 120+ cols: full labels, spaced with 2-char gaps
- The top bar uses `surface` background — slightly elevated from the content area below

### Status Bar (replaces both footers)

- Single bottom row on `surface` background
- Left side: global shortcuts `[:] command  [/] filter  [?] help` — always the same, always visible
- Right side: live status summary — `3 agents working · $47.23 spent · 2 unread` — contextual data that changes based on workspace state
- View-specific hints appear **inline within the view content**, not in the footer. Each view shows its own hint line below its header where it's actually relevant

### Content Area

- Uses `base` background
- `padding: 1` on all sides (1 char horizontal, 1 row vertical)
- This is the only area that changes between views
- When drilled into a detail: a **breadcrumb** appears as the first line: `Agents › eng-01 › Peek` with `›` in `text-ghost` and the current level in `text-bright`

---

## Component Library — New Design

### 1. MetricCard (redesigned)

The current cards are just bordered boxes with a number. The new design uses **sparkline context** and **pill badges**.

**At 120+ cols, expanded format:**

```
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  12              │  │  5              │  │  3              │  │  1              │
│  Total Agents    │  │  ◆ Working      │  │  ○ Idle         │  │  ▲ Stuck        │
│  ▁▂▃▃▅▅▇▇██     │  │  ▁▃▅▇█▇▅▃▁▃    │  │                 │  │                 │
└─────────────────┘  └─────────────────┘  └─────────────────┘  └─────────────────┘
```

- Big number in `text-bright` bold
- Label below with status symbol + color
- Optional **braille sparkline** showing 10-point trend history (btop-inspired)
- Card border: `border-subtle`, padding: 1 char horizontal
- Cards use `surface` background, making them visually elevated from `base`

**At 80 cols, inline format:**

```
 12 agents · ◆ 5 working · ○ 3 idle · ▲ 1 stuck
```

### 2. DataTable (redesigned)

The current table has no visual distinction between header and data. New design:

```
 STATUS    NAME          ROLE         STATE        COST       UPTIME
 ─────────────────────────────────────────────────────────────────────
 ▸ ◆      eng-01        ● engineer   ◆ working    $12.50     2h 15m      ← selected: full row in blue
   ◆      eng-02        ● engineer   ◆ working    $8.30      1h 45m
   ○      eng-03        ● engineer   ○ idle        $3.20     3h 10m      ← idle rows slightly dimmer
   ✓      eng-04        ● engineer   ✓ done        $6.80     0h 55m
   ▲      eng-05        ● engineer   ▲ stuck       $4.10     1h 20m      ← amber tint on status
 ──────── managers ──────────────────────────────────────────────────      ← group separator
   ○      mgr-01        ▲ manager    ○ idle        $6.12     4h 00m
```

Key differences from current:

- Column headers in `text-muted` with a `border-ghost` separator line below — not bold, not bright
- Selected row: `▸` in `blue` + entire row text shifts to `blue-bright`. No background color change (looks bad in most terminals)
- Role glyphs (●▲◇) instead of text role names — saves horizontal space, adds visual texture
- Group separators: `──── group name ────` in `text-ghost`, lowercase, minimal
- Idle/stopped rows: entire row in `text-muted` (dimmed), making active agents visually prominent
- Status pills: symbol + state text colored together

### 3. ChatMessage (completely redesigned)

The current chat is broken — bubbles overflow, no visual sender distinction, all messages look the same. New design uses a **Slack/Discord-style flat layout** instead of bubbles:

```
 ▲ mgr-01                                                              12:30
   Team: prioritize the auth module. @eng-01 take the JWT refresh flow.
   👍 2  ✓ 1

 ● eng-01                                                              12:31
   On it. Starting with token.go now.

 ◇ tl-01                                                               12:32
   Make sure to add refresh token rotation. See RFC 6749 section 6.
   This is important for security compliance — the current implementation
   doesn't rotate tokens on refresh which is a vulnerability.
```

Key design decisions:

- **No borders/bubbles at all** — borders cause overflow issues and waste space. Use spacing instead
- Sender name: role glyph (colored) + bold name + timestamp right-aligned in `text-muted`
- Message body: indented 3 chars under sender, in `text-normal`
- `@mentions`: bold + `blue` colored
- Reactions: below message, in `text-muted` with emoji
- 1 blank line between messages for visual separation
- Own messages: same layout, but sender name shows `(you)` suffix in `text-muted`
- Unread messages: a `── 3 new messages ──` divider in `blue` before unread content

**Message input** (bottom of chat):

```
 ─────────────────────────────────────────────────────────────────
  ▸ Type a message...  (@mention to tag agents)           [Enter]
```

- Single-line input with `text-muted` placeholder
- `surface` background to elevate from chat content
- `border-ghost` top border only

### 4. Panel (simplified)

```
  Health ──────────────────────────────
  ● 92% healthy

  ◆ Working   5   ████████████░░░  80%
  ○ Idle      3   ████░░░░░░░░░░░  20%
  ▲ Stuck     1
```

- **No box borders** — just a title followed by a thin horizontal rule in `border-ghost`
- Title in `text-bright` bold
- Content starts on the next line
- Uses whitespace and alignment for structure, not borders
- This eliminates the border overflow issues and saves 2 chars horizontal + 2 rows vertical per panel

### 5. ProgressBar (new thresholds)

```
  ████████████░░░░░░░░  52%     ← blue (normal)
  █████████████████░░░  84%     ← amber (warning)
  ███████████████████░  96%     ← red (critical, pulsing)
```

- Uses `▓` (medium shade) for filled instead of `█` — slightly lighter, more legible
- Uses `░` (light shade) for empty
- Threshold colors: blue <75%, amber 75-90%, red >90%
- At >95%: the percentage number itself turns red and bold

### 6. CommandBar (floating overlay — biggest change)

The current command bar appears inline at the bottom. The new design is a **centered floating palette** like VS Code's Ctrl+P:

```
                    ┌──────────────────────────────────────────────┐
                    │                                              │
                    │  : agents█                                   │
                    │                                              │
                    │  ▸ agents       Navigate to Agents view      │
                    │    agent-send   Send message to agent        │
                    │    attach       Attach to tmux session       │
                    │    agent-peek   View agent output            │
                    │                                              │
                    │  Tab complete · Esc cancel                   │
                    └──────────────────────────────────────────────┘
```

- `overlay` background with `border-subtle` border
- Centered horizontally, positioned at ~25% from top vertically
- Width: 50 chars (fixed)
- `:` prompt in bold `blue`
- Cursor as blinking `█` (design as solid block)
- Selected suggestion: `▸` prefix + `blue` text + bold
- Non-selected: `text-normal` command + `text-muted` description
- Fuzzy match characters highlighted in `violet`
- Dim hint text at bottom
- **The underlying view is darkened** (add a `void` color overlay at ~50% opacity — simulated by using very dim/ghost text for the background content)

### 7. FilterBar (inline, top of view)

```
  / engineer█                                        Esc close · c clear
```

- Appears as the first line of the content area, pushing content down
- `/` in bold `blue`
- Search text in `text-bright`
- Hints right-aligned in `text-muted`
- Matching items in the list below: matched characters highlighted in `violet` bold

### 8. ConfirmDialog (modal)

```
              ┌─────────────────────────────────────┐
              │                                     │
              │  Kill eng-01?                        │
              │                                     │
              │  This force-terminates the tmux      │
              │  session and removes the worktree.   │
              │                                     │
              │            [y] Yes    [n] No         │
              │                                     │
              └─────────────────────────────────────┘
```

- Centered floating modal on darkened background
- **Red** border for destructive (kill, delete), **amber** for cautionary (stop, restart)
- Title in bold, same color as border
- Body text in `text-normal`
- Key hints: `[y]` in bold border-color, `[n]` in `text-muted`

### 9. EmptyState (new component)

For every view when there's no data:

```
              ○

         No agents running

    Run  bc up  to start your team,
    or  bc agent create  to add one.

         [:] command to navigate
```

- Centered vertically and horizontally in the content area
- Large status symbol at top in `text-ghost`
- Title in `text-muted`
- Helpful commands in `text-normal` with command text in `blue`
- This replaces the current blank screen that gives zero guidance

### 10. Skeleton (loading placeholder — new)

```
  ░░░░░░░░░░   ░░░░░░   ░░░░░░░░░   ░░░░░
  ░░░░░░       ░░░░░    ░░░░░       ░░░
  ░░░░░░░░     ░░░░░░░  ░░░░░░░░    ░░░░
  ░░░░░░░░░    ░░░░     ░░░░░░      ░░░░░░
```

- Gray placeholder rows that mirror table column widths
- 2-frame pulse animation: `text-ghost` ↔ `text-muted` (subtle pulse)
- Used during initial data load, replaced by real content when data arrives
- Show skeleton for max 5s, then "Taking longer than expected..." message, then at 10s show retry option

---

## View Designs (10 screens)

Design each at 120×30 grid with realistic data.

### 1. Dashboard

The nerve center. Every character earns its place.

```
 bc ◆ myproject    Dashboard  Agents  Channels  Costs  Logs  Roles  Memory  Worktrees  Help
─────────────────────────────────────────────────────────────────────────────────────────────────

  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
  │  12              │  │  5              │  │  3              │  │  1              │
  │  Total Agents    │  │  ◆ Working      │  │  ○ Idle         │  │  ▲ Stuck        │
  │  ▁▂▃▃▅▅▇▇██     │  │  ▁▃▅▇█▇▅▃▁▃    │  │                 │  │                 │
  └─────────────────┘  └─────────────────┘  └─────────────────┘  └─────────────────┘

  Activity ─────────────────────────────────────────────────  Health ────────────────
  12:34:56  ● eng-01   ◆ working   Implementing JWT refresh  ● 92% healthy
  12:34:52  ▲ mgr-01   ✓ done      Code review complete       ◆ 5 working · ○ 3 idle
  12:34:48  ● eng-02   ▲ stuck     Waiting for dependency     ▲ 1 stuck
  12:34:45  ■ qa-01    ◆ working   Running test suite
  12:34:40  ◇ tl-01    ✓ done      Architecture doc merged   Cost ───────────────────
  12:34:35  ● eng-03   ○ idle      Waiting for assignment     $47.23 / $500
  12:34:30  ◆ root     ◆ working   Spawning eng-06            ▓▓▓▓▓▓░░░░░░░░░  9.4%
  12:34:25  ● eng-04   ✓ done      Tests passing              $3.21/hr burn
  12:34:20  ● eng-05   ◆ working   Refactoring auth module
  12:34:15  ● eng-06   ◆ working   Setting up worktree       Roles ──────────────────
  12:34:10  ▲ mgr-02   ◆ working   Sprint planning            ● engineer  8
  12:34:05  ■ qa-02    ○ idle      Awaiting test plan         ▲ manager   2
                                                               ■ qa        2
  ⠧ live · r refresh                                           ◇ tech-lead 1

─────────────────────────────────────────────────────────────────────────────────────────────────
 [:] command  [/] filter  [?] help                                          5 working · $47.23
```

Layout:

- Row 1: Top Bar
- Row 2: Separator
- Rows 3-7: MetricCards (4 across, 5 rows each including border)
- Rows 8-22: Two-column — Activity Feed (65%) | Stats Panels (35%)
- Row 23: Separator
- Row 24: Status Bar

**80×24 compact:**

```
 bc ◆ myproject  1·Dash 2·Agt 3·Ch 4·Cost 5·Log 6·Rol 7·Mem 8·Wt 9·Help
──────────────────────────────────────────────────────────────────────────
 12 agents · ◆ 5 working · ○ 3 idle · ▲ 1 stuck · ✗ 0 error

 12:34:56  ● eng-01   ◆ working   Implementing JWT refresh
 12:34:52  ▲ mgr-01   ✓ done      Code review complete
 12:34:48  ● eng-02   ▲ stuck     Waiting for dependency
 12:34:45  ■ qa-01    ◆ working   Running test suite
 12:34:40  ◇ tl-01    ✓ done      Architecture doc merged
 12:34:35  ● eng-03   ○ idle      Waiting for assignment
 12:34:30  ◆ root     ◆ working   Spawning eng-06
 12:34:25  ● eng-04   ✓ done      Tests passing
 12:34:20  ● eng-05   ◆ working   Refactoring auth module

 ● 92% healthy · $47.23/$500 (9.4%) · $3.21/hr
 ⠧ live · r refresh

──────────────────────────────────────────────────────────────────────────
 [:] command  [/] filter  [?] help                        5 working
```

No wasted space. Single-column. Inline metrics. Activity fills the screen.

### 2. Agents View

```
 > Agents (21)                                                    j/k nav · Enter attach · p peek

 STATUS    NAME          ROLE         STATE         COST       UPTIME
 ─────────────────────────────────────────────────────────────────────────
 ▸ ◆      eng-01        ● engineer   ◆ working     $12.50     2h 15m
   ◆      eng-02        ● engineer   ◆ working     $8.30      1h 45m
   ○      eng-03        ● engineer   ○ idle         $3.20      3h 10m
   ✓      eng-04        ● engineer   ✓ done         $6.80      0h 55m
   ▲      eng-05        ● engineer   ▲ stuck        $4.10      1h 20m
 ─── managers ───────────────────────────────────────────────────────────
   ○      mgr-01        ▲ manager    ○ idle         $6.12      4h 00m
   ◆      mgr-02        ▲ manager    ◆ working      $2.30      0h 30m
 ─── tech-leads ─────────────────────────────────────────────────────────
   ✓      tl-01         ◇ tech-lead  ✓ done         $3.91      2h 10m
 ─── qa ─────────────────────────────────────────────────────────────────
   ◆      qa-01         ■ qa         ◆ working      $2.80      1h 00m
```

- Full-width table filling all available rows
- Idle/stopped agents are in `text-muted` (dimmed entire row)
- Group separators in `text-ghost`, lowercase
- Selected row: `▸` + blue text, shifts entire row
- View-specific hints in the header line (right-aligned), not in footer
- Toggle `v` for flat view (no groups), `g` for grouped

**Agent Detail (on Enter):**

```
 > Agents › eng-01                                                               Esc back

 ● eng-01 · engineer · ◆ working                        $12.50 · 2h 15m · 34 msgs
 ─────────────────────────────────────────────────────────────────────────────────────

 Output ──────────────────────────────────────────────────────────────── (following)
  $ git diff --stat
    src/auth/login.go | 45 ++++++++++++
    src/auth/token.go | 12 +++

  ✓ All tests passing (34/34)

  ▲ Warning: unused variable 'tmp' at token.go:42

  Working on: implementing JWT refresh token logic...
  Reading token.go to understand current refresh flow...

  $ go test ./src/auth/...
  ok   src/auth  0.234s
```

- Metadata in a single dense line below breadcrumb
- Output is syntax-colored: `$` commands in `blue`, errors in `red`, warnings in `amber`, success `✓` in `green`, standard output in `text-normal`
- `(following)` indicator in `text-muted` when auto-scrolling is on
- Content fills **all remaining rows** — no wasted space

### 3. Channels View

**List:**

```
 > Channels (12)                                           j/k nav · Enter open · m compose

 CHANNEL               UNREAD      MEMBERS   DESCRIPTION
 ────────────────────────────────────────────────────────────────────────────
 ▸ #engineering         ● 3 new     6         Main engineering discussion
   #general                         12        All agents
   #code-review         ● 1 new     4         PR reviews and feedback
   #alerts                          2         System alerts only
   #standup                         8         Daily status updates
   #product             ● 14 new    9         Product roadmap, priorities
```

- Unread: `● N new` in `amber` (not just a number — a pill)
- Channels with no unread: dash or blank in UNREAD column
- Stale/empty channels in `text-muted`

**Chat (on Enter)** — uses the flat Slack-style ChatMessage layout described above.

### 4. Costs View

```
 > Costs                                                          j/k nav · s sort · Enter detail

 Total $47.23 / $500.00 budget                    Burn $3.21/hr · Projected $89.50 · Cache 34%
 ▓▓▓▓▓▓░░░░░░░░░░░░░░░░░░░░░░░░  9.4%
 ─────────────────────────────────────────────────────────────────────────────────────

 AGENT         PROVIDER    COST       %         BAR
 ─────────────────────────────────────────────────────────────────────────────────────
 ▸ eng-01      claude      $12.50     26%       ▓▓▓▓▓▓▓▓░░░░░░░
   eng-02      claude      $8.30      18%       ▓▓▓▓▓░░░░░░░░░░
   mgr-01      claude      $6.12      13%       ▓▓▓▓░░░░░░░░░░░
   eng-04      claude      $6.80      14%       ▓▓▓▓▓░░░░░░░░░░
   eng-05      claude      $4.10       9%       ▓▓▓░░░░░░░░░░░░
   tl-01       claude      $3.91       8%       ▓▓░░░░░░░░░░░░░
   eng-03      gemini      $3.20       7%       ▓▓░░░░░░░░░░░░░
   qa-01       claude      $2.80       6%       ▓▓░░░░░░░░░░░░░
```

- Summary line with total, burn rate, projected, cache hit all on one dense line
- Wide progress bar below summary
- Agent table sorted by cost (default), with inline progress bars
- Bars use `blue` <75%, `amber` 75-90%, `red` >90% of per-agent budget

### 5. Logs View

```
 > Logs                                            [s] all · [a] all agents · [t] all time · [/] search

 TIME        AGENT        TYPE              MESSAGE
 ─────────────────────────────────────────────────────────────────────────────────────
 12:35:03    ● eng-01     agent.working     Implementing JWT refresh
 12:35:01    · system     checkpoint        Auto-save triggered
 12:34:57    ▲ mgr-01     agent.error       Timeout waiting for response
 12:34:56    ● eng-02     agent.working     (×3) Running test suite
 12:34:52    ◇ tl-01      agent.done        Architecture doc merged
 12:34:48    ● eng-05     agent.stuck       No progress for 5 min
 12:34:45    ■ qa-01      agent.working     Fuzzing auth endpoints
 12:34:40    ◆ root       system.spawn      Started eng-06
 12:34:35    ● eng-03     agent.idle        Waiting for assignment
```

- Error rows: entire line in `red` (not just the symbol)
- Warning/stuck rows: status + type in `amber`
- Collapsed repeated events: `(×3)` count in `text-muted`
- Filter controls shown inline in header (active filters highlighted in `blue`)

### 6. Memory View

```
 > Memory › eng-01                                                               Esc back

 [Learnings]  [Experiences]  [Prompt]                          12 learnings · 8 experiences
 ─────────────────────────────────────────────────────────────────────────────────────

  CATEGORY         LEARNING
 ─────────────────────────────────────────────────────────────────────────────────────
  ▸ patterns       Always use context.WithTimeout for DB calls
    patterns       Prefer table-driven tests over individual test cases
    anti-patterns  Don't use sleep in retry loops — use backoff
    tips           Use t.TempDir() for test isolation, auto-cleaned
    best-practice  Propagate context through all call chains
    debugging      Check tmux session logs before restarting agents
    architecture   Keep pkg/ packages self-contained, no cross-imports
```

- Sub-tab bar: active tab `text-bright` + underline, inactive `text-muted`
- Category tags in `violet` (the AI/memory accent color)
- Content fills available rows

### 7. Roles View

```
 > Roles (8)                                             j/k nav · Enter details · /search

 NAME              GLYPH   AGENTS   CAPABILITIES
 ─────────────────────────────────────────────────────────────────────────────────────
 ▸ root            ◆       1        monitor_health, system_ops, all
   engineer        ●       8        implement_tasks, write_tests, create_prs
   manager         ▲       2        assign_work, review, coordinate
   tech-lead       ◇       1        review, architect, implement
   qa              ■       1        test, verify, report_bugs
   ux              ○       1        design, prototype, user_research
   docs            ·       1        write_docs, create_prs
   marketing       ·       1        implement_tasks, create_prs
```

### 8. Worktrees View

```
 > Worktrees (11)  13 orphaned                              j/k nav · Enter details · p prune

 AGENT         STATUS      BRANCH                    PATH
 ─────────────────────────────────────────────────────────────────────────────────────
 ▸ eng-01      ✓ OK        feat/jwt-refresh          .bc/worktrees/eng-01
   eng-02      ✓ OK        feat/auth-tests           .bc/worktrees/eng-02
   eng-03      ▲ MISSING   fix/login-bug             .bc/worktrees/eng-03
   mgr-01      ✓ OK        review/sprint-3           .bc/worktrees/mgr-01
   tl-01       ✓ OK        feat/architecture         .bc/worktrees/tl-01
 ─── orphaned ──────────────────────────────────────────────────────────────────────
   stress-1    · ORPHANED  -                         .bc/worktrees/stress-1
   stress-2    · ORPHANED  -                         .bc/worktrees/stress-2
```

- `MISSING` in `amber`, `ORPHANED` in `text-ghost` (dimmed)
- Orphaned count in header as a quiet badge

### 9. Tools View

```
 > Tools (4)                                                              r refresh

 PROVIDER       VERSION    STATUS       AGENTS USING
 ─────────────────────────────────────────────────────────────────────────────────────
 ▸ claude       3.2.1      ✓ ready      8
   gemini       1.5.0      ✓ ready      2
   cursor       0.42       ✓ ready      1
   codex        0.42.0     ✗ error      0
```

### 10. Help View

Two-column keyboard reference card:

```
 > Help

 GLOBAL                                   NAVIGATION
 ──────────────────────────────           ──────────────────────────────
 Tab / Shift+Tab   Cycle views            j / ↓          Move down
 1-9               Jump to view           k / ↑          Move up
 :                 Command palette        g              Top
 /                 Filter                 G              Bottom
 ?                 Toggle help            Enter          Select / Open
 q / Esc           Quit / Back            Esc            Go back

 AGENTS                                   CHANNELS
 ──────────────────────────────           ──────────────────────────────
 Enter              Attach tmux           Enter          Open channel
 p                  Peek output           m              Compose message
 d                  Detail view           j/k            Scroll messages
 x                  Stop agent
 X                  Kill (force)
 R                  Restart

 COSTS                                    MEMORY
 ──────────────────────────────           ──────────────────────────────
 s                  Cycle sort            1/2            Switch tabs
 Enter              Agent detail          Enter          View details
 m                  Toggle models         /              Search memories
```

- Two columns side by side, each with category headers
- Clean, scannable, no borders or boxes — just alignment

---

## Interaction States to Design

For each major view, create frames for:

| State | What to show |
|-------|-------------|
| **Populated** | Full realistic data (the main design) |
| **Empty** | Centered EmptyState component with helpful guidance |
| **Loading** | Skeleton rows (3-5 gray pulsing placeholders) + spinner `⠧` in header |
| **Loading (slow, 5s)** | Same skeleton + "Taking longer than expected..." below spinner |
| **Error** | Red-tinted panel with `✗` symbol, error message, `[r] retry` |
| **Selected** | One row highlighted in `blue` with `▸` |
| **Filtered** | FilterBar visible at top, non-matching rows hidden, matches highlighted in `violet` |
| **Overlay** | CommandBar floating over dimmed content |

---

## Animation Frames

Design as horizontal strips:

1. **Spinner** (10 frames, blue): `⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏`
2. **Skeleton pulse** (2 frames): `text-ghost` ↔ slightly brighter
3. **Live indicator** (2 frames): `⠧ live` with dot pulsing `blue` ↔ `text-muted`

---

## Design Principles

1. **Fill the screen** — The #1 problem with the current UI is wasted space. Every view should use all available rows. Tables should extend to fill. Activity feeds should show as many entries as will fit. No blank deserts.

2. **Depth through background tiers, not borders** — Use the 4 background levels (`void` → `base` → `surface` → `overlay`) to create hierarchy. Minimize box borders. When borders are needed, keep them barely visible (`border-ghost`). Only focus borders should pop (`border-focus`).

3. **Single navigation bar** — One top bar, one bottom status bar. No duplicated hints. View-specific shortcuts go inline with the view header, where they're actually relevant.

4. **Color = meaning, always** — Blue is interactive/working. Green is good/done. Amber is warning/attention. Red is error/destructive. Violet is AI/memory. Gray is structural/inactive. Never use color decoratively.

5. **Status pills, not just symbols** — Combine symbol + color + muted background to create scannable badge-like indicators. A `◆ working` on a blue-muted background is far more visible than just a colored dot.

6. **Flat chat, not bubbles** — Slack-style flat messages with sender + timestamp header, indented body below. No borders around messages (they break in terminals). Use spacing for visual separation.

7. **Dense but breathable** — 1-char padding inside panels, 1-row margin between sections. At 80 cols, sacrifice everything except readability. At 160 cols, add breathing room but never empty space.

8. **Consistent geometric glyphs** — `◆●▲◇■○·` for roles. `◆✓✗▲○–` for status. No emoji (they render inconsistently across terminals, cause alignment issues, and look unprofessional). Geometric Unicode glyphs are monospace-safe and feel like a professional instrument panel.

9. **The cockpit aesthetic** — This should feel like a Bloomberg terminal, air traffic control screen, or submarine sonar display. Dense, dark, precise, glowing. Every element is purposeful. The beauty comes from the *system* — consistent spacing, careful color, deliberate hierarchy — not from decoration.

---

## Deliverables

1. **Design System page** — Color tokens, text levels, status system, role glyphs, border levels. A reference card a developer can pin up.
2. **Component Library** — All components (MetricCard, DataTable, ChatMessage, Panel, ProgressBar, CommandBar, FilterBar, ConfirmDialog, EmptyState, Skeleton, StatusBadge, Breadcrumb, Spinner) with all states.
3. **10 View screens at 120×30** — Dashboard, Agents (list + detail), Channels (list + chat), Costs, Logs, Memory, Roles, Worktrees, Help.
4. **3 Responsive sets** — Dashboard, Agents, Channels at 80×24 / 120×30 / 160×40.
5. **Overlay compositions** — CommandBar over Dashboard, ConfirmDialog over Agents, FilterBar in Logs.
6. **Empty + Error states** — Each view with no data, and one error example.
7. **Animation frames** — Spinner strip, skeleton pulse, live indicator.
8. **Before/After comparison** — Current UI screenshot vs new design for Dashboard and Channels (to show the improvement).
