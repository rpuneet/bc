/**
 * Breakdown row drill-down — the inline detail that expands under a
 * "Where it goes" row.
 *
 * Depth follows what the ledger actually offers per dimension:
 * - agents: spend-over-time + share trend (lazy `/costs/agent/{id}`,
 *   last 30 ledger days ∩ selected period), period token split, and a
 *   link row (open agent, recent activity count).
 * - models: period token split, calls, per-call averages.
 * - repos: the underlying repo paths folded into this label.
 */

import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import { api } from "../../api/client";
import type {
  AgentCostDetail, AgentCostSummary, ModelCostSummary,
} from "../../api/client";
import { fmtTokens } from "../../components/shared/stats-primitives";
import { formatCost } from "../../utils/format";
import { ACCENT, TICK, AX, TT_STYLE, fmtShortDate } from "./chrome";
import { TokenCompositionStrip } from "./TokenPanel";

// ── Small pieces ────────────────────────────────────────────────────────────

function DetailStat({ label, value, sub }: { label: string; value: React.ReactNode; sub?: string }) {
  return (
    <div className="min-w-0">
      <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em] truncate">{label}</div>
      <div className="mt-0.5 text-base font-semibold tabular-nums text-mycel-text truncate">{value}</div>
      {sub && <div className="text-[11px] text-mycel-muted truncate">{sub}</div>}
    </div>
  );
}

function SectionLabel({ children, trailing }: { children: React.ReactNode; trailing?: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-2">
      <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">{children}</span>
      {trailing && <span className="text-[11px] text-mycel-muted tabular-nums">{trailing}</span>}
    </div>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-1 mb-1 rounded-md border border-mycel-border bg-mycel-surface p-4 space-y-4">
      {children}
    </div>
  );
}

// ── Agent detail ────────────────────────────────────────────────────────────

interface SpendSharePoint {
  date: string;
  cost: number;
  /** Share of the fleet's spend that day, 0–100. */
  share: number | null;
}

/** Join an agent's daily ledger against the fleet's for spend + share. */
export function buildAgentSpendSeries(
  detail: AgentCostDetail | null,
  fleetDaily: { date: string; cost_usd: number }[],
  days: string[],
): SpendSharePoint[] {
  const agentByDay = new Map((detail?.daily ?? []).map((d) => [d.date, d.cost_usd]));
  const fleetByDay = new Map(fleetDaily.map((d) => [d.date, d.cost_usd]));
  // The detail endpoint covers the last 30 ledger days — clamp the
  // window so longer periods don't render fabricated zeros.
  const first = detail?.daily?.[0]?.date;
  return days
    .filter((d) => first === undefined || d >= first)
    .map((date) => {
      const cost = agentByDay.get(date) ?? 0;
      const fleet = fleetByDay.get(date) ?? 0;
      return { date, cost, share: fleet > 0 ? (cost / fleet) * 100 : null };
    });
}

function AgentSpendChart({ series }: { series: SpendSharePoint[] }) {
  if (!series.some((p) => p.cost > 0)) {
    return <div className="py-6 text-center text-sm text-mycel-muted">No spend days in this window</div>;
  }
  return (
    <ResponsiveContainer width="100%" height={140}>
      <BarChart data={series} margin={{ top: 4, right: 8, left: 0, bottom: 0 }} barCategoryGap="18%">
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
          width={52}
          tickFormatter={(v: number) =>
            v === 0 ? "$0" : v >= 10 ? `$${Math.round(v).toLocaleString()}` : `$${v.toFixed(2)}`}
        />
        <Tooltip
          contentStyle={TT_STYLE}
          labelFormatter={(v) => fmtShortDate(String(v))}
          formatter={(v, name) =>
            name === "share"
              ? [`${Number(v ?? 0).toFixed(1)}%`, "Share of fleet"]
              : [formatCost(Number(v ?? 0)), "Spend"]}
        />
        <Bar dataKey="cost" name="cost" fill={ACCENT} fillOpacity={0.85} radius={[2, 2, 0, 0]} isAnimationActive={false} />
      </BarChart>
    </ResponsiveContainer>
  );
}

/** Tiny inline share-of-fleet trend (0–100%). */
function ShareSpark({ series }: { series: SpendSharePoint[] }) {
  const pts = series.filter((p) => p.share !== null);
  if (pts.length < 2) {
    return <div className="text-sm text-mycel-muted">Not enough spend days for a trend</div>;
  }
  const W = 120;
  const H = 24;
  const step = W / (pts.length - 1);
  const y = (v: number) => H - 2 - (Math.min(v, 100) / 100) * (H - 4);
  const line = pts.map((p, i) => `${(i * step).toFixed(1)},${y(p.share ?? 0).toFixed(1)}`).join(" ");
  const last = pts[pts.length - 1]?.share ?? 0;
  return (
    <span className="inline-flex items-center gap-2">
      <svg width={W} height={H} aria-hidden>
        <polyline points={line} fill="none" stroke="var(--mycel-chart-1)" strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
      </svg>
      <span className="text-[11px] text-mycel-muted tabular-nums">{last.toFixed(0)}% latest</span>
    </span>
  );
}

