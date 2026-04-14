const COLORS: Record<string, string> = {
  idle: "bg-bc-muted/20 text-bc-muted",
  working: "bg-green-500/20 text-green-400",
  starting: "bg-green-500/20 text-green-400",
  done: "bg-bc-success/20 text-bc-success",
  stuck: "bg-amber-500/20 text-amber-400",
  error: "bg-bc-error/20 text-red-400",
  stopped: "bg-bc-muted/10 text-bc-muted",
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
