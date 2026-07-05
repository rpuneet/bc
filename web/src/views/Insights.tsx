/**
 * Insights — Metrics + Costs merged behind one nav item.
 *
 * Two tabs (?tab=metrics | ?tab=costs) rendering the existing Stats and
 * CostsGlobal views unchanged as tab content. Only the active tab is
 * mounted so each view's useHeaderSlot (range picker / date picker)
 * owns the header without fighting the other. /stats, /metrics and
 * /costs redirect here (see App.tsx) so old links keep working.
 *
 * Tab styling follows AgentDetail's HUD tabs: small uppercase buttons
 * with an accent underline on the active tab.
 */

import { useSearchParams } from "react-router-dom";
import { Stats } from "./Stats";
import { CostsGlobal } from "./CostsGlobal";

const TABS = [
  { key: "metrics", label: "Metrics" },
  { key: "costs", label: "Costs" },
] as const;

type TabKey = (typeof TABS)[number]["key"];

export function Insights() {
  const [params, setParams] = useSearchParams();
  const tab: TabKey = params.get("tab") === "costs" ? "costs" : "metrics";

  return (
    <div className="flex flex-col h-full min-h-0">
      <div
        role="tablist"
        aria-label="Insights sections"
        className="shrink-0 flex items-center gap-1 px-4 pt-1 border-b border-mycel-border"
      >
        {TABS.map((t) => {
          const isActive = tab === t.key;
          return (
            <button
              key={t.key}
              type="button"
              role="tab"
              aria-selected={isActive}
              onClick={() => setParams({ tab: t.key }, { replace: true })}
              className={`relative px-2.5 py-2 text-[11px] font-medium tracking-wide uppercase transition-colors shrink-0 ${
                isActive ? "text-mycel-accent" : "text-mycel-muted hover:text-mycel-text"
              }`}
            >
              {t.label}
              {isActive && (
                <span className="absolute -bottom-px left-2 right-2 h-[2px] bg-mycel-accent rounded-full" />
              )}
            </button>
          );
        })}
      </div>
      <div className="flex-1 min-h-0 overflow-auto">
        {tab === "metrics" ? <Stats /> : <CostsGlobal />}
      </div>
    </div>
  );
}
