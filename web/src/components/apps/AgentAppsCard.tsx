/**
 * AgentAppsCard — the "Apps" card on the agent detail Config tab.
 *
 * Lists this agent's channel subscriptions (platform icon + channel +
 * mention-only toggle + remove) and adds new ones from the channels the
 * connected apps have discovered. Same subscription endpoints the
 * channel-side SubscriptionPanel uses, scoped to one agent.
 */

import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import type { NotificationSource, NotifySubscription } from "../../api/client";
import { sourcePlatform } from "./messageUtils";
import { AppIcon } from "./PlatformIcons";

function channelLeaf(ch: string): string {
  const i = ch.lastIndexOf(":");
  return i >= 0 ? ch.slice(i + 1) : ch;
}

function ChannelGlyph({ channel }: { channel: string }) {
  return <AppIcon base={sourcePlatform(channel)} size={13} />;
}

export function AgentAppsCard({ agentName }: { agentName: string }) {
  const [subs, setSubs] = useState<NotifySubscription[]>([]);
  const [channels, setChannels] = useState<NotificationSource[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [addChannel, setAddChannel] = useState("");

  const fetchData = useCallback(async () => {
    try {
      const [allSubs, sources] = await Promise.all([
        api.listSubscriptions().catch(() => [] as NotifySubscription[]),
        api.listNotificationSources().catch(() => [] as NotificationSource[]),
      ]);
      setSubs((allSubs ?? []).filter((s) => s.agent === agentName));
      setChannels((sources ?? []).filter((s) => sourcePlatform(s.name) !== "internal"));
      setLoaded(true);
    } catch { /* keep previous state */ }
  }, [agentName]);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => void fetchData(), 12000);
    return () => { clearInterval(interval); };
  }, [fetchData]);

  const subscribedSet = new Set(subs.map((s) => s.channel));
  const available = channels.filter((c) => !subscribedSet.has(c.name));

  const handleAdd = async () => {
    if (!addChannel) return;
    setBusy(true);
    try {
      await api.subscribe(addChannel, agentName, false);
      setAddChannel("");
      await fetchData();
    } catch { /* best effort */ }
    setBusy(false);
  };

  const handleRemove = async (channel: string) => {
    setBusy(true);
    try {
      await api.unsubscribe(channel, agentName);
      await fetchData();
    } catch { /* best effort */ }
    setBusy(false);
  };

  const handleToggleMention = async (sub: NotifySubscription) => {
    try {
      await api.setMentionOnly(sub.channel, agentName, !sub.mention_only);
      await fetchData();
    } catch { /* best effort */ }
  };

  return (
    <div data-testid="agent-apps-card">
      {/* Subscription rows */}
      {!loaded ? (
        <div className="text-xs text-mycel-muted py-2">Loading app subscriptions…</div>
      ) : subs.length === 0 ? (
        <p className="text-xs text-mycel-muted italic mb-3">
          Not listening to any app channels. Subscribe below, or connect new apps on the{" "}
          <Link to="/apps" className="text-mycel-accent hover:underline not-italic">Apps</Link> page.
        </p>
      ) : (
        <div className="mb-3 rounded-md border border-mycel-border overflow-hidden divide-y divide-mycel-border">
          {subs.map((sub) => (
            <div key={sub.channel} className="flex items-center gap-2.5 px-3 py-2 bg-mycel-surface group">
              <span className="shrink-0 flex items-center justify-center w-4">
                <ChannelGlyph channel={sub.channel} />
              </span>
              <Link
                to={`/apps/${sub.channel}`}
                className="flex-1 min-w-0 text-[12px] text-mycel-text truncate hover:text-mycel-accent transition-colors"
                title={sub.channel}
              >
                {channelLeaf(sub.channel)}
                <span className="ml-2 font-mono text-[10px] text-mycel-muted">{sub.channel}</span>
              </Link>
              <button
                type="button"
                onClick={() => { void handleToggleMention(sub); }}
                className={`shrink-0 text-[10px] px-2 py-0.5 rounded-md border transition-all duration-150 ${
                  sub.mention_only
                    ? "border-mycel-accent bg-mycel-accent-subtle text-mycel-accent"
                    : "border-mycel-border text-mycel-muted hover:border-mycel-border-strong"
                }`}
                title={sub.mention_only ? "Delivers @mentions only" : "Delivers all messages"}
              >
                {sub.mention_only ? "@ mentions" : "all msgs"}
              </button>
              <button
                type="button"
                onClick={() => { void handleRemove(sub.channel); }}
                disabled={busy}
                className="shrink-0 text-[11px] text-mycel-muted hover:text-mycel-error transition-colors opacity-0 group-hover:opacity-100 disabled:opacity-30"
                aria-label={`Unsubscribe from ${sub.channel}`}
              >
                remove
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Add row */}
      {available.length > 0 && (
        <div className="flex items-center gap-2">
          <select
            value={addChannel}
            onChange={(e) => { setAddChannel(e.target.value); }}
            className="flex-1 min-w-0 rounded-md border border-mycel-border-strong bg-mycel-bg px-2.5 py-1.5 text-[11px] text-mycel-text outline-none focus:border-mycel-accent transition-colors"
            aria-label="Channel to subscribe"
          >
            <option value="">— add a channel —</option>
            {available.map((c) => (
              <option key={c.name} value={c.name}>{c.name}</option>
            ))}
          </select>
          <button
            type="button"
            onClick={() => { void handleAdd(); }}
            disabled={busy || !addChannel}
            className="shrink-0 inline-flex items-center px-3 py-1.5 rounded-md border border-mycel-accent bg-mycel-accent-subtle text-xs font-medium text-mycel-accent hover:bg-mycel-accent hover:text-mycel-accent-fg transition-colors disabled:opacity-40"
          >
            Subscribe
          </button>
        </div>
      )}
    </div>
  );
}
