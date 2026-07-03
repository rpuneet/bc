import { useEffect, useState } from "react";
import { api } from "../../api/client";
import type { AgentActivityItem } from "../../api/client";
import { formatAbsolute, formatRelative } from "../../utils/time";
import { MONO } from "../../utils/typography";

const formatTime = (t?: string): string => formatAbsolute(t);

/* ═══════════════════════════════════════════════════════════════════
   Types & helpers
   ═══════════════════════════════════════════════════════════════════ */

interface TimelineEvent {
  key: string;
  label: string;
  timestamp?: string;
  detail?: string;
  active: boolean;
}

interface AgentActivityStreamProps {
  agentName: string;
  agentState: string;
  agentTask?: string;
  stoppedAt?: string;
  updatedAt?: string;
  startedAt?: string;
  createdAt?: string;
}

function buildTimeline(
  agentState: string,
  createdAt?: string,
  startedAt?: string,
  updatedAt?: string,
  stoppedAt?: string,
  agentTask?: string,
): TimelineEvent[] {
  const events: TimelineEvent[] = [];
  const isRunning = agentState !== "stopped" && agentState !== "error";

  if (createdAt) {
    events.push({
      key: "created",
      label: "Created",
      timestamp: createdAt,
      active: false,
    });
  }
  if (startedAt) {
    events.push({
      key: "started",
      label: "Started",
      timestamp: startedAt,
      active: false,
    });
  }
  if (isRunning) {
    events.push({
      key: "current",
      label:
        agentState === "working"
          ? "Working"
          : agentState === "starting"
            ? "Starting"
            : agentState === "idle"
              ? "Idle"
              : "Active",
      timestamp: updatedAt,
      detail: agentTask,
      active: true,
    });
  } else if (stoppedAt) {
    events.push({
      key: "stopped",
      label: agentState === "error" ? "Errored" : "Stopped",
      timestamp: stoppedAt,
      detail: agentTask,
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

/* ═══════════════════════════════════════════════════════════════════
   Component
   ═══════════════════════════════════════════════════════════════════ */

export function AgentActivityStream({
  agentName,
  agentState,
  agentTask,
  stoppedAt,
  updatedAt,
  startedAt,
  createdAt,
}: AgentActivityStreamProps) {
  const [activity, setActivity] = useState<AgentActivityItem[]>([]);
  const [activeFilter, setActiveFilter] = useState<EventFilter>("all");

  useEffect(() => {
    let cancelled = false;
    api
      .getAgentActivity(agentName)
      .then((items) => {
        if (!cancelled) setActivity(items);
      })
      .catch(() => {
        /* best-effort */
      });
    return () => {
      cancelled = true;
    };
  }, [agentName]);

  // SSE live events
  useEffect(() => {
    if (agentState === "stopped" || agentState === "error") return;

    const es = new EventSource(`/api/agents/${encodeURIComponent(agentName)}/events`);

    es.addEventListener("hook", (e: MessageEvent) => {
      try {
        const data = JSON.parse(String(e.data)) as {
          event?: string;
          timestamp?: string;
          tool_name?: string;
          tool_input?: { command?: string };
          message?: string;
        };
        const newItem: AgentActivityItem = {
          event: data.event ?? "unknown",
          timestamp: data.timestamp ?? new Date().toISOString(),
          message: data.tool_name
            ? `${data.tool_name}${data.tool_input?.command ? ": " + data.tool_input.command : ""}`
            : (data.message ?? ""),
        };
        setActivity((prev) => {
          // Deduplicate: skip if an event with the same timestamp+event already exists
          const isDup = prev.some(
            (it) => it.timestamp === newItem.timestamp && it.event === newItem.event,
          );
          if (isDup) return prev;
          return [newItem, ...prev].slice(0, 50);
        });
      } catch {
        /* ignore malformed events */
      }
    });

    es.onerror = () => {
      /* auto-reconnects */
    };

    return () => es.close();
  }, [agentName, agentState]);

  const isStopped = agentState === "stopped" || agentState === "error";
  const isRunning = !isStopped;
  const derivedTimeline = buildTimeline(
    agentState,
    createdAt,
    startedAt,
    updatedAt,
    stoppedAt,
    agentTask,
  );
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

  const lastActivity = stoppedAt ?? updatedAt ?? startedAt ?? createdAt;

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="max-w-3xl mx-auto space-y-8">
        {/* ── CURRENT TASK BANNER ── */}
        {(agentTask || isStopped) && (
          <div
            className={`rounded-md border px-4 py-3 transition-colors ${
              isStopped
                ? "border-mycel-border/40 bg-mycel-surface/30"
                : "border-mycel-accent/20 bg-mycel-accent/[0.04]"
            }`}
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex-1 min-w-0">
                <div
                  className="text-[9px] font-bold uppercase tracking-[0.2em] text-mycel-muted/60 mb-1"
                  style={{ fontFamily: MONO }}
                >
                  {isStopped ? "last task" : "current task"}
                </div>
                <p
                  className="text-sm text-mycel-text/90 break-words leading-relaxed"
                  style={{ fontFamily: MONO }}
                >
                  {agentTask ?? (
                    <span className="text-mycel-muted italic">none</span>
                  )}
                </p>
              </div>
              {lastActivity && (
                <span
                  className="text-[11px] text-mycel-muted tabular-nums shrink-0 pt-0.5"
                  title={formatTime(lastActivity)}
                  style={{ fontFamily: MONO }}
                >
                  {formatRelative(lastActivity, { emptyLabel: "" })}
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
              className="text-[10px] font-bold uppercase tracking-[0.2em] text-mycel-muted/70"
              style={{ fontFamily: MONO }}
            >
              Event Stream
            </span>
            <span className="flex-1 h-px bg-gradient-to-r from-mycel-border/50 to-transparent" />
            {/* Live indicator */}
            <span
              className="flex items-center gap-1 text-[10px] tabular-nums"
              style={{ fontFamily: MONO }}
            >
              <span
                className={`w-1.5 h-1.5 rounded-full ${isRunning ? "bg-green-500" : "bg-mycel-muted/40"}`}
              />
              <span className={isRunning ? "text-green-400" : "text-mycel-muted"}>
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
                    ? "border-mycel-accent/30 bg-mycel-accent/15 text-mycel-accent"
                    : "border-mycel-border/40 text-mycel-muted/60 hover:text-mycel-muted hover:border-mycel-border/50"
                }`}
                style={{ fontFamily: MONO }}
              >
                {f.label}
              </button>
            ))}
          </div>

          {timeline.length === 0 ? (
            <p className="text-xs text-mycel-muted italic pl-1">
              {allTimeline.length === 0
                ? "No activity recorded yet."
                : "No events match this filter."}
            </p>
          ) : (
            <ol className="relative ml-1.5">
              {/* Vertical rail */}
              <span
                aria-hidden
                className="absolute left-[3.5px] top-2.5 bottom-2.5 w-px bg-mycel-border/40"
              />
              {timeline.map((evt) => (
                <li key={evt.key} className="relative pl-7 pb-5 last:pb-0">
                  {/* Dot */}
                  <span
                    aria-hidden
                    className={`absolute left-0 top-[7px] w-2 h-2 rounded-full border-[1.5px] transition-colors ${
                      evt.active
                        ? "bg-mycel-accent border-mycel-accent shadow-[0_0_6px_rgba(var(--mycel-accent-rgb,255,165,0),0.4)]"
                        : "bg-mycel-bg border-mycel-muted/50"
                    }`}
                  />
                  <div className="flex items-baseline justify-between gap-4">
                    <span
                      className={`text-[13px] font-semibold ${
                        evt.active ? "text-mycel-accent" : "text-mycel-text/80"
                      }`}
                      style={{ fontFamily: MONO }}
                    >
                      <span className="mr-1.5 opacity-70">{eventIcon(evt.label)}</span>
                      {evt.label}
                    </span>
                    {evt.timestamp && (
                      <span
                        className="text-[10px] text-mycel-muted tabular-nums shrink-0"
                        title={formatTime(evt.timestamp)}
                        style={{ fontFamily: MONO }}
                      >
                        {formatRelative(evt.timestamp, { emptyLabel: "" })}
                      </span>
                    )}
                  </div>
                  {evt.detail && (
                    <p className="mt-1 text-xs text-mycel-muted/70 break-words leading-relaxed">
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
            className="text-[10px] text-mycel-muted italic pl-1"
            style={{ fontFamily: MONO }}
          >
            Agent is not running. Showing last known activity.
          </p>
        )}
      </div>
    </div>
  );
}
