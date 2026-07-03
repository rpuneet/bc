/**
 * ChannelStream — the entire notifications-page surface for a single
 * gateway channel. Renders three sections:
 *
 *   1. Subscriptions — which agents receive this channel's stream,
 *      and whether they filter to @-mentions only.
 *   2. Delivery log — the last N inbound-delivery attempts (agent,
 *      status, content preview, timestamp).
 *   3. A stub info line explaining that mycel does not chat back —
 *      outbound is per-agent via env.json + direct platform API
 *      (see docs/architecture-notifications.md#outbound-cookbook).
 *
 * Explicitly does NOT render:
 *   - A message composer (mycel is not a chat client).
 *   - Message-by-message history threading, avatars, reactions.
 *   - Per-message emoji / read-state UI.
 *
 * Notifications are a one-way inbound stream routed to subscribed
 * agents — the UI is a routing + observability surface, not a chat
 * surface.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../../api/client";
import type {
  NotificationSource,
  NotifySubscription,
  DeliveryEntry,
} from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { EmptyState } from "../EmptyState";
import { LoadingSkeleton } from "../LoadingSkeleton";
import { useHeaderSlot } from "../../context/HeaderSlotContext";
import { TabHeaderTitle } from "../Header";
import { MONO } from "../../utils/typography";

function platformOf(channel: string): { platform: string; name: string } {
  const idx = channel.indexOf(":");
  if (idx < 0) return { platform: "", name: channel };
  return { platform: channel.slice(0, idx), name: channel.slice(idx + 1) };
}

function shortAgo(iso: string): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "";
  const s = Math.max(1, Math.floor((Date.now() - t) / 1000));
  if (s < 60) return `${String(s)}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${String(m)}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${String(h)}h`;
  const d = Math.floor(h / 24);
  return `${String(d)}d`;
}

export function ChannelStream({
  channelName,
  channel,
}: {
  channelName: string;
  channel?: NotificationSource;
}) {
  const { platform, name } = platformOf(channelName);
  const displayName = channel?.name?.split(":").pop() ?? name;

  useHeaderSlot({
    title: (
      <TabHeaderTitle>
        <span className="text-mycel-muted mr-1">#</span>
        <span>{displayName}</span>
        {platform && (
          <span className="ml-2 text-[10px] uppercase tracking-[0.15em] text-mycel-muted/70" style={{ fontFamily: MONO }}>
            {platform}
          </span>
        )}
      </TabHeaderTitle>
    ),
  });

  const subsFetcher = useCallback(
    () => api.getChannelSubscriptions(channelName),
    [channelName],
  );
  const { data: subs, loading: subsLoading, refresh: refreshSubs } = usePolling(subsFetcher, 15000);

  const activityFetcher = useCallback(
    () => api.getChannelActivity(channelName, 100),
    [channelName],
  );
  const { data: activity, loading: activityLoading, refresh: refreshActivity } = usePolling(activityFetcher, 5000);

  const [busyUnsub, setBusyUnsub] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const unsubscribe = async (agent: string) => {
    setBusyUnsub(agent);
    setErr(null);
    try {
      const r = await fetch(
        `/api/notify/subscriptions?channel=${encodeURIComponent(channelName)}&agent=${encodeURIComponent(agent)}`,
        { method: "DELETE" },
      );
      if (!r.ok) throw new Error(`${String(r.status)} ${r.statusText}`);
      refreshSubs();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "unsubscribe failed");
    } finally {
      setBusyUnsub(null);
    }
  };

  useEffect(() => {
    setErr(null);
  }, [channelName]);

  const stats = useMemo(() => {
    const list: DeliveryEntry[] = activity ?? [];
    const delivered = list.filter((e) => e.status === "delivered").length;
    const failed = list.filter((e) => e.status === "failed").length;
    return { total: list.length, delivered, failed };
  }, [activity]);

  return (
    <div className="flex-1 flex flex-col min-h-0 overflow-y-auto">
      <div className="max-w-4xl w-full mx-auto p-6 space-y-8">
        {/* Subscriptions */}
        <section className="space-y-3">
          <header className="flex items-baseline justify-between">
            <h2 className="text-sm font-semibold text-mycel-text/90" style={{ fontFamily: MONO }}>
              Subscriptions
            </h2>
            <span className="text-[10px] uppercase tracking-[0.15em] text-mycel-muted">
              {subs ? `${String(subs.length)} agent${subs.length === 1 ? "" : "s"}` : "…"}
            </span>
          </header>
          {subsLoading && !subs && <LoadingSkeleton variant="text" rows={3} />}
          {subs && subs.length === 0 && (
            <EmptyState
              icon="—"
              title="No subscribers"
              description="No agent is currently routed messages from this channel."
            />
          )}
          {subs && subs.length > 0 && (
            <div className="rounded border border-mycel-border/40 overflow-hidden">
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-left text-mycel-muted/70">
                    <th className="px-3 py-2 font-medium">Agent</th>
                    <th className="px-3 py-2 font-medium">Filter</th>
                    <th className="px-3 py-2 font-medium">Since</th>
                    <th className="px-3 py-2" />
                  </tr>
                </thead>
                <tbody>
                  {subs.map((s: NotifySubscription) => (
                    <tr key={`${s.channel}/${s.agent}`} className="border-t border-mycel-border/30">
                      <td className="px-3 py-2 text-mycel-text/90 font-medium" style={{ fontFamily: MONO }}>
                        {s.agent}
                      </td>
                      <td className="px-3 py-2 text-mycel-muted">
                        {s.mention_only ? "@-mentions only" : "all messages"}
                      </td>
                      <td className="px-3 py-2 text-mycel-muted tabular-nums">
                        {shortAgo(s.created_at)}
                      </td>
                      <td className="px-3 py-2 text-right">
                        <button
                          type="button"
                          onClick={() => void unsubscribe(s.agent)}
                          disabled={busyUnsub === s.agent}
                          className="text-[11px] text-mycel-muted hover:text-mycel-error disabled:opacity-40"
                        >
                          {busyUnsub === s.agent ? "…" : "unsubscribe"}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {err && <p className="text-[11px] text-mycel-error">{err}</p>}
        </section>

        {/* Delivery log */}
        <section className="space-y-3">
          <header className="flex items-baseline justify-between">
            <h2 className="text-sm font-semibold text-mycel-text/90" style={{ fontFamily: MONO }}>
              Delivery log
            </h2>
            <div className="flex items-center gap-3 text-[10px] uppercase tracking-[0.15em] text-mycel-muted">
              <span>{stats.delivered} delivered</span>
              {stats.failed > 0 && <span className="text-mycel-error">{stats.failed} failed</span>}
              <button
                type="button"
                onClick={refreshActivity}
                className="text-mycel-muted hover:text-mycel-text"
              >
                refresh
              </button>
            </div>
          </header>
          {activityLoading && !activity && <LoadingSkeleton variant="text" rows={5} />}
          {activity && activity.length === 0 && (
            <EmptyState
              icon="—"
              title="No deliveries yet"
              description="Inbound messages routed to subscribed agents will appear here."
            />
          )}
          {activity && activity.length > 0 && (
            <ul className="space-y-1">
              {activity.map((e: DeliveryEntry) => (
                <li
                  key={e.id}
                  className="grid grid-cols-[auto_1fr_auto] items-center gap-3 px-3 py-1.5 rounded border border-mycel-border/30 text-xs"
                >
                  <span
                    className={`inline-block w-1.5 h-1.5 rounded-full shrink-0 ${
                      e.status === "delivered"
                        ? "bg-mycel-success"
                        : e.status === "failed"
                          ? "bg-mycel-error"
                          : "bg-mycel-muted/50"
                    }`}
                    title={e.status}
                  />
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-mycel-text/90 font-medium" style={{ fontFamily: MONO }}>
                        {e.agent}
                      </span>
                      {e.status === "failed" && e.error && (
                        <span className="text-mycel-error text-[10px] truncate" title={e.error}>
                          {e.error}
                        </span>
                      )}
                    </div>
                    {e.preview && (
                      <p className="text-mycel-muted/80 truncate">{e.preview}</p>
                    )}
                  </div>
                  <span className="text-mycel-muted tabular-nums text-[10px]">
                    {shortAgo(e.logged_at)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>

        {/* Outbound note */}
        <section className="text-[11px] text-mycel-muted/80 border-t border-mycel-border/30 pt-4">
          <p>
            mycel does not proxy agent replies back to the platform.
            When an agent needs to post to {platform || "the platform"},
            it calls the official REST API directly with a bot token
            loaded from its own <code className="text-mycel-text/70">env.json</code>.
            See <a
              className="text-mycel-accent hover:underline"
              href="https://rpuneet.github.io/mycel/architecture-notifications/#outbound-cookbook"
              target="_blank"
              rel="noopener noreferrer"
            >
              the outbound cookbook
            </a> for recipes.
          </p>
        </section>
      </div>
    </div>
  );
}
