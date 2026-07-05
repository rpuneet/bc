import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import { stripAnsi } from "../utils/text";

interface InlineTerminalProps {
  agentName: string;
  lines?: number;
}

export function InlineTerminal({ agentName, lines = 10 }: InlineTerminalProps) {
  const [outputLines, setOutputLines] = useState<string[]>([]);
  const [sseError, setSseError] = useState(false);
  const [loading, setLoading] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);

  // Fetch initial snapshot via peek endpoint
  useEffect(() => {
    setOutputLines([]);
    setSseError(false);
    setLoading(true);

    api
      .getAgentPeek(agentName, lines)
      .then(({ output }) => {
        if (output) {
          setOutputLines(stripAnsi(output).split("\n"));
        }
      })
      .catch(() => {
        // Peek may fail for stopped agents
      })
      .finally(() => {
        setLoading(false);
      });
  }, [agentName, lines]);

  // Connect to SSE stream for live output
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

    es.onmessage = handleOutputEvent;
    es.addEventListener("agent.output", handleOutputEvent as EventListener);

    es.onerror = () => {
      errorCount++;
      if (errorCount > 3) {
        setSseError(true);
        es.close();
      }
    };

    return () => {
      es.close();
    };
  }, [agentName]);

  // Auto-scroll when near bottom
  useEffect(() => {
    const container = scrollRef.current;
    if (!container) return;
    const isNearBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight <
      80;
    if (isNearBottom) {
      container.scrollTop = container.scrollHeight;
    }
  }, [outputLines]);

  return (
    <div className="bg-mycel-bg border-t border-mycel-border px-4 py-3">
      <div
        ref={scrollRef}
        className="rounded-md bg-mycel-bg border border-mycel-border p-3 font-mono text-xs leading-relaxed text-mycel-text max-h-48 overflow-auto whitespace-pre-wrap"
      >
        {loading ? (
          <span className="text-mycel-muted animate-pulse">Loading output...</span>
        ) : outputLines.length > 0 ? (
          outputLines.join("\n")
        ) : sseError ? (
          <span className="text-mycel-muted">Agent not running.</span>
        ) : (
          <span className="text-mycel-muted">No output available.</span>
        )}
      </div>
      <div className="mt-2 text-right">
        <Link
          to={`/agents/${encodeURIComponent(agentName)}`}
          onClick={(e) => e.stopPropagation()}
          className="text-xs text-mycel-accent hover:underline"
        >
          View Detail &rarr;
        </Link>
      </div>
    </div>
  );
}
