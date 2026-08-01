/**
 * BudgetPanel — the monthly cost cap, living in Insights (spend lives
 * here, so the cap that governs spend belongs here too, not in Settings).
 * Caps compare against spend computed from provider usage; the workspace
 * cap alerts at 80%. Per-agent caps are set from an agent's own page and
 * are listed read-only here for context.
 *
 * Moved out of Settings → Budgets (#2674 IA pass). Same endpoints:
 * GET/POST/DELETE /api/costs/budgets.
 */

import { useCallback, useEffect, useState } from "react";
import { api, type BudgetStatus } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { SectionRule } from "../../components/shared/SectionRule";

type SaveStatus = "idle" | "saving" | "saved" | "error";

export function BudgetPanel() {
  const fetcher = useCallback(() => api.getCostBudgets(), []);
  const { data, refresh } = usePolling<BudgetStatus[]>(fetcher, 30000);
  // Guard against a non-array payload (a degraded endpoint) so the whole
  // Insights page never crashes on a bad budgets response.
  const budgets = Array.isArray(data) ? data : [];
  const workspace = budgets.find((b) => b.scope === "workspace");

  const [limit, setLimit] = useState("");
  const [status, setStatus] = useState<SaveStatus>("idle");

  const wsLimit = workspace?.limit_usd;
  useEffect(() => {
    if (wsLimit !== undefined) setLimit(String(wsLimit));
  }, [wsLimit]);

  const save = async () => {
    const value = Number(limit);
    setStatus("saving");
    try {
      if (!limit.trim() || value <= 0) {
        await api.deleteCostBudget("workspace").catch(() => { /* nothing to remove */ });
      } else {
        await api.createCostBudget({ scope: "workspace", period: "monthly", limit_usd: value, alert_at: 0.8 });
      }
      refresh();
      setStatus("saved");
      setTimeout(() => setStatus("idle"), 2000);
    } catch {
      setStatus("error");
    }
  };

  const perAgent = budgets.filter((b) => b.scope.startsWith("agent:"));

  return (
    <section>
      <SectionRule label="Budget" trailing={<span className="text-[11px] text-mycel-muted">monthly cap · alerts at 80%</span>} />
      <div className="rounded-lg border border-mycel-border bg-mycel-surface shadow-mycel-sm p-4 space-y-3">
        <div className="flex items-center gap-2">
          <span className="text-xs text-mycel-muted">$</span>
          <input
            type="number"
            min={0}
            step={5}
            value={limit}
            placeholder="No limit"
            onChange={(e) => setLimit(e.target.value)}
            aria-label="Monthly cost cap in dollars"
            className="w-40 px-2.5 py-1.5 text-[13px] tabular-nums rounded-md border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:border-mycel-accent focus:ring-1 focus:ring-mycel-accent transition-colors"
          />
          <button
            type="button"
            onClick={save}
            disabled={status === "saving"}
            className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover active:scale-[0.98] shadow-mycel-sm transition-all disabled:opacity-60"
          >
            {status === "saving" ? "Saving…" : "Save"}
          </button>
          {status === "saved" && <span className="text-[11px] text-mycel-success">Saved</span>}
          {status === "error" && <span className="text-[11px] text-mycel-error">Couldn&apos;t save the cap.</span>}
        </div>

        {perAgent.length > 0 && (
          <div className="rounded-lg border border-mycel-border bg-mycel-bg divide-y divide-mycel-border overflow-hidden">
            {perAgent.map((b) => (
              <div key={b.scope} className="flex items-center justify-between gap-3 px-3 py-2 text-xs">
                <span className="text-mycel-text truncate">{b.scope.replace(/^agent:/, "")}</span>
                <span className="text-mycel-muted shrink-0 tabular-nums">${b.limit_usd}/{b.period}</span>
                <button
                  type="button"
                  onClick={() => void api.deleteCostBudget(b.scope).then(refresh)}
                  className="text-mycel-muted hover:text-mycel-error shrink-0 cursor-pointer transition-colors"
                  title="Remove cap"
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        )}
        <p className="text-[11px] text-mycel-muted">
          Set per-agent caps from an agent&apos;s page. Caps compare against spend computed from provider usage.
        </p>
      </div>
    </section>
  );
}