export function AgentDetail({
  agentId,
  name,
  summary,
  fleetDaily,
  days,
  activityCount,
}: {
  /** Namespaced ledger id, e.g. "mycel-a1b2c3-zen-zebra". */
  agentId: string;
  /** Bare display name. */
  name: string;
  /** Period-scoped ledger summary from the breakdown fetch. */
  summary: AgentCostSummary | undefined;
  fleetDaily: { date: string; cost_usd: number }[];
  /** Ascending day keys of the selected period. */
  days: string[];
  activityCount: number;
}) {
  const [detail, setDetail] = useState<AgentCostDetail | null>(null);
  const [failed, setFailed] = useState(false);
  const fetchedFor = useRef<string | null>(null);

  useEffect(() => {
    if (fetchedFor.current === agentId) return;
    fetchedFor.current = agentId;
    setDetail(null);
    setFailed(false);
    api.getCostAgentDetail(agentId).then(setDetail).catch(() => setFailed(true));
  }, [agentId]);

  const series = useMemo(
    () => buildAgentSpendSeries(detail, fleetDaily, days),
    [detail, fleetDaily, days],
  );
  const windowLabel = days.length > 30 ? "last 30 ledger days" : `last ${days.length}d`;

  return (
    <Shell>
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <DetailStat label="Spend" value={formatCost(summary?.total_cost_usd ?? 0)} sub="this period" />
        <DetailStat
          label="Tokens"
          value={fmtTokens(summary?.total_tokens ?? 0)}
          sub={`${fmtTokens(summary?.input_tokens ?? 0)} in · ${fmtTokens(summary?.output_tokens ?? 0)} out`}
        />
        <DetailStat
          label="Calls"
          value={(summary?.record_count ?? 0).toLocaleString()}
          sub={
            summary && summary.record_count > 0
              ? `${formatCost(summary.total_cost_usd / summary.record_count)}/call avg`
              : undefined
          }
        />
        <DetailStat label="Recent activity" value={activityCount.toLocaleString()} sub="events in the live feed" />
      </div>

      <div>
        <SectionLabel trailing={windowLabel}>Spend over time</SectionLabel>
        {failed ? (
          <div className="py-6 text-center text-sm text-mycel-muted">Could not load the agent ledger</div>
        ) : detail === null ? (
          <div className="py-6 text-center text-sm text-mycel-muted">Loading agent ledger…</div>
        ) : (
          <AgentSpendChart series={series} />
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
        <div className="space-y-2 lg:col-span-1">
          <SectionLabel>Token split · this period</SectionLabel>
          <TokenCompositionStrip
            input={summary?.input_tokens ?? 0}
            output={summary?.output_tokens ?? 0}
            read={summary?.cache_read_tokens ?? 0}
            write={summary?.cache_write_tokens ?? 0}
          />
        </div>
        <div className="space-y-2 lg:col-span-2">
          <SectionLabel>Share of fleet spend</SectionLabel>
          {detail === null || failed ? (
            <div className="text-sm text-mycel-muted">—</div>
          ) : (
            <ShareSpark series={series} />
          )}
          <div className="pt-2">
            <Link
              to={`/agents/${encodeURIComponent(name)}`}
              className="inline-flex items-center gap-1 text-xs text-mycel-accent hover:underline"
            >
              Open agent →
            </Link>
          </div>
        </div>
      </div>
    </Shell>
  );
}

// ── Model detail ────────────────────────────────────────────────────────────

export function ModelDetail({ model }: { model: ModelCostSummary | undefined }) {
  if (!model) return null;
  const calls = model.record_count;
  return (
    <Shell>
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <DetailStat label="Spend" value={formatCost(model.total_cost_usd)} sub="this period" />
        <DetailStat
          label="Tokens"
          value={fmtTokens(model.total_tokens)}
          sub={`${fmtTokens(model.input_tokens)} in · ${fmtTokens(model.output_tokens)} out`}
        />
        <DetailStat label="Calls" value={calls.toLocaleString()} />
        <DetailStat
          label="Per call"
          value={calls > 0 ? formatCost(model.total_cost_usd / calls) : "—"}
          sub={calls > 0 ? `${fmtTokens(Math.round(model.total_tokens / calls))} tokens avg` : undefined}
        />
      </div>
      <div className="space-y-2 max-w-md">
        <SectionLabel>Token split · this period</SectionLabel>
        <TokenCompositionStrip
          input={model.input_tokens}
          output={model.output_tokens}
          read={model.cache_read_tokens ?? 0}
          write={model.cache_write_tokens ?? 0}
        />
      </div>
    </Shell>
  );
}

// ── Repo detail ─────────────────────────────────────────────────────────────

export function RepoDetail({
  label,
  paths,
}: {
  label: string;
  /** All rollup rows folded under this label. */
  paths: { key: string; total: number }[];
}) {
  const total = paths.reduce((s, p) => s + p.total, 0);
  const sorted = [...paths].sort((a, b) => b.total - a.total);
  return (
    <Shell>
      <SectionLabel trailing={`${sorted.length} ${sorted.length === 1 ? "path" : "paths"}`}>
        Checkouts billed as “{label}”
      </SectionLabel>
      <div className="space-y-1">
        {sorted.map((p) => (
          <div key={p.key} className="flex items-center justify-between gap-3 text-xs">
            <span className="font-mono text-mycel-muted truncate">{p.key}</span>
            <span className="tabular-nums text-mycel-text shrink-0">
              {formatCost(p.total)}
              <span className="text-mycel-muted"> · {total > 0 ? ((p.total / total) * 100).toFixed(0) : 0}%</span>
            </span>
          </div>
        ))}
      </div>
      <p className="text-[11px] leading-relaxed text-mycel-muted">
        Agents work in per-agent worktrees, so one project can bill under
        several checkout paths — they fold into this row.
      </p>
    </Shell>
  );
}
