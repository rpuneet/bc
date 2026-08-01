/**
 * Insights — spend, attribution, and activity for the whole fleet.
 *
 * The page answers five questions, in order, and nothing else:
 *
 *   1. What is the fleet costing, and is it trending?   → stat band + daily bars
 *   2. Where is it going?                               → one breakdown, dim switch
 *   3. Why is the bill sane?                            → cache efficiency module
 *   4. What are the agents actually doing?              → recent activity chart
 *   5. What is it doing to the machine?                 → live system row
 *
 * Every number is period-scoped off the cost ledger (computed directly
 * from provider transcripts) via `?since=` — no mixed lifetime/range
 * figures on one surface. Ledger days are UTC. /stats, /metrics and
 * /costs redirect here (see App.tsx).
 *
 * Depth is progressive disclosure, never upfront: the Tokens stat, every
 * breakdown row and every system tile expand into an inline drill-down
 * (see views/insights/*). Expanded state lives in the URL hash so a
 * refresh restores it; drill-downs lazy-fetch on open.
 */

import { useCallback, useMemo, useState } from "react";
import {
  BarChart, Bar, Cell, XAxis, YAxis, CartesianGrid,
  Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../api/client";
import type {
  Agent, CostSummary, AgentCostSummary, ModelCostSummary, DailyCost, AgentActivityItem,
} from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { SectionRule } from "../components/shared/SectionRule";
import { fmtTokens } from "../components/shared/stats-primitives";
import { AgentChip } from "../components/agent-ui";
import { formatCost } from "../utils/format";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import {
  ACCENT, TICK, AX, TT_STYLE, fmtClock, fmtShortDate, stripAgentPrefix,
} from "./insights/chrome";
import type { ChartTooltipProps } from "./insights/chrome";
import { Disclosure, Chevron, useHashPanel } from "./insights/disclosure";
import { SystemRow } from "./insights/SystemRow";
import { TokenPanel, buildTokenSeries } from "./insights/TokenPanel";
import { AgentDetail, ModelDetail, RepoDetail } from "./insights/BreakdownDetail";
import { BudgetPanel } from "./insights/BudgetPanel";

// ── Periods ─────────────────────────────────────────────────────────────────
//
// The ledger is daily, so the shortest honest window is a week. Sub-day
// ranges (the old 1h/6h picker) made "spend this range" show a full
// day's ledger and read as nonsense.

const PERIODS = [
  { key: "7d", label: "7d", days: 7 },
  { key: "30d", label: "30d", days: 30 },
  { key: "90d", label: "90d", days: 90 },
  { key: "all", label: "All", days: 365 },
] as const;
type PeriodKey = (typeof PERIODS)[number]["key"];

const DAY_MS = 86_400_000;

/** UTC day key (YYYY-MM-DD) — matches the ledger's day bucketing. */
function dayKey(ms: number): string {
  return new Date(ms).toISOString().slice(0, 10);
}

/** Ascending list of the last n UTC day keys, ending today. */
function lastNDays(n: number): string[] {
  const now = Date.now();
  const out: string[] = [];
  for (let i = n - 1; i >= 0; i--) out.push(dayKey(now - i * DAY_MS));
  return out;
}

// ── Stat band ───────────────────────────────────────────────────────────────

function StatCell({
  label,
  value,
  sub,
  onClick,
  open,
}: {
  label: string;
  value: React.ReactNode;
  sub?: React.ReactNode;
  /** Present = the cell is a drill-down trigger (chevron affordance). */
  onClick?: () => void;
  open?: boolean;
}) {
  const body = (
    <>
      <div className="flex items-center gap-1.5">
        <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em] truncate">{label}</span>
        {onClick && <Chevron open={open ?? false} />}
      </div>
      <div className="mt-1 text-xl font-semibold tabular-nums text-mycel-text truncate">{value}</div>
      <div className="mt-0.5 text-[11px] text-mycel-muted truncate">{sub ?? " "}</div>
    </>
  );
  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        aria-expanded={open}
        title={`${open ? "Collapse" : "Expand"} ${label} detail`}
        className={`bg-mycel-surface p-4 min-w-0 text-left cursor-pointer transition-colors hover:bg-mycel-surface-hover ${open ? "bg-mycel-surface-hover" : ""}`}
      >
        {body}
      </button>
    );
  }
  return <div className="bg-mycel-surface p-4 min-w-0">{body}</div>;
}

