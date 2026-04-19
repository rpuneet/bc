import { useState, useEffect, useRef, useCallback } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../../api/client";
import type {
  Agent,
  NotificationSource,
  ChannelMessage,
  DeliveryEntry,
  NotifySubscription,
} from "../../api/client";
import { useWebSocket } from "../../hooks/useWebSocket";
import { MessageContent } from "../MessageContent";
import {
  gatewayPlatform,
  formatRelativeTime,
  groupMessages,
  agentColor,
  dateKey,
  formatDayLabel,
  parseGitHubCard,
} from "./messageUtils";
import type { GitHubCard } from "./messageUtils";

/* ── Helpers ──────────────────────────────────────────────────── */

/** Strip "[telegram] " or "[slack] " prefix from sender names for cleaner display. */
function cleanSender(sender: string): string {
  const match = sender.match(/^\[(?:telegram|slack|discord)\]\s*(.+)$/i);
  return match?.[1] ?? sender;
}

/** Get the first two letters for avatar, stripping platform prefix. */
function senderInitials(sender: string): string {
  const clean = cleanSender(sender);
  return clean.slice(0, 2).toUpperCase();
}

/* ── Platform glyphs ─────────────────────────────────────────── */

function SlackGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" style={{ flexShrink: 0 }}>
      <path d="M6 15a2 2 0 1 1-2-2h2v2zm1 0a2 2 0 0 1 4 0v5a2 2 0 0 1-4 0v-5z" fill="#E01E5A" />
      <path d="M9 6a2 2 0 1 1 2-2v2H9zm0 1a2 2 0 0 1 0 4H4a2 2 0 0 1 0-4h5z" fill="#36C5F0" />
      <path d="M18 9a2 2 0 1 1 2 2h-2V9zm-1 0a2 2 0 0 1-4 0V4a2 2 0 0 1 4 0v5z" fill="#2EB67D" />
      <path d="M15 18a2 2 0 1 1-2 2v-2h2zm0-1a2 2 0 0 1 0-4h5a2 2 0 0 1 0 4h-5z" fill="#ECB22E" />
    </svg>
  );
}

function TelegramGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <circle cx="12" cy="12" r="11" fill="#229ED9" />
      <path d="M5.5 11.5l12-4.5-2 12-4-3-2 2-1-3.5 7-5.5-8 3.5-2-1z" fill="#fff" />
    </svg>
  );
}

function DiscordGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <circle cx="12" cy="12" r="11" fill="#5865F2" />
      <text x="12" y="16" textAnchor="middle" fill="#fff" fontSize="10" fontWeight="bold">D</text>
    </svg>
  );
}

const PLATFORM_GLYPHS: Record<string, React.FC<{ size?: number }>> = {
  slack: SlackGlyph,
  telegram: TelegramGlyph,
  discord: DiscordGlyph,
};

/* ── GitHub card renderer ────────────────────────────────────── */

const STATUS_COLORS: Record<string, string> = {
  OPEN: "bg-green-500/15 text-green-400 border-green-500/20",
  MERGED: "bg-purple-500/15 text-purple-400 border-purple-500/20",
  CLOSED: "bg-red-500/15 text-red-400 border-red-500/20",
  SYNCHRONIZE: "bg-blue-500/15 text-blue-400 border-blue-500/20",
};

