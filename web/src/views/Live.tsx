import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import type { Agent } from "../api/client";
import { useAgentActivity } from "../hooks/useAgentActivity";
import { EmptyState } from "../components/EmptyState";
import type {
  FilterType,
} from "../components/live/liveTypes";
import {
  nodeMatchesSearch,
} from "../components/live/liveHelpers";
import { AgentCard, AgentDrillDown } from "../components/live/LiveRenderers";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
/* ── Live (Live Operations Center) ─────────────────────────────────── */

/** One stat tile in the Live summary strip.
 *
 *  Editorial "billboard" treatment: the number is the display element
 *  (Geist Sans, 24px, semibold, tabular). Label is a tiny caps caption
 *  above it. A subtle vertical hairline separates cells so the strip
 *  reads as one composed row without the old welded-grid look. */
function SummaryStat({
  label,
  value,
  accent,
  mono,
  first,
}: {
  label: string;
  value: React.ReactNode;
  accent?: string;
  mono?: boolean;
  /** When true, suppress the left divider (first cell in row). */
  first?: boolean;
}) {
  return (
    <div
      className={`relative flex flex-col justify-between gap-1.5 px-5 py-3 ${
        first ? "" : "sm:before:absolute sm:before:inset-y-3 sm:before:left-0 sm:before:w-px sm:before:bg-mycel-border/50 sm:before:content-['']"
      }`}
    >
      <span className="text-[9px] uppercase tracking-[0.12em] text-mycel-muted/60 font-medium">
        {label}
      </span>
      <span
        className={`text-[22px] leading-none tabular-nums ${
          mono ? "font-mono" : "font-semibold"
        } ${accent ?? "text-mycel-text"}`}
      >
        {value}
      </span>
    </div>
  );
}

function formatTokens(n: number): string {
  if (n <= 0) return "0";
  if (n < 1_000) return n.toString();
  if (n < 1_000_000) return `${(n / 1_000).toFixed(1)}k`;
  return `${(n / 1_000_000).toFixed(2)}M`;
}

/** Compact relative-time renderer (mm:ss / Xs / Ym) for the last-event tile. */
function RelTime({ ms }: { ms: number }) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  const delta = Math.max(0, Math.floor((now - ms) / 1000));
  if (delta < 60) return <>{delta}s ago</>;
  if (delta < 3600) return <>{Math.floor(delta / 60)}m ago</>;
  if (delta < 86400) return <>{Math.floor(delta / 3600)}h ago</>;
  return <>{Math.floor(delta / 86400)}d ago</>;
}


export const SHOW_STOPPED_STORAGE_KEY = "bc-live-show-stopped";
export const ACTIVE_STATES = new Set(["idle", "starting", "working", "stuck", "done"]);

function readShowStopped(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(SHOW_STOPPED_STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

function writeShowStopped(value: boolean): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(SHOW_STOPPED_STORAGE_KEY, value ? "1" : "0");
  } catch {
    /* ignore */
  }
}

