import { useCallback, useMemo, useState } from "react";
import {
  AreaChart, Area,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../api/client";
import type { Agent, AgentStatsSummary, AgentMetricTS, ComputedStats } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { Panel, fmtTime, fmtBytes, fmtTokens } from "./shared/stats-primitives";

// ── Constants ────────────────────────────────────────────────────────────────────

// Chart palette — first entry follows the theme accent; the rest are
// the per-theme earthy chart tokens in canonical CVD-validated slot
// order (see Stats.tsx and theme/tokens.css).
const ACCENT = "var(--mycel-accent)";
const COLORS = [
  ACCENT,
  "var(--mycel-chart-1)", // spore blue
  "var(--mycel-chart-2)", // dusty rose
  "var(--mycel-chart-3)", // café gold
  "var(--mycel-chart-4)", // spore lavender
  "var(--mycel-chart-5)", // moss
  "var(--mycel-chart-6)", // lichen teal
  "var(--mycel-chart-7)", // olive
];
const RANGES = [
  { label: "1h", seconds: 3600 },
  { label: "6h", seconds: 21600 },
  { label: "12h", seconds: 43200 },
  { label: "24h", seconds: 86400 },
] as const;

const TT: React.CSSProperties = {
  backgroundColor: "var(--mycel-surface-2)", border: "1px solid var(--mycel-border)",
  borderRadius: "6px", color: "var(--mycel-text)", fontSize: "12px",
  boxShadow: "var(--mycel-shadow-lg)",
};
const AX = { axisLine: false as const, tickLine: false as const };
const TICK = { fill: "var(--mycel-text-2)", fontSize: 10 };

// ── Helpers ──────────────────────────────────────────────────────────────────────

const fmtMB = (b: number) => {
  if (!b || !isFinite(b)) return "0.0";
  return (b / 1024 / 1024).toFixed(1);
};
const fmtDiskBytes = (b: number): string => {
  if (!b || !isFinite(b) || b === 0) return "0 B";
  if (b >= 1024 * 1024 * 1024) return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`;
  if (b >= 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`;
  if (b >= 1024) return `${(b / 1024).toFixed(1)} KB`;
  return `${b} B`;
};
// Cost formatting delegates to the canonical util so comma grouping,
// sub-cent handling, and zero fallback stay consistent across Stats,
// StatsTab, and CostsGlobal.
import { formatCost } from "../utils/format";
const fmtCost = (v: number): string => {
  if (!isFinite(v)) return "$0.00";
  return formatCost(v);
};
const fromParam = (seconds: number) => new Date(Date.now() - seconds * 1000).toISOString();

// ── Primitives ───────────────────────────────────────────────────────────────────

function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: boolean }) {
  return (
    <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4">
      <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">{label}</p>
      <p className={`mt-1 text-xl font-bold tabular-nums ${accent ? "text-mycel-accent" : ""}`}>{value}</p>
      {sub && <p className="text-xs text-mycel-muted">{sub}</p>}
    </div>
  );
}

// ── Data types ───────────────────────────────────────────────────────────────────

interface TabData {
  summary: AgentStatsSummary | null;
  computed: ComputedStats | null;
  cpu: AgentMetricTS[];
  mem: AgentMetricTS[];
  net: AgentMetricTS[];
}

// ── Component ────────────────────────────────────────────────────────────────────

