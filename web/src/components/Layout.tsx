import { useCallback, useEffect, useState } from "react";
import { NavLink, Outlet, useLocation, useMatch } from "react-router-dom";
import { useTheme, THEME_LABELS } from "../context/ThemeContext";
import { useMediaQuery } from "../hooks/useMediaQuery";
import { CommandPalette } from "./CommandPalette";
import { api } from "../api/client";
import type { NotificationSource, GatewayHealth, GatewayStatus, NotifySubscription } from "../api/client";
import { sourcePlatform } from "./notifications/messageUtils";
import { SetupWizard, PlatformChooser, PLATFORM_MAP } from "./notifications/SetupWizard";
import { PLATFORM_ICON_MAP } from "./notifications/PlatformIcons";
import { Header } from "./Header";
import { SidebarToggle, WorkspaceDropdown } from "./WorkspaceDropdown";
import { HeaderSlotProvider, useHeaderSlotContext } from "../context/HeaderSlotContext";
import { useWorkspace } from "../context/WorkspaceContext";

const SIDEBAR_KEY = "bc-sidebar-collapsed";

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
  const IconComponent = PLATFORM_ICON_MAP[base] ?? null;
  if (def) return { label: def.label, color: def.color, IconComponent };
  return { label: p, color: "#8c7e72", IconComponent };
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

/* ── Channel list with show more/less toggle ───────────────── */

const SIDEBAR_CHANNEL_LIMIT = 15;

