import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import { useWebSocket } from "./useWebSocket";
import type {
  AgentActivity,
  HookEvent,
  RawEvent,
  TaskItem,
  ToolNode,
} from "../components/live/liveTypes";
import {
  AUTO_COLLAPSE_MS,
  FLUSH_INTERVAL,
  MAX_NODES,
} from "../components/live/liveTypes";
// Close out any still-"running" nodes. Called when an agent's turn or
// session ends (or its state flips to idle/stopped): without a matching
// PostToolUse the node would stay "running" forever, leaving a stale
// frozen snapshot with a ticking elapsed timer (#3267).
function finalizeRunningNodes(nodes: ToolNode[], endTime: number): ToolNode[] {
  let changed = false;
  const result = nodes.map((n) => {
    const children = n.children.length > 0 ? finalizeRunningNodes(n.children, endTime) : n.children;
    if (n.status === "running") {
      changed = true;
      return { ...n, children, status: "completed" as const, endTime };
    }
    if (children !== n.children) {
      changed = true;
      return { ...n, children };
    }
    return n;
  });
  return changed ? result : nodes;
}

const TERMINAL_STATES = new Set(["idle", "stopped", "done", "error"]);

import {
  activityItemsToNodes,
  findLastIdx,
  nextId,
  parseTaskCreate,
  parseTaskListResponse,
  parseTaskUpdate,
  providerFailureNode,
  summarizeArgs,
  summarizeInternalEvent,
  updateSubagentChild,
  updateTopLevelNode,
} from "../components/live/liveHelpers";

/* ── useAgentActivity ───────────────────────────────────────────────
   Subscribes to the live WebSocket event stream and maintains a map
   of per-agent activity state. When `agentName` is provided, only
   events for that agent are processed.
─────────────────────────────────────────────────────────────────── */

/** A buffered state_changed event, replayed in order on resume. */
interface PendingStateChange {
  name: string;
  state: string;
  task?: string;
  role?: string;
}

export interface UseAgentActivityOptions {
  /** When true, incoming events are buffered instead of applied — the
   *  stream keeps counting (via `pausedCount`) but the UI stays frozen
   *  until `paused` flips back to false, at which point everything
   *  buffered is flushed through in order. */
  paused?: boolean;
}

