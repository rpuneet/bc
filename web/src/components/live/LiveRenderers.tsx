import { useCallback, useEffect, useMemo, useState, memo, type ReactNode } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Link } from "react-router-dom";
import type {
  AgentActivity,
  DrillDownTab,
  FilterType,
  RawEvent,
  TaskItem,
  ToolNode,
} from "./liveTypes";
import {
  estimateCost,
  flattenNodes,
  idleDuration,
  nodeMatchesSearch,
  partitionRunning,
  redactValue,
  stateBadgeClass,
} from "./liveHelpers";
import { CopyButton, EventRow } from "./EventRow";
import { LiveAgentCharacter } from "../agent-ui";

// Shared row primitives live in EventRow.tsx; re-export for compatibility.
export { CopyButton, ElapsedTimer, EventRow, RelativeTimestamp, SearchHighlight } from "./EventRow";

/* ── State Dots ────────────────────────────────────────────────────── */

export function StateDot({ state }: { state: string }) {
  if (state === "working")
    return (
      <span className="relative flex h-2.5 w-2.5">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-mycel-success opacity-75" />
        <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-mycel-success" />
      </span>
    );
  if (state === "stuck")
    return (
      <span className="relative flex h-2.5 w-2.5">
        <span className="absolute inline-flex h-full w-full animate-pulse rounded-full bg-mycel-warning opacity-50" />
        <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-mycel-warning" />
      </span>
    );
  if (state === "error" || state === "stopped")
    return <span className="inline-flex h-2.5 w-2.5 rounded-full bg-mycel-error" />;
  return <span className="inline-flex h-2.5 w-2.5 rounded-full bg-mycel-muted" />;
}

/* ── Pinned "Running" section ──────────────────────────────────────── */

/**
 * A pinned band of currently-running rows kept above the chronological
 * stream, so in-flight tool calls and still-executing subagents stay
 * visible instead of being pushed down as new events arrive. When a row
 * completes it drops out of `nodes` (status flips off "running") and
 * takes its normal chronological place below — rows are keyed by node id,
 * so React just moves the existing DOM node, no reorder animation.
 *
 * `sticky` keeps the band anchored to the top of a scrollable feed (the
 * agent-detail Live tab). In the compact Live card there is no scroll
 * container, so it renders inline at the top instead.
 */
export function RunningSection({
  nodes,
  searchQuery = "",
  sticky = false,
}: {
  nodes: ToolNode[];
  searchQuery?: string;
  sticky?: boolean;
}) {
  if (nodes.length === 0) return null;
  return (
    <div className={sticky ? "sticky top-0 z-10" : ""}>
      <div className="flex items-center gap-2 px-3 py-1 bg-mycel-surface/95 backdrop-blur-sm border-b border-mycel-border">
        <span className="relative flex h-2 w-2 shrink-0" aria-hidden>
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-mycel-accent opacity-60 motion-reduce:hidden" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-mycel-accent" />
        </span>
        <span className="text-[10px] uppercase tracking-wide font-semibold text-mycel-accent">
          Running
        </span>
        <span className="text-[10px] text-mycel-muted font-mono tabular-nums">
          {nodes.length}
        </span>
      </div>
      <div className="bg-mycel-accent-subtle/30 border-b-2 border-mycel-accent-subtle">
        {nodes.map((node) => (
          <EventRow key={node.id} node={node} searchQuery={searchQuery} />
        ))}
      </div>
    </div>
  );
}

/* ── Idle Timer ───────────────────────────────────────────────────── */

export function IdleTimer({ lastEventTime }: { lastEventTime: number }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(id);
  }, []);
  return <>{idleDuration(lastEventTime)}</>;
}

/* ── Agent Drill-Down View ─────────────────────────────────────────── */

