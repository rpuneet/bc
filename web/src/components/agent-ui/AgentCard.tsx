/* ── AgentCard ──────────────────────────────────────────────────────
   Grid-scale identity: live character + name + state + current task.
   For agent grids and summary panels. */

import { memo } from "react";
import { LiveAgentCharacter } from "./AgentCharacter";
import { AgentStatusBadge } from "./AgentStatusBadge";

export interface AgentCardProps {
  name: string;
  state: string;
  task?: string;
  tool?: string;
  /** Character size in px (default 40). */
  size?: number;
  className?: string;
  onClick?: () => void;
}

export const AgentCard = memo(function AgentCard({
  name,
  state,
  task,
  tool,
  size = 40,
  className = "",
  onClick,
}: AgentCardProps) {
  const inner = (
    <>
      <LiveAgentCharacter name={name} state={state} size={size} tool={tool} />
      <span className="flex flex-col min-w-0 text-left">
        <span className="flex items-center gap-2 min-w-0">
          <span className="font-mono text-sm font-semibold text-mycel-text truncate">
            {name}
          </span>
          <AgentStatusBadge state={state} size="sm" />
        </span>
        {task && (
          <span className="text-xs text-mycel-muted truncate" title={task}>
            {task}
          </span>
        )}
      </span>
    </>
  );
  const cls =
    `flex items-center gap-3 rounded-lg border border-mycel-border bg-mycel-surface px-3 py-2.5 min-w-0 ${className}`.trim();
  if (onClick) {
    return (
      <button type="button" onClick={onClick} className={`${cls} hover:bg-mycel-surface-hover transition-colors`}>
        {inner}
      </button>
    );
  }
  return <div className={cls}>{inner}</div>;
});
