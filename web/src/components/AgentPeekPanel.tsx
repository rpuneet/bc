import { useEffect, useRef, useState, useCallback } from "react";
import { StatusBadge } from "./StatusBadge";
import type { Agent } from "../api/client";
import { api } from "../api/client";
import { usePolling } from "../hooks/usePolling";

/** Strip ANSI escape codes from terminal output. */
function stripAnsi(text: string): string {
  // Covers CSI sequences, OSC sequences, and simple escape codes
  return text.replace(
    // eslint-disable-next-line no-control-regex
    /\x1b\[[0-9;]*[A-Za-z]|\x1b\][^\x07]*\x07|\x1b[()][AB012]|\x1b[=>]|\x1b\[[?]?[0-9;]*[hlm]/g,
    "",
  );
}

interface AgentPeekPanelProps {
  agentName: string;
  onClose: () => void;
}

export function AgentPeekPanel({ agentName, onClose }: AgentPeekPanelProps) {
  const [outputLines, setOutputLines] = useState<string[]>([]);
  const [sseError, setSseError] = useState(false);
  const outputRef = useRef<HTMLPreElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);

  // Fetch agent details for header metadata
  const agentFetcher = useCallback(async () => {
    return api.getAgent(agentName);
  }, [agentName]);
  const { data: agent } = usePolling<Agent>(agentFetcher, 5000);

  // Fetch initial output via peek endpoint
  useEffect(() => {
    setOutputLines([]);
    setSseError(false);

    api
      .getAgentPeek(agentName, 100)
      .then(({ output }) => {
        if (output) {
          setOutputLines(stripAnsi(output).split("\n"));
        }
      })
      .catch(() => {
        // Peek may fail for stopped agents — ignore
      });
  }, [agentName]);

  // Connect to SSE stream for live terminal output
  useEffect(() => {
    const es = new EventSource(
      `/api/agents/${encodeURIComponent(agentName)}/output`,
    );
    let errorCount = 0;

    const handleOutputEvent = (e: MessageEvent) => {
      try {
        const parsed = JSON.parse(e.data as string) as { output?: string };
        if (parsed.output) {
          const newLines = stripAnsi(parsed.output).split("\n");
          setOutputLines((prev) => [...prev, ...newLines].slice(-500));
        }
      } catch {
        // ignore malformed data
      }
    };

    // Initial snapshot comes as a plain "message" event (no event: field)
    es.onmessage = handleOutputEvent;

    // Incremental updates arrive as named "agent.output" events
    es.addEventListener("agent.output", handleOutputEvent as EventListener);

    es.onerror = () => {
      errorCount++;
      // After several failed reconnects, assume agent is not running
      if (errorCount > 3) {
        setSseError(true);
        es.close();
      }
    };

    return () => {
      es.close();
    };
  }, [agentName]);

  // Auto-scroll when output grows, only if near bottom
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const isNearBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight <
      120;
    if (isNearBottom) {
      container.scrollTop = container.scrollHeight;
    }
  }, [outputLines]);

  return (
    <div className="w-[420px] shrink-0 border-l border-mycel-border flex flex-col bg-mycel-bg">
      {/* Header */}
      <div className="px-4 py-2 border-b border-mycel-border bg-mycel-surface flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-sm font-medium truncate">{agentName}</span>
          {agent && <StatusBadge status={agent.state} />}
        </div>
        <button
          onClick={onClose}
          className="text-mycel-muted hover:text-mycel-text text-xs ml-2 shrink-0 transition-colors"
          aria-label="Close peek panel"
        >
          close
        </button>
      </div>

      {/* Agent metadata */}
      {agent && (
        <div className="px-4 py-1.5 border-b border-mycel-border text-xs text-mycel-muted flex gap-3">
          <span>Role: {agent.role}</span>
          <span>Tool: {agent.tool || "\u2014"}</span>
          {agent.total_cost_usd != null && (
            <span>Cost: ${agent.total_cost_usd.toFixed(4)}</span>
          )}
        </div>
      )}

      {/* Live output */}
      <div ref={scrollContainerRef} className="flex-1 overflow-auto">
        <pre
          ref={outputRef}
          className="p-3 text-xs font-mono whitespace-pre-wrap break-words leading-relaxed text-mycel-text-2"
        >
          {outputLines.length > 0
            ? outputLines.join("\n")
            : sseError
              ? "Agent not running."
              : "Connecting..."}
        </pre>
      </div>
    </div>
  );
}
