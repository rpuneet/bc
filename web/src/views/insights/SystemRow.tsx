/**
 * System row — the machine's vitals, restored from the old Stats page
 * as progressive disclosure: four compact live tiles (current value +
 * 15-min sparkline) that expand one at a time into a full drill-down.
 *
 * Data sources:
 * - `/api/stats/system` — live host snapshot (CPU %, memory, disk),
 *   sampled every SAMPLE_MS. The daemon keeps no host history, so the
 *   sparkline window is accumulated client-side and persisted in
 *   sessionStorage — it survives refresh and builds live while the
 *   page is open.
 * - `/api/agents/stats/{cpu,mem,net,disk}` — per-agent container/tmux
 *   series from the metrics store, lazily fetched when a drill-down
 *   opens. When the metrics store is offline the panel says so
 *   honestly instead of drawing an empty chart.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../../api/client";
import type { SystemStats, AgentMetricTS } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { fmtBytes } from "../../components/shared/stats-primitives";
import { ACCENT, TICK, AX, TT_STYLE, fmtClock } from "./chrome";
import { Disclosure, Chevron, useHashPanel } from "./disclosure";

// ── Sampling ────────────────────────────────────────────────────────────────

export const SAMPLE_MS = 10_000;
const WINDOW_MS = 15 * 60_000;
const STORE_KEY = "mycel:insights:sys-history:v1";

export interface SysSample {
  t: number;
  cpu: number;
  memPct: number;
  memUsed: number;
  memTotal: number;
  diskPct: number;
  diskUsed: number;
  diskTotal: number;
}

/** Append a snapshot to the rolling window (pure — exported for tests). */
export function pushSample(hist: SysSample[], snap: SystemStats, now: number): SysSample[] {
  const s: SysSample = {
    t: now,
    cpu: snap.cpu_usage_percent ?? 0,
    memPct: snap.memory_usage_percent ?? 0,
    memUsed: snap.memory_used_bytes ?? 0,
    memTotal: snap.memory_total_bytes ?? 0,
    diskPct: snap.disk_usage_percent ?? 0,
    diskUsed: snap.disk_used_bytes ?? 0,
    diskTotal: snap.disk_total_bytes ?? 0,
  };
  const cutoff = now - WINDOW_MS;
  const kept = hist.filter((p) => p.t >= cutoff && p.t < now);
  return [...kept, s];
}