/** "▲ 23% vs prior 30d" — quiet, never colored. No prior data → nothing.
 *  A many-fold change reads as a bug ("▲ 2400%"), so past 2× the label
 *  falls back to the absolute prior figure, which stays believable. */
function deltaLabel(current: number, previous: number, periodLabel: string): string | null {
  if (previous <= 0) return null;
  const pct = ((current - previous) / previous) * 100;
  if (Math.abs(pct) > 200) return `vs ${formatCost(previous)} prior ${periodLabel}`;
  const arrow = pct >= 0 ? "▲" : "▼";
  return `${arrow} ${Math.abs(pct).toFixed(0)}% vs prior ${periodLabel}`;
}

// ── Spend chart tooltip ─────────────────────────────────────────────────────

interface SpendPoint {
  date: string;
  cost: number;
  tokens: number;
  records: number;
}

function SpendTooltip({ active, payload }: ChartTooltipProps) {
  const p = payload?.[0]?.payload as SpendPoint | undefined;
  if (!active || !p) return null;
  return (
    <div style={TT_STYLE} className="px-3 py-2">
      <div className="text-[11px] text-mycel-muted">{fmtShortDate(p.date)}</div>
      <div className="font-semibold tabular-nums">{formatCost(p.cost)}</div>
      {p.records > 0 && (
        <div className="text-[11px] text-mycel-muted tabular-nums">
          {fmtTokens(p.tokens)} tokens · {p.records.toLocaleString()} calls
        </div>
      )}
    </div>
  );
}

// ── Breakdown (where it goes) ───────────────────────────────────────────────

type Dimension = "agent" | "model" | "repo";

interface BreakdownRow {
  name: string;
  cost: number;
  /** Stable drill-down id (`<dimension>:<entity>`); absent = not expandable. */
  id?: string;
  /** Live agent state — renders the row's living character chip. */
  agentState?: string;
  muted?: boolean;
}

/** Top rows by cost with the tail folded into one muted "everything else". */
function topRows(rows: BreakdownRow[], max = 8): BreakdownRow[] {
  const sorted = rows.filter((r) => r.cost > 0).sort((a, b) => b.cost - a.cost);
  if (sorted.length <= max) return sorted;
  const head: BreakdownRow[] = sorted.slice(0, max);
  const rest = sorted.slice(max).reduce((s, r) => s + r.cost, 0);
  head.push({ name: `everything else (${sorted.length - max})`, cost: rest, muted: true });
  return head;
}

function Breakdown({
  rows,
  total,
  expandedId,
  onToggle,
  renderDetail,
}: {
  rows: BreakdownRow[];
  total: number;
  /** Currently expanded row id (from the URL hash), or null. */
  expandedId: string | null;
  onToggle: (id: string) => void;
  renderDetail: (id: string) => React.ReactNode;
}) {
  const max = rows.reduce((m, r) => Math.max(m, r.cost), 0);
  if (rows.length === 0) {
    return <div className="py-10 text-center text-sm text-mycel-muted">No spend in this period</div>;
  }
  return (
    <div className="space-y-1">
      {rows.map((r) => {
        const share = total > 0 ? (r.cost / total) * 100 : 0;
        const width = max > 0 ? Math.max((r.cost / max) * 100, 0.75) : 0;
        const open = r.id !== undefined && r.id === expandedId;
        const inner = (
          <>
            <span className={`flex items-center gap-1.5 min-w-0 text-xs ${r.muted ? "text-mycel-muted italic" : "text-mycel-text"}`}>
              {r.id !== undefined && <Chevron open={open} />}
              {r.agentState !== undefined ? (
                <AgentChip name={r.name} state={r.agentState} size={16} showDot={false} className="min-w-0" preview />
              ) : (
                <span className="truncate">{r.name}</span>
              )}
            </span>
            <span className="relative h-2 rounded-full bg-mycel-border/40 overflow-hidden">
              <span
                className="absolute inset-y-0 left-0 rounded-full"
                style={{ width: `${width}%`, backgroundColor: r.muted ? "var(--mycel-muted)" : ACCENT, opacity: r.muted ? 0.45 : 0.85 }}
              />
            </span>
            <span className="text-xs tabular-nums text-mycel-text text-right">{formatCost(r.cost)}</span>
            <span className="text-[11px] tabular-nums text-mycel-muted text-right">
              {share >= 0.05 ? `${share.toFixed(share < 10 ? 1 : 0)}%` : "<0.1%"}
            </span>
          </>
        );
        const cls = "grid w-full items-center gap-3 px-1 py-1.5 rounded-md grid-cols-[minmax(7rem,12rem)_1fr_5rem_2.9rem]";
        const id = r.id;
        if (id === undefined) {
          return <div key={r.name} className={cls}>{inner}</div>;
        }
        return (
          <div key={id}>
            <button
              type="button"
              onClick={() => onToggle(id)}
              aria-expanded={open}
              aria-label={`${open ? "Collapse" : "Expand"} ${r.name}`}
              className={`${cls} text-left hover:bg-mycel-surface-hover transition-colors cursor-pointer ${open ? "bg-mycel-surface-hover" : ""}`}
              title={`${open ? "Collapse" : "Expand"} ${r.name}`}
            >
              {inner}
            </button>
            <Disclosure open={open} onClose={() => onToggle(id)} label={`${r.name} detail`}>
              <div className="pt-1">{renderDetail(id)}</div>
            </Disclosure>
          </div>
        );
      })}
    </div>
  );
}

