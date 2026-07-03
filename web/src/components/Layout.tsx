import { useCallback, useEffect, useState } from "react";
import { NavLink, Outlet, useLocation, useMatch } from "react-router-dom";
import { motion, AnimatePresence } from "framer-motion";
import { useTheme, THEME_LABELS } from "../context/ThemeContext";
import { useMediaQuery } from "../hooks/useMediaQuery";
import { CommandPalette } from "./CommandPalette";
import { api } from "../api/client";
import type { NotificationSource, GatewayHealth, GatewayStatus, NotifySubscription } from "../api/client";
import { sourcePlatform } from "./notifications/messageUtils";
import { SetupWizard, PlatformChooser, PLATFORM_MAP } from "./notifications/SetupWizard";
import { DefaultAppIcon, PLATFORM_ICON_MAP } from "./notifications/PlatformIcons";
import { Header } from "./Header";
import { SidebarToggle } from "./SidebarToggle";
import { HeaderSlotProvider, useHeaderSlotContext } from "../context/HeaderSlotContext";

const SIDEBAR_KEY = "bc-sidebar-collapsed";

/* ── Route transition wrapper ────────────────────────────────────────
   Subtle 120ms fade + 4px lift on every route change so navigating
   between sidebar tabs feels intentional rather than abrupt. Keyed
   on the route's first segment so deep links under the same view
   (Agent detail tabs, channel selection) do not retrigger the
   transition. Honors prefers-reduced-motion. */
function RouteTransition({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const segments = location.pathname.split("/").filter(Boolean).slice(0, 1);
  const key = "/" + segments.join("/");
  return (
    <AnimatePresence mode="wait" initial={false}>
      <motion.div
        key={key}
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -4 }}
        transition={{ duration: 0.12, ease: [0.4, 0, 0.2, 1] }}
        className="h-full"
      >
        {children}
      </motion.div>
    </AnimatePresence>
  );
}

/* ── Refined icons at 14px ──────────────────────────────────── */

function Icon({ name, size = 14 }: { name: string; size?: number }) {
  const s = String(size);
  const icons: Record<string, JSX.Element> = {
    live: <>
      <circle cx="7" cy="7" r="2" fill="currentColor" opacity="0.8" />
      <path d="M3 11A6 6 0 0111 3" strokeLinecap="round" opacity="0.4" />
    </>,
    agents: <path d="M7 3.5a2 2 0 100 4 2 2 0 000-4zM3.5 11.5c0-1.8 1.6-3 3.5-3s3.5 1.2 3.5 3" />,
    notifications: <><path d="M7 1.5a4 4 0 00-4 4v2.5l-1.5 2h11L11 8V5.5a4 4 0 00-4-4zM5.5 12a1.5 1.5 0 003 0" /></>,
    roles: <path d="M7 2.5l4.5 2.5v3.5L7 11 2.5 8.5V5z" />,
    templates: <><rect x="2.5" y="2.5" width="9" height="9" rx="1" /><path d="M5 5.5h4M5 7.5h4M5 9.5h2" opacity="0.5" /></>,
    tools: <path d="M9.5 2.5l3 3-7 7H2.5v-3z" />,
    cron: <><circle cx="7" cy="7" r="4.5" /><path d="M7 4.5v2.5l1.5 1.5" /></>,
    secrets: <path d="M7 2.5a2 2 0 00-2 2V6H4v4.5h6V6H9V4.5a2 2 0 00-2-2zm0 5.5a.75.75 0 110 1.5.75.75 0 010-1.5z" />,
    metrics: <path d="M2 10l2.5-3.5 2 1.5L10 3" strokeLinecap="round" strokeLinejoin="round" />,
    code: <><path d="M5 3.5L1.5 7L5 10.5" strokeLinecap="round" strokeLinejoin="round" /><path d="M9 3.5L12.5 7L9 10.5" strokeLinecap="round" strokeLinejoin="round" /></>,
    settings: <><circle cx="7" cy="7" r="2" /><path d="M7 1.5v1.5M7 11v1.5M1.5 7H3M11 7h1.5M3 3l1 1M10 10l1 1M3 11l1-1M10 4l1-1" opacity="0.5" /></>,
    chevron: <path d="M5 3l4 4-4 4" strokeLinecap="round" strokeLinejoin="round" />,
  };
  return (
    <svg width={s} height={s} viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3">
      {icons[name] ?? <rect x="3" y="3" width="8" height="8" />}
    </svg>
  );
}

/* ── Platform config ─────────────────────────────────────────── */

function getPlatformMeta(p: string) {
  // Handle compound keys like "telegram:gateway" — look up base platform
  const base = p.includes(":") ? (p.split(":")[0] ?? p) : p;
  const def = PLATFORM_MAP[base];
  const IconComponent = PLATFORM_ICON_MAP[base] ?? DefaultAppIcon;
  if (def) return { base, label: def.label, color: def.color, IconComponent };
  return { base, label: p, color: "#8c7e72", IconComponent };
}

/** Extract display channel name (last segment after platform and optional server). */
function displaySourceName(name: string): string {
  // "discord:Server Name:general" → "general"
  // "slack:engineering" → "engineering"
  const parts = name.split(":");
  return parts[parts.length - 1] || name;
}

/** Extract the group identifier (server/workspace/bot) from a channel name. */
function sourceGroup(name: string): string | null {
  // "discord:Server Name:general" → "Server Name"
  // "slack:engineering" → null (no sub-group)
  const parts = name.split(":");
  if (parts.length >= 3) return parts.slice(1, -1).join(":");
  return null;
}