function GitHubCardView({ card }: { card: GitHubCard }) {
  const statusClass = STATUS_COLORS[card.status ?? ""] ?? "bg-bc-surface/30 text-bc-muted border-bc-border/30";
  const stateColor = card.status === "MERGED" ? "#a855f7" : card.status === "CLOSED" ? "#ef4444" : "#22c55e";
  const icon = card.type === "pr" ? (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" className="text-bc-muted/60 shrink-0">
      <path d="M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm5.677-.177L9.573.677A.25.25 0 0 1 10 .854V2.5h1A2.5 2.5 0 0 1 13.5 5v5.628a2.251 2.251 0 1 1-1.5 0V5a1 1 0 0 0-1-1h-1v1.646a.25.25 0 0 1-.427.177L7.177 3.427a.25.25 0 0 1 0-.354ZM3.75 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm0 9.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm8.25.75a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Z" />
    </svg>
  ) : (
    <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" className="text-bc-muted/60 shrink-0">
      <path d="M8 9.5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3Z" />
      <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Z" />
    </svg>
  );

  return (
    <div
      className="mt-1.5 max-w-lg overflow-hidden"
      style={{
        background: "#151515",
        borderRadius: 6,
        borderLeft: `2px solid ${stateColor}`,
      }}
    >
      <div className="flex items-start gap-2 px-3 py-2">
        {icon}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {card.url ? (
              <a
                href={card.url}
                target="_blank"
                rel="noopener noreferrer"
                className="hover:underline truncate"
                style={{ fontSize: 13, fontWeight: 500, color: "#e5e5e5" }}
              >
                {card.title}
              </a>
            ) : (
              <span style={{ fontSize: 13, fontWeight: 500, color: "#e5e5e5" }} className="truncate">
                {card.title}
              </span>
            )}
            {card.number && (
              <span style={{ fontSize: 11, color: "#a0a0a0", fontFamily: "'JetBrains Mono', monospace" }}>
                #{card.number}
              </span>
            )}
            {card.status && (
              <span className={`text-[9px] font-semibold uppercase px-1.5 py-0.5 rounded border ${statusClass}`}>
                {card.status}
              </span>
            )}
          </div>
          {card.repo && (
            <span style={{ fontSize: 10, color: "#6b6b6b", fontFamily: "'JetBrains Mono', monospace" }}>
              {card.repo}
            </span>
          )}
          {(card.additions !== undefined || card.deletions !== undefined || card.changedFiles !== undefined) && (
            <div className="flex items-center gap-2 mt-1" style={{ fontSize: 11, fontFamily: "'JetBrains Mono', monospace" }}>
              {card.changedFiles !== undefined && (
                <span style={{ color: "#6b6b6b" }}>{card.changedFiles} file{card.changedFiles !== 1 ? "s" : ""}</span>
              )}
              {card.additions !== undefined && (
                <span style={{ color: "#22c55e" }}>+{card.additions}</span>
              )}
              {card.deletions !== undefined && (
                <span style={{ color: "#ef4444" }}>-{card.deletions}</span>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/* ── Component ───────────────────────────────────────────────── */

export function GatewayFeed({
  channelName,
  channel,
  onPeekAgent,
}: {
  channelName: string;
  channel?: NotificationSource;
  onPeekAgent: (name: string) => void;
}) {
  const PAGE_SIZE = 30;
  const [messages, setMessages] = useState<ChannelMessage[]>([]);
  const [deliveries, setDeliveries] = useState<DeliveryEntry[]>([]);
  const [subscriptions, setSubscriptions] = useState<NotifySubscription[]>([]);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentLoading, setAgentLoading] = useState<string | null>(null);
  const [popoverLoading, setPopoverLoading] = useState(false);
  const [showAgents, setShowAgents] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [initialLoading, setInitialLoading] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const agentsPopoverRef = useRef<HTMLDivElement>(null);
  const { subscribe } = useWebSocket();

  const platform = gatewayPlatform(channelName);
  const channelLabel = channelName.includes(":")
    ? channelName.split(":").slice(1).join(":")
    : channelName;

  /* ── Data fetching ─────────────────────────────────────────── */

  const fetchInitial = useCallback(async () => {
    setInitialLoading(true);
    try {
      const [msgs, activity, subs] = await Promise.all([
        api.getChannelHistory(channelName, PAGE_SIZE),
        api.getChannelActivity(channelName, 100).catch(() => []),
        api.getChannelSubscriptions(channelName).catch(() => []),
      ]);
      const m = msgs ?? [];
      setMessages(m);
      setHasMore(m.length >= PAGE_SIZE);
      setDeliveries(activity ?? []);
      setSubscriptions(subs ?? []);
    } catch {
      setMessages([]);
    } finally {
      setInitialLoading(false);
    }
  }, [channelName]);

  const fetchAgents = useCallback(async () => {
    try {
      const [agentList, subs] = await Promise.all([
        api.listAgents(),
        api.getChannelSubscriptions(channelName).catch(() => []),
      ]);
      setAgents(agentList ?? []);
      setSubscriptions(subs ?? []);
    } catch { /* keep previous */ }
    setPopoverLoading(false);
  }, [channelName]);

  useEffect(() => {
    if (!showAgents) return;
    setPopoverLoading(true);
    setAgents([]);
    void fetchAgents();
    const interval = setInterval(() => void fetchAgents(), 8000);
    return () => clearInterval(interval);
  }, [showAgents, fetchAgents]);

  // Close popover on outside click
  useEffect(() => {
    if (!showAgents) return;
    const handleClick = (e: MouseEvent) => {
      if (agentsPopoverRef.current && !agentsPopoverRef.current.contains(e.target as Node)) {
        setShowAgents(false);
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setShowAgents(false);
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [showAgents]);

  const handleSubscribe = async (agentName: string) => {
    setAgentLoading(agentName);
    try {
      await api.subscribe(channelName, agentName, false);
      await fetchAgents();
    } catch { /* */ }
    setAgentLoading(null);
  };

  const handleUnsubscribe = async (agentName: string) => {
    setAgentLoading(agentName);
    try {
      await api.unsubscribe(channelName, agentName);
      await fetchAgents();
    } catch { /* */ }
    setAgentLoading(null);
  };

  const handleToggleMention = async (agentName: string, current: boolean) => {
    setAgentLoading(agentName);
    try {
      await api.setMentionOnly(channelName, agentName, !current);
      await fetchAgents();
    } catch { /* */ }
    setAgentLoading(null);
  };

  useEffect(() => {
    void fetchInitial();
  }, [fetchInitial]);

  // Load more older messages when scrolling to top
  const loadMore = useCallback(async () => {
    if (loadingMore || !hasMore || messages.length === 0) return;
    setLoadingMore(true);
    try {
      const oldestMsg = [...messages].sort(
        (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
      )[0];
      const oldestId = oldestMsg?.id;
      const older = await api.getChannelHistory(channelName, PAGE_SIZE, oldestId);
      if (!older || older.length === 0) {
        setHasMore(false);
      } else {
        setMessages((prev) => {
          const ids = new Set(prev.map((m) => m.id));
          const newMsgs = older.filter((m) => !ids.has(m.id));
          return [...newMsgs, ...prev];
        });
        setHasMore(older.length >= PAGE_SIZE);
      }
    } catch { /* */ }
    setLoadingMore(false);
  }, [channelName, messages, loadingMore, hasMore]);

  // IntersectionObserver for infinite scroll
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          void loadMore();
        }
      },
      { rootMargin: "200px" },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [loadMore]);

  // Auto-scroll to bottom on initial load and new messages
  const prevMsgCountRef = useRef(0);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const isNewMessage = messages.length > prevMsgCountRef.current;
    prevMsgCountRef.current = messages.length;
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
    if (isNewMessage && (nearBottom || !initialLoading)) {
      requestAnimationFrame(() => {
        el.scrollTop = el.scrollHeight;
      });
    }
  }, [messages.length, initialLoading]);

  // Scroll to bottom on first render with messages
  useEffect(() => {
    if (!initialLoading && messages.length > 0) {
      const el = scrollRef.current;
      if (el) {
        requestAnimationFrame(() => {
          el.scrollTop = el.scrollHeight;
        });
      }
    }
  }, [initialLoading]); // eslint-disable-line react-hooks/exhaustive-deps

  /* ── Live WebSocket updates ────────────────────────────────── */

  useEffect(() => {
    const unsub1 = subscribe("channel.message", (event) => {
      const data = event.data as {
        channel?: string;
        message?: ChannelMessage;
      };
      if (data.channel === channelName && data.message) {
        const msg = {
          ...data.message,
          created_at: data.message.created_at || new Date().toISOString(),
        };
        setMessages((prev) => {
          if (prev.some((m) => m.id === msg.id)) return prev;
          return [...prev, msg];
        });
      }
    });
    const unsub2 = subscribe("gateway.message", (event) => {
      const data = event.data as { channel?: string };
      if (data.channel === channelName) {
        void api
          .getChannelActivity(channelName, 100)
          .then((d) => setDeliveries(d ?? []))
          .catch(() => {});
      }
    });
    return () => {
      unsub1();
      unsub2();
    };
  }, [subscribe, channelName]);

  /* ── Delivery matching ─────────────────────────────────────── */

  const deliveryByPreview = new Map<string, DeliveryEntry[]>();
  for (const d of deliveries) {
    const key = d.preview ?? "";
    const list = deliveryByPreview.get(key) ?? [];
    list.push(d);
    deliveryByPreview.set(key, list);
  }

  const agentNames = new Set(agents.map((a) => a.name));
  const subAgents = new Set(subscriptions.map((s) => s.agent));

  const subMap = new Map<string, NotifySubscription>();
  for (const sub of subscriptions) subMap.set(sub.agent, sub);
  const subscribedAgents = agents.filter((a) => subMap.has(a.name));

  const agentSortOrder = (agent: { state: string }) => {
    if (agent.state === "working" || agent.state === "running") return 0;
    if (agent.state === "stopped") return 2;
    return 1;
  };

  const availableAgents = agents
    .filter((a) => !subMap.has(a.name))
    .sort((a, b) => agentSortOrder(a) - agentSortOrder(b) || a.name.localeCompare(b.name));

  /* ── Message grouping ─────────────────────────────────────── */

  const sorted = [...messages].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );
  const groups = groupMessages(sorted);

  let lastDateKey = "";

  const PlatformGlyph = PLATFORM_GLYPHS[platform ?? ""];

  return (
    <div className="flex flex-col h-full" style={{ background: "#0d0d0d" }}>
      {/* ── Header ─────────────────────────────────────────────── */}
      <div
        className="shrink-0 flex items-center"
        style={{
          height: 48,
          gap: 10,
          padding: "0 16px",
          borderBottom: "1px solid #222222",
          background: "#0d0d0d",
        }}
      >
        {/* Channel name */}
        <div className="flex items-center shrink-0" style={{ gap: 6 }}>
          {platform && (
            <span style={{ color: "#6b6b6b", fontFamily: "'JetBrains Mono', monospace", fontSize: 15, fontWeight: 400 }}>
              #
            </span>
          )}
          <span style={{ fontSize: 14, fontWeight: 600, color: "#e5e5e5", whiteSpace: "nowrap" }}>
            {channelLabel}
          </span>
        </div>

        {/* Platform badge */}
        {platform && PlatformGlyph && (
          <div
            className="flex items-center shrink-0"
            style={{
              gap: 5,
              padding: "2px 7px 2px 6px",
              background: "#1a1a1a",
              borderRadius: 4,
              fontSize: 11,
              color: "#a0a0a0",
              fontWeight: 500,
              whiteSpace: "nowrap",
            }}
          >
            <PlatformGlyph size={11} />
            <span>{platform}</span>
          </div>
        )}

        {/* Message count */}
        <div
          className="flex items-center shrink-0"
          style={{
            gap: 8,
            color: "#6b6b6b",
            fontSize: 11.5,
            fontFamily: "'JetBrains Mono', monospace",
            whiteSpace: "nowrap",
          }}
        >
          <span>{messages.length} msgs</span>
        </div>

        {/* Agents popover trigger */}
        <div className="relative ml-auto shrink-0" ref={agentsPopoverRef}>
          <button
            type="button"
            onClick={() => setShowAgents((v) => !v)}
            className="flex items-center"
            style={{
              gap: 6,
              padding: "3px 8px 3px 6px",
              borderRadius: 5,
              fontSize: 11,
              color: showAgents ? "#e5e5e5" : "#a0a0a0",
              background: showAgents ? "#212121" : "#1a1a1a",
              cursor: "pointer",
              fontFamily: "'JetBrains Mono', monospace",
              userSelect: "none",
              whiteSpace: "nowrap",
              border: "none",
              transition: "background 100ms",
            }}
          >
            {/* Mini avatar stack */}
            {subscribedAgents.length > 0 && (
              <span className="flex">
                {subscribedAgents.slice(0, 3).map((a, i) => (
                  <span
                    key={a.name}
                    className="flex items-center justify-center"
                    style={{
                      width: 16,
                      height: 16,
                      borderRadius: 4,
                      background: agentColor(a.name),
                      marginLeft: i === 0 ? 0 : -5,
                      border: "1.5px solid #0d0d0d",
                      fontSize: 8.5,
                      fontWeight: 700,
                      color: "#0d0d0d",
                      fontFamily: "'JetBrains Mono', monospace",
                    }}
                  >
                    {a.name.slice(0, 2).toUpperCase()}
                  </span>
                ))}
              </span>
            )}
            {(() => {
              const liveCount = subscribedAgents.filter(
                (a) => a.state === "running" || a.state === "working",
              ).length;
              return (
                <span>{liveCount}/{subscribedAgents.length + availableAgents.length} agents</span>
              );
            })()}
            {/* Pulse dot */}
            {subscribedAgents.some(a => a.state === "running" || a.state === "working") && (
              <span
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: 999,
                  background: "#22c55e",
                  boxShadow: "0 0 5px rgba(34,197,94,0.6)",
                }}
              />
            )}
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ marginLeft: 1, color: "#6b6b6b" }}>
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>

          {/* ── Agents popover ─────────────────────────────── */}
          {showAgents && (
            <div
              style={{
                position: "absolute",
                top: 40,
                right: 0,
                width: 420,
                background: "#1a1a1a",
                borderRadius: 10,
                boxShadow: "0 20px 60px rgba(0,0,0,0.6), 0 0 0 1px #2a2a2a",
                zIndex: 50,
                overflow: "hidden",
                display: "flex",
                flexDirection: "column",
                maxHeight: 520,
                animation: "fadeIn 120ms ease-out",
              }}
              onClick={(e) => e.stopPropagation()}
            >
              {/* Popover header */}
              <div
                className="flex items-center"
                style={{
                  padding: "12px 14px 10px",
                  borderBottom: "1px solid #222222",
                  gap: 8,
                }}
              >
                <span style={{ fontSize: 13, fontWeight: 600, color: "#e5e5e5" }}>
                  Subscribed agents
                </span>
                <span
                  className="flex items-center ml-auto"
                  style={{
                    gap: 5,
                    fontSize: 11,
                    color: "#6b6b6b",
                    fontFamily: "'JetBrains Mono', monospace",
                  }}
                >
                  {subscribedAgents.some(a => a.state === "running" || a.state === "working") && (
                    <span
                      style={{
                        width: 6,
                        height: 6,
                        borderRadius: 999,
                        background: "#22c55e",
                        boxShadow: "0 0 5px rgba(34,197,94,0.6)",
                      }}
                    />
                  )}
                  <span>
                    {subscribedAgents.filter(a => a.state === "running" || a.state === "working").length} live
                    {" "}· {agents.length} total
                  </span>
                </span>
              </div>

              {/* Loading skeleton */}
              {popoverLoading && agents.length === 0 && (
                <div className="p-3 space-y-3 animate-pulse">
                  {[...Array(4)].map((_, i) => (
                    <div key={i} className="flex items-center gap-2">
                      <div className="w-7 h-7 rounded-md" style={{ background: "#212121" }} />
                      <div className="h-3 rounded" style={{ background: "#212121", width: `${50 + i * 12}%` }} />
                    </div>
                  ))}
                </div>
              )}

              {/* Agent list */}
              <div className="flex-1 overflow-auto" style={{ padding: "4px 0 8px", scrollbarWidth: "thin", scrollbarColor: "#2a2a2a transparent" }}>
                <AnimatePresence>
                  {/* Subscribed agents section */}
                  {subscribedAgents.length > 0 && (
                    <div>
                      <div
                        className="flex items-center"
                        style={{
                          padding: "10px 14px 4px",
                          fontSize: 10,
                          color: "#6b6b6b",
                          textTransform: "uppercase",
                          letterSpacing: 0.6,
                          fontWeight: 600,
                          gap: 6,
                        }}
                      >
                        <span style={{ width: 5, height: 5, borderRadius: 999, background: "#22c55e" }} />
                        <span>Listening</span>
                        <span className="ml-auto" style={{ color: "#4a4a4a", fontFamily: "'JetBrains Mono', monospace", fontWeight: 400 }}>
                          {subscribedAgents.length}
                        </span>
                      </div>
                      {subscribedAgents.map((agent) => {
                        const sub = subMap.get(agent.name);
                        const isOnline = agent.state === "running" || agent.state === "working";
                        const color = agentColor(agent.name);
                        return (
                          <motion.div
                            key={agent.name}
                            layout
                            initial={{ opacity: 0, x: 8 }}
                            animate={{ opacity: 1, x: 0 }}
                            exit={{ opacity: 0, x: -8 }}
                            transition={{ duration: 0.12 }}
                            className="flex"
                            style={{ gap: 10, padding: "8px 14px", cursor: "pointer" }}
                          >
                            {/* Agent avatar */}
                            <div className="relative" style={{ width: 28, height: 28, minWidth: 28 }}>
                              <span
                                className="flex items-center justify-center"
                                style={{
                                  width: 28,
                                  height: 28,
                                  borderRadius: 6,
                                  background: color,
                                  color: "#0d0d0d",
                                  fontWeight: 700,
                                  fontSize: 10.5,
                                  fontFamily: "'JetBrains Mono', monospace",
                                }}
                              >
                                {agent.name.slice(0, 2).toUpperCase()}
                              </span>
                              <span
                                style={{
                                  position: "absolute",
                                  bottom: -1,
                                  right: -1,
                                  width: 8,
                                  height: 8,
                                  borderRadius: 999,
                                  background: isOnline ? "#22c55e" : agent.state === "idle" ? "#f59e0b" : "#4a4a4a",
                                  border: "2px solid #1a1a1a",
                                  boxSizing: "content-box",
                                }}
                              />
                            </div>
                            {/* Agent info */}
                            <div className="flex-1 min-w-0">
                              <div className="flex items-baseline" style={{ gap: 6 }}>
                                <span style={{ fontSize: 12.5, fontWeight: 600, color: "#e5e5e5", fontFamily: "'JetBrains Mono', monospace" }}>
                                  {agent.name}
                                </span>
                                <span
                                  style={{
                                    fontSize: 10,
                                    color: "#6b6b6b",
                                    fontFamily: "'JetBrains Mono', monospace",
                                    padding: "0 5px",
                                    background: "#212121",
                                    borderRadius: 3,
                                    lineHeight: "14px",
                                  }}
                                >
                                  {agent.role}
                                </span>
                                <span className="ml-auto" style={{ fontSize: 10.5, color: "#6b6b6b", fontFamily: "'JetBrains Mono', monospace" }}>
                                  {agent.state}
                                </span>
                              </div>
                              <div className="flex items-center mt-1" style={{ gap: 6 }}>
                                <button
                                  type="button"
                                  onClick={() => handleToggleMention(agent.name, sub?.mention_only ?? false)}
                                  disabled={agentLoading !== null}
                                  className="transition-all"
                                  style={{
                                    fontSize: 9.5,
                                    padding: "1px 6px",
                                    borderRadius: 3,
                                    border: sub?.mention_only ? "1px solid rgba(249,115,22,0.3)" : "1px solid #2a2a2a",
                                    background: sub?.mention_only ? "rgba(249,115,22,0.08)" : "transparent",
                                    color: sub?.mention_only ? "#f97316" : "#6b6b6b",
                                    cursor: agentLoading === agent.name ? "wait" : "pointer",
                                    fontFamily: "'JetBrains Mono', monospace",
                                  }}
                                >
                                  {agentLoading === agent.name ? (
                                    <span className="inline-block w-3 h-3 border border-current border-t-transparent rounded-full animate-spin" />
                                  ) : sub?.mention_only ? "@ mentions" : "all msgs"}
                                </button>
                                <button
                                  type="button"
                                  onClick={() => handleUnsubscribe(agent.name)}
                                  disabled={agentLoading !== null}
                                  style={{
                                    fontSize: 9.5,
                                    color: "#4a4a4a",
                                    cursor: agentLoading === agent.name ? "wait" : "pointer",
                                    background: "none",
                                    border: "none",
                                    marginLeft: "auto",
                                    fontFamily: "'JetBrains Mono', monospace",
                                  }}
                                  className="hover:text-red-400 transition-colors"
                                >
                                  {agentLoading === agent.name ? (
                                    <span className="inline-block w-2.5 h-2.5 border border-current border-t-transparent rounded-full animate-spin" />
                                  ) : "remove"}
                                </button>
                              </div>
                            </div>
                          </motion.div>
                        );
                      })}
                    </div>
                  )}

                  {/* Divider */}
                  {subscribedAgents.length > 0 && availableAgents.length > 0 && (
                    <div style={{ margin: "4px 14px", borderTop: "1px solid #222222" }} />
                  )}

                  {/* Available agents */}
                  {availableAgents.length > 0 && (
                    <div>
                      <div
                        className="flex items-center"
                        style={{
                          padding: "10px 14px 4px",
                          fontSize: 10,
                          color: "#6b6b6b",
                          textTransform: "uppercase",
                          letterSpacing: 0.6,
                          fontWeight: 600,
                          gap: 6,
                        }}
                      >
                        <span style={{ width: 5, height: 5, borderRadius: 999, background: "#4a4a4a" }} />
                        <span>Available</span>
                        <span className="ml-auto" style={{ color: "#4a4a4a", fontFamily: "'JetBrains Mono', monospace", fontWeight: 400 }}>
                          {availableAgents.length}
                        </span>
                      </div>
                      {availableAgents.map((agent) => {
                        const isOnline = agent.state === "running" || agent.state === "working";
                        const color = agentColor(agent.name);
                        return (
                          <motion.div
                            key={agent.name}
                            layout
                            initial={{ opacity: 0, x: 8 }}
                            animate={{ opacity: 1, x: 0 }}
                            exit={{ opacity: 0, x: -8 }}
                            transition={{ duration: 0.12 }}
                            className="flex items-center hover:bg-white/[0.03] transition-colors"
                            style={{ gap: 10, padding: "8px 14px", cursor: "pointer" }}
                          >
                            <div className="relative" style={{ width: 28, height: 28, minWidth: 28 }}>
                              <span
                                className="flex items-center justify-center"
                                style={{
                                  width: 28,
                                  height: 28,
                                  borderRadius: 6,
                                  background: `${color}40`,
                                  color: color,
                                  fontWeight: 700,
                                  fontSize: 10.5,
                                  fontFamily: "'JetBrains Mono', monospace",
                                }}
                              >
                                {agent.name.slice(0, 2).toUpperCase()}
                              </span>
                              <span
                                style={{
                                  position: "absolute",
                                  bottom: -1,
                                  right: -1,
                                  width: 8,
                                  height: 8,
                                  borderRadius: 999,
                                  background: isOnline ? "#22c55e" : agent.state === "idle" ? "#f59e0b" : "#4a4a4a",
                                  border: "2px solid #1a1a1a",
                                  boxSizing: "content-box",
                                }}
                              />
                            </div>
                            <div className="flex-1 min-w-0">
                              <div className="flex items-baseline" style={{ gap: 6 }}>
                                <span style={{ fontSize: 12.5, fontWeight: 600, color: "#a0a0a0", fontFamily: "'JetBrains Mono', monospace" }}>
                                  {agent.name}
                                </span>
                                <span
                                  style={{
                                    fontSize: 10,
                                    color: "#6b6b6b",
                                    fontFamily: "'JetBrains Mono', monospace",
                                    padding: "0 5px",
                                    background: "#212121",
                                    borderRadius: 3,
                                    lineHeight: "14px",
                                  }}
                                >
                                  {agent.role}
                                </span>
                              </div>
                            </div>
                            <button
                              type="button"
                              onClick={() => handleSubscribe(agent.name)}
                              disabled={agentLoading !== null}
                              style={{
                                fontSize: 9.5,
                                color: "#4a4a4a",
                                cursor: agentLoading === agent.name ? "wait" : "pointer",
                                background: "none",
                                border: "none",
                                fontFamily: "'JetBrains Mono', monospace",
                              }}
                              className="hover:text-orange-400 transition-colors"
                            >
                              {agentLoading === agent.name ? (
                                <span className="inline-block w-2.5 h-2.5 border border-current border-t-transparent rounded-full animate-spin" />
                              ) : "+ add"}
                            </button>
                          </motion.div>
                        );
                      })}
                    </div>
                  )}
                </AnimatePresence>

                {agents.length === 0 && !popoverLoading && (
                  <div className="p-6 text-center" style={{ fontSize: 11, color: "#4a4a4a" }}>
                    No agents
                  </div>
                )}
              </div>

              {/* Popover footer */}
              <div
                className="flex"
                style={{
                  padding: "8px 10px",
                  borderTop: "1px solid #222222",
                  gap: 6,
                  background: "#1a1a1a",
                }}
              >
                <button
                  type="button"
                  className="flex items-center justify-center flex-1"
                  style={{
                    padding: "5px 8px",
                    borderRadius: 5,
                    background: "#212121",
                    color: "#a0a0a0",
                    fontSize: 11,
                    fontWeight: 500,
                    cursor: "pointer",
                    border: "none",
                    gap: 5,
                  }}
                >
                  <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <circle cx="6" cy="6" r="2.5" /><circle cx="18" cy="18" r="2.5" /><circle cx="18" cy="6" r="2.5" />
                    <path d="M6 8v8a2 2 0 0 0 2 2h7" /><path d="M18 8.5v7" />
                  </svg>
                  <span>Routing rules</span>
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Header action buttons */}
        <div className="flex items-center shrink-0" style={{ gap: 4 }}>
          {/* Search */}
          <button
            type="button"
            className="flex items-center justify-center"
            style={{ width: 26, height: 26, borderRadius: 6, color: "#6b6b6b", cursor: "pointer", background: "none", border: "none" }}
            title="Search"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
            </svg>
          </button>
          {/* Filter */}
          <button
            type="button"
            className="flex items-center justify-center"
            style={{ width: 26, height: 26, borderRadius: 6, color: "#6b6b6b", cursor: "pointer", background: "none", border: "none" }}
            title="Filter"
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
            </svg>
          </button>
          {/* Settings */}
          <button
            type="button"
            className="flex items-center justify-center"
            style={{ width: 26, height: 26, borderRadius: 6, color: "#6b6b6b", cursor: "pointer", background: "none", border: "none" }}
            title="Settings"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" /><circle cx="5" cy="12" r="1" />
            </svg>
          </button>
        </div>
      </div>

      {/* Channel description / topic bar */}
      {channel?.description && channel.description !== "Gateway channel" && (
        <div
          className="flex items-start"
          style={{
            padding: "10px 18px",
            fontSize: 12,
            color: "#6b6b6b",
            lineHeight: 1.55,
            borderBottom: "1px solid #222222",
            background: "#151515",
            gap: 10,
          }}
        >
          <svg width="11" height="11" viewBox="0 0 24 24" fill="#f97316" stroke="none" style={{ marginTop: 1, flexShrink: 0 }}>
            <path d="M12 17v5M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z" />
          </svg>
          <span>{channel.description}</span>
        </div>
      )}

      {/* ── Message stream ─────────────────────────────────────── */}
      <div className="flex-1 relative">
        <div
          ref={scrollRef}
          className="absolute inset-0 overflow-auto"
          style={{
            scrollbarWidth: "thin",
            scrollbarColor: "#2a2a2a transparent",
          }}
        >
          <div style={{ padding: "14px 0 8px", display: "flex", flexDirection: "column", marginTop: "auto" }}>
            {initialLoading && messages.length === 0 && (
              <div className="space-y-4 py-4 px-5 animate-pulse">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="flex items-start gap-3">
                    <div className="w-7 h-7 rounded-md flex-shrink-0" style={{ background: "#212121" }} />
                    <div className="flex-1 space-y-2">
                      <div className="flex items-center gap-2">
                        <div className="h-3 w-20 rounded" style={{ background: "#1a1a1a" }} />
                        <div className="h-2 w-12 rounded" style={{ background: "#151515" }} />
                      </div>
                      <div className="h-3 rounded" style={{ background: "#151515", width: `${60 + (i * 7) % 30}%` }} />
                    </div>
                  </div>
                ))}
              </div>
            )}
            {!initialLoading && messages.length === 0 && (
              <div className="flex flex-col items-center justify-center py-24 text-center">
                <svg width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="1.2" style={{ color: "#4a4a4a" }} className="mb-4">
                  <path d="M4 16h6m12 0h6M16 4v6m0 12v6" strokeLinecap="round" />
                  <circle cx="16" cy="16" r="3" />
                </svg>
                <h3 style={{ fontSize: 14, fontWeight: 500, color: "#6b6b6b", marginBottom: 4 }}>
                  Waiting for messages
                </h3>
                <p style={{ fontSize: 12, color: "#4a4a4a" }}>
                  Activity from {platform ?? "this channel"} will stream here in real-time.
                </p>
              </div>
            )}

            {/* Beginning of history */}
            {!hasMore && messages.length > 0 && (
              <div className="flex items-center py-6 px-5" style={{ gap: 10 }}>
                <div className="flex-1" style={{ height: 1, background: "#222222" }} />
                <span
                  style={{
                    fontSize: 11,
                    fontWeight: 600,
                    color: "#a0a0a0",
                    background: "#1a1a1a",
                    padding: "3px 10px",
                    borderRadius: 999,
                  }}
                >
                  Beginning of history
                </span>
                <div className="flex-1" style={{ height: 1, background: "#222222" }} />
              </div>
            )}
            {hasMore && (
              <div ref={sentinelRef} className="py-4 text-center">
                {loadingMore ? (
                  <span style={{ fontSize: 10, color: "#4a4a4a" }}>Loading older messages...</span>
                ) : (
                  <span style={{ fontSize: 10, color: "#4a4a4a" }}>Scroll up for more</span>
                )}
              </div>
            )}

            {/* Message groups */}
            {groups.map((group, gi) => {
              const dk = dateKey(group.timestamp);
              const showDateSep = dk !== lastDateKey;
              lastDateKey = dk;

              return (
                <div key={group.messages[0]?.id ?? gi}>
                  {/* Date separator */}
                  {showDateSep && (
                    <div
                      className="flex items-center"
                      style={{
                        gap: 10,
                        padding: "14px 18px",
                        position: "sticky",
                        top: 0,
                        background: "linear-gradient(#0d0d0d 40%, transparent)",
                        zIndex: 2,
                      }}
                    >
                      <div className="flex-1" style={{ height: 1, background: "#222222" }} />
                      <div
                        style={{
                          fontSize: 11,
                          fontWeight: 600,
                          color: "#a0a0a0",
                          background: "#1a1a1a",
                          padding: "3px 10px",
                          borderRadius: 999,
                        }}
                      >
                        {formatDayLabel(group.timestamp)}
                      </div>
                      <div className="flex-1" style={{ height: 1, background: "#222222" }} />
                    </div>
                  )}

                  {/* Message group — first message with avatar, subsequent without */}
                  <div style={{ paddingTop: 10 }}>
                    {/* Sender line with avatar */}
                    <div className="flex" style={{ padding: "0 18px", gap: 10 }}>
                      {/* Square avatar */}
                      <span
                        className="flex items-center justify-center shrink-0"
                        style={{
                          width: 30,
                          height: 30,
                          minWidth: 30,
                          borderRadius: 6,
                          background: agentColor(group.sender),
                          color: "#0d0d0d",
                          fontWeight: 700,
                          fontSize: 10.5,
                          fontFamily: "'JetBrains Mono', monospace",
                          marginTop: 2,
                        }}
                      >
                        {senderInitials(group.sender)}
                      </span>
                      <div className="flex-1 min-w-0">
                        {/* Name row */}
                        <div className="flex items-baseline flex-wrap" style={{ gap: 7, marginBottom: 1 }}>
                          <button
                            type="button"
                            onClick={() => onPeekAgent(group.sender)}
                            className="hover:underline cursor-pointer decoration-1 underline-offset-2"
                            style={{
                              fontSize: 13.5,
                              fontWeight: 600,
                              color: "#e5e5e5",
                              fontFamily: "'JetBrains Mono', monospace",
                              background: "none",
                              border: "none",
                              padding: 0,
                            }}
                          >
                            {cleanSender(group.sender)}
                          </button>
                          <span
                            style={{
                              fontSize: 11,
                              color: "#6b6b6b",
                              fontFamily: "'JetBrains Mono', monospace",
                            }}
                            title={new Date(group.timestamp).toLocaleString()}
                          >
                            {formatRelativeTime(group.timestamp)}
                          </span>
                        </div>

                        {/* Messages in group */}
                        {group.messages.map((msg) => {
                          const preview = msg.content.slice(0, 120);
                          const msgDeliveries = deliveryByPreview.get(preview) ?? [];
                          const delivered = msgDeliveries.filter((d) => d.status === "delivered");
                          const failed = msgDeliveries.filter((d) => d.status === "failed");
                          const hasDelivery = delivered.length > 0 || failed.length > 0;
                          const ghCard = platform === "github" ? parseGitHubCard(msg.content) : null;

                          return (
                            <div key={msg.id} className="group/msg relative">
                              <div
                                className="rounded-md transition-colors duration-100 hover:bg-white/[0.02]"
                                style={{ padding: "2px 0" }}
                              >
                                {ghCard ? (
                                  <GitHubCardView card={ghCard} />
                                ) : (
                                  <div
                                    className="whitespace-pre-wrap break-words"
                                    style={{
                                      fontSize: 13.5,
                                      color: "#e5e5e5",
                                      lineHeight: 1.55,
                                      wordBreak: "break-word",
                                    }}
                                  >
                                    <MessageContent content={msg.content} agentNames={agentNames} />
                                  </div>
                                )}

                                {/* Delivery indicators */}
                                {hasDelivery && (
                                  <div className="hidden group-hover/msg:flex items-center gap-3 mt-0.5" style={{ fontSize: 9 }}>
                                    {delivered.length > 0 && (
                                      <span style={{ color: "rgba(34,197,94,0.5)" }} title={delivered.map((d) => d.agent).join(", ")}>
                                        → {delivered.map((d) => d.agent).join(", ")}
                                      </span>
                                    )}
                                    {failed.length > 0 && (
                                      <span style={{ color: "rgba(239,68,68,0.5)" }} title={failed.map((d) => `${d.agent}: ${d.error ?? "failed"}`).join(", ")}>
                                        ✗ {failed.map((d) => d.agent).join(", ")}
                                      </span>
                                    )}
                                  </div>
                                )}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}

            {/* Bottom spacer */}
            <div className="h-2" />
          </div>
        </div>
      </div>

      {/* ── Footer / Composer area ────────────────────────────── */}
      <div
        className="shrink-0"
        style={{
          padding: "10px 18px 14px",
          borderTop: "1px solid #222222",
          background: "#0d0d0d",
        }}
      >
        <div className="flex items-center justify-between" style={{ fontSize: 10 }}>
          <span style={{ color: "#4a4a4a" }}>
            {platform && (
              <span className="inline-flex items-center" style={{ gap: 6 }}>
                <span
                  style={{
                    width: 5,
                    height: 5,
                    borderRadius: 999,
                    background: "#22c55e",
                  }}
                />
                <span>sending via bc gateway → {platform}</span>
              </span>
            )}
            {!platform && "bc notifications"}
          </span>
          {subAgents.size > 0 && (
            <span style={{ color: "#4a4a4a", fontFamily: "'JetBrains Mono', monospace" }}>
              {subAgents.size} agent{subAgents.size !== 1 ? "s" : ""} subscribed
            </span>
          )}
        </div>
      </div>

      <style>{`@keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: translateY(0); } }`}</style>
    </div>
  );
}