// ── Activity ────────────────────────────────────────────────────────────────
//
// The activity feed is chatty (~1000 hook events can span under two
// hours during busy work), so the chart shows exactly the window the
// fetched events cover and says so — no pretending it's a fixed range.

const ACTIVITY_SERIES = [
  { key: "tools", label: "Tool calls", color: ACCENT },
  { key: "prompts", label: "Prompts & replies", color: "var(--mycel-chart-1)" },
  { key: "other", label: "Lifecycle", color: "var(--mycel-chart-4)" },
] as const;

const TOOL_EVENTS = new Set(["PreToolUse", "PostToolUse", "PostToolUseFailure"]);
const PROMPT_EVENTS = new Set(["UserPromptSubmit", "Stop", "Notification"]);

interface ActivityBucket {
  t: number;
  tools: number;
  prompts: number;
  other: number;
}

export interface ActivitySummary {
  buckets: ActivityBucket[];
  bucketMinutes: number;
  windowLabel: string;
  eventCount: number;
  agentCount: number;
}

/** Bucket recent events into ~30 even time slots (snapped to 1–60 min). */
export function summarizeActivity(items: AgentActivityItem[]): ActivitySummary {
  const times = items
    .map((i) => new Date(i.timestamp).getTime())
    .filter((t) => Number.isFinite(t));
  if (times.length === 0) {
    return { buckets: [], bucketMinutes: 0, windowLabel: "", eventCount: 0, agentCount: 0 };
  }
  const newest = Math.max(...times);
  const oldest = Math.min(...times);
  const spanMin = Math.max((newest - oldest) / 60_000, 1);
  const sizes = [1, 2, 5, 10, 15, 30, 60];
  const bucketMinutes = sizes.find((s) => spanMin / s <= 32) ?? 60;
  const bucketMs = bucketMinutes * 60_000;
  const start = Math.floor(oldest / bucketMs) * bucketMs;

  const map = new Map<number, ActivityBucket>();
  const agents = new Set<string>();
  for (const item of items) {
    const t = new Date(item.timestamp).getTime();
    if (!Number.isFinite(t)) continue;
    if (item.agent) agents.add(item.agent);
    const b = Math.floor((t - start) / bucketMs);
    const key = start + b * bucketMs;
    const bucket = map.get(key) ?? { t: key, tools: 0, prompts: 0, other: 0 };
    if (TOOL_EVENTS.has(item.event)) bucket.tools++;
    else if (PROMPT_EVENTS.has(item.event)) bucket.prompts++;
    else bucket.other++;
    map.set(key, bucket);
  }
  // Fill empty buckets so quiet minutes render as honest gaps, not
  // interpolation.
  const buckets: ActivityBucket[] = [];
  for (let t = start; t <= newest; t += bucketMs) {
    buckets.push(map.get(t) ?? { t, tools: 0, prompts: 0, other: 0 });
  }

  const spanLabel = spanMin >= 90 ? `${(spanMin / 60).toFixed(1)}h` : `${Math.round(spanMin)}m`;
  return {
    buckets,
    bucketMinutes,
    windowLabel: `last ${spanLabel}`,
    eventCount: times.length,
    agentCount: agents.size,
  };
}

