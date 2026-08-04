import { useCallback, useMemo } from "react";
import {
  Bar, BarChart, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from "recharts";
import { api } from "../../api/client";
import type { AgentCostSummary, DailyCost } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { formatCost, costSpendLabel } from "../../utils/format";
import { ACCENT, AX, TICK, TT_STYLE, fmtShortDate, stripAgentPrefix } from "../insights/chrome";
import { HomeModule, todayKey } from "./Module";

/* ── CostCharts ─────────────────────────────────────────────────────
   Two small live cost modules for the Home right rail, sharing the
   Insights chart tokens and honest-chart rules (zero-filled days, no
   smoothing, today emphasized):

     • Estimated spend — last 7 days, daily bars (UTC ledger days).
     • Top agents — today's top-5 spenders as a proportional bar list.

   Dollars are model-table priced (not provider billing). Data refreshes
   on a 60s interval via the existing cost endpoints (`/costs/daily`,
   `/costs/agents?since=<today>`).
─────────────────────────────────────────────────────────────────── */

const DAY_MS = 86_400_000;

interface CostData {
  daily: DailyCost[];
  topAgents: AgentCostSummary[];
}

/** Last 7 UTC day keys including today, oldest first. */
function last7Days(): string[] {
  const now = Date.now();
  const out: string[] = [];
  for (let i = 6; i >= 0; i--) out.push(new Date(now - i * DAY_MS).toISOString().slice(0, 10));
  return out;
}

export function CostCharts() {
  const fetcher = useCallback(async (): Promise<CostData> => {
    const since = todayKey();
    const [daily, byAgent] = await Promise.allSettled([
      api.getCostDaily(7),
      api.getCostByAgent({ since, limit: 5 }),
    ]);
    return {
      daily: daily.status === "fulfilled" && Array.isArray(daily.value) ? daily.value : [],
      topAgents:
        byAgent.status === "fulfilled" && Array.isArray(byAgent.value)
          ? byAgent.value.filter((a) => typeof a.agent_id === "string" && typeof a.total_cost_usd === "number")
          : [],
    };
  }, []);
  const { data } = usePolling(fetcher, 60_000);

  const tk = todayKey();

  const series = useMemo(() => {
    const byDay = new Map((data?.daily ?? []).map((d) => [d.date, d]));
    return last7Days().map((date) => ({ date, cost: byDay.get(date)?.cost_usd ?? 0 }));
  }, [data?.daily]);

  const top = useMemo(
    () => [...(data?.topAgents ?? [])].sort((a, b) => b.total_cost_usd - a.total_cost_usd).slice(0, 5),
    [data?.topAgents],
  );
  const maxAgent = top[0]?.total_cost_usd ?? 0;
  const hasSpend = series.some((d) => d.cost > 0);

  return (
    <>
      <HomeModule
        label={`${costSpendLabel()} · 7 days`}
        to="/insights"
        toLabel="insights"
        testId="home-cost-daily"
        trailing={<span className="text-[10px] text-mycel-muted tabular-nums">daily · UTC · estimated</span>}
      >
        {data === null ? (
          <div className="py-4 text-center text-[11px] text-mycel-muted">Loading…</div>
        ) : !hasSpend ? (
          <div className="py-4 text-center text-[11px] text-mycel-muted">No spend in the last 7 days</div>
        ) : (
          <ResponsiveContainer width="100%" height={96}>
            <BarChart data={series} margin={{ top: 4, right: 0, left: 0, bottom: 0 }} barCategoryGap="22%">
              <XAxis
                dataKey="date"
                tick={{ ...TICK, fontSize: 9 }}
                {...AX}
                tickFormatter={fmtShortDate}
                interval="preserveStartEnd"
                minTickGap={24}
              />
              <YAxis
                tick={{ ...TICK, fontSize: 9 }}
                {...AX}
                width={36}
                tickFormatter={(v: number) =>
                  v === 0 ? "$0" : v >= 10 ? `$${Math.round(v)}` : `$${v.toFixed(2)}`}
              />
              <Tooltip
                contentStyle={TT_STYLE}
                cursor={{ fill: "var(--mycel-surface-hover)", fillOpacity: 0.5 }}
                labelFormatter={(v) => fmtShortDate(String(v))}
                formatter={(v) => [formatCost(Number(v ?? 0)), costSpendLabel()]}
              />
              <Bar dataKey="cost" radius={[2, 2, 0, 0]} isAnimationActive={false}>
                {series.map((d) => (
                  <Cell key={d.date} fill={ACCENT} fillOpacity={d.date === tk ? 1 : 0.55} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}
      </HomeModule>

      <HomeModule label="Top agents · today" to="/insights" toLabel="insights" testId="home-cost-agents">
        {data === null ? (
          <div className="py-4 text-center text-[11px] text-mycel-muted">Loading…</div>
        ) : top.length === 0 ? (
          <div className="py-4 text-center text-[11px] text-mycel-muted">No spend today</div>
        ) : (
          <ul className="space-y-1.5">
            {top.map((a) => (
              <li key={a.agent_id} className="min-w-0">
                <div className="flex items-baseline justify-between gap-2 min-w-0">
                  <span className="text-[11px] font-mono text-mycel-text-2 truncate" title={a.agent_id}>
                    {stripAgentPrefix(a.agent_id)}
                  </span>
                  <span className="shrink-0 text-[11px] tabular-nums text-mycel-text">
                    {formatCost(a.total_cost_usd)}
                  </span>
                </div>
                <div className="mt-[3px] h-1 rounded-full bg-mycel-bg overflow-hidden">
                  <div
                    className="h-full rounded-full bg-mycel-accent"
                    style={{ width: `${maxAgent > 0 ? Math.max(2, (a.total_cost_usd / maxAgent) * 100) : 0}%` }}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </HomeModule>
    </>
  );
}
