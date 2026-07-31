/* ── StatBand ────────────────────────────────────────────────────────
   The hairline-divided stat band from the Insights page, extracted as a
   shared primitive so agent-scoped surfaces (the detail Metrics tab)
   read with the exact same visual language as the fleet-wide Insights
   view — one bordered card, cells split by 1px rules, small-caps labels
   over an xl tabular value. */

import type { ReactNode } from "react";

export function StatBand({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-mycel-border shadow-mycel-sm overflow-hidden">
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-px bg-mycel-border">{children}</div>
    </div>
  );
}

export function StatCell({
  label,
  value,
  sub,
  accent,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  /** Render the value in the accent hue (e.g. cost). */
  accent?: boolean;
}) {
  return (
    <div className="bg-mycel-surface p-4 min-w-0">
      <div className="text-[10px] font-medium text-mycel-muted uppercase tracking-[0.08em] truncate">
        {label}
      </div>
      <div
        className={`mt-1 text-xl font-semibold tabular-nums truncate ${
          accent ? "text-mycel-accent" : "text-mycel-text"
        }`}
      >
        {value}
      </div>
      <div className="mt-0.5 text-[11px] text-mycel-muted truncate">{sub ?? " "}</div>
    </div>
  );
}