function ActivityTooltip({ active, payload, label }: ChartTooltipProps) {
  if (!active || !payload?.length) return null;
  const total = payload.reduce((s, p) => s + (Number(p.value) || 0), 0);
  return (
    <div style={TT_STYLE} className="px-3 py-2">
      <div className="text-[11px] text-mycel-muted">{fmtClock(Number(label))}</div>
      <div className="font-semibold tabular-nums">{total.toLocaleString()} events</div>
      {ACTIVITY_SERIES.map((s) => {
        const p = payload.find((x) => x.dataKey === s.key);
        const v = Number(p?.value) || 0;
        if (v === 0) return null;
        return (
          <div key={s.key} className="flex items-center gap-1.5 text-[11px] text-mycel-muted tabular-nums">
            <span className="w-2 h-2 rounded-sm" style={{ backgroundColor: s.color }} />
            {s.label}: {v.toLocaleString()}
          </div>
        );
      })}
    </div>
  );
}

// ── Data ────────────────────────────────────────────────────────────────────

interface InsightsData {
  daily: DailyCost[];
  summary: CostSummary | null;
  byAgent: AgentCostSummary[];
  byModel: ModelCostSummary[];
  byRepo: { key: string; label: string; total: number }[];
  activity: AgentActivityItem[];
  agents: Agent[];
}

// ── Main ────────────────────────────────────────────────────────────────────