function ChannelList({
  channels,
  subCountMap,
  prefix,
}: {
  channels: NotificationSource[];
  subCountMap: Map<string, number>;
  prefix: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const visible = expanded ? channels : channels.slice(0, SIDEBAR_CHANNEL_LIMIT);
  const hasMore = channels.length > SIDEBAR_CHANNEL_LIMIT;

  return (
    <>
      {visible.map((ch) => {
        const count = subCountMap.get(ch.name) ?? 0;
        const chName = displaySourceName(ch.name);
        return (
          <NavLink
            key={ch.name}
            to={`${prefix}/notifications/${ch.name}`}
            className="block"
            style={({ isActive }: { isActive: boolean }) => ({
              display: "flex",
              alignItems: "center",
              gap: 8,
              height: 24,
              padding: "0 8px",
              borderRadius: 5,
              fontSize: 12.5,
              color: isActive ? "var(--mycel-text, #e5e5e5)" : count > 0 ? "var(--mycel-text, #e5e5e5)" : "var(--mycel-muted, #a0a0a0)",
              background: isActive ? "rgba(249, 115, 22, 0.12)" : "transparent",
              fontWeight: isActive ? 600 : count > 0 ? 500 : 400,
              cursor: "pointer",
              marginBottom: 1,
              textDecoration: "none",
            })}
          >
            <span
              style={{
                width: 12,
                color: "var(--mycel-muted, #4a4a4a)",
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
            {count > 0 && (
              <span
                style={{
                  fontSize: 10.5,
                  fontWeight: 600,
                  color: "var(--mycel-muted, #a0a0a0)",
                  fontFamily: "'JetBrains Mono', monospace",
                  padding: "1px 5px",
                  borderRadius: 999,
                  background: "var(--mycel-surface, #212121)",
                }}
              >
                {count}
              </span>
            )}
          </NavLink>
        );
      })}
      {hasMore && (
        <button
          type="button"
          onClick={() => setExpanded((prev) => !prev)}
          style={{
            display: "block",
            width: "100%",
            padding: "3px 8px",
            fontSize: 11,
            color: "var(--mycel-muted, #6b6b6b)",
            background: "none",
            border: "none",
            cursor: "pointer",
            textAlign: "left",
          }}
        >
          {expanded ? "show less" : `show ${channels.length - SIDEBAR_CHANNEL_LIMIT} more...`}
        </button>
      )}
    </>
  );
}

/* ── Notification tree (inline in nav) ───────────────────────── */

function NotificationNavTree() {
  const [sources, setSources] = useState<NotificationSource[]>([]);
  const [gateways, setGateways] = useState<GatewayStatus[]>([]);
  const [subs, setSubs] = useState<NotifySubscription[]>([]);
  const [health, setHealth] = useState<Map<string, GatewayHealth>>(new Map());
  const [expandedGw, setExpandedGw] = useState<Set<string>>(new Set(["slack", "telegram", "discord"]));
  const [setupPlatform, setSetupPlatform] = useState<string | null>(null);

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

      // Fetch health for each enabled gateway
      const enabledGws = (gws ?? []).filter((g) => g.enabled);
      if (enabledGws.length > 0) {
        const healthResults = await Promise.all(
          enabledGws.map((g) =>
            api.getGatewayHealth(g.platform).catch(() => null),
          ),
        );
        const hmap = new Map<string, GatewayHealth>();
        for (const h of healthResults) {
          if (h) hmap.set(h.platform, h);
        }
        setHealth(hmap);
      }
    } catch { /* */ }
  }, []);

  useEffect(() => {
    void fetchData();
    const interval = setInterval(() => void fetchData(), 12000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const toggleGw = (p: string) => {
    setExpandedGw((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p); else next.add(p);
      return next;
    });
  };

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

  const [showConnectMenu, setShowConnectMenu] = useState(false);

  const healthTooltip = (platform: string): string | undefined => {
    const h = health.get(platform);
    if (!h) return undefined;
    if (h.connected) {
      let tip = "Connected";
      if (h.last_message_at) {
        const ts = new Date(h.last_message_at).getTime();
        // Guard against zero / unset / pre-2001 timestamps that produce
        // nonsensical "17753690h ago" tooltips.
        if (Number.isFinite(ts) && ts > 978307200000) {
          const ago = Date.now() - ts;
          const mins = Math.floor(ago / 60000);
          if (mins < 1) tip += " · last message: just now";
          else if (mins < 60) tip += ` · last message: ${mins}m ago`;
          else if (mins < 1440) {
            const hrs = Math.floor(mins / 60);
            tip += ` · last message: ${hrs}h ago`;
          } else {
            const days = Math.floor(mins / 1440);
            tip += ` · last message: ${days}d ago`;
          }
        }
      }
      return tip;
    }
    return `Disconnected${h.error ? ": " + h.error : ""}`;
  };

  const { workspace } = useWorkspace();
  const prefix = workspace ? `/w/${workspace.id}` : "";

  return (
    <div
      style={{
        paddingLeft: 10,
        marginLeft: 9,
        borderLeft: "1px solid var(--mycel-border, rgba(255,255,255,0.08))",
        marginTop: 2,
        marginBottom: 4,
        maxHeight: 280,
        overflowY: "auto",
      }}
    >
      {[...bucketMap.entries()].map(([platform, chs]) => {
        const meta = getPlatformMeta(platform);
        const gwStatus = gwMap.get(platform);
        const isConnected = (gwStatus?.enabled && (gwStatus?.channels?.length ?? 0) > 0) || chs.length > 0;
        const isExpanded = expandedGw.has(platform);

        return (
          <div key={platform}>
            {/* Platform header — icon + server/workspace name */}
            <button
              type="button"
              onClick={() => toggleGw(platform)}
              className="w-full flex items-center"
              style={{
                gap: 6,
                padding: "5px 8px 2px",
                fontSize: 11.5,
                color: "var(--mycel-text-2, #a0a0a0)",
                fontWeight: 500,
                background: "none",
                border: "none",
                cursor: "pointer",
              }}
            >
              {meta.IconComponent ? <meta.IconComponent size={12} /> : <span style={{ fontSize: 12 }}>{"📌"}</span>}
              <span className="truncate">{gwStatus?.bot_name || (chs.length > 0 ? sourceGroup(chs[0]?.name ?? "") : null) || meta.label}</span>
              {isConnected && (
                <span
                  className="ml-auto shrink-0"
                  title={healthTooltip(platform)}
                  style={{
                    width: 5,
                    height: 5,
                    borderRadius: 999,
                    background: "#22c55e",
                    boxShadow: "0 0 5px rgba(34,197,94,0.5)",
                  }}
                />
              )}
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ opacity: 0.4, transform: isExpanded ? "rotate(0deg)" : "rotate(-90deg)", transition: "transform 0.15s" }}>
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </button>

            {/* Channel rows */}
            {isExpanded && <ChannelList channels={chs} subCountMap={subCountMap} prefix={prefix} />}
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
          color: "var(--mycel-muted, #6b6b6b)",
          cursor: "pointer",
          border: "1px dashed var(--mycel-border, #2a2a2a)",
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
] as const;

const UTIL_NAV_ITEMS = [
  { to: "/settings", label: "Settings", icon: "settings" },
] as const;

// Global (non-workspace-scoped) nav items — rendered below a separator
// in the sidebar. Costs here aggregates across every workspace.
const GLOBAL_NAV_ITEMS = [
  { to: "/costs", label: "Costs", icon: "metrics" },
] as const;

const NAV_ITEMS = [...MAIN_NAV_ITEMS, ...UTIL_NAV_ITEMS, ...GLOBAL_NAV_ITEMS];

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
  global = false,
}: {
  items: ReadonlyArray<{ to: string; label: string; icon: string }>;
  collapsed: boolean;
  isMobile: boolean;
  notificationsExpanded?: boolean;
  onToggleNotifications?: () => void;
  /** When true, render links verbatim — skip the /w/<id>/ prefix.
   *  Used for cross-workspace routes like /costs. */
  global?: boolean;
}) {
  const isIconOnly = collapsed && !isMobile;
  const showTree = !isIconOnly && notificationsExpanded;
  // Workspace-scoped targets get /w/<id>/ prefixed at render time.
  // Global items skip the prefix so they hit top-level routes.
  const { workspace } = useWorkspace();
  const prefix = !global && workspace ? `/w/${workspace.id}` : "";

  return (
    <>
      {items.map(({ to, label, icon }) => {
        const isNotifications = label === "Notifications";
        const scopedTo = `${prefix}${to}`;
        return (
          <li key={to}>
            <NavLink
              to={scopedTo}
              end={!isNotifications}
              title={isIconOnly ? label : undefined}
              className={({ isActive }) =>
                `relative flex items-center gap-2.5 ${isIconOnly ? "justify-center px-2" : "pl-4 pr-3"} py-[7px] text-[13px] outline-none transition-colors duration-75 ${
                  isActive
                    ? "text-mycel-accent font-medium border-l-2 border-mycel-accent bg-mycel-bg/60"
                    : "text-mycel-muted/70 hover:text-mycel-text hover:bg-mycel-bg/30 border-l-2 border-transparent"
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
                <span className="w-1.5 h-1.5 rounded-full bg-red-500 animate-pulse ml-auto" />
              )}
              {isNotifications && !isIconOnly && onToggleNotifications && (
                <button
                  type="button"
                  onClick={(e) => { e.preventDefault(); e.stopPropagation(); onToggleNotifications(); }}
                  className="ml-auto shrink-0 p-0.5 rounded text-mycel-muted/40 hover:text-mycel-muted/70 transition-all"
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
  const scopedNotifications = useMatch("/w/:wsId/notifications/*");
  const legacyNotifications = useMatch("/notifications/*");
  const legacyChannelsRoute = useMatch("/channels/*");
  const legacyChannelsScopedRoute = useMatch("/w/:wsId/channels/*");
  const onNotifRoute = Boolean(scopedNotifications || legacyNotifications || legacyChannelsRoute || legacyChannelsScopedRoute);
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
    // Match against the trailing segment of the URL so /w/<id>/live and
    // the legacy /live both produce the "Live" title.
    const match = NAV_ITEMS.find((item) => {
      const seg = item.to.replace(/^\//, "");
      return (
        location.pathname === item.to ||
        location.pathname.startsWith(`${item.to}/`) ||
        location.pathname.endsWith(`/${seg}`) ||
        location.pathname.includes(`/${seg}/`)
      );
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

      {mobileOpen && <div className="fixed inset-0 z-40 bg-black/50 md:hidden" onClick={() => setMobileOpen(false)} />}

      {/* Sidebar */}
      <nav
        className={`fixed inset-y-0 left-0 z-50 ${sidebarWidth} shrink-0 border-r border-mycel-border/50 bg-mycel-surface shadow-mycel flex flex-col transition-all duration-200 md:relative md:translate-x-0 ${
          isMobile ? (mobileOpen ? "translate-x-0 w-48" : "-translate-x-full") : ""
        }`}
        style={{ scrollbarWidth: "thin", scrollbarColor: "rgba(255,255,255,0.04) transparent" }}
      >
        {/* Header */}
        <div className="px-3 py-3 border-b border-mycel-border/30 flex items-center justify-between">
          {(!collapsed || isMobile) ? (
            <div className="flex items-center gap-2 overflow-hidden">
              <span
                className="w-6 h-6 shrink-0 flex items-center justify-center font-bold"
                style={{
                  borderRadius: 7,
                  background: "var(--mycel-accent, #f97316)",
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
                  {userName ? (userName.startsWith("@") ? userName : `@${userName}`) : "@mycel"}
                </p>
                <p className="text-[9px] text-mycel-muted/40 -mt-0.5" style={{ fontFamily: "'JetBrains Mono', monospace" }}>workspace</p>
              </div>
            </div>
          ) : (
            <span
              className="w-6 h-6 shrink-0 flex items-center justify-center font-bold"
              style={{
                borderRadius: 7,
                background: "var(--mycel-accent, #f97316)",
                color: "#0d0d0d",
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
              className="p-0.5 rounded text-mycel-muted/40 hover:text-mycel-text transition-colors" aria-label="Close navigation"
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

        {/* Nav */}
        <ul className="flex-1 py-1.5 overflow-y-auto" style={{ scrollbarWidth: "thin" }}>
          <NavList
            items={MAIN_NAV_ITEMS}
            collapsed={collapsed}
            isMobile={isMobile}
            notificationsExpanded={notificationsExpanded}
            onToggleNotifications={toggleNotifications}
          />
          <li className={`my-1.5 ${collapsed && !isMobile ? "mx-2" : "mx-3"}`}>
            <div className="border-t border-mycel-border/15" />
          </li>
          <NavList items={GLOBAL_NAV_ITEMS} collapsed={collapsed} isMobile={isMobile} global />
          <li className={`my-1.5 ${collapsed && !isMobile ? "mx-2" : "mx-3"}`}>
            <div className="border-t border-mycel-border/15" />
          </li>
          <NavList items={UTIL_NAV_ITEMS} collapsed={collapsed} isMobile={isMobile} />
        </ul>

        {/* Theme toggle — matches nav-item width and padding for visual alignment */}
        <div className="py-1.5 border-t border-mycel-border/20">
          <button type="button" onClick={toggle}
            className={`relative flex items-center gap-2.5 ${collapsed && !isMobile ? "justify-center px-2" : "pl-4 pr-3"} py-[7px] w-full text-[11px] text-mycel-muted/50 hover:text-mycel-text hover:bg-mycel-bg/30 border-l-2 border-transparent transition-colors`}
            title={`Theme: ${THEME_LABELS[mode]}`}
          >
            <span className="shrink-0 flex items-center justify-center w-4 opacity-60">
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
                <circle cx="7" cy="7" r="3" />
                <path d="M7 1v2M7 11v2M1 7h2M11 7h2M2.5 2.5l1.5 1.5M10 10l1.5 1.5M2.5 11.5L4 10M10 4l1.5-1.5" strokeLinecap="round" />
              </svg>
            </span>
            {(!collapsed || isMobile) && (
              <span className="truncate">{THEME_LABELS[mode]}</span>
            )}
          </button>
        </div>
      </nav>

      <HeaderSlotProvider>
        <main className="flex-1 flex flex-col overflow-hidden bg-mycel-bg">
          <LayoutHeader collapsed={collapsed} onToggleCollapsed={toggleCollapsed} />
          <div className="flex-1 overflow-auto">
            <Outlet />
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
  return (
    <Header
      left={
        <>
          <SidebarToggle collapsed={collapsed} onToggle={onToggleCollapsed} />
          <WorkspaceDropdown />
        </>
      }
      center={slot.title}
      actions={slot.actions}
    />
  );
}
