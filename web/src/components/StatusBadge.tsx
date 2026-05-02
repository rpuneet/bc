const COLORS: Record<string, string> = {
  idle: "bg-amber-500/15 text-amber-400",
  working: "bg-green-500/20 text-green-400",
  starting: "bg-green-500/20 text-green-400",
  done: "bg-mycel-success/20 text-mycel-success",
  stuck: "bg-amber-500/20 text-amber-400",
  error: "bg-mycel-error/20 text-red-400",
  stopped: "bg-mycel-muted/10 text-mycel-muted",
};

export function StatusBadge({ status }: { status: string }) {
  const cls = COLORS[status] ?? COLORS["idle"]!;
  return (
    <span
      className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${cls}`}
    >
      {status}
    </span>
  );
}
