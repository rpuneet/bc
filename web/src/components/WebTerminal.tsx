import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

/** Connection lifecycle as seen from the parent. */
export type TerminalConnectionState = "connecting" | "open" | "closed" | "error";

export interface TerminalConnectionDetail {
  /** WebSocket close code (1000 = normal). Present on "closed"/"error". */
  code?: number;
  reason?: string;
}

interface WebTerminalProps {
  agentName: string;
  /**
   * Bumping this value (e.g. incrementing a counter) triggers a clean
   * reconnect of the underlying WebSocket without rebuilding the xterm
   * instance, so scrollback is preserved.
   */
  reconnectToken?: number;
  /** Back-compat: called once when the socket is first torn down. */
  onDisconnect?: () => void;
  /** Detailed connection lifecycle — used by the Attach-tab overlay. */
  onConnectionStateChange?: (
    state: TerminalConnectionState,
    detail?: TerminalConnectionDetail,
  ) => void;
}

/**
 * WebTerminal — xterm.js surface + WebSocket plumbing for live agent
 * terminals.
 *
 * The xterm instance is created exactly once per mount, and its lifecycle
 * is intentionally decoupled from the underlying WebSocket so that the
 * parent can trigger a reconnect (via `reconnectToken`) without losing
 * scrollback. The parent is responsible for any visible "disconnected /
 * stopped / retry" UX — this component only surfaces state via
 * `onConnectionStateChange`.
 */
export function WebTerminal({
  agentName,
  reconnectToken = 0,
  onDisconnect,
  onConnectionStateChange,
}: WebTerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  // Stable refs for the callbacks so the WS effect can depend only on
  // the inputs that should actually trigger a reconnect.
  const onDisconnectRef = useRef(onDisconnect);
  const onStateRef = useRef(onConnectionStateChange);
  useEffect(() => {
    onDisconnectRef.current = onDisconnect;
    onStateRef.current = onConnectionStateChange;
  }, [onDisconnect, onConnectionStateChange]);

  // ── Terminal lifecycle: create once per mount. ────────────────────
  useEffect(() => {
    if (!containerRef.current) return;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: "'Space Mono', 'Menlo', 'Consolas', monospace",
      // Dark-roast terminal palette — matches the landing's terminal
      // tokens (always dark, in both app themes): espresso ground,
      // cream ink, chanterelle amber cursor.
      theme: {
        background: "#14100b",
        foreground: "#f2eadc",
        cursor: "#e8a33d",
        selectionBackground: "#4a3b2a",
      },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());
    term.open(containerRef.current);
    fitAddon.fit();

    termRef.current = term;
    fitRef.current = fitAddon;

    // Forward keyboard + binary input to whichever WS is currently live.
    term.onData((data: string) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(data);
    });
    term.onBinary((data: string) => {
      const ws = wsRef.current;
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      const buf = new Uint8Array(data.length);
      for (let i = 0; i < data.length; i++) buf[i] = data.charCodeAt(i);
      ws.send(buf.buffer);
    });
    term.onResize(({ cols, rows }) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols, rows }));
      }
    });

    const onResize = () => {
      fitAddon.fit();
      const dims = fitAddon.proposeDimensions();
      const ws = wsRef.current;
      if (dims && ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols: dims.cols, rows: dims.rows }));
      }
    };
    const resizeObserver = new ResizeObserver(onResize);
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      term.dispose();
      termRef.current = null;
      fitRef.current = null;
    };
  }, []);

  // ── WebSocket lifecycle: re-run on agent or reconnectToken change. ─
  useEffect(() => {
    const term = termRef.current;
    const fitAddon = fitRef.current;
    if (!term || !fitAddon) return;

    onStateRef.current?.("connecting");

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${proto}//${window.location.host}/api/agents/${encodeURIComponent(agentName)}/terminal`;
    const ws = new WebSocket(wsUrl);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    // If onerror fires we want to treat the subsequent close as an error,
    // not as a clean shutdown. Track that here.
    let sawError = false;

    ws.onopen = () => {
      onStateRef.current?.("open");
      const dims = fitAddon.proposeDimensions();
      if (dims) {
        ws.send(JSON.stringify({ type: "resize", cols: dims.cols, rows: dims.rows }));
      }
    };

    ws.onmessage = (evt: MessageEvent) => {
      if (evt.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(evt.data));
      } else {
        term.write(evt.data as string);
      }
    };

    ws.onerror = () => {
      sawError = true;
      onStateRef.current?.("error");
    };

    ws.onclose = (evt: CloseEvent) => {
      const detail: TerminalConnectionDetail = {
        code: evt.code,
        reason: evt.reason || undefined,
      };
      onStateRef.current?.(sawError ? "error" : "closed", detail);
      onDisconnectRef.current?.();
    };

    return () => {
      // Clear handlers before close() so we don't report a synthetic
      // "closed" state back to the parent after unmount / reconnect.
      ws.onopen = null;
      ws.onmessage = null;
      ws.onerror = null;
      ws.onclose = null;
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        ws.close(1000, "client reconnect");
      }
      if (wsRef.current === ws) wsRef.current = null;
    };
  }, [agentName, reconnectToken]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full min-h-[300px] rounded-lg border border-mycel-border overflow-hidden"
    />
  );
}
