import { useCallback, useEffect, useRef, useState } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type { Agent, AgentActivityItem } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatusBadge } from "../components/StatusBadge";
import { StatsTab as StatsTabComponent } from "../components/StatsTab";
import { WebTerminal } from "../components/WebTerminal";
import { stripAnsi } from "../utils/text";
import { AgentIcon } from "../components/agent-ui";
import { LoopIconButton, RalphLoopModal, useRalphLoop } from "../components/RalphLoopModal";

/* ═══════════════════════════════════════════════════════════════════
   Utility
   ═══════════════════════════════════════════════════════════════════ */

const MONO =
  "'JetBrains Mono', 'Fira Code', 'Space Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

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

function formatRelative(t?: string): string {
  if (!t) return "";
  try {
    const d = new Date(t);
    if (isNaN(d.getTime())) return "";
    const diffMs = Date.now() - d.getTime();
    const diffSec = Math.floor(Math.abs(diffMs) / 1000);
    if (diffSec < 60) return `${String(diffSec)}s ago`;
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${String(diffMin)}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${String(diffHr)}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay < 30) return `${String(diffDay)}d ago`;
    return d.toLocaleDateString();
  } catch {
    return "";
  }
}

/* ═══════════════════════════════════════════════════════════════════
   Tab types — v2: Terminal / Activity / Config / Stats
   ═══════════════════════════════════════════════════════════════════ */

type Tab = "terminal" | "activity" | "config" | "stats";

const TABS: { key: Tab; label: string; shortcut: string }[] = [
  { key: "terminal", label: "Terminal", shortcut: "1" },
  { key: "activity", label: "Activity", shortcut: "2" },
  { key: "config", label: "Config", shortcut: "3" },
  { key: "stats", label: "Stats", shortcut: "4" },
];

/* ═══════════════════════════════════════════════════════════════════
   Section chrome
   ═══════════════════════════════════════════════════════════════════ */

function SectionRule({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-4 flex items-center gap-3">
      <span
        className="text-[10px] font-bold uppercase tracking-[0.2em] text-bc-muted/70"
        style={{ fontFamily: MONO }}
      >
        {children}
      </span>
      <span className="flex-1 h-px bg-gradient-to-r from-bc-border/50 to-transparent" />
    </div>
  );
}

