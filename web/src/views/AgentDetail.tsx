import { useCallback, useEffect, useRef, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api } from "../api/client";
import type { Agent } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatusBadge } from "../components/StatusBadge";
import { StatsTab as StatsTabComponent } from "../components/StatsTab";
import { WebTerminal } from "../components/WebTerminal";
import { stripAnsi } from "../utils/text";

function RoleBadge({ role }: { role: string }) {
  return (
    <span className="inline-block px-2 py-0.5 rounded text-xs font-medium bg-bc-accent/20 text-bc-accent">
      {role}
    </span>
  );
}

function MetadataRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex items-start gap-2 py-1.5 border-b border-bc-border/30 last:border-0">
      <span className="text-bc-muted text-sm w-32 shrink-0">{label}</span>
      <span className="text-sm break-all">{value ?? "\u2014"}</span>
    </div>
  );
}

function formatTime(t?: string): string {
  if (!t) return "\u2014";
  try {
    const d = new Date(t);
    if (isNaN(d.getTime())) return "\u2014";
    return d.toLocaleString();
  } catch {
    return "\u2014";
  }
}

/* ───────────────────────── Tab types ───────────────────────── */

type Tab = "logs" | "terminal" | "overview" | "stats" | "role";

const TABS: { key: Tab; label: string; shortcut: string }[] = [
  { key: "logs", label: "Logs", shortcut: "1" },
  { key: "terminal", label: "Terminal", shortcut: "2" },
  { key: "overview", label: "Overview", shortcut: "3" },
  { key: "stats", label: "Stats", shortcut: "4" },
  { key: "role", label: "Role", shortcut: "5" },
];

/* ───────────────────────── Tab content ───────────────────────── */

function LogsTab({
  outputLines,
  outputRef,
}: {
  outputLines: string[];
  outputRef: React.RefObject<HTMLPreElement>;
}) {
  return (
    <div className="space-y-2">
      <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
        Live Output
      </h2>
      <pre
        ref={outputRef}
        className="rounded-lg border border-bc-border/50 bg-bc-bg p-4 text-xs leading-relaxed overflow-y-auto overflow-x-hidden max-h-[50vh] md:max-h-[70vh] whitespace-pre-wrap break-words text-bc-text/90 shadow-inner w-full min-w-0"
        style={{
          fontFamily:
            "'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
        }}
      >
        {outputLines.length > 0 ? (
          outputLines.join("\n")
        ) : (
          <span className="text-bc-muted italic">
            No output yet. Agent may be idle or stopped.
          </span>
        )}
      </pre>
    </div>
  );
}

function OverviewTab({ agent }: { agent: Agent }) {
  return (
    <div className="space-y-6">
      {/* Task */}
      {agent.task && (
        <div className="rounded border border-bc-border bg-bc-surface p-3">
          <span className="text-xs text-bc-muted uppercase tracking-wide">
            Current Task
          </span>
          <p className="mt-1 text-sm">{agent.task}</p>
        </div>
      )}

      {/* Identity */}
      <div className="space-y-2">
        <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
          Identity
        </h2>
        <div className="rounded border border-bc-border bg-bc-surface p-4">
          <MetadataRow label="Name" value={agent.name} />
          <MetadataRow label="Role" value={agent.role} />
          <MetadataRow
            label="State"
            value={<StatusBadge status={agent.state} />}
          />
          <MetadataRow label="Tool" value={agent.tool || "\u2014"} />
          <MetadataRow label="Runtime" value={agent.runtime_backend || "\u2014"} />
        </div>
      </div>

      {/* Hierarchy */}
      <div className="space-y-2">
        <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
          Hierarchy
        </h2>
        <div className="rounded border border-bc-border bg-bc-surface p-4">
          <MetadataRow label="Parent" value={agent.parent_id || "\u2014"} />
          <MetadataRow
            label="Children"
            value={
              agent.children && agent.children.length > 0
                ? agent.children.join(", ")
                : "\u2014"
            }
          />
        </div>
      </div>

      {/* Paths */}
      <div className="space-y-2">
        <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
          Paths
        </h2>
        <div className="rounded border border-bc-border bg-bc-surface p-4">
          <MetadataRow label="Session" value={agent.session || "\u2014"} />
        </div>
      </div>

      {/* Timestamps */}
      <div className="space-y-2">
        <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
          Timestamps
        </h2>
        <div className="rounded border border-bc-border bg-bc-surface p-4">
          <MetadataRow label="Created" value={formatTime(agent.created_at)} />
          <MetadataRow label="Started" value={formatTime(agent.started_at)} />
          <MetadataRow label="Updated" value={formatTime(agent.updated_at)} />
          <MetadataRow label="Stopped" value={formatTime(agent.stopped_at)} />
        </div>
      </div>
    </div>
  );
}

function StatsTab({ agent }: { agent: Agent }) {
  return <StatsTabComponent agent={agent} />;
}

function RoleTab({ agent }: { agent: Agent }) {
  return (
    <div className="space-y-2">
      <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
        Role
      </h2>
      <div className="rounded border border-bc-border bg-bc-surface p-4">
        <MetadataRow label="Role" value={<RoleBadge role={agent.role} />} />
        <MetadataRow label="Tool" value={agent.tool || "\u2014"} />
        <MetadataRow label="Parent" value={agent.parent_id || "\u2014"} />
        <MetadataRow
          label="Children"
          value={
            agent.children && agent.children.length > 0
              ? agent.children.join(", ")
              : "\u2014"
          }
        />
      </div>
    </div>
  );
}

/* ───────────────────────── Main component ───────────────────────── */

