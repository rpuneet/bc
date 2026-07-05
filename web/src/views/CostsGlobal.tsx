import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { CostSummary, AgentCostSummary, ModelCostSummary } from "../api/client";
import { formatCost, formatTokens } from "../utils/format";
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

/** Small stat cell for the token/cost summary strip. */
function Stat({ label, value, title }: { label: string; value: string; title?: string }) {
  return (
    <div className="flex flex-col gap-0.5" title={title}>
      <span className="text-[10px] font-medium uppercase tracking-[0.14em] text-mycel-muted">
        {label}
      </span>
      <span className="text-[18px] font-semibold text-mycel-text leading-tight tabular-nums">
        {value}
      </span>
    </div>
  );
}

const thLeft = "text-left font-normal px-3 py-2";
const thRight = "text-right font-normal px-3 py-2";
const tdMono = "px-3 py-2 text-right font-mono";

export function CostsGlobal() {
  const [start, setStart] = useState<string>(defaultStart);
  const groupBy: GroupBy = "repo";
  const [rows, setRows] = useState<CostRow[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  // Ledger summaries — costs are computed FROM tokens, so token usage is
  // shown alongside every dollar figure. These endpoints (/costs,
  // /costs/agents, /costs/models) carry input/output token fields; the
  // cross-repo rollup (/api/global/costs) does NOT expose tokens per
  // repo row, so the repo table stays dollars-only (noted, not an API
  // change in this pass).
  const [summary, setSummary] = useState<CostSummary | null>(null);
  const [byAgent, setByAgent] = useState<AgentCostSummary[]>([]);
  const [byModel, setByModel] = useState<ModelCostSummary[]>([]);

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

  // Token-bearing ledger summaries load independently of the repo rollup —
  // a failure here degrades to the dollars-only view instead of erroring.
  useEffect(() => {
    let cancelled = false;
    void Promise.allSettled([
      api.getCostSummary(),
      api.getCostByAgent(),
      api.getCostByModel(),
    ]).then(([s, a, m]) => {
      if (cancelled) return;
      if (s.status === "fulfilled") setSummary(s.value);
      if (a.status === "fulfilled") setByAgent(a.value ?? []);
      if (m.status === "fulfilled") setByModel(m.value ?? []);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const agentRows = useMemo(
    () => [...byAgent].sort((a, b) => b.total_cost_usd - a.total_cost_usd),
    [byAgent],
  );
  const modelRows = useMemo(
    () => [...byModel].sort((a, b) => b.total_cost_usd - a.total_cost_usd),
    [byModel],
  );

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

      {/* Token usage strip — costs derive from tokens, so the ledger's
          token totals sit right beside the dollar headline. All-time
          ledger figures (the /costs summary has no range parameter). */}
      {summary && (
        <div className="rounded-md border border-mycel-border bg-mycel-surface px-4 py-3 grid grid-cols-2 sm:grid-cols-4 gap-3">
          <Stat
            label="Total tokens"
            value={formatTokens(summary.total_tokens)}
            title={`${summary.total_tokens.toLocaleString()} tokens (input + output, all time)`}
          />
          <Stat
            label="Input tokens"
            value={formatTokens(summary.input_tokens)}
            title={`${summary.input_tokens.toLocaleString()} input tokens`}
          />
          <Stat
            label="Output tokens"
            value={formatTokens(summary.output_tokens)}
            title={`${summary.output_tokens.toLocaleString()} output tokens`}
          />
          <Stat
            label="Ledger cost"
            value={formatCost(summary.total_cost_usd)}
            title="All-time cost computed from the token ledger"
          />
        </div>
      )}

      <div className="rounded-md border border-mycel-border overflow-hidden shadow-mycel">
        <table className="w-full text-[13px]">
          <thead className="bg-mycel-surface text-mycel-muted">
            <tr>
              <th className={thLeft}>
                {groupBy === "repo" ? "Repo" : "Project"}
              </th>
              <th className={thRight}>Cost</th>
              <th className={`${thRight} w-24`}>Share</th>
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

      {/* By agent — token columns beside every dollar figure. */}
      {agentRows.length > 0 && (
        <div className="rounded-md border border-mycel-border overflow-hidden shadow-mycel">
          <table className="w-full text-[13px]">
            <thead className="bg-mycel-surface text-mycel-muted">
              <tr>
                <th className={thLeft}>Agent</th>
                <th className={thRight}>Input</th>
                <th className={thRight}>Output</th>
                <th className={thRight}>Tokens</th>
                <th className={thRight}>Cost</th>
              </tr>
            </thead>
            <tbody>
              {agentRows.map((a) => (
                <tr key={a.agent_id} className="border-t border-mycel-border hover:bg-mycel-surface-hover transition-colors">
                  <td className="px-3 py-2 text-mycel-text truncate max-w-[40%]">{a.agent_id}</td>
                  <td className={`${tdMono} text-mycel-text-2`} title={a.input_tokens.toLocaleString()}>
                    {formatTokens(a.input_tokens)}
                  </td>
                  <td className={`${tdMono} text-mycel-text-2`} title={a.output_tokens.toLocaleString()}>
                    {formatTokens(a.output_tokens)}
                  </td>
                  <td className={`${tdMono} text-mycel-text`} title={(a.input_tokens + a.output_tokens).toLocaleString()}>
                    {formatTokens(a.input_tokens + a.output_tokens)}
                  </td>
                  <td className={`${tdMono} text-mycel-text`}>{formatCost(a.total_cost_usd)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* By model — same token-beside-cost treatment. */}
      {modelRows.length > 0 && (
        <div className="rounded-md border border-mycel-border overflow-hidden shadow-mycel">
          <table className="w-full text-[13px]">
            <thead className="bg-mycel-surface text-mycel-muted">
              <tr>
                <th className={thLeft}>Model</th>
                <th className={thRight}>Input</th>
                <th className={thRight}>Output</th>
                <th className={thRight}>Tokens</th>
                <th className={thRight}>Cost</th>
              </tr>
            </thead>
            <tbody>
              {modelRows.map((m) => (
                <tr key={m.model} className="border-t border-mycel-border hover:bg-mycel-surface-hover transition-colors">
                  <td className="px-3 py-2 text-mycel-text truncate max-w-[40%]">{m.model}</td>
                  <td className={`${tdMono} text-mycel-text-2`} title={m.input_tokens.toLocaleString()}>
                    {formatTokens(m.input_tokens)}
                  </td>
                  <td className={`${tdMono} text-mycel-text-2`} title={m.output_tokens.toLocaleString()}>
                    {formatTokens(m.output_tokens)}
                  </td>
                  <td className={`${tdMono} text-mycel-text`} title={m.total_tokens.toLocaleString()}>
                    {formatTokens(m.total_tokens)}
                  </td>
                  <td className={`${tdMono} text-mycel-text`}>{formatCost(m.total_cost_usd)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
