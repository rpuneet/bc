/**
 * AppsHome — the /apps hub shown when no channel is selected. Four
 * sections built from data we already have:
 *
 *   1. Apps strip — one pill per connected app instance with the same
 *      status semantics as the drawer (shared appStatus utils), plus a
 *      "+ Connect" pill opening the catalog-driven connect flow.
 *   2. Notifications — the newest messages across active channels; the
 *      primary column on the left.
 *   3. Channels — all app channels grouped by instance (slim secondary
 *      column); WhatsApp splits into Groups / People. Search + Filters
 *      live in the header slot.
 *   4. Custom Keys — the encrypted vault keys agents reference via
 *      ${secret:NAME} (absorbed from the old standalone Secrets page).
 *
 * Data comes from GET /api/apps (catalog + instances) enriched by
 * GET /api/notifications/overview when available, and degrades
 * gracefully when the overview endpoint or its metadata are missing.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { api, instancesToStatuses } from "../../api/client";
import type {
  AppsCatalog,
  ChannelMessage,
  ChannelStats,
  GatewayHealth,
  GatewayStatus,
  NotificationSource,
  NotificationsOverview,
  NotifySubscription,
} from "../../api/client";
import { usePolling } from "../../hooks/usePolling";
import { useHeaderSlot } from "../../context/HeaderSlotContext";
import { EmptyState } from "../EmptyState";
import { LoadingSkeleton } from "../LoadingSkeleton";
import { DefaultAppIcon, PLATFORM_ICON_MAP } from "./PlatformIcons";
import { ConnectWizard, AppChooser } from "./ConnectApp";
import { CustomKeysSection } from "./CustomKeys";
import { sourcePlatform } from "./messageUtils";
import { IdentityAvatar } from "./IdentityAvatar";
import { formatRelative } from "../../utils/time";
import {
  disconnectReason,
  getAppStatus,
  parseActivityTs,
  StatusDot,
  type AppStatus,
} from "./appStatus";

/* ── Pure helpers (exported for tests) ───────────────────────── */

export type ChannelKind = "group" | "person" | null;

/** Leaf segment of a channel key: "whatsapp:123@g.us" → "123@g.us". */
export function channelLeaf(name: string): string {
  const parts = name.split(":");
  return parts[parts.length - 1] || name;
}

/** Classify a WhatsApp channel by JID shape when no metadata exists:
 *  "…@g.us" → group, "…@s.whatsapp.net" → person. */
export function whatsappKindFromId(name: string): ChannelKind {
  const id = channelLeaf(name);
  if (id.endsWith("@g.us")) return "group";
  if (id.endsWith("@s.whatsapp.net")) return "person";
  return null;
}

/** Kind from adapter metadata when present, JID-shape fallback for
 *  WhatsApp, null (unclassified) otherwise. */
export function resolveChannelKind(name: string, platform: string, kind?: string): ChannelKind {
  if (kind === "group" || kind === "person") return kind;
  if (platform === "whatsapp") return whatsappKindFromId(name);
  return null;
}

export interface ChannelItem {
  name: string;
  /** App bucket this channel belongs to (gateway platform key). */
  app: string;
  platform: string;
  displayName: string;
  kind: ChannelKind;
  participantCount: number | null;
  subscribers: string[];
  subscriberCount: number;
  messageCount: number | null;
  lastActivity: number | null;
}

export interface AppItem {
  /** Bucket key — a gateway platform key, possibly compound ("telegram:bot"). */
  key: string;
  base: string;
  label: string;
  botName?: string;
  status: AppStatus;
  reason: string | null;
  channelCount: number;
  lastActivity: number | null;
}

export interface HomeSnapshot {
  overview: NotificationsOverview | null;
  sources: NotificationSource[];
  gateways: GatewayStatus[];
  /** Descriptor labels by app id, from the /api/apps catalog. */
  labels: Record<string, string>;
  health: Record<string, GatewayHealth>;
  subs: NotifySubscription[];
  stats: ChannelStats[];
}