/* ── Channel list — guild sub-groups, unread accents, row cap ── */

/** Max channel rows shown per app before the "N more" expander. */
const APP_CHANNEL_CAP = 8;
/** Total channel count above which the drawer filter input appears. */
const CHANNEL_FILTER_THRESHOLD = 15;

function ChannelRow({
  ch,
  count,
  unread,
  prefix,
  onView,
}: {
  ch: NotificationSource;
  count: number;
  unread: boolean;
  prefix: string;
  onView: (name: string) => void;
}) {
  const chName = displaySourceName(ch.name);
  // Badge: agents subscribed when any; otherwise fall back to the source's
  // member count so channels with data still read as populated.
  const badge = count > 0 ? count : ch.member_count > 0 ? ch.member_count : 0;
  return (
    <NavLink
      to={`${prefix}/notifications/${ch.name}`}
      className="block"
      title={count > 0 ? `${ch.name} · ${count} subscribed` : ch.name}
      onClick={() => onView(ch.name)}
      style={({ isActive }: { isActive: boolean }) => ({
        display: "flex",
        alignItems: "center",
        gap: 8,
        height: 24,
        padding: "0 8px",
        borderRadius: 5,
        fontSize: 12.5,
        color: isActive || unread ? "var(--mycel-text)" : count > 0 ? "var(--mycel-text-2)" : "var(--mycel-muted)",
        background: isActive ? "color-mix(in srgb, var(--mycel-accent) 14%, transparent)" : "transparent",
        fontWeight: isActive ? 600 : unread || count > 0 ? 500 : 400,
        cursor: "pointer",
        marginBottom: 1,
        textDecoration: "none",
      })}
    >
      <span
        style={{
          width: 12,
          color: unread ? "var(--mycel-accent)" : "var(--mycel-muted)",
          fontFamily: "'JetBrains Mono', monospace",
          fontSize: 12,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
        }}
      >
        #
      </span>
      <span style={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
        {chName}
      </span>
      {unread && (
        <span
          className="shrink-0"
          style={{ width: 4, height: 4, borderRadius: 999, background: "var(--mycel-accent)" }}
        />
      )}
      {badge > 0 && (
        <span
          style={{
            fontSize: 10.5,
            fontWeight: 600,
            color: "var(--mycel-muted)",
            fontFamily: "'JetBrains Mono', monospace",
            padding: "1px 5px",
            borderRadius: 999,
            background: "var(--mycel-surface)",
          }}
        >
          {badge}
        </span>
      )}
    </NavLink>
  );
}

function ChannelList({
  channels,
  subCountMap,
  prefix,
  appLastMsg,
  viewedMap,
  onView,
  filtering,
}: {
  channels: NotificationSource[];
  subCountMap: Map<string, number>;
  prefix: string;
  appLastMsg: number | null;
  viewedMap: Record<string, number>;
  onView: (name: string) => void;
  filtering: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const showAll = expanded || filtering;

  if (channels.length === 0) {
    return (
      <div style={{ padding: "3px 8px 5px", fontSize: 11, color: "var(--mycel-muted)", fontStyle: "italic" }}>
        No channels yet
      </div>
    );
  }

  // Sub-group by server/guild — canonical keys are "discord:<guild>:<channel>".
  const groupOrder: (string | null)[] = [];
  const grouped = new Map<string | null, NotificationSource[]>();
  for (const ch of channels) {
    const g = sourceGroup(ch.name);
    const list = grouped.get(g);
    if (list) {
      list.push(ch);
    } else {
      grouped.set(g, [ch]);
      groupOrder.push(g);
    }
  }
  groupOrder.sort((a, b) => (a ?? "").localeCompare(b ?? ""));
  for (const list of grouped.values()) {
    list.sort((a, b) => displaySourceName(a.name).localeCompare(displaySourceName(b.name)));
  }
  // Guild labels only earn a row when there is more than one group —
  // a single server/bot is already named in the app header.
  const showGroupLabels = groupOrder.length > 1;

  const cap = showAll ? channels.length : APP_CHANNEL_CAP;
  let rendered = 0;
  const rows: JSX.Element[] = [];
  for (const g of groupOrder) {
    if (rendered >= cap) break;
    const items = (grouped.get(g) ?? []).slice(0, cap - rendered);
    if (items.length === 0) continue;
    if (showGroupLabels && g !== null) {
      rows.push(
        <div
          key={`group:${g}`}
          className="truncate"
          style={{
            padding: "4px 8px 1px",
            fontSize: 10,
            color: "var(--mycel-muted)",
            textTransform: "uppercase",
            letterSpacing: 0.6,
            fontWeight: 600,
          }}
        >
          {g}
        </div>,
      );
    }
    for (const ch of items) {
      const viewedAt = viewedMap[ch.name];
      // Unread accent: app-level last activity newer than this channel's
      // last-viewed timestamp. Channels never opened only light up for
      // activity within the past hour so a fresh browser doesn't glow all over.
      const unread =
        appLastMsg !== null &&
        appLastMsg > (viewedAt ?? 0) &&
        (viewedAt !== undefined || Date.now() - appLastMsg < 3_600_000);
      rows.push(
        <ChannelRow
          key={ch.name}
          ch={ch}
          count={subCountMap.get(ch.name) ?? 0}
          unread={unread}
          prefix={prefix}
          onView={onView}
        />,
      );
      rendered++;
    }
  }

  const hidden = channels.length - rendered;

  return (
    <>
      {rows}
      {!filtering && (hidden > 0 || expanded) && (
        <button
          type="button"
          onClick={() => setExpanded((prev) => !prev)}
          style={{
            display: "block",
            width: "100%",
            padding: "3px 8px",
            fontSize: 11,
            color: "var(--mycel-muted)",
            background: "none",
            border: "none",
            cursor: "pointer",
            textAlign: "left",
          }}
        >
          {expanded ? "less" : `${hidden} more`}
        </button>
      )}
    </>
  );
}

/* ── Drawer state + gateway status helpers ───────────────────── */

const NOTIF_COLLAPSED_KEY = "mycel-notif-collapsed-apps";
const NOTIF_VIEWED_KEY = "mycel-notif-last-viewed";

function readCollapsedApps(): Set<string> {
  try {
    const raw = localStorage.getItem(NOTIF_COLLAPSED_KEY);
    const arr: unknown = raw ? JSON.parse(raw) : null;
    return new Set(Array.isArray(arr) ? arr.filter((v): v is string => typeof v === "string") : []);
  } catch {
    return new Set();
  }
}

function writeCollapsedApps(s: Set<string>) {
  try { localStorage.setItem(NOTIF_COLLAPSED_KEY, JSON.stringify([...s])); } catch { /* */ }
}

function readViewedMap(): Record<string, number> {
  try {
    const raw = localStorage.getItem(NOTIF_VIEWED_KEY);
    const obj: unknown = raw ? JSON.parse(raw) : null;
    return obj && typeof obj === "object" && !Array.isArray(obj) ? (obj as Record<string, number>) : {};
  } catch {
    return {};
  }
}

// Parse an ISO timestamp, guarding zero / unset / pre-2001 values that
// produce nonsensical "17753690h ago" strings.
function parseActivityTs(iso?: string): number | null {
  if (!iso) return null;
  const ts = new Date(iso).getTime();
  return Number.isFinite(ts) && ts > 978307200000 ? ts : null;
}

/** Compact relative time: "now", "5m", "3h", "2d". */
function formatAgoShort(ts: number): string {
  const mins = Math.floor((Date.now() - ts) / 60000);
  if (mins < 1) return "now";
  if (mins < 60) return `${mins}m`;
  if (mins < 1440) return `${Math.floor(mins / 60)}h`;
  return `${Math.floor(mins / 1440)}d`;
}

type AppStatus = "connected" | "connecting" | "error" | "idle";

function getAppStatus(gw?: GatewayStatus, h?: GatewayHealth): AppStatus {
  if (h) {
    if (h.connected) return "connected";
    return h.status === "connecting" ? "connecting" : "error";
  }
  // Enabled gateway whose health has not reported yet reads as connecting.
  if (gw?.enabled) return "connecting";
  return "idle";
}

/** Map raw gateway errors to a short human-readable reason. */
function disconnectReason(base: string, h?: GatewayHealth): string {
  if (base === "whatsapp") return "Scan QR to re-pair";
  const err = h?.error ?? "";
  if (/\b402\b|payment required|quota/i.test(err)) return "API quota/payment required";
  if (/\b401\b|unauthorized|invalid[ _-]?(auth|token|credentials)/i.test(err)) return "Invalid credentials";
  if (/\b403\b|forbidden/i.test(err)) return "Access denied";
  if (/\b429\b|rate[ _-]?limit/i.test(err)) return "Rate limited";
  if (/timeout|timed out|refused|unreachable|network|no such host|dns/i.test(err)) return "Connection failed";
  return err || "Disconnected";
}

const STATUS_DOT_TOKEN: Record<AppStatus, string> = {
  connected: "var(--mycel-success)",
  connecting: "var(--mycel-warning)",
  error: "var(--mycel-error)",
  idle: "var(--mycel-muted)",
};

/** 6px connection dot — the only status signal a healthy app shows. */
function StatusDot({ status, title }: { status: AppStatus; title?: string }) {
  const token = STATUS_DOT_TOKEN[status];
  return (
    <span
      className="shrink-0"
      title={title}
      style={{
        width: 6,
        height: 6,
        borderRadius: 999,
        background: token,
        opacity: status === "idle" ? 0.35 : 1,
        boxShadow: status === "connected" ? `0 0 5px color-mix(in srgb, ${token} 50%, transparent)` : "none",
      }}
    />
  );
}

/* ── Notification tree (inline in nav) ───────────────────────── */

function NotificationNavTree() {
  const [sources, setSources] = useState<NotificationSource[]>([]);
  const [gateways, setGateways] = useState<GatewayStatus[]>([]);
  const [subs, setSubs] = useState<NotifySubscription[]>([]);
  const [health, setHealth] = useState<Map<string, GatewayHealth>>(new Map());
  const [collapsedApps, setCollapsedApps] = useState<Set<string>>(readCollapsedApps);
  const [viewedMap, setViewedMap] = useState<Record<string, number>>(readViewedMap);
  const [filter, setFilter] = useState("");
  const [setupPlatform, setSetupPlatform] = useState<string | null>(null);
  const [showConnectMenu, setShowConnectMenu] = useState(false);

  const fetchData = useCallback(async () => {
    try {
      const [chs, gws, subList] = await Promise.all([
        api.listNotificationSources().catch(() => [] as NotificationSource[]),
        api.listGateways().catch(() => [] as GatewayStatus[]),
        api.listSubscriptions().catch(() => [] as NotifySubscription[]),
      ]);
      setSources(chs ?? []);
      setGateways(gws ?? []);
      setSubs(subList ?? []);

      // Fetch health for each enabled gateway, keyed by the gateway's own
      // platform key so compound keys ("telegram:trade_research") stay stable.
      const enabledGws = (gws ?? []).filter((g) => g.enabled);
      const healthEntries = await Promise.all(
        enabledGws.map(async (g) => {
          const h = await api.getGatewayHealth(g.platform).catch(() => null);
          return h ? ([g.platform, h] as const) : null;
        }),
      );
      const hmap = new Map<string, GatewayHealth>();
      for (const entry of healthEntries) {
        if (entry) hmap.set(entry[0], entry[1]);
      }
      setHealth(hmap);
    } catch { /* */ }
  }, []);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => void fetchData(), 12000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const toggleApp = (p: string) => {
    setCollapsedApps((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p); else next.add(p);
      writeCollapsedApps(next);
      return next;
    });
  };

  const markViewed = useCallback((name: string) => {
    setViewedMap((prev) => {
      const next = { ...prev, [name]: Date.now() };
      try { localStorage.setItem(NOTIF_VIEWED_KEY, JSON.stringify(next)); } catch { /* */ }
      return next;
    });
  }, []);

  const subCountMap = new Map<string, number>();
  for (const sub of subs) subCountMap.set(sub.channel, (subCountMap.get(sub.channel) ?? 0) + 1);

  const gwMap = new Map<string, GatewayStatus>();
  for (const gw of gateways) gwMap.set(gw.platform, gw);

  // Group channels by their gateway platform key (e.g., "telegram:gateway", "telegram:trade_research")
  // so each connected bot/server gets its own sidebar section.
  const bucketMap = new Map<string, NotificationSource[]>();
  for (const src of sources) {
    // Match source to its gateway entry (e.g., "telegram:gateway:marketing" → gateway "telegram:gateway")
    const matchedGw = gateways.find((gw) => src.name.startsWith(gw.platform + ":"));
    const key = matchedGw?.platform ?? sourcePlatform(src.name);
    if (key === "internal") continue;
    const list = bucketMap.get(key) ?? [];
    list.push(src);
    bucketMap.set(key, list);
  }
  for (const gw of gateways) {
    if (!bucketMap.has(gw.platform)) bucketMap.set(gw.platform, []);
  }

  let totalChannels = 0;
  for (const list of bucketMap.values()) totalChannels += list.length;

  const query = filter.trim().toLowerCase();
  const filtering = query.length > 0;

  // One visual system per app: brand icon + status dot, stable sort —
  // connected apps first, then by platform name.
  const apps = [...bucketMap.entries()]
    .map(([platform, chs]) => {
      const meta = getPlatformMeta(platform);
      const gwStatus = gwMap.get(platform);
      const gwHealth = health.get(platform);
      const status = getAppStatus(gwStatus, gwHealth);
      const visibleChs = filtering
        ? chs.filter((c) => c.name.toLowerCase().includes(query))
        : chs;
      return { platform, chs, meta, gwStatus, gwHealth, status, visibleChs };
    })
    .filter((app) => !filtering || app.visibleChs.length > 0)
    .sort((a, b) => {
      const ra = a.status === "connected" ? 0 : 1;
      const rb = b.status === "connected" ? 0 : 1;
      if (ra !== rb) return ra - rb;
      return a.meta.label.localeCompare(b.meta.label) || a.platform.localeCompare(b.platform);
    });

  const prefix = "";

  return (
    <div
      style={{
        paddingLeft: 10,
        marginLeft: 9,
        borderLeft: "1px solid var(--mycel-border, rgba(255,255,255,0.08))",
        marginTop: 2,
        marginBottom: 4,
        maxHeight: 320,
        overflowY: "auto",
      }}
    >
      {/* Channel filter — only earns its place when the tree is long */}
      {totalChannels > CHANNEL_FILTER_THRESHOLD && (
        <div style={{ position: "relative", margin: "4px 4px 3px" }}>
          <svg
            width="10"
            height="10"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.2"
            style={{
              position: "absolute",
              left: 8,
              top: "50%",
              transform: "translateY(-50%)",
              color: "var(--mycel-muted)",
              pointerEvents: "none",
            }}
          >
            <circle cx="11" cy="11" r="7" />
            <line x1="16.5" y1="16.5" x2="21" y2="21" strokeLinecap="round" />
          </svg>
          <input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter channels"
            aria-label="Filter channels"
            style={{
              width: "100%",
              height: 24,
              padding: "0 8px 0 24px",
              fontSize: 11.5,
              color: "var(--mycel-text)",
              background: "var(--mycel-surface)",
              border: "1px solid var(--mycel-border)",
              borderRadius: 5,
              outline: "none",
            }}
          />
        </div>
      )}

      {filtering && apps.length === 0 && (
        <div style={{ padding: "4px 8px", fontSize: 11, color: "var(--mycel-muted)", fontStyle: "italic" }}>
          No matching channels
        </div>
      )}

      {apps.map(({ platform, chs, meta, gwStatus, gwHealth, status, visibleChs }) => {
        const isCollapsed = collapsedApps.has(platform) && !filtering;
        const lastMsg = parseActivityTs(gwHealth?.last_message_at);
        const ago = status === "connected" && lastMsg ? formatAgoShort(lastMsg) : null;
        const tooltip =
          status === "connected"
            ? lastMsg
              ? `Connected · last message ${ago === "now" ? "just now" : `${ago ?? ""} ago`}`
              : "Connected"
            : status === "connecting"
              ? "Connecting…"
              : status === "error"
                ? `Disconnected${gwHealth?.error ? ": " + gwHealth.error : ""}`
                : "Not connected";
        const AppGlyph = meta.IconComponent;

        return (
          <div key={platform}>
            {/* App header — brand icon + name, dot is the only healthy signal */}
            <button
              type="button"
              onClick={() => toggleApp(platform)}
              className="w-full flex items-center"
              style={{
                gap: 6,
                padding: "5px 8px 2px",
                fontSize: 11.5,
                color: "var(--mycel-text-2)",
                fontWeight: 500,
                background: "none",
                border: "none",
                cursor: "pointer",
              }}
            >
              <AppGlyph size={12} />
              {(() => {
                const subLabel = gwStatus?.bot_name || (chs.length > 0 ? sourceGroup(chs[0]?.name ?? "") : null);
                // When a bot/server name is present, show platform + bot for clarity
                // (e.g., "Slack · bc_gateway"); otherwise just the platform label.
                if (subLabel && subLabel !== meta.label) {
                  return (
                    <span className="truncate flex items-baseline" style={{ gap: 5, minWidth: 0 }}>
                      <span style={{ flexShrink: 0 }}>{meta.label}</span>
                      <span style={{ color: "var(--mycel-muted)", fontSize: 10.5 }}>·</span>
                      <span className="truncate" style={{ minWidth: 0 }}>{subLabel}</span>
                    </span>
                  );
                }
                return <span className="truncate">{meta.label}</span>;
              })()}
              <span className="ml-auto shrink-0 flex items-center" style={{ gap: 6 }}>
                {isCollapsed && chs.length > 0 && (
                  <span
                    style={{
                      fontSize: 10,
                      fontWeight: 600,
                      color: "var(--mycel-muted)",
                      fontFamily: "'JetBrains Mono', monospace",
                    }}
                  >
                    {chs.length}
                  </span>
                )}
                {!isCollapsed && ago && (
                  <span style={{ fontSize: 10, color: "var(--mycel-muted)", fontVariantNumeric: "tabular-nums" }}>
                    {ago}
                  </span>
                )}
                <StatusDot status={status} title={tooltip} />
                <svg
                  width="10"
                  height="10"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  style={{ opacity: 0.4, transform: isCollapsed ? "rotate(-90deg)" : "rotate(0deg)", transition: "transform 0.15s" }}
                >
                  <polyline points="6 9 12 15 18 9" />
                </svg>
              </span>
            </button>

            {/* Disconnected apps get an action row, not a chip */}
            {status === "error" && (
              <button
                type="button"
                onClick={() => setSetupPlatform(meta.base)}
                className="w-full flex items-center"
                title={gwHealth?.error || undefined}
                style={{
                  gap: 5,
                  padding: "1px 8px 3px 26px",
                  fontSize: 11,
                  color: "var(--mycel-error)",
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  textAlign: "left",
                }}
              >
                <svg width="10" height="10" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="shrink-0">
                  <path d="M7 1.5l6 11H1z" strokeLinejoin="round" />
                  <path d="M7 6v3M7 10.8v.01" strokeLinecap="round" />
                </svg>
                <span className="truncate">{disconnectReason(meta.base, gwHealth)}</span>
                <span style={{ marginLeft: "auto", flexShrink: 0, fontSize: 10, opacity: 0.7 }}>→</span>
              </button>
            )}

            {/* Channel rows */}
            {!isCollapsed && (
              <ChannelList
                channels={filtering ? visibleChs : chs}
                subCountMap={subCountMap}
                prefix={prefix}
                appLastMsg={lastMsg}
                viewedMap={viewedMap}
                onView={markViewed}
                filtering={filtering}
              />
            )}
          </div>
        );
      })}

      {/* Connect app */}
      <button
        type="button"
        onClick={() => setShowConnectMenu(true)}
        className="w-full flex items-center"
        style={{
          gap: 8,
          height: 26,
          padding: "0 8px",
          marginTop: 4,
          borderRadius: 5,
          fontSize: 12,
          color: "var(--mycel-muted)",
          cursor: "pointer",
          border: "1px dashed var(--mycel-border)",
          background: "none",
          whiteSpace: "nowrap",
        }}
      >
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
        </svg>
        <span>Connect app</span>
      </button>

      {showConnectMenu && (
        <PlatformChooser
          onSelect={(key) => { setShowConnectMenu(false); setSetupPlatform(key); }}
          onClose={() => setShowConnectMenu(false)}
        />
      )}

      {setupPlatform && setupPlatform !== "_choose" && (
        <SetupWizard platform={setupPlatform} onClose={() => setSetupPlatform(null)} onConnected={() => void fetchData()} />
      )}
    </div>
  );
}

/* ── Nav items ───────────────────────────────────────────────── */

// Primary nav — workspace-scoped surfaces + one cross-workspace surface
// (Costs). Section dividers previously wrapped a single item each, which
// design + UX audit flagged as chrome-for-nothing (#3205 P1a). Settings
// moved to the footer next to About + the theme toggle so all utility
// affordances read as one row of chrome rather than three separate
// nav groups.
const MAIN_NAV_ITEMS = [
  { to: "/live", label: "Live", icon: "live" },
  { to: "/agents", label: "Agents", icon: "agents" },
  { to: "/notifications", label: "Notifications", icon: "notifications" },
  { to: "/code", label: "Code", icon: "code" },
  { to: "/templates", label: "Templates", icon: "templates" },
  { to: "/tools", label: "Tools", icon: "tools" },
  { to: "/cron", label: "Cron", icon: "cron" },
  { to: "/secrets", label: "Secrets", icon: "secrets" },
  { to: "/stats", label: "Metrics", icon: "metrics" },
  { to: "/costs", label: "Costs", icon: "metrics" },
] as const;

// Retained so external callers (TITLE_ITEMS, /settings header) that still
// walk a flat nav list keep working. Settings + Costs both appear here
// even though they're rendered elsewhere in the sidebar chrome.
const UTIL_NAV_ITEMS = [
  { to: "/settings", label: "Settings", icon: "settings" },
] as const;

const NAV_ITEMS = [...MAIN_NAV_ITEMS, ...UTIL_NAV_ITEMS];

/**
 * TITLE_ITEMS — extends NAV_ITEMS with surfaces that resolve to a
 * document title but live outside the sidebar nav lists (e.g. /about
 * sits in the sidebar footer next to the theme toggle, not in any
 * NavList). Keep this list in sync with non-nav top-level routes.
 */
const TITLE_ITEMS = [...NAV_ITEMS, { to: "/about", label: "About" }];

function readCollapsed(): boolean {
  try { return localStorage.getItem(SIDEBAR_KEY) === "true"; } catch { return false; }
}
function writeCollapsed(v: boolean) {
  try { localStorage.setItem(SIDEBAR_KEY, String(v)); } catch { /* */ }
}

/* ── Nav list ────────────────────────────────────────────────── */

function NavList({
  items,
  collapsed,
  isMobile,
  notificationsExpanded,
  onToggleNotifications,
}: {
  items: ReadonlyArray<{ to: string; label: string; icon: string; global?: boolean }>;
  collapsed: boolean;
  isMobile: boolean;
  notificationsExpanded?: boolean;
  onToggleNotifications?: () => void;
}) {
  const isIconOnly = collapsed && !isMobile;
  const showTree = !isIconOnly && notificationsExpanded;

  return (
    <>
      {items.map(({ to, label, icon }) => {
        const isNotifications = label === "Notifications";
        const scopedTo = to;
        return (
          <li key={to}>
            <NavLink
              to={scopedTo}
              end={!isNotifications}
              title={isIconOnly ? label : undefined}
              className={({ isActive }) =>
                `relative flex items-center gap-2.5 ${isIconOnly ? "justify-center px-2" : "pl-4 pr-3"} py-[7px] text-[13px] outline-none transition-colors duration-75 ${
                  isActive
                    ? "text-mycel-accent font-medium border-l-2 border-mycel-accent bg-mycel-surface-hover"
                    : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l-2 border-transparent"
                }`
              }
            >
              <span className="shrink-0 flex items-center justify-center w-4 opacity-60">
                <Icon name={icon} size={14} />
              </span>
              {(!collapsed || isMobile) && (
                <span className="truncate">{label}</span>
              )}
              {label === "Live" && (
                <span className="w-1.5 h-1.5 rounded-full bg-mycel-live animate-pulse ml-auto" />
              )}
              {isNotifications && !isIconOnly && onToggleNotifications && (
                <button
                  type="button"
                  onClick={(e) => { e.preventDefault(); e.stopPropagation(); onToggleNotifications(); }}
                  className="ml-auto shrink-0 p-0.5 rounded text-mycel-muted hover:text-mycel-muted/70 transition-all"
                  aria-label={notificationsExpanded ? "Collapse channels" : "Expand channels"}
                >
                  <svg
                    width="12" height="12" viewBox="0 0 14 14" fill="none"
                    stroke="currentColor" strokeWidth="1.5"
                    style={{
                      transform: notificationsExpanded ? "rotate(90deg)" : "rotate(0deg)",
                      transition: "transform 150ms ease",
                    }}
                  >
                    <path d="M5 3l4 4-4 4" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </button>
              )}
            </NavLink>
            {isNotifications && showTree && <NotificationNavTree />}
          </li>
        );
      })}
    </>
  );
}

/* ── Degraded services banner ────────────────────────────────────────
   Slim amber strip shown when /api/health reports degraded services
   (stores that failed to initialize at daemon boot — notify, cron,
   secrets, …). One line, service names only; full reasons live in the
   hover tooltip and `mycel doctor`. Dismissible for the session. */
export function DegradedBanner() {
  const [degraded, setDegraded] = useState<Record<string, string> | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    fetch("/api/health")
      .then((r) => r.json())
      .then((d) => {
        if (d?.status === "degraded" && d.degraded && Object.keys(d.degraded).length > 0) {
          setDegraded(d.degraded as Record<string, string>);
        }
      })
      .catch(() => {});
  }, []);

  if (!degraded || dismissed) return null;
  const names = Object.keys(degraded).sort().join(", ");
  const detail = Object.entries(degraded)
    .map(([name, reason]) => `${name}: ${reason}`)
    .join("\n");
  return (
    <div
      role="status"
      title={detail}
      className="flex items-center gap-2 px-3 py-1.5 text-[11px] bg-mycel-warning/10 border-b border-mycel-warning/30 text-mycel-warning"
    >
      <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" className="shrink-0">
        <path d="M7 1.5l6 11H1z" strokeLinejoin="round" />
        <path d="M7 6v3M7 10.8v.01" strokeLinecap="round" />
      </svg>
      <span className="truncate">
        Degraded services: <span className="font-medium">{names}</span> — some features are unavailable, run mycel doctor for details
      </span>
      <button
        type="button"
        onClick={() => setDismissed(true)}
        className="ml-auto shrink-0 p-0.5 rounded text-mycel-warning/70 hover:text-mycel-warning transition-colors"
        aria-label="Dismiss degraded services banner"
      >
        <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M3 3l8 8M11 3l-8 8" />
        </svg>
      </button>
    </div>
  );
}

