import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { formatCost } from "../utils/format";
import { useHeaderSlot } from "../context/HeaderSlotContext";

type GroupBy = "workspace" | "project";

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
  const [groupBy, setGroupBy] = useState<GroupBy>("workspace");
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
    title: "Costs across workspaces",
    actions: (
      <div className="flex items-center gap-2 text-[12px]">
        <label className="text-bc-muted">Since</label>
        <input
          type="date"
          value={start}
          onChange={(e) => setStart(e.target.value)}
          className="bg-bc-bg border border-bc-border rounded px-2 py-1 text-bc-text outline-none focus:border-bc-accent"
        />
        <div className="flex rounded border border-bc-border overflow-hidden">
          {(["workspace", "project"] as const).map((g) => (
            <button
              key={g}
              type="button"
              onClick={() => setGroupBy(g)}
              className={`px-2.5 py-1 text-[11px] ${
                groupBy === g
                  ? "bg-bc-accent/15 text-bc-accent"
                  : "bg-bc-surface text-bc-muted hover:text-bc-text"
              }`}
            >
              {g === "workspace" ? "By workspace" : "By project"}
            </button>
          ))}
        </div>
      </div>
    ),
  });

  return (
    <div className="p-6 flex flex-col gap-4">
      {error && (
        <div className="rounded border border-bc-error/40 bg-bc-error/5 px-3 py-2 text-[12px] text-bc-error">
          {error}
        </div>
      )}

      <div className="flex items-baseline gap-3">
        <span className="text-[11px] uppercase tracking-wider text-bc-muted/60">Total</span>
        <span className="text-2xl font-semibold text-bc-text">{formatCost(total)}</span>
        <span className="text-[11px] text-bc-muted/60">
          since {start} · {rows?.length ?? 0} {groupBy === "workspace" ? "workspaces" : "projects"}
        </span>
      </div>

      <div className="rounded-md border border-bc-border/40 overflow-hidden">
        <table className="w-full text-[13px]">
          <thead className="bg-bc-surface/40 text-bc-muted">
            <tr>
              <th className="text-left font-normal px-3 py-2">
                {groupBy === "workspace" ? "Workspace" : "Project"}
              </th>
              <th className="text-right font-normal px-3 py-2">Cost</th>
              <th className="text-right font-normal px-3 py-2 w-24">Share</th>
            </tr>
          </thead>
          <tbody>
            {loading && !rows && (
              <tr>
                <td colSpan={3} className="px-3 py-6 text-center text-bc-muted text-[12px]">
                  Loading…
                </td>
              </tr>
            )}
            {!loading && rows && rows.length === 0 && (
              <tr>
                <td colSpan={3} className="px-3 py-6 text-center text-bc-muted text-[12px]">
                  No cost data in range.
                </td>
              </tr>
            )}
            {rows?.map((r) => {
              const share = total > 0 ? (r.total / total) * 100 : 0;
              const content = (
                <>
                  <td className="px-3 py-2 text-bc-text truncate max-w-[50%]">{r.label}</td>
                  <td className="px-3 py-2 text-right font-mono text-bc-text">{formatCost(r.total)}</td>
                  <td className="px-3 py-2 text-right text-bc-muted">{share.toFixed(1)}%</td>
                </>
              );
              // Only workspace rows deep-link; project grouping rolls up
              // several workspaces under one label and lacks a single target.
              if (groupBy === "workspace" && r.key !== "unattributed") {
                return (
                  <tr
                    key={r.key}
                    className="border-t border-bc-border/20 hover:bg-bc-surface/30 transition-colors"
                  >
                    <td colSpan={3} className="p-0">
                      <Link to={`/w/${r.key}/stats`} className="contents">
                        <table className="w-full"><tbody><tr>{content}</tr></tbody></table>
                      </Link>
                    </td>
                  </tr>
                );
              }
              return (
                <tr key={r.key} className="border-t border-bc-border/20">
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
