import { useCallback } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import type { SystemStats } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";

/* ── SystemPulse ────────────────────────────────────────────────────
   The tiniest module: the host machine's live CPU / memory / disk as
   inline text chips (not full tiles), polled from /api/stats/system on
   the same 10s cadence the Insights SystemRow uses. Links to Insights
   for the full drill-down.
─────────────────────────────────────────────────────────────────── */

const gb = (b: number) => `${(b / 1024 ** 3).toFixed(1)}G`;

function Chip({
  label,
  value,
  warn,
  title,
}: {
  label: string;
  value: string;
  warn?: boolean;
  /** Override hover detail (e.g. absolute MEM when chip shows %). */
  title?: string;
}) {
  return (
    <span
      className="inline-flex items-baseline gap-1 rounded-md border border-mycel-border bg-mycel-surface px-1.5 py-0.5"
      title={title ?? `${label}: ${value}`}
    >
      <span className="text-[9px] uppercase tracking-wide text-mycel-muted">{label}</span>
      <span className={`text-[11px] font-mono tabular-nums ${warn ? "text-mycel-warning" : "text-mycel-text"}`}>
        {value}
      </span>
    </span>
  );
}

export function SystemPulse() {
  const fetcher = useCallback(async (): Promise<SystemStats | null> => {
    const snap = await api.getStatsSystem().catch(() => null);
    return snap && typeof snap.cpu_usage_percent === "number" ? snap : null;
  }, []);
  const { data } = usePolling(fetcher, 10_000);

  return (
    <div data-testid="home-system-pulse" className="flex items-center gap-1.5 flex-wrap">
      <Link
        to="/insights"
        className="text-[9px] font-semibold uppercase tracking-widest text-mycel-muted hover:text-mycel-accent transition-colors"
      >
        Host
      </Link>
      {data ? (
        <>
          <Chip label="CPU" value={`${data.cpu_usage_percent.toFixed(0)}%`} warn={data.cpu_usage_percent >= 85} />
          <Chip
            label="MEM"
            value={`${data.memory_usage_percent.toFixed(0)}%`}
            warn={data.memory_usage_percent >= 90}
            title={`MEM: ${gb(data.memory_used_bytes)} / ${gb(data.memory_total_bytes)} (${data.memory_usage_percent.toFixed(0)}%)`}
          />
          <Chip label="DISK" value={`${data.disk_usage_percent.toFixed(0)}%`} warn={data.disk_usage_percent >= 90} />
        </>
      ) : (
        <span className="text-[10px] text-mycel-muted">host metrics unavailable</span>
      )}
    </div>
  );
}
