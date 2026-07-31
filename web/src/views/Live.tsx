import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAgentActivity } from "../hooks/useAgentActivity";
import { EmptyState } from "../components/EmptyState";
import type { FilterType } from "../components/live/liveTypes";
import { flattenNodes, nodeMatchesSearch } from "../components/live/liveHelpers";
import { AgentCard, AgentDrillDown } from "../components/live/LiveRenderers";

import { useHeaderSlot } from "../context/HeaderSlotContext";

/* ── Live ──────────────────────────────────────────────────────────────
 *
 * Calm control room. The page answers one question at a glance — "what
 * are my agents doing right now" — with a single presence sentence and
 * the agent cards themselves. Everything secondary (pause, export, type
 * filter, shortcuts) lives behind one ⋯ menu so the default surface has
 * exactly three elements: presence, search, more.
 */

export const SHOW_STOPPED_STORAGE_KEY = "mycel-live-show-stopped";
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

/** The one-line fleet summary. Reads as a sentence, not a dashboard:
 *  `● 2 working · 1 idle · 3 stopped (hidden)`. The leading dot doubles
 *  as the SSE health indicator — quiet green pulse when live, amber when
 *  reconnecting, red when down. Counts only speak when nonzero. */
function PresenceLine({
  working,
  idle,
  errored,
  stopped,
  showStopped,
  onToggleStopped,
  connected,
  reconnecting,
}: {
  working: number;
  idle: number;
  errored: number;
  stopped: number;
  showStopped: boolean;
  onToggleStopped: () => void;
  connected: boolean;
  reconnecting: boolean;
}) {
  const dotColor = connected ? "bg-mycel-success" : reconnecting ? "bg-mycel-warning" : "bg-mycel-error";
  const dotTitle = connected ? "Live — event stream connected" : reconnecting ? "Reconnecting…" : "Disconnected";
  const parts: React.ReactNode[] = [];
  if (working > 0) {
    parts.push(
      <span key="working" className="text-mycel-text">
        <span className="font-semibold tabular-nums">{working}</span> working
      </span>,
    );
  }
  if (idle > 0) {
    parts.push(
      <span key="idle" className="text-mycel-muted">
        <span className="font-semibold tabular-nums">{idle}</span> idle
      </span>,
    );
  }
  if (errored > 0) {
    parts.push(
      <span key="errored" className="text-mycel-error">
        <span className="font-semibold tabular-nums">{errored}</span> stuck
      </span>,
    );
  }
  if (parts.length === 0) {
    parts.push(
      <span key="none" className="text-mycel-muted">
        no active agents
      </span>,
    );
  }

  return (
    <div
      className="flex items-center gap-2 text-sm min-w-0"
      data-testid="live-state-badge"
    >
      <span className="relative flex h-2 w-2 shrink-0" title={dotTitle}>
        {connected && (
          <span className={`absolute inline-flex h-full w-full rounded-full ${dotColor} opacity-60 animate-ping [animation-duration:2.5s]`} />
        )}
        <span className={`relative inline-flex h-2 w-2 rounded-full ${dotColor}${reconnecting ? " animate-pulse" : ""}`} />
      </span>
      {!connected && (
        <span className={`text-xs ${reconnecting ? "text-mycel-warning" : "text-mycel-error"}`}>
          {reconnecting ? "reconnecting" : "disconnected"}
        </span>
      )}
      <span className="flex items-center gap-1.5 truncate">
        {parts.map((p, i) => (
          <span key={i} className="flex items-center gap-1.5">
            {i > 0 && <span className="text-mycel-border select-none">·</span>}
            {p}
          </span>
        ))}
        {stopped > 0 && (
          <button
            type="button"
            onClick={onToggleStopped}
            aria-pressed={showStopped}
            aria-label={showStopped ? "Hide stopped agents" : "Show stopped agents"}
            data-testid="toggle-show-stopped"
            className="inline-flex items-center gap-1.5 text-mycel-muted hover:text-mycel-text transition-colors"
          >
            <span className="text-mycel-border select-none">·</span>
            <span>
              <span className="font-semibold tabular-nums">{stopped}</span> stopped{" "}
              <span className="underline decoration-dotted underline-offset-2">
                {showStopped ? "(shown)" : "(hidden)"}
              </span>
            </span>
          </button>
        )}
      </span>
    </div>
  );
}

