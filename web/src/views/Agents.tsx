import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Agent, BulkResult } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useWebSocket } from "../hooks/useWebSocket";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { InlineTerminal } from "../components/InlineTerminal";
import { truncate } from "../utils/text";
import { AgentIcon } from "../components/agent-ui";
import { CreateAgentModal } from "../components/CreateAgentModal";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
import { MONO } from "../utils/typography";

/**
 * RuntimeChip / ProviderChip — visually distinguish two adjacent columns
 * whose text values (tmux/docker vs claude/gemini/cursor/pi/codex) used
 * to render as identical monospace pills. Runtime carries a shape glyph
 * (terminal for tmux, container for docker) so at-a-glance readers can
 * tell "how it runs" from "what it runs" without reading the label.
 */
function RuntimeChip({ runtime }: { runtime?: string | null }) {
  if (!runtime) return <span className="text-mycel-muted">—</span>;
  const isDocker = runtime === "docker";
  return (
    <span
      className="inline-flex items-center gap-1.5 text-[11px] px-1.5 py-[3px] rounded border border-mycel-border/40 bg-mycel-surface/40 text-mycel-muted"
      style={{ fontFamily: MONO }}
      title={`Runtime: ${runtime}`}
    >
      <span className="opacity-70">
        {isDocker ? (
          <svg width="10" height="10" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.2">
            <rect x="1" y="4" width="10" height="6" rx="0.5" />
            <rect x="3" y="1.5" width="6" height="2" rx="0.4" />
            <path d="M2.5 5.5h1M4.5 5.5h1M6.5 5.5h1M8.5 5.5h1" opacity="0.5" />
          </svg>
        ) : (
          <svg width="10" height="10" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.2">
            <rect x="1" y="1.5" width="10" height="9" rx="0.8" />
            <path d="M3 5l1.5 1.5L3 8M5.5 8h3" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        )}
      </span>
      {runtime}
    </span>
  );
}

/** Per-provider hue as an accent dot so Provider chips differ from the
 *  Runtime chips beside them. Kept subtle — the dot is muted-hue at
 *  ~50% opacity, not a marketing swatch. */
const PROVIDER_DOTS: Record<string, string> = {
  claude:   "bg-orange-400/70",
  gemini:   "bg-sky-400/70",
  cursor:   "bg-fuchsia-400/70",
  codex:    "bg-emerald-400/70",
  pi:       "bg-teal-400/70",
};

function ProviderChip({ tool }: { tool?: string | null }) {
  if (!tool) return <span className="text-mycel-muted">—</span>;
  const dot = PROVIDER_DOTS[tool] ?? "bg-mycel-muted/50";
  return (
    <span
      className="inline-flex items-center gap-1.5 text-[11px] px-1.5 py-[3px] rounded border border-mycel-border/40 bg-mycel-surface/40 text-mycel-text/85"
      style={{ fontFamily: MONO }}
      title={`Provider: ${tool}`}
    >
      <span className={`inline-block w-1.5 h-1.5 rounded-full ${dot}`} aria-hidden />
      {tool}
    </span>
  );
}

// --- Inline Rename ---

function InlineAgentName({
  agent,
  onRenamed,
}: {
  agent: Agent;
  onRenamed: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [newName, setNewName] = useState(agent.name);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    const trimmed = newName.trim();
    if (!trimmed || trimmed === agent.name) {
      setEditing(false);
      setNewName(agent.name);
      return;
    }
    setSaving(true);
    try {
      await api.renameAgent(agent.name, trimmed);
      setEditing(false);
      onRenamed();
    } catch {
      setNewName(agent.name);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  };

  if (editing) {
    return (
      <input
        type="text"
        value={newName}
        onChange={(e) => setNewName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") handleSave();
          if (e.key === "Escape") {
            setEditing(false);
            setNewName(agent.name);
          }
        }}
        onBlur={handleSave}
        disabled={saving}
        autoFocus
        onClick={(e) => e.stopPropagation()}
        className="px-1 py-0.5 text-sm font-medium rounded border border-mycel-accent bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent w-32"
        aria-label="Rename agent"
      />
    );
  }

  return (
    <span
      className="font-medium"
      // Double-click enters rename mode; single click should fall through
      // to the row click (which opens the agent detail page).
      onDoubleClick={(e) => {
        e.stopPropagation();
        setEditing(true);
      }}
      title="Double-click to rename"
      aria-label={`Agent ${agent.name} (double-click to rename)`}
    >
      {agent.name}
    </span>
  );
}

