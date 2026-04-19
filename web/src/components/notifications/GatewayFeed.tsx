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

/** Get the first letter for avatar, stripping platform prefix. */
function senderInitial(sender: string): string {
  return cleanSender(sender).charAt(0).toUpperCase();
}

/* ── Platform colors ─────────────────────────────────────────── */

const PLATFORM_ACCENT: Record<string, string> = {
  slack: "#E01E5A",
  telegram: "#26A5E4",
  discord: "#5865F2",
  github: "#8B949E",
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
    <div className="mt-1 mb-1 border border-bc-border/30 rounded-lg bg-bc-surface/10 px-3 py-2 max-w-md">
      <div className="flex items-start gap-2">
        {icon}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {card.url ? (
              <a
                href={card.url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-[13px] font-medium text-bc-text hover:text-bc-accent hover:underline truncate"
              >
                {card.title}
              </a>
            ) : (
              <span className="text-[13px] font-medium text-bc-text truncate">
                {card.title}
              </span>
            )}
            {card.number && (
              <span className="text-[11px] text-bc-muted/40">#{card.number}</span>
            )}
            {card.status && (
              <span className={`text-[9px] font-semibold uppercase px-1.5 py-0.5 rounded border ${statusClass}`}>
                {card.status}
              </span>
            )}
          </div>
          {card.repo && (
            <span className="text-[10px] text-bc-muted/40">{card.repo}</span>
          )}
          {(card.additions !== undefined || card.deletions !== undefined || card.changedFiles !== undefined) && (
            <div className="flex items-center gap-2 mt-1 text-[10px]">
              {card.changedFiles !== undefined && (
                <span className="text-bc-muted/50">{card.changedFiles} file{card.changedFiles !== 1 ? "s" : ""}</span>
              )}
              {card.additions !== undefined && (
                <span className="text-green-400/70">+{card.additions}</span>
              )}
              {card.deletions !== undefined && (
                <span className="text-red-400/70">-{card.deletions}</span>
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
  const [agentLoading, setAgentLoading] = useState<string | null>(null); // tracks which agent action is in progress
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
  const platformColor = PLATFORM_ACCENT[platform ?? ""] ?? "var(--bc-accent)";
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
    setAgents([]); // clear stale data
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
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
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

  // Load more older messages when scrolling to top (chat-style)
  const loadMore = useCallback(async () => {
    if (loadingMore || !hasMore || messages.length === 0) return;
    setLoadingMore(true);
    try {
      // Find the oldest message by created_at
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

  // IntersectionObserver for infinite scroll (top sentinel for older messages)
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
    // Only auto-scroll if user is near the bottom or on initial load
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

  // Sort agents by availability: working (green) > idle (amber) > stopped (gray)
  const agentSortOrder = (agent: { state: string }) => {
    if (agent.state === "working" || agent.state === "running") return 0;
    if (agent.state === "stopped") return 2;
    return 1; // idle or other states
  };

  const availableAgents = agents
    .filter((a) => !subMap.has(a.name))
    .sort((a, b) => agentSortOrder(a) - agentSortOrder(b) || a.name.localeCompare(b.name));

  /* ── Message grouping (chat order: oldest first, newest at bottom) ── */

  const sorted = [...messages].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );
  const groups = groupMessages(sorted);

  // Day separators
  let lastDateKey = "";

  return (
    <div className="flex flex-col h-full">
      <style>{`@keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: translateY(0); } }`}</style>
      {/* ── Integrated channel header ──────────────────────────── */}
      <div className="shrink-0 px-5 py-2.5 border-b border-bc-border/30 bg-bc-surface/5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            {/* Platform color accent */}
            <div
              className="w-0.5 h-5 rounded-full"
              style={{ backgroundColor: platformColor }}
            />
            <h1 className="text-[15px] font-semibold text-bc-text tracking-tight">
              {platform ? "#" : ""}{channelLabel}
            </h1>
            {platform && (
              <span
                className="text-[9px] font-semibold uppercase tracking-[0.08em] px-1.5 py-0.5 rounded text-white/90"
                style={{
                  backgroundColor: platformColor,
                }}
              >
                {platform}
              </span>
            )}
            <span className="text-[11px] text-bc-muted/50 tabular-nums ml-1">
              {messages.length} message{messages.length !== 1 ? "s" : ""}
            </span>
          </div>

          <div className="flex items-center gap-3 text-[11px]">

            {/* Settings gear (placeholder) */}
            <button
              type="button"
              className="text-bc-muted/30 hover:text-bc-muted/60 transition-colors p-1 rounded"
              title="Channel settings"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
              </svg>
            </button>

            {/* Agents popover trigger with avatar dots */}
            <div className="relative" ref={agentsPopoverRef}>
              <button
                type="button"
                onClick={() => setShowAgents((v) => !v)}
                className={`flex items-center gap-1.5 px-2 py-1 rounded-md border transition-all duration-150 text-[11px] ${
                  showAgents
                    ? "border-bc-accent/40 bg-bc-accent/8 text-bc-accent"
                    : subAgents.size > 0
                    ? "border-bc-success/25 bg-bc-success/5 text-bc-success/70 hover:border-bc-success/40"
                    : "border-bc-border/30 text-bc-muted/50 hover:border-bc-border/50 hover:text-bc-muted"
                }`}
              >
                {/* Stacked avatar dots for subscribed agents */}
                {subscribedAgents.length > 0 && (
                  <span className="flex -space-x-1.5">
                    {subscribedAgents.slice(0, 4).map((a) => (
                      <span
                        key={a.name}
                        className="w-4 h-4 rounded-full flex items-center justify-center text-[7px] font-bold ring-1 ring-bc-bg"
                        style={{
                          backgroundColor: `${agentColor(a.name)}25`,
                          color: agentColor(a.name),
                        }}
                        title={a.name}
                      >
                        {a.name.charAt(0).toUpperCase()}
                      </span>
                    ))}
                    {subscribedAgents.length > 4 && (
                      <span className="w-4 h-4 rounded-full bg-bc-surface/60 text-[7px] text-bc-muted flex items-center justify-center ring-1 ring-bc-bg font-bold">
                        +{subscribedAgents.length - 4}
                      </span>
                    )}
                  </span>
                )}
                {(() => {
                  const liveCount = subscribedAgents.filter(
                    (a) => a.state === "running" || a.state === "working",
                  ).length;
                  return liveCount > 0 ? (
                    <span className="flex items-center gap-1">
                      <span className="w-1 h-1 rounded-full bg-bc-success animate-pulse" />
                      {liveCount} live
                    </span>
                  ) : (
                    <span>{subAgents.size} agent{subAgents.size !== 1 ? "s" : ""}</span>
                  );
                })()}
              </button>

              {/* Agents popover */}
              {showAgents && (
                <div
                  className="absolute right-0 top-full mt-1.5 w-64 bg-bc-bg border border-bc-border/50 rounded-lg shadow-xl z-20 max-h-[400px] overflow-auto"
                  style={{ scrollbarWidth: "thin", scrollbarColor: "rgba(255,255,255,0.04) transparent", animation: "fadeIn 120ms ease-out" }}
                >
                  {/* Popover header */}
                  <div className="px-3 py-2.5 border-b border-bc-border/30">
                    <h3 className="text-[11px] font-bold text-bc-muted/70 uppercase tracking-[0.12em]">
                      Agents
                    </h3>
                  </div>

                  {/* Loading skeleton */}
                  {popoverLoading && agents.length === 0 && (
                    <div className="p-3 space-y-3 animate-pulse">
                      {[...Array(4)].map((_, i) => (
                        <div key={i} className="flex items-center gap-2">
                          <div className="w-1.5 h-1.5 rounded-full bg-bc-surface/40" />
                          <div className="h-3 rounded bg-bc-surface/30" style={{ width: `${50 + i * 12}%` }} />
                        </div>
                      ))}
                    </div>
                  )}

                  <AnimatePresence>
                    {/* Subscribed agents */}
                    {subscribedAgents.length > 0 && (
                      <div>
                        <div className="px-3 pt-2.5 pb-1">
                          <div className="flex items-center gap-1.5">
                            <span className="w-1 h-1 rounded-full bg-bc-success" />
                            <span className="text-[9px] font-bold text-bc-success/70 uppercase tracking-[0.1em]">
                              Listening ({subscribedAgents.length})
                            </span>
                          </div>
                        </div>
                        {subscribedAgents.map((agent) => {
                          const sub = subMap.get(agent.name);
                          const isOnline = agent.state === "running" || agent.state === "working";
                          const nameColor = agentColor(agent.name);
                          return (
                            <motion.div
                              key={agent.name}
                              layout
                              initial={{ opacity: 0, x: 8 }}
                              animate={{ opacity: 1, x: 0 }}
                              exit={{ opacity: 0, x: -8 }}
                              transition={{ duration: 0.12 }}
                              className="px-3 py-1.5 hover:bg-bc-surface/30 transition-colors duration-100"
                            >
                              <div className="flex items-center gap-2">
                                <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${isOnline ? "bg-bc-success" : "bg-bc-muted/20"}`} />
                                <span className="text-[11px] truncate font-medium" style={{ color: nameColor }}>{agent.name}</span>
                                <span className={`text-[9px] ml-auto shrink-0 ${isOnline ? "text-bc-success/60" : "text-bc-muted/30"}`}>
                                  {agent.state}
                                </span>
                              </div>
                              <div className="flex items-center gap-1.5 mt-1 ml-4">
                                <button
                                  type="button"
                                  onClick={() => handleToggleMention(agent.name, sub?.mention_only ?? false)}
                                  disabled={agentLoading !== null}
                                  className={`text-[9px] px-2 py-0.5 rounded-md border transition-all duration-150 ${
                                    agentLoading === agent.name ? "opacity-60 cursor-wait" :
                                    sub?.mention_only
                                      ? "border-bc-accent/30 bg-bc-accent/8 text-bc-accent"
                                      : "border-bc-border/30 text-bc-muted/50 hover:border-bc-border/50 hover:text-bc-muted"
                                  }`}
                                >
                                  {agentLoading === agent.name ? (
                                    <span className="inline-block w-3 h-3 border border-current border-t-transparent rounded-full animate-spin" />
                                  ) : sub?.mention_only ? "@ mentions" : "all msgs"}
                                </button>
                                <button
                                  type="button"
                                  onClick={() => handleUnsubscribe(agent.name)}
                                  disabled={agentLoading !== null}
                                  className="text-[9px] text-bc-muted/25 hover:text-bc-error/60 transition-colors ml-auto"
                                >
                                  {agentLoading === agent.name ? (
                                    <span className="inline-block w-2.5 h-2.5 border border-current border-t-transparent rounded-full animate-spin" />
                                  ) : "remove"}
                                </button>
                              </div>
                            </motion.div>
                          );
                        })}
                      </div>
                    )}

                    {/* Divider */}
                    {subscribedAgents.length > 0 && availableAgents.length > 0 && (
                      <div className="mx-3 my-2 border-t border-bc-border/15" />
                    )}

                    {/* Available agents */}
                    {availableAgents.length > 0 && (
                      <div>
                        <div className="px-3 pt-2 pb-1">
                          <span className="text-[9px] font-bold text-bc-muted/30 uppercase tracking-[0.1em]">
                            Available ({availableAgents.length})
                          </span>
                        </div>
                        {availableAgents.map((agent) => {
                          const isOnline = agent.state === "running" || agent.state === "working";
                          return (
                            <motion.div
                              key={agent.name}
                              layout
                              initial={{ opacity: 0, x: 8 }}
                              animate={{ opacity: 1, x: 0 }}
                              exit={{ opacity: 0, x: -8 }}
                              transition={{ duration: 0.12 }}
                              className="px-3 py-1.5 hover:bg-bc-surface/15 transition-colors duration-100 flex items-center gap-2"
                            >
                              <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${isOnline ? "bg-bc-success" : "bg-bc-muted/20"}`} />
                              <span className="text-[11px] truncate font-medium text-bc-muted/50">{agent.name}</span>
                              <span className={`text-[9px] shrink-0 ${isOnline ? "text-bc-success/60" : "text-bc-muted/20"}`}>
                                {agent.state}
                              </span>
                              <button
                                type="button"
                                onClick={() => handleSubscribe(agent.name)}
                                disabled={agentLoading !== null}
                                className="text-[9px] text-bc-muted/30 hover:text-bc-accent transition-colors ml-auto"
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

                  {agents.length === 0 && (
                    <div className="p-6 text-center text-[11px] text-bc-muted/25">
                      No agents
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>
        {channel?.description && channel.description !== "Gateway channel" && (
          <p className="text-[11px] text-bc-muted/40 mt-1 ml-3">
            {channel.description}
          </p>
        )}
      </div>

      {/* ── Message stream ─────────────────────────────────────── */}
      <div className="flex-1 relative">
        <div
          ref={scrollRef}
          className="absolute inset-0 overflow-auto"
          style={{
            scrollbarWidth: "thin",
            scrollbarColor: "rgba(255,255,255,0.06) transparent",
          }}
        >
          <div className="px-5 py-3">
            {initialLoading && messages.length === 0 && (
              <div className="space-y-4 py-4 animate-pulse">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="flex items-start gap-3">
                    <div className="w-7 h-7 rounded-md bg-bc-surface/40 flex-shrink-0" />
                    <div className="flex-1 space-y-2">
                      <div className="flex items-center gap-2">
                        <div className="h-3 w-20 rounded bg-bc-surface/30" />
                        <div className="h-2 w-12 rounded bg-bc-surface/20" />
                      </div>
                      <div className="h-3 rounded bg-bc-surface/20" style={{ width: `${60 + (i * 7) % 30}%` }} />
                    </div>
                  </div>
                ))}
              </div>
            )}
            {!initialLoading && messages.length === 0 && (
              <div className="flex flex-col items-center justify-center py-24 text-center">
                <svg width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="1.2" className="text-bc-muted/20 mb-4">
                  <path d="M4 16h6m12 0h6M16 4v6m0 12v6" strokeLinecap="round" />
                  <circle cx="16" cy="16" r="3" />
                </svg>
                <h3 className="text-[14px] font-medium text-bc-muted/50 mb-1">Waiting for messages</h3>
                <p className="text-[12px] text-bc-muted/30">
                  Activity from {platform ?? "this channel"} will stream here in real-time.
                </p>
              </div>
            )}

            {/* Load more sentinel — at top for chat-style scroll */}
            {!hasMore && messages.length > 0 && (
              <div className="flex items-center gap-3 py-6">
                <div className="flex-1 h-px bg-bc-border/15" />
                <span className="text-[9px] text-bc-muted/25 uppercase tracking-widest font-medium">
                  Beginning of history
                </span>
                <div className="flex-1 h-px bg-bc-border/15" />
              </div>
            )}
            {hasMore && (
              <div ref={sentinelRef} className="py-4 text-center">
                {loadingMore ? (
                  <span className="text-[10px] text-bc-muted/30">Loading older messages...</span>
                ) : (
                  <span className="text-[10px] text-bc-muted/20">Scroll up for more</span>
                )}
              </div>
            )}

            {/* Message groups — chat order (oldest first, newest at bottom) */}
              {groups.map((group, gi) => {
                const dk = dateKey(group.timestamp);
                const showDateSep = dk !== lastDateKey;
                lastDateKey = dk;

                return (
                  <div key={group.messages[0]?.id ?? gi}>
                    {/* Date separator */}
                    {showDateSep && (
                      <div className="flex items-center gap-3 my-4">
                        <div className="flex-1 h-px bg-bc-border/30" />
                        <span className="text-[10px] font-medium text-bc-muted/40 tracking-wide">
                          {formatDayLabel(group.timestamp)}
                        </span>
                        <div className="flex-1 h-px bg-bc-border/30" />
                      </div>
                    )}

                    {/* Message group */}
                    <div className="mb-3 group/block">
                      {/* Sender line */}
                      <div className="flex items-center gap-2 mb-0.5 pl-1">
                        {/* Agent initial */}
                        <span
                          className="w-5 h-5 rounded-md flex items-center justify-center text-[10px] font-bold shrink-0"
                          style={{
                            backgroundColor: `${agentColor(group.sender)}15`,
                            color: agentColor(group.sender),
                          }}
                        >
                          {senderInitial(group.sender)}
                        </span>
                        <button
                          type="button"
                          onClick={() => onPeekAgent(group.sender)}
                          className="text-[13px] font-semibold hover:underline cursor-pointer decoration-1 underline-offset-2"
                          style={{ color: agentColor(group.sender) }}
                        >
                          {cleanSender(group.sender)}
                        </button>
                        <span className="text-[10px] text-bc-muted/30 tabular-nums" title={new Date(group.timestamp).toLocaleString()}>
                          {formatRelativeTime(group.timestamp)}
                        </span>
                      </div>

                      {/* Messages */}
                      {group.messages.map((msg) => {
                        const preview = msg.content.slice(0, 120);
                        const msgDeliveries =
                          deliveryByPreview.get(preview) ?? [];
                        const delivered = msgDeliveries.filter(
                          (d) => d.status === "delivered",
                        );
                        const failed = msgDeliveries.filter(
                          (d) => d.status === "failed",
                        );
                        const hasDelivery =
                          delivered.length > 0 || failed.length > 0;

                        // GitHub webhook card detection
                        const ghCard = platform === "github" ? parseGitHubCard(msg.content) : null;

                        return (
                          <div key={msg.id} className="group/msg relative">
                            <div className="py-[3px] pl-8 pr-3 rounded-md transition-colors duration-100 hover:bg-bc-surface/40">
                              {ghCard ? (
                                <GitHubCardView card={ghCard} />
                              ) : (
                                <div className="text-[13px] text-bc-text/80 whitespace-pre-wrap break-words leading-[1.65] [word-break:break-word]">
                                  <MessageContent content={msg.content} agentNames={agentNames} />
                                </div>
                              )}

                              {/* Delivery tooltip — hover only */}
                              {hasDelivery && (
                                <div className="hidden group-hover/msg:flex items-center gap-3 mt-0.5 text-[9px]">
                                  {delivered.length > 0 && (
                                    <span className="text-bc-success/50" title={delivered.map((d) => d.agent).join(", ")}>
                                      → {delivered.map((d) => d.agent).join(", ")}
                                    </span>
                                  )}
                                  {failed.length > 0 && (
                                    <span className="text-bc-error/50" title={failed.map((d) => `${d.agent}: ${d.error ?? "failed"}`).join(", ")}>
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
                );
              })}

            {/* Bottom spacer for chat feel */}
            <div className="h-2" />
          </div>
        </div>
      </div>

      {/* ── Footer ─────────────────────────────────────────────── */}
      <div className="shrink-0 px-5 py-2.5 border-t border-bc-border/20 bg-bc-surface/5">
        <div className="flex items-center justify-between text-[10px]">
          <span className="text-bc-muted/40">
            {platform && (
              <span className="inline-flex items-center gap-1.5">
                <span className="w-1 h-1 rounded-full" style={{ backgroundColor: platformColor }} />
                agents respond via platform API
              </span>
            )}
            {!platform && "bc notifications"}
          </span>
          {subAgents.size > 0 && (
            <span className="text-bc-muted/35">
              {subAgents.size} agent{subAgents.size !== 1 ? "s" : ""} subscribed
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
