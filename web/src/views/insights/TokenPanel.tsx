/**
 * Token drill-down — expands from the Tokens stat in the stat band.
 *
 * Composition over time comes from the daily ledger (input/output per
 * UTC day, stacked); cache tokens are only reported as period totals
 * by the ledger, so they render as a period composition strip beside
 * the chart rather than a fabricated time series.
 */

import { useMemo } from "react";
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from "recharts";
import type { CostSummary, DailyCost } from "../../api/client";
import { fmtTokens } from "../../components/shared/stats-primitives";
import { ACCENT, TICK, AX, TT_STYLE, fmtShortDate } from "./chrome";
import type { ChartTooltipProps } from "./chrome";

export interface TokenPoint {
  date: string;
  input: number;
  output: number;
}

/** Daily input/output series over an explicit day window (zero-filled). */
export function buildTokenSeries(daily: DailyCost[], days: string[]): TokenPoint[] {
  const byDay = new Map(daily.map((d) => [d.date, d]));
  return days.map((date) => {
    const row = byDay.get(date);
    return { date, input: row?.input_tokens ?? 0, output: row?.output_tokens ?? 0 };
  });
}

const SERIES = [
  { key: "input", label: "Input", color: "var(--mycel-chart-1)" },
  { key: "output", label: "Output", color: ACCENT },
] as const;

function TokenTooltip({ active, payload, label }: ChartTooltipProps) {
  if (!active || !payload?.length) return null;
  const total = payload.reduce((s, p) => s + (Number(p.value) || 0), 0);
  return (
    <div style={TT_STYLE} className="px-3 py-2">
      <div className="text-[11px] text-mycel-muted">{fmtShortDate(String(label))}</div>
      <div className="font-semibold tabular-nums">{fmtTokens(total)} tokens</div>
      {SERIES.map((s) => {
        const p = payload.find((x) => x.dataKey === s.key);
        const v = Number(p?.value) || 0;
        return (
          <div key={s.key} className="flex items-center gap-1.5 text-[11px] text-mycel-muted tabular-nums">
            <span className="w-2 h-2 rounded-sm" style={{ backgroundColor: s.color }} />
            {s.label}: {fmtTokens(v)}
          </div>
        );
      })}
    </div>
  );
}

/** Segmented all-tokens bar + legend, shared with breakdown drill-downs. */
export function TokenCompositionStrip({
  input,
  output,
  read,
  write,
}: {
  input: number;
  output: number;
  read: number;
  write: number;
}) {
  const all = input + output + read + write;
  if (all === 0) return <div className="text-sm text-mycel-muted">No token data</div>;
  const rows = [
    { k: "Cache reads", v: read, c: "var(--mycel-chart-5)" },
    { k: "Cache writes", v: write, c: "var(--mycel-chart-3)" },
    { k: "Fresh input", v: input, c: "var(--mycel-chart-1)" },
    { k: "Output", v: output, c: ACCENT },
  ];
  return (
    <>
      <div
        className="flex h-2 rounded-full overflow-hidden bg-mycel-border/40"
        role="img"
        aria-label="Share of processed tokens by kind"
      >
        {rows.map((seg) =>
          seg.v > 0 ? (
            <span key={seg.k} style={{ width: `${(seg.v / all) * 100}%`, backgroundColor: seg.c }} />
          ) : null,
        )}
      </div>
      <dl className="space-y-1 text-[11px]">
        {rows.map((r) => (
          <div key={r.k} className="flex items-center justify-between gap-2">
            <dt className="flex items-center gap-1.5 text-mycel-muted">
              <span className="w-2 h-2 rounded-sm" style={{ backgroundColor: r.c }} />
              {r.k}
            </dt>
            <dd className="tabular-nums text-mycel-text">
              {fmtTokens(r.v)}
              <span className="text-mycel-muted"> · {((r.v / all) * 100).toFixed(1)}%</span>
            </dd>
          </div>
        ))}
      </dl>
    </>
  );
}

export function TokenPanel({
  series,
  summary,
  periodLabel,
}: {
  series: TokenPoint[];
  summary: CostSummary | null;
  periodLabel: string;
}) {
  const comp = useMemo(() => {
    const input = summary?.input_tokens ?? 0;
    const output = summary?.output_tokens ?? 0;
    const read = summary?.cache_read_tokens ?? 0;
    const write = summary?.cache_write_tokens ?? 0;
    const all = input + output + read + write;
    return { input, output, read, write, all };
  }, [summary]);

  const hasSeries = series.some((p) => p.input + p.output > 0);

  return (
    <div className="border-t border-mycel-border bg-mycel-surface p-4">
      <div className="flex items-baseline justify-between gap-2 mb-3">
        <span className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
          Token composition · {periodLabel}
        </span>
        <span className="text-[11px] text-mycel-muted tabular-nums">daily, UTC</span>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
        <div className="lg:col-span-2">
          {!hasSeries ? (
            <div className="py-10 text-center text-sm text-mycel-muted">No token ledger in this period</div>
          ) : (
            <>
              <div className="flex items-center gap-3 mb-2">
                {SERIES.map((s) => (
                  <span key={s.key} className="inline-flex items-center gap-1.5 text-[11px] text-mycel-muted">
                    <span className="w-2 h-2 rounded-sm" style={{ backgroundColor: s.color }} />
                    {s.label}
                  </span>
                ))}
              </div>
              <ResponsiveContainer width="100%" height={180}>
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
                  <YAxis tick={TICK} {...AX} width={48} tickFormatter={(v: number) => fmtTokens(v)} />
                  <Tooltip content={<TokenTooltip />} cursor={{ fill: "var(--mycel-surface-hover)", fillOpacity: 0.5 }} />
                  {SERIES.map((s, i) => (
                    <Bar
                      key={s.key}
                      dataKey={s.key}
                      name={s.label}
                      stackId="tok"
                      fill={s.color}
                      stroke="var(--mycel-surface)"
                      strokeWidth={1}
                      radius={i === SERIES.length - 1 ? [2, 2, 0, 0] : undefined}
                      isAnimationActive={false}
                    />
                  ))}
                </BarChart>
              </ResponsiveContainer>
            </>
          )}
        </div>

        {/* Every token the fleet processed this period, by kind. The
            ledger reports cache tokens only as period totals — shown
            here as composition, not a made-up time series. */}
        <div className="space-y-2">
          <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em]">
            All tokens processed
          </div>
          {comp.all === 0 ? (
            <div className="text-sm text-mycel-muted">No token data</div>
          ) : (
            <>
              <TokenCompositionStrip input={comp.input} output={comp.output} read={comp.read} write={comp.write} />
              <p className="text-[11px] leading-relaxed text-mycel-muted">
                The chart stacks billable input/output per day; cached
                context is reported by the ledger as period totals only.
              </p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
