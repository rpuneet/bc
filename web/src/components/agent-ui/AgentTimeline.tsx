import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../../api/client";
import type { AgentActivityItem } from "../../api/client";
import { EventRow } from "../live/EventRow";
import { activityItemToNode } from "../live/liveHelpers";
import type { ToolNode } from "../live/liveTypes";
import { EmptyState } from "../EmptyState";
import { formatAbsolute } from "../../utils/time";

/* ── AgentTimeline ────────────────────────────────────────────────────
   The durable, scrollable record of an agent's whole lifecycle — spawn,
   tasks, tool calls, state changes, stop — built entirely from the
   append-only events store via GET /api/agents/{name}/activity. Unlike
   the Live tab (ephemeral, current-session WebSocket stream) this is
   read-only history: it loads a page on mount and pages backwards in
   time with "load older", never re-fetching what's already on screen.

   Rows reuse the same EventRow/activityItemToNode path as Live so a tool
   call reads identically in both places — same title, same expandable
   input/output.
─────────────────────────────────────────────────────────────────── */

const PAGE_SIZE = 50;

interface TimelineRow {
  node: ToolNode;
  id: number;
  dayKey: string;
}

function toRow(item: AgentActivityItem): TimelineRow {
  return {
    node: activityItemToNode(item),
    id: item.id ?? 0,
    dayKey: formatAbsolute(item.timestamp, { dateOnly: true }),
  };
}

export function AgentTimeline({ agentName }: { agentName: string }) {
  const [rows, setRows] = useState<TimelineRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const cursorRef = useRef<number | undefined>(undefined);
  const requestIdRef = useRef(0);

  const loadInitial = useCallback(() => {
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setError(null);
    api.getAgentActivity(agentName, PAGE_SIZE)
      .then((items) => {
        if (requestIdRef.current !== requestId) return;
        const next = items.map(toRow);
        setRows(next);
        const oldest = items[items.length - 1];
        cursorRef.current = oldest?.id;
        setHasMore(items.length === PAGE_SIZE && oldest?.id !== undefined);
      })
      .catch(() => {
        if (requestIdRef.current !== requestId) return;
        setError("Couldn't load activity history");
      })
      .finally(() => {
        if (requestIdRef.current !== requestId) return;
        setLoading(false);
      });
  }, [agentName]);

  useEffect(() => {
    loadInitial();
  }, [loadInitial]);

  const loadOlder = useCallback(() => {
    if (loadingMore || !hasMore || cursorRef.current === undefined) return;
    setLoadingMore(true);
    api.getAgentActivity(agentName, PAGE_SIZE, cursorRef.current)
      .then((items) => {
        const next = items.map(toRow);
        setRows((prev) => [...prev, ...next]);
        const oldest = items[items.length - 1];
        cursorRef.current = oldest?.id;
        setHasMore(items.length === PAGE_SIZE && oldest?.id !== undefined);
      })
      .catch(() => {
        setError("Couldn't load older activity");
      })
      .finally(() => {
        setLoadingMore(false);
      });
  }, [agentName, hasMore, loadingMore]);

  if (loading) {
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <p className="text-sm text-mycel-muted italic">Loading activity…</p>
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center p-6">
        <EmptyState
          icon="~"
          title="No recorded activity yet"
          description="This agent's spawn, tasks, tool calls, and state changes will appear here as they happen — the durable record survives restarts and session boundaries."
        />
      </div>
    );
  }

  // Group consecutive rows by day so a long history reads as day
  // sections rather than one undifferentiated column of rows.
  let lastDayKey = "";

  return (
    <div className="flex flex-col h-full overflow-y-auto p-6 gap-0">
      <div className="rounded-lg border border-mycel-border overflow-hidden">
        {rows.map((row) => {
          const showDayHeader = row.dayKey !== lastDayKey;
          lastDayKey = row.dayKey;
          return (
            <div key={row.node.id}>
              {showDayHeader && (
                <div className="px-3 py-1.5 text-[11px] font-medium uppercase tracking-wide text-mycel-muted bg-mycel-surface border-b border-mycel-border">
                  {row.dayKey}
                </div>
              )}
              <EventRow node={row.node} />
            </div>
          );
        })}
      </div>

      <div className="flex flex-col items-center gap-2 py-4">
        {error && <p className="text-xs text-mycel-error">{error}</p>}
        {hasMore ? (
          <button
            type="button"
            onClick={loadOlder}
            disabled={loadingMore}
            className="h-8 px-3 inline-flex items-center rounded-md border border-mycel-border-strong text-[13px] text-mycel-text hover:bg-mycel-surface-hover transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loadingMore ? "Loading…" : "Load older"}
          </button>
        ) : (
          <p className="text-xs text-mycel-muted">Beginning of recorded history</p>
        )}
      </div>
    </div>
  );
}
