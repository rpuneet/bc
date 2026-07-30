/* ── Shared agent event bus ─────────────────────────────────────────
   One SSE connection fanned out to every mounted character. Listeners
   register per agent name (or "*" for everything), so a busy stream
   only re-renders the characters it actually mentions. The EventSource
   is opened lazily on the first listener and closed when the last one
   leaves. */

import type { WSEvent } from "../../api/types";

export type PulseKind = "message" | "tool" | "state";

type PulseListener = (kind: PulseKind) => void;

/** Wildcard key — listeners registered under it hear every agent. */
export const ANY_AGENT = "*";

const listeners = new Map<string, Set<PulseListener>>();

let es: EventSource | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | undefined;

function dispatch(name: string, kind: PulseKind) {
  listeners.get(name)?.forEach((fn) => fn(kind));
  listeners.get(ANY_AGENT)?.forEach((fn) => fn(kind));
}

/** Map a raw SSE event to (agent, pulse kind) and fan out.
 *  Exported so tests can drive the bus without a live EventSource. */
export function handleAgentEvent(ev: WSEvent): void {
  const d = ev.data;
  let name: unknown;
  let kind: PulseKind;
  switch (ev.type) {
    case "agent.hook":
      name = d.agent ?? d.name;
      kind = "tool";
      break;
    case "agent.state_changed":
    case "agent.started":
    case "agent.stopped":
    case "agent.created":
      name = d.name ?? d.agent;
      kind = "state";
      break;
    case "channel.message":
      name = d.sender ?? d.agent;
      kind = "message";
      break;
    case "gateway.delivery":
      name = d.agent ?? d.sender;
      kind = "message";
      break;
    default:
      return;
  }
  if (typeof name === "string" && name !== "") dispatch(name, kind);
}

function openStream() {
  if (es || typeof EventSource === "undefined") return;
  try {
    es = new EventSource("/api/events");
  } catch {
    es = null;
    return;
  }
  es.onmessage = (e: MessageEvent) => {
    try {
      handleAgentEvent(JSON.parse(e.data as string) as WSEvent);
    } catch {
      // ignore malformed frames
    }
  };
  es.onerror = () => {
    es?.close();
    es = null;
    // Retry only while someone is still listening.
    if (listeners.size > 0) {
      reconnectTimer = setTimeout(openStream, 5000);
    }
  };
}

function closeStreamIfIdle() {
  if (listeners.size > 0) return;
  clearTimeout(reconnectTimer);
  es?.close();
  es = null;
}

/** Listen for pulses for one agent (or ANY_AGENT). Returns unsubscribe. */
export function subscribeAgentPulse(name: string, fn: PulseListener): () => void {
  let set = listeners.get(name);
  if (!set) {
    set = new Set();
    listeners.set(name, set);
  }
  set.add(fn);
  openStream();
  return () => {
    const cur = listeners.get(name);
    cur?.delete(fn);
    if (cur && cur.size === 0) listeners.delete(name);
    closeStreamIfIdle();
  };
}
