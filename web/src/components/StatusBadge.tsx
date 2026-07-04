// Status pills surface agent lifecycle state across Agents, Live, and the
// agent detail header. The map is keyed on the canonical agent state names
// emitted by pkg/agent (idle, working, starting, stuck, done, error,
// stopped). Each variant pairs a subtle semantic tint with its semantic
// foreground so the badge stays readable in both themes and the
// inactive states are still visibly chips — not just muted text.
const COLORS: Record<string, string> = {
  idle: "bg-mycel-warning-subtle text-mycel-warning ring-1 ring-inset ring-mycel-border",
  working: "bg-mycel-success-subtle text-mycel-success ring-1 ring-inset ring-mycel-border",
  starting: "bg-mycel-success-subtle text-mycel-success ring-1 ring-inset ring-mycel-border",
  done: "bg-mycel-info-subtle text-mycel-info ring-1 ring-inset ring-mycel-border",
  stuck: "bg-mycel-warning-subtle text-mycel-warning ring-1 ring-inset ring-mycel-border",
  error: "bg-mycel-error-subtle text-mycel-error ring-1 ring-inset ring-mycel-border",
  stopped: "bg-mycel-surface-hover text-mycel-text-2 ring-1 ring-inset ring-mycel-border",
};

export function StatusBadge({ status }: { status: string }) {
  const cls = COLORS[status] ?? COLORS["idle"]!;
  return (
    <span
      className={`inline-block px-2 py-0.5 rounded text-xs font-medium tabular-nums ${cls}`}
    >
      {status}
    </span>
  );
}
