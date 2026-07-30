/* ── useAgentPulse ──────────────────────────────────────────────────
   Transient event reactions for one agent's character. Subscribes to
   the shared SSE bus and returns the currently-active pulse kind (or
   null). Rate-limited: one pulse animation at a time, extras are
   dropped, so a busy agent never seizures. Honors reduced motion by
   staying silent. */

import { useEffect, useState } from "react";
import { subscribeAgentPulse } from "./agentEventBus";
import type { PulseKind } from "./agentEventBus";

/** How long a single pulse animation plays. */
export const PULSE_MS = 700;
/** Quiet gap after a pulse before the next may start. */
export const PULSE_GAP_MS = 200;

export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return false;
  }
  try {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  } catch {
    return false;
  }
}

export function useAgentPulse(name: string): PulseKind | null {
  const [pulse, setPulse] = useState<PulseKind | null>(null);

  useEffect(() => {
    if (!name || prefersReducedMotion()) return;
    let active = false;
    let cooldownUntil = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const unsub = subscribeAgentPulse(name, (kind) => {
      // One pulse at a time; queue-drop extras.
      if (active || Date.now() < cooldownUntil) return;
      active = true;
      setPulse(kind);
      timer = setTimeout(() => {
        active = false;
        cooldownUntil = Date.now() + PULSE_GAP_MS;
        setPulse(null);
      }, PULSE_MS);
    });

    return () => {
      unsub();
      clearTimeout(timer);
    };
  }, [name]);

  return pulse;
}