export function Live() {
  useHeaderSlot({
    title: (
      <TabHeaderTitle>
        <span className="inline-flex items-center gap-2">
          Live
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-mycel-error opacity-75" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-mycel-error" />
          </span>
        </span>
      </TabHeaderTitle>
    ),
  });

  const { activities, tasks, rawEventsRef, connected, reconnecting, eventCount } = useAgentActivity();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentFilter, setAgentFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState<FilterType>("all");
  const [searchFilter, setSearchFilter] = useState("");
  const [showStopped, setShowStoppedState] = useState<boolean>(() => readShowStopped());

  const setShowStopped = useCallback((value: boolean | ((prev: boolean) => boolean)) => {
    setShowStoppedState((prev) => {
      const next = typeof value === "function" ? (value as (p: boolean) => boolean)(prev) : value;
      writeShowStopped(next);
      return next;
    });
  }, []);
  const [paused, setPaused] = useState(false);
  const [pausedCount, setPausedCount] = useState(0);
  const [collapsedOverrides, setCollapsedOverrides] = useState<Map<string, boolean>>(new Map());
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);
  const [newEventsSinceScroll, setNewEventsSinceScroll] = useState(0);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [focusedCardIdx, setFocusedCardIdx] = useState(-1);
  const [drillDownAgent, setDrillDownAgent] = useState<string | null>(null);
  const [rawEventsVersion, setRawEventsVersion] = useState(0);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Seed agent list for the filter dropdown
  useEffect(() => {
    api.listAgents().then((agentList) => {
      setAgents(agentList);
    }).catch(() => {});
  }, []);

  // Track rawEventsVersion for re-render when raw events change
  useEffect(() => {
    // rawEventsRef is mutated by the hook — bump version on each hook event
    // by subscribing to eventCount changes (which the hook increments per event)
    setRawEventsVersion((v) => v + 1);
  }, [eventCount]);

  const handleResume = useCallback(() => {
    setPaused(false);
    setPausedCount(0);
  }, []);

  // Count active/stopped for the badge — before the show-stopped filter applies.
  // Also computes the secondary stats (idle, error, tokens, last-event) the
  // summary strip surfaces above the agent cards.
  const summary = useMemo(() => {
    let active = 0, idle = 0, working = 0, errored = 0, stopped = 0, tokens = 0, lastEvent = 0;
    for (const a of activities.values()) {
      if (ACTIVE_STATES.has(a.state)) active++;
      else stopped++;
      if (a.state === "idle") idle++;
      else if (a.state === "working" || a.state === "starting") working++;
      else if (a.state === "error" || a.state === "stuck") errored++;
      tokens += a.tokens || 0;
      if (a.lastEventTime > lastEvent) lastEvent = a.lastEventTime;
    }
    return { active, idle, working, errored, stopped, tokens, lastEvent };
  }, [activities]);
  const { active: activeCount, stopped: stoppedCount } = summary;

  const sorted = useMemo(() => {
    const filtered = Array.from(activities.values())
      .map((a) => collapsedOverrides.has(a.name) ? { ...a, collapsed: collapsedOverrides.get(a.name)! } : a)
      .filter((a) => {
        // Hide stopped/error agents unless the user opts in, but never hide an
        // agent the user explicitly selected in the agent filter dropdown.
        if (!showStopped && !ACTIVE_STATES.has(a.state) && agentFilter !== a.name) return false;
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
  }, [activities, collapsedOverrides, agentFilter, typeFilter, searchFilter, showStopped]);

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
    setCollapsedOverrides((prev) => {
      const next = new Map(prev);
      const activity = activities.get(name);
      const currentCollapsed = prev.has(name) ? prev.get(name)! : (activity?.collapsed ?? false);
      next.set(name, !currentCollapsed);
      return next;
    });
  }, [activities]);

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
  }, [sorted, focusedCardIdx, toggleAgent]);  

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
      {/* Toolbar: status indicators + actions (title is in the top-bar chip) */}
      <div className="flex items-center justify-end mb-4">
        <span className="flex items-center gap-2">
          {/* SSE connection indicator */}
          <span className={`inline-flex items-center gap-1.5 px-2 py-1 rounded-md text-[11px] font-mono ${connected ? "bg-mycel-success/10 text-mycel-success" : reconnecting ? "bg-mycel-warning/10 text-mycel-warning" : "bg-mycel-error/10 text-mycel-error"}`} title={sseTooltip}>
            <span className={`inline-flex h-1.5 w-1.5 rounded-full ${sseDotColor}${reconnecting ? " animate-pulse" : ""}`} />
            <span className="hidden sm:inline">{connected ? "Connected" : reconnecting ? "Reconnecting" : "Disconnected"}</span>
          </span>
          {/* Event count pill */}
          <span className="text-[11px] text-mycel-muted font-mono tabular-nums px-2 py-1 rounded-md bg-mycel-surface border border-mycel-border">{eventCount.toLocaleString()} events</span>
          {/* Pause/Resume button */}
          <button
            type="button"
            onClick={() => paused ? handleResume() : setPaused(true)}
            className={`relative inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-md border transition-colors ${paused ? "border-mycel-warning bg-mycel-warning/15 text-mycel-warning hover:bg-mycel-warning/25" : "border-mycel-border hover:border-mycel-accent bg-mycel-surface text-mycel-text"}`}
            title={paused ? `Resume (${pausedCount} buffered)` : "Pause stream"}
          >
            {paused ? (
              <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor"><polygon points="1,0 10,5 1,10" /></svg>
            ) : (
              <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor"><rect x="1" y="0" width="3" height="10" /><rect x="6" y="0" width="3" height="10" /></svg>
            )}
            <span className="hidden sm:inline">{paused ? "Resume" : "Pause"}</span>
            {paused && pausedCount > 0 && (
              <span className="text-[10px] font-bold text-mycel-warning ml-0.5">({pausedCount})</span>
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
            className="inline-flex items-center gap-1.5 text-xs px-2.5 py-1 rounded-md border border-mycel-border hover:border-mycel-accent bg-mycel-surface text-mycel-muted hover:text-mycel-text transition-colors"
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
            className="inline-flex items-center justify-center h-7 w-7 rounded-md border border-mycel-border hover:border-mycel-accent bg-mycel-surface text-mycel-muted hover:text-mycel-text text-xs transition-colors"
            title="Keyboard shortcuts (?)"
          >
            ?
          </button>
        </span>
      </div>

      {/* Keyboard Shortcuts Overlay */}
      {showShortcuts && (
        <div className="absolute top-16 right-6 z-50 bg-mycel-surface border border-mycel-border rounded-lg shadow-lg p-4 w-64">
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm font-semibold text-mycel-text">Keyboard Shortcuts</span>
            <button
              type="button"
              onClick={() => setShowShortcuts(false)}
              className="text-mycel-muted hover:text-mycel-text text-sm"
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
                <kbd className="inline-flex items-center justify-center min-w-[24px] h-5 px-1.5 rounded bg-mycel-bg border border-mycel-border text-mycel-text font-mono text-[11px]">
                  {key}
                </kbd>
                <span className="text-mycel-muted">{desc}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-2 mb-4 sticky top-0 z-10 bg-mycel-bg py-2">
        <select
          value={agentFilter}
          onChange={(e) => setAgentFilter(e.target.value)}
          className="text-sm rounded-md border border-mycel-border bg-mycel-surface px-2.5 py-1.5 text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent appearance-none pr-7"
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
          className="text-sm rounded-md border border-mycel-border bg-mycel-surface px-2.5 py-1.5 text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent appearance-none pr-7"
          style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg width='10' height='6' viewBox='0 0 10 6' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%238c7e72' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E")`, backgroundRepeat: "no-repeat", backgroundPosition: "right 8px center" }}
        >
          <option value="all">All</option>
          <option value="tools">Tool Calls</option>
          <option value="state">State Changes</option>
        </select>
        {/* Search with magnifying glass icon */}
        <div className="relative">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-mycel-muted pointer-events-none">
            <circle cx="6" cy="6" r="4.5" />
            <path d="M9.5 9.5L13 13" />
          </svg>
          <input
            ref={searchInputRef}
            type="text"
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            placeholder="Search events... (/ to focus)"
            className="text-sm rounded-md border border-mycel-border bg-mycel-surface pl-8 pr-2.5 py-1.5 text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent w-56"
          />
        </div>
        {/* Active/stopped count badge with toggle */}
        <span
          className="inline-flex items-center gap-1.5 whitespace-nowrap text-[11px] font-mono px-2 py-1 rounded-md border border-mycel-border bg-mycel-surface text-mycel-muted"
          data-testid="live-state-badge"
          title={showStopped ? "Showing all agents" : "Stopped/errored agents hidden"}
        >
          <span className="text-mycel-success tabular-nums">{activeCount}</span>
          <span>active</span>
          {stoppedCount > 0 && (
            <>
              <span className="text-mycel-border">&middot;</span>
              <span className="tabular-nums">{stoppedCount}</span>
              <span>stopped</span>
              <button
                type="button"
                onClick={() => setShowStopped((prev) => !prev)}
                className="text-mycel-accent hover:text-mycel-text underline decoration-dotted underline-offset-2 transition-colors"
                aria-pressed={showStopped}
                aria-label={showStopped ? "Hide stopped agents" : "Show stopped agents"}
                data-testid="toggle-show-stopped"
              >
                ({showStopped ? "hide" : "show"})
              </button>
            </>
          )}
        </span>
        {/* Active filter pills */}
        {hasFilters && (
          <div className="flex items-center gap-1.5">
            {agentFilter && (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-1 rounded-full bg-mycel-accent/10 text-mycel-accent border border-mycel-accent/30">
                {agentFilter}
                <button type="button" onClick={() => setAgentFilter("")} className="hover:text-mycel-text ml-0.5" aria-label="Remove agent filter">&times;</button>
              </span>
            )}
            {typeFilter !== "all" && (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-1 rounded-full bg-mycel-accent/10 text-mycel-accent border border-mycel-accent/30">
                {typeFilter === "tools" ? "Tool Calls" : "State Changes"}
                <button type="button" onClick={() => setTypeFilter("all")} className="hover:text-mycel-text ml-0.5" aria-label="Remove type filter">&times;</button>
              </span>
            )}
            {searchFilter && (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono px-2 py-1 rounded-full bg-mycel-accent/10 text-mycel-accent border border-mycel-accent/30">
                &ldquo;{searchFilter.length > 16 ? searchFilter.slice(0, 14) + "..." : searchFilter}&rdquo;
                <button type="button" onClick={() => setSearchFilter("")} className="hover:text-mycel-text ml-0.5" aria-label="Remove search filter">&times;</button>
              </span>
            )}
            <button
              type="button"
              onClick={() => { setAgentFilter(""); setTypeFilter("all"); setSearchFilter(""); }}
              className="text-[11px] text-mycel-muted hover:text-mycel-text px-2 py-1 rounded-md border border-mycel-border hover:border-mycel-accent transition-colors"
            >
              Clear all
            </button>
          </div>
        )}
      </div>

      {/* Summary strip — at-a-glance counters + tokens + last-event time.
          Rendered only when there is at least one agent so the page doesn't
          show a "0 / 0 / 0" header on the cold-start empty state. */}
      {activities.size > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-5 rounded-lg border border-mycel-border bg-mycel-surface mb-4">
          <SummaryStat label="Working" value={summary.working} accent="text-emerald-300" first />
          <SummaryStat label="Idle" value={summary.idle} accent="text-amber-300" />
          <SummaryStat label="Errored" value={summary.errored} accent={summary.errored > 0 ? "text-rose-300" : "text-mycel-muted/60"} />
          <SummaryStat label="Tokens" value={formatTokens(summary.tokens)} mono />
          <SummaryStat label="Last event" value={summary.lastEvent > 0 ? <RelTime ms={summary.lastEvent} /> : "—"} mono />
        </div>
      )}

      {/* Agent Activity Cards.
          overflow-anchor: none stops the browser auto-scrolling when a
          card above the viewport changes height (the source of the
          "jumps to bottom / latest at top swap" the team flagged).
          Newest is anchored at the top via the sort comparator above,
          and the explicit Jump-to-latest button handles intentional
          recentering when the user has scrolled away. */}
      <div
        ref={scrollContainerRef}
        className="flex-1 overflow-y-auto min-h-0 space-y-3 relative max-h-full"
        style={{ overflowAnchor: "none", scrollbarGutter: "stable" }}
      >
        {sorted.length === 0 ? (
          !showStopped && activeCount === 0 && stoppedCount > 0 ? (
            <EmptyState
              icon=">"
              title="No active agents"
              description={`${stoppedCount} stopped or errored ${stoppedCount === 1 ? "agent is" : "agents are"} hidden. Click "(show)" above to reveal.`}
            />
          ) : (
            <EmptyState
              icon=">"
              title="No activity yet"
              description="Events will stream here in real-time as agents work."
            />
          )
        ) : (
          sorted.map((activity, idx) => (
            <div
              key={activity.name}
              className={focusedCardIdx === idx ? "ring-2 ring-mycel-accent rounded-lg" : ""}
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
          className="absolute bottom-8 right-8 z-20 inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-mycel-border bg-mycel-surface text-mycel-text text-sm shadow-lg hover:border-mycel-accent hover:bg-mycel-surface-hover transition-colors"
        >
          <span>&darr;</span>
          Jump to latest
          {newEventsSinceScroll > 0 && (
            <span className="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 text-[11px] font-bold text-white bg-mycel-accent rounded-full leading-none">
              {newEventsSinceScroll}
            </span>
          )}
        </button>
      )}
    </div>
  );
}
