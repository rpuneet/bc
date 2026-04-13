import { useCallback, useMemo, useState } from "react";
import {
  AreaChart, Area, BarChart, Bar, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../api/client";
import type { Agent, AgentStatsSummary, AgentMetricTS, TokenMetricTS } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { calculateCost } from "../views/Stats";

// ── Constants ────────────────────────────────────────────────────────────────────

const COLORS = ["#FF6B35", "#3B82F6", "#10B981", "#A855F7", "#F59E0B", "#EC4899", "#06B6D4", "#84CC16"];
const RANGES = [
  { label: "1h", seconds: 3600 },
  { label: "6h", seconds: 21600 },
  { label: "12h", seconds: 43200 },
  { label: "24h", seconds: 86400 },
] as const;

const TT: React.CSSProperties = {
  backgroundColor: "var(--color-bc-surface)", border: "1px solid var(--color-bc-border)",
  borderRadius: "6px", color: "var(--color-bc-text)", fontSize: "12px",
};
const AX = { axisLine: false as const, tickLine: false as const };
const TICK = { fill: "var(--color-bc-muted)", fontSize: 10 };

// ── Helpers ──────────────────────────────────────────────────────────────────────

const fmtTime = (iso: string) => {
  try { return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }); }
  catch { return iso; }
};
const fmtBytes = (b: number) => {
  if (!b) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(b) / Math.log(1024));
  return `${(b / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};
const fmtMB = (b: number) => {
  if (!b || !isFinite(b)) return "0.0";
  return (b / 1024 / 1024).toFixed(1);
};
const fmtTokens = (n: number) => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
};
const trunc = (s: string, n: number) => s.length > n ? s.slice(0, n) + "\u2026" : s;
const fromParam = (seconds: number) => new Date(Date.now() - seconds * 1000).toISOString();

// ── Primitives ───────────────────────────────────────────────────────────────────

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded border border-bc-border bg-bc-surface overflow-hidden">
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-bc-border bg-bc-bg/50">
        <span className="text-[11px] font-medium text-bc-muted uppercase tracking-wider">{title}</span>
      </div>
      <div className="p-3">{children}</div>
    </div>
  );
}

function Empty({ msg = "No data yet" }: { msg?: string }) {
  return <div className="flex items-center justify-center h-[200px] text-sm text-bc-muted">{msg}</div>;
}

function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: boolean }) {
  return (
    <div className="rounded border border-bc-border bg-bc-surface p-3">
      <p className="text-[11px] text-bc-muted uppercase tracking-wider">{label}</p>
      <p className={`mt-1 text-xl font-bold ${accent ? "text-bc-accent" : ""}`}>{value}</p>
      {sub && <p className="text-[10px] text-bc-muted">{sub}</p>}
    </div>
  );
}

// ── Data types ───────────────────────────────────────────────────────────────────

interface TabData {
  summary: AgentStatsSummary | null;
  cpu: AgentMetricTS[];
  mem: AgentMetricTS[];
  net: AgentMetricTS[];
  tokens: TokenMetricTS[];
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
      api.getAgentTokenStats(p),
    ]);
    return {
      summary: r0.status === "fulfilled" ? r0.value : null,
      cpu: r1.status === "fulfilled" ? (r1.value ?? []) : [],
      mem: r2.status === "fulfilled" ? (r2.value ?? []) : [],
      net: r3.status === "fulfilled" ? (r3.value ?? []) : [],
      tokens: r4.status === "fulfilled" ? (r4.value ?? []) : [],
    };
  }, [agent.name, from]);

  const { data, loading } = usePolling(fetcher, 10000);
  const s = data?.summary;

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

  const tokenChart = useMemo(() => {
    const buckets = new Map<string, { time: string; input: number; output: number }>();
    for (const t of data?.tokens ?? []) {
      const k = fmtTime(t.time);
      const b = buckets.get(k) ?? { time: k, input: 0, output: 0 };
      b.input += t.input_tokens;
      b.output += t.output_tokens;
      buckets.set(k, b);
    }
    return Array.from(buckets.values());
  }, [data?.tokens]);

  const costBarData = useMemo(() =>
    (s?.cost_by_model ?? [])
      .map(m => ({
        name: trunc(m.model || "unknown", 24),
        cost: parseFloat(calculateCost(m.model, m.input_tokens, m.output_tokens).toFixed(4)),
      }))
      .filter(d => d.cost > 0)
      .sort((a, b) => b.cost - a.cost)
      .slice(0, 8),
    [s?.cost_by_model],
  );

  // ── Summary values ─────────────────────────────────────────────────────────

  const cpuAvg = isFinite(s?.cpu_avg ?? 0) ? (s?.cpu_avg ?? 0) : 0;
  const cpuMax = isFinite(s?.cpu_max ?? 0) ? (s?.cpu_max ?? 0) : 0;
  const memAvgMB = s ? parseFloat(fmtMB(s.mem_avg_bytes)) || 0 : 0;
  const memMaxMB = s ? parseFloat(fmtMB(s.mem_max_bytes)) || 0 : 0;
  const totalIn = s?.input_tokens ?? 0;
  const totalOut = s?.output_tokens ?? 0;
  const totalCost = isFinite(s?.total_cost_usd ?? 0) ? (s?.total_cost_usd ?? 0) : 0;

  // Has any live data? Used to show a helpful banner when the stats store
  // is empty (e.g. TimescaleDB not configured, or agent never ran).
  const hasAnyData =
    cpuChart.length > 0 ||
    memChart.length > 0 ||
    tokenChart.length > 0 ||
    totalIn > 0 ||
    totalOut > 0 ||
    totalCost > 0;
  const isStopped = agent.state === "stopped" || agent.state === "error";

  // ── Render ─────────────────────────────────────────────────────────────────

  if (loading && !data) {
    return <div className="p-4 text-sm text-bc-muted">Loading stats for {agent.name}...</div>;
  }

  return (
    <div className="space-y-4">
      {/* Empty-state banner when stats store is unreachable or agent never recorded data */}
      {!hasAnyData && !loading && (
        <div className="rounded border border-bc-border/60 bg-bc-surface/30 p-3 text-[11px] text-bc-muted/80 leading-relaxed">
          {isStopped ? (
            <>
              <span className="font-medium text-bc-muted">Stats unavailable for this agent.</span>{" "}
              No time-series data was recorded. This happens when the agent ran before the TimescaleDB stats collector was active, or when the agent was stopped too quickly for a sample to be captured.
            </>
          ) : (
            <>
              <span className="font-medium text-bc-muted">Waiting for metrics…</span>{" "}
              The TimescaleDB stats collector samples every 30 seconds. Live metrics will appear here once the first sample lands.
            </>
          )}
        </div>
      )}

      {/* Time range selector */}
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-bc-muted">{agent.name} <span className="text-bc-muted/60">({agent.role})</span></span>
        <div className="flex gap-1">
          {RANGES.map((r, i) => (
            <button key={r.label} type="button" onClick={() => setRange(i)}
              className={`px-2.5 py-1 text-xs rounded border transition-colors ${
                i === range
                  ? "border-bc-accent bg-bc-accent/10 text-bc-accent"
                  : "border-bc-border text-bc-muted hover:text-bc-text hover:border-bc-muted"
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
        <StatCard label="Cost" value={`$${totalCost.toFixed(2)}`} accent />
      </div>

      {/* Row 2: CPU + Memory charts */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="CPU (%)">
          {cpuChart.length < 2 ? <Empty /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={cpuChart} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-bc-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK} {...AX} />
                <YAxis tick={TICK} {...AX} tickFormatter={(v: number) => `${v}%`} />
                <Tooltip contentStyle={TT} />
                <Area type="monotone" dataKey="cpu" name="CPU %" stroke="#FF6B35" fill="#FF6B35" fillOpacity={0.12} strokeWidth={1.5} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Memory (MB)">
          {memChart.length < 2 ? <Empty /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={memChart} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-bc-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK} {...AX} />
                <YAxis tick={TICK} {...AX} />
                <Tooltip contentStyle={TT} formatter={(v) => [`${Number(v ?? 0).toFixed(1)} MB`]} />
                <Area type="monotone" dataKey="mem" name="Memory MB" stroke="#3B82F6" fill="#3B82F6" fillOpacity={0.12} strokeWidth={1.5} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
      </div>

      {/* Row 3: Network I/O + Token Usage */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Network I/O">
          {netChart.length < 2 ? <Empty /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={netChart} margin={{ top: 4, right: 8, left: -10, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-bc-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK} {...AX} />
                <YAxis tick={TICK} {...AX} tickFormatter={(v: number) => fmtBytes(v)} />
                <Tooltip contentStyle={TT} formatter={(v) => [fmtBytes(Number(v ?? 0))]} />
                <Area type="monotone" dataKey="rx" name="RX" stroke="#10B981" fill="#10B981" fillOpacity={0.12} strokeWidth={1.5} dot={false} />
                <Area type="monotone" dataKey="tx" name="TX" stroke="#FF6B35" fill="#FF6B35" fillOpacity={0.12} strokeWidth={1.5} dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="Token Usage">
          {tokenChart.length < 2 ? <Empty /> : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={tokenChart} margin={{ top: 4, right: 8, left: -8, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-bc-border)" vertical={false} />
                <XAxis dataKey="time" tick={TICK} {...AX} />
                <YAxis tick={TICK} {...AX} tickFormatter={(v: number) => fmtTokens(v)} />
                <Tooltip contentStyle={TT} formatter={(v, n) => [Number(v ?? 0).toLocaleString(), n === "input" ? "Input" : "Output"]} />
                <Area type="monotone" dataKey="input" name="Input" stroke="#3B82F6" fill="#3B82F6" fillOpacity={0.12} strokeWidth={1.5} stackId="1" dot={false} />
                <Area type="monotone" dataKey="output" name="Output" stroke="#FF6B35" fill="#FF6B35" fillOpacity={0.12} strokeWidth={1.5} stackId="1" dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </Panel>
      </div>

      {/* Row 4: Cost by Model + Tool Usage */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <Panel title="Cost by Model">
          {costBarData.length === 0 ? <Empty msg="No cost data" /> : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart layout="vertical" data={costBarData} margin={{ top: 0, right: 8, left: 4, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--color-bc-border)" horizontal={false} />
                <XAxis type="number" tick={TICK} {...AX} tickFormatter={(v: number) => `$${v}`} />
                <YAxis type="category" dataKey="name" tick={{ ...TICK, fill: "var(--color-bc-text)", fontSize: 9 }} {...AX} width={120} />
                <Tooltip contentStyle={TT} formatter={(v) => [`$${Number(v ?? 0).toFixed(4)}`]} />
                <Bar dataKey="cost" radius={[0, 3, 3, 0]}>
                  {costBarData.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </Panel>
        <Panel title="I/O Summary">
          {!s ? <Empty /> : (
            <div className="grid grid-cols-2 gap-3 py-4">
              <div className="text-center">
                <p className="text-[11px] text-bc-muted uppercase">Net RX</p>
                <p className="text-lg font-bold text-[#10B981]">{fmtBytes(s.net_rx_bytes)}</p>
              </div>
              <div className="text-center">
                <p className="text-[11px] text-bc-muted uppercase">Net TX</p>
                <p className="text-lg font-bold text-bc-accent">{fmtBytes(s.net_tx_bytes)}</p>
              </div>
              <div className="text-center">
                <p className="text-[11px] text-bc-muted uppercase">Disk Read</p>
                <p className="text-lg font-bold text-[#3B82F6]">{fmtBytes(s.disk_read_bytes)}</p>
              </div>
              <div className="text-center">
                <p className="text-[11px] text-bc-muted uppercase">Disk Write</p>
                <p className="text-lg font-bold text-[#A855F7]">{fmtBytes(s.disk_write_bytes)}</p>
              </div>
            </div>
          )}
        </Panel>
      </div>
    </div>
  );
}
