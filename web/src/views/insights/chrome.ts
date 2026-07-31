/**
 * Shared chart chrome for the Insights page and its drill-downs —
 * one place for tick/axis/tooltip styling so every chart on the
 * surface reads as the same system.
 */

export const ACCENT = "var(--mycel-accent)";
export const TICK = { fill: "var(--mycel-muted)", fontSize: 10 };
export const AX = { axisLine: false as const, tickLine: false as const };
export const TT_STYLE: React.CSSProperties = {
  backgroundColor: "var(--mycel-surface-2)",
  border: "1px solid var(--mycel-border)",
  borderRadius: "6px",
  color: "var(--mycel-text)",
  fontSize: "12px",
  boxShadow: "var(--mycel-shadow-lg)",
};

/** Minimal shape recharts passes to a custom tooltip `content`. */
export interface ChartTooltipProps {
  active?: boolean;
  label?: string | number;
  payload?: { dataKey?: string | number; value?: number | string; payload?: unknown }[];
}

export const fmtClock = (ms: number): string =>
  new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

// Cost-ledger agent ids may be namespaced "mycel-<hash>-<agent>". Show
// the bare agent name; fall back to the raw id if stripping empties it.
export function stripAgentPrefix(id: string): string {
  if (id.startsWith("mycel-")) {
    const rest = id.split("-").slice(2).join("-");
    return rest || id;
  }
  return id;
}

export const fmtShortDate = (d: string): string => {
  // "2026-07-30" → "Jul 30" without timezone drift.
  const [, m, day] = d.split("-");
  const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  return `${months[Number(m) - 1]} ${Number(day)}`;
};