function MetaCell({
  label,
  children,
  mono,
}: {
  label: string;
  children: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="space-y-1">
      <dt
        className="text-[9px] font-bold uppercase tracking-[0.2em] text-bc-muted/60"
        style={{ fontFamily: MONO }}
      >
        {label}
      </dt>
      <dd
        className="text-[13px] text-bc-text/90 leading-tight break-all"
        style={mono ? { fontFamily: MONO } : undefined}
      >
        {children}
      </dd>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 1 — Terminal (default)
   Immersive, zero-chrome. Fills the entire flex area.
   ═══════════════════════════════════════════════════════════════════ */

function TerminalTab({
  agent,
  outputLines,
  outputRef,
}: {
  agent: Agent;
  outputLines: string[];
  outputRef: React.RefObject<HTMLPreElement>;
}) {
  const isStopped = agent.state === "stopped" || agent.state === "error";
  const [attached, setAttached] = useState(false);

  // Reset attached state when agent stops
  useEffect(() => {
    if (isStopped) setAttached(false);
  }, [isStopped]);

  if (isStopped) {
    return (
      <div className="flex flex-col flex-1 min-h-0">
        {/* Stopped banner */}
        <div className="flex items-center justify-between px-4 py-2 border-b border-bc-border/40 bg-bc-surface/30">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-bc-muted/40" />
            <span className="text-xs text-bc-muted" style={{ fontFamily: MONO }}>
              {agent.name}
            </span>
            <span className="text-[10px] text-bc-muted/50">
              {agent.state === "error" ? "errored" : "stopped"}
              {agent.stopped_at ? ` \u00b7 ${formatRelative(agent.stopped_at)}` : ""}
            </span>
          </div>
          <button
            type="button"
            className="px-2.5 py-1 rounded text-[11px] font-medium bg-bc-accent/15 text-bc-accent hover:bg-bc-accent/25 transition-colors"
            style={{ fontFamily: MONO }}
          >
            Start agent
          </button>
        </div>

        {/* Last captured output */}
        <pre
          ref={outputRef}
          className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-4 text-xs leading-relaxed whitespace-pre-wrap break-words text-bc-text/70 bg-bc-bg"
          style={{ fontFamily: MONO }}
        >
          {outputLines.length > 0 ? (
            outputLines.join("\n")
          ) : (
            <span className="text-bc-muted/40 italic">
              No captured output from the last run.
            </span>
          )}
        </pre>
      </div>
    );
  }

  // Running — show overlay first, then WebTerminal when attached
  if (!attached) {
    return (
      <div className="flex-1 min-h-0 flex items-center justify-center">
        <div className="flex flex-col items-center gap-4">
          <div className="w-12 h-12 rounded-full bg-bc-accent/20 flex items-center justify-center">
            <span className="w-3 h-3 rounded-full bg-bc-accent animate-pulse" />
          </div>
          <span className="text-sm text-bc-text/80" style={{ fontFamily: MONO }}>
            {agent.name} is running
          </span>
          <button
            type="button"
            onClick={() => setAttached(true)}
            className="px-4 py-2 rounded bg-bc-accent/15 text-bc-accent text-sm hover:bg-bc-accent/25 transition-colors"
            style={{ fontFamily: MONO }}
          >
            Click to attach terminal
          </button>
          {agent.task && (
            <span className="text-xs text-bc-muted" style={{ fontFamily: MONO }}>
              {agent.task}
            </span>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Thin top bar with detach button */}
      <div className="flex items-center justify-end px-3 py-1 border-b border-bc-border/30 bg-bc-surface/20">
        <button
          type="button"
          onClick={() => setAttached(false)}
          className="text-[11px] text-bc-muted/60 hover:text-bc-muted transition-colors"
          style={{ fontFamily: MONO }}
        >
          [Detach]
        </button>
      </div>
      <div className="flex-1 min-h-0">
        <WebTerminal agentName={agent.name} />
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 2 — Activity
   Vertical event stream with task banner + timeline
   ═══════════════════════════════════════════════════════════════════ */

interface TimelineEvent {
  key: string;
  label: string;
  timestamp?: string;
  detail?: string;
  active: boolean;
}

function buildTimeline(agent: Agent): TimelineEvent[] {
  const events: TimelineEvent[] = [];
  const isRunning = agent.state !== "stopped" && agent.state !== "error";

  if (agent.created_at) {
    events.push({
      key: "created",
      label: "Created",
      timestamp: agent.created_at,
      active: false,
    });
  }
  if (agent.started_at) {
    events.push({
      key: "started",
      label: "Started",
      timestamp: agent.started_at,
      active: false,
    });
  }
  if (isRunning) {
    events.push({
      key: "current",
      label:
        agent.state === "working"
          ? "Working"
          : agent.state === "starting"
            ? "Starting"
            : agent.state === "idle"
              ? "Idle"
              : "Active",
      timestamp: agent.updated_at,
      detail: agent.task,
      active: true,
    });
  } else if (agent.stopped_at) {
    events.push({
      key: "stopped",
      label: agent.state === "error" ? "Errored" : "Stopped",
      timestamp: agent.stopped_at,
      detail: agent.task,
      active: true,
    });
  }
  return events;
}

function humanizeEvent(type: string): string {
  const cleaned = type.replace(/^agent\./, "").replace(/[._-]/g, " ");
  return cleaned.charAt(0).toUpperCase() + cleaned.slice(1);
}

function eventIcon(label: string): string {
  const lower = label.toLowerCase();
  if (lower === "created" || lower === "sessionstart") return "▶";
  if (lower === "started") return "▶";
  if (lower === "working" || lower === "tooluse") return "🔧";
  if (lower === "starting") return "⚡";
  if (lower === "idle") return "○";
  if (lower === "stopped" || lower === "sessionend") return "⏹";
  if (lower === "errored") return "✗";
  if (lower === "taskcreate" || lower === "taskcompleted") return "◎";
  if (lower === "permissionrequest") return "🔐";
  return "·";
}

type EventFilter = "all" | "tools" | "tasks" | "lifecycle";

const FILTER_LABELS: { key: EventFilter; label: string }[] = [
  { key: "all", label: "All" },
  { key: "tools", label: "Tools" },
  { key: "tasks", label: "Tasks" },
  { key: "lifecycle", label: "Lifecycle" },
];

function matchesFilter(label: string, filter: EventFilter): boolean {
  if (filter === "all") return true;
  const lower = label.toLowerCase();
  if (filter === "tools") {
    return lower === "working" || lower === "tooluse" || lower === "🔧";
  }
  if (filter === "tasks") {
    return (
      lower === "taskcreate" ||
      lower === "taskcompleted" ||
      lower.includes("task")
    );
  }
  if (filter === "lifecycle") {
    return (
      lower === "created" ||
      lower === "sessionstart" ||
      lower === "started" ||
      lower === "starting" ||
      lower === "idle" ||
      lower === "stopped" ||
      lower === "sessionend" ||
      lower === "errored" ||
      lower === "active"
    );
  }
  return true;
}

function ActivityTab({ agent }: { agent: Agent }) {
  const [activity, setActivity] = useState<AgentActivityItem[]>([]);
  const [activeFilter, setActiveFilter] = useState<EventFilter>("all");

  useEffect(() => {
    let cancelled = false;
    api
      .getAgentActivity(agent.name)
      .then((items) => {
        if (!cancelled) setActivity(items);
      })
      .catch(() => {
        /* best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, [agent.name]);

  // SSE live events
  useEffect(() => {
    if (agent.state === "stopped" || agent.state === "error") return;

    const es = new EventSource(`/api/agents/${encodeURIComponent(agent.name)}/events`);

    es.addEventListener("hook", (e: MessageEvent) => {
      try {
        const data = JSON.parse(String(e.data)) as {
          event?: string;
          timestamp?: string;
          tool_name?: string;
          tool_input?: { command?: string };
          message?: string;
        };
        setActivity((prev) =>
          [
            {
              event: data.event ?? "unknown",
              timestamp: data.timestamp ?? new Date().toISOString(),
              message: data.tool_name
                ? `${data.tool_name}${data.tool_input?.command ? ": " + data.tool_input.command : ""}`
                : (data.message ?? ""),
            },
            ...prev,
          ].slice(0, 50),
        );
      } catch {
        /* ignore malformed events */
      }
    });

    es.onerror = () => {
      /* auto-reconnects */
    };

    return () => es.close();
  }, [agent.name, agent.state]);

  const isStopped = agent.state === "stopped" || agent.state === "error";
  const isRunning = !isStopped;
  const derivedTimeline = buildTimeline(agent);
  const allTimeline: TimelineEvent[] =
    activity.length > 0
      ? activity.slice(0, 12).map((it, idx) => ({
          key: `${it.event}-${String(idx)}`,
          label: humanizeEvent(it.event),
          timestamp: it.timestamp,
          detail: it.message,
          active: idx === 0,
        }))
      : derivedTimeline;

  const timeline = allTimeline.filter((evt) =>
    matchesFilter(evt.label, activeFilter),
  );

  const lastActivity =
    agent.stopped_at ?? agent.updated_at ?? agent.started_at ?? agent.created_at;

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-3xl mx-auto space-y-8">
        {/* ── CURRENT TASK BANNER ── */}
        {(agent.task || isStopped) && (
          <div
            className={`rounded-md border px-4 py-3 transition-colors ${
              isStopped
                ? "border-bc-border/40 bg-bc-surface/30"
                : "border-bc-accent/20 bg-bc-accent/[0.04]"
            }`}
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div
                  className="text-[9px] font-bold uppercase tracking-[0.2em] text-bc-muted/60 mb-1"
                  style={{ fontFamily: MONO }}
                >
                  {isStopped ? "last task" : "current task"}
                </div>
                <p
                  className="text-sm text-bc-text/90 break-words leading-relaxed"
                  style={{ fontFamily: MONO }}
                >
                  {agent.task ?? (
                    <span className="text-bc-muted/40 italic">none</span>
                  )}
                </p>
              </div>
              {lastActivity && (
                <span
                  className="text-[11px] text-bc-muted tabular-nums shrink-0 pt-0.5"
                  title={formatTime(lastActivity)}
                  style={{ fontFamily: MONO }}
                >
                  {formatRelative(lastActivity)}
                </span>
              )}
            </div>
          </div>
        )}

        {/* ── EVENT STREAM ── */}
        <section>
          {/* Section header with live indicator */}
          <div className="mb-4 flex items-center gap-3">
            <span
              className="text-[10px] font-bold uppercase tracking-[0.2em] text-bc-muted/70"
              style={{ fontFamily: MONO }}
            >
              Event Stream
            </span>
            <span className="flex-1 h-px bg-gradient-to-r from-bc-border/50 to-transparent" />
            {/* Live indicator */}
            <span
              className="flex items-center gap-1 text-[10px] tabular-nums"
              style={{ fontFamily: MONO }}
            >
              <span
                className={`w-1.5 h-1.5 rounded-full ${isRunning ? "bg-green-500" : "bg-bc-muted/40"}`}
              />
              <span className={isRunning ? "text-green-400" : "text-bc-muted/40"}>
                {isRunning ? "Live" : "Offline"}
              </span>
            </span>
          </div>

          {/* Filter chips */}
          <div className="flex items-center gap-1.5 mb-4 flex-wrap">
            {FILTER_LABELS.map((f) => (
              <button
                key={f.key}
                type="button"
                onClick={() => setActiveFilter(f.key)}
                className={`px-2 py-0.5 rounded border text-[10px] font-medium transition-colors ${
                  activeFilter === f.key
                    ? "border-bc-accent/30 bg-bc-accent/15 text-bc-accent"
                    : "border-bc-border/30 text-bc-muted/60 hover:text-bc-muted hover:border-bc-border/50"
                }`}
                style={{ fontFamily: MONO }}
              >
                {f.label}
              </button>
            ))}
          </div>

          {timeline.length === 0 ? (
            <p className="text-xs text-bc-muted/40 italic pl-1">
              {allTimeline.length === 0
                ? "No activity recorded yet."
                : "No events match this filter."}
            </p>
          ) : (
            <ol className="relative ml-1.5">
              {/* Vertical rail */}
              <span
                aria-hidden
                className="absolute left-[3.5px] top-2.5 bottom-2.5 w-px bg-bc-border/40"
              />
              {timeline.map((evt) => (
                <li key={evt.key} className="relative pl-7 pb-5 last:pb-0">
                  {/* Dot */}
                  <span
                    aria-hidden
                    className={`absolute left-0 top-[7px] w-2 h-2 rounded-full border-[1.5px] transition-colors ${
                      evt.active
                        ? "bg-bc-accent border-bc-accent shadow-[0_0_6px_rgba(var(--bc-accent-rgb,255,165,0),0.4)]"
                        : "bg-bc-bg border-bc-muted/50"
                    }`}
                  />
                  <div className="flex items-baseline justify-between gap-4">
                    <span
                      className={`text-[13px] font-semibold ${
                        evt.active ? "text-bc-accent" : "text-bc-text/80"
                      }`}
                      style={{ fontFamily: MONO }}
                    >
                      <span className="mr-1.5 opacity-70">{eventIcon(evt.label)}</span>
                      {evt.label}
                    </span>
                    {evt.timestamp && (
                      <span
                        className="text-[10px] text-bc-muted/60 tabular-nums shrink-0"
                        title={formatTime(evt.timestamp)}
                        style={{ fontFamily: MONO }}
                      >
                        {formatRelative(evt.timestamp)}
                      </span>
                    )}
                  </div>
                  {evt.detail && (
                    <p className="mt-1 text-xs text-bc-muted/70 break-words leading-relaxed">
                      {evt.detail}
                    </p>
                  )}
                </li>
              ))}
            </ol>
          )}
        </section>

        {/* Stopped note */}
        {isStopped && (
          <p
            className="text-[10px] text-bc-muted/40 italic pl-1"
            style={{ fontFamily: MONO }}
          >
            Agent is not running. Showing last known activity.
          </p>
        )}
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 3 — Config
   System prompt, MCP servers, metadata, danger zone
   ═══════════════════════════════════════════════════════════════════ */

interface AgentConfig {
  system_prompt: string;
  mcp_servers: string[];
  runtime_backend: string;
  tool: string;
  session: string;
  worktree_path: string;
  created_at: string;
  started_at: string;
}

interface MCPServer {
  name: string;
}

function ConfigTab({ agent }: { agent: Agent }) {
  const navigate = useNavigate();
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveStatus, setSaveStatus] = useState<"" | "saved" | "error">("");
  const [saveError, setSaveError] = useState("");
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // MCP server management state
  const [mcpList, setMcpList] = useState<string[] | null>(null);
  const [mcpLoading, setMcpLoading] = useState(true);
  const [mcpInput, setMcpInput] = useState("");
  const [mcpAdding, setMcpAdding] = useState(false);
  const [mcpDeleting, setMcpDeleting] = useState<string | null>(null);

  const fetchMcps = useCallback(() => {
    setMcpLoading(true);
    fetch(`/api/agents/${encodeURIComponent(agent.name)}/mcps`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${String(res.status)}`);
        return res.json() as Promise<MCPServer[]>;
      })
      .then((data) => {
        setMcpList(data.map((m) => m.name));
      })
      .catch(() => {
        setMcpList(null); // fall back to static chips
      })
      .finally(() => {
        setMcpLoading(false);
      });
  }, [agent.name]);

  useEffect(() => {
    let cancelled = false;
    setConfigLoading(true);
    fetch(`/api/agents/${encodeURIComponent(agent.name)}/config`)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${String(res.status)}`);
        return res.json() as Promise<AgentConfig>;
      })
      .then((data) => {
        if (!cancelled) {
          setConfig(data);
          setDraft(data.system_prompt ?? "");
        }
      })
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        if (!cancelled) setConfigLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [agent.name]);

  useEffect(() => {
    fetchMcps();
  }, [fetchMcps]);

  // Clear save feedback timer on unmount
  useEffect(() => {
    return () => {
      if (saveTimerRef.current !== null) {
        clearTimeout(saveTimerRef.current);
      }
    };
  }, []);

  const handleEdit = () => {
    setDraft(config?.system_prompt ?? "");
    setSaveStatus("");
    setSaveError("");
    setEditing(true);
  };

  const handleCancel = () => {
    setDraft(config?.system_prompt ?? "");
    setSaveStatus("");
    setSaveError("");
    setEditing(false);
  };

  const handleSave = () => {
    setSaving(true);
    setSaveStatus("");
    setSaveError("");
    fetch(`/api/agents/${encodeURIComponent(agent.name)}/config`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ system_prompt: draft }),
    })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${String(res.status)}`);
        return res.json() as Promise<AgentConfig>;
      })
      .then((data) => {
        setConfig(data);
        setDraft(data.system_prompt ?? draft);
        setEditing(false);
        setSaveStatus("saved");
        if (saveTimerRef.current !== null) clearTimeout(saveTimerRef.current);
        saveTimerRef.current = setTimeout(() => setSaveStatus(""), 2000);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : "unknown error";
        setSaveStatus("error");
        setSaveError(msg);
      })
      .finally(() => {
        setSaving(false);
      });
  };

  // If live fetch succeeded use that list; otherwise fall back to static record
  const mcpServers: string[] =
    mcpList !== null
      ? mcpList
      : config?.mcp_servers && config.mcp_servers.length > 0
        ? config.mcp_servers
        : (agent.mcp_servers ?? []);

  const useLiveMcp = mcpList !== null;

  const handleMcpAdd = () => {
    const name = mcpInput.trim();
    if (!name) return;
    setMcpAdding(true);
    fetch(`/api/agents/${encodeURIComponent(agent.name)}/mcps`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${String(res.status)}`);
        setMcpInput("");
        fetchMcps();
      })
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        setMcpAdding(false);
      });
  };

  const handleMcpDelete = (serverName: string) => {
    setMcpDeleting(serverName);
    fetch(
      `/api/agents/${encodeURIComponent(agent.name)}/mcps/${encodeURIComponent(serverName)}`,
      { method: "DELETE" },
    )
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${String(res.status)}`);
        fetchMcps();
      })
      .catch(() => {
        /* best-effort */
      })
      .finally(() => {
        setMcpDeleting(null);
      });
  };

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-3xl mx-auto space-y-10">
        {/* ── SYSTEM PROMPT ── */}
        <section>
          <div className="mb-4 flex items-center gap-3">
            <span
              className="text-[10px] font-bold uppercase tracking-[0.2em] text-bc-muted/70"
              style={{ fontFamily: MONO }}
            >
              System Prompt
            </span>
            <span className="flex-1 h-px bg-gradient-to-r from-bc-border/50 to-transparent" />
            {/* Action buttons */}
            {!configLoading && (
              <div className="flex items-center gap-2">
                {saveStatus === "saved" && (
                  <span
                    className="text-[11px] text-green-400 transition-opacity"
                    style={{ fontFamily: MONO }}
                  >
                    Saved
                  </span>
                )}
                {saveStatus === "error" && (
                  <span
                    className="text-[11px] text-bc-error"
                    style={{ fontFamily: MONO }}
                    title={saveError}
                  >
                    Error: {saveError}
                  </span>
                )}
                {editing ? (
                  <>
                    <span
                      className="text-[10px] text-bc-accent/60 italic"
                      style={{ fontFamily: MONO }}
                    >
                      Editing...
                    </span>
                    <button
                      type="button"
                      onClick={handleCancel}
                      disabled={saving}
                      className="px-2.5 py-1 rounded border border-bc-border/40 text-[11px] text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors disabled:opacity-40"
                      style={{ fontFamily: MONO }}
                    >
                      Cancel
                    </button>
                    <button
                      type="button"
                      onClick={handleSave}
                      disabled={saving}
                      className="px-2.5 py-1 rounded border border-bc-accent/30 bg-bc-accent/10 text-[11px] text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
                      style={{ fontFamily: MONO }}
                    >
                      {saving ? "Saving…" : "Save"}
                    </button>
                  </>
                ) : (
                  <button
                    type="button"
                    onClick={handleEdit}
                    className="px-2.5 py-1 rounded border border-bc-border/40 text-[11px] text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors"
                    style={{ fontFamily: MONO }}
                  >
                    Edit
                  </button>
                )}
              </div>
            )}
          </div>

          {configLoading ? (
            <div className="rounded-md border border-bc-border/30 bg-bc-surface/20 p-4">
              <p
                className="text-xs text-bc-muted/40 italic"
                style={{ fontFamily: MONO }}
              >
                Loading…
              </p>
            </div>
          ) : editing ? (
            <textarea
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              className="w-full min-h-[300px] rounded-md border border-bc-accent/50 bg-bc-bg/80 p-4 text-xs text-bc-text/90 leading-relaxed resize-y outline-none focus:border-bc-accent/60 transition-colors"
              style={{ fontFamily: MONO }}
              spellCheck={false}
            />
          ) : (
            <textarea
              value={config?.system_prompt ?? ""}
              readOnly
              className="w-full min-h-[300px] rounded-md border border-bc-border/40 bg-bc-bg p-4 text-xs text-bc-text/70 leading-relaxed resize-y outline-none cursor-default"
              style={{ fontFamily: MONO }}
            />
          )}
        </section>

        {/* ── MCP SERVERS ── */}
        <section>
          <SectionRule>MCP Servers</SectionRule>
          {mcpLoading ? (
            <p className="text-xs text-bc-muted/40 italic pl-1" style={{ fontFamily: MONO }}>
              Loading…
            </p>
          ) : mcpServers.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {mcpServers.map((s) => (
                <span
                  key={s}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md border border-bc-border/30 bg-bc-surface/30 text-[11px] text-bc-text/80 font-medium"
                  style={{ fontFamily: MONO }}
                >
                  <span className="w-1.5 h-1.5 rounded-full bg-bc-accent/60" />
                  {s.replace(/^mcp__/, "")}
                  {useLiveMcp && (
                    <button
                      type="button"
                      onClick={() => { handleMcpDelete(s); }}
                      disabled={mcpDeleting === s}
                      className="ml-0.5 text-bc-muted/50 hover:text-bc-error transition-colors disabled:opacity-40 leading-none"
                      title={`Remove ${s}`}
                      aria-label={`Remove MCP server ${s}`}
                    >
                      ×
                    </button>
                  )}
                </span>
              ))}
            </div>
          ) : (
            <p className="text-xs text-bc-muted/40 italic pl-1">
              No MCP servers configured.
            </p>
          )}
          {useLiveMcp && (
            <div className="mt-3 flex items-center gap-2">
              <input
                type="text"
                value={mcpInput}
                onChange={(e) => { setMcpInput(e.target.value); }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleMcpAdd();
                }}
                placeholder="mcp-server-name"
                disabled={mcpAdding}
                className="flex-1 max-w-[240px] rounded border border-bc-border/40 bg-bc-bg px-2.5 py-1 text-[11px] text-bc-text/90 placeholder:text-bc-muted/40 outline-none focus:border-bc-accent/50 transition-colors disabled:opacity-40"
                style={{ fontFamily: MONO }}
              />
              <button
                type="button"
                onClick={handleMcpAdd}
                disabled={mcpAdding || !mcpInput.trim()}
                className="px-2.5 py-1 rounded border border-bc-accent/30 bg-bc-accent/10 text-[11px] text-bc-accent hover:bg-bc-accent/20 transition-colors disabled:opacity-40"
                style={{ fontFamily: MONO }}
              >
                {mcpAdding ? "Adding…" : "+ Add MCP"}
              </button>
            </div>
          )}
        </section>

        {/* ── METADATA ── */}
        <section>
          <SectionRule>Metadata</SectionRule>
          <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-6 gap-y-4">
            <MetaCell label="Provider" mono>
              {agent.tool || "\u2014"}
            </MetaCell>
            <MetaCell label="Runtime" mono>
              {agent.runtime_backend || "\u2014"}
            </MetaCell>
            <MetaCell label="Session" mono>
              {agent.session || "\u2014"}
            </MetaCell>
            <MetaCell label="Created">
              <span className="tabular-nums">{formatTime(agent.created_at)}</span>
            </MetaCell>
            <MetaCell label="Started">
              <span className="tabular-nums">{formatTime(agent.started_at)}</span>
            </MetaCell>
            {agent.stopped_at && (
              <MetaCell label="Stopped">
                <span className="tabular-nums">{formatTime(agent.stopped_at)}</span>
              </MetaCell>
            )}
          </dl>
        </section>

        {/* ── DANGER ZONE ── */}
        <section>
          <SectionRule>Danger Zone</SectionRule>
          <div className="rounded-md border border-bc-error/20 bg-bc-error/[0.02] px-4 py-3">
          <div className="flex flex-wrap gap-2 items-center">
            {/* Clone — placeholder, navigates to /agents */}
            <button
              type="button"
              onClick={() => navigate("/agents")}
              className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-border/40 text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors"
              style={{ fontFamily: MONO }}
            >
              Clone
            </button>

            {/* Archive — placeholder */}
            <button
              type="button"
              className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-border/40 text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors"
              style={{ fontFamily: MONO }}
            >
              Archive
            </button>

            {/* Delete — with confirmation */}
            {confirmDelete ? (
              <>
                <span
                  className="text-[11px] text-bc-error/80"
                  style={{ fontFamily: MONO }}
                >
                  Are you sure?
                </span>
                <button
                  type="button"
                  disabled={deleting}
                  onClick={() => {
                    setDeleting(true);
                    fetch(`/api/agents/${encodeURIComponent(agent.name)}`, {
                      method: "DELETE",
                    })
                      .then(() => navigate("/agents"))
                      .catch(() => {
                        setDeleting(false);
                        setConfirmDelete(false);
                      });
                  }}
                  className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-error/50 bg-bc-error/10 text-bc-error hover:bg-bc-error/20 transition-colors disabled:opacity-40"
                  style={{ fontFamily: MONO }}
                >
                  {deleting ? "Deleting…" : "Confirm"}
                </button>
                <button
                  type="button"
                  disabled={deleting}
                  onClick={() => setConfirmDelete(false)}
                  className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-border/40 text-bc-muted hover:text-bc-text hover:border-bc-border transition-colors disabled:opacity-40"
                  style={{ fontFamily: MONO }}
                >
                  Cancel
                </button>
              </>
            ) : (
              <button
                type="button"
                onClick={() => setConfirmDelete(true)}
                className="px-3 py-1.5 rounded-md text-[11px] font-medium border border-bc-error/30 text-bc-error/80 hover:bg-bc-error/10 transition-colors"
                style={{ fontFamily: MONO }}
              >
                Delete
              </button>
            )}
          </div>
          </div>
        </section>
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Tab 4 — Stats
   Wraps existing StatsTabComponent, adds stopped-agent context
   ═══════════════════════════════════════════════════════════════════ */

function StatsTab({ agent }: { agent: Agent }) {
  const isStopped = agent.state === "stopped" || agent.state === "error";
  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-4xl mx-auto space-y-4">
        {isStopped && (
          <p
            className="text-[10px] text-bc-muted/40 italic"
            style={{ fontFamily: MONO }}
          >
            Agent is not running. Stats show last known values.
          </p>
        )}
        <StatsTabComponent agent={agent} />
      </div>
    </div>
  );
}

/* ═══════════════════════════════════════════════════════════════════
   Main component
   ═══════════════════════════════════════════════════════════════════ */

export function AgentDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<Tab>("terminal");
  const [outputLines, setOutputLines] = useState<string[]>([]);
  const [loopOpen, setLoopOpen] = useState(false);
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

  // Poll peek output every 2s
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
          /* peek may fail for stopped agents */
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
        /* ignore */
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
        /* ignore */
      }
    }) as EventListener);
    es.onerror = () => {
      /* SSE reconnects automatically */
    };
    return () => {
      es.close();
    };
  }, [name]);

  // Auto-scroll
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [outputLines]);

  // Refresh on agent state changes
  useEffect(() => {
    return subscribe("agent.state_changed", () => void refresh());
  }, [subscribe, refresh]);

  // Keyboard shortcuts: 1-4 for tabs, s for start/stop, Esc for back
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA") return;

      switch (e.key) {
        case "1":
          setActiveTab("terminal");
          break;
        case "2":
          setActiveTab("activity");
          break;
        case "3":
          setActiveTab("config");
          break;
        case "4":
          setActiveTab("stats");
          break;
        case "Escape":
          navigate("/agents");
          break;
      }
    };
    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
    };
  }, [navigate]);

  /* ─── Loading / Error ─── */

  if (loading && !agent) {
    return (
      <div className="flex items-center justify-center h-full">
        <span className="text-sm text-bc-muted/50" style={{ fontFamily: MONO }}>
          loading\u2026
        </span>
      </div>
    );
  }
  if (error && !agent) {
    return (
      <div className="p-6 space-y-3">
        <div className="text-sm text-bc-error" style={{ fontFamily: MONO }}>
          error: {error}
        </div>
        <Link
          to="/agents"
          className="text-xs text-bc-accent hover:underline"
          style={{ fontFamily: MONO }}
        >
          \u2190 back to agents
        </Link>
      </div>
    );
  }
  if (!agent) return null;

  const lastSeen =
    agent.updated_at ?? agent.started_at ?? agent.created_at;

  /* ─── Render ─── */

  return (
    <div className="flex flex-col h-full">
      {/* ═══ HEADER — single compact line: breadcrumb + identity + badges ═══ */}
      <header className="shrink-0 px-6 pt-4 pb-0 space-y-2.5">
        {/* Combined breadcrumb + identity — one line */}
        <div className="flex items-center gap-2 flex-wrap min-w-0">
          <Link
            to="/agents"
            className="text-xs text-bc-muted/50 hover:text-bc-text transition-colors shrink-0"
            style={{ fontFamily: MONO }}
          >
            Agents
          </Link>
          <span className="text-xs text-bc-muted/30">/</span>
          <AgentIcon state={agent.state} size={36} />
          <span
            className="text-sm font-bold text-bc-text tracking-tight shrink-0"
            style={{ fontFamily: MONO }}
          >
            {agent.name}
          </span>

          {/* Inline badges */}
          {agent.runtime_backend && (
            <span
              className="px-1.5 py-0.5 rounded text-[10px] font-medium border border-bc-border/30 text-bc-muted/60 bg-bc-surface/20"
              style={{ fontFamily: MONO }}
            >
              {agent.runtime_backend}
            </span>
          )}
          {agent.tool && (
            <span
              className="px-1.5 py-0.5 rounded text-[10px] font-medium border border-bc-border/30 text-bc-muted/60 bg-bc-surface/20"
              style={{ fontFamily: MONO }}
            >
              {agent.tool}
            </span>
          )}

          <StatusBadge status={agent.state} />

          {/* Ralph Loop icon */}
          <LoopIconButton agentName={agent.name} onClick={() => setLoopOpen(true)} />

          {/* Live activity hint */}
          {agent.task && (
            <span
              className="text-[10px] text-bc-muted/50 truncate max-w-[280px]"
              title={agent.task}
              style={{ fontFamily: MONO }}
            >
              {agent.task}
            </span>
          )}

          {lastSeen && (
            <span
              className="text-[10px] text-bc-muted/30 tabular-nums ml-auto shrink-0"
              title={formatTime(lastSeen)}
              style={{ fontFamily: MONO }}
            >
              {formatRelative(lastSeen)}
            </span>
          )}
        </div>

        {/* Tab bar */}
        <nav className="flex gap-0 border-b border-bc-border/40 -mx-6 px-6">
          {TABS.map((tab) => {
            const isActive = activeTab === tab.key;
            return (
              <button
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                className={`relative px-4 py-2.5 text-[12px] font-semibold tracking-wide uppercase transition-colors ${
                  isActive
                    ? "text-bc-accent"
                    : "text-bc-muted/50 hover:text-bc-muted"
                }`}
                style={{ fontFamily: MONO }}
              >
                {tab.label}
                <span className="ml-1.5 text-[9px] opacity-40">{tab.shortcut}</span>
                {/* Active indicator — bottom glow bar */}
                {isActive && (
                  <span className="absolute bottom-0 left-2 right-2 h-[2px] rounded-full bg-bc-accent shadow-[0_0_8px_rgba(var(--bc-accent-rgb,255,165,0),0.5)]" />
                )}
              </button>
            );
          })}
        </nav>
      </header>

      {/* ═══ TAB CONTENT ═══ */}
      <div className="flex-1 min-h-0 flex flex-col">
        {activeTab === "terminal" && (
          <TerminalTab agent={agent} outputLines={outputLines} outputRef={outputRef} />
        )}
        {activeTab === "activity" && <ActivityTab agent={agent} />}
        {activeTab === "config" && <ConfigTab agent={agent} />}
        {activeTab === "stats" && <StatsTab agent={agent} />}
      </div>

      {/* Ralph Loop modal + auto-reprompt hook */}
      <RalphLoopModal
        open={loopOpen}
        agentName={agent.name}
        onClose={() => setLoopOpen(false)}
      />
      <RalphLoopHook agentName={agent.name} agentState={agent.state} />
    </div>
  );
}

// Wrapper component so the hook runs inside the render tree
function RalphLoopHook({ agentName, agentState }: { agentName: string; agentState: string }) {
  useRalphLoop(agentName, agentState);
  return null;
}
