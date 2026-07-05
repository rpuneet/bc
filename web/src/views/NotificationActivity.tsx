/**
 * NotificationActivity — the full-page view behind the home page's
 * "Recent activity" rail (#3310 follow-up). It shows a richer, deeper
 * feed of the newest messages across every gateway channel with more
 * controls than the inline preview:
 *
 *   • filter by app / platform
 *   • filter by channel
 *   • free-text search over sender + content
 *
 * The controls live in the shared Header via useHeaderSlot, consistent
 * with the rest of the app. Data reuses the same endpoints and helpers
 * the home rail composes from — just a wider net (more channels, more
 * messages each).
 */

import { useCallback, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type {
  ChannelMessage,
  ChannelStats,
  NotificationSource,
} from "../api/client";
import { usePolling } from "../hooks/usePolling";
import { useHeaderSlot } from "../context/HeaderSlotContext";
import { EmptyState } from "../components/EmptyState";
import { LoadingSkeleton } from "../components/LoadingSkeleton";
import { MessageContent } from "../components/MessageContent";
import { DefaultAppIcon, PLATFORM_ICON_MAP } from "../components/notifications/PlatformIcons";
import { channelLeaf } from "../components/notifications/NotificationsHome";
import { agentColor, sourcePlatform } from "../components/notifications/messageUtils";
import { parseActivityTs } from "../components/notifications/appStatus";
import { formatRelative } from "../utils/time";

/* ── Config ──────────────────────────────────────────────────── */

// Wider net than the home rail's 6×10 preview: pull recent history from
// more channels and more messages per channel for a real feed.
const CHANNEL_LIMIT = 24;
const PER_CHANNEL = 25;
const MAX_MESSAGES = 300;

interface ActivityMessage extends ChannelMessage {
  channel: string;
}

/** Strip "[telegram] " style prefixes for cleaner sender display. */
function cleanSender(sender: string): string {
  const match = /^\[[a-z]+\]\s*(.+)$/i.exec(sender);
  return match?.[1] ?? sender;
}

function AppIcon({ base, size }: { base: string; size: number }) {
  const Icon = PLATFORM_ICON_MAP[base] ?? DefaultAppIcon;
  return <Icon size={size} />;
}

/* ── Component ───────────────────────────────────────────────── */

export function NotificationActivity() {
  const navigate = useNavigate();

  const fetcher = useCallback(async (): Promise<ActivityMessage[]> => {
    const [sources, stats] = await Promise.all([
      api.listNotificationSources().catch(() => [] as NotificationSource[]),
      api.getStatsChannels().catch(() => [] as ChannelStats[]),
    ]);
    const gwSources = (sources ?? []).filter((s) => sourcePlatform(s.name) !== "internal");
    const statByName = new Map((stats ?? []).map((s) => [s.name, s]));
    const ranked = [...gwSources]
      .sort((a, b) => {
        const ta = parseActivityTs(statByName.get(a.name)?.last_activity) ?? 0;
        const tb = parseActivityTs(statByName.get(b.name)?.last_activity) ?? 0;
        return tb - ta;
      })
      .slice(0, CHANNEL_LIMIT);
    const histories = await Promise.all(
      ranked.map(async (ch) => {
        const msgs = await api.getChannelHistory(ch.name, PER_CHANNEL).catch(() => [] as ChannelMessage[]);
        return (msgs ?? []).map((m) => ({ ...m, channel: ch.name }));
      }),
    );
    return histories
      .flat()
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
      .slice(0, MAX_MESSAGES);
  }, []);

  const { data, loading, error, refresh } = usePolling(fetcher, 15000);
  const messages = useMemo(() => data ?? [], [data]);

  const [search, setSearch] = useState("");
  const [platform, setPlatform] = useState("");
  const [channel, setChannel] = useState("");

  // Distinct platforms / channels present in the current feed drive the
  // filter dropdowns — no separate metadata fetch needed.
  const platforms = useMemo(() => {
    const set = new Set<string>();
    for (const m of messages) set.add(sourcePlatform(m.channel));
    return [...set].sort();
  }, [messages]);

  const channels = useMemo(() => {
    const set = new Set<string>();
    for (const m of messages) {
      if (!platform || sourcePlatform(m.channel) === platform) set.add(m.channel);
    }
    return [...set].sort();
  }, [messages, platform]);

  const query = search.trim().toLowerCase();
  const filtered = useMemo(() => {
    return messages.filter((m) => {
      if (platform && sourcePlatform(m.channel) !== platform) return false;
      if (channel && m.channel !== channel) return false;
      if (query && !m.content.toLowerCase().includes(query) && !m.sender.toLowerCase().includes(query)) return false;
      return true;
    });
  }, [messages, platform, channel, query]);

  const hasFilters = query !== "" || platform !== "" || channel !== "";

  /* ── Header slot: back + count · search · platform · channel ─── */
  useHeaderSlot({
    title: (
      <div className="flex items-center min-w-0 gap-2">
        <button
          type="button"
          onClick={() => { navigate(-1); }}
          className="flex items-center justify-center shrink-0 w-[22px] h-[22px] rounded-md text-mycel-muted hover:text-mycel-text"
          title="Go back"
          aria-label="Go back"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
        <span className="truncate text-[13px] font-semibold text-mycel-text">Activity</span>
        <span className="shrink-0 text-xs text-mycel-muted tabular-nums">
          {hasFilters ? `${String(filtered.length)} of ${String(messages.length)}` : `${String(messages.length)} message${messages.length === 1 ? "" : "s"}`}
        </span>
      </div>
    ),
    actions: (
      <>
        <input
          type="text"
          value={search}
          onChange={(e) => { setSearch(e.target.value); }}
          placeholder="Search messages"
          className="flex-1 min-w-[96px] max-w-md h-9 px-3 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
          aria-label="Search messages"
        />
        <select
          value={platform}
          onChange={(e) => { setPlatform(e.target.value); setChannel(""); }}
          className="shrink-0 h-9 px-2 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent"
          aria-label="Filter by app"
        >
          <option value="">All apps</option>
          {platforms.map((p) => (
            <option key={p} value={p}>{p}</option>
          ))}
        </select>
        <select
          value={channel}
          onChange={(e) => { setChannel(e.target.value); }}
          className="shrink-0 h-9 px-2 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text focus:outline-none focus:ring-1 focus:ring-mycel-accent max-w-[180px]"
          aria-label="Filter by channel"
        >
          <option value="">All channels</option>
          {channels.map((c) => (
            <option key={c} value={c}>{channelLeaf(c)}</option>
          ))}
        </select>
      </>
    ),
  });

  /* ── States ─────────────────────────────────────────────────── */

  if (loading && !data) {
    return (
      <div className="p-6">
        <LoadingSkeleton variant="text" rows={10} />
      </div>
    );
  }
  if (error && !data) {
    return (
      <div className="p-6">
        <EmptyState icon="!" title="Failed to load activity" description={error} actionLabel="Retry" onAction={refresh} />
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-3xl p-6 pb-24">
        {filtered.length === 0 ? (
          <div className="rounded-lg border border-mycel-border p-10 text-center text-sm text-mycel-muted">
            {hasFilters ? "No messages match the current filters." : "No messages yet."}
          </div>
        ) : (
          <ul className="rounded-lg border border-mycel-border overflow-hidden divide-y divide-mycel-border bg-mycel-surface">
            {filtered.map((m) => {
              const color = agentColor(m.sender);
              const sender = cleanSender(m.sender);
              return (
                <li key={`${m.channel}:${String(m.id)}`}>
                  <Link
                    to={`/notifications/${m.channel}`}
                    className="flex gap-3 px-4 py-3 hover:bg-mycel-surface-hover transition-colors"
                  >
                    <span
                      className="w-8 h-8 rounded-full flex items-center justify-center text-xs font-semibold shrink-0"
                      style={{ backgroundColor: `${color}20`, color }}
                      aria-hidden
                    >
                      {sender.charAt(0).toUpperCase()}
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="font-medium text-sm text-mycel-text truncate">{sender}</span>
                        <span className="flex items-center gap-1 shrink-0 text-[11px] text-mycel-muted min-w-0">
                          <AppIcon base={sourcePlatform(m.channel)} size={11} />
                          <span className="truncate max-w-[160px]">{channelLeaf(m.channel)}</span>
                        </span>
                        <time className="ml-auto shrink-0 text-[11px] text-mycel-muted tabular-nums" title={new Date(m.created_at).toLocaleString()}>
                          {formatRelative(m.created_at)}
                        </time>
                      </div>
                      <div className="mt-0.5 text-sm text-mycel-text-2 break-words whitespace-pre-wrap leading-[1.6]">
                        <MessageContent content={m.content} />
                      </div>
                    </div>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
