/**
 * Shared UI primitives and formatters used by Stats.tsx and StatsTab.tsx.
 */

// ── Primitives ──────────────────────────────────────────────────────────────

export function Panel({
  title,
  children,
  className,
}: {
  title: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={`rounded border border-mycel-border bg-mycel-surface overflow-hidden ${className ?? ""}`}
    >
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-mycel-border bg-mycel-bg">
        <span className="text-[11px] font-medium text-mycel-muted uppercase tracking-wider">
          {title}
        </span>
      </div>
      <div className="p-3">{children}</div>
    </div>
  );
}

export function Empty({ msg = "No data yet" }: { msg?: string }) {
  return (
    <div className="flex items-center justify-center h-[200px] text-sm text-mycel-muted">
      {msg}
    </div>
  );
}

// ── Formatters ──────────────────────────────────────────────────────────────

export const fmtTime = (iso: string): string => {
  try {
    return new Date(iso).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
};

export const fmtBytes = (b: number): string => {
  if (!b) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(b) / Math.log(1024));
  return `${(b / Math.pow(1024, i)).toFixed(1)} ${u[i]}`;
};

export const fmtTokens = (n: number): string => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
};
