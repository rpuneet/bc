import { useCallback, useEffect, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useTheme, THEME_LABELS } from "../context/ThemeContext";
import { useMediaQuery } from "../hooks/useMediaQuery";
import { CommandPalette } from "./CommandPalette";
import { api } from "../api/client";
import type { Channel, GatewayHealth, GatewayStatus, NotifySubscription } from "../api/client";
import { channelPlatform } from "./channels/messageUtils";
import { SetupWizard } from "./channels/SetupWizard";

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
    channels: <><path d="M2 4.5h10" /><path d="M2 7.5h7" opacity="0.5" /><path d="M2 10.5h10" /></>,
    roles: <path d="M7 2.5l4.5 2.5v3.5L7 11 2.5 8.5V5z" />,
    tools: <path d="M9.5 2.5l3 3-7 7H2.5v-3z" />,
    cron: <><circle cx="7" cy="7" r="4.5" /><path d="M7 4.5v2.5l1.5 1.5" /></>,
    secrets: <path d="M7 2.5a2 2 0 00-2 2V6H4v4.5h6V6H9V4.5a2 2 0 00-2-2zm0 5.5a.75.75 0 110 1.5.75.75 0 010-1.5z" />,
    metrics: <path d="M2 10l2.5-3.5 2 1.5L10 3" strokeLinecap="round" strokeLinejoin="round" />,
    workspace: <path d="M2.5 3.5h9v7h-9zM4.5 3.5V2.5h5v1" />,
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

const PLATFORM_META: Record<string, { label: string; color: string }> = {
  slack: { label: "Slack", color: "#E01E5A" },
  telegram: { label: "Telegram", color: "#26A5E4" },
  discord: { label: "Discord", color: "#5865F2" },
  github: { label: "GitHub", color: "#8B949E" },
  gmail: { label: "Gmail", color: "#EA4335" },
};

function getPlatformMeta(p: string) {
  return PLATFORM_META[p] ?? { label: p, color: "#8c7e72" };
}

function displayChannelName(name: string): string {
  const idx = name.indexOf(":");
  return idx > 0 ? name.slice(idx + 1) : name;
}

/* ── Channel tree (inline in nav) ────────────────────────────── */