export function useAgentActivity(agentName?: string, options?: UseAgentActivityOptions): {
  activities: Map<string, AgentActivity>;
  tasks: Map<string, TaskItem>;
  rawEventsRef: React.MutableRefObject<Map<string, RawEvent[]>>;
  connected: boolean;
  reconnecting: boolean;
  eventCount: number;
  pausedCount: number;
} {
  const paused = options?.paused ?? false;
  const [activities, setActivities] = useState<Map<string, AgentActivity>>(new Map());
  const [tasks, setTasks] = useState<Map<string, TaskItem>>(new Map());
  const [eventCount, setEventCount] = useState(0);
  const [pausedCount, setPausedCount] = useState(0);
  const rawEventsRef = useRef<Map<string, RawEvent[]>>(new Map());
  const eventBuffer = useRef<HookEvent[]>([]);
  const pausedRef = useRef(false);
  const pausedBuffer = useRef<HookEvent[]>([]);
  const pausedStateBuffer = useRef<PendingStateChange[]>([]);
  const { connected, reconnecting, subscribe } = useWebSocket();

  // Seed from agents API + initial logs
  useEffect(() => {
    api.listAgents().then((agentList) => {
      setActivities((prev) => {
        const next = new Map(prev);
        for (const a of agentList) {
          // If filtering by agent, only seed that agent
          if (agentName && a.name !== agentName) continue;
          if (!next.has(a.name)) {
            const updatedAt = a.updated_at ? new Date(a.updated_at).getTime() : 0;
            const agentCost = a.total_cost_usd ?? 0;
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
              missingSecrets: a.missing_secrets ?? [],
            });
          }
        }
        return next;
      });

      // When filtering by single agent, fetch historical activity to pre-populate
      if (agentName) {
        api.getAgentActivity(agentName, 500).then((items) => {
          setActivities((prev) => {
            const next = new Map(prev);
            const existing = next.get(agentName);
            if (existing && existing.nodes.length === 0 && items.length > 0) {
              // Newest first here, as the detail feed reads; paired first so a
              // tool call is one row with a duration rather than two without.
              const nodes: ToolNode[] = activityItemsToNodes(items).reverse();
              next.set(agentName, { ...existing, nodes });
            }
            return next;
          });
        }).catch(() => { /* best effort */ });
      } else {
        // Multi-agent view (Live page): hydrate each agent from its own
        // activity feed. A single shared cross-agent fetch (the old
        // getActivity(400)) collapses to minutes of history when one busy
        // agent floods the event stream, leaving quieter agents' cards
        // empty after a reload (#3279).
        const active = agentList.filter((a) => a.state !== "stopped");
        for (const a of active) {
          api.getAgentActivity(a.name, 100).then((items) => {
            if (items.length === 0) return;
            setActivities((prev) => {
              const next = new Map(prev);
              const existing = next.get(a.name);
              if (!existing || existing.nodes.length > 0) return prev;
              // Oldest-first inside each card; the REST feed is newest-first.
              const nodes: ToolNode[] = activityItemsToNodes(items);
              next.set(a.name, { ...existing, nodes });
              return next;
            });
          }).catch(() => { /* best effort */ });
        }
      }
    }).catch(() => {});

    api.getLogs(50).then((logs) => {
      setEventCount((c) => c + logs.length);
    }).catch(() => {});
  }, [agentName]);

  // Process buffered hook events
  const flushEvents = useCallback(() => {
    const events = eventBuffer.current.splice(0);
    if (events.length === 0) return;

    if (pausedRef.current) {
      pausedBuffer.current.push(...events);
      setPausedCount((c) => c + events.length);
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
        const evtAgent = evt.agent;
        if (!evtAgent) continue;

        let activity = next.get(evtAgent) ?? {
          name: evtAgent, state: "working", task: "", tool: "", role: "", tokens: 0, inputTokens: 0, outputTokens: 0, costUsd: 0, lastEventTime: 0, nodes: [], collapsed: false,
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

          // The daemon raises this when an agent's provider CLI is running but
          // cannot serve a turn — no credential, a spent quota, a model the
          // account can't use. Such an agent reports nothing else, so this row
          // is the entire explanation for a feed that would otherwise be empty
          // and a state that would otherwise claim the agent is fine (#3512).
          case "ProviderFailure":
            activity.state = "error";
            activity.nodes.push(
              providerFailureNode(evt.error ?? evt.message ?? "", Date.now()),
            );
            break;

          case "PreToolUse": {
            activity.state = "working";
            const newNode = {
              id: nextId(), toolName: evt.tool_name ?? "unknown", args: summarizeArgs(evt),
              fullInput: evt.tool_input, fullOutput: null, status: "running" as const,
              startTime: Date.now(), children: [],
            };

            if (evt.tool_name === "Agent") {
              activity.nodes.push(newNode);
              activity.activeSubagentIdx = activity.nodes.length - 1;
            } else if (activity.activeSubagentIdx !== undefined && activity.activeSubagentIdx >= 0) {
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

          // The server resolves a pending PermissionRequest/Elicitation with
          // ElicitationResult (pkg/agent/hooks.go) — without handling it here
          // the "waiting for permission" row never clears and stays pinned
          // as "running" until the turn ends (#2674).
          case "ElicitationResult": {
            const matchWaiting = (n: AgentActivity["nodes"][number]): boolean =>
              (n.toolName === "PermissionRequest" || n.toolName === "Elicitation") && n.status === "running";
            const markResolved = (n: AgentActivity["nodes"][number]): AgentActivity["nodes"][number] => ({
              ...n, status: "completed" as const, endTime: Date.now(),
              fullOutput: evt.message ?? evt.tool_response ?? null,
            });

            const found = updateSubagentChild(activity, matchWaiting, markResolved);
            if (!found) {
              updateTopLevelNode(activity, matchWaiting, markResolved);
            }
            break;
          }

          case "SessionStart": case "SessionEnd": case "Stop": case "TaskCompleted":
            // Turn/session boundary: whatever was still "running" is over.
            activity.state = "idle";
            activity.nodes = finalizeRunningNodes(activity.nodes, Date.now());
            activity.activeSubagentIdx = undefined;
            break;

          // mycel-internal / informational events that previously fell
          // through the switch unrendered — only visible in the Raw tab
          // (#2674). Surface each as a compact completed row so the
          // timeline shows every known server event.
          case "ChannelMessage":
          case "ChannelSent":
          case "AgentMessage":
          case "CostUpdate":
          case "Notification":
          case "ConfigChange":
          case "WorktreeCreate":
          case "WorktreeRemove":
          case "PreCompact":
          case "PostCompact":
            activity.nodes.push({
              id: nextId(), toolName: evt.event, args: summarizeInternalEvent(evt),
              fullInput: evt.tool_input ?? null, fullOutput: null, status: "completed",
              startTime: Date.now(), endTime: Date.now(), children: [],
            });
            break;

          // Any other known-but-unhandled event (TeammateIdle,
          // InstructionsLoaded, Pre/PostInvocation, …): render a generic
          // compact row rather than dropping it silently.
          default:
            if (evt.event) {
              activity.nodes.push({
                id: nextId(), toolName: evt.event, args: summarizeInternalEvent(evt),
                fullInput: evt.tool_input ?? null, fullOutput: null, status: "completed",
                startTime: Date.now(), endTime: Date.now(), children: [],
              });
            }
            break;
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

        next.set(evtAgent, activity);
      }
      return next;
    });
  }, []);

  // Flush interval
  useEffect(() => {
    const id = setInterval(flushEvents, FLUSH_INTERVAL);
    return () => clearInterval(id);
  }, [flushEvents]);

  // Subscribe to agent.hook events
  useEffect(() => {
    const unsub = subscribe("agent.hook", (wsEvent) => {
      const d = wsEvent.data as unknown as HookEvent;
      if (d?.agent) {
        // Filter by agentName if provided
        if (agentName && d.agent !== agentName) return;
        eventBuffer.current.push(d);
        // Capture raw event for drill-down raw stream
        const agentRaw = rawEventsRef.current.get(d.agent) ?? [];
        agentRaw.push({ timestamp: Date.now(), eventType: d.event, raw: d });
        if (agentRaw.length > 500) agentRaw.splice(0, agentRaw.length - 500);
        rawEventsRef.current.set(d.agent, agentRaw);
      }
    });
    return unsub;
  }, [subscribe, agentName]);

  // Applies one state_changed event to `activities`. Shared by the live
  // subscription below and by the resume-flush replay so buffered state
  // changes land exactly as they would have live.
  const applyStateChange = useCallback((change: PendingStateChange) => {
    setEventCount((c) => c + 1);
    setActivities((prev) => {
      const next = new Map(prev);
      const existing = next.get(change.name);
      if (existing) {
        const updates: Partial<AgentActivity> = { state: change.state, lastEventTime: Date.now() };
        if (change.task) updates.task = change.task;
        if (change.role) updates.role = change.role;
        // Agent went idle/stopped: age out stale "running" rows so the
        // stream shows the real last events, not a frozen snapshot.
        if (TERMINAL_STATES.has(change.state)) {
          updates.nodes = finalizeRunningNodes(existing.nodes, Date.now());
          updates.activeSubagentIdx = undefined;
        }
        next.set(change.name, { ...existing, ...updates });
      }
      return next;
    });
  }, []);

  // Subscribe to agent.state_changed events
  useEffect(() => {
    const unsub = subscribe("agent.state_changed", (wsEvent) => {
      const d = wsEvent.data as Record<string, unknown>;
      const name = (d.name ?? d.agent) as string;
      const state = d.state as string;
      if (!name || !state) return;

      // Filter by agentName if provided
      if (agentName && name !== agentName) return;

      const change: PendingStateChange = {
        name, state,
        task: d.task as string | undefined,
        role: d.role as string | undefined,
      };

      if (pausedRef.current) {
        pausedStateBuffer.current.push(change);
        setPausedCount((c) => c + 1);
        return;
      }

      applyStateChange(change);
    });
    return unsub;
  }, [subscribe, agentName, applyStateChange]);

  // Resume: flush everything buffered while paused, in order, then reset
  // the paused counter. Hook events are handed back to the normal buffer
  // so the next flush tick processes them through the same switch as a
  // live event would; state changes are replayed directly.
  const prevPausedRef = useRef(paused);
  useEffect(() => {
    pausedRef.current = paused;
    if (prevPausedRef.current && !paused) {
      const bufferedHooks = pausedBuffer.current.splice(0);
      if (bufferedHooks.length > 0) {
        eventBuffer.current.push(...bufferedHooks);
      }
      const bufferedStates = pausedStateBuffer.current.splice(0);
      for (const change of bufferedStates) {
        applyStateChange(change);
      }
      setPausedCount(0);
    }
    prevPausedRef.current = paused;
  }, [paused, applyStateChange]);

  return {
    activities,
    tasks,
    rawEventsRef,
    connected,
    reconnecting,
    eventCount,
    pausedCount,
  };
}
