// Status pills surface agent lifecycle state across Agents, Live, and the
// agent detail header. The map is keyed on the canonical agent state names
// emitted by pkg/agent (idle, working, starting, stuck, done, error,
// stopped). Each variant pairs a soft background with a higher-contrast
// foreground so the badge stays readable on the dark surface and the
// inactive states are still visibly chips — not just muted text.
const COLORS: Record<string, string> = {
  idle: "bg-amber-500/15 text-amber-300 ring-1 ring-amber-500/20",
  working: "bg-emerald-500/20 text-emerald-300 ring-1 ring-emerald-500/30",
  starting: "bg-emerald-500/15 text-emerald-300 ring-1 ring-emerald-500/25",
  done: "bg-sky-500/15 text-sky-300 ring-1 ring-sky-500/25",
  stuck: "bg-amber-500/20 text-amber-300 ring-1 ring-amber-500/30",
  error: "bg-rose-500/15 text-rose-300 ring-1 ring-rose-500/30",
  stopped: "bg-zinc-500/15 text-zinc-300 ring-1 ring-zinc-500/25",
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
