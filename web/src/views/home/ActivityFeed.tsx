import { useCallback } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import type { ChannelMessage, ChannelStats, NotificationSource } from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { DefaultAppIcon, PLATFORM_ICON_MAP } from "../../components/apps/PlatformIcons";
import { channelLeaf } from "../../components/apps/AppsHome";
import { sourcePlatform } from "../../components/apps/messageUtils";
import { parseActivityTs } from "../../components/apps/appStatus";
import { formatRelative } from "../../utils/time";
import { HomeModule } from "./Module";

/* ── ActivityFeed ───────────────────────────────────────────────────
   The Home "Notifications" feed: newest messages across every connected
   app channel, compact one-line rows (app icon · sender · channel ·
   snippet · relative time). Same endpoints as the Apps Notifications
   page, just a narrower net; the header links to the full feed. Polls
   15s.
─────────────────────────────────────────────────────────────────── */

const CHANNEL_LIMIT = 10;
const PER_CHANNEL = 10;
const MAX_MESSAGES = 30;

interface FeedMessage extends ChannelMessage {
  channel: string;
}

/** Strip "[telegram] " style prefixes for cleaner sender display. */
function cleanSender(sender: string): string {
  const match = /^\[[a-z]+\]\s*(.+)$/i.exec(sender);
  return match?.[1] ?? sender;
}

/** One-line snippet: collapse whitespace, keep it short. */
function snippet(content: string): string {
  const s = content.replace(/\s+/g, " ").trim();
  return s.length > 140 ? s.slice(0, 137) + "…" : s;
}

function AppIcon({ base, size }: { base: string; size: number }) {
  const Icon = PLATFORM_ICON_MAP[base] ?? DefaultAppIcon;
  return <Icon size={size} />;
}

export function ActivityFeed() {
  const fetcher = useCallback(async (): Promise<FeedMessage[]> => {
    const [sources, stats] = await Promise.all([
      api.listNotificationSources().catch(() => [] as NotificationSource[]),
      api.getStatsChannels().catch(() => [] as ChannelStats[]),
    ]);
    const gwSources = (Array.isArray(sources) ? sources : []).filter(
      (s) => sourcePlatform(s.name) !== "internal",
    );
    const statByName = new Map((Array.isArray(stats) ? stats : []).map((s) => [s.name, s]));
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
        return (Array.isArray(msgs) ? msgs : []).map((m) => ({ ...m, channel: ch.name }));
      }),
    );
    return histories
      .flat()
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
      .slice(0, MAX_MESSAGES);
  }, []);

  const { data } = usePolling(fetcher, 15_000);
  const messages = data ?? [];

  return (
    <HomeModule label="Notifications" to="/apps/activity" testId="home-activity" fill>
      {data === null ? (
        <div className="py-4 text-center text-[11px] text-mycel-muted">Loading…</div>
      ) : messages.length === 0 ? (
        <div className="flex flex-col items-center gap-1.5 py-6 text-center">
          <span className="text-mycel-muted" aria-hidden>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            </svg>
          </span>
          <span className="text-[11px] text-mycel-muted">No notifications yet</span>
          <Link to="/apps" className="text-[11px] text-mycel-accent hover:underline">
            Connect an app →
          </Link>
        </div>
      ) : (
        <ul className="-m-1">
          {messages.map((m) => (
            <li key={`${m.channel}:${String(m.id)}`}>
              <Link
                to={`/apps/${m.channel}`}
                className="flex items-start gap-2 rounded-md px-1.5 py-[5px] hover:bg-mycel-surface-hover transition-colors min-w-0"
              >
                <span className="shrink-0 mt-[3px] text-mycel-muted" aria-hidden>
                  <AppIcon base={sourcePlatform(m.channel)} size={11} />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-baseline gap-1.5 min-w-0">
                    <span className="text-[11.5px] font-medium text-mycel-text truncate">
                      {cleanSender(m.sender)}
                    </span>
                    <span className="text-[10px] font-mono text-mycel-muted truncate">
                      #{channelLeaf(m.channel)}
                    </span>
                    <time
                      className="ml-auto shrink-0 text-[10px] text-mycel-muted tabular-nums"
                      title={new Date(m.created_at).toLocaleString()}
                    >
                      {formatRelative(m.created_at)}
                    </time>
                  </span>
                  <span className="block text-[11px] leading-[1.45] text-mycel-text-2 truncate">
                    {snippet(m.content)}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </HomeModule>
  );
}
