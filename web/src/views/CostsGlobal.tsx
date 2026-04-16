import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { formatCost } from "../utils/format";

/** Shape returned by GET /api/global/costs. */
interface GlobalCostsResponse {
  range: { start: string; end: string };
  groupBy: "workspace" | "project";
  rows: Array<{
    key: string;
    label: string;
    total: number;
    agentCount: number;
  }>;
}

type GroupBy = "workspace" | "project";

/** Format a USD total with thousands separators. Falls back to formatCost
 *  when the value is below the comma threshold. */
function formatCostWithCommas(n: number): string {
  if (!Number.isFinite(n)) return formatCost(0);
  if (n < 1000) return formatCost(n);
  return `$${n.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

/** YYYY-MM-DD for <input type="date"> values. */
function toDateInput(d: Date): string {
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function defaultRange(): { start: string; end: string } {
  const end = new Date();
  const start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000);
  return { start: toDateInput(start), end: toDateInput(end) };
}

export function CostsGlobal() {
  const navigate = useNavigate();
  const [{ start, end }, setRange] = useState<{ start: string; end: string }>(
    () => defaultRange(),
  );
  const [groupBy, setGroupBy] = useState<GroupBy>("workspace");
  const [data, setData] = useState<GlobalCostsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const qs = new URLSearchParams({
        start,
        end,
        groupBy,
      }).toString();
      const res = await fetch(`/api/global/costs?${qs}`);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}) as { error?: string });
        throw new Error(body.error || `HTTP ${res.status}`);
      }
      const json = (await res.json()) as GlobalCostsResponse;
      setData(json);
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load costs");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [start, end, groupBy]);

  useEffect(() => {
    void fetchData();
  }, [fetchData]);

  const total = useMemo(
    () => (data?.rows ?? []).reduce((acc, r) => acc + r.total, 0),
    [data],
  );

  return (
    <div className="p-6 space-y-4">
      {/* Header slot */}
      <header className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-lg font-semibold text-bc-text">
            Costs across workspaces
          </h1>
          <p className="text-xs text-bc-muted mt-0.5">
            Aggregate API spend across every registered bc workspace.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Date range */}
          <label className="flex items-center gap-1 text-xs text-bc-muted">
            <span>From</span>
            <input
              type="date"
              value={start}
              onChange={(e) => setRange((r) => ({ ...r, start: e.target.value }))}
              className="px-2 py-1 rounded border border-bc-border bg-bc-bg text-bc-text text-xs"
            />
          </label>
          <label className="flex items-center gap-1 text-xs text-bc-muted">
            <span>To</span>
            <input
              type="date"
              value={end}
              onChange={(e) => setRange((r) => ({ ...r, end: e.target.value }))}
              className="px-2 py-1 rounded border border-bc-border bg-bc-bg text-bc-text text-xs"
            />
          </label>

          {/* Group-by toggle */}
          <div
            role="group"
            aria-label="Group by"
            className="inline-flex rounded border border-bc-border overflow-hidden text-xs"
          >
            {(["workspace", "project"] as const).map((g) => (
              <button
                key={g}
                type="button"
                onClick={() => setGroupBy(g)}
                className={`px-3 py-1 transition-colors ${
                  groupBy === g
                    ? "bg-bc-accent/10 text-bc-accent"
                    : "text-bc-muted hover:text-bc-text"
                }`}
              >
                {g === "workspace" ? "Workspace" : "Project"}
              </button>
            ))}
          </div>
        </div>
      </header>

      {/* Table */}
      <section
        className="rounded border border-bc-border overflow-hidden"
        aria-busy={loading}
      >
        <div className="flex items-center justify-between px-4 py-2 border-b border-bc-border bg-bc-surface">
          <span className="text-xs text-bc-muted">
            {data
              ? `${data.rows.length} ${groupBy}${
                  data.rows.length === 1 ? "" : "s"
                }`
              : "\u00A0"}
          </span>
          <span className="text-xs tabular-nums text-bc-text">
            Total: {formatCostWithCommas(total)}
          </span>
        </div>

        {loading && <LoadingSkeleton rows={6} variant="table" />}

        {!loading && error && (
          <div className="p-4 text-sm text-red-500">Error: {error}</div>
        )}

        {!loading && !error && data && data.rows.length === 0 && (
          <EmptyState
            title="No cost records in range"
            description="Try expanding the date range or verify that workspaces are mirroring cost data to the global store."
          />
        )}

        {!loading && !error && data && data.rows.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-bc-muted">
                <th className="px-4 py-2 font-medium">
                  {groupBy === "workspace" ? "Workspace" : "Project"}
                </th>
                <th className="px-4 py-2 font-medium text-right">Agents</th>
                <th className="px-4 py-2 font-medium text-right">Total</th>
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row) => {
                const isWs = groupBy === "workspace";
                // Deep-link: workspace rows navigate into the per-workspace
                // costs page. Project rows are non-navigable (no per-project
                // route yet).
                const href = isWs
                  ? `/w/${encodeURIComponent(row.key)}/costs`
                  : null;
                const onClick = href ? () => navigate(href) : undefined;
                return (
                  <tr
                    key={row.key}
                    onClick={onClick}
                    className={`border-t border-bc-border/50 transition-colors ${
                      href
                        ? "hover:bg-bc-bg/40 cursor-pointer"
                        : ""
                    }`}
                  >
                    <td className="px-4 py-2">
                      <div className="font-medium text-bc-text truncate">
                        {row.label}
                      </div>
                      {row.label !== row.key && (
                        <div className="text-[10px] text-bc-muted/60 truncate">
                          {row.key}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-2 text-right tabular-nums text-bc-muted">
                      {row.agentCount}
                    </td>
                    <td className="px-4 py-2 text-right tabular-nums font-medium">
                      {formatCostWithCommas(row.total)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      {data?.range.start && data?.range.end && (
        <p className="text-[11px] text-bc-muted/60 text-right">
          Range: {data.range.start} &rarr; {data.range.end}
        </p>
      )}
    </div>
  );
}
