/* ── AgentHoverCard ─────────────────────────────────────────────────
   Small popover shown when hovering an AgentChip in a list or the
   drawer. Surfaces the agent's state, current task, provider and total
   spend without a navigation — a cheap way to make the identity chips
   feel connected to the fuller agent surfaces.

   Rendered through a portal at a fixed, viewport-clamped position so it
   is never clipped by a scrolling drawer/list container. Purely
   informational: pointer-events are disabled so the card never steals
   the hover it is describing (dismissal is owned by the chip's
   mouse-leave). Data is fetched lazily and cached per name; callers that
   already hold the Agent can pass it as `seed` to skip the round-trip. */

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import type { Agent } from "../../api/client";
import { api } from "../../api/client";
import { LiveAgentCharacter } from "./AgentCharacter";
import { prefersReducedMotion } from "./useAgentPulse";

const cache = new Map<string, Agent>();
const inflight = new Map<string, Promise<Agent | null>>();

/** Fetch (and memoize) an agent, coalescing concurrent requests. */
function loadAgent(name: string): Promise<Agent | null> {
  const cached = cache.get(name);
  if (cached) return Promise.resolve(cached);
  const existing = inflight.get(name);
  if (existing) return existing;
  const p = api
    .getAgent(name)
    .then((a) => {
      cache.set(name, a);
      inflight.delete(name);
      return a;
    })
    .catch(() => {
      inflight.delete(name);
      return null;
    });
  inflight.set(name, p);
  return p;
}

const CARD_WIDTH = 244;
const EST_HEIGHT = 128;
const GAP = 8;
const MARGIN = 8;

const STATE_COLORS: Record<string, string> = {
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

export function AgentHoverCard({
  name,
  rect,
  seed,
}: {
  name: string;
  /** Bounding rect of the chip that triggered the card. */
  rect: DOMRect;
  /** Pre-fetched agent data, when the caller already has it. */
  seed?: Agent;
}) {
  const [agent, setAgent] = useState<Agent | null>(seed ?? cache.get(name) ?? null);

  useEffect(() => {
    if (seed) {
      cache.set(name, seed);
      setAgent(seed);
      return;
    }
    let alive = true;
    void loadAgent(name).then((a) => {
      if (alive && a) setAgent(a);
    });
    return () => {
      alive = false;
    };
  }, [name, seed]);

  if (typeof document === "undefined") return null;

  // Anchor below the chip, flipping above when it would overflow the
  // viewport; horizontally clamped so it never spills off-screen.
  let left = rect.left;
  if (left + CARD_WIDTH > window.innerWidth - MARGIN) {
    left = window.innerWidth - CARD_WIDTH - MARGIN;
  }
  if (left < MARGIN) left = MARGIN;
  let top = rect.bottom + GAP;
  if (top + EST_HEIGHT > window.innerHeight - MARGIN) {
    top = Math.max(MARGIN, rect.top - GAP - EST_HEIGHT);
  }

  const state = agent?.state ?? "idle";
  const dot = STATE_COLORS[state] ?? "var(--mycel-muted)";
  const provider = agent
    ? agent.model
      ? `${agent.tool} · ${agent.model}`
      : agent.tool || "—"
    : "—";
  const reduce = prefersReducedMotion();

  return createPortal(
    <div
      data-testid="agent-hover-card"
      role="tooltip"
      style={{
        position: "fixed",
        left,
        top,
        width: CARD_WIDTH,
        zIndex: 80,
        pointerEvents: "none",
        background: "var(--mycel-surface-2)",
        border: "1px solid var(--mycel-border)",
        borderRadius: 10,
        boxShadow: "var(--mycel-shadow-lg)",
        padding: "10px 12px",
        animation: reduce ? undefined : "agentHoverIn 120ms cubic-bezier(0.4,0,0.2,1)",
      }}
    >
      {/* Identity row */}
      <div className="flex items-center" style={{ gap: 8, minWidth: 0 }}>
        <LiveAgentCharacter name={name} state={state} size={26} />
        <span
          className="truncate"
          style={{ fontSize: 12.5, fontWeight: 600, color: "var(--mycel-text)", fontFamily: "'JetBrains Mono', monospace" }}
        >
          {name}
        </span>
        <span className="ml-auto flex items-center shrink-0" style={{ gap: 5 }}>
          <span style={{ width: 6, height: 6, borderRadius: 999, background: dot }} aria-hidden />
          <span style={{ fontSize: 10.5, color: "var(--mycel-muted)" }}>{state}</span>
        </span>
      </div>

      {/* Task */}
      <div
        style={{
          marginTop: 8,
          fontSize: 11.5,
          lineHeight: 1.4,
          color: agent?.task ? "var(--mycel-text-2)" : "var(--mycel-muted)",
          fontStyle: agent?.task ? "normal" : "italic",
          display: "-webkit-box",
          WebkitLineClamp: 2,
          WebkitBoxOrient: "vertical",
          overflow: "hidden",
        }}
      >
        {agent?.task || "No active task"}
      </div>

      {/* Provider · cost footer */}
      <div
        className="flex items-center"
        style={{
          marginTop: 8,
          paddingTop: 8,
          borderTop: "1px solid var(--mycel-border)",
          gap: 8,
          fontSize: 10.5,
          color: "var(--mycel-muted)",
        }}
      >
        <span className="truncate" style={{ fontFamily: "'JetBrains Mono', monospace" }} title={provider}>
          {provider}
        </span>
        {agent && agent.total_cost_usd > 0 && (
          <span
            className="ml-auto shrink-0 tabular-nums"
            style={{ fontFamily: "'JetBrains Mono', monospace", color: "var(--mycel-text-2)" }}
            title="Total spend"
          >
            ${agent.total_cost_usd.toFixed(2)}
          </span>
        )}
      </div>

      <style>{`@keyframes agentHoverIn { from { opacity: 0; transform: translateY(-3px); } to { opacity: 1; transform: translateY(0); } }`}</style>
    </div>,
    document.body,
  );
}
