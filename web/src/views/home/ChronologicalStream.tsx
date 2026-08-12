/**
 * ChronologicalStream — Home's "as it comes" feed (#3642).
 *
 * Flattens tool/lifecycle nodes from every visible agent into one
 * newest-first timeline. Running rows pin to the top (same pattern as
 * AgentCard). Attribution is an avatar-only AgentChip (hover card like
 * the agents dropdown) so the stream stays dense when one agent dominates.
 */

import { useMemo } from "react";
import type { AgentActivity, FilterType, ToolNode } from "../../components/live/liveTypes";
import { flattenNodes, nodeMatchesSearch, partitionRunning } from "../../components/live/liveHelpers";
import { EventRow } from "../../components/live/EventRow";
import { EmptyState } from "../../components/EmptyState";
import { AgentChip } from "../../components/agent-ui";

/** Cap on completed rows in the Home stream — full history lives on agent detail. */
export const HOME_STREAM_MAX_ROWS = 80;

/** Fixed column width for the avatar rail — keeps event text aligned. */
const AVATAR_COL = "w-9";

export interface StreamEntry {
  agentName: string;
  agentState: string;
  node: ToolNode;
}

function buildEntries(
  agents: AgentActivity[],
  searchTerm: string,
  typeFilter: FilterType,
): StreamEntry[] {
  const out: StreamEntry[] = [];
  for (const a of agents) {
    if (typeFilter === "state") {
      out.push({
        agentName: a.name,
        agentState: a.state,
        node: {
          id: `state:${a.name}`,
          toolName: "Stop",
          args: a.task || a.state,
          fullInput: null,
          fullOutput: null,
          status: "completed",
          startTime: a.lastEventTime || 0,
          children: [],
        },
      });
      continue;
    }
    for (const node of flattenNodes(a.nodes)) {
      if (searchTerm && !nodeMatchesSearch(node, searchTerm)) continue;
      out.push({ agentName: a.name, agentState: a.state, node });
    }
  }
  return out.sort((x, y) => y.node.startTime - x.node.startTime);
}

function AttributedEventRow({
  entry,
  searchTerm,
  onOpenAgent,
  showAvatar,
}: {
  entry: StreamEntry;
  searchTerm: string;
  onOpenAgent: (name: string) => void;
  /** False when the previous row is the same agent — keeps a quiet rail. */
  showAvatar: boolean;
}) {
  return (
    <div
      className="flex items-stretch border-b border-mycel-border/70 last:border-b-0 hover:bg-mycel-surface-hover/40 transition-colors"
      data-testid="home-stream-row"
      data-agent={entry.agentName}
    >
      <div className={`flex items-center justify-center shrink-0 ${AVATAR_COL} py-1`}>
        {showAvatar ? (
          <AgentChip
            name={entry.agentName}
            state={entry.agentState}
            size={22}
            showName={false}
            showDot={false}
            preview
            onClick={() => onOpenAgent(entry.agentName)}
            className="rounded-md p-0.5 hover:bg-mycel-accent-subtle/50 transition-colors"
          />
        ) : (
          <span
            className="block w-0.5 h-3 rounded-full bg-mycel-border"
            aria-hidden
            title={entry.agentName}
          />
        )}
      </div>
      <div className="flex-1 min-w-0 [&_>div]:border-b-0 [&_button]:py-1 [&_button]:pl-1.5 [&_button]:pr-2.5 [&_button]:gap-2">
        <EventRow node={entry.node} searchQuery={searchTerm} />
      </div>
    </div>
  );
}

function StreamBlock({
  entries,
  searchTerm,
  onOpenAgent,
}: {
  entries: StreamEntry[];
  searchTerm: string;
  onOpenAgent: (name: string) => void;
}) {
  return (
    <>
      {entries.map((entry, i) => (
        <AttributedEventRow
          key={`${entry.agentName}:${entry.node.id}`}
          entry={entry}
          searchTerm={searchTerm}
          onOpenAgent={onOpenAgent}
          showAvatar={i === 0 || entries[i - 1]!.agentName !== entry.agentName}
        />
      ))}
    </>
  );
}

export function ChronologicalStream({
  agents,
  searchTerm,
  typeFilter,
  onOpenAgent,
  emptyTitle,
  emptyDescription,
}: {
  agents: AgentActivity[];
  searchTerm: string;
  typeFilter: FilterType;
  onOpenAgent: (name: string) => void;
  emptyTitle: string;
  emptyDescription: string;
}) {
  const entries = useMemo(
    () => buildEntries(agents, searchTerm, typeFilter),
    [agents, searchTerm, typeFilter],
  );

  const { running, rest } = useMemo(() => {
    const split = partitionRunning(entries.map((e) => e.node));
    const runningIds = new Set(split.running.map((n) => n.id));
    return {
      running: entries.filter((e) => runningIds.has(e.node.id)),
      rest: entries.filter((e) => !runningIds.has(e.node.id)),
    };
  }, [entries]);

  const shown = rest.slice(0, HOME_STREAM_MAX_ROWS);
  const hiddenCount = rest.length - shown.length;

  if (entries.length === 0) {
    return (
      <div className="p-3" data-testid="home-chronological-stream" data-empty="true">
        <EmptyState icon=">" title={emptyTitle} description={emptyDescription} />
      </div>
    );
  }

  return (
    <div data-testid="home-chronological-stream" className="flex flex-col min-h-0">
      {running.length > 0 && (
        <div className="bg-mycel-accent-subtle/25 border-b border-mycel-accent-subtle/60">
          <div className="flex items-center gap-2 px-2.5 py-1 bg-mycel-surface/90 border-b border-mycel-border/80">
            <span className="relative flex h-1.5 w-1.5 shrink-0" aria-hidden>
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-mycel-accent opacity-60 motion-reduce:hidden" />
              <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-mycel-accent" />
            </span>
            <span className="text-[10px] uppercase tracking-wide font-semibold text-mycel-accent">
              Running
            </span>
            <span className="text-[10px] text-mycel-muted font-mono tabular-nums">
              {running.length}
            </span>
          </div>
          <StreamBlock entries={running} searchTerm={searchTerm} onOpenAgent={onOpenAgent} />
        </div>
      )}
      <StreamBlock entries={shown} searchTerm={searchTerm} onOpenAgent={onOpenAgent} />
      {hiddenCount > 0 && (
        <div className="px-3 py-1.5 text-[11px] text-mycel-muted font-mono text-center border-t border-mycel-border">
          {hiddenCount} older event{hiddenCount === 1 ? "" : "s"} hidden — open an agent for full history
        </div>
      )}
    </div>
  );
}