// --- Agent Action Buttons ---

function AgentActions({ agent, onDone }: { agent: Agent; onDone: () => void }) {
  const [confirming, setConfirming] = useState<"delete" | null>(null);
  const [busy, setBusy] = useState(false);

  const act = async (action: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await action();
      onDone();
    } catch {
      // errors are transient; the list will refresh
    } finally {
      setBusy(false);
      setConfirming(null);
    }
  };

  const isStopped = agent.state === "stopped" || agent.state === "error";
  const isRunning = !isStopped;

  if (confirming === "delete") {
    return (
      <span
        className="inline-flex items-center gap-1"
        onClick={(e) => e.stopPropagation()}
      >
        <span className="text-xs text-mycel-error mr-1">Delete?</span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            act(() => api.deleteAgent(agent.name));
          }}
          disabled={busy}
          className="px-1.5 py-0.5 text-xs rounded bg-mycel-error/20 text-mycel-error hover:bg-mycel-error/30 disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
          aria-label={`Confirm delete agent ${agent.name}`}
        >
          {busy ? "..." : "Yes"}
        </button>
        <button
          onClick={(e) => {
            e.stopPropagation();
            setConfirming(null);
          }}
          aria-label="Cancel delete"
          className="px-1.5 py-0.5 text-xs rounded bg-mycel-border/50 text-mycel-muted hover:text-mycel-text focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          No
        </button>
      </span>
    );
  }

  return (
    <span
      className="inline-flex items-center gap-1"
      onClick={(e) => e.stopPropagation()}
    >
      {isStopped && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            act(() => api.startAgent(agent.name));
          }}
          disabled={busy}
          title="Start agent"
          aria-label={`Start agent ${agent.name}`}
          className="inline-flex items-center gap-1 px-2 py-1 text-[11px] font-medium rounded border border-emerald-500/30 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20 hover:border-emerald-500/50 disabled:opacity-50 transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          {busy ? "…" : "Start"}
        </button>
      )}
      {isRunning && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            act(() => api.stopAgent(agent.name));
          }}
          disabled={busy}
          title="Stop agent"
          aria-label={`Stop agent ${agent.name}`}
          className="inline-flex items-center gap-1 px-2 py-1 text-[11px] font-medium rounded border border-amber-500/30 bg-amber-500/10 text-amber-300 hover:bg-amber-500/20 hover:border-amber-500/50 disabled:opacity-50 transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
        >
          {busy ? "…" : "Stop"}
        </button>
      )}
      <button
        onClick={(e) => {
          e.stopPropagation();
          setConfirming("delete");
        }}
        title="Delete agent"
        aria-label={`Delete agent ${agent.name}`}
        className="inline-flex items-center gap-1 px-2 py-1 text-[11px] font-medium rounded border border-mycel-border/40 bg-transparent text-mycel-muted hover:border-rose-500/40 hover:bg-rose-500/10 hover:text-rose-300 transition-colors focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
      >
        Delete
      </button>
    </span>
  );
}

// --- Loading skeleton matching the agents table columns ---

