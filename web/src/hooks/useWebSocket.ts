import { useEffect, useRef, useState, useCallback } from "react";
import type { WSEvent, WSEventType } from "../api/types";
import { invalidateAgents } from "../api/client";

type Listener = (event: WSEvent) => void;

// Agent lifecycle events change the shared /api/agents list, so clear its
// cache the moment one arrives — the SSE stream is the real-time source of
// truth and any stale cached list must yield to it immediately.
const AGENT_CACHE_INVALIDATORS: ReadonlySet<WSEventType> = new Set([
  "agent.created",
  "agent.started",
  "agent.stopped",
  "agent.deleted",
  "agent.state_changed",
]);

export function useWebSocket() {
  const esRef = useRef<EventSource | null>(null);
  const listenersRef = useRef<Map<WSEventType, Set<Listener>>>(new Map());
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>();

  const connect = useCallback(() => {
    let es: EventSource;
    try {
      es = new EventSource("/api/events");
    } catch {
      // EventSource not available — degrade gracefully
      return;
    }

    es.onopen = () => {
      setConnected(true);
      setReconnecting(false);
    };

    es.onmessage = (e: MessageEvent) => {
      try {
        const event = JSON.parse(e.data as string) as WSEvent;
        if (AGENT_CACHE_INVALIDATORS.has(event.type)) {
          invalidateAgents();
        }
        const listeners = listenersRef.current.get(event.type);
        listeners?.forEach((fn) => fn(event));
      } catch {
        // ignore malformed messages
      }
    };

    es.onerror = () => {
      setConnected(false);
      setReconnecting(true);
      es.close();
      reconnectTimer.current = setTimeout(connect, 3000);
    };

    esRef.current = es;
  }, []);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      esRef.current?.close();
    };
  }, [connect]);

  const subscribe = useCallback((type: WSEventType, listener: Listener) => {
    if (!listenersRef.current.has(type)) {
      listenersRef.current.set(type, new Set());
    }
    listenersRef.current.get(type)!.add(listener);
    return () => {
      listenersRef.current.get(type)?.delete(listener);
    };
  }, []);

  return { connected, reconnecting, subscribe };
}
