import { useCallback, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  AreaChart, Area, BarChart, Bar, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../api/client";
import type {
  SystemStats, StatsSummary, CostSummary, ModelCostSummary, AgentCostSummary,
  AgentMetricTS, TokenMetricTS, ChannelStats,
} from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { Panel, Empty, fmtTime, fmtBytes, fmtTokens } from "../components/shared/stats-primitives";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
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

// Chart palette. First entry uses the theme accent so charts feel like they
// belong to the current theme (tangerine in Solar Flare / Light, emerald in
// Dark). The rest are theme-agnostic hues that stay readable in all three.
const ACCENT = "var(--mycel-accent)";
const COLORS = [ACCENT, "#3B82F6", "#10B981", "#A855F7", "#F59E0B", "#EC4899", "#06B6D4", "#84CC16"];
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
  backgroundColor: "var(--mycel-surface)", border: "1px solid var(--mycel-border)",
  borderRadius: "6px", color: "var(--mycel-text)", fontSize: "12px",
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
  tokenMetrics: TokenMetricTS[];
  notificationStats: ChannelStats[];
}

type SortKey = "name" | "role" | "provider" | "state" | "cpu" | "mem" | "tokens" | "cost";

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

  useHeaderSlot({
    title: <TabHeaderTitle>Metrics</TabHeaderTitle>,
    actions: (
      <div className="flex gap-1">
        {RANGES.map((r, i) => (
          <button key={r.label} type="button" onClick={() => setRange(i)}
            className={`px-2 py-0.5 text-[11px] rounded border transition-colors ${
              i === range
                ? "border-mycel-accent bg-mycel-accent/10 text-mycel-accent"
                : "border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-muted"
            }`}
          >{r.label}</button>
        ))}
      </div>
    ),
  });

  const from = useMemo(() => fromParam(RANGES[range]?.seconds ?? 3600), [range]);

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
      api.getAgentTokenStats(p),
      api.getStatsChannels(),
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
      tokenMetrics: r9.status === "fulfilled" ? (r9.value ?? []) : [],
      notificationStats: r10.status === "fulfilled" ? (r10.value ?? []) : [],
    };
  }, [from]);

  const { data, loading, error, refresh, timedOut } = usePolling(fetcher, 10000);

  // ── Derived data ────────────────────────────────────────────────────────────

  const cpuChart = useMemo(() => pivotAgentMetric(data?.agentCpu ?? [], "cpu_percent"), [data?.agentCpu]);
  const memChart = useMemo(() => pivotAgentMetric(data?.agentMem ?? [], "mem_mb"), [data?.agentMem]);
  const netChart = useMemo(() => pivotNetOrDisk(data?.agentNet ?? [], "net"), [data?.agentNet]);
  const diskChart = useMemo(() => pivotNetOrDisk(data?.agentDisk ?? [], "disk"), [data?.agentDisk]);
  const tokenChart = useMemo(() => pivotTokens(data?.tokenMetrics ?? []), [data?.tokenMetrics]);
  const costOverTime = useMemo(() => pivotCostOverTime(data?.tokenMetrics ?? []), [data?.tokenMetrics]);
  const tokensByAgent = useMemo(() => pivotTokensByAgent(data?.tokenMetrics ?? []), [data?.tokenMetrics]);
  const tokensByModel = useMemo(() => pivotTokensByModel(data?.tokenMetrics ?? []), [data?.tokenMetrics]);

  // Time-range-filtered cost from token metrics
  const timeRangeCost = useMemo(() => {
    let total = 0;
    for (const t of data?.tokenMetrics ?? []) {
      total += calculateCost(t.model, t.input_tokens, t.output_tokens);
    }
    return total;
  }, [data?.tokenMetrics]);

  const hasCacheData = useMemo(() => (data?.tokenMetrics ?? []).some(t => t.cache_read > 0 || t.cache_create > 0), [data?.tokenMetrics]);
  const cacheChart = useMemo(() => {
    if (!hasCacheData) return [];
    const buckets = new Map<string, { time: string; cache_read: number; cache_create: number }>();
    for (const t of data?.tokenMetrics ?? []) {
      const k = fmtTime(t.time);
      const b = buckets.get(k) ?? { time: k, cache_read: 0, cache_create: 0 };
      b.cache_read += t.cache_read;
      b.cache_create += t.cache_create;
      buckets.set(k, b);
    }
    return Array.from(buckets.values());
  }, [data?.tokenMetrics, hasCacheData]);

  const notificationBarData = useMemo(() => {
    return [...(data?.notificationStats ?? [])]
      .sort((a, b) => b.message_count - a.message_count)
      .slice(0, 10)
      .map(c => ({ name: trunc(c.name, 16), messages: c.message_count }));
  }, [data?.notificationStats]);

  const costByModelBar = useMemo(() => {
    return tokensByModel
      .filter(m => m.cost > 0)
      .slice(0, 8)
      .map(m => ({ name: trunc(m.name, 24), cost: parseFloat(m.cost.toFixed(4)) }));
  }, [tokensByModel]);

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
    { key: "cost", label: "Cost", agg: fmtCost(timeRangeCost) },
  ];

  return (
    <div className="p-6 space-y-4">
      {/* Agent Table */}
      {agentTable.length > 0 && (
        <Panel title={`Agents (${agentTable.length})`}>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-mycel-muted text-left">
                  {colHeaders.map(h => (
                    <th key={h.key} className="py-1.5 px-2 font-medium cursor-pointer hover:text-mycel-text select-none group" onClick={(e) => { e.stopPropagation(); e.preventDefault(); handleSort(h.key); }}>
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
                    className="border-t border-mycel-border/50 hover:bg-mycel-bg/50 cursor-pointer transition-colors"
                    onClick={() => navigate(`/agents/${encodeURIComponent(a.name)}`)}
                  >
                    <td className="py-1.5 px-2 font-medium">
                      <span className="flex items-center gap-1.5">
                        <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: agentColors[a.name] ?? COLORS[0] }} />
                        {a.name}
                      </span>
                    </td>
                    <td className="py-1.5 px-2 text-mycel-muted">{a.role}</td>
                    <td className="py-1.5 px-2 text-mycel-muted">{a.provider}</td>
                    <td className="py-1.5 px-2">
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
                    <td className="py-1.5 px-2 font-mono">{a.cpu.toFixed(1)}</td>
                    <td className="py-1.5 px-2 font-mono">{a.mem.toFixed(0)}</td>
                    <td className="py-1.5 px-2 font-mono">{fmtTokens(a.tokens)}</td>
                    <td className={`py-1.5 px-2 font-mono ${a.cost > 0 ? "text-mycel-accent" : "text-mycel-muted"}`}>${a.cost.toFixed(2)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>
      )}

      {/* Legend removed — agent names + swatches are already in the
          Agents table's Name column immediately above. Interactive legend
          (hover-to-highlight, click-to-toggle) tracked as P2 on #3205. */}

      {/* Row 1: CPU + Memory */}
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
                {cpuChart.agents.map((n) => (
                  <Area
                    key={n}
                    type="monotone"
                    dataKey={n}
                    stroke={agentColors[n] ?? COLORS[0]}
                    fill={agentColors[n] ?? COLORS[0]}
                    fillOpacity={0.08}
                    strokeWidth={1.75}
                    dot={false}
                    connectNulls
                  />
                ))}
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
                {memChart.agents.map((n) => (
                  <Area key={n} type="monotone" dataKey={n} stroke={agentColors[n] ?? COLORS[0]} fill={agentColors[n] ?? COLORS[0]} fillOpacity={0.20} strokeWidth={1.75} dot={false} stackId="mem" />
                ))}
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
      </div>

      {/* Row 2: Token Flow */}
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
        <Panel title="Cost Over Time">
          {costOverTime.length === 0 ? <Empty msg="No cost data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={costOverTime} margin={{ top: 4, right: 8, left: -8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => `$${v.toFixed(2)}`} />
                <Tooltip contentStyle={TT} formatter={(v) => [`$${Number(v ?? 0).toFixed(4)}`]} />
                <Area type="monotone" dataKey="cost" name="Cost" stroke={ACCENT} fill={ACCENT} fillOpacity={0.15} strokeWidth={1.5} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
      </div>

      {/* Row 3: I/O */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
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

      {/* Row 4: Model & Cache */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Model Usage (Tokens)">
          {tokensByModel.length === 0 ? <Empty msg="No model data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={tokensByModel.slice(0, 8).map(m => ({ name: trunc(m.name, 24), tokens: m.tokens }))} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={120} />
                <Tooltip contentStyle={TT} formatter={(v) => [fmtTokens(Number(v ?? 0))]} />
                <Bar dataKey="tokens" radius={[0, 3, 3, 0]}>
                  {tokensByModel.slice(0, 8).map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Cache Efficiency">
          {!hasCacheData ? <Empty msg="Cache data — coming soon" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={cacheChart} margin={{ top: 4, right: 8, left: -8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK_STYLE} {...AX} />
                <YAxis tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                <Tooltip contentStyle={TT} formatter={(v, n) => [fmtTokens(Number(v ?? 0)), n === "cache_read" ? "Cache Read" : "Cache Create"]} />
                <Area type="monotone" dataKey="cache_read" name="Cache Read" stroke="#10B981" fill="#10B981" fillOpacity={0.20} strokeWidth={1.75} dot={false} />
                <Area type="monotone" dataKey="cache_create" name="Cache Create" stroke="#F59E0B" fill="#F59E0B" fillOpacity={0.20} strokeWidth={1.75} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
      </div>

      {/* Row 5: Notifications & Cost Breakdown */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Notification Activity (Top 10)">
          {notificationBarData.length === 0 ? <Empty msg="No notification data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={notificationBarData} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={100} />
                <Tooltip contentStyle={TT} formatter={(v) => [Number(v ?? 0).toLocaleString(), "Messages"]} />
                <Bar dataKey="messages" radius={[0, 3, 3, 0]}>
                  {notificationBarData.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Cost by Agent">
          {tokensByAgent.length === 0 ? <Empty msg="No cost data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={tokensByAgent.slice(0, 8).map(a => ({ name: trunc(a.name, 20), cost: parseFloat(a.cost.toFixed(4)) }))} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" horizontal={false} />
                <XAxis type="number" tick={TICK_STYLE} {...AX} tickFormatter={(v: number) => `$${v}`} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK_STYLE, fill: "var(--mycel-text)", fontSize: 9 }} {...AX} width={100} />
                <Tooltip contentStyle={TT} formatter={(v) => [`$${Number(v ?? 0).toFixed(4)}`]} />
                <Bar dataKey="cost" radius={[0, 3, 3, 0]}>
                  {tokensByAgent.slice(0, 8).map((a, i) => <Cell key={i} fill={agentColors[a.name] ?? COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
      </div>

      {/* Row 6: Agent Tokens & Cost by Model */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
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

function pivotTokens(tokens: TokenMetricTS[]) {
  const buckets = new Map<string, { time: string; input: number; output: number }>();
  for (const t of tokens) {
    const k = fmtTime(t.time);
    const b = buckets.get(k) ?? { time: k, input: 0, output: 0 };
    b.input += t.input_tokens;
    b.output += t.output_tokens;
    buckets.set(k, b);
  }
  return Array.from(buckets.values());
}

function pivotCostOverTime(tokens: TokenMetricTS[]) {
  const buckets = new Map<string, { time: string; cost: number }>();
  for (const t of tokens) {
    const k = fmtTime(t.time);
    const b = buckets.get(k) ?? { time: k, cost: 0 };
    b.cost += calculateCost(t.model, t.input_tokens, t.output_tokens);
    buckets.set(k, b);
  }
  return Array.from(buckets.values());
}

function pivotTokensByAgent(tokens: TokenMetricTS[]) {
  const agents = new Map<string, { name: string; input: number; output: number; cost: number }>();
  for (const t of tokens) {
    const a = agents.get(t.agent_name) ?? { name: t.agent_name, input: 0, output: 0, cost: 0 };
    a.input += t.input_tokens;
    a.output += t.output_tokens;
    a.cost += calculateCost(t.model, t.input_tokens, t.output_tokens);
    agents.set(t.agent_name, a);
  }
  return Array.from(agents.values()).sort((a, b) => b.cost - a.cost);
}

function pivotTokensByModel(tokens: TokenMetricTS[]) {
  const models = new Map<string, { name: string; tokens: number; cost: number }>();
  for (const t of tokens) {
    const m = models.get(t.model) ?? { name: t.model, tokens: 0, cost: 0 };
    m.tokens += t.input_tokens + t.output_tokens;
    m.cost += calculateCost(t.model, t.input_tokens, t.output_tokens);
    models.set(t.model, m);
  }
  return Array.from(models.values()).sort((a, b) => b.cost - a.cost);
}

interface AgentRow { name: string; role: string; provider: string; state: string; cpu: number; mem: number; tokens: number; cost: number }

function buildAgentTable(data: StatsData | null, sortKey: SortKey, sortAsc: boolean): AgentRow[] {
  if (!data) return [];
  const latest = new Map<string, AgentMetricTS>();
  for (const m of data.agentCpu) { if (!isInfra(m.agent_name)) latest.set(m.agent_name, m); }

  // Cost from time-range-filtered token metrics (not all-time costByAgent)
  const costMap = new Map<string, number>();
  for (const t of data.tokenMetrics) {
    costMap.set(t.agent_name, (costMap.get(t.agent_name) ?? 0) + calculateCost(t.model, t.input_tokens, t.output_tokens));
  }

  const tokenMap = new Map<string, number>();
  for (const t of data.tokenMetrics) tokenMap.set(t.agent_name, (tokenMap.get(t.agent_name) ?? 0) + t.input_tokens + t.output_tokens);

  const memLatest = new Map<string, number>();
  for (const m of data.agentMem) { if (!isInfra(m.agent_name)) memLatest.set(m.agent_name, m.mem_used_bytes / 1024 / 1024); }

  const rows: AgentRow[] = Array.from(latest.values()).map(m => ({
    name: m.agent_name, role: m.role, provider: m.tool || "unknown", state: m.state,
    cpu: m.cpu_percent, mem: memLatest.get(m.agent_name) ?? 0,
    tokens: tokenMap.get(m.agent_name) ?? 0, cost: costMap.get(m.agent_name) ?? 0,
  }));

  const dir = sortAsc ? 1 : -1;
  rows.sort((a, b) => {
    const av = a[sortKey], bv = b[sortKey];
    if (typeof av === "string" && typeof bv === "string") return av.localeCompare(bv) * dir;
    return ((av as number) - (bv as number)) * dir;
  });
  return rows;
}
