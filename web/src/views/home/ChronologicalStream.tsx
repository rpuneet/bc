/**
 * ChronologicalStream — Home's "as it comes" feed (#3642).
 *
 * Flattens tool/lifecycle nodes from every visible agent into one
 * newest-first timeline. Running rows pin to the top (same pattern as
 * AgentCard). Each row is attributed to its agent via a compact name
 * chip that opens the Home drill-down.
 */

import { useMemo } from "react";
import type { AgentActivity, FilterType, ToolNode } from "../../components/live/liveTypes";
import { flattenNodes, nodeMatchesSearch, partitionRunning } from "../../components/live/liveHelpers";
import { EventRow } from "../../components/live/EventRow";
import { StateDot } from "../../components/live/LiveRenderers";
import { EmptyState } from "../../components/EmptyState";

/** Cap on completed rows in the Home stream — full history lives on agent detail. */
export const HOME_STREAM_MAX_ROWS = 80;

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

function StreamAgentChip({
  name,
  state,
  onOpen,
}: {
  name: string;
  state: string;
  onOpen: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      title={`Open ${name} detail view`}
      className="shrink-0 inline-flex items-center gap-1.5 max-w-[7.5rem] px-2 py-0.5 rounded-md border border-mycel-border bg-mycel-bg text-[11px] font-medium text-mycel-text hover:border-mycel-accent hover:text-mycel-accent transition-colors"
    >
      <StateDot state={state} />
      <span className="truncate">{name}</span>
    </button>
  );
}

function AttributedEventRow({
  entry,
  searchTerm,
  onOpenAgent,
}: {
  entry: StreamEntry;
  searchTerm: string;
  onOpenAgent: (name: string) => void;
}) {
  return (
    <div
      className="flex items-stretch border-b border-mycel-border last:border-b-0"
      data-testid="home-stream-row"
      data-agent={entry.agentName}
    >
      <div className="flex items-center pl-2.5 pr-1 shrink-0">
        <StreamAgentChip
          name={entry.agentName}
          state={entry.agentState}
          onOpen={() => onOpenAgent(entry.agentName)}
        />
      </div>
      <div className="flex-1 min-w-0 [&_>div]:border-b-0">
        <EventRow node={entry.node} searchQuery={searchTerm} />
      </div>
    </div>
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
        <div className="bg-mycel-accent-subtle/30 border-b-2 border-mycel-accent-subtle">
          <div className="flex items-center gap-2 px-3 py-1 bg-mycel-surface/95 border-b border-mycel-border">
            <span className="relative flex h-2 w-2 shrink-0" aria-hidden>
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-mycel-accent opacity-60 motion-reduce:hidden" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-mycel-accent" />
            </span>
            <span className="text-[10px] uppercase tracking-wide font-semibold text-mycel-accent">
              Running
            </span>
            <span className="text-[10px] text-mycel-muted font-mono tabular-nums">
              {running.length}
            </span>
          </div>
          {running.map((entry) => (
            <AttributedEventRow
              key={`run-${entry.agentName}:${entry.node.id}`}
              entry={entry}
              searchTerm={searchTerm}
              onOpenAgent={onOpenAgent}
            />
          ))}
        </div>
      )}
      {shown.map((entry) => (
        <AttributedEventRow
          key={`${entry.agentName}:${entry.node.id}`}
          entry={entry}
          searchTerm={searchTerm}
          onOpenAgent={onOpenAgent}
        />
      ))}
      {hiddenCount > 0 && (
        <div className="px-3 py-2 text-[11px] text-mycel-muted font-mono text-center border-t border-mycel-border">
          {hiddenCount} older event{hiddenCount === 1 ? "" : "s"} hidden — open an agent for full history
        </div>
      )}
    </div>
  );
}