function ChannelNavTree() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [gateways, setGateways] = useState<GatewayStatus[]>([]);
  const [subs, setSubs] = useState<NotifySubscription[]>([]);
  const [health, setHealth] = useState<Map<string, GatewayHealth>>(new Map());
  const [expandedGw, setExpandedGw] = useState<Set<string>>(new Set(["slack", "telegram", "discord"]));
  const [setupPlatform, setSetupPlatform] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const [chs, gws, subList] = await Promise.all([
        api.listChannels().catch(() => [] as Channel[]),
        api.listGateways().catch(() => [] as GatewayStatus[]),
        api.listSubscriptions().catch(() => [] as NotifySubscription[]),
      ]);
      setChannels(chs ?? []);
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

  const bucketMap = new Map<string, Channel[]>();
  for (const ch of channels) {
    const p = channelPlatform(ch.name);
    if (p === "internal") continue;
    const list = bucketMap.get(p) ?? [];
    list.push(ch);
    bucketMap.set(p, list);
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
        const ago = Date.now() - new Date(h.last_message_at).getTime();
        const mins = Math.floor(ago / 60000);
        if (mins < 1) tip += " · last message: just now";
        else if (mins < 60) tip += ` · last message: ${mins}m ago`;
        else {
          const hrs = Math.floor(mins / 60);
          tip += ` · last message: ${hrs}h ago`;
        }
      }
      return tip;
    }
    return `Disconnected${h.error ? ": " + h.error : ""}`;
  };

  const botDisplayName = (platform: string, gw?: GatewayStatus): string => {
    if (gw?.bot_name) return gw.bot_name;
    return platform;
  };

  return (
    <div className="py-0.5 ml-3 border-l border-bc-border/20">
      {[...bucketMap.entries()].map(([platform, chs]) => {
        const meta = getPlatformMeta(platform);
        const gwStatus = gwMap.get(platform);
        const isConnected = (gwStatus?.enabled && (gwStatus?.channels?.length ?? 0) > 0) || chs.length > 0;
        const isExpanded = expandedGw.has(platform);
        const name = botDisplayName(platform, gwStatus);

        return (
          <div key={platform}>
            <button
              type="button"
              onClick={() => toggleGw(platform)}
              className="w-full flex items-center gap-1.5 pl-3 pr-2 py-[3px] text-[10px] hover:bg-bc-bg/40 transition-colors group"
            >
              <svg width="6" height="6" viewBox="0 0 8 8"
                className={`text-bc-muted/25 transition-transform duration-100 shrink-0 ${isExpanded ? "" : "-rotate-90"}`}
              >
                <path d="M1.5 2L4 5L6.5 2" stroke="currentColor" strokeWidth="1.2" fill="none" strokeLinecap="round" />
              </svg>
              <span className="w-[5px] h-[5px] rounded-full shrink-0"
                title={healthTooltip(platform)}
                style={{ backgroundColor: isConnected ? "#22c55e" : gwStatus?.enabled ? "#fb923c" : "rgba(140,126,114,0.12)" }}
              />
              <span className="font-medium text-bc-text/60 group-hover:text-bc-text/80 truncate">
                @{name}
              </span>
              <span className="text-[8px] px-1 py-px rounded shrink-0" style={{ color: meta.color, opacity: 0.5 }}>
                {meta.label.toLowerCase()}
              </span>
              <span className="ml-auto flex items-center gap-1">
                {isConnected && (
                  <span className="text-[7px] text-bc-success/40 font-medium">live</span>
                )}
                <span className="text-[8px] text-bc-muted/20 tabular-nums">{chs.length || ""}</span>
              </span>
            </button>

            {isExpanded && chs.map((ch) => {
              const count = subCountMap.get(ch.name) ?? 0;
              return (
                <NavLink
                  key={ch.name}
                  to={"/channels/" + ch.name}
                  className={({ isActive }) =>
                    `flex items-center gap-1 pl-7 pr-2 py-[4px] text-[11px] transition-all duration-100 rounded-r ${
                      isActive
                        ? "text-bc-text bg-bc-surface/50 font-medium"
                        : "text-bc-muted/40 hover:text-bc-text/70 hover:bg-bc-surface/25"
                    }`
                  }
                  style={({ isActive }) => ({
                    borderLeft: isActive ? `3px solid ${meta.color}` : "3px solid transparent",
                    marginLeft: "-1px",
                  })}
                >
                  <span className="text-[8px] text-bc-muted/20">#</span>
                  <span className="truncate">{displayChannelName(ch.name)}</span>
                  {count > 0 && <span className="ml-auto text-[8px] text-bc-success/30 tabular-nums">{count}</span>}
                </NavLink>
              );
            })}
          </div>
        );
      })}

      {/* Connect app — inline dropdown */}
      <div className="px-3 pt-1 pb-0.5 relative">
        <button type="button" onClick={() => setShowConnectMenu((v) => !v)}
          className={`w-full py-[3px] text-[9px] border rounded transition-all ${
            showConnectMenu
              ? "text-bc-accent border-bc-accent/20 bg-bc-accent/5"
              : "text-bc-muted/20 hover:text-bc-accent border-bc-border/10 hover:border-bc-accent/15"
          }`}
        >
          + Connect app
        </button>

        {showConnectMenu && (
          <div className="mt-1 border border-bc-border/30 rounded-lg bg-bc-bg overflow-hidden"
            style={{ animation: "fadeIn 100ms ease-out" }}
          >
            {Object.entries(PLATFORM_META).map(([key, meta]) => (
              <button
                key={key}
                type="button"
                onClick={() => { setShowConnectMenu(false); setSetupPlatform(key); }}
                className="w-full flex items-center gap-2 px-3 py-2 text-[10px] text-bc-muted/50 hover:text-bc-text hover:bg-bc-surface/20 transition-colors"
              >
                <span className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: meta.color }} />
                <span>{meta.label}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {setupPlatform && setupPlatform !== "_choose" && (
        <SetupWizard platform={setupPlatform} onClose={() => setSetupPlatform(null)} onConnected={() => void fetchData()} />
      )}
      <style>{`@keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: translateY(0); } }`}</style>
    </div>
  );
}

/* ── Nav items ───────────────────────────────────────────────── */

const MAIN_NAV_ITEMS = [
  { to: "/live", label: "Live", icon: "live" },
  { to: "/agents", label: "Agents", icon: "agents" },
  { to: "/channels", label: "Channels", icon: "channels" },
  { to: "/roles", label: "Roles", icon: "roles" },
  { to: "/tools", label: "Tools", icon: "tools" },
  { to: "/cron", label: "Cron", icon: "cron" },
  { to: "/secrets", label: "Secrets", icon: "secrets" },
  { to: "/stats", label: "Metrics", icon: "metrics" },
] as const;

const UTIL_NAV_ITEMS = [
  { to: "/workspace", label: "Workspace", icon: "workspace" },
  { to: "/settings", label: "Settings", icon: "settings" },
] as const;

const NAV_ITEMS = [...MAIN_NAV_ITEMS, ...UTIL_NAV_ITEMS];

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
  channelsExpanded,
}: {
  items: ReadonlyArray<{ to: string; label: string; icon: string }>;
  collapsed: boolean;
  isMobile: boolean;
  channelsExpanded?: boolean;
}) {
  const isIconOnly = collapsed && !isMobile;
  const showTree = !isIconOnly && channelsExpanded;

  return (
    <>
      {items.map(({ to, label, icon }) => {
        const isChannels = label === "Channels";
        return (
          <li key={to}>
            <NavLink
              to={to}
              end={!isChannels}
              title={isIconOnly ? label : undefined}
              className={({ isActive }) =>
                `relative flex items-center gap-2.5 ${isIconOnly ? "justify-center px-2" : "pl-4 pr-3"} py-[7px] text-[13px] outline-none transition-colors duration-75 ${
                  isActive
                    ? "text-bc-accent font-medium border-l-2 border-bc-accent bg-bc-bg/60"
                    : "text-bc-muted/70 hover:text-bc-text hover:bg-bc-bg/30 border-l-2 border-transparent"
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
            </NavLink>
            {isChannels && showTree && <ChannelNavTree />}
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

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => { const next = !prev; writeCollapsed(next); return next; });
  }, []);

  useEffect(() => { if (isMobile) setCollapsed(true); }, [isMobile]);
  useEffect(() => {
    const match = NAV_ITEMS.find((item) => location.pathname.startsWith(item.to));
    document.title = match ? `${match.label} \u2014 bc` : "bc";
  }, [location.pathname]);
  useEffect(() => { setMobileOpen(false); }, [location.pathname]);

  const sidebarWidth = collapsed && !isMobile ? "w-14" : "w-48";

  return (
    <div className="flex h-screen">
      {/* Mobile hamburger */}
      <button type="button" onClick={() => setMobileOpen(true)}
        className="fixed top-3 left-3 z-40 md:hidden p-2 rounded border border-bc-border bg-bc-surface text-bc-muted hover:text-bc-text transition-colors"
        aria-label="Open navigation"
      >
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.5">
          <path d="M3 5h12M3 9h12M3 13h12" />
        </svg>
      </button>

      {mobileOpen && <div className="fixed inset-0 z-40 bg-black/50 md:hidden" onClick={() => setMobileOpen(false)} />}

      {/* Sidebar */}
      <nav
        className={`fixed inset-y-0 left-0 z-50 ${sidebarWidth} shrink-0 border-r border-bc-border/50 bg-bc-surface shadow-bc flex flex-col transition-all duration-200 md:relative md:translate-x-0 ${
          isMobile ? (mobileOpen ? "translate-x-0 w-48" : "-translate-x-full") : ""
        }`}
        style={{ scrollbarWidth: "thin", scrollbarColor: "rgba(255,255,255,0.04) transparent" }}
      >
        {/* Header */}
        <div className="px-3 py-3 border-b border-bc-border/30 flex items-center justify-between">
          {(!collapsed || isMobile) ? (
            <div className="flex items-center gap-2 overflow-hidden">
              <span className="w-6 h-6 rounded-md bg-bc-accent/15 text-bc-accent flex items-center justify-center text-[10px] font-bold shrink-0">
                {(userName || "U")[0]!.toUpperCase()}
              </span>
              <div className="min-w-0">
                <p className="text-[12px] font-medium text-bc-text truncate">{userName || "User"}</p>
                <p className="text-[9px] text-bc-muted/40 -mt-0.5">workspace</p>
              </div>
            </div>
          ) : (
            <span className="w-6 h-6 rounded-md bg-bc-accent/15 text-bc-accent flex items-center justify-center text-[10px] font-bold shrink-0">
              {(userName || "U")[0]!.toUpperCase()}
            </span>
          )}
          {isMobile ? (
            <button type="button" onClick={() => setMobileOpen(false)}
              className="p-0.5 rounded text-bc-muted/40 hover:text-bc-text transition-colors" aria-label="Close navigation"
            >
              <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5">
                <path d="M3 3l8 8M11 3l-8 8" />
              </svg>
            </button>
          ) : (
            <button type="button" onClick={toggleCollapsed}
              className="p-0.5 rounded text-bc-muted/30 hover:text-bc-muted/70 transition-colors"
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
            channelsExpanded={location.pathname.startsWith("/channels")}
          />
          <li className={`my-1.5 ${collapsed && !isMobile ? "mx-2" : "mx-3"}`}>
            <div className="border-t border-bc-border/15" />
          </li>
          <NavList items={UTIL_NAV_ITEMS} collapsed={collapsed} isMobile={isMobile} />
        </ul>

        {/* Theme toggle */}
        <div className="px-3 py-2 border-t border-bc-border/20">
          <button type="button" onClick={toggle}
            className="px-2 py-1 rounded text-[10px] text-bc-muted/30 hover:text-bc-muted/60 border border-bc-border/15 hover:border-bc-border/30 transition-colors w-full"
            title={`Theme: ${THEME_LABELS[mode]}`}
          >
            {collapsed && !isMobile ? THEME_LABELS[mode][0] : THEME_LABELS[mode]}
          </button>
        </div>
      </nav>

      <main className="flex-1 overflow-auto bg-bc-bg">
        <Outlet />
      </main>
      <CommandPalette />
    </div>
  );
}
