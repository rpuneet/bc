import { useCallback, useEffect, useMemo } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { NotificationSource } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { ChannelStream } from "../components/notifications/ChannelStream";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
export function Notifications() {
  // Set a default header; ChannelStream overrides this when a channel is active.
  // useMemo ensures stable references to avoid infinite re-render loops.
  const defaultTitle = useMemo(() => <TabHeaderTitle>Notifications</TabHeaderTitle>, []);
  useHeaderSlot({ title: defaultTitle });

  const { sourceName: paramSource } = useParams<{ sourceName: string }>();
  const navigate = useNavigate();

  const fetcher = useCallback(() => api.listNotificationSources(), []);
  const { data: sources, loading, error, refresh, timedOut } = usePolling(fetcher, 10000);

  const selected = paramSource ?? null;

  // Auto-select first gateway source if none selected
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

  // Check if there are any gateway sources
  const hasGatewaySources = sourceList.some(
    (c) => c.name.startsWith("slack:") || c.name.startsWith("telegram:") || c.name.startsWith("discord:")
  );

  // Empty state: no gateway sources at all
  if (!hasGatewaySources) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="max-w-lg text-center px-6">
          <div className="text-4xl mb-4 opacity-40">#</div>
          <h2 className="text-xl font-semibold text-mycel-text mb-2">Connect your first app</h2>
          <p className="text-sm text-mycel-muted/60 mb-8">
            Link Slack, Telegram, or Discord to start receiving messages in your agents.
          </p>
          <div className="grid grid-cols-3 gap-3 max-w-sm mx-auto">
            {[
              { name: "Slack", color: "#E01E5A" },
              { name: "Telegram", color: "#26A5E4" },
              { name: "Discord", color: "#5865F2" },
              { name: "GitHub", color: "#8B949E" },
              { name: "Gmail", color: "#EA4335" },
            ].map((p) => (
              <button
                key={p.name}
                type="button"
                className="p-4 border border-mycel-border/40 rounded-xl hover:border-mycel-border/60 hover:bg-mycel-surface/30 transition-all text-center group"
              >
                <div
                  className="w-8 h-8 rounded-lg flex items-center justify-center text-sm font-bold mx-auto mb-2"
                  style={{ backgroundColor: `${p.color}15`, color: p.color }}
                >
                  {p.name.charAt(0)}
                </div>
                <span className="text-xs font-medium text-mycel-muted/60 group-hover:text-mycel-text transition-colors">
                  {p.name}
                </span>
              </button>
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <div className="flex-1 flex flex-col min-w-0">
        {selected ? (
          <ChannelStream channelName={selected} channel={selectedSource} />
        ) : (
          <div className="flex-1 flex items-center justify-center">
            <EmptyState
              icon="#"
              title="Select a channel"
              description="Choose a channel from the sidebar to view its subscriptions and delivery log."
            />
          </div>
        )}
      </div>
    </div>
  );
}
