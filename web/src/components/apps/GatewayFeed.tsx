import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
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
import { useHeaderSlot } from "../../context/HeaderSlotContext";
import {
  gatewayPlatform,
  formatRelativeTime,
  groupMessages,
  dateKey,
  formatDayLabel,
  parseGitHubCard,
  parseRSSCard,
  parseWebhookCard,
} from "./messageUtils";
import { AgentCharacter, LiveAgentCharacter } from "../agent-ui";
import { IdentityAvatar } from "./IdentityAvatar";
import type { GitHubCard, RSSCard, WebhookCard } from "./messageUtils";

/* ── Helpers ──────────────────────────────────────────────────── */

/** Strip "[telegram] " or "[slack] " prefix from sender names for cleaner display. */
function cleanSender(sender: string): string {
  const match = sender.match(/^\[(?:telegram|slack|discord)\]\s*(.+)$/i);
  return match?.[1] ?? sender;
}

/* ── File attachment detection ────────────────────────────────── */

interface FileAttachmentInfo {
  type: "photo" | "video" | "document" | "file";
  name?: string;
  size?: string;
  mimetype?: string;
  icon: string;
}

/** Guess icon from mimetype or filename. */
function fileIcon(mimetype?: string, name?: string): string {
  if (mimetype?.startsWith("image/")) return "image";
  if (mimetype?.startsWith("video/")) return "video";
  if (name) {
    const ext = name.split(".").pop()?.toLowerCase() ?? "";
    if (["png", "jpg", "jpeg", "gif", "webp", "svg"].includes(ext)) return "image";
    if (["mp4", "mov", "webm", "avi"].includes(ext)) return "video";
  }
  return "file";
}

/** Parse file attachment placeholders from message content.
 *  Telegram adapter adds [photo], [document:filename.ext], [video] etc.
 *  Slack adapter adds lines like: 📎 name (size) [mimetype] */
function parseFileAttachments(content: string): FileAttachmentInfo[] {
  const attachments: FileAttachmentInfo[] = [];
  const photoRe = /\[photo(?::([^\]]*))?\]/gi;
  let m;
  while ((m = photoRe.exec(content)) !== null) {
    attachments.push({ type: "photo", name: m[1] || "Photo", icon: "image" });
  }
  const videoRe = /\[video(?::([^\]]*))?\]/gi;
  while ((m = videoRe.exec(content)) !== null) {
    attachments.push({ type: "video", name: m[1] || "Video", icon: "video" });
  }
  const docRe = /\[(?:document|file):([^\]]+)\]/gi;
  while ((m = docRe.exec(content)) !== null) {
    attachments.push({ type: "document", name: m[1], icon: "file" });
  }
  // Slack file_share: 📎 filename (size) [mimetype]
  const clipRe = /\u{1F4CE}\s+(.+?)\s+\(([^)]+)\)\s+\[([^\]]+)\]/gu;
  while ((m = clipRe.exec(content)) !== null) {
    const name = m[1];
    const mt = m[3] ?? "";
    attachments.push({
      type: mt.startsWith("image/") ? "photo" : mt.startsWith("video/") ? "video" : "file",
      name,
      size: m[2],
      mimetype: mt,
      icon: fileIcon(mt, name),
    });
  }
  return attachments;
}

function FileAttachmentCard({ attachment }: { attachment: FileAttachmentInfo }) {
  const iconMap: Record<string, JSX.Element> = {
    image: (
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8.5" cy="8.5" r="1.5" /><polyline points="21 15 16 10 5 21" />
      </svg>
    ),
    video: (
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <polygon points="23 7 16 12 23 17 23 7" /><rect x="1" y="5" width="15" height="14" rx="2" />
      </svg>
    ),
    file: (
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" /><polyline points="14 2 14 8 20 8" />
      </svg>
    ),
  };

  return (
    <div
      className="inline-flex items-center"
      style={{
        gap: 6,
        padding: "3px 8px",
        marginTop: 4,
        borderRadius: 6,
        background: "var(--mycel-surface-hover)",
        border: "1px solid var(--mycel-border)",
        fontSize: 11,
        color: "var(--mycel-text-2)",
        fontFamily: "'JetBrains Mono', monospace",
      }}
    >
      <span style={{ color: "var(--mycel-muted)", display: "flex" }}>
        {iconMap[attachment.icon] ?? iconMap.file}
      </span>
      <span style={{ color: "var(--mycel-text)" }}>{attachment.name}</span>
      {attachment.size && (
        <span style={{ color: "var(--mycel-muted)", fontSize: 10 }}>({attachment.size})</span>
      )}
    </div>
  );
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

function WhatsAppGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <circle cx="12" cy="12" r="11" fill="#25D366" />
      <text x="12" y="16" textAnchor="middle" fill="#fff" fontSize="10" fontWeight="bold">W</text>
    </svg>
  );
}

function RSSGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" style={{ flexShrink: 0 }}>
      <circle cx="12" cy="12" r="11" fill="#F78422" />
      <circle cx="8" cy="16" r="2" fill="#fff" />
      <path d="M6 12a8 8 0 0 1 8 8" stroke="#fff" strokeWidth="2" fill="none" strokeLinecap="round" />
      <path d="M6 8a12 12 0 0 1 12 12" stroke="#fff" strokeWidth="2" fill="none" strokeLinecap="round" />
    </svg>
  );
}

const PLATFORM_GLYPHS: Record<string, React.FC<{ size?: number }>> = {
  slack: SlackGlyph,
  telegram: TelegramGlyph,
  discord: DiscordGlyph,
  whatsapp: WhatsAppGlyph,
  github: ({ size = 11 }) => <GitHubGlyph size={size} />,
  rss: RSSGlyph,
};

/* ── GitHub card renderer ────────────────────────────────────── */