export function AgentDetail() {
  const { name } = useParams<{ name: string }>();
  const [activeTab, setActiveTab] = useState<Tab>("logs");
  const [outputLines, setOutputLines] = useState<string[]>([]);
  const [message, setMessage] = useState("");
  const [sending, setSending] = useState(false);
  const outputRef = useRef<HTMLPreElement>(null);
  const { subscribe } = useWebSocket();

  const agentFetcher = useCallback(async () => {
    if (!name) throw new Error("No agent name");
    return api.getAgent(name);
  }, [name]);

  const {
    data: agent,
    loading,
    error,
    refresh,
  } = usePolling<Agent>(agentFetcher, 3000);

  // Poll peek output every 2 seconds for reliable updates
  useEffect(() => {
    if (!name) return;

    const fetchPeek = () => {
      api
        .getAgentPeek(name, 200)
        .then(({ output }) => {
          if (output) {
            setOutputLines(stripAnsi(output).split("\n"));
          }
        })
        .catch(() => {
          // Peek may fail for stopped agents -- ignore
        });
    };

    fetchPeek();
    const interval = setInterval(fetchPeek, 2000);
    return () => clearInterval(interval);
  }, [name]);

  // Stream live output via SSE
  useEffect(() => {
    if (!name) return;

    const es = new EventSource(
      `/api/agents/${encodeURIComponent(name)}/output`,
    );

    es.onmessage = (e: MessageEvent) => {
      try {
        const parsed = JSON.parse(e.data as string) as { output: string };
        if (parsed.output) {
          const newLines = stripAnsi(parsed.output).split("\n");
          setOutputLines((prev) => [...prev, ...newLines].slice(-500));
        }
      } catch {
        // ignore malformed events
      }
    };

    es.addEventListener("agent.output", ((e: MessageEvent) => {
      try {
        const parsed = JSON.parse(e.data as string) as { output: string };
        if (parsed.output) {
          const newLines = stripAnsi(parsed.output).split("\n");
          setOutputLines((prev) => [...prev, ...newLines].slice(-500));
        }
      } catch {
        // ignore
      }
    }) as EventListener);

    es.onerror = () => {
      // SSE reconnects automatically; no action needed
    };

    return () => {
      es.close();
    };
  }, [name]);

  // Auto-scroll output to bottom
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [outputLines]);

  // Refresh on agent state changes
  useEffect(() => {
    return subscribe("agent.state_changed", () => void refresh());
  }, [subscribe, refresh]);

  // Keyboard shortcuts: 1-4 to switch tabs
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      // Don't intercept when typing in an input
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;

      const idx = parseInt(e.key, 10);
      const tab = TABS[idx - 1];
      if (idx >= 1 && idx <= TABS.length && tab) {
        setActiveTab(tab.key);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const handleSend = async () => {
    if (!name || !message.trim()) return;
    setSending(true);
    try {
      await api.sendToAgent(name, message);
      setMessage("");
    } finally {
      setSending(false);
    }
  };

  if (loading && !agent) {
    return <div className="p-6 text-bc-muted">Loading agent...</div>;
  }
  if (error && !agent) {
    return (
      <div className="p-6 space-y-2">
        <div className="text-bc-error">Error: {error}</div>
        <Link to="/agents" className="text-sm text-bc-accent hover:underline">
          Back to agents
        </Link>
      </div>
    );
  }
  if (!agent) return null;

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto p-6 space-y-4">
        {/* Breadcrumb + Header */}
        <div className="flex items-center gap-4">
          <Link
            to="/agents"
            className="text-bc-muted hover:text-bc-text text-sm"
          >
            &larr; Agents
          </Link>
          <h1 className="text-xl font-bold">{agent.name}</h1>
          <RoleBadge role={agent.role} />
          <StatusBadge status={agent.state} />
        </div>

        {/* Tab bar */}
        <div className="flex flex-wrap gap-1 border-b border-bc-border">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-sm font-medium transition-colors relative ${
                activeTab === tab.key
                  ? "text-bc-accent"
                  : "text-bc-muted hover:text-bc-text"
              }`}
            >
              {tab.label}
              <span className="ml-1.5 text-[10px] text-bc-muted/60">
                {tab.shortcut}
              </span>
              {activeTab === tab.key && (
                <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-bc-accent" />
              )}
            </button>
          ))}
        </div>

        {/* Tab content */}
        {activeTab === "logs" && (
          <LogsTab outputLines={outputLines} outputRef={outputRef} />
        )}
        {activeTab === "terminal" && (
          <div className="space-y-2">
            <h2 className="text-sm font-medium text-bc-muted uppercase tracking-wide">
              Interactive Terminal
            </h2>
            {agent.state === "stopped" || agent.state === "error" ? (
              <div className="rounded border border-bc-border bg-bc-surface p-4 text-bc-muted text-sm">
                Agent is not active. Start the agent to attach to its terminal.
              </div>
            ) : (
              <div className="h-[60vh]">
                <WebTerminal agentName={agent.name} />
              </div>
            )}
          </div>
        )}
        {activeTab === "overview" && <OverviewTab agent={agent} />}
        {activeTab === "stats" && <StatsTab agent={agent} />}
        {activeTab === "role" && <RoleTab agent={agent} />}
      </div>

      {/* Message input bar -- always visible at bottom */}
      <div className="shrink-0 border-t border-bc-border p-4">
        <div className="flex gap-2">
          <input
            type="text"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void handleSend();
            }}
            placeholder="Send message to agent..."
            className="flex-1 bg-bc-bg border border-bc-border rounded px-3 py-1.5 text-sm focus:outline-none focus:border-bc-accent"
          />
          <button
            onClick={() => void handleSend()}
            disabled={sending || !message.trim()}
            className="px-3 py-1.5 bg-bc-accent text-bc-bg rounded text-sm font-medium disabled:opacity-50"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
}