export function Insights() {
  const [period, setPeriod] = useState<PeriodKey>("30d");
  const [dimension, setDimension] = useState<Dimension>("agent");
  // Drill-down state lives in the URL hash so refresh restores it.
  const [tokensOpen, setTokensOpen] = useHashPanel("tokens");
  const [expandedRow, setExpandedRow] = useHashPanel("row");

  const cfg = PERIODS.find((p) => p.key === period) ?? PERIODS[1];
  const isAll = period === "all";
  // Window = last N UTC days including today; `since` is the first day.
  const since = useMemo(
    () => (isAll ? undefined : dayKey(Date.now() - (cfg.days - 1) * DAY_MS)),
    [isAll, cfg.days],
  );

  useHeaderSlot({
    actions: (
      <div className="flex gap-1">
        {PERIODS.map((p) => (
          <button
            key={p.key}
            type="button"
            onClick={() => setPeriod(p.key)}
            className={`px-2 py-0.5 text-[11px] rounded-md border transition-colors ${
              p.key === period
                ? "border-mycel-accent text-mycel-accent"
                : "border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-muted"
            }`}
          >
            {p.label}
          </button>
        ))}
      </div>
    ),
  });

  const fetcher = useCallback(async (): Promise<InsightsData> => {
    // Daily ledger fetches double the window so the stat band can show
    // an honest delta vs the previous period.
    const dailyDays = isAll ? 365 : Math.min(cfg.days * 2, 365);
    const [daily, summary, byAgent, byModel, repos, activity, agents] = await Promise.allSettled([
      api.getCostDaily(dailyDays),
      api.getCostSummary({ since }),
      api.getCostByAgent({ since, limit: 200 }),
      api.getCostByModel({ since }),
      api.globalCosts({ start: since ?? "2000-01-01" }),
      api.getActivity(1000),
      api.listAgents(),
    ]);
    return {
      daily: daily.status === "fulfilled" && Array.isArray(daily.value) ? daily.value : [],
      summary: summary.status === "fulfilled" ? summary.value : null,
      byAgent: byAgent.status === "fulfilled" ? (byAgent.value ?? []) : [],
      byModel: byModel.status === "fulfilled" ? (byModel.value ?? []) : [],
      byRepo: repos.status === "fulfilled" ? (repos.value.rows ?? []).map((r) => ({ key: r.key, label: r.label, total: r.total })) : [],
      activity: activity.status === "fulfilled" ? (activity.value ?? []) : [],
      agents: agents.status === "fulfilled" && Array.isArray(agents.value) ? agents.value : [],
    };
  }, [since, isAll, cfg.days]);

  const { data, loading, error, refresh, timedOut } = usePolling(fetcher, 30000);

  // ── Derived: spend series + stat band ─────────────────────────────────────

  const byDay = useMemo(() => {
    const m = new Map<string, DailyCost>();
    for (const d of data?.daily ?? []) m.set(d.date, d);
    return m;
  }, [data?.daily]);

  const spendSeries: SpendPoint[] = useMemo(() => {
    // "All" shows every ledger day from the first recorded one; periods
    // show a continuous window with zero-filled quiet days.
    const days = isAll
      ? (() => {
          const first = data?.daily?.[0]?.date;
          if (!first) return [];
          const start = new Date(`${first}T00:00:00Z`).getTime();
          const n = Math.floor((Date.now() - start) / DAY_MS) + 1;
          return lastNDays(Math.max(n, 1));
        })()
      : lastNDays(cfg.days);
    return days.map((d) => {
      const row = byDay.get(d);
      return {
        date: d,
        cost: row?.cost_usd ?? 0,
        tokens: row?.total_tokens ?? 0,
        records: row?.record_count ?? 0,
      };
    });
  }, [byDay, cfg.days, isAll, data?.daily]);

  const stat = useMemo(() => {
    const windowDays = lastNDays(cfg.days);
    const winSet = new Set(windowDays);
    const spend = isAll
      ? (data?.daily ?? []).reduce((s, d) => s + d.cost_usd, 0)
      : windowDays.reduce((s, d) => s + (byDay.get(d)?.cost_usd ?? 0), 0);
    // Previous window (for the delta) is the N days immediately before
    // this one — explicit day keys, not "everything else in the fetch".
    let prevSpend = 0;
    if (!isAll) {
      const now = Date.now();
      for (let i = cfg.days; i < cfg.days * 2; i++) {
        const d = byDay.get(dayKey(now - i * DAY_MS));
        if (d && !winSet.has(d.date)) prevSpend += d.cost_usd;
      }
    }
    const today = byDay.get(dayKey(Date.now()))?.cost_usd ?? 0;
    const trailing7 = lastNDays(7).reduce((s, d) => s + (byDay.get(d)?.cost_usd ?? 0), 0);
    return { spend, prevSpend, today, avg7: trailing7 / 7 };
  }, [byDay, cfg.days, isAll, data?.daily]);

  const cache = useMemo(() => {
    const s = data?.summary;
    const read = s?.cache_read_tokens ?? 0;
    const write = s?.cache_write_tokens ?? 0;
    const input = s?.input_tokens ?? 0;
    const output = s?.output_tokens ?? 0;
    const ratio = read + input > 0 ? read / (read + input) : 0;
    return { read, write, input, output, ratio, hasData: read + write + input > 0 };
  }, [data?.summary]);

  // ── Derived: breakdown ────────────────────────────────────────────────────

  // Live agent states keyed by bare name — powers the living character
  // chips on agent rows (the ledger also bills sessions that aren't
  // fleet agents, e.g. observer-sessions; those stay plain text).
  const agentStates = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of data?.agents ?? []) m.set(a.name, a.state);
    return m;
  }, [data?.agents]);

  const breakdown = useMemo(() => {
    let rows: BreakdownRow[];
    if (dimension === "agent") {
      rows = topRows(
        (data?.byAgent ?? []).map((a) => {
          const name = stripAgentPrefix(a.agent_id);
          return {
            name,
            cost: a.total_cost_usd,
            id: `agent:${a.agent_id}`,
            agentState: agentStates.get(name),
          };
        }),
      );
    } else if (dimension === "model") {
      rows = topRows((data?.byModel ?? []).map((m) => ({ name: m.model, cost: m.total_cost_usd, id: `model:${m.model}` })));
    } else {
      // Repos can appear under several historical paths with one label —
      // fold by label so the list reads as projects, not paths.
      const byLabel = new Map<string, number>();
      for (const r of data?.byRepo ?? []) {
        byLabel.set(r.label, (byLabel.get(r.label) ?? 0) + r.total);
      }
      rows = topRows([...byLabel.entries()].map(([name, total]) => ({ name, cost: total, id: `repo:${name}` })));
    }
    const total = rows.reduce((s, r) => s + r.cost, 0);
    return { rows, total };
  }, [data?.byAgent, data?.byModel, data?.byRepo, dimension, agentStates]);

  const activity = useMemo(() => summarizeActivity(data?.activity ?? []), [data?.activity]);

  // Recent-feed event counts per bare agent name, for the agent detail.
  const activityCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const item of data?.activity ?? []) {
      if (item.agent) m.set(item.agent, (m.get(item.agent) ?? 0) + 1);
    }
    return m;
  }, [data?.activity]);

  // Token drill-down series shares the spend chart's day window.
  const tokenSeries = useMemo(
    () => buildTokenSeries(data?.daily ?? [], spendSeries.map((p) => p.date)),
    [data?.daily, spendSeries],
  );

  // One breakdown drill-down at a time, keyed `<dimension>:<entity>`.
  const renderBreakdownDetail = useCallback(
    (id: string): React.ReactNode => {
      const sep = id.indexOf(":");
      const kind = id.slice(0, sep);
      const entity = id.slice(sep + 1);
      if (kind === "agent") {
        const name = stripAgentPrefix(entity);
        return (
          <AgentDetail
            agentId={entity}
            name={name}
            summary={(data?.byAgent ?? []).find((a) => a.agent_id === entity)}
            fleetDaily={data?.daily ?? []}
            days={spendSeries.map((p) => p.date)}
            activityCount={activityCounts.get(name) ?? 0}
          />
        );
      }
      if (kind === "model") {
        return <ModelDetail model={(data?.byModel ?? []).find((m) => m.model === entity)} />;
      }
      return (
        <RepoDetail
          label={entity}
          paths={(data?.byRepo ?? [])
            .filter((r) => r.label === entity)
            .map((r) => ({ key: r.key, total: r.total }))}
        />
      );
    },
    [data?.byAgent, data?.byModel, data?.byRepo, data?.daily, spendSeries, activityCounts],
  );

  // ── Render ────────────────────────────────────────────────────────────────

  if (loading && !data) return <div className="p-6 space-y-6"><LoadingSkeleton variant="cards" rows={4} /></div>;
  if (timedOut && !data) return <div className="p-6"><EmptyState icon="!" title="Insights timed out" actionLabel="Retry" onAction={refresh} /></div>;
  if (error && !data) return <div className="p-6"><EmptyState icon="!" title="Failed to load insights" description={error} actionLabel="Retry" onAction={refresh} /></div>;
  if (!data) return null;

  const periodLabel = isAll ? "all time" : `last ${cfg.label}`;
  const hasLedger = (data.summary?.record_count ?? 0) > 0 || spendSeries.some((d) => d.cost > 0);
  const todayKey = dayKey(Date.now());

  if (!hasLedger && data.daily.length === 0 && data.byAgent.length === 0) {
    // A quiet period on a fleet with history is real data (zeros), but a
    // window with no ledger at all gets one clear explanation instead of
    // four empty panels.
    return (
      <div className="p-6">
        <EmptyState
          icon="$"
          title={isAll ? "No cost data yet" : `No spend in the ${periodLabel}`}
          description="Costs are read directly from provider session transcripts."
          actionLabel={isAll ? undefined : "View all time"}
          onAction={isAll ? undefined : () => setPeriod("all")}
        />
      </div>
    );
  }

  return (
    <div className="p-6 max-w-6xl mx-auto space-y-8">
      {/* ── Stat band — four numbers, hairline-divided, period-scoped ── */}
      <div className="rounded-lg border border-mycel-border shadow-mycel-sm overflow-hidden">
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-px bg-mycel-border">
          <StatCell
            label={`Spend · ${periodLabel}`}
            value={formatCost(stat.spend)}
            sub={isAll
              ? (data.daily[0] ? `since ${fmtShortDate(data.daily[0].date)}` : undefined)
              : deltaLabel(stat.spend, stat.prevSpend, cfg.label) ?? undefined}
          />
          <StatCell
            label="Today"
            value={formatCost(stat.today)}
            sub={stat.avg7 > 0 ? `${formatCost(stat.avg7)}/day avg over 7d` : undefined}
          />
          <StatCell
            label={`Tokens · ${periodLabel}`}
            value={fmtTokens((cache.input ?? 0) + (cache.output ?? 0))}
            sub={`${fmtTokens(cache.input)} in · ${fmtTokens(cache.output)} out`}
            onClick={() => setTokensOpen("1")}
            open={tokensOpen !== null}
          />
          <StatCell
            label="Cache hit rate"
            value={cache.hasData ? `${(cache.ratio * 100).toFixed(1)}%` : "—"}
            sub={cache.hasData ? `${fmtTokens(cache.read)} tokens read from cache` : undefined}
          />
        </div>
        <Disclosure open={tokensOpen !== null} onClose={() => setTokensOpen(null)} label="Token composition">
          <TokenPanel series={tokenSeries} summary={data.summary} periodLabel={periodLabel} />
        </Disclosure>
      </div>

      {/* ── Budget cap — moved here from Settings; it governs the spend
          shown above. ── */}
      <BudgetPanel />

      {/* ── Spend over time ── */}
      <section>
        <SectionRule label="Spend" trailing={
          <span className="text-[11px] text-mycel-muted tabular-nums">{periodLabel} · daily, UTC</span>
        } />
        {spendSeries.length === 0 ? (
          <div className="py-10 text-center text-sm text-mycel-muted">No ledger days yet</div>
        ) : (
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={spendSeries} margin={{ top: 8, right: 8, left: 0, bottom: 0 }} barCategoryGap="18%">
              <CartesianGrid stroke="var(--mycel-border)" strokeOpacity={0.6} vertical={false} />
              <XAxis
                dataKey="date"
                tick={TICK}
                {...AX}
                tickFormatter={fmtShortDate}
                interval="preserveStartEnd"
                minTickGap={48}
              />
              <YAxis
                tick={TICK}
                {...AX}
                width={56}
                tickFormatter={(v: number) =>
                  v === 0 ? "$0" : v >= 10 ? `$${Math.round(v).toLocaleString()}` : `$${v.toFixed(2)}`}
              />
              <Tooltip content={<SpendTooltip />} cursor={{ fill: "var(--mycel-surface-hover)", fillOpacity: 0.5 }} />
              <Bar dataKey="cost" name="Spend" radius={[3, 3, 0, 0]} isAnimationActive={false}>
                {spendSeries.map((d) => (
                  <Cell
                    key={d.date}
                    fill={ACCENT}
                    fillOpacity={d.date === todayKey ? 1 : 0.55}
                  />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}
      </section>

      {/* ── Where it goes + cache efficiency ── */}
      <section>
        <SectionRule
          label="Where it goes"
          trailing={
            <div className="flex gap-1" role="group" aria-label="Breakdown dimension">
              {([
                { key: "agent", label: "Agents" },
                { key: "model", label: "Models" },
                { key: "repo", label: "Repos" },
              ] as const).map((d) => (
                <button
                  key={d.key}
                  type="button"
                  onClick={() => {
                    setDimension(d.key);
                    // A drill-down from another dimension can't render here.
                    if (expandedRow !== null && !expandedRow.startsWith(`${d.key}:`)) {
                      setExpandedRow(null);
                    }
                  }}
                  aria-pressed={dimension === d.key}
                  className={`px-2 py-0.5 text-[11px] rounded-md border transition-colors ${
                    dimension === d.key
                      ? "border-mycel-accent text-mycel-accent"
                      : "border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-muted"
                  }`}
                >
                  {d.label}
                </button>
              ))}
            </div>
          }
        />
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
          <div className="lg:col-span-2">
            <Breakdown
              rows={breakdown.rows}
              total={breakdown.total}
              expandedId={expandedRow}
              onToggle={setExpandedRow}
              renderDetail={renderBreakdownDetail}
            />
          </div>

          {/* Cache efficiency — why the bill stays sane. */}
          <div className="bg-mycel-surface border border-mycel-border rounded-lg shadow-mycel-sm p-4 space-y-3">
            <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
              Cache efficiency · {periodLabel}
            </div>
            {!cache.hasData ? (
              <div className="text-sm text-mycel-muted">No token data in this period</div>
            ) : (
              <>
                <div className="flex items-baseline gap-2">
                  <span className="text-2xl font-semibold text-mycel-text">{(cache.ratio * 100).toFixed(1)}%</span>
                  <span className="text-[11px] text-mycel-muted">of context read from cache</span>
                </div>
                <div
                  className="h-2 rounded-full bg-mycel-border/40 overflow-hidden"
                  role="img"
                  aria-label={`${(cache.ratio * 100).toFixed(1)} percent of context tokens served from cache`}
                >
                  <div className="h-full rounded-full" style={{ width: `${cache.ratio * 100}%`, backgroundColor: "var(--mycel-chart-5)" }} />
                </div>
                <dl className="space-y-1 text-[11px]">
                  {[
                    { k: "Cache reads", v: cache.read },
                    { k: "Cache writes", v: cache.write },
                    { k: "Fresh input", v: cache.input },
                  ].map((r) => (
                    <div key={r.k} className="flex justify-between gap-2">
                      <dt className="text-mycel-muted">{r.k}</dt>
                      <dd className="tabular-nums text-mycel-text">{fmtTokens(r.v)}</dd>
                    </div>
                  ))}
                </dl>
                <p className="text-[11px] leading-relaxed text-mycel-muted">
                  Cached context is re-read at roughly a tenth of the fresh-input price —
                  the reason billions of context tokens don&apos;t cost like billions.
                </p>
              </>
            )}
          </div>
        </div>
      </section>

      {/* ── Activity ── */}
      <section>
        <SectionRule
          label="Activity"
          trailing={
            activity.eventCount > 0 ? (
              <span className="text-[11px] text-mycel-muted tabular-nums">
                {activity.windowLabel} · {activity.eventCount.toLocaleString()} events · {activity.agentCount} agents
              </span>
            ) : undefined
          }
        />
        {activity.buckets.length === 0 ? (
          <div className="py-10 text-center text-sm text-mycel-muted">No recent agent activity</div>
        ) : (
          <>
            <div className="flex items-center gap-3 mb-2">
              {ACTIVITY_SERIES.map((s) => (
                <span key={s.key} className="inline-flex items-center gap-1.5 text-[11px] text-mycel-muted">
                  <span className="w-2 h-2 rounded-sm" style={{ backgroundColor: s.color }} />
                  {s.label}
                </span>
              ))}
            </div>
            <ResponsiveContainer width="100%" height={180}>
              <BarChart data={activity.buckets} margin={{ top: 4, right: 8, left: 0, bottom: 0 }} barCategoryGap="12%">
                <CartesianGrid stroke="var(--mycel-border)" strokeOpacity={0.6} vertical={false} />
                <XAxis
                  dataKey="t"
                  tick={TICK}
                  {...AX}
                  tickFormatter={(v: number) => fmtClock(v)}
                  interval="preserveStartEnd"
                  minTickGap={64}
                />
                <YAxis tick={TICK} {...AX} width={40} allowDecimals={false} />
                <Tooltip content={<ActivityTooltip />} cursor={{ fill: "var(--mycel-surface-hover)", fillOpacity: 0.5 }} />
                {ACTIVITY_SERIES.map((s, i) => (
                  <Bar
                    key={s.key}
                    dataKey={s.key}
                    name={s.label}
                    stackId="a"
                    fill={s.color}
                    stroke="var(--mycel-surface)"
                    strokeWidth={1}
                    radius={i === ACTIVITY_SERIES.length - 1 ? [2, 2, 0, 0] : undefined}
                    isAnimationActive={false}
                  />
                ))}
              </BarChart>
            </ResponsiveContainer>
          </>
        )}
      </section>

      {/* ── System — the machine underneath, live ── */}
      <section>
        <SectionRule
          label="System"
          trailing={
            <span className="inline-flex items-center gap-1.5 text-[11px] text-mycel-muted tabular-nums">
              <span className="relative flex w-1.5 h-1.5" aria-hidden>
                <span className="absolute inline-flex w-full h-full rounded-full bg-mycel-success opacity-60 animate-ping [animation-duration:3s]" />
                <span className="relative inline-flex w-1.5 h-1.5 rounded-full bg-mycel-success" />
              </span>
              live · host machine
            </span>
          }
        />
        <SystemRow />
      </section>
    </div>
  );
}