export function Live() {
  const { activities, tasks, rawEventsRef, connected, reconnecting, eventCount } = useAgentActivity();
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
  const [menuOpen, setMenuOpen] = useState(false);
  const [focusedCardIdx, setFocusedCardIdx] = useState(-1);
  const [drillDownAgent, setDrillDownAgent] = useState<string | null>(null);
  const [rawEventsVersion, setRawEventsVersion] = useState(0);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Track rawEventsVersion for re-render when raw events change
  useEffect(() => {
    setRawEventsVersion((v) => v + 1);
  }, [eventCount]);

  const handleResume = useCallback(() => {
    setPaused(false);
    setPausedCount(0);
  }, []);

  const summary = useMemo(() => {
    let idle = 0, working = 0, errored = 0, stopped = 0;
    for (const a of activities.values()) {
      if (!ACTIVE_STATES.has(a.state)) stopped++;
      if (a.state === "idle") idle++;
      else if (a.state === "working" || a.state === "starting") working++;
      else if (a.state === "error" || a.state === "stuck") errored++;
    }
    return { idle, working, errored, stopped };
  }, [activities]);
  const stoppedCount = summary.stopped;
  const activeCount = summary.working + summary.idle;

  const sorted = useMemo(() => {
    const filtered = Array.from(activities.values())
      .map((a) => collapsedOverrides.has(a.name) ? { ...a, collapsed: collapsedOverrides.get(a.name)! } : a)
      .filter((a) => {
        if (!showStopped && !ACTIVE_STATES.has(a.state)) return false;
        if (typeFilter === "tools" && a.nodes.length === 0) return false;
        if (searchFilter) {
          const q = searchFilter.toLowerCase();
          const cardHay = `${a.name} ${a.role} ${a.task} ${a.tool}`.toLowerCase();
          if (cardHay.includes(q)) return true;
          const hasMatchingNode = flattenNodes(a.nodes).some((n) => nodeMatchesSearch(n, q));
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
  }, [activities, collapsedOverrides, typeFilter, searchFilter, showStopped]);

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

  // Close the ⋯ menu on outside click / Escape
  useEffect(() => {
    if (!menuOpen) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

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

  const exportEvents = useCallback(() => {
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
    a.download = `mycel-events-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }, [activities, tasks, eventCount]);

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

  // Drill-down view
  const drillDownActivity = drillDownAgent ? activities.get(drillDownAgent) : null;
  const drillDownRawEvents = drillDownAgent ? (rawEventsRef.current.get(drillDownAgent) ?? []) : [];
  void rawEventsVersion;

  // Header slot — the whole control surface lives in the full-width top
  // bar: presence · search · ⋯. The drawer names the section, so no title.
  useHeaderSlot({
    title: (
      <div className="flex items-center gap-3 min-w-0">
        <PresenceLine
          working={summary.working}
          idle={summary.idle}
          errored={summary.errored}
          stopped={stoppedCount}
          showStopped={showStopped}
          onToggleStopped={() => setShowStopped((prev) => !prev)}
          connected={connected}
          reconnecting={reconnecting}
        />
        {paused && (
          <button
            type="button"
            onClick={handleResume}
            className="inline-flex items-center gap-1.5 text-xs font-medium px-2 py-0.5 rounded-md border border-mycel-warning bg-mycel-warning-subtle text-mycel-warning transition-colors"
            title="Stream paused — click to resume"
          >
            <svg width="9" height="9" viewBox="0 0 10 10" fill="currentColor"><polygon points="1,0 10,5 1,10" /></svg>
            paused{pausedCount > 0 && <span className="tabular-nums">+{pausedCount}</span>}
          </button>
        )}
      </div>
    ),
    actions: drillDownAgent ? undefined : (
      <>
        {/* Search — grows into the free header space, capped at max-w-lg */}
        <div className="relative flex-1 min-w-0 max-w-lg">
          <svg width="13" height="13" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-mycel-muted pointer-events-none">
            <circle cx="6" cy="6" r="4.5" />
            <path d="M9.5 9.5L13 13" />
          </svg>
          <input
            ref={searchInputRef}
            type="text"
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            placeholder="Search  /"
            aria-label="Search events"
            className="w-full h-9 text-sm rounded-md border border-mycel-border bg-mycel-surface pl-8 pr-2.5 text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
          />
        </div>

        {/* ⋯ menu: pause, type filter, export, shortcuts */}
        <div className="relative shrink-0" ref={menuRef}>
          <button
            type="button"
            onClick={() => setMenuOpen((v) => !v)}
            aria-label="More options"
            aria-expanded={menuOpen}
            className={`inline-flex items-center justify-center h-8 w-8 rounded-md border text-base leading-none transition-colors ${menuOpen ? "border-mycel-accent text-mycel-text bg-mycel-surface-hover" : "border-mycel-border bg-mycel-surface text-mycel-muted hover:text-mycel-text hover:border-mycel-accent"}`}
          >
            &#x22EF;
          </button>
          {menuOpen && (
            <div
              data-testid="live-more-menu"
              className="absolute right-0 top-full mt-1.5 z-50 w-56 rounded-lg border border-mycel-border bg-mycel-surface-2 shadow-mycel-lg py-1.5 text-sm"
            >
              <button
                type="button"
                onClick={() => {
                  if (paused) handleResume();
                  else setPaused(true);
                  setMenuOpen(false);
                }}
                className="flex w-full items-center justify-between px-3 py-1.5 text-mycel-text hover:bg-mycel-surface-hover transition-colors"
              >
                <span>{paused ? "Resume stream" : "Pause stream"}</span>
                {paused && pausedCount > 0 && <span className="text-[11px] text-mycel-warning tabular-nums">+{pausedCount}</span>}
              </button>
              <div className="my-1 border-t border-mycel-border" />
              <div className="px-3 pt-1 pb-0.5 text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Show</div>
              {([["all", "Everything"], ["tools", "Tool calls only"], ["state", "State changes only"]] as [FilterType, string][]).map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  onClick={() => setTypeFilter(value)}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-mycel-text hover:bg-mycel-surface-hover transition-colors"
                >
                  <span className={`inline-flex h-3 w-3 items-center justify-center rounded-full border ${typeFilter === value ? "border-mycel-accent" : "border-mycel-border"}`}>
                    {typeFilter === value && <span className="h-1.5 w-1.5 rounded-full bg-mycel-accent" />}
                  </span>
                  {label}
                </button>
              ))}
              <div className="my-1 border-t border-mycel-border" />
              <button
                type="button"
                onClick={() => { exportEvents(); setMenuOpen(false); }}
                className="flex w-full items-center px-3 py-1.5 text-mycel-text hover:bg-mycel-surface-hover transition-colors"
              >
                Export events as JSON
              </button>
              <button
                type="button"
                onClick={() => { setShowShortcuts(true); setMenuOpen(false); }}
                className="flex w-full items-center justify-between px-3 py-1.5 text-mycel-text hover:bg-mycel-surface-hover transition-colors"
              >
                <span>Keyboard shortcuts</span>
                <kbd className="inline-flex items-center justify-center h-4 px-1 rounded bg-mycel-bg border border-mycel-border text-mycel-muted font-mono text-[10px]">?</kbd>
              </button>
            </div>
          )}
        </div>
      </>
    ),
  });

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
      {/* Controls live in the full-width header (presence · search · ⋯) */}

      {/* Keyboard Shortcuts Overlay */}
      {showShortcuts && (
        <div className="absolute top-16 right-6 z-50 bg-mycel-surface-2 border border-mycel-border rounded-lg shadow-mycel-lg p-4 w-64">
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

      {/* Agent activity cards — the agents ARE the dashboard.
          overflow-anchor: none stops the browser auto-scrolling when a
          card above the viewport changes height. */}
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
              description={`${stoppedCount} stopped or errored ${stoppedCount === 1 ? "agent is" : "agents are"} hidden — click "(hidden)" above to reveal.`}
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
                isFilterActive={false}
                searchTerm={searchFilter}
                typeFilter={typeFilter}
              />
            </div>
          ))
        )}
      </div>

      {/* Back-to-latest pill. The feed is newest-first, so "latest" is at
          the TOP — the pill floats top-center (like chat apps' new-message
          pills) and the arrow points up. It turns accent when events
          arrived while scrolled away. */}
      {showJumpToLatest && (
        <button
          type="button"
          onClick={jumpToLatest}
          className={`absolute top-3 left-1/2 -translate-x-1/2 z-20 inline-flex items-center gap-1.5 h-8 px-3.5 rounded-full text-xs font-medium shadow-mycel-lg transition-colors ${
            newEventsSinceScroll > 0
              ? "bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover"
              : "border border-mycel-border bg-mycel-surface-2 text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover"
          }`}
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M8 13V3M3.5 7.5L8 3l4.5 4.5" />
          </svg>
          {newEventsSinceScroll > 0
            ? `${newEventsSinceScroll} new event${newEventsSinceScroll === 1 ? "" : "s"}`
            : "Back to latest"}
        </button>
      )}
    </div>
  );
}
