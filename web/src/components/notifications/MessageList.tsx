import { useMemo, useCallback, useRef } from "react";
import { Virtuoso } from "react-virtuoso";
import type { VirtuosoHandle } from "react-virtuoso";
import type { ChannelMessage } from "../../api/client";
import { MessageContent } from "../MessageContent";
import { RoleBadge } from "./RoleBadge";
import {
  groupMessages,
  formatRelativeTime,
  formatDayLabel,
  dateKey,
} from "./messageUtils";
import { LiveAgentCharacter } from "../agent-ui";
import type { MessageGroup } from "./messageUtils";
import { EmptyState } from "../EmptyState";

type ListItem =
  | { type: "separator"; date: string; label: string }
  | { type: "group"; group: MessageGroup };

export function MessageList({
  messages,
  channelName,
  agentRoles,
  onPeekAgent,
  atBottomChange,
  onLoadMore,
  hasMore,
  loadingMore,
}: {
  messages: ChannelMessage[];
  channelName: string;
  agentRoles: Record<string, string>;
  onPeekAgent: (name: string) => void;
  atBottomChange?: (atBottom: boolean) => void;
  onLoadMore?: () => void;
  hasMore?: boolean;
  loadingMore?: boolean;
}) {
  const virtuosoRef = useRef<VirtuosoHandle>(null);

  const items = useMemo(() => {
    const groups = groupMessages(messages);
    const result: ListItem[] = [];
    let lastDate = "";
    for (const group of groups) {
      const day = dateKey(group.timestamp);
      if (day !== lastDate) {
        result.push({
          type: "separator",
          date: day,
          label: formatDayLabel(group.timestamp),
        });
        lastDate = day;
      }
      result.push({ type: "group", group });
    }
    return result;
  }, [messages]);

  const handleStartReached = useCallback(() => {
    if (onLoadMore && hasMore && !loadingMore) {
      onLoadMore();
    }
  }, [onLoadMore, hasMore, loadingMore]);

  const renderItem = useCallback(
    (_index: number, item: ListItem) => {
      if (item.type === "separator") {
        return (
          <div className="flex items-center gap-3 py-3" role="separator">
            <div className="flex-1 h-px bg-mycel-border" />
            <time className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
              {item.label}
            </time>
            <div className="flex-1 h-px bg-mycel-border" />
          </div>
        );
      }

      const { group } = item;
      const firstMsg = group.messages[0];
      if (!firstMsg) return null;
      const role = agentRoles[group.sender];

      return (
        <div className="flex gap-3 py-2.5 px-1 hover:bg-mycel-surface rounded-md transition-colors" role="listitem">
          {/* Sender character — deterministic identity from the name */}
          <span className="shrink-0" style={{ marginTop: 2 }}>
            <LiveAgentCharacter name={group.sender} size={30} />
          </span>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2">
              <button
                type="button"
                onClick={() => onPeekAgent(group.sender)}
                className="font-medium text-sm text-mycel-text hover:underline cursor-pointer decoration-1 underline-offset-2 focus-visible:ring-1 focus-visible:ring-mycel-accent rounded-md"
                title={`Peek at ${group.sender}'s terminal`}
              >
                {group.sender}
              </button>
              <RoleBadge role={role} />
              <time className="text-xs text-mycel-muted tabular-nums" title={new Date(group.timestamp).toLocaleString()}>
                {formatRelativeTime(group.timestamp)}
              </time>
            </div>
            {group.messages.map((msg) => (
              <p
                key={msg.id}
                className="mt-0.5 text-sm whitespace-pre-wrap break-words text-mycel-text-2 leading-[1.65]"
              >
                <MessageContent content={msg.content} />
              </p>
            ))}
          </div>
        </div>
      );
    },
    [agentRoles, onPeekAgent],
  );

  if (messages.length === 0) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <EmptyState
          icon="..."
          title="No messages yet"
          description={`Be the first to send a message in #${channelName}.`}
        />
      </div>
    );
  }

  return (
    <Virtuoso
      ref={virtuosoRef}
      data={items}
      itemContent={renderItem}
      followOutput="smooth"
      initialTopMostItemIndex={items.length > 0 ? items.length - 1 : 0}
      atBottomStateChange={atBottomChange}
      startReached={handleStartReached}
      className="flex-1"
      style={{ height: "100%" }}
      increaseViewportBy={200}
      components={{
        Header: () =>
          loadingMore ? (
            <div className="flex justify-center py-3">
              <span className="text-xs text-mycel-muted animate-pulse">
                Loading older messages...
              </span>
            </div>
          ) : hasMore === false ? (
            <div className="flex justify-center py-3">
              <span className="text-xs text-mycel-muted">
                Beginning of conversation
              </span>
            </div>
          ) : null,
      }}
    />
  );
}
