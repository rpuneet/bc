import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import { formatCost } from "../utils/format";
import { useHeaderSlot } from "../context/HeaderSlotContext";

type GroupBy = "repo" | "project";

interface CostRow {
  key: string;
  label: string;
  total: number;
}

// Default window: 30 days.
function defaultStart(): string {
  const d = new Date();
  d.setDate(d.getDate() - 30);
  return d.toISOString().slice(0, 10); // YYYY-MM-DD
}

export function CostsGlobal() {
  const [start, setStart] = useState<string>(defaultStart);
  const groupBy: GroupBy = "repo";
  const [rows, setRows] = useState<CostRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const total = useMemo(
    () => (rows ? rows.reduce((acc, r) => acc + r.total, 0) : 0),
    [rows],
  );

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    api
      .globalCosts({ start, groupBy })
      .then((resp) => {
        setRows(resp.rows);
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Failed to load costs");
        setRows([]);
      })
      .finally(() => setLoading(false));
  }, [start, groupBy]);

  useEffect(() => {
    load();
  }, [load]);

  useHeaderSlot({
    actions: (
      <div className="flex items-center gap-2 text-[12px]">
        <label className="text-mycel-muted">Since</label>
        <input
          type="date"
          value={start}
          onChange={(e) => setStart(e.target.value)}
          className="bg-mycel-bg border border-mycel-border rounded px-2 py-1 text-mycel-text outline-none focus:border-mycel-accent"
        />
        {/* "By project" toggle removed: project labels currently resolve to
            the same repo name, producing identical rows. Re-add when a
            distinct project axis exists in the data model. */}
      </div>
    ),
  });

  return (
    <div className="p-6 flex flex-col gap-4 max-w-4xl mx-auto">
      {error && (
        <div className="rounded border border-mycel-error bg-mycel-error-subtle px-3 py-2 text-[12px] text-mycel-error">
          {error}
        </div>
      )}

      {/* TOTAL block — label + big number reads as a headline row.
          Counter drops below the number as a caption so it doesn't
          crowd the value. Keeps the "grow into a chart" affordance
          open — the row can accept a donut / spark to its right
          later without another restructure. */}
      <div className="flex flex-col gap-1">
        <span className="text-[10px] font-medium uppercase tracking-[0.14em] text-mycel-muted">
          Total
        </span>
        <span className="text-[28px] font-semibold text-mycel-text leading-none tabular-nums">
          {formatCost(total)}
        </span>
        <span className="text-[11px] text-mycel-muted tabular-nums">
          {/* Date repeats the value in the picker top-right; show the
              counter only to avoid the DD/MM ↔ ISO format mismatch. */}
          {rows?.length ?? 0} {(rows?.length ?? 0) === 1 ? (groupBy === "repo" ? "repo" : "project") : (groupBy === "repo" ? "repos" : "projects")}
        </span>
      </div>

      <div className="rounded-md border border-mycel-border overflow-hidden shadow-mycel">
        <table className="w-full text-[13px]">
          <thead className="bg-mycel-surface text-mycel-muted">
            <tr>
              <th className="text-left font-normal px-3 py-2">
                {groupBy === "repo" ? "Repo" : "Project"}
              </th>
              <th className="text-right font-normal px-3 py-2">Cost</th>
              <th className="text-right font-normal px-3 py-2 w-24">Share</th>
            </tr>
          </thead>
          <tbody>
            {loading && !rows && (
              <tr>
                <td colSpan={3} className="px-3 py-6 text-center text-mycel-muted text-[12px]">
                  Loading…
                </td>
              </tr>
            )}
            {!loading && rows && rows.length === 0 && (
              <tr>
                <td colSpan={3} className="px-3 py-6 text-center text-mycel-muted text-[12px]">
                  No cost data in range.
                </td>
              </tr>
            )}
            {rows?.map((r) => {
              const share = total > 0 ? (r.total / total) * 100 : 0;
              const content = (
                <>
                  <td className="px-3 py-2 text-mycel-text truncate max-w-[50%]">{r.label}</td>
                  <td className="px-3 py-2 text-right font-mono text-mycel-text">{formatCost(r.total)}</td>
                  <td className="px-3 py-2 text-right text-mycel-muted">{share.toFixed(1)}%</td>
                </>
              );
              // Repo rows show the full path in the tooltip; project
              // grouping rolls several repos under one label.
              if (groupBy === "repo" && r.key !== "unattributed") {
                return (
                  <tr
                    key={r.key}
                    title={r.key}
                    className="border-t border-mycel-border hover:bg-mycel-surface-hover transition-colors"
                  >
                    {content}
                  </tr>
                );
              }
              return (
                <tr key={r.key} className="border-t border-mycel-border">
                  {content}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