function GitHubGlyph({ size = 11 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="currentColor" style={{ flexShrink: 0, color: "var(--mycel-muted)" }}>
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}

function GitHubCardView({ card }: { card: GitHubCard }) {
  // Push card — blue border, branch + files changed
  if (card.type === "push") {
    return (
      <div
        className="mt-1.5 max-w-lg overflow-hidden"
        style={{
          background: "var(--mycel-surface)",
          borderRadius: 8,
          borderLeft: "2px solid var(--mycel-info)",
          boxShadow: "var(--mycel-shadow)",
        }}
      >
        <div
          className="flex items-center"
          style={{
            padding: "8px 12px 6px",
            gap: 7,
            fontSize: 10.5,
            color: "var(--mycel-muted)",
            fontFamily: "'JetBrains Mono', monospace",
          }}
        >
          <GitHubGlyph size={11} />
          <span
            style={{
              fontSize: 9.5,
              fontFamily: "'JetBrains Mono', monospace",
              padding: "1px 5px",
              borderRadius: 3,
              background: "var(--mycel-surface-hover)",
              color: "var(--mycel-text-2)",
              textTransform: "uppercase",
              letterSpacing: 0.5,
              fontWeight: 600,
            }}
          >
            push
          </span>
          {card.repo && <span style={{ color: "var(--mycel-muted)" }}>{card.repo}</span>}
          {card.branch && <span style={{ color: "var(--mycel-text-2)", fontWeight: 500 }}>{card.branch}</span>}
        </div>
        <div style={{ padding: "2px 12px 10px" }}>
          <div style={{ fontSize: 13, fontWeight: 500, color: "var(--mycel-text)", lineHeight: 1.4 }}>
            {card.title}
          </div>
          <div
            className="flex flex-wrap"
            style={{
              fontSize: 11,
              color: "var(--mycel-muted)",
              fontFamily: "'JetBrains Mono', monospace",
              marginTop: 4,
              gap: 10,
            }}
          >
            {card.changedFiles !== undefined && (
              <span>{card.changedFiles} file{card.changedFiles !== 1 ? "s" : ""}</span>
            )}
            {card.additions !== undefined && (
              <span style={{ color: "var(--mycel-success)" }}>+{card.additions}</span>
            )}
            {card.deletions !== undefined && (
              <span style={{ color: "var(--mycel-error)" }}>-{card.deletions}</span>
            )}
          </div>
        </div>
      </div>
    );
  }

  // PR / Issue card — colored left border based on state
  const stateColor = card.status === "MERGED" ? "#a855f7" : card.status === "CLOSED" ? "var(--mycel-error)" : "var(--mycel-success)";
  const stateLabel = (card.status ?? "").replace(/_/g, " ");

  return (
    <div
      className="mt-1.5 max-w-lg overflow-hidden"
      style={{
        background: "var(--mycel-surface)",
        borderRadius: 8,
        borderLeft: `2px solid ${stateColor}`,
        boxShadow: "var(--mycel-shadow)",
      }}
    >
      {/* Card header with event type badge */}
      <div
        className="flex items-center"
        style={{
          padding: "8px 12px 6px",
          gap: 7,
          fontSize: 10.5,
          color: "var(--mycel-muted)",
          fontFamily: "'JetBrains Mono', monospace",
        }}
      >
        <GitHubGlyph size={11} />
        <span
          style={{
            fontSize: 9.5,
            fontFamily: "'JetBrains Mono', monospace",
            padding: "1px 5px",
            borderRadius: 3,
            background: "var(--mycel-surface-hover)",
            color: "var(--mycel-text-2)",
            textTransform: "uppercase",
            letterSpacing: 0.5,
            fontWeight: 600,
          }}
        >
          {card.type === "pr" ? "pull_request" : card.type}
        </span>
        {card.repo && <span style={{ color: "var(--mycel-muted)" }}>{card.repo}</span>}
        {card.number != null && <span style={{ color: "var(--mycel-text-2)", fontWeight: 500 }}>#{card.number}</span>}
        {card.status && (
          <span
            style={{
              marginLeft: "auto",
              fontSize: 9.5,
              fontWeight: 700,
              color: stateColor,
              padding: "1px 6px",
              borderRadius: 3,
              background: `color-mix(in oklab, ${stateColor} 14%, transparent)`,
              textTransform: "uppercase",
              letterSpacing: 0.6,
            }}
          >
            {stateLabel}
          </span>
        )}
      </div>
      {/* Card body */}
      <div style={{ padding: "2px 12px 10px" }}>
        <div style={{ fontSize: 13, fontWeight: 500, color: "var(--mycel-text)", lineHeight: 1.4 }}>
          {card.url ? (
            <a
              href={card.url}
              target="_blank"
              rel="noopener noreferrer"
              className="hover:underline"
              style={{ color: "var(--mycel-text)", textDecoration: "none" }}
            >
              {card.title}
            </a>
          ) : (
            card.title
          )}
        </div>
        {(card.additions !== undefined || card.deletions !== undefined || card.changedFiles !== undefined) && (
          <div
            className="flex flex-wrap"
            style={{
              fontSize: 11,
              color: "var(--mycel-muted)",
              fontFamily: "'JetBrains Mono', monospace",
              marginTop: 4,
              gap: 10,
            }}
          >
            {card.changedFiles !== undefined && (
              <span>{card.changedFiles} file{card.changedFiles !== 1 ? "s" : ""}</span>
            )}
            {card.additions !== undefined && (
              <span style={{ color: "var(--mycel-success)" }}>+{card.additions}</span>
            )}
            {card.deletions !== undefined && (
              <span style={{ color: "var(--mycel-error)" }}>-{card.deletions}</span>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

/* ── RSS card renderer ──────────────────────────────────────── */

function RSSCardView({ card }: { card: RSSCard }) {
  const pubDateLabel = card.pubDate && !isNaN(new Date(card.pubDate).getTime())
    ? formatRelativeTime(card.pubDate) : null;
  return (
    <div className="mt-1.5 max-w-lg overflow-hidden" style={{ background: "var(--mycel-surface)", borderRadius: 8, borderLeft: "2px solid #F78422", boxShadow: "var(--mycel-shadow)" }}>
      <div className="flex items-center" style={{ padding: "8px 12px 4px", gap: 7, fontSize: 10.5, color: "var(--mycel-muted)", fontFamily: "'JetBrains Mono', monospace" }}>
        <RSSGlyph size={11} />
        <span style={{ fontSize: 9.5, padding: "1px 5px", borderRadius: 3, background: "var(--mycel-surface-hover)", color: "var(--mycel-text-2)", textTransform: "uppercase", letterSpacing: 0.5, fontWeight: 600 }}>feed</span>
        {card.source && <span>{card.source}</span>}
        {pubDateLabel && <span style={{ marginLeft: "auto" }}>{pubDateLabel}</span>}
      </div>
      <div style={{ padding: "4px 12px 10px" }}>
        <div style={{ fontSize: 13, fontWeight: 500, color: "var(--mycel-text)", lineHeight: 1.4 }}>
          {card.link ? (<a href={card.link} target="_blank" rel="noopener noreferrer" className="hover:underline" style={{ color: "var(--mycel-text)", textDecoration: "none" }}>{card.title}</a>) : card.title}
        </div>
        {card.description && <div style={{ fontSize: 12, color: "var(--mycel-text-2)", marginTop: 4, lineHeight: 1.4, maxHeight: 60, overflow: "hidden" }}>{card.description.slice(0, 200)}</div>}
      </div>
    </div>
  );
}

/* ── Webhook JSON card renderer ────────────────────────────── */

function WebhookCardView({ card }: { card: WebhookCard }) {
  const [expanded, setExpanded] = useState(false);
  const preview = useMemo(() => JSON.stringify(card.payload, null, 2), [card.payload]);
  const short = preview.slice(0, 200);
  return (
    <div className="mt-1.5 max-w-lg overflow-hidden" style={{ background: "var(--mycel-surface)", borderRadius: 8, borderLeft: "2px solid var(--mycel-muted)", boxShadow: "var(--mycel-shadow)" }}>
      <div className="flex items-center" style={{ padding: "8px 12px 4px", gap: 7, fontSize: 10.5, color: "var(--mycel-muted)", fontFamily: "'JetBrains Mono', monospace" }}>
        {card.event && <span style={{ fontSize: 9.5, padding: "1px 5px", borderRadius: 3, background: "var(--mycel-surface-hover)", color: "var(--mycel-text-2)", textTransform: "uppercase", letterSpacing: 0.5, fontWeight: 600 }}>{card.event}</span>}
        {card.action && <span style={{ color: "var(--mycel-text-2)", fontWeight: 500 }}>{card.action}</span>}
      </div>
      <div style={{ padding: "4px 12px 10px" }}>
        <pre style={{ fontSize: 11, color: "var(--mycel-text-2)", fontFamily: "'JetBrains Mono', monospace", lineHeight: 1.4, whiteSpace: "pre-wrap", wordBreak: "break-all", maxHeight: expanded ? "none" : 80, overflow: "hidden", margin: 0 }}>
          {expanded ? preview : short}
        </pre>
        {preview.length > 200 && (
          <button type="button" onClick={() => setExpanded((v) => !v)} style={{ fontSize: 11, fontWeight: 500, color: "var(--mycel-accent)", background: "none", border: "none", cursor: "pointer", padding: "4px 0 0" }}>
            {expanded ? "collapse" : "expand JSON..."}
          </button>
        )}
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
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [filterOpen, setFilterOpen] = useState(false);
  const [filterAgent, setFilterAgent] = useState<string | null>(null);
  const [topicDismissed, setTopicDismissed] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const agentsPopoverRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const filterRef = useRef<HTMLDivElement>(null);
  const { subscribe } = useWebSocket();

  const navigate = useNavigate();
  const platform = gatewayPlatform(channelName);
  // Show the leaf channel segment as the visible label. Internal keys carry
  // the full path (e.g. "discord:server:general"); only the final segment is
  // meaningful to humans.
  const channelLabel = channelName.includes(":")
    ? (channelName.split(":").pop() || channelName)
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
    } catch (e) { console.error("fetchAgents failed:", e); }
    setPopoverLoading(false);
  }, [channelName]);

  // Fetch agents on mount to populate the counter, then refresh when popover opens.
  useEffect(() => {
    void fetchAgents();
  }, [fetchAgents]);

  useEffect(() => {
    if (!showAgents) return;
    setPopoverLoading(true);
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

  // Close filter popover on outside click
  useEffect(() => {
    if (!filterOpen) return;
    const handleClick = (e: MouseEvent) => {
      if (filterRef.current && !filterRef.current.contains(e.target as Node)) {
        setFilterOpen(false);
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setFilterOpen(false);
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [filterOpen]);

  // Focus search input when opened
  useEffect(() => {
    if (searchOpen) searchInputRef.current?.focus();
  }, [searchOpen]);

  const handleSubscribe = async (agentName: string) => {
    setAgentLoading(agentName);
    try {
      await api.subscribe(channelName, agentName, false);
      await fetchAgents();
    } catch (e) { console.error("subscribe failed:", e); }
    setAgentLoading(null);
  };

  const handleUnsubscribe = async (agentName: string) => {
    setAgentLoading(agentName);
    try {
      await api.unsubscribe(channelName, agentName);
      await fetchAgents();
    } catch (e) { console.error("unsubscribe failed:", e); }
    setAgentLoading(null);
  };

  const handleToggleMention = async (agentName: string, current: boolean) => {
    setAgentLoading(agentName);
    try {
      await api.setMentionOnly(channelName, agentName, !current);
      await fetchAgents();
    } catch (e) { console.error("toggle mention failed:", e); }
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
    } catch (e) { console.error("loadMore failed:", e); }
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

  /* ── Message filtering & grouping ──────────────────────────── */

  const filteredMessages = useMemo(() => {
    let msgs = messages;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      msgs = msgs.filter(
        (m) =>
          m.content.toLowerCase().includes(q) ||
          m.sender.toLowerCase().includes(q),
      );
    }
    if (filterAgent) {
      msgs = msgs.filter((m) => m.sender === filterAgent);
    }
    return msgs;
  }, [messages, searchQuery, filterAgent]);

  const sorted = [...filteredMessages].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );
  const groups = groupMessages(sorted);

  let lastDateKey = "";

  const PlatformGlyph = PLATFORM_GLYPHS[platform ?? ""];

  /* ── Unique senders for filter dropdown ──────────────────── */
  const uniqueSenders = useMemo(() => {
    const senders = new Set(messages.map((m) => m.sender));
    return [...senders].sort();
  }, [messages]);

  /* ── Header slot — injects into Layout header ──────────── */
  const liveCount = subscribedAgents.filter(
    (a) => a.state === "running" || a.state === "working",
  ).length;

  const headerTitle = useMemo(
    () => (
      <div className="flex items-center min-w-0" style={{ gap: 8 }}>
        <button
          type="button"
          onClick={() => navigate(-1)}
          className="flex items-center justify-center shrink-0"
          style={{
            width: 22,
            height: 22,
            borderRadius: 5,
            color: "var(--mycel-muted)",
            cursor: "pointer",
            background: "none",
            border: "none",
          }}
          title="Go back"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
        <span className="shrink-0" style={{ color: "var(--mycel-muted)", fontSize: 13 }}>#</span>
        <span
          className="truncate min-w-0"
          title={channelLabel}
          style={{ fontSize: 13, fontWeight: 600, color: "var(--mycel-text)" }}
        >
          {channelLabel}
        </span>
        {platform && PlatformGlyph && (
          <div
            className="hidden sm:flex items-center shrink-0"
            style={{
              gap: 4,
              padding: "1px 6px",
              background: "var(--mycel-surface-hover)",
              borderRadius: 4,
              fontSize: 10,
              color: "var(--mycel-text-2)",
              fontWeight: 500,
            }}
          >
            <PlatformGlyph size={10} />
            <span>{platform}</span>
          </div>
        )}
        <span
          className="hidden sm:inline shrink-0"
          style={{
            color: "var(--mycel-muted)",
            fontSize: 11,
            fontVariantNumeric: "tabular-nums",
          }}
        >
          {messages.length} msgs
        </span>
      </div>
    ),
    [channelLabel, platform, PlatformGlyph, messages.length, navigate],
  );

  const headerActions = useMemo(
    () => (
      <div className="flex items-center" style={{ gap: 4 }}>
        {/* Agents popover trigger */}
        <div className="relative">
          <button
            type="button"
            onClick={() => setShowAgents((v) => !v)}
            className="flex items-center"
            title={`${liveCount} live / ${subscribedAgents.length + availableAgents.length} total agents — click to manage subscriptions`}
            aria-label={`${liveCount} of ${subscribedAgents.length + availableAgents.length} agents live; manage subscriptions`}
            style={{
              gap: 5,
              padding: "3px 8px 3px 6px",
              borderRadius: 6,
              fontSize: 10.5,
              fontWeight: 500,
              color: showAgents ? "var(--mycel-text)" : "var(--mycel-text-2)",
              background: "var(--mycel-surface-hover)",
              cursor: "pointer",
              fontVariantNumeric: "tabular-nums",
              userSelect: "none",
              whiteSpace: "nowrap",
              border: "none",
              transition: "background 100ms",
            }}
          >
            {subscribedAgents.length > 0 && (
              <span className="flex items-center">
                {subscribedAgents.slice(0, 3).map((a, i) => (
                  <span key={a.name} style={{ marginLeft: i === 0 ? 0 : -4 }}>
                    <AgentCharacter name={a.name} state={a.state} size={16} />
                  </span>
                ))}
              </span>
            )}
            <span>{liveCount}/{subscribedAgents.length + availableAgents.length} agents</span>
            {subscribedAgents.some(a => a.state === "running" || a.state === "working") && (
              <span style={{ width: 5, height: 5, borderRadius: 999, background: "var(--mycel-success)", boxShadow: "0 0 5px color-mix(in oklab, var(--mycel-success) 60%, transparent)" }} />
            )}
            <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ color: "var(--mycel-muted)" }}>
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
        </div>

        {/* Search toggle */}
        <button
          type="button"
          onClick={() => { setSearchOpen((v) => !v); if (searchOpen) setSearchQuery(""); }}
          className="flex items-center justify-center"
          style={{
            width: 26,
            height: 26,
            borderRadius: 6,
            color: searchOpen ? "var(--mycel-accent)" : "var(--mycel-muted)",
            cursor: "pointer",
            background: searchOpen ? "var(--mycel-accent-subtle)" : "none",
            border: "none",
          }}
          title="Search messages"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
        </button>

        {/* Filter toggle */}
        <div className="relative" ref={filterRef}>
          <button
            type="button"
            onClick={() => setFilterOpen((v) => !v)}
            className="flex items-center justify-center"
            style={{
              width: 26,
              height: 26,
              borderRadius: 6,
              color: filterAgent ? "var(--mycel-accent)" : filterOpen ? "var(--mycel-text)" : "var(--mycel-muted)",
              cursor: "pointer",
              background: filterAgent ? "var(--mycel-accent-subtle)" : "none",
              border: "none",
            }}
            title="Filter messages"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
            </svg>
          </button>
        </div>
      </div>
    ),
     
    [showAgents, searchOpen, filterOpen, filterAgent, liveCount, subscribedAgents, availableAgents],
  );

  useHeaderSlot({ title: headerTitle, actions: headerActions });

  return (
    <div className="flex flex-col h-full relative" style={{ background: "var(--mycel-bg)" }}>
      {/* ── Agents popover (overlays feed) ────────────────────── */}
      {showAgents && (
        <div
          ref={agentsPopoverRef}
          style={{
            position: "absolute",
            top: 4,
            right: 16,
            width: 420,
            background: "var(--mycel-surface-2)",
            borderRadius: 8,
            boxShadow: "var(--mycel-shadow-lg), 0 0 0 1px var(--mycel-border)",
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
            style={{ padding: "12px 14px 10px", borderBottom: "1px solid var(--mycel-border)", gap: 8 }}
          >
            <span style={{ fontSize: 13, fontWeight: 600, color: "var(--mycel-text)" }}>Subscribed agents</span>
            <span className="flex items-center ml-auto" style={{ gap: 5, fontSize: 11, color: "var(--mycel-muted)", fontVariantNumeric: "tabular-nums" }}>
              {subscribedAgents.some(a => a.state === "running" || a.state === "working") && (
                <span style={{ width: 6, height: 6, borderRadius: 999, background: "var(--mycel-success)", boxShadow: "0 0 5px color-mix(in oklab, var(--mycel-success) 60%, transparent)" }} />
              )}
              <span>{liveCount} live · {agents.length} total</span>
            </span>
          </div>

          {/* Loading skeleton */}
          {popoverLoading && agents.length === 0 && (
            <div className="p-3 space-y-3 animate-pulse">
              {[...Array(4)].map((_, i) => (
                <div key={i} className="flex items-center gap-2">
                  <div className="w-7 h-7 rounded-md" style={{ background: "var(--mycel-surface-hover)" }} />
                  <div className="h-3 rounded" style={{ background: "var(--mycel-surface-hover)", width: `${50 + i * 12}%` }} />
                </div>
              ))}
            </div>
          )}

          {/* Agent list */}
          <div className="flex-1 overflow-auto" style={{ padding: "4px 0 8px", scrollbarWidth: "thin", scrollbarColor: "var(--mycel-border) transparent" }}>
            <AnimatePresence>
              {subscribedAgents.length > 0 && (
                <div>
                  <div className="flex items-center" style={{ padding: "10px 14px 4px", fontSize: 11, color: "var(--mycel-muted)", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 500, gap: 6 }}>
                    <span style={{ width: 5, height: 5, borderRadius: 999, background: "var(--mycel-success)" }} />
                    <span>Listening</span>
                    <span className="ml-auto" style={{ color: "var(--mycel-muted)", fontVariantNumeric: "tabular-nums", fontWeight: 400 }}>{subscribedAgents.length}</span>
                  </div>
                  {subscribedAgents.map((agent) => {
                    const sub = subMap.get(agent.name);
                    const isOnline = agent.state === "running" || agent.state === "working";
                    return (
                      <motion.div key={agent.name} layout initial={{ opacity: 0, x: 8 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -8 }} transition={{ duration: 0.12 }} className="flex" style={{ gap: 10, padding: "8px 14px", cursor: "pointer" }}>
                        <div className="relative" style={{ width: 28, height: 28, minWidth: 28 }}>
                          <LiveAgentCharacter name={agent.name} state={agent.state} size={28} />
                          <span style={{ position: "absolute", bottom: -1, right: -1, width: 8, height: 8, borderRadius: 999, background: isOnline ? "var(--mycel-success)" : agent.state === "idle" ? "var(--mycel-warning)" : "var(--mycel-muted)", border: "2px solid var(--mycel-surface-2)", boxSizing: "content-box" }} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-baseline" style={{ gap: 6 }}>
                            <span style={{ fontSize: 12.5, fontWeight: 600, color: "var(--mycel-text)", fontFamily: "'JetBrains Mono', monospace" }}>{agent.name}</span>
                            <span style={{ fontSize: 10, color: "var(--mycel-muted)", padding: "0 5px", background: "var(--mycel-surface-hover)", borderRadius: 3, lineHeight: "14px" }}>{agent.role}</span>
                            <span className="ml-auto" style={{ fontSize: 10.5, color: "var(--mycel-muted)" }}>{agent.state}</span>
                          </div>
                          <div className="flex items-center mt-1" style={{ gap: 6 }}>
                            <button type="button" onClick={() => handleToggleMention(agent.name, sub?.mention_only ?? false)} disabled={agentLoading !== null} className="transition-all" style={{ fontSize: 10, fontWeight: 500, padding: "1px 6px", borderRadius: 6, border: sub?.mention_only ? "1px solid color-mix(in oklab, var(--mycel-accent) 30%, transparent)" : "1px solid var(--mycel-border)", background: sub?.mention_only ? "var(--mycel-accent-subtle)" : "transparent", color: sub?.mention_only ? "var(--mycel-accent)" : "var(--mycel-muted)", cursor: agentLoading === agent.name ? "wait" : "pointer" }}>
                              {agentLoading === agent.name ? <span className="inline-block w-3 h-3 border border-current border-t-transparent rounded-full animate-spin" /> : sub?.mention_only ? "@ mentions" : "all msgs"}
                            </button>
                            <button type="button" onClick={() => handleUnsubscribe(agent.name)} disabled={agentLoading !== null} style={{ fontSize: 10, fontWeight: 500, color: "var(--mycel-muted)", cursor: agentLoading === agent.name ? "wait" : "pointer", background: "none", border: "none", marginLeft: "auto" }} className="hover:text-mycel-error transition-colors">
                              {agentLoading === agent.name ? <span className="inline-block w-2.5 h-2.5 border border-current border-t-transparent rounded-full animate-spin" /> : "remove"}
                            </button>
                          </div>
                        </div>
                      </motion.div>
                    );
                  })}
                </div>
              )}
              {subscribedAgents.length > 0 && availableAgents.length > 0 && (
                <div style={{ margin: "4px 14px", borderTop: "1px solid var(--mycel-border)" }} />
              )}
              {availableAgents.length > 0 && (
                <div>
                  <div className="flex items-center" style={{ padding: "10px 14px 4px", fontSize: 11, color: "var(--mycel-muted)", textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 500, gap: 6 }}>
                    <span style={{ width: 5, height: 5, borderRadius: 999, background: "var(--mycel-muted)" }} />
                    <span>Available</span>
                    <span className="ml-auto" style={{ color: "var(--mycel-muted)", fontVariantNumeric: "tabular-nums", fontWeight: 400 }}>{availableAgents.length}</span>
                  </div>
                  {availableAgents.map((agent) => {
                    const isOnline = agent.state === "running" || agent.state === "working";
                    return (
                      <motion.div key={agent.name} layout initial={{ opacity: 0, x: 8 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: -8 }} transition={{ duration: 0.12 }} className="flex items-center hover:bg-mycel-surface-hover transition-colors" style={{ gap: 10, padding: "8px 14px", cursor: "pointer", opacity: 0.75 }}>
                        <div className="relative" style={{ width: 28, height: 28, minWidth: 28 }}>
                          <LiveAgentCharacter name={agent.name} state={agent.state} size={28} />
                          <span style={{ position: "absolute", bottom: -1, right: -1, width: 8, height: 8, borderRadius: 999, background: isOnline ? "var(--mycel-success)" : agent.state === "idle" ? "var(--mycel-warning)" : "var(--mycel-muted)", border: "2px solid var(--mycel-surface-2)", boxSizing: "content-box" }} />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-baseline" style={{ gap: 6 }}>
                            <span style={{ fontSize: 12.5, fontWeight: 600, color: "var(--mycel-text-2)", fontFamily: "'JetBrains Mono', monospace" }}>{agent.name}</span>
                            <span style={{ fontSize: 10, color: "var(--mycel-muted)", padding: "0 5px", background: "var(--mycel-surface-hover)", borderRadius: 3, lineHeight: "14px" }}>{agent.role}</span>
                          </div>
                        </div>
                        <button type="button" onClick={() => handleSubscribe(agent.name)} disabled={agentLoading !== null} style={{ fontSize: 10, fontWeight: 500, color: "var(--mycel-muted)", cursor: agentLoading === agent.name ? "wait" : "pointer", background: "none", border: "none" }} className="hover:text-mycel-accent transition-colors">
                          {agentLoading === agent.name ? <span className="inline-block w-2.5 h-2.5 border border-current border-t-transparent rounded-full animate-spin" /> : "+ add"}
                        </button>
                      </motion.div>
                    );
                  })}
                </div>
              )}
            </AnimatePresence>
            {agents.length === 0 && !popoverLoading && (
              <div className="p-6 text-center" style={{ fontSize: 11, color: "var(--mycel-muted)" }}>No agents</div>
            )}
          </div>

        </div>
      )}

      {/* ── Filter popover (overlays feed) ────────────────────── */}
      {filterOpen && (
        <div
          style={{
            position: "absolute",
            top: 4,
            right: 60,
            width: 220,
            background: "var(--mycel-surface-2)",
            borderRadius: 8,
            boxShadow: "var(--mycel-shadow-lg), 0 0 0 1px var(--mycel-border)",
            zIndex: 50,
            overflow: "hidden",
            animation: "fadeIn 120ms ease-out",
          }}
          onClick={(e) => e.stopPropagation()}
        >
          <div style={{ padding: "10px 12px 6px", fontSize: 11, fontWeight: 600, color: "var(--mycel-text-2)", textTransform: "uppercase", letterSpacing: 0.5 }}>
            Filter by sender
          </div>
          <div style={{ maxHeight: 200, overflowY: "auto", padding: "0 6px 8px", scrollbarWidth: "thin", scrollbarColor: "var(--mycel-border) transparent" }}>
            <button
              type="button"
              onClick={() => { setFilterAgent(null); setFilterOpen(false); }}
              style={{
                display: "flex", alignItems: "center", gap: 8, width: "100%", padding: "6px 8px", borderRadius: 5,
                fontSize: 12, color: !filterAgent ? "var(--mycel-accent)" : "var(--mycel-text-2)", background: !filterAgent ? "var(--mycel-accent-subtle)" : "transparent",
                border: "none", cursor: "pointer", textAlign: "left",
              }}
            >
              All senders
            </button>
            {uniqueSenders.map((sender) => (
              <button
                key={sender}
                type="button"
                onClick={() => { setFilterAgent(sender); setFilterOpen(false); }}
                style={{
                  display: "flex", alignItems: "center", gap: 8, width: "100%", padding: "6px 8px", borderRadius: 5,
                  fontSize: 12, color: filterAgent === sender ? "var(--mycel-accent)" : "var(--mycel-text-2)", background: filterAgent === sender ? "var(--mycel-accent-subtle)" : "transparent",
                  border: "none", cursor: "pointer", textAlign: "left",
                  fontFamily: "'JetBrains Mono', monospace",
                }}
              >
                <span className="flex items-center justify-center shrink-0">
                  {platform ? (
                    <IdentityAvatar name={cleanSender(sender)} size={18} />
                  ) : (
                    <AgentCharacter name={sender} size={18} />
                  )}
                </span>
                <span className="truncate">{cleanSender(sender)}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* ── Search bar (inline below header) ─────────────────── */}
      {searchOpen && (
        <div
          className="shrink-0 flex items-center"
          style={{
            padding: "6px 16px",
            borderBottom: "1px solid var(--mycel-border)",
            background: "var(--mycel-surface)",
            gap: 8,
          }}
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--mycel-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0 }}>
            <circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
          <input
            ref={searchInputRef}
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Escape") { setSearchOpen(false); setSearchQuery(""); } }}
            placeholder="Search messages..."
            style={{
              flex: 1,
              background: "none",
              border: "none",
              outline: "none",
              color: "var(--mycel-text)",
              fontSize: 12,
              fontFamily: "'JetBrains Mono', monospace",
            }}
          />
          {searchQuery && (
            <span style={{ fontSize: 10, color: "var(--mycel-muted)", fontFamily: "'JetBrains Mono', monospace", whiteSpace: "nowrap" }}>
              {filteredMessages.length} result{filteredMessages.length !== 1 ? "s" : ""}
            </span>
          )}
          <button
            type="button"
            onClick={() => { setSearchOpen(false); setSearchQuery(""); }}
            style={{ background: "none", border: "none", color: "var(--mycel-muted)", cursor: "pointer", padding: 2 }}
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      )}

      {/* ── Active filter indicator ──────────────────────────── */}
      {filterAgent && !filterOpen && (
        <div
          className="shrink-0 flex items-center"
          style={{
            padding: "4px 16px",
            borderBottom: "1px solid var(--mycel-border)",
            background: "var(--mycel-surface)",
            gap: 6,
            fontSize: 11,
            color: "var(--mycel-text-2)",
          }}
        >
          <span style={{ color: "var(--mycel-muted)" }}>Filtered by:</span>
          <span style={{ color: "var(--mycel-accent)", fontFamily: "'JetBrains Mono', monospace" }}>{cleanSender(filterAgent)}</span>
          <button
            type="button"
            onClick={() => setFilterAgent(null)}
            style={{ background: "none", border: "none", color: "var(--mycel-muted)", cursor: "pointer", padding: 2, marginLeft: "auto" }}
            title="Clear filter"
          >
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      )}

      {/* Channel topic / pinned bar */}
      {!topicDismissed && (
        <div
          className="flex items-start shrink-0"
          style={{
            padding: "10px 18px",
            fontSize: 12,
            color: "var(--mycel-muted)",
            lineHeight: 1.55,
            borderBottom: "1px solid var(--mycel-border)",
            background: "var(--mycel-surface)",
            gap: 10,
          }}
        >
          <svg width="11" height="11" viewBox="0 0 24 24" fill="var(--mycel-accent)" stroke="none" style={{ marginTop: 1, flexShrink: 0 }}>
            <path d="M12 17v5M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z" />
          </svg>
          <span style={{ flex: 1 }}>
            {channel?.description && channel.description !== "Gateway channel"
              ? channel.description
              : (<>
                  Coordination feed for{" "}
                  <span style={{ color: "var(--mycel-text-2)", fontFamily: "'JetBrains Mono', monospace" }}>
                    #{channelLabel}
                  </span>
                  . Agents posting here are managed via{" "}
                  <span
                    style={{ color: "var(--mycel-accent)", cursor: "pointer" }}
                    onClick={() => setShowAgents(true)}
                  >
                    agent subscriptions
                  </span>
                  .
                  {platform && (
                    <>
                      {" "}Attachments are raw{" "}
                      <span style={{ color: "var(--mycel-text-2)", fontFamily: "'JetBrains Mono', monospace" }}>
                        {platform}
                      </span>{" "}
                      webhook payloads.
                    </>
                  )}
                </>)
            }
          </span>
          <button
            type="button"
            onClick={() => setTopicDismissed(true)}
            style={{
              background: "none",
              border: "none",
              color: "var(--mycel-muted)",
              cursor: "pointer",
              padding: 2,
              flexShrink: 0,
              marginTop: 1,
            }}
            title="Dismiss"
          >
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>
      )}

      {/* ── Message stream ─────────────────────────────────────── */}
      <div className="flex-1 relative">
        <div
          ref={scrollRef}
          className="absolute inset-0 overflow-auto"
          style={{
            scrollbarWidth: "thin",
            scrollbarColor: "var(--mycel-border) transparent",
          }}
        >
          <div style={{ padding: "14px 0 8px", display: "flex", flexDirection: "column", marginTop: "auto" }}>
            {initialLoading && messages.length === 0 && (
              <div className="space-y-4 py-4 px-5 animate-pulse">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="flex items-start gap-3">
                    <div className="w-7 h-7 rounded-md flex-shrink-0" style={{ background: "var(--mycel-surface-hover)" }} />
                    <div className="flex-1 space-y-2">
                      <div className="flex items-center gap-2">
                        <div className="h-3 w-20 rounded" style={{ background: "var(--mycel-surface-hover)" }} />
                        <div className="h-2 w-12 rounded" style={{ background: "var(--mycel-surface)" }} />
                      </div>
                      <div className="h-3 rounded" style={{ background: "var(--mycel-surface)", width: `${60 + (i * 7) % 30}%` }} />
                    </div>
                  </div>
                ))}
              </div>
            )}
            {!initialLoading && messages.length === 0 && (
              <div className="flex flex-col items-center justify-center py-24 text-center">
                <svg width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="1.2" style={{ color: "var(--mycel-muted)" }} className="mb-4">
                  <path d="M4 16h6m12 0h6M16 4v6m0 12v6" strokeLinecap="round" />
                  <circle cx="16" cy="16" r="3" />
                </svg>
                <h3 style={{ fontSize: 14, fontWeight: 500, color: "var(--mycel-muted)", marginBottom: 4 }}>
                  Waiting for messages
                </h3>
                <p style={{ fontSize: 12, color: "var(--mycel-muted)" }}>
                  Activity from {platform ?? "this channel"} will stream here in real-time.
                </p>
              </div>
            )}

            {/* Beginning of history */}
            {!hasMore && messages.length > 0 && (
              <div className="flex items-center py-6 px-5" style={{ gap: 10 }}>
                <div className="flex-1" style={{ height: 1, background: "var(--mycel-border)" }} />
                <span
                  style={{
                    fontSize: 11,
                    fontWeight: 600,
                    color: "var(--mycel-text-2)",
                    background: "var(--mycel-surface-hover)",
                    padding: "3px 10px",
                    borderRadius: 999,
                  }}
                >
                  Beginning of history
                </span>
                <div className="flex-1" style={{ height: 1, background: "var(--mycel-border)" }} />
              </div>
            )}
            {hasMore && (
              <div ref={sentinelRef} className="py-4 text-center">
                {loadingMore ? (
                  <span style={{ fontSize: 10, color: "var(--mycel-muted)" }}>Loading older messages...</span>
                ) : (
                  <span style={{ fontSize: 10, color: "var(--mycel-muted)" }}>Scroll up for more</span>
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
                        background: "linear-gradient(var(--mycel-bg) 40%, transparent)",
                        zIndex: 2,
                      }}
                    >
                      <div className="flex-1" style={{ height: 1, background: "var(--mycel-border)" }} />
                      <div
                        style={{
                          fontSize: 11,
                          fontWeight: 600,
                          color: "var(--mycel-text-2)",
                          background: "var(--mycel-surface-hover)",
                          padding: "3px 10px",
                          borderRadius: 999,
                        }}
                      >
                        {formatDayLabel(group.timestamp)}
                      </div>
                      <div className="flex-1" style={{ height: 1, background: "var(--mycel-border)" }} />
                    </div>
                  )}

                  {/* Message group — first message with avatar, subsequent without */}
                  <div style={{ paddingTop: 10 }}>
                    {/* Sender line with avatar */}
                    <div className="flex" style={{ padding: "0 18px", gap: 10 }}>
                      {/* Sender character — a real chat participant on gateway
                          channels (identity avatar, never the mycel mushroom),
                          or an agent on internal channels. */}
                      <span
                        className="flex items-center justify-center shrink-0"
                        style={{ width: 30, height: 30, minWidth: 30, marginTop: 2 }}
                      >
                        {platform ? (
                          <IdentityAvatar
                            name={cleanSender(group.sender)}
                            src={group.messages[0]?.avatar_url || undefined}
                            size={30}
                          />
                        ) : (
                          <LiveAgentCharacter name={group.sender} size={30} />
                        )}
                      </span>
                      <div className="flex-1 min-w-0">
                        {/* Name row */}
                        <div className="flex items-baseline flex-wrap" style={{ gap: 7, marginBottom: 1 }}>
                          {platform ? (
                            <span
                              style={{
                                fontSize: 13.5,
                                fontWeight: 600,
                                color: "var(--mycel-text)",
                                fontFamily: "'JetBrains Mono', monospace",
                              }}
                            >
                              {cleanSender(group.sender)}
                            </span>
                          ) : (
                            <button
                              type="button"
                              onClick={() => onPeekAgent(group.sender)}
                              className="hover:underline cursor-pointer decoration-1 underline-offset-2"
                              style={{
                                fontSize: 13.5,
                                fontWeight: 600,
                                color: "var(--mycel-text)",
                                fontFamily: "'JetBrains Mono', monospace",
                                background: "none",
                                border: "none",
                                padding: 0,
                              }}
                            >
                              {cleanSender(group.sender)}
                            </button>
                          )}
                          <span
                            style={{
                              fontSize: 11,
                              color: "var(--mycel-muted)",
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
                          // Detect webhook JSON payloads on any platform
                          const looksLikeWebhook = msg.content.trimStart().startsWith("{") &&
                            /pull_request|"issue"|"ref".*"commits"|"action"/i.test(msg.content);
                          const ghCard = (platform === "github" || looksLikeWebhook) ? parseGitHubCard(msg.content) : null;
                          const rssCard = platform === "rss" ? parseRSSCard(msg.content) : null;
                          const webhookCard = !ghCard && !rssCard && platform === "webhook" ? parseWebhookCard(msg.content) : null;
                          const fileAttachments = parseFileAttachments(msg.content);

                          return (
                            <div key={msg.id} className="group/msg relative">
                              <div
                                className="rounded-md transition-colors duration-100 hover:bg-mycel-surface-hover"
                                style={{ padding: "2px 0" }}
                              >
                                {ghCard ? (
                                  <GitHubCardView card={ghCard} />
                                ) : rssCard ? (
                                  <RSSCardView card={rssCard} />
                                ) : webhookCard ? (
                                  <WebhookCardView card={webhookCard} />
                                ) : (
                                  <div
                                    className="whitespace-pre-wrap break-words"
                                    style={{
                                      fontSize: 13.5,
                                      color: "var(--mycel-text)",
                                      lineHeight: 1.55,
                                      wordBreak: "break-word",
                                    }}
                                  >
                                    <MessageContent content={msg.content} agentNames={agentNames} />
                                  </div>
                                )}

                                {/* File attachments */}
                                {fileAttachments.length > 0 && (
                                  <div className="flex flex-wrap" style={{ gap: 4, marginTop: 2 }}>
                                    {fileAttachments.map((att, i) => (
                                      <FileAttachmentCard key={i} attachment={att} />
                                    ))}
                                  </div>
                                )}

                                {/* Delivery indicators */}
                                {hasDelivery && (
                                  <div className="hidden group-hover/msg:flex items-center gap-3 mt-0.5" style={{ fontSize: 9 }}>
                                    {delivered.length > 0 && (
                                      <span style={{ color: "color-mix(in oklab, var(--mycel-success) 50%, transparent)" }} title={delivered.map((d) => d.agent).join(", ")}>
                                        → {delivered.map((d) => d.agent).join(", ")}
                                      </span>
                                    )}
                                    {failed.length > 0 && (
                                      <span style={{ color: "color-mix(in oklab, var(--mycel-error) 50%, transparent)" }} title={failed.map((d) => `${d.agent}: ${d.error ?? "failed"}`).join(", ")}>
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

      <style>{`@keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: translateY(0); } }`}</style>
    </div>
  );
}