export function StatsTab({ agent }: { agent: Agent }) {
  const [range, setRange] = useState(0);
  const from = useMemo(() => fromParam(RANGES[range]?.seconds ?? 3600), [range]);

  const fetcher = useCallback(async (): Promise<TabData> => {
    const p = { from, agent: agent.name };
    const [r0, r1, r2, r3, r4] = await Promise.allSettled([
      api.getAgentStatsSummary(agent.name, { from }),
      api.getAgentStats("cpu", p),
      api.getAgentStats("mem", p),
      api.getAgentStats("net", p),
      api.getAgentComputedStats(agent.name),
    ]);
    return {
      summary: r0.status === "fulfilled" ? r0.value : null,
      cpu: r1.status === "fulfilled" ? (r1.value ?? []) : [],
      mem: r2.status === "fulfilled" ? (r2.value ?? []) : [],
      net: r3.status === "fulfilled" ? (r3.value ?? []) : [],
      computed: r4.status === "fulfilled" ? r4.value : null,
    };
  }, [agent.name, from]);

  const { data, loading } = usePolling(fetcher, 10000);
  const s = data?.summary;

  // ── Derived summary values — nested paths from backend AgentSummary struct ──

  // CPU/mem: use live ps-sampled data (accurate for tmux agents) when available.
  // Falls back to TimescaleDB summary only when live data is unavailable.
  const liveCPU = data?.computed?.cpu_percent ?? 0;
  const liveMemBytes = data?.computed?.mem_used_bytes ?? 0;
  const tsdbCPU = s?.cpu?.avg_percent ?? 0;
  const tsdbMem = s?.memory?.avg_bytes ?? 0;
  // Prefer live (more accurate — measures actual claude process via pane PID tree)
  const cpuAvg = liveCPU > 0 ? liveCPU : (isFinite(tsdbCPU) ? tsdbCPU : 0);
  const cpuMax = Math.max(s?.cpu?.max_percent ?? 0, cpuAvg);
  const memAvgMB = liveMemBytes > 0
    ? parseFloat(fmtMB(liveMemBytes)) || 0
    : (tsdbMem > 0 ? parseFloat(fmtMB(tsdbMem)) || 0 : 0);
  const memMaxMB = Math.max(
    s?.memory?.max_bytes ? parseFloat(fmtMB(s.memory.max_bytes)) || 0 : 0,
    memAvgMB
  );

  // Token totals: prefer TimescaleDB summary; fall back to cost-store computed stats; then agent record fields.
  const computedInputTokens = data?.computed?.input_tokens ?? 0;
  const computedOutputTokens = data?.computed?.output_tokens ?? 0;
  const totalIn = s?.tokens?.input
    ?? (computedInputTokens > 0 ? computedInputTokens : (agent.total_tokens ? Math.floor(agent.total_tokens * 0.8) : 0));
  const totalOut = s?.tokens?.output
    ?? (computedOutputTokens > 0 ? computedOutputTokens : (agent.total_tokens ? Math.floor(agent.total_tokens * 0.2) : 0));
  // Cost: prefer TimescaleDB summary; fall back to cost-store computed stats; then agent.cost_usd.
  const summaryTotalUSD = s?.cost?.total_usd ?? 0;
  const computedCostUSD = data?.computed?.cost_usd ?? 0;
  const totalCost = (s != null && isFinite(summaryTotalUSD) && summaryTotalUSD > 0)
    ? summaryTotalUSD
    : (isFinite(computedCostUSD) && computedCostUSD > 0)
      ? computedCostUSD
      : isFinite(agent.total_cost_usd ?? 0) ? (agent.total_cost_usd ?? 0) : 0;

  // ── Derived chart data ───────────────────────────────────────────────────────

  const cpuChart = useMemo(() =>
    (data?.cpu ?? []).map(m => ({ time: fmtTime(m.time), cpu: parseFloat(m.cpu_percent.toFixed(2)) })),
    [data?.cpu],
  );

  const memChart = useMemo(() =>
    (data?.mem ?? []).map(m => ({ time: fmtTime(m.time), mem: parseFloat(fmtMB(m.mem_used_bytes)) })),
    [data?.mem],
  );

  const netChart = useMemo(() =>
    (data?.net ?? []).map(m => ({ time: fmtTime(m.time), rx: m.net_rx_bytes, tx: m.net_tx_bytes })),
    [data?.net],
  );

  // Has any live data? Used to show a helpful banner when the stats store
  // is empty (e.g. TimescaleDB not configured, or agent never ran).
  const hasTimescaleData =
    cpuChart.length > 0 ||
    memChart.length > 0 ||
    (s?.cpu?.avg_percent ?? 0) > 0;

  const computed = data?.computed ?? null;
  const hasComputedData = (computed?.total_events ?? 0) > 0;

  // Live-sampled resource data (ps aux / container stats) — works without
  // TimescaleDB, so any "requires TimescaleDB" messaging must not sit on
  // top of CPU/Mem cards populated from this source.
  const hasLiveResource =
    (computed?.cpu_percent ?? 0) > 0 || (computed?.mem_used_bytes ?? 0) > 0;

  const hasAnyData =
    hasTimescaleData ||
    hasComputedData ||
    totalIn > 0 ||
    totalOut > 0 ||
    totalCost > 0;

  // Tool breakdown sorted by count descending, capped at 8 entries.
  const toolBreakdownData = useMemo(() => {
    if (!computed?.tool_breakdown) return [];
    return Object.entries(computed.tool_breakdown)
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 8);
  }, [computed?.tool_breakdown]);

  // Format session_duration_sec as a human-readable string.
  const fmtDuration = (secs: number): string => {
    if (!secs || secs <= 0) return "—";
    const h = Math.floor(secs / 3600);
    const m = Math.floor((secs % 3600) / 60);
    const s = secs % 60;
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  };

  // Format last_active as a relative time string.
  const fmtRelative = (iso: string): string => {
    if (!iso) return "—";
    const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
    if (diff < 60) return `${diff}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
  };

  const isStopped = agent.state === "stopped" || agent.state === "error";

  // ── Render ─────────────────────────────────────────────────────────────────

  if (loading && !data) {
    return <div className="p-4 text-sm text-mycel-muted">Loading stats for {agent.name}...</div>;
  }

  return (
    <div className="space-y-4">
      {/* Empty-state banner when stats store is unreachable or agent never recorded data */}
      {!hasAnyData && !loading && (
        <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4 text-xs text-mycel-muted leading-relaxed">
          {isStopped ? (
            <>
              <span className="font-medium text-mycel-muted">Stats unavailable for this agent.</span>{" "}
              No time-series data was recorded. This happens when the agent ran before the TimescaleDB stats collector was active, or when the agent was stopped too quickly for a sample to be captured.
            </>
          ) : (
            <>
              <span className="font-medium text-mycel-muted">Waiting for metrics…</span>{" "}
              The TimescaleDB stats collector samples every 30 seconds. Live metrics will appear here once the first sample lands.
            </>
          )}
        </div>
      )}

      {/* Hook-based stats — shown when TimescaleDB is empty but SQLite events exist */}
      {hasComputedData && !loading && (
        <div className="space-y-3">
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Activity (from hook events)</p>

          {/* Summary cards */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <StatCard label="Tool Calls" value={String(computed?.tool_calls ?? 0)} />
            <StatCard label="Total Events" value={String(computed?.total_events ?? 0)} />
            <StatCard label="Session Duration" value={fmtDuration(computed?.session_duration_sec ?? 0)} />
            <StatCard label="Last Active" value={fmtRelative(computed?.last_active ?? "")} />
            {(computed?.input_tokens ?? 0) > 0 && (
              <StatCard
                label="Tokens"
                value={fmtTokens((computed?.input_tokens ?? 0) + (computed?.output_tokens ?? 0))}
                sub={`In: ${fmtTokens(computed?.input_tokens ?? 0)} / Out: ${fmtTokens(computed?.output_tokens ?? 0)}`}
              />
            )}
            {(computed?.cost_usd ?? 0) > 0 && (
              <StatCard label="Cost" value={fmtCost(computed?.cost_usd ?? 0)} accent />
            )}
          </div>

          {/* Disk Usage and Notification Activity */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <StatCard
              label="Disk Usage"
              value={fmtDiskBytes(computed?.disk_bytes ?? 0)}
              sub="worktree size"
            />
            <StatCard
              label="Notifications Sent"
              value={String(computed?.channel_sent ?? 0)}
              sub="messages sent"
            />
            <StatCard
              label="Notifications Received"
              value={String(computed?.channel_received ?? 0)}
              sub="messages received"
            />
            <StatCard
              label="Network I/O"
              value="—"
              sub={computed?.network_note ?? "container runtime only"}
            />
          </div>

          {/* Tool breakdown */}
          {toolBreakdownData.length > 0 && (
            <div className="rounded-lg border border-mycel-border bg-mycel-surface p-4">
              <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted mb-2">Tool Breakdown</p>
              <div className="space-y-1.5">
                {toolBreakdownData.map(({ name: toolName, count }, i) => {
                  const maxCount = toolBreakdownData[0]?.count ?? 1;
                  const pct = Math.round((count / maxCount) * 100);
                  return (
                    <div key={toolName} className="flex items-center gap-2">
                      <span className="w-28 text-[11px] text-mycel-text truncate shrink-0 overflow-hidden text-ellipsis" title={toolName}>{toolName}</span>
                      <div className="flex-1 h-1.5 rounded-full bg-mycel-surface-hover overflow-hidden">
                        <div
                          className="h-full rounded-full"
                          style={{ width: `${pct}%`, backgroundColor: COLORS[i % COLORS.length] }}
                        />
                      </div>
                      <span className="text-[11px] text-mycel-muted w-6 text-right shrink-0">{count}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Time range selector */}
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-mycel-muted">{agent.name}</span>
        <div className="flex gap-1">
          {RANGES.map((r, i) => (
            <button key={r.label} type="button" onClick={() => setRange(i)}
              className={`px-2.5 py-1 text-xs font-medium rounded-md border transition-colors ${
                i === range
                  ? "border-mycel-accent text-mycel-accent"
                  : "border-mycel-border text-mycel-muted hover:text-mycel-text hover:border-mycel-muted"
              }`}
            >{r.label}</button>
          ))}
        </div>
      </div>

      {/* Row 1: Summary Cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <StatCard label="CPU" value={`${cpuAvg.toFixed(1)}%`} sub={`max ${cpuMax.toFixed(1)}%`} />
        <StatCard label="Memory" value={`${memAvgMB} MB`} sub={`max ${memMaxMB} MB`} />
        <StatCard label="Tokens" value={fmtTokens(totalIn + totalOut)} sub={`In: ${fmtTokens(totalIn)} / Out: ${fmtTokens(totalOut)}`} />
        <StatCard label="Cost" value={fmtCost(totalCost)} accent />
      </div>

      {/* TimescaleDB notice — scoped to the historical charts section only.
          The live sampler feeds the CPU/Mem cards above without TSDB, so
          this must never sit on top of populated cards. */}
      {!hasTimescaleData && hasAnyData && !loading && (
        <div className="rounded-lg border border-mycel-border bg-mycel-surface p-2.5 text-xs text-mycel-muted leading-relaxed">
          <span className="font-medium">Historical CPU/Memory charts require TimescaleDB.</span>{" "}
          {hasLiveResource
            ? "The CPU and Memory cards above show live sampled values."
            : "Showing token and cost data from agent session logs."}
        </div>
      )}

      {/* Row 2: CPU + Memory charts — only shown when TSDB has data */}
      {(cpuChart.length >= 2 || memChart.length >= 2) && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {cpuChart.length >= 2 && (
            <Panel title="CPU (%)">
              <ResponsiveContainer width="100%" height={200}>
                <AreaChart data={cpuChart} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                  <XAxis dataKey="time" tick={TICK} {...AX} />
                  <YAxis tick={TICK} {...AX} tickFormatter={(v: number) => `${v}%`} />
                  <Tooltip contentStyle={TT} />
                  <Area type="monotone" dataKey="cpu" name="CPU %" stroke={ACCENT} fill={ACCENT} fillOpacity={0.12} strokeWidth={1.5} dot={false} />
                </AreaChart>
              </ResponsiveContainer>
            </Panel>
          )}
          {memChart.length >= 2 && (
            <Panel title="Memory (MB)">
              <ResponsiveContainer width="100%" height={200}>
                <AreaChart data={memChart} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                  <XAxis dataKey="time" tick={TICK} {...AX} />
                  <YAxis tick={TICK} {...AX} />
                  <Tooltip contentStyle={TT} formatter={(v) => [`${Number(v ?? 0).toFixed(1)} MB`]} />
                  <Area type="monotone" dataKey="mem" name="Memory MB" stroke="var(--mycel-chart-1)" fill="var(--mycel-chart-1)" fillOpacity={0.12} strokeWidth={1.5} dot={false} />
                </AreaChart>
              </ResponsiveContainer>
            </Panel>
          )}
        </div>
      )}

      {/* Row 3: Network I/O — only shown when data exists */}
      {netChart.length >= 2 && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <Panel title="Network I/O">
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={netChart} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--mycel-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK} {...AX} />
                <YAxis tick={TICK} {...AX} tickFormatter={(v: number) => fmtBytes(v)} />
                <Tooltip contentStyle={TT} formatter={(v) => [fmtBytes(Number(v ?? 0))]} />
                <Area type="monotone" dataKey="rx" name="RX" stroke="var(--mycel-chart-5)" fill="var(--mycel-chart-5)" fillOpacity={0.12} strokeWidth={1.5} dot={false} />
                <Area type="monotone" dataKey="tx" name="TX" stroke={ACCENT} fill={ACCENT} fillOpacity={0.12} strokeWidth={1.5} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          </Panel>
        </div>
      )}

      {/* Row 4: I/O Summary — only shown when data exists */}
      {(() => {
        const hasIoData = s && (
          (s.network?.rx_bytes ?? 0) > 0 ||
          (s.network?.tx_bytes ?? 0) > 0 ||
          (s.disk?.read_bytes ?? 0) > 0 ||
          (s.disk?.write_bytes ?? 0) > 0
        );
        if (!hasIoData || !s) return null;
        return (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <Panel title="I/O Summary">
                <div className="grid grid-cols-2 gap-3 py-4">
                  <div className="text-center">
                    <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Net RX</p>
                    <p className="text-lg font-bold tabular-nums text-mycel-chart-5">{fmtBytes(s.network?.rx_bytes ?? 0)}</p>
                  </div>
                  <div className="text-center">
                    <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Net TX</p>
                    <p className="text-lg font-bold tabular-nums text-mycel-accent">{fmtBytes(s.network?.tx_bytes ?? 0)}</p>
                  </div>
                  <div className="text-center">
                    <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Disk Read</p>
                    <p className="text-lg font-bold tabular-nums text-mycel-chart-1">{fmtBytes(s.disk?.read_bytes ?? 0)}</p>
                  </div>
                  <div className="text-center">
                    <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Disk Write</p>
                    <p className="text-lg font-bold tabular-nums text-mycel-chart-4">{fmtBytes(s.disk?.write_bytes ?? 0)}</p>
                  </div>
                </div>
            </Panel>
          </div>
        );
      })()}
    </div>
  );
}