/* ── Layout ──────────────────────────────────────────────────── */

export function Layout() {
  const location = useLocation();
  const { mode, toggle } = useTheme();
  const isMobile = useMediaQuery("(max-width: 767px)");

  const [userName, setUserName] = useState("");
  useEffect(() => {
    fetch("/api/settings").then(r => r.json()).then(d => {
      setUserName(d?.user?.name || "");
    }).catch(() => {});
  }, []);

  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(readCollapsed);

  // Notification tree collapse state — defaults to expanded when on the
  // Notifications route, but the user can toggle it by clicking the nav item.
  const notificationsRoute = useMatch("/notifications/*");
  const onNotifRoute = Boolean(notificationsRoute);
  const [notifManualToggle, setNotifManualToggle] = useState<boolean | null>(null);
  // Auto-expand when navigating to notifications, but respect manual toggle
  const notificationsExpanded = notifManualToggle !== null ? notifManualToggle : onNotifRoute;
  const toggleNotifications = useCallback(() => {
    setNotifManualToggle((prev) => !(prev !== null ? prev : onNotifRoute));
  }, [onNotifRoute]);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => { const next = !prev; writeCollapsed(next); return next; });
  }, []);

  useEffect(() => { if (isMobile) setCollapsed(true); }, [isMobile]);
  useEffect(() => {
    // Compare ONLY the first URL segment so sub-routes such as
    // /agents/<name>/live keep their parent ("Agents") title rather than
    // incorrectly resolving to a same-named top-level tab ("Live").
    const firstSeg = location.pathname.replace(/^\//, "").split("/")[0] ?? "";
    const match = TITLE_ITEMS.find((item) => {
      const seg = item.to.replace(/^\//, "");
      return seg === firstSeg;
    });
    document.title = match ? `${match.label} \u2014 mycel` : "mycel";
  }, [location.pathname]);
  useEffect(() => { setMobileOpen(false); }, [location.pathname]);

  const sidebarWidth = collapsed && !isMobile ? "w-14" : "w-48";

  return (
    <div className="flex h-screen">
      {/* Mobile hamburger */}
      <button type="button" onClick={() => setMobileOpen(true)}
        className="fixed top-3 left-3 z-40 md:hidden p-2 rounded border border-mycel-border bg-mycel-surface text-mycel-muted hover:text-mycel-text transition-colors"
        aria-label="Open navigation"
      >
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M3 5h12M3 9h12M3 13h12" />
        </svg>
      </button>

      {mobileOpen && <div className="fixed inset-0 z-40 bg-mycel-overlay md:hidden" onClick={() => setMobileOpen(false)} />}

      {/* Sidebar */}
      <nav
        className={`fixed inset-y-0 left-0 z-50 ${sidebarWidth} shrink-0 border-r border-mycel-border/50 bg-mycel-surface shadow-mycel flex flex-col transition-all duration-200 md:relative md:translate-x-0 ${
          isMobile ? (mobileOpen ? "translate-x-0 w-48" : "-translate-x-full") : ""
        }`}
        style={{ scrollbarWidth: "thin", scrollbarColor: "var(--mycel-scrollbar-thumb) transparent" }}
      >
        {/* Header — heights kept in sync with Header.tsx compact mode (48px)
            so the drawer top-line aligns pixel-perfect with the main pane
            header across the fold. */}
        <div className="px-3 min-h-[48px] border-b border-mycel-border/40 flex items-center justify-between">
          {(!collapsed || isMobile) ? (
            <div className="flex items-center gap-2 overflow-hidden">
              <span
                className="w-6 h-6 shrink-0 flex items-center justify-center font-bold"
                style={{
                  borderRadius: 7,
                  background: "var(--mycel-accent)",
                  color: "#0d0d0d",
                  fontSize: 14,
                  fontFamily: "'JetBrains Mono', monospace",
                  letterSpacing: -0.5,
                }}
              >
                m
              </span>
              <div className="min-w-0">
                <p className="text-[13px] font-semibold text-mycel-text truncate" style={{ letterSpacing: -0.1 }}>
                  {userName ? (userName.startsWith("@") ? userName : `@${userName}`) : "mycel"}
                </p>
              </div>
            </div>
          ) : (
            <span
              className="w-6 h-6 shrink-0 flex items-center justify-center font-bold"
              style={{
                borderRadius: 7,
                background: "var(--mycel-accent)",
                color: "var(--mycel-accent-fg)",
                fontSize: 14,
                fontFamily: "'JetBrains Mono', monospace",
                letterSpacing: -0.5,
              }}
            >
              m
            </span>
          )}
          {isMobile ? (
            <button type="button" onClick={() => setMobileOpen(false)}
              className="p-0.5 rounded text-mycel-muted hover:text-mycel-text transition-colors" aria-label="Close navigation"
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M3 3l8 8M11 3l-8 8" />
              </svg>
            </button>
          ) : (
            <button type="button" onClick={toggleCollapsed}
              className="p-0.5 rounded text-mycel-muted/30 hover:text-mycel-muted/70 transition-colors"
              aria-label={collapsed ? "Expand navigation" : "Collapse navigation"}
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
                {collapsed ? <path d="M5 3l4 4-4 4" /> : <path d="M9 3l-4 4 4 4" />}
              </svg>
            </button>
          )}
        </div>

        {/* Nav — sectioned. Labeled captions replace the anonymous
            dividers so the grouping (workspace items vs global items vs
            system items) is explicit; the captions collapse to a bare
            hairline when the sidebar is icon-only. */}
        <ul className="flex-1 py-2 overflow-y-auto" style={{ scrollbarWidth: "thin" }}>
          <NavList
            items={MAIN_NAV_ITEMS}
            collapsed={collapsed}
            isMobile={isMobile}
            notificationsExpanded={notificationsExpanded}
            onToggleNotifications={toggleNotifications}
          />
        </ul>

        {/* Unified sidebar footer — About + Theme picker in a single
            row so the two utility affordances read as related chrome,
            not two separate nav items competing for attention. The
            About link takes the primary space (users care about
            version); the theme toggle is a compact icon button on the
            right. Collapsed sidebar falls back to two stacked
            icon-only rows so both remain reachable. */}
        {collapsed && !isMobile ? (
          <div className="border-t border-mycel-border/40 flex flex-col">
            <NavLink
              to="/settings"
              className={({ isActive }) =>
                `flex items-center justify-center px-2 py-[9px] ${isActive ? "text-mycel-accent bg-mycel-surface-hover border-l-2 border-mycel-accent" : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l-2 border-transparent"} transition-colors`
              }
              title="Settings"
            >
              <Icon name="settings" size={14} />
            </NavLink>
            <NavLink
              to="/about"
              className={({ isActive }) =>
                `flex items-center justify-center px-2 py-[9px] ${isActive ? "text-mycel-accent bg-mycel-surface-hover border-l-2 border-mycel-accent" : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l-2 border-transparent"} transition-colors`
              }
              title="About / version"
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
                <circle cx="7" cy="7" r="5.5" />
                <path d="M7 4.5v.01M6 6.5h1v3h1" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </NavLink>
            <button type="button" onClick={toggle}
              className="flex items-center justify-center px-2 py-[9px] text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l-2 border-transparent transition-colors"
              title={`Theme: ${THEME_LABELS[mode]}`}
              aria-label={`Switch theme — currently ${THEME_LABELS[mode]}`}
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <circle cx="7" cy="7" r="5" stroke="currentColor" strokeWidth="1.4" />
                <path d="M7 2a5 5 0 010 10z" fill="currentColor" />
              </svg>
            </button>
          </div>
        ) : (
          <div className="border-t border-mycel-border/40 flex items-stretch">
            <NavLink
              to="/settings"
              className={({ isActive }) =>
                `flex items-center gap-2 pl-4 pr-2 py-[9px] text-[11px] ${isActive ? "text-mycel-accent bg-mycel-surface-hover border-l-2 border-mycel-accent" : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l-2 border-transparent"} transition-colors`
              }
              title="Settings"
            >
              <span className="shrink-0 flex items-center justify-center w-4 opacity-80">
                <Icon name="settings" size={14} />
              </span>
              <span className="truncate font-medium tracking-tight">Settings</span>
            </NavLink>
            <NavLink
              to="/about"
              className={({ isActive }) =>
                `flex-1 flex items-center gap-2 pl-3 pr-2 py-[9px] text-[11px] ${isActive ? "text-mycel-accent bg-mycel-surface-hover border-l border-mycel-border/40" : "text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l border-mycel-border/40"} transition-colors`
              }
              title="About / version"
            >
              <span className="shrink-0 flex items-center justify-center w-4 opacity-80">
                <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <circle cx="7" cy="7" r="5.5" />
                  <path d="M7 4.5v.01M6 6.5h1v3h1" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              </span>
              <span className="truncate">About</span>
            </NavLink>
            <button type="button" onClick={toggle}
              className="shrink-0 flex items-center justify-center w-9 py-[9px] text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover border-l border-mycel-border/40 transition-colors"
              title={`Theme: ${THEME_LABELS[mode]} — click to switch`}
              aria-label={`Switch theme — currently ${THEME_LABELS[mode]}`}
            >
              {/* Half-shaded circle — semantically "theme mode" (dark/light
                  split), visually distinct from the gear (Settings) icon. */}
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
                <circle cx="7" cy="7" r="5" stroke="currentColor" strokeWidth="1.4" />
                <path d="M7 2a5 5 0 010 10z" fill="currentColor" />
              </svg>
            </button>
          </div>
        )}
      </nav>

      <HeaderSlotProvider>
        <main className="flex-1 flex flex-col overflow-hidden bg-mycel-bg">
          <LayoutHeader collapsed={collapsed} onToggleCollapsed={toggleCollapsed} />
          <DegradedBanner />
          <div className="flex-1 overflow-auto">
            <RouteTransition>
              <Outlet />
            </RouteTransition>
          </div>
        </main>
      </HeaderSlotProvider>
      <CommandPalette />
    </div>
  );
}

/* ── LayoutHeader ───────────────────────────────────────────────
   Renders the shared Header with workspace dropdown + sidebar toggle
   on the left, pulling per-page title/actions from HeaderSlotContext.
──────────────────────────────────────────────────────────────── */
function LayoutHeader({
  collapsed,
  onToggleCollapsed,
}: {
  collapsed: boolean;
  onToggleCollapsed: () => void;
}) {
  const { slot } = useHeaderSlotContext();
  // Pages that render their own self-contained top band (AgentDetail's
  // HUD bar) opt out of the LayoutHeader entirely so we don't render an
  // empty 42px row + border above their own header. Sidebar toggle +
  // workspace dropdown still live in the sidebar, so navigation is
  // unaffected.
  if (slot.hidden) return null;
  return (
    <Header
      left={<SidebarToggle collapsed={collapsed} onToggle={onToggleCollapsed} />}
      center={slot.title}
      actions={slot.actions}
    />
  );
}
