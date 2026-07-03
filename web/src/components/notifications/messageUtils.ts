import type { ChannelMessage } from "../../api/client";
import { formatRelative } from "../../utils/time";

/** Gateway notification sources are bridges to external platforms — read-only activity feeds. */
export const GATEWAY_PREFIXES = [
  "slack:", "telegram:", "discord:", "whatsapp:", "github:", "webhook:",
  "rss:", "mqtt:", "irc:", "matrix:", "mattermost:", "reddit:", "twitter:",
  "notion:", "signal:", "nostr:", "homeassistant:", "imessage:",
];

export function isGatewaySource(name: string): boolean {
  return GATEWAY_PREFIXES.some((p) => name.startsWith(p));
}

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

/** Format a timestamp as a relative time string ("2m ago", "1h ago", "3d ago"). */
export function formatRelativeTime(iso: string): string {
  return formatRelative(iso);
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

export const DEFAULT_ROLE_COLOR = { bg: "bg-mycel-muted/20", text: "text-mycel-muted" };

export function getRoleColor(role: string | undefined): { bg: string; text: string } {
  if (!role) return DEFAULT_ROLE_COLOR;
  return ROLE_COLORS[role] ?? DEFAULT_ROLE_COLOR;
}

/**
 * Generate a consistent color for an agent name.
 *
 * The first agent (by hash bucket 0) uses the *theme accent* so the
 * chart's most prominent series feels native to whatever theme is
 * active (tangerine under Solar Flare / Light, emerald under Dark).
 * The remaining agents draw from a hue palette chosen to stay
 * distinguishable across dark and light backgrounds — pastels are
 * avoided since they collapse into the muted grays on light theme.
 *
 * Cache is intentionally global — two different renders should give
 * the same agent the same color. Theme changes don't invalidate the
 * cache since the theme-accent slot is a `var(--mycel-accent)` string,
 * not a resolved hex.
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
  const bucket = hashString(name) % (AGENT_HUES.length + 1);
  // Bucket 0 = theme accent so charts feel native to whichever theme is
  // active. Any other bucket draws from the fixed hue palette.
  const color = bucket === 0
    ? "var(--mycel-accent)"
    : `hsl(${AGENT_HUES[(bucket - 1) % AGENT_HUES.length]}, 65%, 65%)`;
  AGENT_COLOR_CACHE.set(name, color);
  return color;
}

export function agentColorMuted(name: string): string {
  const bucket = hashString(name) % (AGENT_HUES.length + 1);
  if (bucket === 0) {
    return "color-mix(in srgb, var(--mycel-accent) 10%, transparent)";
  }
  const hue = AGENT_HUES[(bucket - 1) % AGENT_HUES.length];
  return `hsla(${hue}, 40%, 50%, 0.08)`;
}

/* ── GitHub webhook card parsing ────────────────────────────── */

export interface GitHubCard {
  type: "pr" | "issue" | "push" | "release";
  title: string;
  number?: number;
  url?: string;
  status?: string;
  action?: string;
  additions?: number;
  deletions?: number;
  changedFiles?: number;
  branch?: string;
  repo?: string;
}

/**
 * Try to extract a GitHub card from a message that looks like it came
 * from a GitHub webhook.  Returns null if the content does not match.
 */
export function parseGitHubCard(content: string): GitHubCard | null {
  // Try to parse as JSON first (raw webhook payload forwarded as message)
  try {
    const obj = JSON.parse(content);
    if (obj && typeof obj === "object") {
      // Pull request event
      if (obj.pull_request) {
        const pr = obj.pull_request;
        return {
          type: "pr",
          title: pr.title ?? "Pull Request",
          number: pr.number,
          url: pr.html_url,
          status: pr.merged ? "MERGED" : pr.state?.toUpperCase() ?? "OPEN",
          action: obj.action,
          additions: pr.additions,
          deletions: pr.deletions,
          changedFiles: pr.changed_files,
          repo: obj.repository?.full_name,
        };
      }
      // Issue event
      if (obj.issue) {
        return {
          type: "issue",
          title: obj.issue.title ?? "Issue",
          number: obj.issue.number,
          url: obj.issue.html_url,
          status: obj.issue.state?.toUpperCase() ?? "OPEN",
          action: obj.action,
          repo: obj.repository?.full_name,
        };
      }
      // Push event
      if (obj.ref && obj.commits) {
        return {
          type: "push",
          title: `${obj.commits.length} commit${obj.commits.length !== 1 ? "s" : ""} pushed`,
          branch: obj.ref?.replace("refs/heads/", ""),
          repo: obj.repository?.full_name,
        };
      }
    }
  } catch {
    // not JSON — try regex patterns below
  }

  // Match common GitHub notification text patterns
  // e.g., "[user/repo] Pull request #123: Title (opened/merged/closed)"
  const prMatch = content.match(
    /\[([^\]]+)\]\s*(?:Pull request|PR)\s*#(\d+):\s*(.+?)(?:\s*\((opened|closed|merged|synchronize)\))?$/i,
  );
  if (prMatch) {
    return {
      type: "pr",
      repo: prMatch[1] ?? "",
      number: parseInt(prMatch[2] ?? "0", 10),
      title: (prMatch[3] ?? "").trim(),
      status: (prMatch[4] ?? "OPEN").toUpperCase(),
    };
  }

  // Issue pattern
  const issueMatch = content.match(
    /\[([^\]]+)\]\s*Issue\s*#(\d+):\s*(.+?)(?:\s*\((opened|closed)\))?$/i,
  );
  if (issueMatch) {
    return {
      type: "issue",
      repo: issueMatch[1] ?? "",
      number: parseInt(issueMatch[2] ?? "0", 10),
      title: (issueMatch[3] ?? "").trim(),
      status: (issueMatch[4] ?? "OPEN").toUpperCase(),
    };
  }

  return null;
}

/* ── RSS / webhook card parsing ───────────────────────────────── */

export interface RSSCard {
  title: string;
  link?: string;
  description?: string;
  pubDate?: string;
  source?: string;
}

export function parseRSSCard(content: string): RSSCard | null {
  try {
    const obj = JSON.parse(content);
    if (obj && typeof obj === "object" && !Array.isArray(obj) && obj.title && (obj.link || obj.url)) {
      return {
        title: obj.title,
        link: obj.link ?? obj.url,
        description: obj.description ?? obj.summary,
        pubDate: obj.pubDate ?? obj.published ?? obj.date,
        source: obj.feed ?? obj.source,
      };
    }
  } catch {
    // not JSON
  }
  return null;
}

export interface WebhookCard {
  event?: string;
  action?: string;
  payload: Record<string, unknown>;
}

export function parseWebhookCard(content: string): WebhookCard | null {
  try {
    const obj = JSON.parse(content);
    if (obj && typeof obj === "object" && !Array.isArray(obj)) {
      return {
        event: obj.event_type ?? obj.event ?? obj.type,
        action: obj.action,
        payload: obj,
      };
    }
  } catch {
    // not JSON
  }
  return null;
}
