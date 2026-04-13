interface AgentStatusBadgeProps {
  state: string;
  size?: "sm" | "md";
}

const STATE_COLORS: Record<string, { dot: string; text: string }> = {
  running: { dot: "bg-bc-success", text: "text-bc-success" },
  working: { dot: "bg-bc-success", text: "text-bc-success" },
  idle: { dot: "bg-bc-success", text: "text-bc-success" },
  stuck: { dot: "bg-bc-warning", text: "text-bc-warning" },
  error: { dot: "bg-bc-error", text: "text-bc-error" },
  stopped: { dot: "bg-bc-muted", text: "text-bc-muted" },
  waiting: { dot: "bg-purple-500", text: "text-purple-400" },
  starting: { dot: "bg-bc-info", text: "text-bc-info" },
};

const DEFAULT_COLORS = { dot: "bg-bc-muted", text: "text-bc-muted" };

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
