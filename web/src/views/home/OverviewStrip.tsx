import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import type { DailyCost, NotificationSource } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { formatCost } from "../../utils/format";
import { sourcePlatform } from "../../components/apps/messageUtils";
import { Spark, todayKey } from "./Module";

/* ── OverviewStrip ──────────────────────────────────────────────────
   The Home page's top band: five live counters in one hairline-divided
   strip — agents, apps, channels, today's spend, events/min. Each cell
   links to its owning surface. Agent counts and the event rate stream
   in over SSE (props); apps/channels poll at 30s and spend at 60s.
─────────────────────────────────────────────────────────────────── */

interface FleetSummary {
  working: number;
  idle: number;
  stuck: number;
  stopped: number;
  total: number;
}

/** Sample the SSE event counter every 10s into an events/min rate and a
 *  per-10s delta sparkline (client-side window, like insights SystemRow). */
export function useEventRate(eventCount: number): { rate: number; deltas: number[] } {
  const countRef = useRef(eventCount);
  countRef.current = eventCount;
  const [samples, setSamples] = useState<{ t: number; n: number }[]>([]);

  useEffect(() => {
    setSamples([{ t: Date.now(), n: countRef.current }]);
    const id = setInterval(() => {
      setSamples((prev) => {
        const now = Date.now();
        return [...prev, { t: now, n: countRef.current }].filter((s) => now - s.t <= 3 * 60_000);
      });
    }, 10_000);
    return () => clearInterval(id);
  }, []);

  const deltas: number[] = [];
  for (let i = 1; i < samples.length; i++) {
    deltas.push(Math.max(0, samples[i]!.n - samples[i - 1]!.n));
  }
  // Rate over the trailing minute, scaled to /min.
  const now = Date.now();
  const within = samples.filter((s) => now - s.t <= 60_000);
  let rate = 0;
  const first = within[0];
  const last = within[within.length - 1];
  if (first && last && last.t > first.t) {
    rate = Math.round(((last.n - first.n) / ((last.t - first.t) / 1000)) * 60);
  }
  return { rate, deltas };
}

function Cell({
  label,
  value,
  sub,
  to,
  spark,
  testId,
}: {
  label: string;
  value: React.ReactNode;
  sub?: React.ReactNode;
  to?: string;
  spark?: React.ReactNode;
  testId?: string;
}) {
  const body = (
    <>
      <span className="block text-[9.5px] font-medium text-mycel-muted uppercase tracking-[0.08em] truncate">
        {label}
      </span>
      <span className="mt-0.5 flex items-end justify-between gap-2 min-w-0">
        <span className="min-w-0">
          <span className="block text-[17px] leading-6 font-semibold tabular-nums text-mycel-text truncate">
            {value}
          </span>
          <span className="block text-[10.5px] text-mycel-muted truncate tabular-nums">{sub ?? " "}</span>
        </span>
        {spark && <span className="shrink-0 pb-0.5">{spark}</span>}
      </span>
    </>
  );
  const cls = "block bg-mycel-surface px-3 py-1.5 min-w-0 transition-colors";
  return to ? (
    <Link to={to} data-testid={testId} className={`${cls} hover:bg-mycel-surface-hover`}>
      {body}
    </Link>
  ) : (
    <div data-testid={testId} className={cls}>
      {body}
    </div>
  );
}

interface StripData {
  appsConnected: number;
  appsTotal: number;
  channels: number;
}

interface SpendData {
  today: number;
  avg7: number;
}

export function OverviewStrip({
  summary,
  eventCount,
  connected,
}: {
  summary: FleetSummary;
  eventCount: number;
  connected: boolean;
}) {
  const { rate, deltas } = useEventRate(eventCount);

  const appsFetcher = useCallback(async (): Promise<StripData> => {
    const [apps, sources] = await Promise.all([
      api.getApps().catch(() => null),
      api.listNotificationSources().catch(() => [] as NotificationSource[]),
    ]);
    const instances = apps?.instances ?? [];
    return {
      appsConnected: instances.filter((i) => i.enabled && i.connected).length,
      appsTotal: instances.length,
      channels: (sources ?? []).filter((s) => sourcePlatform(s.name) !== "internal").length,
    };
  }, []);
  const { data: apps } = usePolling(appsFetcher, 30_000);

  const spendFetcher = useCallback(async (): Promise<SpendData> => {
    const daily = await api.getCostDaily(7).catch(() => [] as DailyCost[]);
    const rows = Array.isArray(daily) ? daily : [];
    const tk = todayKey();
    const today = rows.find((d) => d.date === tk)?.cost_usd ?? 0;
    const avg7 = rows.reduce((s, d) => s + (d.cost_usd || 0), 0) / 7;
    return { today, avg7 };
  }, []);
  const { data: spend } = usePolling(spendFetcher, 60_000);

  const active = summary.working + summary.idle + summary.stuck;

  const agentSub: string[] = [];
  if (summary.working > 0) agentSub.push(`${summary.working} working`);
  if (summary.stuck > 0) agentSub.push(`${summary.stuck} stuck`);
  if (agentSub.length === 0) agentSub.push(summary.total > 0 ? "all quiet" : "none yet");

  return (
    <div
      data-testid="home-overview-strip"
      className="shrink-0 rounded-lg border border-mycel-border shadow-mycel-sm overflow-hidden"
    >
      {/* 5 cells: on 2-col mobile the last cell spans full width so we never
          leave an empty sixth slot; 3-col tablet is 2+2+1; desktop is one row. */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-px bg-mycel-border [&>*:last-child]:col-span-2 sm:[&>*:last-child]:col-span-1 lg:[&>*:last-child]:col-span-1">
        <Cell
          label="Agents"
          value={
            <>
              {active}
              <span className="text-mycel-muted font-normal text-[13px]"> / {summary.total}</span>
            </>
          }
          sub={agentSub.join(" · ")}
          to="/agents"
          testId="overview-agents"
        />
        <Cell
          label="Apps"
          value={apps ? apps.appsConnected : "—"}
          sub={apps ? `of ${apps.appsTotal} connected` : "loading"}
          to="/apps"
          testId="overview-apps"
        />
        <Cell
          label="Channels"
          value={apps ? apps.channels : "—"}
          sub={apps && apps.appsConnected > 0 ? `across ${apps.appsConnected} app${apps.appsConnected === 1 ? "" : "s"}` : undefined}
          to="/apps"
          testId="overview-channels"
        />
        <Cell
          label="Estimated spend today"
          value={spend ? formatCost(spend.today) : "—"}
          sub={spend && spend.avg7 > 0 ? `${formatCost(spend.avg7)}/day 7d avg` : undefined}
          to="/insights"
          testId="overview-spend"
        />
        <Cell
          label="Events"
          value={
            <span className={connected ? "" : "text-mycel-muted"}>
              {rate}
              <span className="text-mycel-muted font-normal text-[13px]">/min</span>
            </span>
          }
          sub={connected ? "live stream" : "stream offline"}
          spark={<Spark points={deltas.slice(-16)} />}
          testId="overview-events"
        />
      </div>
    </div>
  );
}
