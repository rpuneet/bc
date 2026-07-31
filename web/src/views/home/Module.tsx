import { Link } from "react-router-dom";
import { MONO } from "../../utils/typography";

/* ── HomeModule ─────────────────────────────────────────────────────
   Shared chrome for the Home right-rail modules: bordered card, small-
   caps mono header, optional trailing slot / "view all" link. Dense by
   design — small paddings, small type.
─────────────────────────────────────────────────────────────────── */

export function HomeModule({
  label,
  trailing,
  to,
  toLabel,
  children,
  testId,
  fill,
}: {
  label: string;
  trailing?: React.ReactNode;
  /** Optional "view all" destination rendered in the header. */
  to?: string;
  toLabel?: string;
  children: React.ReactNode;
  testId?: string;
  /** When true the module flexes to fill its parent and its body scrolls
   *  internally — used so a long feed never pushes sibling modules off
   *  the fold. */
  fill?: boolean;
}) {
  return (
    <section
      data-testid={testId}
      className={`rounded-lg border border-mycel-border bg-mycel-surface shadow-mycel-sm overflow-hidden ${
        fill ? "flex flex-col min-h-0 flex-1" : "shrink-0"
      }`}
    >
      <header className="shrink-0 flex items-center gap-2 px-3 py-1.5 border-b border-mycel-border bg-mycel-bg">
        <span
          className="text-[10px] font-semibold text-mycel-muted uppercase tracking-widest truncate"
          style={{ fontFamily: MONO }}
        >
          {label}
        </span>
        <span className="ml-auto flex items-center gap-2 shrink-0">
          {trailing}
          {to && (
            <Link
              to={to}
              className="text-[10px] text-mycel-muted hover:text-mycel-accent transition-colors"
            >
              {toLabel ?? "view all"} &rarr;
            </Link>
          )}
        </span>
      </header>
      <div className={`p-2.5 ${fill ? "flex-1 min-h-0 overflow-y-auto" : ""}`}>{children}</div>
    </section>
  );
}

/** Tiny client-side sparkline — accent line over a soft area fill. */
export function Spark({ points, max, width = 64, height = 18 }: { points: number[]; max?: number; width?: number; height?: number }) {
  if (points.length < 2) {
    return (
      <svg width={width} height={height} aria-hidden className="opacity-40">
        <line x1="0" y1={height - 1} x2={width} y2={height - 1} stroke="var(--mycel-border)" strokeWidth="1.5" strokeDasharray="2 3" />
      </svg>
    );
  }
  const hi = max ?? Math.max(...points, 1);
  const step = width / (points.length - 1);
  const y = (v: number) => height - 1.5 - (Math.min(v, hi) / (hi || 1)) * (height - 3);
  const line = points.map((v, i) => `${(i * step).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
  const area = `0,${height} ${line} ${width},${height}`;
  return (
    <svg width={width} height={height} aria-hidden>
      <polygon points={area} fill="var(--mycel-accent)" opacity="0.12" />
      <polyline points={line} fill="none" stroke="var(--mycel-accent)" strokeWidth="1.25" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}

/** UTC day key (YYYY-MM-DD) — matches the cost ledger's day bucketing. */
export function todayKey(): string {
  return new Date().toISOString().slice(0, 10);
}
