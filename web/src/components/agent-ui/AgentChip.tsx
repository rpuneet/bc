/* ── AgentChip ──────────────────────────────────────────────────────
   Inline identity: live character + mono name + status dot. The one
   way an agent appears in lists, dropdowns, message feeds and the
   drawer. Compact by design — characters at 16-20px still read via
   silhouette + face. */

import { memo } from "react";
import { LiveAgentCharacter } from "./AgentCharacter";

const DOT_COLORS: Record<string, string> = {
  running: "var(--mycel-success)",
  working: "var(--mycel-success)",
  starting: "var(--mycel-success)",
  done: "var(--mycel-info)",
  idle: "var(--mycel-warning)",
  waiting: "var(--mycel-warning)",
  stuck: "var(--mycel-warning)",
  error: "var(--mycel-error)",
  stopped: "var(--mycel-muted)",
};

export interface AgentChipProps {
  name: string;
  state?: string;
  /** Character size in px (default 18). */
  size?: number;
  /** Hide the status dot (e.g. sender chips in message feeds). */
  showDot?: boolean;
  className?: string;
  onClick?: () => void;
}

export const AgentChip = memo(function AgentChip({
  name,
  state,
  size = 18,
  showDot = true,
  className = "",
  onClick,
}: AgentChipProps) {
  const dot = DOT_COLORS[state ?? ""] ?? "var(--mycel-muted)";
  const body = (
    <>
      <LiveAgentCharacter name={name} state={state ?? "idle"} size={size} />
      <span
        className="truncate font-mono"
        style={{ fontSize: Math.max(11, Math.min(13, size * 0.68)) }}
      >
        {name}
      </span>
      {showDot && state !== undefined && (
        <span
          data-testid="agent-chip-dot"
          className="shrink-0 rounded-full"
          style={{ width: 6, height: 6, background: dot }}
          title={state}
          aria-hidden
        />
      )}
    </>
  );
  const cls = `inline-flex items-center gap-1.5 min-w-0 ${className}`.trim();
  if (onClick) {
    return (
      <button type="button" onClick={onClick} className={cls} title={name}>
        {body}
      </button>
    );
  }
  return (
    <span className={cls} title={name}>
      {body}
    </span>
  );
});
