/* ── AgentChip ──────────────────────────────────────────────────────
   Inline identity: live character + mono name + status dot. The one
   way an agent appears in lists, dropdowns, message feeds and the
   drawer. Compact by design — characters at 16-20px still read via
   silhouette + face. */

import { memo, useCallback, useEffect, useRef, useState } from "react";
import type { Agent } from "../../api/client";
import { LiveAgentCharacter } from "./AgentCharacter";
import { AgentHoverCard } from "./AgentHoverCard";

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
  /** Show a hover popover (state, task, provider, spend) on mouse-over. */
  preview?: boolean;
  /** Pre-fetched agent data for the preview, when the caller has it. */
  previewSeed?: Agent;
}

/** Delay before the hover card appears — long enough that a cursor
 *  passing over a list doesn't flash cards, short enough to feel instant
 *  on a deliberate hover. */
const PREVIEW_DELAY_MS = 240;

export const AgentChip = memo(function AgentChip({
  name,
  state,
  size = 18,
  showDot = true,
  className = "",
  onClick,
  preview = false,
  previewSeed,
}: AgentChipProps) {
  const dot = DOT_COLORS[state ?? ""] ?? "var(--mycel-muted)";
  const [hoverRect, setHoverRect] = useState<DOMRect | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout>>();
  const anchorRef = useRef<HTMLElement>(null);

  const openPreview = useCallback(() => {
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      const el = anchorRef.current;
      if (el) setHoverRect(el.getBoundingClientRect());
    }, PREVIEW_DELAY_MS);
  }, []);
  const closePreview = useCallback(() => {
    clearTimeout(timerRef.current);
    setHoverRect(null);
  }, []);

  // Clear a pending open timer if the chip unmounts mid-hover.
  useEffect(() => () => clearTimeout(timerRef.current), []);

  // While the card is open, dismiss it on scroll/resize — the anchor rect
  // is captured once, so a moving list would otherwise leave a stale card
  // floating in place. Capture-phase catches inner scroll containers too.
  useEffect(() => {
    if (!hoverRect) return;
    const dismiss = () => closePreview();
    window.addEventListener("scroll", dismiss, true);
    window.addEventListener("resize", dismiss);
    return () => {
      window.removeEventListener("scroll", dismiss, true);
      window.removeEventListener("resize", dismiss);
    };
  }, [hoverRect, closePreview]);

  const previewHandlers = preview
    ? {
        onMouseEnter: openPreview,
        onMouseLeave: closePreview,
        onFocus: openPreview,
        onBlur: closePreview,
      }
    : {};
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
  const card = preview && hoverRect && (
    <AgentHoverCard name={name} rect={hoverRect} seed={previewSeed} />
  );
  if (onClick) {
    return (
      <>
        <button
          type="button"
          ref={anchorRef as React.RefObject<HTMLButtonElement>}
          onClick={onClick}
          className={cls}
          title={name}
          {...previewHandlers}
        >
          {body}
        </button>
        {card}
      </>
    );
  }
  return (
    <>
      <span ref={anchorRef as React.RefObject<HTMLSpanElement>} className={cls} title={name} {...previewHandlers}>
        {body}
      </span>
      {card}
    </>
  );
});
