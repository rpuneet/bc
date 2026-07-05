import { useCallback, useState } from "react";
import { useParams } from "react-router-dom";
import { api } from "../api/client";
import type { NotificationSource } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { AgentPeekPanel } from "../components/AgentPeekPanel";
import { GatewayFeed } from "../components/notifications/GatewayFeed";
import { NotificationsHome } from "../components/notifications/NotificationsHome";

/**
 * /notifications — the hub (NotificationsHome) when no channel is
 * selected; the existing channel feed when one is. The hub owns its own
 * data fetching, loading and empty states.
 */
export function Notifications() {
  // No page title in the header — the drawer names the section.
  // GatewayFeed contributes its channel breadcrumb when a channel is active.
  const { sourceName: paramSource } = useParams<{ sourceName: string }>();
  const selected = paramSource ?? null;
  const [peekAgent, setPeekAgent] = useState<string | null>(null);

  // Sources are only needed to hand the selected channel's metadata
  // (description/topic) to the feed; the hub fetches its own data.
  const fetcher = useCallback(() => api.listNotificationSources(), []);
  const { data: sources } = usePolling(fetcher, 10000);

  if (!selected) {
    return <NotificationsHome />;
  }

  const selectedSource = (sources ?? []).find((c: NotificationSource) => c.name === selected);

  return (
    <div className="flex h-full">
      <div className="flex-1 flex flex-col min-w-0">
        <GatewayFeed
          channelName={selected}
          channel={selectedSource}
          onPeekAgent={setPeekAgent}
        />
      </div>

      {peekAgent && (
        <AgentPeekPanel
          agentName={peekAgent}
          onClose={() => setPeekAgent(null)}
        />
      )}
    </div>
  );
}