/** "telegram" → "Telegram" when no catalog label exists. */
function fallbackLabel(base: string): string {
  return base.charAt(0).toUpperCase() + base.slice(1);
}

/** Internal, non-connectable surfaces that must never show up as an app
 *  pill or filter option — they are page sections, not external apps. */
const NON_CONNECTABLE_APPS = new Set(["notifications", "secrets", "internal"]);

/** True when a bucket key is a real, connectable external app. */
export function isConnectableApp(keyOrBase: string): boolean {
  const base = keyOrBase.includes(":") ? (keyOrBase.split(":")[0] ?? keyOrBase) : keyOrBase;
  return !NON_CONNECTABLE_APPS.has(keyOrBase) && !NON_CONNECTABLE_APPS.has(base);
}

/** Build the page model, preferring overview metadata and falling back
 *  to the composed endpoints field by field. Pure — unit tested. */
export function buildHomeModel(snap: HomeSnapshot): { apps: AppItem[]; channels: ChannelItem[] } {
  const ovChannels = snap.overview?.channels ?? [];
  const ovApps = snap.overview?.apps ?? [];
  const ovByName = new Map(ovChannels.map((c) => [c.channel, c]));
  const statByName = new Map(snap.stats.map((s) => [s.name, s]));

  const subsByChannel = new Map<string, string[]>();
  for (const sub of snap.subs) {
    const list = subsByChannel.get(sub.channel) ?? [];
    list.push(sub.agent);
    subsByChannel.set(sub.channel, list);
  }

  // Union of /channels sources and overview channels.
  const names: string[] = snap.sources.map((s) => s.name);
  const seen = new Set(names);
  for (const c of ovChannels) {
    if (!seen.has(c.channel)) {
      seen.add(c.channel);
      names.push(c.channel);
    }
  }

  const appKeyFor = (name: string): string => {
    const matched = snap.gateways.find((gw) => name.startsWith(gw.platform + ":"));
    return matched?.platform ?? sourcePlatform(name);
  };

  const channels: ChannelItem[] = [];
  for (const name of names) {
    const app = appKeyFor(name);
    // Skip every non-connectable pseudo-app (internal, notifications,
    // secrets) — the same guard that keeps them out of the pill list.
    if (!isConnectableApp(app)) continue;
    const ov = ovByName.get(name);
    const st = statByName.get(name);
    const platform = ov?.platform ?? sourcePlatform(name);
    const subscribers = subsByChannel.get(name) ?? [];
    channels.push({
      name,
      app,
      platform,
      displayName: ov?.display_name?.trim() ? ov.display_name : channelLeaf(name),
      kind: resolveChannelKind(name, platform, ov?.kind),
      participantCount: ov?.participant_count ?? null,
      subscribers,
      subscriberCount: Math.max(ov?.subscriber_count ?? 0, subscribers.length),
      messageCount: ov?.message_count ?? st?.message_count ?? null,
      lastActivity:
        parseActivityTs(ov?.last_activity) ?? parseActivityTs(st?.last_activity),
    });
  }

  // App buckets: every gateway plus every bucket that has channels.
  // Internal surfaces (notifications, secrets) are page sections, not
  // connectable apps — they never earn a pill or a filter option.
  const bucketKeys = new Set<string>(snap.gateways.map((g) => g.platform).filter(isConnectableApp));
  for (const ch of channels) if (isConnectableApp(ch.app)) bucketKeys.add(ch.app);

  const apps: AppItem[] = [...bucketKeys].map((key) => {
    const base = key.includes(":") ? (key.split(":")[0] ?? key) : key;
    const gw = snap.gateways.find((g) => g.platform === key);
    const h = snap.health[key];
    const ovApp = ovApps.find((a) => a.name === key || a.platform === key || a.platform === base);
    const chs = channels.filter((c) => c.app === key);

    let status = getAppStatus(gw, h);
    // No live gateway/health entry — trust the overview's verdict.
    if (!gw && !h && ovApp) status = ovApp.connected ? "connected" : "error";

    const reason =
      status === "error"
        ? (h ? disconnectReason(base, h) : ovApp?.disconnect_reason || disconnectReason(base, h))
        : null;

    const chActivity = chs.reduce<number | null>(
      (acc, c) => (c.lastActivity !== null && (acc === null || c.lastActivity > acc) ? c.lastActivity : acc),
      null,
    );

    return {
      key,
      base,
      label: snap.labels[base] ?? fallbackLabel(base),
      botName: gw?.bot_name,
      status,
      reason,
      channelCount: chs.length > 0 ? chs.length : ovApp?.channel_count ?? 0,
      lastActivity:
        parseActivityTs(h?.last_message_at) ??
        parseActivityTs(ovApp?.last_activity) ??
        chActivity,
    };
  });

  // Connected apps first, then alphabetical — mirrors the drawer.
  apps.sort((a, b) => {
    const ra = a.status === "connected" ? 0 : 1;
    const rb = b.status === "connected" ? 0 : 1;
    if (ra !== rb) return ra - rb;
    return a.label.localeCompare(b.label) || a.key.localeCompare(b.key);
  });

  return { apps, channels };
}