function loadHistory(): SysSample[] {
  try {
    const raw = window.sessionStorage.getItem(STORE_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    const cutoff = Date.now() - WINDOW_MS;
    return (parsed as SysSample[]).filter((p) => typeof p?.t === "number" && p.t >= cutoff);
  } catch {
    return [];
  }
}

function saveHistory(hist: SysSample[]): void {
  try {
    window.sessionStorage.setItem(STORE_KEY, JSON.stringify(hist));
  } catch {
    // Quota/private-mode failures just lose the sparkline seed.
  }
}

// Module-level so navigating away from /insights and back within a
// session keeps the accumulated window.
let sysHistory: SysSample[] = [];
let sysHistoryLoaded = false;

interface SysData {
  snap: SystemStats | null;
  hist: SysSample[];
  netLatest: AgentMetricTS[];
}

function useSystemVitals(): SysData | null {
  const fetcher = useCallback(async (): Promise<SysData> => {
    if (!sysHistoryLoaded) {
      sysHistory = loadHistory();
      sysHistoryLoaded = true;
    }
    const [snapR, netR] = await Promise.allSettled([
      api.getStatsSystem(),
      api.getAgentStatsLatest(),
    ]);
    const snap =
      snapR.status === "fulfilled" && snapR.value && !Array.isArray(snapR.value)
        ? snapR.value
        : null;
    if (snap && typeof snap.cpu_usage_percent === "number") {
      sysHistory = pushSample(sysHistory, snap, Date.now());
      saveHistory(sysHistory);
    }
    return {
      snap,
      hist: sysHistory,
      netLatest: netR.status === "fulfilled" && Array.isArray(netR.value) ? netR.value : [],
    };
  }, []);
  const { data } = usePolling(fetcher, SAMPLE_MS);
  return data;
}

// ── Sparkline ───────────────────────────────────────────────────────────────

function Sparkline({ points, max }: { points: number[]; max?: number }) {
  const W = 88;
  const H = 26;
  if (points.length < 2) {
    return (
      <svg width={W} height={H} aria-hidden className="opacity-40">
        <line x1="0" y1={H - 1} x2={W} y2={H - 1} stroke="var(--mycel-border)" strokeWidth="1.5" strokeDasharray="2 3" />
      </svg>
    );
  }
  const hi = max ?? Math.max(...points, 1);
  const lo = 0;
  const step = W / (points.length - 1);
  const y = (v: number) => H - 2 - ((Math.min(v, hi) - lo) / (hi - lo || 1)) * (H - 4);
  const line = points.map((v, i) => `${(i * step).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
  const area = `0,${H} ${line} ${W},${H}`;
  return (
    <svg width={W} height={H} aria-hidden>
      <polygon points={area} fill={ACCENT} opacity="0.12" />
      <polyline points={line} fill="none" stroke={ACCENT} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

// ── Tiles ───────────────────────────────────────────────────────────────────

type SysKey = "cpu" | "memory" | "network" | "disk";

const TILES: { key: SysKey; label: string }[] = [
  { key: "cpu", label: "CPU" },
  { key: "memory", label: "Memory" },
  { key: "network", label: "Network" },
  { key: "disk", label: "Disk" },
];

function SysTile({
  label,
  value,
  sub,
  spark,
  sparkMax,
  open,
  onClick,
  tick,
}: {
  label: string;
  value: React.ReactNode;
  sub?: string;
  spark: number[];
  sparkMax?: number;
  open: boolean;
  onClick: () => void;
  tick: number;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-expanded={open}
      className={`bg-mycel-surface p-4 min-w-0 text-left cursor-pointer transition-colors hover:bg-mycel-surface-hover ${
        open ? "bg-mycel-surface-hover" : ""
      }`}
      title={`${open ? "Collapse" : "Expand"} ${label} detail`}
    >
      <div className="flex items-center gap-1.5">
        <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em] truncate">{label}</span>
        <Chevron open={open} />
      </div>
      <div className="mt-1 flex items-end justify-between gap-2">
        <div className="min-w-0">
          {/* Keyed by sample tick: each fresh sample re-runs the fade so
              the tile visibly breathes with live data. */}
          <div key={tick} className="text-xl font-semibold tabular-nums text-mycel-text truncate animate-[sysTick_400ms_ease]">
            {value}
          </div>
          <div className="mt-0.5 text-[11px] text-mycel-muted truncate">{sub ?? " "}</div>
        </div>
        <Sparkline points={spark} max={sparkMax} />
      </div>
    </button>
  );
}

// ── Per-agent drill-down series ─────────────────────────────────────────────

const METRIC_PATH: Record<SysKey, string> = { cpu: "cpu", memory: "mem", network: "net", disk: "disk" };

interface AgentSeries {
  agents: string[];
  data: Record<string, number | string>[];
}

/** Pivot per-agent samples into recharts rows keyed by time bucket. */
export function pivotAgentSeries(metrics: AgentMetricTS[], key: SysKey): AgentSeries {
  const value = (m: AgentMetricTS): number => {
    switch (key) {
      case "cpu": return Number(m.cpu_percent.toFixed(2));
      case "memory": return Number((m.mem_used_bytes / 1024 / 1024).toFixed(1));
      case "network": return m.net_rx_bytes + m.net_tx_bytes;
      case "disk": return m.disk_read_bytes + m.disk_write_bytes;
    }
  };
  const agents = [...new Set(metrics.map((m) => m.agent_name))];
  const rows = new Map<number, Record<string, number | string>>();
  for (const m of metrics) {
    const t = new Date(m.time).getTime();
    if (!Number.isFinite(t)) continue;
    const row = rows.get(t) ?? { t };
    row[m.agent_name] = value(m);
    rows.set(t, row);
  }
  const data = [...rows.values()].sort((a, b) => Number(a.t) - Number(b.t));
  // Backfill gaps with 0 so stacked areas don't collapse mid-window.
  for (const row of data) {
    for (const a of agents) if (row[a] === undefined) row[a] = 0;
  }
  return { agents, data };
}

const AGENT_COLORS = [
  ACCENT,
  "var(--mycel-chart-1)",
  "var(--mycel-chart-2)",
  "var(--mycel-chart-3)",
  "var(--mycel-chart-4)",
  "var(--mycel-chart-5)",
];

function useAgentSeries(key: SysKey | null): { series: AgentSeries | null; loading: boolean } {
  const [state, setState] = useState<{ key: SysKey; series: AgentSeries } | null>(null);
  const [loading, setLoading] = useState(false);
  const seq = useRef(0);
  useEffect(() => {
    if (key === null) return;
    const id = ++seq.current;
    setLoading(true);
    const from = new Date(Date.now() - 60 * 60_000).toISOString();
    api
      .getAgentStats(METRIC_PATH[key], { from })
      .then((metrics) => {
        if (seq.current !== id) return;
        setState({ key, series: pivotAgentSeries(metrics ?? [], key) });
      })
      .catch(() => {
        if (seq.current === id) setState({ key, series: { agents: [], data: [] } });
      })
      .finally(() => {
        if (seq.current === id) setLoading(false);
      });
  }, [key]);
  if (key === null || state?.key !== key) return { series: null, loading };
  return { series: state.series, loading };
}

// ── Drill-down panel ────────────────────────────────────────────────────────

// Axis ticks stay integers ("0%", "25%"); tooltips keep one decimal.
const pct = (v: number) => `${Number.isInteger(v) ? v : v.toFixed(1)}%`;

const PANEL_META: Record<SysKey, { title: string; unit: (v: number) => string }> = {
  cpu: { title: "CPU", unit: pct },
  memory: { title: "Memory", unit: (v) => `${v.toFixed(1)} GB` },
  network: { title: "Network", unit: (v) => fmtBytes(v) },
  disk: { title: "Disk", unit: pct },
};

/** HH:MM:SS — host samples land every 10s, so minute-only labels dupe. */
const fmtClockSec = (ms: number): string =>
  new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

function HostChart({ metric, hist }: { metric: SysKey; hist: SysSample[] }) {
  const data = useMemo(
    () =>
      hist.map((p) => ({
        t: p.t,
        v: metric === "cpu" ? p.cpu : metric === "memory" ? p.memUsed / 1024 ** 3 : p.diskPct,
      })),
    [hist, metric],
  );
  const meta = PANEL_META[metric];
  if (data.length < 2) {
    return (
      <div className="py-8 text-center text-sm text-mycel-muted">
        Building the live window — samples land every {SAMPLE_MS / 1000}s
      </div>
    );
  }
  const domain: [number, number | string] =
    metric === "cpu" || metric === "disk" ? [0, 100] : [0, "auto"];
  return (
    <ResponsiveContainer width="100%" height={180}>
      <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
        <CartesianGrid stroke="var(--mycel-border)" strokeOpacity={0.6} vertical={false} />
        <XAxis
          dataKey="t"
          tick={TICK}
          {...AX}
          tickFormatter={(v: number) => fmtClockSec(v)}
          interval="preserveStartEnd"
          minTickGap={88}
        />
        <YAxis tick={TICK} {...AX} width={48} domain={domain} tickFormatter={(v: number) => meta.unit(v)} />
        <Tooltip
          contentStyle={TT_STYLE}
          labelFormatter={(v) => fmtClockSec(Number(v))}
          formatter={(v) => [
            metric === "memory" ? `${Number(v ?? 0).toFixed(2)} GB` : `${Number(v ?? 0).toFixed(1)}%`,
            meta.title,
          ]}
        />
        <Area type="monotone" dataKey="v" stroke={ACCENT} fill={ACCENT} fillOpacity={0.12} strokeWidth={1.75} dot={false} isAnimationActive={false} />
      </AreaChart>
    </ResponsiveContainer>
  );
}

function AgentSplitChart({ metric, series }: { metric: SysKey; series: AgentSeries }) {
  const shown = series.agents.slice(0, 6);
  return (
    <>
      <div className="flex flex-wrap items-center gap-3 mb-1">
        {shown.map((a, i) => (
          <span key={a} className="inline-flex items-center gap-1.5 text-[11px] text-mycel-muted font-mono">
            <span className="w-2 h-2 rounded-sm" style={{ backgroundColor: AGENT_COLORS[i % AGENT_COLORS.length] }} />
            {a}
          </span>
        ))}
      </div>
      <ResponsiveContainer width="100%" height={160}>
        <AreaChart data={series.data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid stroke="var(--mycel-border)" strokeOpacity={0.6} vertical={false} />
          <XAxis
            dataKey="t"
            tick={TICK}
            {...AX}
            tickFormatter={(v: number) => fmtClock(v)}
            interval="preserveStartEnd"
            minTickGap={64}
          />
          <YAxis
            tick={TICK}
            {...AX}
            width={48}
            tickFormatter={(v: number) => (metric === "network" || metric === "disk" ? fmtBytes(v) : metric === "memory" ? `${v.toFixed(0)}M` : `${v}%`)}
          />
          <Tooltip
            contentStyle={TT_STYLE}
            labelFormatter={(v) => fmtClock(Number(v))}
            formatter={(v, name) => [
              metric === "cpu" ? `${Number(v ?? 0).toFixed(2)}%`
                : metric === "memory" ? `${Number(v ?? 0).toFixed(1)} MB`
                : fmtBytes(Number(v ?? 0)),
              String(name),
            ]}
          />
          {shown.map((a, i) => (
            <Area
              key={a}
              type="monotone"
              dataKey={a}
              stroke={AGENT_COLORS[i % AGENT_COLORS.length]}
              fill={AGENT_COLORS[i % AGENT_COLORS.length]}
              fillOpacity={0.08}
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>
    </>
  );
}

// ── Main row ────────────────────────────────────────────────────────────────

export function SystemRow() {
  const data = useSystemVitals();
  const [openKeyRaw, setOpen] = useHashPanel("sys");
  const openKey = (TILES.some((t) => t.key === openKeyRaw) ? openKeyRaw : null) as SysKey | null;
  const { series, loading: seriesLoading } = useAgentSeries(openKey);
  const rootRef = useRef<HTMLDivElement>(null);

  // Click-away closes the expanded panel (Esc lives in Disclosure).
  useEffect(() => {
    if (openKey === null) return;
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(null);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [openKey, setOpen]);

  const snap = data?.snap ?? null;
  const hist = data?.hist ?? [];
  const tick = hist[hist.length - 1]?.t ?? 0;

  const net = useMemo(() => {
    const latest = data?.netLatest ?? [];
    let rx = 0;
    let tx = 0;
    for (const m of latest) {
      rx += m.net_rx_bytes;
      tx += m.net_tx_bytes;
    }
    return { rx, tx, hasData: latest.length > 0 };
  }, [data?.netLatest]);

  const gb = (b: number) => `${(b / 1024 ** 3).toFixed(1)} GB`;

  return (
    <div ref={rootRef}>
      <style>{`@keyframes sysTick { from { opacity: 0.55; } to { opacity: 1; } }`}</style>
      <div className="rounded-lg border border-mycel-border shadow-mycel-sm overflow-hidden">
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-px bg-mycel-border">
          <SysTile
            label="CPU"
            value={snap ? `${snap.cpu_usage_percent.toFixed(1)}%` : "—"}
            sub={snap ? `${snap.cpus} cores` : undefined}
            spark={hist.map((p) => p.cpu)}
            sparkMax={100}
            open={openKey === "cpu"}
            onClick={() => setOpen("cpu")}
            tick={tick}
          />
          <SysTile
            label="Memory"
            value={snap ? gb(snap.memory_used_bytes) : "—"}
            sub={snap ? `${snap.memory_usage_percent.toFixed(0)}% of ${gb(snap.memory_total_bytes)}` : undefined}
            spark={hist.map((p) => p.memPct)}
            sparkMax={100}
            open={openKey === "memory"}
            onClick={() => setOpen("memory")}
            tick={tick}
          />
          <SysTile
            label="Network"
            value={net.hasData ? `${fmtBytes(net.rx)} ↓` : "—"}
            sub={net.hasData ? `${fmtBytes(net.tx)} ↑ · fleet, since start` : "no container metrics"}
            spark={[]}
            open={openKey === "network"}
            onClick={() => setOpen("network")}
            tick={tick}
          />
          <SysTile
            label="Disk"
            value={snap ? `${snap.disk_usage_percent.toFixed(0)}%` : "—"}
            sub={snap ? `${gb(snap.disk_used_bytes)} of ${gb(snap.disk_total_bytes)} used` : undefined}
            spark={hist.map((p) => p.diskPct)}
            sparkMax={100}
            open={openKey === "disk"}
            onClick={() => setOpen("disk")}
            tick={tick}
          />
        </div>

        <Disclosure open={openKey !== null} onClose={() => setOpen(null)} label="System detail">
          {openKey !== null && (
            <div className="border-t border-mycel-border bg-mycel-surface p-4 space-y-4">
              {/* Network has no host-level counters, so its panel is
                  per-agent only — no empty host section to scroll past. */}
              {openKey !== "network" && (
                <>
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
                      {PANEL_META[openKey].title} · host
                    </span>
                    <span className="text-[11px] text-mycel-muted tabular-nums">
                      last 15 min · sampled every {SAMPLE_MS / 1000}s
                    </span>
                  </div>
                  <HostChart metric={openKey} hist={hist} />
                </>
              )}

              <div className="flex items-baseline justify-between gap-2 pt-1">
                <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
                  {PANEL_META[openKey].title} · per agent
                </span>
                <span className="text-[11px] text-mycel-muted tabular-nums">last 1h · metrics store</span>
              </div>
              {seriesLoading && !series ? (
                <div className="py-6 text-center text-sm text-mycel-muted">Loading per-agent series…</div>
              ) : series && series.data.length > 0 ? (
                <AgentSplitChart metric={openKey} series={series} />
              ) : (
                <div className="py-6 text-center text-sm text-mycel-muted">
                  {openKey === "network"
                    ? "Network I/O is tracked per agent by the metrics store (bc-db) — it's offline or the fleet is idle"
                    : "No per-agent samples — the metrics store (bc-db) is offline or agents are idle"}
                </div>
              )}
            </div>
          )}
        </Disclosure>
      </div>
    </div>
  );
}