export function DrillDownTasksSection({ tasks, agentName }: { tasks: Map<string, TaskItem>; agentName: string }) {
  const [collapsed, setCollapsed] = useState(false);

  const agentTasks = useMemo(() => {
    const result: TaskItem[] = [];
    for (const [, task] of tasks) {
      if (task.owner === agentName || !task.owner) {
        if (task.status !== "deleted") result.push(task);
      }
    }
    return result;
  }, [tasks, agentName]);

  const completedCount = agentTasks.filter((t) => t.status === "completed").length;
  const total = agentTasks.length;
  const progressPct = total > 0 ? Math.round((completedCount / total) * 100) : 0;

  if (total === 0) return null;

  return (
    <div className="rounded-lg border border-mycel-border bg-mycel-surface overflow-hidden mb-4">
      <button
        type="button"
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 w-full px-4 py-2.5 text-left hover:bg-mycel-surface-hover transition-colors"
      >
        <span className="text-mycel-muted text-[10px] select-none w-3 text-center">
          {collapsed ? "▶" : "▼"}
        </span>
        <span className="text-sm font-semibold text-mycel-text">Tasks</span>
        <span className="text-xs text-mycel-muted font-mono tabular-nums">
          ({completedCount}/{total} complete)
        </span>
        <span className="flex-1 mx-2 h-1.5 bg-mycel-bg rounded-full overflow-hidden max-w-[200px]">
          <span
            className="h-full bg-mycel-success rounded-full transition-all duration-300"
            style={{ width: `${progressPct}%` }}
          />
        </span>
      </button>

      {!collapsed && (
        <div className="border-t border-mycel-border px-4 py-2 space-y-1.5">
          {agentTasks.map((task) => {
            const isBlocked = task.blockedBy && task.blockedBy.length > 0 && task.status !== "completed";
            const borderColor = task.status === "completed" ? "border-l-mycel-success" :
              task.status === "in_progress" ? "border-l-mycel-accent" :
              task.status === "pending" ? "border-l-mycel-muted" :
              "border-l-mycel-error";
            return (
              <div key={task.id} className={`flex items-center gap-2 py-1.5 px-2.5 rounded-md bg-mycel-bg border-l-2 ${borderColor} ${isBlocked ? "opacity-50" : ""}`}>
                <span className="text-[11px] text-mycel-muted font-mono shrink-0">#{task.id}</span>
                <span className={`text-sm font-mono min-w-0 ${
                  task.status === "completed" ? "line-through text-mycel-muted" :
                  task.status === "in_progress" ? "text-mycel-accent font-semibold" :
                  "text-mycel-text"
                }`}>
                  {task.subject.length > 80 ? task.subject.slice(0, 77) + "..." : task.subject}
                </span>
                {isBlocked && (
                  <span className="text-[10px] text-mycel-warning font-mono shrink-0">
                    Blocked by {task.blockedBy!.map((b) => `#${b}`).join(", ")}
                  </span>
                )}
                <span className={`text-[10px] px-2 py-0.5 rounded-full font-mono capitalize shrink-0 ml-auto ${
                  task.status === "completed" ? "bg-mycel-success-subtle text-mycel-success" :
                  task.status === "in_progress" ? "bg-mycel-accent-subtle text-mycel-accent" :
                  task.status === "pending" ? "bg-mycel-surface-hover text-mycel-text-2" :
                  "bg-mycel-error-subtle text-mycel-error"
                }`}>
                  {task.status.replace("_", " ")}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ── Drill-Down: Raw event type badge colors ──────────────────────── */

export function rawEventBadgeColor(eventType: string): string {
  if (eventType === "PreToolUse") return "bg-mycel-info-subtle text-mycel-info";
  if (eventType === "PostToolUse") return "bg-mycel-success-subtle text-mycel-success";
  if (eventType === "PostToolUseFailure") return "bg-mycel-error-subtle text-mycel-error";
  if (eventType === "UserPromptSubmit") return "bg-mycel-accent-subtle text-mycel-accent";
  if (eventType.startsWith("Subagent")) return "bg-mycel-warning-subtle text-mycel-warning";
  if (eventType === "PermissionRequest" || eventType === "Elicitation") return "bg-mycel-warning-subtle text-mycel-warning";
  if (eventType === "SessionStart" || eventType === "SessionEnd") return "bg-mycel-surface-hover text-mycel-text-2";
  return "bg-mycel-surface-hover text-mycel-muted";
}

export function AgentDrillDown({
  activity,
  rawEvents,
  tasks,
  onBack,
  hideRawStream,
  emptyState,
}: {
  activity: AgentActivity;
  rawEvents: RawEvent[];
  tasks: Map<string, TaskItem>;
  onBack?: () => void;
  hideRawStream?: boolean;
  /**
   * Shown instead of a bare "no tool events" line when the stream is empty.
   * An agent can have lifecycle events (started, state changed) and still no
   * tool events at all, which is what a broken agent looks like — so callers
   * that know the agent's state can explain the silence instead of describing
   * it. See EmptyFeed.
   */
  emptyState?: ReactNode;
}) {
  const [activeTab, setActiveTab] = useState<DrillDownTab>("live");
  const [rawExpanded, setRawExpanded] = useState<Set<string>>(new Set());

  const cost = estimateCost(activity);

  // Show ALL events individually — flat, newest at top, no aggregation.
  // Subagent child tool calls are flattened into the same stream so
  // nothing hides inside a nested tree.
  const allNodes = useMemo(() => {
    return flattenNodes(activity.nodes).sort((a, b) => b.startTime - a.startTime);
  }, [activity.nodes]);

  // Running rows are pinned above the chronological stream; a completed
  // row leaves the pinned band and re-joins the list in time order.
  const { running: runningNodes, rest: restNodes } = useMemo(
    () => partitionRunning(allNodes),
    [allNodes],
  );

  const toggleRawExpanded = useCallback((key: string) => {
    setRawExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  // Reverse raw events so newest at top
  const reversedRawEvents = useMemo(() => [...rawEvents].reverse(), [rawEvents]);

  const tabs: { key: DrillDownTab; label: string }[] = [
    { key: "live", label: "Live Stream" },
    ...(hideRawStream ? [] : [{ key: "raw" as DrillDownTab, label: "Raw Stream" }]),
  ];

  return (
    <div className="flex flex-col h-full">
      {/* Header bar — only shown when onBack is provided (live page drill-down) */}
      {onBack && (
        <div className="flex items-center gap-3 mb-4 flex-wrap">
          <button
            type="button"
            onClick={onBack}
            className="inline-flex items-center gap-1.5 text-sm text-mycel-muted hover:text-mycel-text px-2 py-1 rounded border border-mycel-border hover:border-mycel-accent transition-colors shrink-0"
          >
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M8 2l-4 4 4 4" />
            </svg>
            Back
          </button>
          <LiveAgentCharacter name={activity.name} state={activity.state} size={30} tool={activity.tool} />
          <span className="text-lg font-bold text-mycel-text">{activity.name}</span>
          {activity.role && activity.role !== "base" && (
            <span className="text-xs text-mycel-muted font-mono">({activity.role})</span>
          )}
          <span className="text-xs text-mycel-muted capitalize font-mono">{activity.state}</span>
          {activity.tokens > 0 && (
            <span className="text-xs text-mycel-muted font-mono tabular-nums">
              {activity.tokens.toLocaleString()} tok
            </span>
          )}
          {cost > 0 && (
            <span className="text-xs text-mycel-success font-mono tabular-nums">
              ${cost.toFixed(2)}
            </span>
          )}
          {activity.task && (
            <span className="text-xs text-mycel-muted truncate max-w-[400px]">{activity.task}</span>
          )}
        </div>
      )}

      {/* Tasks section — above tabs */}
      <DrillDownTasksSection tasks={tasks} agentName={activity.name} />

      {/* Tabs */}
      <div className="flex items-center gap-0 border-b border-mycel-border mb-4">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === tab.key
                ? "border-mycel-accent text-mycel-text"
                : "border-transparent text-mycel-muted hover:text-mycel-text hover:border-mycel-border"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {/* Live Stream tab — flat rows, newest first */}
        {activeTab === "live" && (
          <div>
            {allNodes.length === 0 ? (
              emptyState ?? (
                <div className="text-sm text-mycel-muted italic py-8 text-center">
                  No tool events yet for this agent.
                </div>
              )
            ) : (
              <>
                <RunningSection nodes={runningNodes} sticky />
                {restNodes.map((node) => (
                  <EventRow key={node.id} node={node} />
                ))}
              </>
            )}
          </div>
        )}

        {/* Raw Stream tab — timestamped JSON blocks, newest at top */}
        {activeTab === "raw" && (
          <div className="space-y-1">
            {reversedRawEvents.length === 0 ? (
              <div className="text-sm text-mycel-muted italic py-8 text-center">
                No raw events captured for this agent yet.
              </div>
            ) : (
              reversedRawEvents.map((evt) => {
                const evtKey = `${evt.timestamp}-${evt.eventType}`;
                const isOpen = rawExpanded.has(evtKey);
                const jsonStr = JSON.stringify(redactValue(evt.raw), null, 2);
                return (
                  <div key={evtKey} className="border border-mycel-border rounded bg-mycel-surface overflow-hidden">
                    <button
                      type="button"
                      onClick={() => toggleRawExpanded(evtKey)}
                      className="flex items-center gap-2 w-full px-3 py-1.5 text-left hover:bg-mycel-surface-hover transition-colors"
                    >
                      <span className="text-mycel-muted text-[10px] select-none w-3 text-center">
                        {isOpen ? "▼" : "▶"}
                      </span>
                      <span className="text-[10px] text-mycel-muted font-mono tabular-nums shrink-0">
                        {new Date(evt.timestamp).toISOString().replace("T", " ").slice(0, 23)}
                      </span>
                      <span className={`text-[11px] font-mono font-medium shrink-0 px-1.5 py-0.5 rounded ${rawEventBadgeColor(evt.eventType)}`}>
                        {evt.eventType}
                      </span>
                      <span className="text-[11px] text-mycel-muted font-mono truncate flex-1">
                        {jsonStr.length > 100 ? jsonStr.slice(0, 97) + "..." : jsonStr.replace(/\n/g, " ")}
                      </span>
                    </button>
                    {isOpen && (
                      <div className="border-t border-mycel-border px-3 py-2 bg-mycel-bg">
                        <div className="flex justify-end mb-1">
                          <CopyButton text={jsonStr} />
                        </div>
                        <pre className="text-[11px] font-mono text-mycel-muted whitespace-pre-wrap break-all max-h-64 overflow-y-auto">
                          {jsonStr.split("\n").map((line, i) => (
                            <span key={i} className="flex">
                              <span className="select-none text-mycel-muted w-8 text-right pr-3 shrink-0 tabular-nums">{i + 1}</span>
                              <span>{line}</span>
                              {"\n"}
                            </span>
                          ))}
                        </pre>
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/* ── Agent Activity Card ───────────────────────────────────────────── */

/** Cap on visible rows in a Live card — the full stream lives in the
 *  agent detail page (linked below the stream). */
export const LIVE_CARD_MAX_ROWS = 30;

export const AgentCard = memo(function AgentCard({
  activity,
  onToggle,
  onDrillDown,
  isFilterActive,
  searchTerm,
  typeFilter,
}: {
  activity: AgentActivity;
  onToggle: () => void;
  onDrillDown: () => void;
  isFilterActive: boolean;
  searchTerm: string;
  typeFilter: FilterType;
}) {
  // Flat hook-event stream — same visual language as the agent detail
  // page. Newest at top, stable keys, no aggregation, no reordering:
  // a finished tool call updates in place, it never moves.
  const flatNodes = useMemo(() => {
    return flattenNodes(activity.nodes).sort((a, b) => b.startTime - a.startTime);
  }, [activity.nodes]);

  const visibleNodes = useMemo(() => {
    if (!searchTerm) return flatNodes;
    const q = searchTerm.toLowerCase();
    return flatNodes.filter((n) => nodeMatchesSearch(n, q));
  }, [flatNodes, searchTerm]);

  // Pin running rows above the chronological list; only the completed
  // rows are subject to the row cap, so an in-flight call is never hidden.
  const { running: runningNodes, rest: restNodes } = useMemo(
    () => partitionRunning(visibleNodes),
    [visibleNodes],
  );
  const shownNodes = restNodes.slice(0, LIVE_CARD_MAX_ROWS);
  const hiddenCount = restNodes.length - shownNodes.length;

  const errorCount = flatNodes.filter((n) => n.status === "failed").length;
  const matchCount = searchTerm ? visibleNodes.length : 0;
  const showToolNodes = typeFilter !== "state";

  const cost = estimateCost(activity);
  const isIdle = activity.state === "idle" || activity.state === "stopped";

  return (
    <motion.div
      className={`rounded-lg border bg-mycel-surface overflow-hidden transition-colors ${isFilterActive ? "border-mycel-accent ring-1 ring-mycel-accent" : "border-mycel-border"}`}
      whileHover={{ y: -1 }}
      transition={{ duration: 0.15 }}
    >
      <div className="flex items-center">
        {/* Expand/collapse chevron for tool list */}
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); onToggle(); }}
          className="flex items-center justify-center min-h-[44px] min-w-[44px] hover:bg-mycel-surface-hover transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent shrink-0"
          aria-label={`${activity.collapsed ? "Expand" : "Collapse"} ${activity.name} tool list`}
        >
          <motion.svg
            width="12" height="12" viewBox="0 0 12 12" fill="none"
            stroke="currentColor" strokeWidth="2"
            className="text-mycel-muted"
            animate={{ rotate: activity.collapsed ? 0 : 90 }}
            transition={{ duration: 0.15 }}
          >
            <path d="M4 2l4 4-4 4" />
          </motion.svg>
        </button>

        {/* Clickable header area -- opens drill-down */}
        <button
          type="button"
          onClick={onDrillDown}
          className="group flex-1 flex items-center gap-3 py-3 pr-4 hover:bg-mycel-surface-hover transition-colors text-left focus-visible:ring-2 focus-visible:ring-mycel-accent min-w-0 cursor-pointer"
          title={`Open ${activity.name} detail view`}
        >
          {/* Living character — reacts to state + event pulses */}
          <span className="shrink-0">
            <LiveAgentCharacter
              name={activity.name}
              state={activity.state}
              size={30}
              tool={activity.tool}
            />
          </span>

          {/* Name + role + state */}
          <div className="flex flex-col min-w-0">
            <span className="flex items-center gap-2">
              <span className="font-bold text-[15px] text-mycel-text leading-tight">
                {activity.name}
              </span>
              <StateDot state={activity.state} />
              {errorCount > 0 && (
                <span
                  className="inline-flex items-center gap-1 pl-1 pr-1.5 h-[18px] text-[10px] font-mono font-medium text-mycel-error border border-mycel-border bg-mycel-error-subtle rounded leading-none"
                  title={`${errorCount} failed tool ${errorCount === 1 ? "call" : "calls"} in this session`}
                  aria-label={`${errorCount} failed tool ${errorCount === 1 ? "call" : "calls"}`}
                >
                  <svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" strokeWidth="1.6">
                    <circle cx="5" cy="5" r="3.5" />
                    <path d="M5 3.5v2M5 6.5v.01" strokeLinecap="round" />
                  </svg>
                  <span className="tabular-nums">{errorCount}</span>
                </span>
              )}
              {(activity.missingSecrets?.length ?? 0) > 0 && (
                <span
                  className="inline-flex items-center h-[18px] px-1.5 text-[10px] font-mono font-medium text-mycel-warning border border-mycel-border bg-mycel-warning-subtle rounded leading-none"
                  title={`Missing secrets: ${activity.missingSecrets!.join(", ")}`}
                  aria-label={`Degraded — missing secrets: ${activity.missingSecrets!.join(", ")}`}
                >
                  degraded
                </span>
              )}
            </span>
            <span className="flex items-center gap-2 mt-0.5">
              {activity.role && activity.role !== "base" && (
                <span className="text-[11px] text-mycel-muted font-mono">{activity.role}</span>
              )}
              <span className={`text-[10px] font-mono capitalize px-1.5 py-0.5 rounded-full leading-none ${stateBadgeClass(activity.state)}`}>
                {activity.state}
              </span>
            </span>
          </div>

          {searchTerm && matchCount > 0 && (
            <span className="text-[11px] text-mycel-accent font-mono shrink-0">
              {matchCount} {matchCount === 1 ? "match" : "matches"}
            </span>
          )}

          {activity.task && (
            <span className="text-[12px] text-mycel-muted truncate max-w-[250px] hidden lg:inline">
              {activity.task}
            </span>
          )}

          {/* Quiet secondary metadata — cost + tokens as muted, right-
              aligned figures (tabular-nums so they don't jitter). The
              running count is intentionally NOT shown here: the pinned
              "Running n" section header below already labels it, and
              duplicating it in the toolbar was noise. */}
          <span className="ml-auto flex items-center gap-3 shrink-0 text-[11px] font-mono tabular-nums text-mycel-muted">
            {cost > 0 && (
              <span className="text-mycel-text-2" title={activity.costUsd > 0 ? "From API" : "Estimated from tokens"}>
                ${cost.toFixed(2)}
              </span>
            )}
            {activity.tokens > 0 && (
              <span>{activity.tokens.toLocaleString()} tok</span>
            )}
            {/* Idle chip */}
            {isIdle && activity.lastEventTime > 0 && (
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-mycel-surface-hover">
                <IdleTimer lastEventTime={activity.lastEventTime} />
              </span>
            )}
            <span className="text-mycel-muted group-hover:text-mycel-accent transition-colors hidden sm:inline">
              &rarr;
            </span>
          </span>
        </button>
      </div>

      <AnimatePresence initial={false}>
        {!activity.collapsed && showToolNodes && (shownNodes.length > 0 || runningNodes.length > 0) && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.2, ease: "easeOut" }}
            className="border-t border-mycel-border overflow-hidden"
          >
            {/* Flat event stream — newest first. Running rows pin to the
                top; completed rows are keyed by event id and never animate
                position, so new events prepend without reflowing below. */}
            <RunningSection nodes={runningNodes} searchQuery={searchTerm} />
            {shownNodes.map((node) => (
              <EventRow key={node.id} node={node} searchQuery={searchTerm} />
            ))}
            {hiddenCount > 0 && (
              <div className="flex justify-center border-t border-mycel-border">
                <Link
                  to={`/agents/${activity.name}/live`}
                  className="text-[11px] text-mycel-muted hover:text-mycel-accent font-mono py-1.5 px-3 transition-colors"
                >
                  {hiddenCount} more — view all in {activity.name} &rarr;
                </Link>
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {!activity.collapsed && showToolNodes && visibleNodes.length === 0 && searchTerm && (
        <div className="border-t border-mycel-border py-3 px-4 text-[12px] text-mycel-muted italic">
          No events match &ldquo;{searchTerm}&rdquo;
        </div>
      )}

      {!activity.collapsed && showToolNodes && visibleNodes.length === 0 && !searchTerm && (
        <div className="border-t border-mycel-border py-3 px-4 text-[12px] text-mycel-muted italic flex items-center gap-2">
          {activity.lastEventTime > 0 ? (
            <>
              <span>No recent events</span>
              <span className="text-mycel-border">·</span>
              <IdleTimer lastEventTime={activity.lastEventTime} />
            </>
          ) : (
            "Waiting for activity..."
          )}
        </div>
      )}

      {!activity.collapsed && typeFilter === "state" && (
        <div className="border-t border-mycel-border py-3 px-4 text-[12px] text-mycel-muted">
          <span className="capitalize font-medium text-mycel-text">{activity.state}</span>
          {activity.task && <span className="ml-2">--- {activity.task}</span>}
          {activity.tokens > 0 && (
            <span className="ml-2 font-mono tabular-nums">{activity.tokens.toLocaleString()} tokens</span>
          )}
        </div>
      )}
    </motion.div>
  );
});