/* ── Recent activity ─────────────────────────────────────────── */

export interface RecentMessage extends ChannelMessage {
  channel: string;
}

/** Strip "[telegram] " style prefixes for cleaner sender display. */
function cleanSender(sender: string): string {
  const match = /^\[[a-z]+\]\s*(.+)$/i.exec(sender);
  return match?.[1] ?? sender;
}

function oneLine(content: string): string {
  return content.replace(/\s+/g, " ").trim().slice(0, 140);
}

/* ── Small render helpers ────────────────────────────────────── */

function AppIcon({ base, size }: { base: string; size: number }) {
  const Icon = PLATFORM_ICON_MAP[base] ?? DefaultAppIcon;
  return <Icon size={size} />;
}

function SubscriberAvatars({ subscribers, count }: { subscribers: string[]; count: number }) {
  if (count <= 0) return null;
  return (
    <span className="flex items-center shrink-0" title={`${String(count)} subscribed agent${count === 1 ? "" : "s"}${subscribers.length > 0 ? ": " + subscribers.join(", ") : ""}`}>
      {subscribers.slice(0, 3).map((a, i) => (
        <span key={a} className="ring-1 ring-mycel-surface rounded-full" style={{ marginLeft: i === 0 ? 0 : -5 }}>
          <IdentityAvatar name={a} size={16} />
        </span>
      ))}
      <span className="ml-1 text-[10.5px] text-mycel-muted tabular-nums">
        {subscribers.length > 3 ? `+${String(count - 3)}` : count > subscribers.length || subscribers.length === 0 ? String(count) : null}
      </span>
    </span>
  );
}

function ChannelRowButton({ ch, onOpen }: { ch: ChannelItem; onOpen: (name: string) => void }) {
  const rawId = channelLeaf(ch.name);
  const hasDisplayName = ch.displayName !== rawId;
  return (
    <button
      type="button"
      onClick={() => { onOpen(ch.name); }}
      className="w-full flex items-center gap-3 px-3 py-2 bg-mycel-surface hover:bg-mycel-surface-hover text-left transition-colors"
      title={ch.name}
    >
      <IdentityAvatar name={ch.displayName} kind={ch.kind} size={28} />
      <span className="min-w-0 flex-1 flex items-baseline gap-2">
        <span className={`truncate text-[13px] font-medium text-mycel-text ${hasDisplayName ? "" : "font-mono"}`}>
          {ch.displayName}
        </span>
        {hasDisplayName && (
          <span className="hidden md:inline truncate font-mono text-[10.5px] text-mycel-muted">{rawId}</span>
        )}
      </span>
      <span className="hidden sm:flex items-center gap-3 shrink-0 text-[11px] text-mycel-muted tabular-nums">
        {ch.kind === "group" && ch.participantCount !== null && (
          <span>{String(ch.participantCount)} member{ch.participantCount === 1 ? "" : "s"}</span>
        )}
        <SubscriberAvatars subscribers={ch.subscribers} count={ch.subscriberCount} />
        {ch.messageCount !== null && <span>{String(ch.messageCount)} msgs</span>}
        {ch.lastActivity !== null && <span>{formatRelative(ch.lastActivity)}</span>}
      </span>
    </button>
  );
}

