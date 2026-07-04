interface AgentStatusBadgeProps {
  state: string;
  size?: "sm" | "md";
}

const STATE_COLORS: Record<string, { dot: string; text: string }> = {
  running: { dot: "bg-mycel-success", text: "text-mycel-success" },
  working: { dot: "bg-mycel-success", text: "text-mycel-success" },
  idle: { dot: "bg-mycel-warning", text: "text-mycel-warning" },
  stuck: { dot: "bg-mycel-warning", text: "text-mycel-warning" },
  error: { dot: "bg-mycel-error", text: "text-mycel-error" },
  stopped: { dot: "bg-mycel-muted", text: "text-mycel-muted" },
  done: { dot: "bg-mycel-info", text: "text-mycel-info" },
  waiting: { dot: "bg-mycel-warning", text: "text-mycel-warning" },
  starting: { dot: "bg-mycel-success", text: "text-mycel-success" },
};

const DEFAULT_COLORS = { dot: "bg-mycel-muted", text: "text-mycel-muted" };

export function AgentStatusBadge({ state, size = "md" }: AgentStatusBadgeProps) {
  const colors = STATE_COLORS[state] ?? DEFAULT_COLORS;
  const isSm = size === "sm";

  return (
    <span className={`inline-flex items-center gap-1.5 ${isSm ? "text-xs" : "text-sm"}`}>
      <span
        className={`inline-block rounded-full ${colors.dot} ${isSm ? "w-1.5 h-1.5" : "w-2 h-2"}`}
      />
      <span className={`font-medium ${colors.text}`}>{state}</span>
    </span>
  );
}
