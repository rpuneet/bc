import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { NotificationSource } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { AgentPeekPanel } from "../components/AgentPeekPanel";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { GatewayFeed } from "../components/notifications/GatewayFeed";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
export function Notifications() {
  // Set a default header; GatewayFeed overrides this when a channel is active.
  // useMemo ensures stable references to avoid infinite re-render loops.
  const defaultTitle = useMemo(() => <TabHeaderTitle>Notifications</TabHeaderTitle>, []);
  useHeaderSlot({ title: defaultTitle });

  const { sourceName: paramSource } = useParams<{ sourceName: string }>();
  const navigate = useNavigate();

  const fetcher = useCallback(() => api.listNotificationSources(), []);
  const { data: sources, loading, error, refresh, timedOut } = usePolling(fetcher, 10000);

  const selected = paramSource ?? null;
  const [peekAgent, setPeekAgent] = useState<string | null>(null);

  useEffect(() => {
    if (!selected && sources && sources.length > 0) {
      const gwSource = sources.find((c) => c.name.includes(":"));
      if (gwSource) {
        navigate("/notifications/" + gwSource.name, { replace: true });
      }
    }
  }, [selected, sources, navigate]);

  if (loading && !sources) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-6 w-28 animate-pulse rounded bg-mycel-border/50" />
        <LoadingSkeleton variant="text" rows={5} />
      </div>
    );
  }

  if (timedOut && !sources) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Notifications took too long to load"
          description="The server may be unavailable."
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  if (error && !sources) {
    return (
      <div className="p-6">
        <EmptyState
          icon="!"
          title="Failed to load notifications"
          description={error}
          actionLabel="Retry"
          onAction={refresh}
        />
      </div>
    );
  }

  const sourceList = sources ?? [];
  const selectedSource = sourceList.find((c: NotificationSource) => c.name === selected);

  const hasGatewaySources = sourceList.some(
    (c) => c.name.startsWith("slack:") || c.name.startsWith("telegram:") || c.name.startsWith("discord:")
  );

  if (!hasGatewaySources) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="max-w-lg text-center px-6">
          <div className="text-4xl mb-4 opacity-40">#</div>
          <h2 className="text-xl font-semibold text-mycel-text mb-2">Connect your first app</h2>
          <p className="text-sm text-mycel-muted/60 mb-8">
            Link Slack, Telegram, or Discord to start receiving messages in your agents.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <div className="flex-1 flex flex-col min-w-0">
        {selected ? (
          <GatewayFeed
            channelName={selected}
            channel={selectedSource}
            onPeekAgent={setPeekAgent}
          />
        ) : (
          <div className="flex-1 flex items-center justify-center">
            <EmptyState
              icon="#"
              title="Select a channel"
              description="Choose a channel from the sidebar to view its activity feed."
            />
          </div>
        )}
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
