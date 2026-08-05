/**
 * AgentAppsCard — Notifications subscriptions on the agent detail Config tab.
 *
 * Lists this agent's channel subscriptions (platform icon + channel +
 * mention-only toggle + remove) and adds new ones via a searchable,
 * platform-grouped picker (#3647).
 */

import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import type { NotificationSource, NotifySubscription } from "../../api/client";
import { sourcePlatform } from "./messageUtils";
import { AppIcon } from "./PlatformIcons";

function channelLeaf(ch: string): string {
  const i = ch.lastIndexOf(":");
  const leaf = i >= 0 ? ch.slice(i + 1) : ch;
  return leaf === "*" ? "catch-all" : leaf;
}

function ChannelGlyph({ channel }: { channel: string }) {
  return <AppIcon base={sourcePlatform(channel)} size={13} />;
}

function platformLabel(platform: string): string {
  if (!platform) return "Other";
  return platform.charAt(0).toUpperCase() + platform.slice(1);
}

/** Group available channels by platform, filtered by search. */
function groupChannels(
  channels: NotificationSource[],
  query: string,
): { platform: string; items: NotificationSource[] }[] {
  const q = query.trim().toLowerCase();
  const filtered = q
    ? channels.filter((c) => {
        const plat = sourcePlatform(c.name).toLowerCase();
        const leaf = channelLeaf(c.name).toLowerCase();
        return (
          c.name.toLowerCase().includes(q) ||
          plat.includes(q) ||
          leaf.includes(q) ||
          (c.description ?? "").toLowerCase().includes(q)
        );
      })
    : channels;

  const byPlat = new Map<string, NotificationSource[]>();
  for (const c of filtered) {
    const plat = sourcePlatform(c.name) || "other";
    const list = byPlat.get(plat) ?? [];
    list.push(c);
    byPlat.set(plat, list);
  }

  return [...byPlat.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([platform, items]) => ({
      platform,
      items: items.sort((x, y) => x.name.localeCompare(y.name)),
    }));
}

export function AgentAppsCard({ agentName }: { agentName: string }) {
  const [subs, setSubs] = useState<NotifySubscription[]>([]);
  const [channels, setChannels] = useState<NotificationSource[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const rootRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

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

  useEffect(() => {
    if (!pickerOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) {
        setPickerOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => { document.removeEventListener("mousedown", onDoc); };
  }, [pickerOpen]);

  useEffect(() => {
    if (pickerOpen) {
      setHighlight(0);
      // Focus search when the picker opens.
      requestAnimationFrame(() => searchRef.current?.focus());
    }
  }, [pickerOpen]);

  const subscribedSet = useMemo(() => new Set(subs.map((s) => s.channel)), [subs]);
  const available = useMemo(
    () => channels.filter((c) => !subscribedSet.has(c.name)),
    [channels, subscribedSet],
  );
  const groups = useMemo(() => groupChannels(available, query), [available, query]);
  const flat = useMemo(() => groups.flatMap((g) => g.items), [groups]);

  const handleSubscribe = async (channel: string) => {
    if (!channel || busy) return;
    setBusy(true);
    try {
      await api.subscribe(channel, agentName, false);
      setPickerOpen(false);
      setQuery("");
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

  const onSearchKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setHighlight((h) => Math.min(h + 1, Math.max(flat.length - 1, 0)));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setHighlight((h) => Math.max(h - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const pick = flat[highlight];
      if (pick) void handleSubscribe(pick.name);
    } else if (e.key === "Escape") {
      setPickerOpen(false);
      setQuery("");
    }
  };

  return (
    <div data-testid="agent-apps-card" ref={rootRef}>
      {!loaded ? (
        <div className="text-xs text-mycel-muted py-2">Loading notification subscriptions…</div>
      ) : subs.length === 0 ? (
        <p className="text-xs text-mycel-muted italic mb-3">
          Not listening to any notification channels. Subscribe below, or connect platforms on the{" "}
          <Link to="/apps" className="text-mycel-accent hover:underline not-italic">Apps</Link> page.
        </p>
      ) : (
        <div className="mb-3 rounded-md border border-mycel-border overflow-hidden divide-y divide-mycel-border">
          {subs
            .slice()
            .sort((a, b) => a.channel.localeCompare(b.channel))
            .map((sub) => (
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

      {available.length > 0 && (
        <div className="relative">
          <button
            type="button"
            onClick={() => { setPickerOpen((o) => !o); }}
            disabled={busy}
            className="w-full flex items-center justify-between gap-2 rounded-md border border-mycel-border-strong bg-mycel-bg px-2.5 py-1.5 text-[11px] text-mycel-muted hover:border-mycel-accent hover:text-mycel-text transition-colors disabled:opacity-40"
            aria-expanded={pickerOpen}
            aria-haspopup="listbox"
            aria-label="Add notification channel"
          >
            <span>— add a channel —</span>
            <span className="font-mono text-[10px] opacity-70">{available.length} available</span>
          </button>

          {pickerOpen && (
            <div
              className="absolute z-30 mt-1 w-full rounded-md border border-mycel-border-strong bg-mycel-surface shadow-lg overflow-hidden"
              role="listbox"
              aria-label="Available notification channels"
            >
              <div className="p-2 border-b border-mycel-border">
                <input
                  ref={searchRef}
                  type="search"
                  value={query}
                  onChange={(e) => {
                    setQuery(e.target.value);
                    setHighlight(0);
                  }}
                  onKeyDown={onSearchKeyDown}
                  placeholder="Search by app or channel…"
                  className="w-full rounded-md border border-mycel-border bg-mycel-bg px-2.5 py-1.5 text-[12px] text-mycel-text outline-none focus:border-mycel-accent"
                  aria-label="Search channels"
                />
              </div>
              <div className="max-h-64 overflow-auto py-1">
                {flat.length === 0 ? (
                  <p className="px-3 py-2 text-[11px] text-mycel-muted">No channels match “{query}”.</p>
                ) : (
                  groups.map((g) => (
                    <div key={g.platform} className="mb-1">
                      <div className="sticky top-0 z-10 flex items-center gap-1.5 px-3 py-1 bg-mycel-surface-hover/95 text-[10px] font-semibold uppercase tracking-[0.08em] text-mycel-muted">
                        <AppIcon base={g.platform} size={11} />
                        {platformLabel(g.platform)}
                        <span className="font-mono font-normal normal-case tracking-normal opacity-70">
                          {g.items.length}
                        </span>
                      </div>
                      {g.items.map((c) => {
                        const idx = flat.indexOf(c);
                        const active = idx === highlight;
                        return (
                          <button
                            key={c.name}
                            type="button"
                            role="option"
                            aria-selected={active}
                            onMouseEnter={() => { setHighlight(idx); }}
                            onClick={() => { void handleSubscribe(c.name); }}
                            className={`w-full flex items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors ${
                              active
                                ? "bg-mycel-accent-subtle text-mycel-accent"
                                : "text-mycel-text hover:bg-mycel-surface-hover"
                            }`}
                          >
                            <span className="truncate flex-1 min-w-0">
                              <span className="text-mycel-text">{channelLeaf(c.name)}</span>
                              <span className="ml-2 font-mono text-[10px] text-mycel-muted">{c.name}</span>
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  ))
                )}
              </div>
            </div>
          )}
        </div>
      )}

      <p className="mt-2 text-[11px] text-mycel-muted">
        Notification channels this agent listens to — messages route here from connected platforms
        (Slack, Gmail, Telegram, WhatsApp, …).
      </p>
    </div>
  );
}
