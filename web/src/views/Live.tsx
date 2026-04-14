import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { Agent } from "../api/client";
import { useWebSocket } from "../hooks/useWebSocket";
import { EmptyState } from "../components/EmptyState";
import type {
  AgentActivity,
  FilterType,
  HookEvent,
  RawEvent,
  TaskItem,
} from "../components/live/liveTypes";
import {
  AUTO_COLLAPSE_MS,
  FLUSH_INTERVAL,
  MAX_NODES,
} from "../components/live/liveTypes";
import {
  findLastIdx,
  nextId,
  nodeMatchesSearch,
  parseTaskCreate,
  parseTaskListResponse,
  parseTaskUpdate,
  summarizeArgs,
  updateSubagentChild,
  updateTopLevelNode,
} from "../components/live/liveHelpers";
import { AgentCard, AgentDrillDown } from "../components/live/LiveRenderers";

/* ── Live (Live Operations Center) ─────────────────────────────────── */

export function Live() {
  const [activities, setActivities] = useState<Map<string, AgentActivity>>(new Map());
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentFilter, setAgentFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState<FilterType>("all");
  const [searchFilter, setSearchFilter] = useState("");
  const [eventCount, setEventCount] = useState(0);
  const [paused, setPaused] = useState(false);
  const pausedRef = useRef(false);
  const pausedBuffer = useRef<HookEvent[]>([]);
  const [pausedCount, setPausedCount] = useState(0);
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);
  const [newEventsSinceScroll, setNewEventsSinceScroll] = useState(0);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [focusedCardIdx, setFocusedCardIdx] = useState(-1);
  const [tasks, setTasks] = useState<Map<string, TaskItem>>(new Map());
  const [drillDownAgent, setDrillDownAgent] = useState<string | null>(null);
  const rawEventsRef = useRef<Map<string, RawEvent[]>>(new Map());
  const [rawEventsVersion, setRawEventsVersion] = useState(0);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const eventBuffer = useRef<HookEvent[]>([]);
  const { connected, reconnecting, subscribe } = useWebSocket();

  // Keep pausedRef in sync so interval/event handlers always see current value
  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  // Seed from agents API + initial logs
  useEffect(() => {
    api.listAgents().then((agentList) => {
      setAgents(agentList);
      setActivities((prev) => {
        const next = new Map(prev);
        for (const a of agentList) {
          if (!next.has(a.name)) {
            const updatedAt = a.updated_at ? new Date(a.updated_at).getTime() : 0;
            const agentCost = a.cost_usd ?? (a as unknown as Record<string, unknown>).total_cost_usd as number ?? 0;
            next.set(a.name, {
              name: a.name,
              state: a.state,
              task: a.task ?? "",
              tool: a.tool,
              role: a.role ?? "",
              tokens: a.total_tokens ?? 0,
              inputTokens: 0,
              outputTokens: 0,
              costUsd: agentCost,
              lastEventTime: updatedAt > 0 && !isNaN(updatedAt) ? updatedAt : 0,
              nodes: [],
              collapsed: a.state === "stopped",
            });
          }
        }
        return next;
      });
    }).catch(() => {});

    api.getLogs(50).then((logs) => {
      setEventCount((c) => c + logs.length);
    }).catch(() => {});
  }, []);

  // Process buffered hook events
  const flushEvents = useCallback(() => {
    const events = eventBuffer.current.splice(0);
    if (events.length === 0) return;

    if (pausedRef.current) {
      pausedBuffer.current.push(...events);
      setPausedCount(pausedBuffer.current.length);
      return;
    }

    setEventCount((c) => c + events.length);

    // Process task-related events
    setTasks((prevTasks) => {
      let nextTasks = prevTasks;
      let changed = false;

      for (const evt of events) {
        const toolName = evt.tool_name ?? "";

        // TaskCreate: on PostToolUse, parse the created task
        if (evt.event === "PostToolUse" && toolName.includes("TaskCreate")) {
          const task = parseTaskCreate(evt.tool_input, evt.tool_response, evt.agent);
          if (task) {
            if (!changed) { nextTasks = new Map(prevTasks); changed = true; }
            nextTasks.set(task.id, task);
          }
        }

        // TaskCreate: also parse ID from tool_response string like "Task #95 created successfully: Subject"
        if (evt.event === "PostToolUse" && toolName.includes("TaskCreate")) {
          const resp = evt.tool_response;
          if (typeof resp === "string") {
            const match = resp.match(/Task\s+#(\d+)/);
            if (match) {
              const numId = match[1]!;
              let replaced = false;
              for (const [key, task] of nextTasks) {
                if (key.startsWith("task-") && task.owner === evt.agent) {
                  if (!changed) { nextTasks = new Map(prevTasks); changed = true; }
                  nextTasks.delete(key);
                  nextTasks.set(numId, { ...task, id: numId });
                  replaced = true;
                  break;
                }
              }
              if (!replaced && !nextTasks.has(numId)) {
                if (!changed) { nextTasks = new Map(prevTasks); changed = true; }
                const subjectMatch = resp.match(/Task\s+#\d+\s+created\s+successfully:\s*(.+)/);
                const subject = subjectMatch ? subjectMatch[1]!.trim() : "Task #" + numId;
                nextTasks.set(numId, { id: numId, subject, status: "pending", owner: evt.agent });
              }
            }
          }
        }

        // TaskUpdate: update status
        if ((evt.event === "PreToolUse" || evt.event === "PostToolUse") && toolName.includes("TaskUpdate")) {
          const update = parseTaskUpdate(evt.tool_input);
          if (update) {
            if (!changed) { nextTasks = new Map(prevTasks); changed = true; }
            const existing = nextTasks.get(update.taskId);
            if (existing) {
              const merged = { ...existing, status: update.status };
              if (update.blockedBy) merged.blockedBy = [...(existing.blockedBy ?? []), ...update.blockedBy];
              nextTasks.set(update.taskId, merged);
            }
          }
        }

        // TaskList: bootstrap/sync task state from full list
        if (evt.event === "PostToolUse" && toolName.includes("TaskList")) {
          const resp = evt.tool_response;
          if (typeof resp === "string" && resp.trim().length > 0) {
            const parsed = parseTaskListResponse(resp);
            if (parsed.length > 0) {
              if (!changed) { nextTasks = new Map(prevTasks); changed = true; }
              nextTasks.clear();
              for (const task of parsed) {
                nextTasks.set(task.id, task);
              }
            }
          }
        }
      }

      return nextTasks;
    });

    setActivities((prev) => {
      const next = new Map(prev);

      for (const evt of events) {
        const agentName = evt.agent;
        if (!agentName) continue;

        let activity = next.get(agentName) ?? {
          name: agentName, state: "working", task: "", tool: "", role: "", tokens: 0, inputTokens: 0, outputTokens: 0, costUsd: 0, lastEventTime: 0, nodes: [], collapsed: false,
        };
        activity = { ...activity, nodes: [...activity.nodes] };
        activity.lastEventTime = Date.now();

        if (evt.task) activity.task = evt.task;
        if (evt.input_tokens) { activity.tokens += evt.input_tokens; activity.inputTokens += evt.input_tokens; }
        if (evt.output_tokens) { activity.tokens += evt.output_tokens; activity.outputTokens += evt.output_tokens; }

        switch (evt.event) {
          case "UserPromptSubmit":
            activity.state = "working";
            activity.nodes.push({
              id: nextId(), toolName: "UserPromptSubmit",
              args: evt.prompt ? (evt.prompt.length > 120 ? evt.prompt.slice(0, 117) + "..." : evt.prompt) : evt.task ?? "",
              fullInput: evt.prompt ?? evt.tool_input, fullOutput: null, status: "completed",
              startTime: Date.now(), endTime: Date.now(), children: [],
            });
            break;

          case "PreToolUse": {
            activity.state = "working";
            const newNode = {
              id: nextId(), toolName: evt.tool_name ?? "unknown", args: summarizeArgs(evt),
              fullInput: evt.tool_input, fullOutput: null, status: "running" as const,
              startTime: Date.now(), children: [],
            };

            // If tool_name is "Agent", this spawns a subagent -- add as top-level
            // and track as active subagent for nesting child events
            if (evt.tool_name === "Agent") {
              activity.nodes.push(newNode);
              activity.activeSubagentIdx = activity.nodes.length - 1;
            } else if (activity.activeSubagentIdx !== undefined && activity.activeSubagentIdx >= 0) {
              // Nest inside the active subagent node
              const parentNode = activity.nodes[activity.activeSubagentIdx];
              if (parentNode && parentNode.status === "running") {
                const updatedParent = { ...parentNode, children: [...parentNode.children, newNode] };
                activity.nodes[activity.activeSubagentIdx] = updatedParent;
              } else {
                activity.nodes.push(newNode);
                activity.activeSubagentIdx = undefined;
              }
            } else {
              activity.nodes.push(newNode);
            }
            break;
          }

          case "PostToolUse": {
            const matchRunning = (n: AgentActivity["nodes"][number]): boolean => n.toolName === evt.tool_name && n.status === "running";
            const markCompleted = (n: AgentActivity["nodes"][number]): AgentActivity["nodes"][number] => ({ ...n, status: "completed" as const, endTime: Date.now(), fullOutput: evt.tool_response ?? evt.tool_input });

            let found = updateSubagentChild(activity, matchRunning, markCompleted);

            if (evt.tool_name === "Agent") {
              const matchAgent = (n: AgentActivity["nodes"][number]): boolean => n.toolName === "Agent" && n.status === "running";
              found = updateTopLevelNode(activity, matchAgent, markCompleted) || found;
              activity.activeSubagentIdx = undefined;
            }

            if (!found) {
              updateTopLevelNode(activity, matchRunning, markCompleted);
            }
            break;
          }

          case "PostToolUseFailure": {
            const matchRunning = (n: AgentActivity["nodes"][number]): boolean => n.toolName === evt.tool_name && n.status === "running";
            const markFailed = (n: AgentActivity["nodes"][number]): AgentActivity["nodes"][number] => ({ ...n, status: "failed" as const, endTime: Date.now(), error: evt.error ?? "Tool execution failed", fullOutput: evt.tool_response ?? evt.tool_input });

            const found = updateSubagentChild(activity, matchRunning, markFailed);
            if (!found) {
              updateTopLevelNode(activity, matchRunning, markFailed);
            }
            break;
          }

          case "SubagentStart": {
            const subNode: AgentActivity["nodes"][number] = {
              id: nextId(), toolName: `Agent: ${evt.subagent_id ?? "sub"}`,
              args: evt.subagent_type ?? "", fullInput: evt.tool_input, fullOutput: null,
              status: "running" as const, startTime: Date.now(), children: [],
            };

            if (activity.activeSubagentIdx !== undefined && activity.activeSubagentIdx >= 0) {
              const parentNode = activity.nodes[activity.activeSubagentIdx];
              if (parentNode && parentNode.status === "running") {
                activity.nodes[activity.activeSubagentIdx] = { ...parentNode, children: [...parentNode.children, subNode] };
                break;
              }
            }

            activity.nodes.push(subNode);
            activity.activeSubagentIdx = activity.nodes.length - 1;
            break;
          }

          case "SubagentStop": {
            const matchRunningAgent = (n: AgentActivity["nodes"][number]): boolean => n.toolName.startsWith("Agent:") && n.status === "running";
            const markDone = (n: AgentActivity["nodes"][number]): AgentActivity["nodes"][number] => ({ ...n, status: "completed" as const, endTime: Date.now() });

            const found = updateSubagentChild(activity, matchRunningAgent, markDone);
            if (!found) {
              const idx = findLastIdx(activity.nodes, matchRunningAgent);
              if (idx >= 0) {
                activity.nodes[idx] = markDone(activity.nodes[idx]!);
                if (activity.activeSubagentIdx === idx) {
                  activity.activeSubagentIdx = undefined;
                }
              }
            }
            break;
          }

          case "PermissionRequest":
          case "Elicitation":
            activity.state = "stuck";
            activity.nodes.push({
              id: nextId(), toolName: evt.event, args: evt.tool_name ?? "",
              fullInput: evt.tool_input, fullOutput: null, status: "running",
              startTime: Date.now(), children: [],
            });
            break;

          case "SessionStart": activity.state = "idle"; break;
          case "SessionEnd": case "Stop": activity.state = "idle"; break;
          case "TaskCompleted": activity.state = "idle"; break;
        }

        if (activity.nodes.length > MAX_NODES) {
          activity.nodes = activity.nodes.slice(-MAX_NODES);
        }

        const now = Date.now();
        activity.nodes = activity.nodes.map((n) =>
          n.status !== "running" && n.endTime && now - n.endTime > AUTO_COLLAPSE_MS
            ? { ...n, fullInput: undefined, fullOutput: undefined }
            : n,
        );

        next.set(agentName, activity);
      }
      return next;
    });
  }, []);

  const handleResume = useCallback(() => {
    setPaused(false);
    if (pausedBuffer.current.length > 0) {
      eventBuffer.current.push(...pausedBuffer.current);
      pausedBuffer.current = [];
      setPausedCount(0);
    }
  }, []);

  useEffect(() => {
    const id = setInterval(flushEvents, FLUSH_INTERVAL);
    return () => clearInterval(id);
  }, [flushEvents]);

  useEffect(() => {
    const unsub = subscribe("agent.hook", (wsEvent) => {
      const d = wsEvent.data as unknown as HookEvent;
      if (d?.agent) {
        eventBuffer.current.push(d);
        // Capture raw event for drill-down raw stream
        const agentRaw = rawEventsRef.current.get(d.agent) ?? [];
        agentRaw.push({ timestamp: Date.now(), eventType: d.event, raw: d });
        if (agentRaw.length > 500) agentRaw.splice(0, agentRaw.length - 500);
        rawEventsRef.current.set(d.agent, agentRaw);
        setRawEventsVersion((v) => v + 1);
      }
    });
    return unsub;
  }, [subscribe]);

  useEffect(() => {
    const unsub = subscribe("agent.state_changed", (wsEvent) => {
      const d = wsEvent.data as Record<string, unknown>;
      const name = (d.name ?? d.agent) as string;
      const state = d.state as string;
      if (!name || !state) return;

      // When paused, buffer state changes as synthetic hook events
      if (pausedRef.current) {
        pausedBuffer.current.push({ agent: name, event: "state_changed", task: d.task as string | undefined });
        setPausedCount(pausedBuffer.current.length);
        return;
      }

      setEventCount((c) => c + 1);
      setActivities((prev) => {
          const next = new Map(prev);
          const existing = next.get(name);
          if (existing) {
            const updates: Partial<AgentActivity> = { state, lastEventTime: Date.now() };
            if (d.task) updates.task = d.task as string;
            if (d.role) updates.role = d.role as string;
            next.set(name, { ...existing, ...updates });
          }
          return next;
        });
    });
    return unsub;
  }, [subscribe]);

  const sorted = useMemo(() => {
    const filtered = Array.from(activities.values()).filter((a) => {
      if (agentFilter && a.name !== agentFilter) return false;
      if (typeFilter === "tools" && a.nodes.length === 0) return false;
      if (searchFilter) {
        const q = searchFilter.toLowerCase();
        const cardHay = `${a.name} ${a.role} ${a.task} ${a.tool}`.toLowerCase();
        if (cardHay.includes(q)) return true;
        const hasMatchingNode = a.nodes.some((n) => nodeMatchesSearch(n, q));
        if (!hasMatchingNode) return false;
      }
      return true;
    });
    return filtered.sort((a, b) => {
      const order: Record<string, number> = { working: 0, stuck: 1, idle: 2, stopped: 3, error: 4 };
      const oa = order[a.state] ?? 5;
      const ob = order[b.state] ?? 5;
      if (oa !== ob) return oa - ob;
      return a.name.localeCompare(b.name);
    });
  }, [activities, agentFilter, typeFilter, searchFilter]);

  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const onScroll = () => {
      const isAtTop = container.scrollTop < 50;
      setShowJumpToLatest(!isAtTop);
      if (isAtTop) setNewEventsSinceScroll(0);
    };
    container.addEventListener("scroll", onScroll, { passive: true });
    return () => container.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    if (showJumpToLatest) {
      setNewEventsSinceScroll((c) => c + 1);
    }
  }, [eventCount]); // eslint-disable-line react-hooks/exhaustive-deps

  const jumpToLatest = useCallback(() => {
    scrollContainerRef.current?.scrollTo({ top: 0, behavior: "smooth" });
    setNewEventsSinceScroll(0);
  }, []);

  const toggleAgent = useCallback((name: string) => {
    setActivities((prev) => {
      const next = new Map(prev);
      const existing = next.get(name);
      if (existing) next.set(name, { ...existing, collapsed: !existing.collapsed });
      return next;
    });
  }, []);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      const isInput = target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;

      if (e.key === "Escape") {
        setSearchFilter("");
        setShowShortcuts(false);
        (document.activeElement as HTMLElement)?.blur();
        return;
      }

      if (e.key === "/" && !isInput) {
        e.preventDefault();
        searchInputRef.current?.focus();
        return;
      }

      if (isInput) return;

      if (e.key === "?") {
        e.preventDefault();
        setShowShortcuts((prev) => !prev);
        return;
      }

      if (e.key === "j") {
        e.preventDefault();
        setFocusedCardIdx((prev) => Math.min(prev + 1, sorted.length - 1));
        return;
      }

      if (e.key === "k") {
        e.preventDefault();
        setFocusedCardIdx((prev) => Math.max(prev - 1, 0));
        return;
      }

      if (e.key === "Enter" && focusedCardIdx >= 0 && focusedCardIdx < sorted.length) {
        e.preventDefault();
        const card = sorted[focusedCardIdx];
        if (card) toggleAgent(card.name);
        return;
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [sorted, focusedCardIdx, toggleAgent]); // eslint-disable-line react-hooks/exhaustive-deps

  const hasFilters = agentFilter || typeFilter !== "all" || searchFilter;

  const sseDotColor = connected ? "bg-emerald-500" : reconnecting ? "bg-yellow-500" : "bg-red-500";
  const sseTooltip = connected ? "SSE connected" : reconnecting ? "Reconnecting..." : "Disconnected";

  // Drill-down view
  const drillDownActivity = drillDownAgent ? activities.get(drillDownAgent) : null;
  const drillDownRawEvents = drillDownAgent ? (rawEventsRef.current.get(drillDownAgent) ?? []) : [];
  // Reference rawEventsVersion to trigger re-render when raw events change
  void rawEventsVersion;

  if (drillDownAgent && drillDownActivity) {
    return (
      <div className="p-6 flex flex-col h-full relative">
        <AgentDrillDown
          activity={drillDownActivity}
          rawEvents={drillDownRawEvents}
          tasks={tasks}
          onBack={() => setDrillDownAgent(null)}
        />
      </div>
    );
  }

  return (
    <div className="p-6 flex flex-col h-full relative">
      {/* Header */}
      <div className="flex items-center gap-3 mb-4">
        <h1 className="text-xl font-bold text-bc-text flex items-center gap-2 shrink-0 pl-10 sm:pl-0">
          Live
          <span className="relative flex h-2.5 w-2.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-bc-error opacity-75" />
            <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-bc-error" />
          </span>
        </h1>
        <span className="text-sm text-bc-muted hidden sm:inline">Real-time agent activity</span>
        <span className="ml-auto flex items-center gap-2">
          {/* SSE connection indicator */}
          <span className={`inline-flex items-center gap-1.5 px-2 py-1 rounded-md text-[11px] font-mono ${connected ? "bg-bc-success/10 text-bc-success" : reconnecting ? "bg-bc-warning/10 text-bc-warning" : "bg-bc-error/10 text-bc-error"}`} title={sseTooltip}>
            <span className={`inline-flex h-1.5 w-1.5 rounded-full ${sseDotColor}${reconnecting ? " animate-pulse" : ""}`} />
            <span className="hidden sm:inline">{connected ? "Connected" : reconnecting ? "Reconnecting" : "Disconnected"}</span>
          </span>
          {/* Event count pill */}
          <span className="text-[11px] text-bc-muted font-mono tabular-nums px-2 py-1 rounded-md bg-bc-surface border border-bc-border">{eventCount.toLocaleString()} events</span>
          {/* Pause/Resume button */}
          <button
            type="button"
            onClick={() => paused ? handleResume() : setPaused(true)}
            className={`relative inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-md border transition-colors ${paused ? "border-bc-warning bg-bc-warning/15 text-bc-warning hover:bg-bc-warning/25" : "border-bc-border hover:border-bc-accent bg-bc-surface text-bc-text"}`}
            title={paused ? `Resume (${pausedCount} buffered)` : "Pause stream"}
          >
            {paused ? (
              <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor"><polygon points="1,0 10,5 1,10" /></svg>
            ) : (
              <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor"><rect x="1" y="0" width="3" height="10" /><rect x="6" y="0" width="3" height="10" /></svg>
            )}
            <span className="hidden sm:inline">{paused ? "Resume" : "Pause"}</span>
            {paused && pausedCount > 0 && (
              <span className="text-[10px] font-bold text-bc-warning ml-0.5">({pausedCount})</span>
            )}
          </button>
          {/* Export button with download icon */}
          <button
            type="button"
            onClick={() => {
              const exportData = {
                exportedAt: new Date().toISOString(),
                eventCount,
                activities: Object.fromEntries(
                  Array.from(activities.entries()).map(([name, a]) => [name, {
                    name: a.name, state: a.state, role: a.role, task: a.task,
                    tokens: a.tokens, inputTokens: a.inputTokens, outputTokens: a.outputTokens,
                    costUsd: a.costUsd, lastEventTime: a.lastEventTime,
                    nodes: a.nodes.map((n) => ({
                      id: n.id, toolName: n.toolName, args: n.args,
                      status: n.status, startTime: n.startTime, endTime: n.endTime,
                      error: n.error,
                    })),
                  }]),
                ),
                tasks: Object.fromEntries(Array.from(tasks.entries())),
              };
              const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: "application/json" });
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a");
              a.href = url;
              a.download = `bc-events-${Date.now()}.json`;
              a.click();
              URL.revokeObjectURL(url);
            }}
            className="inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-md border border-bc-border hover:border-bc-accent bg-bc-surface text-bc-muted hover:text-bc-text transition-colors"
            title="Export event feed as JSON"
          >
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5">
              <path d="M6 1v7M3 5l3 3 3-3M2 10h8" />
            </svg>
            <span className="hidden sm:inline">Export</span>
          </button>
          {/* Help button */}
          <button
            type="button"
            onClick={() => setShowShortcuts((prev) => !prev)}
            className="inline-flex items-center justify-center h-7 w-7 rounded-md border border-bc-border hover:border-bc-accent bg-bc-surface text-bc-muted hover:text-bc-text text-xs transition-colors"
            title="Keyboard shortcuts (?)"
          >
            ?
          </button>
        </span>
      </div>

      {/* Keyboard Shortcuts Overlay */}
      {showShortcuts && (
        <div className="absolute top-16 right-6 z-50 bg-bc-surface border border-bc-border rounded-lg shadow-lg p-4 w-64">
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm font-semibold text-bc-text">Keyboard Shortcuts</span>
            <button
              type="button"
              onClick={() => setShowShortcuts(false)}
              className="text-bc-muted hover:text-bc-text text-sm"
            >
              &times;
            </button>
          </div>
          <div className="space-y-1.5 text-xs">
            {[
              ["/", "Focus search"],
              ["Esc", "Clear search / close"],
              ["j", "Next agent card"],
              ["k", "Previous agent card"],
              ["Enter", "Expand/collapse focused card"],
              ["?", "Toggle this help"],
            ].map(([key, desc]) => (
              <div key={key} className="flex items-center gap-2">
                <kbd className="inline-flex items-center justify-center min-w-[24px] h-5 px-1.5 rounded bg-bc-bg border border-bc-border text-bc-text font-mono text-[11px]">
                  {key}
                </kbd>
                <span className="text-bc-muted">{desc}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-2 mb-4 sticky top-0 z-10 bg-bc-bg py-2">
        <select
          value={agentFilter}
          onChange={(e) => setAgentFilter(e.target.value)}
          className="text-sm rounded-md border border-bc-border bg-bc-surface px-2.5 py-1.5 text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent appearance-none pr-7"
          style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg width='10' height='6' viewBox='0 0 10 6' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%238c7e72' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`, backgroundRepeat: "no-repeat", backgroundPosition: "right 8px center" }}
        >
          <option value="">All agents</option>
          {agents.map((a) => (
            <option key={a.name} value={a.name}>{a.name}</option>
          ))}
        </select>
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value as FilterType)}
          className="text-sm rounded-md border border-bc-border bg-bc-surface px-2.5 py-1.5 text-bc-text focus:outline-none focus:ring-1 focus:ring-bc-accent appearance-none pr-7"
          style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg width='10' height='6' viewBox='0 0 10 6' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%238c7e72' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`, backgroundRepeat: "no-repeat", backgroundPosition: "right 8px center" }}
        >
          <option value="all">All</option>
          <option value="tools">Tool Calls</option>
          <option value="state">State Changes</option>
        </select>
        {/* Search with magnifying glass icon */}
        <div className="relative">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-bc-muted pointer-events-none">
            <circle cx="6" cy="6" r="4.5" />
            <path d="M9.5 9.5L13 13" />
          </svg>
          <input
            ref={searchInputRef}
            type="text"
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            placeholder="Search events... (/ to focus)"
            className="text-sm rounded-md border border-bc-border bg-bc-surface pl-8 pr-2.5 py-1.5 text-bc-text placeholder:text-bc-muted focus:outline-none focus:ring-1 focus:ring-bc-accent w-56"
          />
        </div>
        {/* Active filter pills */}
        {hasFilters && (
          <div className="flex items-center gap-1.5">
            {agentFilter && (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-1 rounded-full bg-bc-accent/10 text-bc-accent border border-bc-accent/30">
                {agentFilter}
                <button type="button" onClick={() => setAgentFilter("")} className="hover:text-bc-text ml-0.5" aria-label="Remove agent filter">&times;</button>
              </span>
            )}
            {typeFilter !== "all" && (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-1 rounded-full bg-bc-accent/10 text-bc-accent border border-bc-accent/30">
                {typeFilter === "tools" ? "Tool Calls" : "State Changes"}
                <button type="button" onClick={() => setTypeFilter("all")} className="hover:text-bc-text ml-0.5" aria-label="Remove type filter">&times;</button>
              </span>
            )}
            {searchFilter && (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-1 rounded-full bg-bc-accent/10 text-bc-accent border border-bc-accent/30">
                &ldquo;{searchFilter.length > 16 ? searchFilter.slice(0, 14) + "..." : searchFilter}&rdquo;
                <button type="button" onClick={() => setSearchFilter("")} className="hover:text-bc-text ml-0.5" aria-label="Remove search filter">&times;</button>
              </span>
            )}
            <button
              type="button"
              onClick={() => { setAgentFilter(""); setTypeFilter("all"); setSearchFilter(""); }}
              className="text-[11px] text-bc-muted hover:text-bc-text px-2 py-1 rounded-md border border-bc-border hover:border-bc-accent transition-colors"
            >
              Clear all
            </button>
          </div>
        )}
      </div>

      {/* Agent Activity Cards */}
      <div ref={scrollContainerRef} className="flex-1 overflow-y-auto min-h-0 space-y-3 relative">
        {sorted.length === 0 ? (
          <EmptyState
            icon=">"
            title="No activity yet"
            description="Events will stream here in real-time as agents work."
          />
        ) : (
          sorted.map((activity, idx) => (
            <div
              key={activity.name}
              className={focusedCardIdx === idx ? "ring-2 ring-bc-accent rounded-lg" : ""}
            >
              <AgentCard
                activity={activity}
                onToggle={() => toggleAgent(activity.name)}
                onDrillDown={() => setDrillDownAgent(activity.name)}
                isFilterActive={agentFilter === activity.name}
                searchTerm={searchFilter}
                typeFilter={typeFilter}
                isPaused={paused}
              />
            </div>
          ))
        )}
      </div>

      {/* Jump to Latest Button */}
      {showJumpToLatest && (
        <button
          type="button"
          onClick={jumpToLatest}
          className="absolute bottom-8 right-8 z-20 inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-bc-border bg-bc-surface text-bc-text text-sm shadow-lg hover:border-bc-accent hover:bg-bc-surface-hover transition-colors"
        >
          <span>&darr;</span>
          Jump to latest
          {newEventsSinceScroll > 0 && (
            <span className="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-bold text-white bg-bc-accent rounded-full leading-none">
              {newEventsSinceScroll}
            </span>
          )}
        </button>
      )}
    </div>
  );
}