function AgentsTableSkeleton() {
  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-mycel-border text-left">
          <th className="px-2 py-2 w-8"><div className="h-3 w-3 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2"><div className="h-3 w-16 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2 hidden sm:table-cell"><div className="h-3 w-14 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2 hidden sm:table-cell"><div className="h-3 w-14 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2"><div className="h-3 w-12 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2"><div className="h-3 w-10 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2 hidden md:table-cell"><div className="h-3 w-8 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2"><div className="h-3 w-14 rounded animate-pulse bg-mycel-border/40" /></th>
          <th className="px-4 py-2 w-10" />
        </tr>
      </thead>
      <tbody>
        {Array.from({ length: 5 }).map((_, i) => (
          <tr key={i} className="border-b border-mycel-border/50">
            <td className="px-2 py-3"><div className="h-3 w-3 rounded animate-pulse bg-mycel-border/30" /></td>
            <td className="px-4 py-3">
              <div className="flex items-center gap-2">
                <div className="h-7 w-7 rounded-full animate-pulse bg-mycel-border/30 shrink-0" />
                <div className="h-3 rounded animate-pulse bg-mycel-border/30" style={{ width: `${60 + (i % 4) * 15}px` }} />
              </div>
            </td>
            <td className="px-4 py-3 hidden sm:table-cell"><div className="h-3 w-12 rounded animate-pulse bg-mycel-border/30" /></td>
            <td className="px-4 py-3 hidden sm:table-cell"><div className="h-3 w-14 rounded animate-pulse bg-mycel-border/30" /></td>
            <td className="px-4 py-3"><div className="h-4 w-16 rounded-full animate-pulse bg-mycel-border/30" /></td>
            <td className="px-4 py-3"><div className="h-3 rounded animate-pulse bg-mycel-border/30" style={{ width: `${80 + (i % 3) * 30}px` }} /></td>
            <td className="px-4 py-3 hidden md:table-cell"><div className="h-4 w-10 rounded animate-pulse bg-mycel-border/30" /></td>
            <td className="px-4 py-3"><div className="h-4 w-20 rounded animate-pulse bg-mycel-border/30" /></td>
            <td className="px-4 py-3" />
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// --- Main Agents View ---

export function Agents() {
  const fetcher = useCallback(async () => {
    const res = await api.listAgents();
    return res;
  }, []);
  const {
    data: agents,
    loading,
    error,
    refresh,
    timedOut,
  } = usePolling(fetcher, 5000);
  const { subscribe } = useWebSocket();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [peekAgent, setPeekAgent] = useState<string | null>(null);
  const [stoppingAll, setStoppingAll] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  // "Group by repo" — inserts a section header row every time the
  // sorted list crosses a `repo` boundary. Default ON: the repo is a
  // property on every agent, so grouping by it is the primary way users
  // navigate a mixed fleet. No-ops when every visible agent shares one
  // repo (the common single-repo case). Persist across reloads.
  const [groupByRepo, setGroupByRepo] = useState<boolean>(() => {
    try {
      const v = localStorage.getItem("mycel-agents-group-by-repo");
      return v === null ? true : v === "1";
    } catch { return true; }
  });
  const toggleGroupByRepo = useCallback(() => {
    setGroupByRepo(prev => {
      const next = !prev;
      try { localStorage.setItem("mycel-agents-group-by-repo", next ? "1" : "0"); } catch { /* ignore */ }
      return next;
    });
  }, []);

  // Header slot: title + "Create agent" action
  useHeaderSlot({
    title: <TabHeaderTitle>Agents</TabHeaderTitle>,
    actions: (
      <>
        <span className="text-[10px] text-mycel-muted tabular-nums" style={{ fontFamily: MONO }}>
          {agents ? `${String(agents.length)} total` : "\u2014"}
        </span>
        <button
          type="button"
          onClick={() => setCreateOpen(true)}
          className="px-3 py-1 rounded text-[11px] font-medium border border-mycel-accent/40 bg-mycel-accent/10 text-mycel-accent hover:bg-mycel-accent/20 transition-colors"
          style={{ fontFamily: MONO }}
        >
          + New agent
        </button>
      </>
    ),
  });

  // Search + filter + bulk state (URL-synced where useful)
  const [search, setSearch] = useState(searchParams.get("q") ?? "");
  const roleFilter = searchParams.get("role") ?? "";
  const stateFilter = searchParams.get("state") ?? "";
  const toolFilter = searchParams.get("tool") ?? "";
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkBusy, setBulkBusy] = useState(false);
  const [bulkError, setBulkError] = useState<string | null>(null);
  const [focusIndex, setFocusIndex] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const updateFilter = (key: "role" | "state" | "tool", value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    setSearchParams(next, { replace: true });
  };

  // Debounced search → URL sync
  useEffect(() => {
    const t = setTimeout(() => {
      const next = new URLSearchParams(searchParams);
      if (search) next.set("q", search);
      else next.delete("q");
      setSearchParams(next, { replace: true });
    }, 250);
    return () => { clearTimeout(t); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search]);

  // Keyboard shortcuts now live after displayRows is declared below —
  // see the useEffect labelled "Global keyboard shortcuts".

  const handleStopAll = async () => {
    if (typeof window !== "undefined" && !window.confirm("Stop all running agents? This cannot be undone.")) {
      return;
    }
    setStoppingAll(true);
    try {
      await api.stopAllAgents();
      refresh();
    } catch {
      // transient error; list will refresh
    } finally {
      setStoppingAll(false);
    }
  };

  // Live state/task overrides from SSE — the same agent.state_changed
  // stream the detail page consumes. The 5s poll can lag a transition by
  // several seconds, which made the table contradict the detail view
  // ("idle" here, "working" there). Events are applied to rows
  // immediately; a poll whose row is at least as fresh wins.
  const [liveStates, setLiveStates] = useState<
    Map<string, { state: string; task?: string; at: number }>
  >(new Map());

  // Refresh on agent lifecycle events via SSE
  useEffect(() => {
    const unsubs = [
      subscribe("agent.state_changed", (ev) => {
        const d = ev.data;
        const name = (d.name ?? d.agent) as string | undefined;
        const state = d.state as string | undefined;
        if (name && state) {
          setLiveStates((prev) => {
            const next = new Map(prev);
            next.set(name, {
              state,
              task: typeof d.task === "string" ? d.task : undefined,
              at: Date.now(),
            });
            return next;
          });
        }
        void refresh();
      }),
      subscribe("agent.created", () => void refresh()),
      subscribe("agent.stopped", () => void refresh()),
      subscribe("agent.deleted", () => void refresh()),
    ];
    return () => unsubs.forEach((fn) => fn());
  }, [subscribe, refresh]);

  const handlePeekToggle = (agentName: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setPeekAgent((prev) => (prev === agentName ? null : agentName));
  };

  const columns = [
    "Select",
    "Name",
    "Runtime",
    "Provider",
    "Status",
    "Task",
    "MCP",
    "Actions",
    "",
  ] as const;

  // Compute filter options from agent list.
  // Merge live SSE overrides over polled rows: an override applies only
  // while it is newer than the row's server-side updated_at (which moves
  // on every state transition), so a fresh poll naturally supersedes it.
  const allAgents = useMemo(() => {
    const base = agents ?? [];
    if (liveStates.size === 0) return base;
    return base.map((a) => {
      const live = liveStates.get(a.name);
      if (!live) return a;
      const fetchedAt = a.updated_at ? Date.parse(a.updated_at) : 0;
      if (fetchedAt >= live.at) return a;
      return {
        ...a,
        state: live.state,
        task: live.task !== undefined ? live.task : a.task,
      };
    });
  }, [agents, liveStates]);
  const runningCount = useMemo(
    () => allAgents.filter((a) => a.state !== "stopped" && a.state !== "error").length,
    [allAgents],
  );
  const { availableStates, availableTools } = useMemo(() => {
    const s = new Set<string>();
    const t = new Set<string>();
    for (const a of allAgents) {
      if (a.state) s.add(a.state);
      if (a.tool) t.add(a.tool);
    }
    return {
      availableStates: Array.from(s).sort((x, y) => x.localeCompare(y)),
      availableTools: Array.from(t).sort((x, y) => x.localeCompare(y)),
    };
  }, [allAgents]);

  // Apply filters + search
  const filteredAgents = useMemo(() => {
    const q = search.trim().toLowerCase();
    return allAgents.filter((a) => {
      if (q && !a.name.toLowerCase().includes(q) && !(a.task ?? "").toLowerCase().includes(q)) {
        return false;
      }
      if (roleFilter && a.role !== roleFilter) return false;
      if (stateFilter && a.state !== stateFilter) return false;
      if (toolFilter && a.tool !== toolFilter) return false;
      return true;
    });
  }, [allAgents, search, roleFilter, stateFilter, toolFilter]);

  // Sort: working first, then idle, then stopped/error.
  // When `groupByRepo` is on we sort primarily by `repo` so the table
  // can insert a section header at each boundary.
  const displayRows = useMemo(() => {
    const rank = (s: string) => (s === "working" || s === "starting" ? 0 : s === "idle" ? 1 : 2);
    return [...filteredAgents].sort((a, b) => {
      if (groupByRepo) {
        const ra = a.repo ?? "";
        const rb = b.repo ?? "";
        const cmp = ra.localeCompare(rb);
        if (cmp !== 0) return cmp;
      }
      return rank(a.state) - rank(b.state) || a.name.localeCompare(b.name);
    });
  }, [filteredAgents, groupByRepo]);

  // How many distinct repos are in view — the toggle collapses to a
  // read-only info line when there's only one.
  const distinctRepoCount = useMemo(() => {
    return new Set(displayRows.map(a => a.repo ?? "").filter(Boolean)).size;
  }, [displayRows]);

  // Clamp focusIndex when displayRows shrinks (e.g. after filtering).
  useEffect(() => {
    if (focusIndex >= displayRows.length && displayRows.length > 0) {
      setFocusIndex(displayRows.length - 1);
    }
  }, [displayRows.length, focusIndex]);

  // Global keyboard shortcuts. These work when focus is anywhere on the page,
  // except inside inputs/textareas/contenteditable elements.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const isInput =
        target != null &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable);

      // "/" always focuses search even from inputs? no — only outside.
      if (e.key === "/" && !isInput) {
        e.preventDefault();
        searchInputRef.current?.focus();
        return;
      }
      if (e.key === "Escape" && selected.size > 0) {
        setSelected(new Set());
        return;
      }
      if (isInput) return;

      // Row navigation
      if (e.key === "j" || e.key === "ArrowDown") {
        e.preventDefault();
        setFocusIndex((i) => Math.min(i + 1, Math.max(0, displayRows.length - 1)));
        return;
      }
      if (e.key === "k" || e.key === "ArrowUp") {
        e.preventDefault();
        setFocusIndex((i) => Math.max(i - 1, 0));
        return;
      }
      // Enter opens the focused agent
      if (e.key === "Enter") {
        const row = displayRows[focusIndex];
        if (row) {
          e.preventDefault();
          navigate(`/agents/${encodeURIComponent(row.name)}`);
        }
        return;
      }
      // Space toggles peek for the focused row
      if (e.key === " ") {
        const row = displayRows[focusIndex];
        if (row) {
          e.preventDefault();
          setPeekAgent((prev) => (prev === row.name ? null : row.name));
        }
        return;
      }
      // x toggles selection on the focused row
      if (e.key === "x") {
        const row = displayRows[focusIndex];
        if (row) {
          e.preventDefault();
          setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(row.name)) next.delete(row.name);
            else next.add(row.name);
            return next;
          });
        }
        return;
      }
      // a selects all visible
      if (e.key === "a") {
        e.preventDefault();
        setSelected((prev) => {
          const next = new Set(prev);
          const names = displayRows.map((r) => r.name);
          const allSel = names.every((n) => next.has(n));
          if (allSel) {
            for (const n of names) next.delete(n);
          } else {
            for (const n of names) next.add(n);
          }
          return next;
        });
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => { window.removeEventListener("keydown", onKeyDown); };
  }, [selected.size, displayRows, focusIndex, navigate]);

  // Bulk action helpers
  const visibleNames = filteredAgents.map((a) => a.name);
  const allVisibleSelected = visibleNames.length > 0 && visibleNames.every((n) => selected.has(n));
  const toggleAllVisible = () => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const n of visibleNames) next.delete(n);
      } else {
        for (const n of visibleNames) next.add(n);
      }
      return next;
    });
  };
  const toggleOne = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };
  const summarizeResults = (results: BulkResult[]): string | null => {
    const failed = results.filter((r) => r.status === "error");
    if (failed.length === 0) return null;
    return `${String(failed.length)}/${String(results.length)} failed: ${failed.slice(0, 3).map((f) => `${f.agent} (${f.error ?? "error"})`).join(", ")}`;
  };
  const runBulk = async (fn: () => Promise<{ results: BulkResult[] }>) => {
    setBulkBusy(true);
    setBulkError(null);
    try {
      const { results } = await fn();
      const err = summarizeResults(results);
      if (err) setBulkError(err);
      refresh();
    } catch (e) {
      setBulkError(e instanceof Error ? e.message : "Bulk operation failed");
    } finally {
      setBulkBusy(false);
    }
  };
  const handleBulkStart = () => runBulk(() => api.bulkStartAgents(Array.from(selected)));
  const handleBulkStop = () => runBulk(() => api.bulkStopAgents(Array.from(selected)));
  const handleBulkDelete = () => {
    if (!window.confirm(`Delete ${String(selected.size)} agent(s)? This cannot be undone.`)) return;
    void runBulk(() => api.bulkDeleteAgents(Array.from(selected), false)).then(() => {
      setSelected(new Set());
    });
  };
  const handleBulkMessage = () => {
    const msg = window.prompt(`Send message to ${String(selected.size)} agent(s):`);
    if (msg == null || msg.trim() === "") return;
    void runBulk(() => api.bulkMessageAgents(Array.from(selected), msg.trim()));
  };
  const clearSelection = () => { setSelected(new Set()); };
  const clearFilters = () => {
    setSearch("");
    setSearchParams(new URLSearchParams(), { replace: true });
  };
  const hasFilters = search !== "" || roleFilter !== "" || stateFilter !== "" || toolFilter !== "";

  if (timedOut && !agents) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Agents took too long to load"
          description="The server may be unavailable. Check your connection and try again."
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }
  if (error && !agents) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load agents"
          description={error}
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4 pb-24">
      {/* Sub-toolbar: count summary + Stop All (title + Create live in the top-bar chip) */}
      {allAgents.length > 0 && (
        <div className="flex items-center justify-end gap-3">
          <span className="text-sm text-mycel-muted">
            {hasFilters
              ? `${String(filteredAgents.length)} of ${String(allAgents.length)} agents`
              : `${String(runningCount)} active`}
          </span>
          {allAgents.some(
            (a) => a.state !== "stopped" && a.state !== "error",
          ) && (
            <button
              type="button"
              onClick={handleStopAll}
              disabled={stoppingAll}
              className="px-3 py-1.5 text-xs font-medium rounded border border-mycel-error/40 bg-mycel-error/10 text-mycel-error hover:bg-mycel-error/20 hover:border-mycel-error/60 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mycel-error focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
              aria-label="Stop all agents"
            >
              {stoppingAll ? "Stopping..." : "Stop All"}
            </button>
          )}
        </div>
      )}

      {/* Search + filter toolbar */}
      {allAgents.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative flex-1 min-w-[200px]">
            <input
              ref={searchInputRef}
              type="text"
              value={search}
              onChange={(e) => { setSearch(e.target.value); }}
              placeholder="Search by name or task...  (press / to focus)"
              className="w-full px-3 py-1.5 text-sm rounded border border-mycel-border bg-mycel-bg text-mycel-text placeholder:text-mycel-muted/60 focus:outline-none focus:ring-1 focus:ring-mycel-accent"
              aria-label="Search agents"
            />
          </div>
          <select
            value={stateFilter}
            onChange={(e) => { updateFilter("state", e.target.value); }}
            className="px-2 py-1.5 text-sm rounded border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent"
            aria-label="Filter by state"
          >
            <option value="">All states</option>
            {availableStates.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <select
            value={toolFilter}
            onChange={(e) => { updateFilter("tool", e.target.value); }}
            className="px-2 py-1.5 text-sm rounded border border-mycel-border bg-mycel-bg text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent"
            aria-label="Filter by tool"
          >
            <option value="">All tools</option>
            {availableTools.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="px-2 py-1.5 text-xs text-mycel-muted hover:text-mycel-text border border-mycel-border rounded focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
              aria-label="Clear filters"
            >
              Clear
            </button>
          )}
          {/* Group-by-repo toggle. Default ON. Disabled + hidden
              when every visible agent shares one repo. */}
          <button
            type="button"
            onClick={toggleGroupByRepo}
            disabled={distinctRepoCount <= 1}
            aria-pressed={groupByRepo}
            className={`px-2 py-1.5 text-xs rounded border transition-colors ${
              groupByRepo && distinctRepoCount > 1
                ? "border-mycel-accent text-mycel-accent"
                : "border-mycel-border text-mycel-muted hover:text-mycel-text disabled:opacity-50 disabled:cursor-not-allowed"
            }`}
            title={distinctRepoCount <= 1 ? "All agents share one repo — nothing to group" : `Group by repo (${String(distinctRepoCount)} repos in view)`}
          >
            Group by repo
          </button>
        </div>
      )}

      {/* Keyboard hints removed — shortcuts still work (/, j/k, Enter, space, x, a, Esc) */}

      <div className="rounded border border-mycel-border overflow-x-auto">
        {loading && !agents ? (
          <AgentsTableSkeleton />
        ) : allAgents.length === 0 ? (
          <EmptyState
            icon=">"
            title="No agents yet"
            description="Create your first agent using the + Create Agent button."
          />
        ) : filteredAgents.length === 0 ? (
          <EmptyState
            icon=">"
            title="No agents match your filters"
            description="Try adjusting your search or clearing the filters."
            actionLabel="Clear filters"
            onAction={clearFilters}
          />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-mycel-border text-left">
                <th className="px-2 py-2 font-medium text-mycel-muted w-8">
                  <input
                    type="checkbox"
                    checked={allVisibleSelected}
                    onChange={toggleAllVisible}
                    className="cursor-pointer accent-mycel-accent"
                    aria-label="Select all visible agents"
                  />
                </th>
                <th className="px-4 py-2 font-medium text-mycel-muted">Name</th>
                <th className="px-4 py-2 font-medium text-mycel-muted hidden sm:table-cell">
                  Runtime
                </th>
                <th className="px-4 py-2 font-medium text-mycel-muted hidden sm:table-cell">
                  Provider
                </th>
                <th className="px-4 py-2 font-medium text-mycel-muted">Status</th>
                <th className="px-4 py-2 font-medium text-mycel-muted">Task</th>
                <th
                  className="px-4 py-2 font-medium text-mycel-muted hidden md:table-cell"
                  title="MCP server configuration"
                >
                  MCP
                </th>
                <th className="px-4 py-2 font-medium text-mycel-muted">Actions</th>
                <th className="px-4 py-2 font-medium text-mycel-muted w-10"></th>
              </tr>
            </thead>
            <tbody>
              {displayRows.map((a, rowIdx) => (
                <Fragment key={a.name}>
                  {/* Repo section header — rendered whenever the sorted
                      list crosses a repo boundary and grouping is
                      enabled. Shows the repo basename; the full path
                      lives in the tooltip. */}
                  {groupByRepo && distinctRepoCount > 1 &&
                    (rowIdx === 0 || (displayRows[rowIdx - 1]!.repo ?? "") !== (a.repo ?? "")) && (
                    <tr>
                      <td
                        colSpan={columns.length}
                        className="px-4 pt-4 pb-1 text-[10px] uppercase tracking-[0.12em] text-mycel-muted font-medium"
                        title={a.repo ?? undefined}
                      >
                        {a.repo ? (a.repo.split("/").pop() ?? a.repo) : "(no repo)"}
                      </td>
                    </tr>
                  )}
                  {/* Subtle divider between active and stopped groups */}
                  {rowIdx > 0 &&
                    (a.state === "stopped" || a.state === "error") &&
                    displayRows[rowIdx - 1]!.state !== "stopped" &&
                    displayRows[rowIdx - 1]!.state !== "error" && (
                    <tr><td colSpan={columns.length} className="h-px bg-mycel-border/40" /></tr>
                  )}
                  <tr
                    onClick={() =>
                      navigate(`/agents/${encodeURIComponent(a.name)}`)
                    }
                    onKeyDown={(e) => {
                      if (e.key === "Enter") navigate(`/agents/${encodeURIComponent(a.name)}`);
                    }}
                    role="link"
                    tabIndex={0}
                    className={`border-b border-mycel-border/50 cursor-pointer transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg ${
                      rowIdx === focusIndex ? "ring-1 ring-inset ring-mycel-accent/40 " : ""
                    }${
                      peekAgent === a.name ? "bg-mycel-accent/5 " : ""
                    }${
                      selected.has(a.name) ? "bg-mycel-accent/10 hover:bg-mycel-accent/15" : "hover:bg-mycel-surface"
                    }`}
                    style={(a.state === "stopped" || a.state === "error") ? { opacity: 0.55 } : undefined}
                  >
                    <td
                      className="px-2 py-2"
                      onClick={(e) => { e.stopPropagation(); }}
                    >
                      <input
                        type="checkbox"
                        checked={selected.has(a.name)}
                        onChange={() => { toggleOne(a.name); }}
                        className="cursor-pointer accent-mycel-accent"
                        aria-label={`Select agent ${a.name}`}
                      />
                    </td>
                    <td className="px-4 py-2">
                      <span className="inline-flex items-center gap-2">
                        <AgentIcon state={a.state} size={28} tool={a.tool} />
                        <InlineAgentName agent={a} onRenamed={refresh} />
                      </span>
                    </td>
                    <td className="px-4 py-1.5 hidden sm:table-cell">
                      <RuntimeChip runtime={a.runtime_backend} />
                    </td>
                    <td className="px-4 py-1.5 hidden sm:table-cell">
                      <ProviderChip tool={a.tool} />
                    </td>
                    <td className="px-4 py-2">
                      <StatusBadge status={a.state} />
                    </td>
                    <td className="px-4 py-2">
                      {/* Task = the agent's own report (report_status).
                          Lifecycle events live in the activity stream. */}
                      <span className="text-mycel-muted" title={a.task}>
                        {a.task ? truncate(a.task, 50) : "—"}
                      </span>
                    </td>
                    <td className="px-4 py-1.5 hidden md:table-cell">
                      {(() => {
                        const servers = a.mcp_servers ?? [];
                        // Every agent has the built-in "mycel" (formerly
                        // "bc") MCP server. Showing it on every row was
                        // pure visual noise. Only surface EXTRA servers
                        // beyond the default; render "\u2014" otherwise.
                        const extras = servers
                          .map((s) => s.replace(/^mcp__/, ""))
                          .filter((s) => s !== "bc" && s !== "mycel");
                        if (extras.length === 0) {
                          return <span className="text-mycel-muted text-[11px]">{"\u2014"}</span>;
                        }
                        const fullList = extras.join(", ");
                        if (extras.length <= 2) {
                          return (
                            <div className="flex flex-wrap gap-1" title={fullList}>
                              {extras.map((s) => (
                                <span key={s} className="text-[10px] px-1.5 py-0.5 rounded bg-mycel-accent/10 text-mycel-accent font-medium">
                                  {s}
                                </span>
                              ))}
                            </div>
                          );
                        }
                        const rest = extras.slice(1).join(", ");
                        return (
                          <div className="flex flex-wrap gap-1" title={fullList}>
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-mycel-accent/10 text-mycel-accent font-medium">
                              {extras[0]}
                            </span>
                            <span
                              className="text-[10px] px-1.5 py-0.5 rounded border border-mycel-border text-mycel-muted cursor-help"
                              title={rest}
                            >
                              +{String(extras.length - 1)}
                            </span>
                          </div>
                        );
                      })()}
                    </td>
                    <td className="px-4 py-2">
                      <AgentActions agent={a} onDone={refresh} />
                    </td>
                    <td className="px-4 py-2 text-center">
                      <button
                        onClick={(e) => handlePeekToggle(a.name, e)}
                        className={`inline-flex items-center justify-center w-7 h-7 rounded transition-colors focus:ring-2 focus:ring-mycel-accent focus:outline-none ${
                          peekAgent === a.name
                            ? "bg-mycel-accent/20 text-mycel-accent"
                            : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface"
                        }`}
                        title={
                          peekAgent === a.name ? "Hide output" : "Peek output"
                        }
                        aria-label={
                          peekAgent === a.name ? "Hide output" : "Peek output"
                        }
                      >
                        {peekAgent === a.name ? "\u2296" : "\u2295"}
                      </button>
                    </td>
                  </tr>
                  {peekAgent === a.name && (
                    <tr
                      key={`${a.name}-peek`}
                      className="border-b border-mycel-border/50"
                    >
                      <td colSpan={columns.length} className="p-0">
                        <InlineTerminal agentName={a.name} lines={10} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Bulk action bar */}
      {selected.size > 0 && (
        <div className="fixed left-0 right-0 bottom-0 z-40 border-t border-mycel-border bg-mycel-surface/95 backdrop-blur shadow-mycel-lg">
          <div className="max-w-6xl mx-auto px-6 py-3 flex items-center gap-3 flex-wrap">
            <span className="text-sm font-medium text-mycel-text">
              {selected.size} selected
            </span>
            {bulkError && (
              <span className="text-xs text-mycel-error truncate max-w-md" title={bulkError}>
                {bulkError}
              </span>
            )}
            <div className="flex items-center gap-2 ml-auto">
              <button
                onClick={handleBulkStart}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-mycel-success/20 text-mycel-success hover:bg-mycel-success/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
                aria-label="Start selected agents"
              >
                {bulkBusy ? "..." : "Start"}
              </button>
              <button
                onClick={handleBulkStop}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-mycel-warning/20 text-mycel-warning hover:bg-mycel-warning/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
                aria-label="Stop selected agents"
              >
                {bulkBusy ? "..." : "Stop"}
              </button>
              <button
                onClick={handleBulkMessage}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-mycel-accent/20 text-mycel-accent hover:bg-mycel-accent/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
                aria-label="Send message to selected agents"
              >
                {bulkBusy ? "..." : "Message"}
              </button>
              <button
                onClick={handleBulkDelete}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded bg-mycel-error/20 text-mycel-error hover:bg-mycel-error/30 disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
                aria-label="Delete selected agents"
              >
                {bulkBusy ? "..." : "Delete"}
              </button>
              <button
                onClick={clearSelection}
                disabled={bulkBusy}
                className="px-3 py-1.5 text-sm rounded border border-mycel-border text-mycel-muted hover:text-mycel-text disabled:opacity-50 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-mycel-accent"
                aria-label="Clear selection"
              >
                Clear
              </button>
            </div>
          </div>
        </div>
      )}

      <CreateAgentModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        existingNames={allAgents.map((a) => a.name)}
        existingAgents={allAgents}
      />
    </div>
  );
}
