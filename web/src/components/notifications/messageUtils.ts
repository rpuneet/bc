import type { ChannelMessage } from "../../api/client";

/** Gateway notification sources are bridges to external platforms — read-only activity feeds. */
export const GATEWAY_PREFIXES = ["slack:", "telegram:", "discord:"];

export function isGatewaySource(name: string): boolean {
  return GATEWAY_PREFIXES.some((p) => name.startsWith(p));
}

/** @deprecated Use isGatewaySource instead */
export const isGatewayChannel = isGatewaySource;

/** Extract platform name from gateway source for display. */
export function gatewayPlatform(name: string): string | null {
  for (const p of GATEWAY_PREFIXES) {
    if (name.startsWith(p)) return p.slice(0, -1);
  }
  return null;
}

/** Derive platform bucket key from source name. */
export function sourcePlatform(name: string): string {
  for (const p of GATEWAY_PREFIXES) {
    if (name.startsWith(p)) return p.slice(0, -1);
  }
  return "internal";
}

/** @deprecated Use sourcePlatform instead */
export const channelPlatform = sourcePlatform;

export interface MessageGroup {
  sender: string;
  timestamp: string;
  messages: ChannelMessage[];
}

/** Time window (ms) for grouping consecutive messages from same sender. */
const GROUP_WINDOW_MS = 5 * 60 * 1000;

/** Group consecutive messages from the same sender within a 5-minute window. */
export function groupMessages(msgs: ChannelMessage[]): MessageGroup[] {
  const groups: MessageGroup[] = [];
  for (const msg of msgs) {
    const last = groups[groups.length - 1];
    if (last && last.sender === msg.sender) {
      const lastMsg = last.messages[last.messages.length - 1];
      const timeDiff = lastMsg
        ? new Date(msg.created_at).getTime() -
          new Date(lastMsg.created_at).getTime()
        : 0;
      if (timeDiff < GROUP_WINDOW_MS) {
        last.messages.push(msg);
        continue;
      }
    }
    groups.push({
      sender: msg.sender,
      timestamp: msg.created_at,
      messages: [msg],
    });
  }
  return groups;
}

export function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  const isToday =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  if (isToday) {
    return d.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    });
  }
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** Format a date for day separators. */
export function formatDayLabel(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  const isToday =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  if (isToday) return "Today";
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  const isYesterday =
    d.getFullYear() === yesterday.getFullYear() &&
    d.getMonth() === yesterday.getMonth() &&
    d.getDate() === yesterday.getDate();
  if (isYesterday) return "Yesterday";
  return d.toLocaleDateString(undefined, {
    weekday: "long",
    month: "long",
    day: "numeric",
  });
}

/** Get the date key (YYYY-MM-DD) from an ISO timestamp. */
export function dateKey(iso: string): string {
  return new Date(iso).toLocaleDateString("sv-SE"); // YYYY-MM-DD format
}

/** Role-based avatar colors. */
export const ROLE_COLORS: Record<string, { bg: string; text: string }> = {
  root: { bg: "bg-purple-500/20", text: "text-purple-400" },
  engineer: { bg: "bg-blue-500/20", text: "text-blue-400" },
  manager: { bg: "bg-green-500/20", text: "text-green-400" },
  lead: { bg: "bg-amber-500/20", text: "text-amber-400" },
  product_manager: { bg: "bg-rose-500/20", text: "text-rose-400" },
  infra_lead: { bg: "bg-cyan-500/20", text: "text-cyan-400" },
  ui_lead: { bg: "bg-pink-500/20", text: "text-pink-400" },
  api_lead: { bg: "bg-teal-500/20", text: "text-teal-400" },
  feature_dev: { bg: "bg-indigo-500/20", text: "text-indigo-400" },
  base: { bg: "bg-slate-500/20", text: "text-slate-400" },
};

export const DEFAULT_ROLE_COLOR = { bg: "bg-bc-muted/20", text: "text-bc-muted" };

export function getRoleColor(role: string | undefined): { bg: string; text: string } {
  if (!role) return DEFAULT_ROLE_COLOR;
  return ROLE_COLORS[role] ?? DEFAULT_ROLE_COLOR;
}

/**
 * Generate a consistent HSL color for an agent name.
 * Each agent gets a unique hue derived from their name hash.
 */
const AGENT_COLOR_CACHE = new Map<string, string>();

const AGENT_HUES = [
  28, 45, 160, 195, 210, 260, 280, 320, 340, 15, 50, 140, 175, 230, 300,
];

function hashString(s: string): number {
  let hash = 0;
  for (let i = 0; i < s.length; i++) {
    hash = ((hash << 5) - hash + s.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

export function agentColor(name: string): string {
  if (AGENT_COLOR_CACHE.has(name)) return AGENT_COLOR_CACHE.get(name)!;
  const hue = AGENT_HUES[hashString(name) % AGENT_HUES.length];
  const color = `hsl(${hue}, 65%, 65%)`;
  AGENT_COLOR_CACHE.set(name, color);
  return color;
}

export function agentColorMuted(name: string): string {
  const hue = AGENT_HUES[hashString(name) % AGENT_HUES.length];
  return `hsla(${hue}, 40%, 50%, 0.08)`;
}
