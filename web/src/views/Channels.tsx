import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { Channel } from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { AgentPeekPanel } from "../components/AgentPeekPanel";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { EmptyState } from "../components/EmptyState";
import { GatewayFeed } from "../components/channels/GatewayFeed";

import { useHeaderSlot } from "../context/HeaderSlotContext";
import { TabHeaderTitle } from "../components/Header";
export function Channels() {
  useHeaderSlot({ title: <TabHeaderTitle>Notifications</TabHeaderTitle> });

  const { channelName: paramChannel } = useParams<{ channelName: string }>();
  const navigate = useNavigate();

  const fetcher = useCallback(() => api.listChannels(), []);
  const { data: channels, loading, error, refresh, timedOut } = usePolling(fetcher, 10000);

  const selected = paramChannel ?? null;
  const [peekAgent, setPeekAgent] = useState<string | null>(null);

  // Auto-select first gateway channel if none selected
  useEffect(() => {
    if (!selected && channels && channels.length > 0) {
      const gwChannel = channels.find((c) =>
        c.name.startsWith("slack:") || c.name.startsWith("telegram:") || c.name.startsWith("discord:")
      );
      if (gwChannel) {
        navigate("/channels/" + gwChannel.name, { replace: true });
      }
    }
  }, [selected, channels, navigate]);

  if (loading && !channels) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-6 w-28 animate-pulse rounded bg-bc-border/50" />
        <LoadingSkeleton variant="text" rows={5} />
      </div>
    );
  }

  if (timedOut && !channels) {
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

  if (error && !channels) {
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

  const channelList = channels ?? [];
  const selectedChannel = channelList.find((c: Channel) => c.name === selected);

  // Check if there are any gateway channels
  const hasGatewayChannels = channelList.some(
    (c) => c.name.startsWith("slack:") || c.name.startsWith("telegram:") || c.name.startsWith("discord:")
  );

  // Empty state: no gateway channels at all
  if (!hasGatewayChannels) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="max-w-lg text-center px-6">
          <div className="text-4xl mb-4 opacity-40">#</div>
          <h2 className="text-xl font-semibold text-bc-text mb-2">Connect your first app</h2>
          <p className="text-sm text-bc-muted/60 mb-8">
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
                className="p-4 border border-bc-border/30 rounded-xl hover:border-bc-border/60 hover:bg-bc-surface/30 transition-all text-center group"
              >
                <div
                  className="w-8 h-8 rounded-lg flex items-center justify-center text-sm font-bold mx-auto mb-2"
                  style={{ backgroundColor: `${p.color}15`, color: p.color }}
                >
                  {p.name.charAt(0)}
                </div>
                <span className="text-xs font-medium text-bc-muted/60 group-hover:text-bc-text transition-colors">
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
      {/* Activity feed — full width, notification tree is now in the main nav sidebar */}
      <div className="flex-1 flex flex-col min-w-0">
        {selected ? (
          <GatewayFeed
            channelName={selected}
            channel={selectedChannel}
            onPeekAgent={setPeekAgent}
          />
        ) : (
          <div className="flex-1 flex items-center justify-center">
            <EmptyState
              icon="#"
              title="Select a notification source"
              description="Choose a notification source from the sidebar to view activity."
            />
          </div>
        )}
      </div>

      {/* Agent peek overlay */}
      {peekAgent && (
        <AgentPeekPanel
          agentName={peekAgent}
          onClose={() => setPeekAgent(null)}
        />
      )}
    </div>
  );
}
