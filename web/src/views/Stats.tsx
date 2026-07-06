import { useCallback, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  AreaChart, Area, BarChart, Bar, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../api/client";
import type {
  SystemStats, StatsSummary, CostSummary, ModelCostSummary, AgentCostSummary,
  AgentMetricTS, ChannelStats, DailyCost,
} from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { Panel, Empty, fmtTime, fmtBytes, fmtTokens } from "../components/shared/stats-primitives";

import { useHeaderSlot } from "../context/HeaderSlotContext";
// ── Model Pricing ───────────────────────────────────────────────────────────────

const MODEL_PRICING: Record<string, { input: number; output: number }> = {
  "claude-opus-4-6": { input: 15, output: 75 },
  "claude-sonnet-4-6": { input: 3, output: 15 },
  "claude-haiku-4-5-20251001": { input: 0.80, output: 4 },
  "claude-3-5-sonnet-20241022": { input: 3, output: 15 },
  "claude-3-5-haiku-20241022": { input: 0.80, output: 4 },
};

export function calculateCost(model: string, inputTokens: number, outputTokens: number): number {
  const pricing = MODEL_PRICING[model] ?? { input: 3, output: 15 };
  return (inputTokens / 1_000_000) * pricing.input + (outputTokens / 1_000_000) * pricing.output;
}

// ── Constants ───────────────────────────────────────────────────────────────────

// Chart palette. First entry uses the theme accent token so charts feel
// like they belong to the current theme (mycel orange in both Dark and
// Light). The rest are theme-agnostic hues that stay readable in both.
const ACCENT = "var(--mycel-accent)";
// Categorical palette chosen for six-plus-series distinguishability
// across both themes. First slot follows the theme accent; the
// rest are picked from the Radix + Tailwind categorical palettes so
// adjacent series never collide. Deuteranopia-checked pairs:
// cobalt/emerald safe, rose/violet safe, amber/tangerine handled by
// dash pattern rotation below.
const COLORS = [
  ACCENT,      // theme accent — var(--mycel-accent), orange in both themes
  "#3B82F6",   // cobalt
  "#EC4899",   // rose
  "#F59E0B",   // amber
  "#A855F7",   // violet
  "#06B6D4",   // cyan
  "#84CC16",   // lime
  "#F97316",   // tangerine (only conflicts with the orange accent — kicked to slot 7)
];

// Dash-pattern palette for a11y — color-only encoding fails deuteranopia
// simulators; giving each series a distinct stroke pattern keeps the
// chart legible without needing to distinguish adjacent hues. Slot 0 is
// always solid so the theme-accent series stays "hero"; other slots
// rotate through dash / dot / long-dash patterns. #3205 P2 a11y.
const DASH_PATTERNS: string[] = [
  "",              // solid (theme accent)
  "5 3",           // long dash
  "2 2",           // dot
  "6 3 2 3",       // dash-dot
  "4 4",           // short dash
  "8 4",           // extra-long dash
  "3 2 2 2 2 2",   // dash-dot-dot
  "1 3",           // sparse dot
];

/**
 * Per-agent Area chart series with the repeated stroke/fill/dot/connect
 * boilerplate consolidated. `color` is optional — falls back to the
 * theme-accented first palette slot when the agent has no assigned color.
 * Pass `dashIndex` to select a stroke pattern (see DASH_PATTERNS) for a11y.
 * #3205 v0.3.3 batches 7+8.
 */
function AgentArea({
  name,
  color,
  fillOpacity,
  stackId,
  dashIndex,
}: {
  name: string;
  color?: string;
  fillOpacity: number;
  stackId?: string;
  dashIndex?: number;
}) {
  const c = color ?? COLORS[0];
  const dash = dashIndex !== undefined ? DASH_PATTERNS[dashIndex % DASH_PATTERNS.length] : "";
  return (
    <Area
      type="monotone"
      dataKey={name}
      stroke={c}
      strokeDasharray={dash || undefined}
      fill={c}
      fillOpacity={fillOpacity}
      strokeWidth={1.75}
      dot={false}
      connectNulls
      stackId={stackId}
    />
  );
}

/**
 * Interactive per-chart legend. Renders one chip per agent with the
 * matching swatch + dash pattern; hover raises the paired chart series
 * to full opacity via `hidden` set on siblings, and click toggles the
 * series' visibility (persisted in `hidden` state). Passed as a
 * <Panel actions={...} /> to sit inline with the panel title.
 * #3205 v0.3.3 batch 9.
 */
function InteractiveLegend({
  agents,
  colors,
  hovered,
  hidden,
  onHover,
  onToggle,
}: {
  agents: string[];
  colors: Record<string, string>;
  hovered: string | null;
  hidden: Set<string>;
  onHover: (n: string | null) => void;
  onToggle: (n: string) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {agents.map((n, i) => {
        const isHidden = hidden.has(n);
        const dash = DASH_PATTERNS[i % DASH_PATTERNS.length];
        return (
          <button
            key={n}
            type="button"
            onClick={() => onToggle(n)}
            onMouseEnter={() => onHover(n)}
            onMouseLeave={() => onHover(null)}
            className={`inline-flex items-center gap-1.5 rounded-md border px-1.5 py-0.5 text-[10px] font-mono transition-opacity ${
              isHidden
                ? "border-mycel-border opacity-40 text-mycel-muted"
                : hovered && hovered !== n
                  ? "border-mycel-border opacity-50"
                  : "border-mycel-border text-mycel-text"
            }`}
            aria-pressed={!isHidden}
            title={isHidden ? `Show ${n}` : `Hide ${n}`}
          >
            <svg width="14" height="6" viewBox="0 0 14 6" aria-hidden>
              <line
                x1="0" y1="3" x2="14" y2="3"
                stroke={colors[n] ?? COLORS[0]}
                strokeWidth={2}
                strokeDasharray={dash || undefined}
              />
            </svg>
            <span className="truncate max-w-[110px]">{n}</span>
          </button>
        );
      })}
    </div>
  );
}
const RANGES = [
  { label: "1h", seconds: 3600 },
  { label: "6h", seconds: 21600 },
  { label: "12h", seconds: 43200 },
  { label: "24h", seconds: 86400 },
  { label: "7d", seconds: 604800 },
  { label: "30d", seconds: 2592000 },
] as const;

// Infrastructure container names to exclude from agent charts. Both the
// canonical mycel-* namespace (v0.3.1+) and the legacy bc-* namespace are
// filtered for one release cycle so pre-rename containers don't leak in.
const INFRA = [
  "mycel-db", "mycel-daemon", "mycel-playwright",
  "bc-db", "bc-daemon", "bc-playwright",
];
const isInfra = (n: string) => INFRA.some(p => n === p || n.startsWith(p + "-")) || n.length <= 3;

const TT: React.CSSProperties = {
  backgroundColor: "var(--mycel-surface-2)", border: "1px solid var(--mycel-border)",
  borderRadius: "6px", color: "var(--mycel-text)", fontSize: "12px",
  boxShadow: "var(--mycel-shadow-lg)",
};
const AX = { axisLine: false as const, tickLine: false as const };
const TICK_STYLE = { fill: "var(--mycel-muted)", fontSize: 10 };

// ── Helpers ─────────────────────────────────────────────────────────────────────

// Use the shared formatter so large totals get comma-grouped instead
// of being collapsed to "$X.XK". The audit flagged three divergent
// cost formatters (utils/format.formatCost, StatsTab.fmtCost, and this
// one); this module now delegates to the utility.
import { formatCost } from "../utils/format";
const fmtCost = (n: number) => formatCost(n);
const trunc = (s: string, n: number) => s.length > n ? s.slice(0, n) + "\u2026" : s;

// Cost-ledger agent ids are namespaced "bc-<workspace>-<agent>" (e.g.
// "bc-bc-zen-zebra"). Drop the "bc-" prefix and the workspace segment to
// show the bare agent name ("zen-zebra"); fall back to the raw id if that
// leaves nothing.
function stripAgentPrefix(id: string): string {
  if (id.startsWith("bc-")) {
    const rest = id.split("-").slice(2).join("-");
    return rest || id;
  }
  return id;
}

function fromParam(seconds: number): string {
  return new Date(Date.now() - seconds * 1000).toISOString();
}

// ── Data ────────────────────────────────────────────────────────────────────────

interface StatsData {
  system: SystemStats | null;
  summary: StatsSummary | null;
  costSummary: CostSummary | null;
  costByModel: ModelCostSummary[];
  costByAgent: AgentCostSummary[];
  agentCpu: AgentMetricTS[];
  agentMem: AgentMetricTS[];
  agentNet: AgentMetricTS[];
  agentDisk: AgentMetricTS[];
  notificationStats: ChannelStats[];
  costDaily: DailyCost[];
}

type SortKey = "name" | "role" | "provider" | "state" | "cpu" | "mem" | "tokens" | "cost";

// ── Dashboard chrome ──────────────────────────────────────────────────────────────

// Single-page dashboard sections. Order drives both the sticky anchor-nav
// and the on-page render order. Each id is the scroll target for its pill.
const SECTIONS = [
  { id: "agents", label: "Agents" },
  { id: "cost", label: "Cost" },
  { id: "usage", label: "Usage" },
  { id: "system", label: "System" },
  { id: "activity", label: "Activity" },
] as const;

// States that count as a live agent for the "Active agents" KPI.
const ALIVE_STATES = new Set(["working", "idle", "running", "starting"]);

/** Sticky anchor-nav — quiet pills that smooth-scroll to each section. */
function AnchorNav() {
  return (
    <div className="sticky top-0 z-10 -mx-6 px-6 py-2 bg-mycel-bg/80 backdrop-blur-sm border-b border-mycel-border flex items-center gap-1.5 flex-wrap">
      {SECTIONS.map((s) => (
        <button
          key={s.id}
          type="button"
          onClick={() => document.getElementById(s.id)?.scrollIntoView({ behavior: "smooth", block: "start" })}
          className="px-2.5 py-1 text-[11px] font-medium rounded-full border border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-muted transition-colors"
        >
          {s.label}
        </button>
      ))}
    </div>
  );
}

/** Section header matching the Tools.tsx pattern: label + count + rule. */
function SectionHeader({ label, count }: { label: string; count: number | string }) {
  return (
    <div className="flex items-baseline gap-2 mb-3">
      <h2 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em]">{label}</h2>
      <span className="text-[11px] text-mycel-muted tabular-nums">{count}</span>
      <span className="flex-1 h-px bg-mycel-border self-center" aria-hidden />
    </div>
  );
}