function SubsectionLabel({ label, count }: { label: string; count: number }) {
  return (
    <div className="flex items-center gap-2 px-3 py-2 bg-mycel-bg">
      <span className="text-[10.5px] font-semibold uppercase tracking-[0.09em] text-mycel-text-2">{label}</span>
      <span className="inline-flex items-center justify-center min-w-[18px] h-[16px] px-1 rounded-full bg-mycel-surface-hover text-[10px] font-medium text-mycel-muted tabular-nums">{count}</span>
    </div>
  );
}

/* ── Component ───────────────────────────────────────────────── */

export function AppsHome() {
  const navigate = useNavigate();

  const fetcher = useCallback(async (): Promise<HomeSnapshot & { recent: RecentMessage[] }> => {
    const [overview, sources, apps, subs, stats] = await Promise.all([
      api.getNotificationsOverview().catch(() => null),
      api.listNotificationSources().catch(() => [] as NotificationSource[]),
      api.getApps().catch(() => null as AppsCatalog | null),
      api.listSubscriptions().catch(() => [] as NotifySubscription[]),
      api.getStatsChannels().catch(() => [] as ChannelStats[]),
    ]);
    const gws = instancesToStatuses(apps?.instances ?? []);
    const labels = Object.fromEntries((apps?.catalog ?? []).map((d) => [d.id, d.label]));
    const healthEntries = await Promise.all(
      gws.filter((g) => g.enabled).map(async (g) => {
        const h = await api.getAppHealth(g.platform).catch(() => null);
        return h ? ([g.platform, h] as const) : null;
      }),
    );
    const health: Record<string, GatewayHealth> = {};
    for (const e of healthEntries) if (e) health[e[0]] = e[1];

    // Recent activity: newest messages from the most recently active
    // gateway channels (per-channel history is the endpoint we have).
    const gwSources = (sources ?? []).filter((s) => sourcePlatform(s.name) !== "internal");
    const statByName = new Map((stats ?? []).map((s) => [s.name, s]));
    const ranked = [...gwSources]
      .sort((a, b) => {
        const ta = parseActivityTs(statByName.get(a.name)?.last_activity) ?? 0;
        const tb = parseActivityTs(statByName.get(b.name)?.last_activity) ?? 0;
        return tb - ta;
      })
      .slice(0, 6);
    const histories = await Promise.all(
      ranked.map(async (ch) => {
        const msgs = await api.getChannelHistory(ch.name, 10).catch(() => [] as ChannelMessage[]);
        return (msgs ?? []).map((m) => ({ ...m, channel: ch.name }));
      }),
    );
    const recent = histories
      .flat()
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
      .slice(0, 15);

    return {
      overview,
      sources: gwSources,
      gateways: gws,
      labels,
      health,
      subs: subs ?? [],
      stats: stats ?? [],
      recent,
    };
  }, []);

  const { data, loading, error, refresh, timedOut } = usePolling(fetcher, 15000);

  const [search, setSearch] = useState("");
  const [appSel, setAppSel] = useState<Set<string>>(new Set());
  const [onlySubscribed, setOnlySubscribed] = useState(false);
  const [onlyActive24h, setOnlyActive24h] = useState(false);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const filtersRef = useRef<HTMLDivElement>(null);
  const [chooserOpen, setChooserOpen] = useState(false);
  const [connectAppId, setConnectAppId] = useState<string | null>(null);

  // Deep links: /apps?action=connect opens the catalog; /apps#custom-keys
  // (the old /secrets bookmark) scrolls to the Custom Keys section.
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  useEffect(() => {
    if (searchParams.get("action") === "connect") {
      setChooserOpen(true);
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        next.delete("action");
        return next;
      }, { replace: true });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams]);
  useEffect(() => {
    if (location.hash === "#custom-keys") {
      // Wait a frame so the section exists after data render.
      requestAnimationFrame(() => {
        document.getElementById("custom-keys")?.scrollIntoView({ block: "start" });
      });
    }
  }, [location.hash, data]);

  useEffect(() => {
    if (!filtersOpen) return;
    const onMouseDown = (e: MouseEvent) => {
      if (filtersRef.current && !filtersRef.current.contains(e.target as Node)) setFiltersOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setFiltersOpen(false);
    };
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [filtersOpen]);

  const model = useMemo(() => (data ? buildHomeModel(data) : { apps: [], channels: [] }), [data]);
  const { apps, channels } = model;
  const recent = data?.recent ?? [];

  const query = search.trim().toLowerCase();
  const hasFilters = query !== "" || appSel.size > 0 || onlySubscribed || onlyActive24h;
  const activeFilterCount = appSel.size + (onlySubscribed ? 1 : 0) + (onlyActive24h ? 1 : 0);

  const filteredChannels = useMemo(() => {
    const dayAgo = Date.now() - 24 * 3_600_000;
    return channels.filter((ch) => {
      if (query && !ch.displayName.toLowerCase().includes(query) && !ch.name.toLowerCase().includes(query)) return false;
      if (appSel.size > 0 && !appSel.has(ch.app)) return false;
      if (onlySubscribed && ch.subscriberCount === 0) return false;
      if (onlyActive24h && (ch.lastActivity === null || ch.lastActivity < dayAgo)) return false;
      return true;
    });
  }, [channels, query, appSel, onlySubscribed, onlyActive24h]);

  const clearFilters = () => {
    setSearch("");
    setAppSel(new Set());
    setOnlySubscribed(false);
    setOnlyActive24h(false);
  };

  const toggleApp = (key: string) => {
    setAppSel((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const openChannel = useCallback((name: string) => {
    navigate(`/apps/${name}`);
  }, [navigate]);

  const hasAnything = apps.length > 0 || channels.length > 0;

  /* ── Header slot: count summary · search · Filters chip ─────── */
  useHeaderSlot({
    title: hasAnything ? (
      <span className="text-xs text-mycel-text-2 tabular-nums truncate">
        {hasFilters
          ? `${String(filteredChannels.length)} of ${String(channels.length)} channels`
          : `${String(channels.length)} channel${channels.length === 1 ? "" : "s"} · ${String(apps.length)} app${apps.length === 1 ? "" : "s"}`}
      </span>
    ) : undefined,
    actions: hasAnything ? (
      <>
        <input
          type="text"
          value={search}
          onChange={(e) => { setSearch(e.target.value); }}
          placeholder="Search channels"
          className="flex-1 min-w-[96px] max-w-md h-9 px-3 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent"
          aria-label="Search channels"
        />
        <div className="relative shrink-0" ref={filtersRef}>
          <button
            type="button"
            onClick={() => { setFiltersOpen((v) => !v); }}
            aria-label="Filters"
            aria-haspopup="true"
            aria-expanded={filtersOpen}
            className={`inline-flex items-center gap-1.5 h-8 px-2.5 rounded-md border text-xs font-medium transition-colors ${
              filtersOpen || activeFilterCount > 0
                ? "border-mycel-accent text-mycel-text bg-mycel-surface"
                : "border-mycel-border bg-mycel-surface text-mycel-muted hover:text-mycel-text hover:border-mycel-accent"
            }`}
          >
            <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M1.5 2.5h11l-4.2 5v4l-2.6-1.5V7.5z" />
            </svg>
            Filters
            {activeFilterCount > 0 && (
              <span className="inline-flex items-center justify-center min-w-[16px] h-4 px-1 rounded-full bg-mycel-accent text-mycel-accent-fg text-[10px] font-semibold tabular-nums">
                {activeFilterCount}
              </span>
            )}
          </button>
          {filtersOpen && (
            <div
              data-testid="apps-filters-popover"
              className="absolute right-0 top-full mt-1.5 z-50 w-60 rounded-lg border border-mycel-border bg-mycel-surface-2 shadow-mycel-lg p-3 space-y-2.5 text-sm"
            >
              {apps.length > 1 && (
                <div>
                  <span className="block mb-1 text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">Apps</span>
                  <div className="space-y-1 max-h-40 overflow-y-auto">
                    {apps.map((app) => (
                      <label key={app.key} className="flex items-center gap-2 text-xs text-mycel-text-2 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={appSel.has(app.key)}
                          onChange={() => { toggleApp(app.key); }}
                          className="accent-[var(--mycel-accent)]"
                        />
                        <AppIcon base={app.base} size={11} />
                        <span className="truncate">{app.label}{app.botName ? ` · ${app.botName}` : ""}</span>
                      </label>
                    ))}
                  </div>
                </div>
              )}
              <label className="flex items-center gap-2 text-xs text-mycel-text-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={onlySubscribed}
                  onChange={() => { setOnlySubscribed((v) => !v); }}
                  className="accent-[var(--mycel-accent)]"
                />
                Has subscribers
              </label>
              <label className="flex items-center gap-2 text-xs text-mycel-text-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={onlyActive24h}
                  onChange={() => { setOnlyActive24h((v) => !v); }}
                  className="accent-[var(--mycel-accent)]"
                />
                Active in last 24h
              </label>
              {hasFilters && (
                <div className="pt-1.5 border-t border-mycel-border flex">
                  <button
                    type="button"
                    onClick={clearFilters}
                    className="ml-auto px-2 py-1.5 text-xs text-mycel-muted hover:text-mycel-text border border-mycel-border rounded-md focus-visible:ring-2 focus-visible:ring-mycel-accent focus-visible:ring-offset-1 focus-visible:ring-offset-mycel-bg"
                    aria-label="Clear filters"
                  >
                    Clear
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </>
    ) : undefined,
  });

  /* ── Loading / error states ─────────────────────────────────── */

  if (loading && !data) {
    return (
      <div className="p-6 space-y-4">
        <div className="h-20 animate-pulse rounded-lg bg-mycel-surface-hover" />
        <LoadingSkeleton variant="text" rows={6} />
      </div>
    );
  }
  if (timedOut && !data) {
    return (
      <div className="p-6">
        <EmptyState icon="!" title="Apps took too long to load" description="The server may be unavailable." actionLabel="Retry" onAction={refresh} />
      </div>
    );
  }
  if (error && !data) {
    return (
      <div className="p-6">
        <EmptyState icon="!" title="Failed to load apps" description={error} actionLabel="Retry" onAction={refresh} />
      </div>
    );
  }

  /* ── Empty state: nothing configured yet ────────────────────── */
  if (!hasAnything) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="max-w-lg text-center px-6">
          <div className="text-4xl mb-4 opacity-40">#</div>
          <h2 className="font-display text-xl font-semibold text-mycel-text mb-2">Connect your first app</h2>
          <p className="text-sm text-mycel-muted mb-6">
            Link Slack, Telegram, WhatsApp, Discord and more to start routing messages to your agents.
          </p>
          <button
            type="button"
            onClick={() => { setChooserOpen(true); }}
            className="inline-flex items-center h-8 px-3 rounded-md text-xs font-medium bg-mycel-accent text-mycel-accent-fg hover:bg-mycel-accent-hover shadow-mycel-sm transition-colors"
          >
            + Connect an app
          </button>
        </div>
        {chooserOpen && (
          <AppChooser
            onSelect={(key) => { setChooserOpen(false); setConnectAppId(key); }}
            onClose={() => { setChooserOpen(false); }}
          />
        )}
        {connectAppId && (
          <ConnectWizard appId={connectAppId} onClose={() => { setConnectAppId(null); }} onConnected={() => { refresh(); }} />
        )}
      </div>
    );
  }

  /* ── Channel grouping for render ────────────────────────────── */
  const channelsByApp = new Map<string, ChannelItem[]>();
  for (const ch of filteredChannels) {
    const list = channelsByApp.get(ch.app) ?? [];
    list.push(ch);
    channelsByApp.set(ch.app, list);
  }
  for (const list of channelsByApp.values()) {
    list.sort((a, b) => (b.lastActivity ?? 0) - (a.lastActivity ?? 0) || a.displayName.localeCompare(b.displayName));
  }
  const sectionApps = apps.filter((a) => (channelsByApp.get(a.key) ?? []).length > 0);

  return (
    <div className="p-6 pb-10 space-y-6">
        {/* ── Apps strip — compact pills ─────────────────────── */}
        <div className="flex flex-wrap gap-2">
          {apps.map((app) => {
            const ago = app.lastActivity !== null ? formatRelative(app.lastActivity) : null;
            const selected = appSel.has(app.key);
            const isError = app.status === "error";
            const name = app.botName ?? app.label;
            const statusLabel =
              app.status === "connected" ? "connected"
                : app.status === "connecting" ? "connecting"
                : app.status === "error" ? "disconnected"
                : "idle";
            // Icon carries the platform, so the label text is dropped — the
            // aria-label restores platform + name + status for screen readers.
            const aria = isError
              ? `${app.label}${app.botName ? ` (${app.botName})` : ""} — ${app.reason ?? "disconnected"}, reconnect`
              : `${app.label}${app.botName ? ` (${app.botName})` : ""} — ${statusLabel}${ago ? `, active ${ago}` : ""}`;
            return (
              <button
                key={app.key}
                type="button"
                data-testid={`app-pill-${app.key}`}
                onClick={() => { if (isError) { setConnectAppId(app.base); } else { toggleApp(app.key); } }}
                aria-label={aria}
                aria-pressed={selected}
                title={aria}
                className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm transition-colors ${
                  selected
                    ? "border-mycel-accent bg-mycel-surface-hover text-mycel-text"
                    : "border-mycel-border bg-mycel-surface hover:bg-mycel-surface-hover text-mycel-text"
                }`}
              >
                <StatusDot status={app.status} title={statusLabel} />
                <AppIcon base={app.base} size={15} />
                <span className="truncate max-w-[160px] font-medium">{name}</span>
                {isError ? (
                  <span className="truncate max-w-[180px] text-[12px] text-mycel-error">{app.reason}</span>
                ) : ago ? (
                  <span className="text-[12px] text-mycel-muted tabular-nums">{ago}</span>
                ) : null}
              </button>
            );
          })}
          <button
            type="button"
            onClick={() => { setChooserOpen(true); }}
            aria-label="Connect an app"
            className="inline-flex items-center gap-1.5 rounded-full border border-dashed border-mycel-border bg-mycel-surface px-3 py-1.5 text-sm text-mycel-muted hover:text-mycel-text hover:border-mycel-accent hover:bg-mycel-surface-hover transition-colors"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden>
              <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Connect
          </button>
        </div>

        {/* ── Notifications (primary) + channels (secondary) ─── */}
        <div className="grid gap-6 items-start lg:grid-cols-[minmax(0,1fr)_340px]">
          {/* Notifications — the primary stream: newest messages across every channel */}
          <aside className="rounded-lg border border-mycel-border bg-mycel-surface overflow-hidden" aria-label="Notifications">
            <Link
              to="/apps/activity"
              className="flex items-center gap-2 px-4 py-2.5 border-b border-mycel-border hover:bg-mycel-surface-hover transition-colors"
              aria-label="View all"
            >
              <h3 className="text-xs font-semibold uppercase tracking-[0.08em] text-mycel-text">Notifications</h3>
              <span className="ml-auto text-[11px] font-medium text-mycel-accent">View all →</span>
            </Link>
            <div className="divide-y divide-mycel-border">
              {recent.length === 0 && (
                <div className="px-4 py-10 text-center text-xs text-mycel-muted">No messages yet</div>
              )}
              {recent.map((m) => {
                const sender = cleanSender(m.sender);
                return (
                  <button
                    key={`${m.channel}:${String(m.id)}`}
                    type="button"
                    onClick={() => { openChannel(m.channel); }}
                    className="w-full text-left px-4 py-2.5 flex items-start gap-2.5 hover:bg-mycel-surface-hover transition-colors"
                  >
                    {/* Real chat participant — initials avatar, never an agent mushroom */}
                    <IdentityAvatar name={sender} size={26} className="mt-0.5" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5 mb-0.5 min-w-0">
                        <AppIcon base={sourcePlatform(m.channel)} size={11} />
                        <span className="truncate text-[11px] text-mycel-text-2">{channelLeaf(m.channel)}</span>
                        <span className="ml-auto shrink-0 text-[10.5px] text-mycel-muted tabular-nums">
                          {formatRelative(m.created_at)}
                        </span>
                      </div>
                      <div className="truncate text-[13px] text-mycel-text-2">
                        <span className="font-medium text-mycel-text">{sender}</span>{" "}
                        {oneLine(m.content)}
                      </div>
                    </div>
                  </button>
                );
              })}
            </div>
          </aside>

          {/* Channels — slim secondary column, grouped by app */}
          <div className="space-y-5 min-w-0">
            {sectionApps.length === 0 && (
              <div className="rounded-lg border border-dashed border-mycel-border bg-mycel-surface px-6 py-10 text-center">
                <p className="text-sm font-medium text-mycel-text-2">
                  {hasFilters ? "No chats match your filters" : "No chats yet"}
                </p>
                <p className="mt-1 text-xs text-mycel-muted">
                  {hasFilters ? "Try clearing a filter or search term." : "Chats appear here automatically as messages arrive."}
                </p>
              </div>
            )}
            {sectionApps.map((app) => {
              const chs = channelsByApp.get(app.key) ?? [];
              const groups = chs.filter((c) => c.kind === "group");
              const people = chs.filter((c) => c.kind === "person");
              const other = chs.filter((c) => c.kind === null);
              const split = app.base === "whatsapp" && (groups.length > 0 || people.length > 0);
              return (
                <section key={app.key} aria-label={`${app.label} channels`}>
                  <div className="flex items-center gap-2 mb-1.5">
                    {/* Icon conveys the platform — the redundant platform word
                        is dropped; the section aria-label keeps it for a11y. */}
                    <AppIcon base={app.base} size={13} />
                    <h3 className="text-[11px] font-medium uppercase tracking-[0.08em] text-mycel-muted">
                      <span className="sr-only">{app.label}</span>
                      {app.botName ? <span>{app.botName}</span> : null}
                    </h3>
                    <span className="text-[11px] text-mycel-muted tabular-nums">{chs.length}</span>
                  </div>
                  <div className="rounded-lg border border-mycel-border overflow-hidden divide-y divide-mycel-border">
                    {split ? (
                      <>
                        {groups.length > 0 && (
                          <>
                            <SubsectionLabel label="Group chats" count={groups.length} />
                            {groups.map((ch) => <ChannelRowButton key={ch.name} ch={ch} onOpen={openChannel} />)}
                          </>
                        )}
                        {people.length > 0 && (
                          <>
                            <SubsectionLabel label="People" count={people.length} />
                            {people.map((ch) => <ChannelRowButton key={ch.name} ch={ch} onOpen={openChannel} />)}
                          </>
                        )}
                        {other.length > 0 && (
                          <>
                            <SubsectionLabel label="Other" count={other.length} />
                            {other.map((ch) => <ChannelRowButton key={ch.name} ch={ch} onOpen={openChannel} />)}
                          </>
                        )}
                      </>
                    ) : (
                      chs.map((ch) => <ChannelRowButton key={ch.name} ch={ch} onOpen={openChannel} />)
                    )}
                  </div>
                </section>
              );
            })}
          </div>
        </div>

      {/* Custom Keys — encrypted vault keys agents reference via
          ${secret:NAME}; absorbed from the old standalone Secrets page. */}
      <CustomKeysSection />

      {/* Connect / reconnect flows — the catalog-driven setup */}
      {chooserOpen && (
        <AppChooser
          onSelect={(key) => { setChooserOpen(false); setConnectAppId(key); }}
          onClose={() => { setChooserOpen(false); }}
        />
      )}
      {connectAppId && (
        <ConnectWizard appId={connectAppId} onClose={() => { setConnectAppId(null); }} onConnected={() => { refresh(); }} />
      )}
    </div>
  );
}