/** Compact KPI tile for the top strip; a Link when `to` is provided. */
function KpiTile({ label, value, sub, to }: { label: string; value: React.ReactNode; sub?: string; to?: string }) {
  const cls = `block bg-mycel-surface border border-mycel-border rounded-lg shadow-mycel-sm p-3${to ? " hover:border-mycel-accent transition-colors" : ""}`;
  const body = (
    <>
      <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em] truncate">{label}</div>
      <div className="mt-1 text-lg font-semibold tabular-nums text-mycel-text truncate">{value}</div>
      {sub && <div className="text-[10px] text-mycel-muted truncate">{sub}</div>}
    </>
  );
  return to ? <Link to={to} className={cls}>{body}</Link> : <div className={cls}>{body}</div>;
}

// ── Main ────────────────────────────────────────────────────────────────────────

export function Stats() {
  // Slot the range picker directly into the page header actions area so
  // it sits inline with the title instead of floating in a mostly-empty
  // sub-row (#3205 v0.3.2). The picker component is defined below and
  // reads/writes the local `range` state.

  const navigate = useNavigate();
  const [range, setRange] = useState(0);
  const [sortKey, setSortKey] = useState<SortKey>("cost");
  const [sortAsc, setSortAsc] = useState(false);
  // Interactive-legend state, shared by the CPU + Memory charts since
  // they share an agent set. Hover raises the paired series; click
  // toggles visibility. #3205 v0.3.3 batch 9.
  const [hoveredAgent, setHoveredAgent] = useState<string | null>(null);
  const [hiddenAgents, setHiddenAgents] = useState<Set<string>>(new Set());
  const toggleAgent = useCallback((n: string) => {
    setHiddenAgents((prev) => {
      const next = new Set(prev);
      if (next.has(n)) next.delete(n); else next.add(n);
      return next;
    });
  }, []);

  useHeaderSlot({
    actions: (
      <div className="flex gap-1">
        {RANGES.map((r, i) => (
          <button key={r.label} type="button" onClick={() => setRange(i)}
            className={`px-2 py-0.5 text-[11px] rounded-md border transition-colors ${
              /* Selected state uses accent text + border only — the
                 tinted accent bg was a triple-accent that competed
                 with the sidebar brand tile + active nav under Dark
                 (#3205 batch 10). */
              i === range
                ? "border-mycel-accent text-mycel-accent"
                : "border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-muted"
            }`}
          >{r.label}</button>
        ))}
      </div>
    ),
  });

  const from = useMemo(() => fromParam(RANGES[range]?.seconds ?? 3600), [range]);
  // Daily cost ledger window sized to the selected range: sub-day ranges
  // still pull a single day so the fallback chart has something to show.
  const daysForRange = useMemo(
    () => Math.max(1, Math.ceil((RANGES[range]?.seconds ?? 3600) / 86400)),
    [range],
  );

  const fetcher = useCallback(async (): Promise<StatsData> => {
    const p = { from };
    const [r0, r1, r2, r3, r4, r5, r6, r7, r8, r9, r10] = await Promise.allSettled([
      api.getStatsSystem(),
      api.getStatsSummary(),
      api.getCostSummary(),
      api.getCostByModel(),
      api.getCostByAgent(),
      api.getAgentStats("cpu", p),
      api.getAgentStats("mem", p),
      api.getAgentStats("net", p),
      api.getAgentStats("disk", p),
      api.getStatsChannels(),
      api.getCostDaily(daysForRange),
    ]);
    return {
      system: r0.status === "fulfilled" ? r0.value : null,
      summary: r1.status === "fulfilled" ? r1.value : null,
      costSummary: r2.status === "fulfilled" ? r2.value : null,
      costByModel: r3.status === "fulfilled" ? r3.value : [],
      costByAgent: r4.status === "fulfilled" ? (r4.value ?? []) : [],
      agentCpu: r5.status === "fulfilled" ? (r5.value ?? []) : [],
      agentMem: r6.status === "fulfilled" ? (r6.value ?? []) : [],
      agentNet: r7.status === "fulfilled" ? (r7.value ?? []) : [],
      agentDisk: r8.status === "fulfilled" ? (r8.value ?? []) : [],
      notificationStats: r9.status === "fulfilled" ? (r9.value ?? []) : [],
      costDaily: r10.status === "fulfilled" && Array.isArray(r10.value) ? r10.value : [],
    };
  }, [from, daysForRange]);

  const { data, loading, error, refresh, timedOut } = usePolling(fetcher, 10000);

  // ── Derived data ────────────────────────────────────────────────────────────

  const cpuChart = useMemo(() => pivotAgentMetric(data?.agentCpu ?? [], "cpu_percent"), [data?.agentCpu]);
  const memChart = useMemo(() => pivotAgentMetric(data?.agentMem ?? [], "mem_mb"), [data?.agentMem]);
  const netChart = useMemo(() => pivotNetOrDisk(data?.agentNet ?? [], "net"), [data?.agentNet]);
  const diskChart = useMemo(() => pivotNetOrDisk(data?.agentDisk ?? [], "disk"), [data?.agentDisk]);
  // ── Cost + token charts (cost ledger is the single source of truth) ──────────
  // The stats-store token timeseries (/agents/stats/tokens) is vestigial and
  // always empty, so every cost/token view derives directly from the cost
  // ledger: /costs (summary), /costs/agents, /costs/models, /costs/daily.

  // Cost Over Time — daily ledger, one point per day.
  const costOverTimeData = useMemo(
    () => (data?.costDaily ?? []).map(d => ({ time: d.date, cost: d.cost_usd })),
    [data?.costDaily],
  );

  // Token Throughput over time — daily ledger, input/output stacked.
  const tokenChart = useMemo(
    () => (data?.costDaily ?? []).map(d => ({ time: d.date, input: d.input_tokens, output: d.output_tokens })),
    [data?.costDaily],
  );

  // Cost by Agent — per-agent ledger, top 8 by spend.
  const costByAgentData = useMemo(
    () => (data?.costByAgent ?? [])
      .map(a => ({ name: stripAgentPrefix(a.agent_id), cost: a.total_cost_usd }))
      .filter(a => a.cost > 0)
      .sort((a, b) => b.cost - a.cost)
      .slice(0, 8),
    [data?.costByAgent],
  );

  // Cost by Model — per-model ledger, top 8 by spend.
  const costByModelBar = useMemo(
    () => (data?.costByModel ?? [])
      .map(m => ({ name: trunc(m.model, 24), cost: m.total_cost_usd }))
      .filter(m => m.cost > 0)
      .sort((a, b) => b.cost - a.cost)
      .slice(0, 8),
    [data?.costByModel],
  );

  // Model Usage (Tokens) — per-model ledger, top 8 by total tokens.
  const tokensByModel = useMemo(
    () => (data?.costByModel ?? [])
      .map(m => ({ name: trunc(m.model, 24), tokens: m.total_tokens }))
      .filter(m => m.tokens > 0)
      .sort((a, b) => b.tokens - a.tokens)
      .slice(0, 8),
    [data?.costByModel],
  );

  // Agent Token Breakdown — per-agent ledger, input/output split, top 8.
  const tokensByAgent = useMemo(
    () => (data?.costByAgent ?? [])
      .map(a => ({ name: stripAgentPrefix(a.agent_id), input: a.input_tokens, output: a.output_tokens, total: a.total_tokens }))
      .filter(a => a.total > 0)
      .sort((a, b) => b.total - a.total)
      .slice(0, 8),
    [data?.costByAgent],
  );

  // Cache Efficiency — the ledger exposes only aggregate cache totals (no
  // per-time series), so this is a summary view: read vs write tokens plus a
  // hit ratio = cache_read / (cache_read + input_tokens).
  const cacheStats = useMemo(() => {
    const s = data?.costSummary;
    const read = s?.cache_read_tokens ?? 0;
    const write = s?.cache_write_tokens ?? 0;
    const input = s?.input_tokens ?? 0;
    const ratio = read + input > 0 ? read / (read + input) : 0;
    return { read, write, ratio, hasData: read > 0 || write > 0 };
  }, [data?.costSummary]);
  const cacheBar = useMemo(
    () => [
      { name: "Read", tokens: cacheStats.read },
      { name: "Write", tokens: cacheStats.write },
    ],
    [cacheStats],
  );

  const notificationBarData = useMemo(() => {
    return [...(data?.notificationStats ?? [])]
      .sort((a, b) => b.message_count - a.message_count)
      .slice(0, 10)
      .map(c => ({ name: trunc(c.name, 16), fullName: c.name, messages: c.message_count }));
  }, [data?.notificationStats]);

  const agentTable = useMemo(() => buildAgentTable(data, sortKey, sortAsc), [data, sortKey, sortAsc]);

  const agentColors = useMemo(() => {
    const names = [...new Set((data?.agentCpu ?? []).map(m => m.agent_name).filter(n => !isInfra(n)))];
    const map: Record<string, string> = {};
    names.forEach((n, i) => { map[n] = COLORS[i % COLORS.length]!; });
    return map;
  }, [data?.agentCpu]);

  // Aggregates from time-range data
  const avgCpu = agentTable.length > 0 ? agentTable.reduce((s, a) => s + a.cpu, 0) / agentTable.length : 0;
  const totalMem = agentTable.reduce((s, a) => s + a.mem, 0);
  const totalTokens = agentTable.reduce((s, a) => s + a.tokens, 0);
  const totalCost = agentTable.reduce((s, a) => s + a.cost, 0);

  // ── KPI strip values ─────────────────────────────────────────────────────────
  // Every KPI is range-scoped off the cost ledger. Spend and tokens sum the
  // daily ledger for the selected window; burn is that spend over the window's
  // hours; the top cost driver is the per-agent ledger's biggest spender.
  const aliveCount = agentTable.filter((a) => ALIVE_STATES.has(a.state)).length;
  const rangeSpend = useMemo(
    () => (data?.costDaily ?? []).reduce((s, d) => s + (d.cost_usd ?? 0), 0),
    [data?.costDaily],
  );
  const rangeTokens = useMemo(
    () => (data?.costDaily ?? []).reduce((s, d) => s + (d.total_tokens ?? 0), 0),
    [data?.costDaily],
  );
  const displaySpend = rangeSpend;
  const kpiTokens = rangeTokens;
  // Burn rate = range spend over the window the daily ledger covers
  // (daysForRange * 24h). With no spend a rate is meaningless → "—".
  const burnRate = rangeSpend > 0 ? rangeSpend / (daysForRange * 24) : null;
  const topDriver = useMemo(() => {
    const top = (data?.costByAgent ?? []).reduce<AgentCostSummary | null>(
      (t, a) => (a.total_cost_usd > 0 && (!t || a.total_cost_usd > t.total_cost_usd) ? a : t),
      null,
    );
    return top ? { name: stripAgentPrefix(top.agent_id), cost: top.total_cost_usd } : null;
  }, [data?.costByAgent]);

  // ── Render ──────────────────────────────────────────────────────────────────

  if (loading && !data) return <div className="p-6 space-y-6"><LoadingSkeleton variant="cards" rows={4} /></div>;
  if (timedOut && !data) return <div className="p-6"><EmptyState icon="!" title="Stats timed out" actionLabel="Retry" onAction={refresh} /></div>;
  if (error && !data) return <div className="p-6"><EmptyState icon="!" title="Failed to load stats" description={error} actionLabel="Retry" onAction={refresh} /></div>;
  if (!data) return null;

  function handleSort(key: SortKey) {
    if (sortKey === key) setSortAsc(!sortAsc);
    else { setSortKey(key); setSortAsc(false); }
  }

  const colHeaders: { key: SortKey; label: string; agg?: string }[] = [
    { key: "name", label: "Name" },
    { key: "role", label: "Role" },
    { key: "provider", label: "Provider" },
    { key: "state", label: "State" },
    { key: "cpu", label: "CPU%", agg: `avg ${avgCpu.toFixed(1)}` },
    { key: "mem", label: "Mem MB", agg: `total ${totalMem >= 1024 ? `${(totalMem / 1024).toFixed(1)}G` : `${totalMem.toFixed(0)}M`}` },
    { key: "tokens", label: "Tokens", agg: fmtTokens(totalTokens) },
    { key: "cost", label: "Cost", agg: fmtCost(totalCost) },
  ];

  return (
    <div className="p-6 space-y-6">
      {/* KPI strip — compact derived tiles across the top of the dashboard. */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3">
        <KpiTile label="Spend (this range)" value={fmtCost(displaySpend)} />
        <KpiTile label="Tokens" value={fmtTokens(kpiTokens)} />
        <KpiTile label="Active agents" value={`${aliveCount} / ${agentTable.length}`} />
        <KpiTile label="Burn rate" value={burnRate === null ? "—" : `$${burnRate.toFixed(2)}/hr`} />
        {/* Fix #3: sub-label clarifies this is ALL-TIME, not range-scoped.
            The per-agent cost ledger sums the full lifetime so this KPI can
            read 20× the adjacent "Spend (this range)" tile — the "all-time"
            qualifier makes the discrepancy immediately legible. */}
        {topDriver ? (
          <KpiTile
            label="Top cost driver"
            value={topDriver.name}
            sub={`$${topDriver.cost.toFixed(2)} · all-time`}
            to={`/agents/${encodeURIComponent(topDriver.name)}`}
          />
        ) : (
          <KpiTile label="Top cost driver" value="—" />
        )}
      </div>

      {/* Sticky anchor-nav — smooth-scrolls to each section. */}
      <AnchorNav />

      {/* ── Agents ── */}
      <section id="agents" className="scroll-mt-16">
        <SectionHeader label="Agents" count={agentTable.length} />
      {/* Agent Table */}
      {agentTable.length > 0 && (
        <Panel title={`Agents (${agentTable.length})`}>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-mycel-muted text-left">
                  {colHeaders.map(h => (
                    <th key={h.key} className="py-2.5 px-3 font-medium cursor-pointer hover:text-mycel-text select-none group" onClick={(e) => { e.stopPropagation(); e.preventDefault(); handleSort(h.key); }}>
                      <div className="flex items-center">
                        {h.label}
                        {/* Neutral sort affordance surfaces on hover for every
                            column so users see the option; active column shows
                            the current direction in the accent color.
                            (#3205 v0.3.2) */}
                        <span className={`ml-1 text-[9px] leading-none tabular-nums ${
                          sortKey === h.key ? "text-mycel-accent opacity-100" : "opacity-0 group-hover:opacity-60"
                        }`}>
                          {sortKey === h.key ? (sortAsc ? "\u25B2" : "\u25BC") : "\u25BE"}
                        </span>
                      </div>
                      {h.agg && <div className="text-[10px] font-normal text-mycel-muted">{h.agg}</div>}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {agentTable.map(a => (
                  <tr key={a.name}
                    className="border-t border-mycel-border hover:bg-mycel-surface-hover cursor-pointer transition-colors"
                    onClick={() => navigate(`/agents/${encodeURIComponent(a.name)}`)}
                  >
                    <td className="py-2.5 px-3 font-medium">
                      <span className="flex items-center gap-1.5">
                        <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: agentColors[a.name] ?? COLORS[0] }} />
                        {a.name}
                      </span>
                    </td>
                    <td className="py-2.5 px-3 text-mycel-muted">{a.role}</td>
                    <td className="py-2.5 px-3 text-mycel-muted">{a.provider}</td>
                    <td className="py-2.5 px-3">
                      <span className="flex items-center gap-1.5">
                        {/* Distinct semantic tokens per state so idle vs
                            working vs running don't all collapse to the
                            same green. Unknown → muted grey (rendered
                            without a text label since it's usually
                            infra like `server`). */}
                        <span className={`w-1.5 h-1.5 rounded-full ${
                          a.state === "working" ? "bg-mycel-success"
                          : a.state === "idle" ? "bg-mycel-warning"
                          : a.state === "running" ? "bg-mycel-info"
                          : a.state === "stuck" ? "bg-mycel-warning"
                          : a.state === "error" ? "bg-mycel-error"
                          : "bg-mycel-muted"
                        }`} />
                        <span className={(!a.state || a.state === "unknown") ? "text-mycel-muted italic" : ""}>
                          {(!a.state || a.state === "unknown") ? "system" : a.state}
                        </span>
                      </span>
                    </td>
                    <td className="py-2.5 px-3 font-mono">{a.cpu.toFixed(1)}</td>
                    <td className="py-2.5 px-3 font-mono">{a.mem.toFixed(0)}</td>
                    <td className="py-2.5 px-3 font-mono">{fmtTokens(a.tokens)}</td>
                    <td className={`py-2.5 px-3 font-mono ${a.cost > 0 ? "text-mycel-accent" : "text-mycel-muted"}`}>${a.cost.toFixed(2)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>
      )}

      </section>

      {/* ── Cost ── */}
      <section id="cost" className="scroll-mt-16">
        <SectionHeader label="Cost" count={3} />
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Cost Over Time">
          {costOverTimeData.length === 0 ? <Empty msg="No cost data" /> : costOverTimeData.length < 2 ? (
            /* Fix #5: A single data point renders as a lone disconnected dot.
               Show a compact text summary instead — legible and accurate. */
            <div className="flex flex-col items-center justify-center h-[200px] gap-1">
              <span className="text-2xl font-semibold tabular-nums text-mycel-text">
                {fmtCost(costOverTimeData[0]?.cost ?? 0)}
              </span>
              <span className="text-xs text-mycel-muted">
                {costOverTimeData[0]?.time ?? "today"}
              </span>
            </div>
          ) : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={costOverTimeData} margin={{ top: 4, right: 8, left: -8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => `$${v.toFixed(2)}`} />
                <Tooltip contentStyle={TT} formatter={(v) => [`$${Number(v ?? 0).toFixed(4)}`]} />
                <Area type="monotone" dataKey="cost" name="Cost" stroke={ACCENT} fill={ACCENT} fillOpacity={0.15} strokeWidth={1.5} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Cost by Agent">
          {costByAgentData.length === 0 ? <Empty msg="No cost data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={costByAgentData} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => `$${v}`} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={100} />
                <Tooltip contentStyle={TT} formatter={(v) => [`$${Number(v ?? 0).toFixed(4)}`]} />
                <Bar dataKey="cost" radius={[0, 3, 3, 0]}>
                  {costByAgentData.map((a, i) => <Cell key={i} fill={agentColors[a.name] ?? COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Cost by Model">
          {costByModelBar.length === 0 ? <Empty msg="No cost data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={costByModelBar} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => `$${v}`} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={120} />
                <Tooltip contentStyle={TT} formatter={(v) => [`$${Number(v ?? 0).toFixed(4)}`]} />
                <Bar dataKey="cost" radius={[0, 3, 3, 0]}>
                  {costByModelBar.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        </div>
      </section>

      {/* ── Usage ── */}
      <section id="usage" className="scroll-mt-16">
        <SectionHeader label="Usage" count={4} />
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Token Throughput">
          {tokenChart.length === 0 ? <Empty msg="No token data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={tokenChart} margin={{ top: 4, right: 8, left: -8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                <Tooltip contentStyle={TT} formatter={(v, n) => [Number(v ?? 0).toLocaleString(), n === "input" ? "Input" : "Output"]} />
                <Area type="monotone" dataKey="input" name="Input" stroke="#3B82F6" fill="#3B82F6" fillOpacity={0.20} strokeWidth={1.75} stackId="1" dot={false} />
                <Area type="monotone" dataKey="output" name="Output" stroke={ACCENT} fill={ACCENT} fillOpacity={0.20} strokeWidth={1.75} stackId="1" dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Model Usage (Tokens)">
          {tokensByModel.length === 0 ? <Empty msg="No model data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={tokensByModel} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={120} />
                <Tooltip contentStyle={TT} formatter={(v) => [fmtTokens(Number(v ?? 0))]} />
                <Bar dataKey="tokens" radius={[0, 3, 3, 0]}>
                  {tokensByModel.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Cache Efficiency">
          {!cacheStats.hasData ? <Empty msg="No cache data" /> : (
            <div className="flex flex-col gap-3">
              {/* Cache hit ratio headline + read/write token totals. The ledger
                  reports only aggregate cache counts (no per-time series), so
                  this is a summary rather than a time chart. */}
              <div className="flex items-baseline gap-6 px-1">
                <div>
                  <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">Hit ratio</div>
                  <div className="mt-0.5 text-2xl font-semibold tabular-nums text-mycel-accent">{(cacheStats.ratio * 100).toFixed(1)}%</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">Cache read</div>
                  <div className="mt-0.5 text-lg font-semibold tabular-nums text-mycel-text">{fmtTokens(cacheStats.read)}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">Cache write</div>
                  <div className="mt-0.5 text-lg font-semibold tabular-nums text-mycel-text">{fmtTokens(cacheStats.write)}</div>
                </div>
              </div>
              <ResponsiveContainer width="100%" height={120}>
                <BarChart layout="vertical" data={cacheBar} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                  <XAxis type="number" tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                  <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={60} />
                  <Tooltip contentStyle={TT} formatter={(v) => [fmtTokens(Number(v ?? 0))]} />
                  <Bar dataKey="tokens" radius={[0, 3, 3, 0]}>
                    <Cell fill="#10B981" />
                    <Cell fill="#F59E0B" />
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}
        </Panel>
        <Panel title="Agent Token Breakdown">
          {tokensByAgent.length === 0 ? <Empty msg="No token data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart data={tokensByAgent.slice(0, 8).map(a => ({ name: trunc(a.name, 12), input: a.input, output: a.output }))} margin={{ top: 4, right: 8, left: -8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="name" tick={{ ...TICK_STYLE, fontSize: 9 }} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                <Tooltip contentStyle={TT} formatter={(v, n) => [fmtTokens(Number(v ?? 0)), n === "input" ? "Input" : "Output"]} />
                <Bar dataKey="input" name="Input" fill="#3B82F6" radius={[3, 3, 0, 0]} />
                <Bar dataKey="output" name="Output" fill={ACCENT} radius={[3, 3, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        </div>
      </section>

      {/* ── System ── */}
      <section id="system" className="scroll-mt-16">
        <SectionHeader label="System" count={4} />
        {/* Interactive shared legend for the CPU + Memory charts. Hover
            raises the paired series, click toggles visibility. Both charts
            key off the same agent set so one legend controls both. */}
        {(cpuChart.agents.length > 0 || memChart.agents.length > 0) && (
          <div className="mb-3">
            <InteractiveLegend
              agents={cpuChart.agents.length > 0 ? cpuChart.agents : memChart.agents}
              colors={agentColors}
              hovered={hoveredAgent}
              hidden={hiddenAgents}
              onHover={setHoveredAgent}
              onToggle={toggleAgent}
            />
          </div>
        )}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="CPU by Agent (%)">
          {cpuChart.data.length === 0 ? <Empty msg="No CPU data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={cpuChart.data} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                {/* Y auto-scales but stays at least 5% tall so a busy-agent tiny value
                    isn't stranded on the baseline invisible. Not stacked — CPU% across
                    agents isn't additive; stacking made low values disappear when one
                    agent spiked. */}
                <YAxis
                  tick={TICK_STYLE}
                  {...AX}
                  domain={[0, (dataMax: number) => Math.max(dataMax * 1.1, 5)]}
                  tickFormatter={(v: number) => `${v.toFixed(v < 10 ? 1 : 0)}%`}
                />
                <Tooltip contentStyle={TT} formatter={(v, name) => [`${Number(v ?? 0).toFixed(2)}%`, name]} />
                {cpuChart.agents.filter(n => !hiddenAgents.has(n)).map((n) => {
                  const i = cpuChart.agents.indexOf(n);
                  const dim = hoveredAgent && hoveredAgent !== n;
                  return (
                    <AgentArea
                      key={n}
                      name={n}
                      color={agentColors[n]}
                      fillOpacity={dim ? 0.02 : 0.08}
                      dashIndex={i}
                    />
                  );
                })}
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Memory by Agent (MB)">
          {memChart.data.length === 0 ? <Empty msg="No memory data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={memChart.data} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} />
                <Tooltip contentStyle={TT} formatter={(v) => [`${Number(v ?? 0).toFixed(1)} MB`]} />
                {memChart.agents.filter(n => !hiddenAgents.has(n)).map((n) => {
                  const i = memChart.agents.indexOf(n);
                  const dim = hoveredAgent && hoveredAgent !== n;
                  return (
                    <AgentArea
                      key={n}
                      name={n}
                      color={agentColors[n]}
                      fillOpacity={dim ? 0.05 : 0.20}
                      stackId="mem"
                      dashIndex={i}
                    />
                  );
                })}
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Network I/O">
          {netChart.length === 0 ? <Empty msg="No network data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={netChart} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtBytes(v)} />
                <Tooltip contentStyle={TT} formatter={(v) => [fmtBytes(Number(v ?? 0))]} />
                <Area type="monotone" dataKey="rx" name="RX" stroke="#10B981" fill="#10B981" fillOpacity={0.20} strokeWidth={1.75} dot={false} />
                <Area type="monotone" dataKey="tx" name="TX" stroke={ACCENT} fill={ACCENT} fillOpacity={0.20} strokeWidth={1.75} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Disk I/O">
          {diskChart.length === 0 ? <Empty msg="No disk data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={diskChart} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtBytes(v)} />
                <Tooltip contentStyle={TT} formatter={(v) => [fmtBytes(Number(v ?? 0))]} />
                <Area type="monotone" dataKey="read" name="Read" stroke="#3B82F6" fill="#3B82F6" fillOpacity={0.20} strokeWidth={1.75} dot={false} />
                <Area type="monotone" dataKey="write" name="Write" stroke="#A855F7" fill="#A855F7" fillOpacity={0.20} strokeWidth={1.75} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        </div>
      </section>

      {/* ── Activity ── */}
      <section id="activity" className="scroll-mt-16">
        <SectionHeader label="Activity" count={notificationBarData.length} />
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Notification Activity (Top 10)">
          {notificationBarData.length === 0 ? <Empty msg="No notification data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={notificationBarData} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={100} />
                <Tooltip contentStyle={TT} formatter={(v) => [Number(v ?? 0).toLocaleString(), "Messages"]} />
                {/* Bars drill into the channel's notification feed, matching
                    the agents table → agent detail pattern. */}
                <Bar
                  dataKey="messages"
                  radius={[0, 3, 3, 0]}
                  cursor="pointer"
                  onClick={(entry) => {
                    const full = (entry as { fullName?: string }).fullName;
                    if (full) navigate(`/notifications/${encodeURIComponent(full)}`);
                  }}
                >
                  {notificationBarData.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        </div>
      </section>
    </div>
  );
}

// ── Data Transforms ─────────────────────────────────────────────────────────────

function pivotAgentMetric(metrics: AgentMetricTS[], mode: "cpu_percent" | "mem_mb") {
  const agents = [...new Set(metrics.filter(m => !isInfra(m.agent_name)).map(m => m.agent_name))];
  type Pt = Record<string, string | number>;
  const buckets = new Map<string, Pt>();
  for (const m of metrics) {
    if (isInfra(m.agent_name)) continue;
    const t = fmtTime(m.time);
    const b = buckets.get(t) ?? { time: t };
    b[m.agent_name] = mode === "cpu_percent"
      ? parseFloat(m.cpu_percent.toFixed(2))
      : parseFloat((m.mem_used_bytes / 1024 / 1024).toFixed(1));
    buckets.set(t, b);
  }
  // Backfill missing agents in each bucket with 0 — otherwise stacked areas
  // collapse at any bucket where an agent has no sample (#3182).
  for (const b of buckets.values()) {
    for (const a of agents) {
      if (b[a] === undefined) b[a] = 0;
    }
  }
  return { agents, data: Array.from(buckets.values()) };
}

function pivotNetOrDisk(metrics: AgentMetricTS[], kind: "net" | "disk") {
  const buckets = new Map<string, { time: string; rx: number; tx: number; read: number; write: number }>();
  for (const m of metrics) {
    if (isInfra(m.agent_name)) continue;
    const t = fmtTime(m.time);
    const b = buckets.get(t) ?? { time: t, rx: 0, tx: 0, read: 0, write: 0 };
    if (kind === "net") { b.rx += m.net_rx_bytes; b.tx += m.net_tx_bytes; }
    else { b.read += m.disk_read_bytes; b.write += m.disk_write_bytes; }
    buckets.set(t, b);
  }
  return Array.from(buckets.values());
}

interface AgentRow { name: string; role: string; provider: string; state: string; cpu: number; mem: number; tokens: number; cost: number }

function buildAgentTable(data: StatsData | null, sortKey: SortKey, sortAsc: boolean): AgentRow[] {
  if (!data) return [];
  const latest = new Map<string, AgentMetricTS>();
  for (const m of data.agentCpu) { if (!isInfra(m.agent_name)) latest.set(m.agent_name, m); }

  // Per-agent tokens + cost come straight from the cost ledger. Ledger ids are
  // namespaced ("bc-bc-zen-zebra") while the metrics agent_name is the bare
  // name ("zen-zebra"), so key the lookup by stripAgentPrefix(agent_id).
  const ledgerByName = new Map<string, { tokens: number; cost: number }>();
  for (const a of data.costByAgent) {
    ledgerByName.set(stripAgentPrefix(a.agent_id), { tokens: a.total_tokens, cost: a.total_cost_usd });
  }

  const memLatest = new Map<string, number>();
  for (const m of data.agentMem) { if (!isInfra(m.agent_name)) memLatest.set(m.agent_name, m.mem_used_bytes / 1024 / 1024); }

  // Fix #4: Filter out background system processes (e.g. "server") that appear
  // in the metrics stream but are not real user agents. An entry is a system
  // process when its state is "system" or when it has no role AND no known
  // provider tool — the combination uniquely identifies infra containers.
  const rows: AgentRow[] = Array.from(latest.values())
    .filter(m => m.state !== "system" && !((!m.role || m.role === "") && (!m.tool || m.tool === "")))
    .map(m => {
      const ledger = ledgerByName.get(m.agent_name);
      return {
        name: m.agent_name, role: m.role, provider: m.tool || "unknown", state: m.state,
        cpu: m.cpu_percent, mem: memLatest.get(m.agent_name) ?? 0,
        tokens: ledger?.tokens ?? 0, cost: ledger?.cost ?? 0,
      };
    });

  const dir = sortAsc ? 1 : -1;
  rows.sort((a, b) => {
    const av = a[sortKey], bv = b[sortKey];
    if (typeof av === "string" && typeof bv === "string") return av.localeCompare(bv) * dir;
    return ((av as number) - (bv as number)) * dir;
  });
  return rows;
}
