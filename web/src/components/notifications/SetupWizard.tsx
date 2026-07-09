import { useEffect, useRef, useState, useCallback } from "react";
import { createPortal } from "react-dom";
import { api } from "../../api/client";
import type { Agent, GatewayStatus } from "../../api/client";
import { PLATFORM_ICON_MAP } from "./PlatformIcons";

export interface PlatformDef {
  key: string;
  label: string;
  icon: string;
  description: string;
  color: string;
  category: string;
  status: "ready" | "webhook" | "poll" | "coming_soon";
  fields: { key: string; label: string; placeholder: string; required?: boolean; type?: string }[];
  docs: string[];
  pairFlow?: "qr";
}

export const PLATFORMS: PlatformDef[] = [
  // --- Chat ---
  {
    key: "slack", status: "ready" as const,
    label: "Slack",
    icon: "\u{1F4AC}",
    description: "Team messaging via Socket Mode",
    color: "#E01E5A",
    category: "Chat",
    fields: [
      { key: "bot_token", label: "Bot Token", placeholder: "xoxb-..." },
      { key: "app_token", label: "App Token", placeholder: "xapp-..." },
    ],
    docs: [
      "Create a Slack app → https://api.slack.com/apps — enable Socket Mode.",
      "Add scopes: channels:read, chat:write, connections:write.",
      "Copy Bot Token from OAuth & Permissions, App Token from Basic Information.",
      "Install the app and invite the bot to your channels.",
    ],
  },
  {
    key: "telegram", status: "ready" as const,
    label: "Telegram",
    icon: "\u2708\uFE0F",
    description: "Bot messages via long polling",
    color: "#26A5E4",
    category: "Chat",
    fields: [{ key: "bot_token", label: "Bot Token", placeholder: "1234567890:AAH..." }],
    docs: [
      "Message @BotFather on Telegram → https://t.me/BotFather — send /newbot.",
      "Copy the bot token. Message the bot in a DM or add it to a group.",
      "Channels appear after the first inbound message (telegram:<username|chat_id|group>).",
      "Do not subscribe to telegram:general — that key is not a real Telegram chat.",
    ],
  },
  {
    key: "discord", status: "ready" as const,
    label: "Discord",
    icon: "\u{1F3AE}",
    description: "Bot messages from Discord servers",
    color: "#5865F2",
    category: "Chat",
    fields: [{ key: "bot_token", label: "Bot Token", placeholder: "MTIz..." }],
    docs: [
      "Create an app → https://discord.com/developers/applications",
      "Enable MESSAGE CONTENT INTENT under Bot settings, copy the bot token.",
      "Generate an invite URL with bot scope and add to your server.",
    ],
  },
  {
    key: "whatsapp", status: "ready" as const,
    label: "WhatsApp",
    icon: "\u{1F4F1}",
    description: "Personal WhatsApp via QR code pairing",
    color: "#25D366",
    category: "Chat",
    fields: [],
    docs: [
      "Click Connect to generate a QR code.",
      "Scan it with WhatsApp → Linked Devices on your phone.",
      "Session persists across restarts — no re-scan needed.",
    ],
    pairFlow: "qr" as const,
  },
  {
    key: "signal", status: "poll" as const,
    label: "Signal",
    icon: "\u{1F510}",
    description: "Private encrypted messaging",
    color: "#3A76F0",
    category: "Chat",
    fields: [
      { key: "api_url", label: "signal-cli REST API URL", placeholder: "http://localhost:8080", required: true },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "10", type: "number" },
    ],
    docs: [
      "Install signal-cli-rest-api → https://github.com/bbernhard/signal-cli-rest-api",
      "Run: docker run -p 8080:8080 bbernhard/signal-cli-rest-api",
      "Register or link your phone number, then enter the API URL.",
    ],
  },
  {
    key: "matrix", status: "ready" as const,
    label: "Matrix",
    icon: "\u{1F30D}",
    description: "Decentralized chat via client-server API",
    color: "#0DBD8B",
    category: "Chat",
    fields: [
      { key: "homeserver", label: "Homeserver URL", type: "text" as const, placeholder: "https://matrix.org" },
      { key: "token", label: "Access Token", type: "password" as const, placeholder: "syt_..." },
    ],
    docs: [
      "Download Element → https://element.io/download",
      "Get an access token: Element → Settings → Help & About → Access Token.",
    ],
  },
  {
    key: "msteams", status: "coming_soon" as const,
    label: "MS Teams",
    icon: "\u{1F4BC}",
    description: "Microsoft Teams integration",
    color: "#6264A7",
    category: "Chat",
    fields: [],
    docs: [],
  },
  {
    key: "googlechat", status: "coming_soon" as const,
    label: "Google Chat",
    icon: "\u{1F4E8}",
    description: "Google Workspace messaging",
    color: "#00AC47",
    category: "Chat",
    fields: [],
    docs: [],
  },
  {
    key: "line", status: "coming_soon" as const,
    label: "Line",
    icon: "\u{1F4AC}",
    description: "Line Messaging API",
    color: "#06C755",
    category: "Chat",
    fields: [],
    docs: [],
  },
  {
    key: "feishu", status: "coming_soon" as const,
    label: "Feishu",
    icon: "\u{1F426}",
    description: "Lark/Feishu bot messaging",
    color: "#3370FF",
    category: "Chat",
    fields: [],
    docs: [],
  },
  {
    key: "mattermost", status: "ready" as const,
    label: "Mattermost",
    icon: "\u{1F5E8}\uFE0F",
    description: "Self-hosted team messaging via WebSocket",
    color: "#0058CC",
    category: "Chat",
    fields: [
      { key: "url", label: "Server URL", type: "text" as const, placeholder: "https://mattermost.example.com" },
      { key: "token", label: "Personal Access Token", type: "password" as const, placeholder: "abc123..." },
    ],
    docs: [
      "Mattermost docs → https://docs.mattermost.com/developer/personal-access-tokens.html",
      "Go to Account Settings → Security → Personal Access Tokens, create and paste above.",
    ],
  },
  {
    key: "irc", status: "ready" as const,
    label: "IRC",
    icon: "\u{1F4DF}",
    description: "Connect to any IRC server with TLS",
    color: "#6B7280",
    category: "Chat",
    fields: [
      { key: "server", label: "Server", placeholder: "irc.libera.chat:6697", required: true },
      { key: "channels", label: "Channels (comma-separated)", placeholder: "#general,#dev" },
    ],
    docs: [
      "Popular servers: irc.libera.chat:6697, irc.oftc.net:6697 (TLS).",
      "List channels to join, separated by commas (e.g. #general,#dev).",
    ],
  },

  // --- Code & DevOps ---
  {
    key: "github", status: "webhook" as const,
    label: "GitHub",
    icon: "\u{1F419}",
    description: "PR, issue, and push webhooks",
    color: "#8B949E",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Webhook Secret", placeholder: "your-webhook-secret" }],
    docs: [
      "Create a webhook → your repo → Settings → Webhooks.",
      "Set the payload URL to your mycel server's /hooks/github endpoint.",
      "Set the secret here to match the webhook secret.",
    ],
  },
  {
    key: "gitlab", status: "coming_soon" as const,
    label: "GitLab",
    icon: "\u{1F98A}",
    description: "Merge request and pipeline webhooks",
    color: "#FC6D26",
    category: "Code & DevOps",
    fields: [{ key: "token", label: "Token", placeholder: "webhook-secret-token" }],
    docs: [
      "Go to your GitLab project > Settings > Webhooks.",
      "Set the URL to your mycel server's /hooks/gitlab endpoint.",
      "Copy the secret token and paste it here.",
    ],
  },
  {
    key: "bitbucket", status: "coming_soon" as const,
    label: "Bitbucket",
    icon: "\u{1FAA3}",
    description: "Push and PR webhooks",
    color: "#0052CC",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Secret", placeholder: "webhook-secret" }],
    docs: [
      "Go to your Bitbucket repo > Settings > Webhooks.",
      "Add a webhook pointing to your mycel server's /hooks/bitbucket endpoint.",
      "Set the secret here for payload verification.",
    ],
  },
  {
    key: "vercel", status: "coming_soon" as const,
    label: "Vercel",
    icon: "\u25B2",
    description: "Deployment and build webhooks",
    color: "#000000",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Secret", placeholder: "whsec_..." }],
    docs: [
      "Go to your Vercel project > Settings > Webhooks.",
      "Add a webhook endpoint pointing to /hooks/vercel on your mycel server.",
      "Copy the signing secret and paste it here.",
    ],
  },
  {
    key: "netlify", status: "coming_soon" as const,
    label: "Netlify",
    icon: "\u25C6",
    description: "Deploy and build notifications",
    color: "#00C7B7",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Secret", placeholder: "webhook-secret" }],
    docs: [
      "Go to your Netlify site > Site settings > Notifications.",
      "Add an outgoing webhook pointing to /hooks/netlify on your mycel server.",
      "Set and copy the secret for payload verification.",
    ],
  },

  // --- Monitoring ---
  {
    key: "sentry", status: "coming_soon" as const,
    label: "Sentry",
    icon: "\u{1F41B}",
    description: "Error and issue alerts",
    color: "#362D59",
    category: "Monitoring",
    fields: [{ key: "client_secret", label: "Client Secret", placeholder: "sentry-client-secret" }],
    docs: [
      "Go to Sentry > Settings > Integrations > Internal Integration.",
      "Create an integration with webhook URL pointing to /hooks/sentry.",
      "Copy the client secret and paste it here.",
    ],
  },
  {
    key: "pagerduty", status: "coming_soon" as const,
    label: "PagerDuty",
    icon: "\u{1F6A8}",
    description: "Incident and alert webhooks",
    color: "#06AC38",
    category: "Monitoring",
    fields: [{ key: "secret", label: "Secret", placeholder: "pagerduty-secret" }],
    docs: [
      "Go to PagerDuty > Integrations > Generic Webhooks V3.",
      "Add a webhook subscription pointing to /hooks/pagerduty.",
      "Copy the signing secret and paste it here.",
    ],
  },
  {
    key: "datadog", status: "coming_soon" as const,
    label: "Datadog",
    icon: "\u{1F415}",
    description: "Monitor and event webhooks",
    color: "#632CA6",
    category: "Monitoring",
    fields: [{ key: "api_key", label: "API Key", placeholder: "datadog-api-key" }],
    docs: [
      "Go to Datadog > Integrations > Webhooks.",
      "Create a webhook pointing to /hooks/datadog on your mycel server.",
      "Copy your API key from Organization Settings > API Keys.",
    ],
  },
  {
    key: "grafana", status: "coming_soon" as const,
    label: "Grafana",
    icon: "\u{1F4CA}",
    description: "Alert notifications",
    color: "#F46800",
    category: "Monitoring",
    fields: [{ key: "api_token", label: "API Token", placeholder: "grafana-api-token" }],
    docs: [
      "Go to Grafana > Alerting > Contact Points.",
      "Add a webhook contact point with URL /hooks/grafana.",
      "Copy an API token from Configuration > API Keys.",
    ],
  },
  {
    key: "homeassistant", status: "coming_soon" as const,
    label: "Home Assistant",
    icon: "\u{1F3E0}",
    description: "Smart home automations and events",
    color: "#41BDF5",
    category: "Monitoring",
    fields: [],
    docs: [],
  },

  // --- Payments ---
  {
    key: "stripe", status: "coming_soon" as const,
    label: "Stripe",
    icon: "\u{1F4B3}",
    description: "Payment and subscription events",
    color: "#635BFF",
    category: "Payments",
    fields: [{ key: "webhook_secret", label: "Webhook Secret", placeholder: "whsec_..." }],
    docs: [
      "Go to Stripe Dashboard > Developers > Webhooks.",
      "Add an endpoint pointing to /hooks/stripe on your mycel server.",
      "Copy the signing secret and paste it here.",
    ],
  },

  // --- Content ---
  {
    key: "rss", status: "ready" as const,
    label: "RSS / Atom",
    icon: "\u{1F4E1}",
    description: "Subscribe to any RSS or Atom feed",
    color: "#F78422",
    category: "Content",
    fields: [
      { key: "url", label: "Feed URL", placeholder: "https://example.com/feed.xml", type: "url" },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "300", type: "number" },
    ],
    docs: [
      "Paste any RSS or Atom feed URL.",
      "Set a poll interval in seconds (default: 300 = 5 minutes).",
    ],
  },
  {
    key: "notion", status: "poll" as const,
    label: "Notion",
    icon: "\u{1F4DD}",
    description: "Database and page change polling",
    color: "#000000",
    category: "Content",
    fields: [
      { key: "token", label: "API Token", placeholder: "secret_..." },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "300", type: "number" },
    ],
    docs: [
      "Create an integration → https://www.notion.so/my-integrations",
      "Copy the API token and share target pages/databases with the integration.",
      "Set a poll interval in seconds (default: 300).",
    ],
  },
  {
    key: "reddit", status: "poll" as const,
    label: "Reddit",
    icon: "\u{1F4E2}",
    description: "Subreddit and post monitoring",
    color: "#FF4500",
    category: "Content",
    fields: [
      { key: "subreddit", label: "Subreddit", placeholder: "golang", required: true },
      { key: "bearer_token", label: "Bearer Token", placeholder: "OAuth bearer token", required: true },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "60", type: "number" },
    ],
    docs: [
      "Create a Reddit app → https://www.reddit.com/prefs/apps (script type).",
      "Generate an OAuth bearer token using client credentials.",
      "Enter the subreddit name (without r/) and poll interval.",
    ],
  },
  {
    key: "twitch", status: "coming_soon" as const,
    label: "Twitch",
    icon: "\u{1F3AE}",
    description: "Stream and chat events",
    color: "#9146FF",
    category: "Content",
    fields: [],
    docs: [],
  },
  {
    key: "twitter", status: "poll" as const,
    label: "Twitter / X",
    icon: "\u{1F426}",
    description: "Mentions and timeline events",
    color: "#1DA1F2",
    category: "Content",
    fields: [
      { key: "bearer_token", label: "Bearer Token", placeholder: "Twitter API v2 bearer token", required: true },
      { key: "user_id", label: "User ID", placeholder: "Numeric user ID to monitor", required: true },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "60", type: "number" },
    ],
    docs: [
      "Create a developer app → https://developer.twitter.com/en/portal/dashboard",
      "Copy the Bearer Token from the app settings.",
      "Find your numeric user ID (not @handle).",
    ],
  },
  {
    key: "nostr", status: "coming_soon" as const,
    label: "Nostr",
    icon: "\u{1F4E1}",
    description: "Decentralized social protocol",
    color: "#8B5CF6",
    category: "Content",
    fields: [],
    docs: [],
  },

  // --- Custom ---
  {
    key: "webhook", status: "webhook" as const,
    label: "Generic Webhook",
    icon: "\u{1F517}",
    description: "Receive any JSON webhook payload",
    color: "#8c7e72",
    category: "Custom",
    fields: [{ key: "secret", label: "Shared Secret (optional)", placeholder: "optional-secret", required: false }],
    docs: [
      "POST JSON to /hooks/webhook on your mycel server.",
      "Optionally set a shared secret for HMAC signature verification.",
    ],
  },
  {
    key: "mqtt", status: "ready" as const,
    label: "MQTT",
    icon: "\u{1F4E1}",
    description: "Subscribe to any MQTT broker topics",
    color: "#660066",
    category: "Custom",
    fields: [
      { key: "broker_url", label: "Broker URL", type: "text" as const, placeholder: "tcp://localhost:1883" },
      { key: "topic", label: "Topic", type: "text" as const, placeholder: "home/sensors/#" },
    ],
    docs: [
      "MQTT broker docs → https://mosquitto.org/ or https://www.hivemq.com/",
      "Enter your broker URL and the topic pattern to subscribe to.",
    ],
  },
  {
    key: "imessage", status: "poll" as const,
    label: "iMessage",
    icon: "\u{1F4AC}",
    description: "iMessage via BlueBubbles API (macOS only)",
    color: "#34C759",
    category: "Chat",
    fields: [
      { key: "api_url", label: "BlueBubbles API URL", placeholder: "http://localhost:1234", required: true },
      { key: "password", label: "API Password", placeholder: "BlueBubbles password", type: "password" as const },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "10", type: "number" },
    ],
    docs: [
      "Install BlueBubbles on a Mac → https://bluebubbles.app",
      "Enable the API server in BlueBubbles settings.",
      "Enter the API URL and password from BlueBubbles.",
    ],
  },
];

export const PLATFORM_MAP = Object.fromEntries(PLATFORMS.map((p) => [p.key, p]));

const CATEGORIES = ["Chat", "Code & DevOps", "Monitoring", "Payments", "Content", "Custom"] as const;

/* ---------- Platform chooser full-screen modal ---------- */

export function PlatformChooser({ onSelect, onClose }: { onSelect: (key: string) => void; onClose: () => void }) {
  const [search, setSearch] = useState("");
  const [connectedGateways, setConnectedGateways] = useState<Map<string, GatewayStatus>>(new Map());
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.listGateways().then((gws) => {
      const m = new Map<string, GatewayStatus>();
      for (const gw of gws ?? []) {
        if (gw.enabled) m.set(gw.platform, gw);
      }
      setConnectedGateways(m);
    }).catch(() => {});
  }, []);

  // Focus search on mount
  useEffect(() => {
    requestAnimationFrame(() => searchRef.current?.focus());
  }, []);

  // Escape to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [onClose]);

  const q = search.toLowerCase();
  const filtered = q
    ? PLATFORMS.filter(
        (p) =>
          p.label.toLowerCase().includes(q) ||
          p.description.toLowerCase().includes(q) ||
          p.category.toLowerCase().includes(q) ||
          p.key.toLowerCase().includes(q),
      )
    : PLATFORMS;

  const connectedPlatforms = filtered.filter((p) => connectedGateways.has(p.key));
  const availablePlatforms = filtered.filter((p) => !connectedGateways.has(p.key));

  const renderCard = (p: PlatformDef) => {
    const isConnected = connectedGateways.has(p.key);
    const isComingSoon = p.status === "coming_soon";

    return (
      <button
        key={p.key}
        type="button"
        onClick={() => !isComingSoon && onSelect(p.key)}
        className={`
          relative flex flex-col p-4 rounded-lg border text-left
          transition-all duration-150 ease-out group
          ${isComingSoon
            ? "border-mycel-border opacity-40 cursor-not-allowed"
            : "border-mycel-border cursor-pointer hover:border-mycel-accent hover:scale-[1.02] hover:shadow-mycel"
          }
          ${isConnected ? "border-mycel-success bg-mycel-success-subtle" : ""}
        `}
      >
        {/* Connected badge */}
        {isConnected && (
          <div className="absolute top-2.5 right-2.5">
            <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
              <circle cx="9" cy="9" r="8" fill="var(--mycel-success)" opacity="0.15" />
              <path d="M5.5 9l2.5 2.5 4.5-4.5" stroke="var(--mycel-success)" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </div>
        )}

        {/* Icon + name */}
        <div className="flex items-center gap-2.5 mb-2">
          {(() => {
            const PIcon = PLATFORM_ICON_MAP[p.key];
            return PIcon
              ? <span className="flex items-center justify-center w-7 h-7"><PIcon size={22} /></span>
              : <span className="text-2xl leading-none select-none">{p.icon}</span>;
          })()}
          <span className={`text-sm font-semibold text-mycel-text transition-colors ${!isComingSoon ? "group-hover:text-mycel-accent" : ""}`}>
            {p.label}
          </span>
        </div>

        {/* Description */}
        <p className="text-xs text-mycel-muted leading-relaxed flex-1">{p.description}</p>

        {/* Status tag */}
        <div className="mt-2.5">
          {isConnected && (
            <span className="inline-flex items-center gap-1 text-[10px] font-medium text-mycel-success">
              <span className="w-1.5 h-1.5 rounded-full bg-mycel-success" />
              Connected
            </span>
          )}
          {!isConnected && p.status === "webhook" && (
            <span className="text-[10px] text-mycel-warning">Webhook &middot; requires public URL</span>
          )}
          {!isConnected && p.status === "ready" && (
            <span className="inline-flex items-center gap-1 text-[10px] text-mycel-accent">
              <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: p.color, opacity: 0.7 }} />
              Ready to connect
            </span>
          )}
          {!isConnected && p.status === "poll" && (
            <span className="text-[10px] text-mycel-info">Polling &middot; needs API token</span>
          )}
          {!isConnected && isComingSoon && (
            <span className="text-[10px] text-mycel-muted">Coming soon</span>
          )}
        </div>
      </button>
    );
  };

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ animation: "fadeIn 120ms ease-out" }}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-mycel-overlay backdrop-blur-md"
        onClick={onClose}
      />

      {/* Modal */}
      <div className="relative z-10 bg-mycel-surface-2 border border-mycel-border rounded-lg shadow-mycel-lg flex flex-col w-[calc(100vw-48px)] max-w-[960px] max-h-[calc(100vh-48px)]">
        {/* Header */}
        <div className="px-6 pt-5 pb-4 border-b border-mycel-border shrink-0">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="text-base font-semibold text-mycel-text tracking-tight">Connect a platform</h2>
              <p className="text-xs text-mycel-muted mt-0.5">Choose a service to receive notifications from</p>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="w-8 h-8 flex items-center justify-center rounded-md text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
              aria-label="Close"
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                <path d="M4 4l8 8M12 4l-8 8" />
              </svg>
            </button>
          </div>

          {/* Search */}
          <div className="relative">
            <svg
              className="absolute left-3 top-1/2 -translate-y-1/2 text-mycel-muted"
              width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.3"
            >
              <circle cx="6" cy="6" r="4" />
              <path d="M9 9l3.5 3.5" strokeLinecap="round" />
            </svg>
            <input
              ref={searchRef}
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search platforms..."
              className="w-full pl-9 pr-3 py-2.5 text-sm rounded-md border border-mycel-border bg-mycel-surface text-mycel-text placeholder:text-mycel-muted focus:outline-none focus:ring-1 focus:ring-mycel-accent focus:border-mycel-accent transition-colors"
            />
          </div>
        </div>

        {/* Grid content */}
        <div className="flex-1 overflow-auto px-6 py-5" style={{ scrollbarWidth: "thin", scrollbarColor: "var(--mycel-border) transparent" }}>
          {/* Connected section */}
          {connectedPlatforms.length > 0 && (
            <div className="mb-6">
              <h3 className="text-[11px] font-medium text-mycel-success uppercase tracking-[0.08em] mb-3 flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-mycel-success" />
                Connected
              </h3>
              <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                {connectedPlatforms.map(renderCard)}
              </div>
            </div>
          )}

          {connectedPlatforms.length > 0 && availablePlatforms.length > 0 && (
            <div className="border-t border-mycel-border my-5" />
          )}

          {/* Categorized available platforms */}
          {CATEGORIES.map((cat) => {
            const items = availablePlatforms.filter((p) => p.category === cat);
            if (items.length === 0) return null;
            return (
              <div key={cat} className="mb-6 last:mb-0">
                <h3 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em] mb-3">{cat}</h3>
                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
                  {items.map(renderCard)}
                </div>
              </div>
            );
          })}

          {filtered.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16 text-mycel-muted">
              <svg width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="currentColor" strokeWidth="1.2" className="mb-3 opacity-30">
                <circle cx="14" cy="14" r="9" />
                <path d="M20.5 20.5l7 7" strokeLinecap="round" />
              </svg>
              <p className="text-sm font-medium">No platforms match &ldquo;{search}&rdquo;</p>
            </div>
          )}
        </div>
      </div>
    </div>,
    document.body,
  );
}

/* ---------- Agent subscription step ---------- */

function AgentSubscriptionStep({
  platform,
  platformLabel,
  onDone,
}: {
  platform: string;
  platformLabel: string;
  onDone: () => void;
}) {
  const isTelegram = platform === "telegram" || platform.startsWith("telegram:");
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [mentionOnly, setMentionOnly] = useState<Set<string>>(new Set());
  const [channels, setChannels] = useState<string[]>([]);
  const [selectedChannels, setSelectedChannels] = useState<Set<string>>(new Set());
  const knownChannelsRef = useRef<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refreshingChannels, setRefreshingChannels] = useState(false);

  const loadChannels = useCallback(async () => {
    try {
      const gws = await api.listGateways();
      const gw = (gws ?? []).find((g) => g.platform === platform);
      const discovered = (gw?.channels ?? []).filter(
        (ch) => ch && !ch.endsWith(":general"),
      );
      setChannels(discovered);
      setSelectedChannels((prev) => {
        const known = knownChannelsRef.current;
        if (prev.size === 0 && known.length === 0) {
          // First load: select everything discovered.
          return new Set(discovered);
        }
        // Keep prior picks that still exist; only auto-select *new* channels
        // so a user deselect + Refresh does not re-check deselected ones.
        const knownSet = new Set(known);
        const next = new Set([...prev].filter((c) => discovered.includes(c)));
        for (const c of discovered) {
          if (!knownSet.has(c)) next.add(c);
        }
        return next;
      });
      knownChannelsRef.current = discovered;
    } catch {
      setChannels([]);
    }
  }, [platform]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const list = await api.listAgents();
        if (!cancelled) setAgents(list ?? []);
      } catch {
        if (!cancelled) setAgents([]);
      }
      if (isTelegram) {
        await loadChannels();
      }
      if (!cancelled) setLoading(false);
    })();
    return () => { cancelled = true; };
  }, [platform, isTelegram, loadChannels]);

  const toggleAgent = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
        setMentionOnly((m) => { const nm = new Set(m); nm.delete(name); return nm; });
      } else {
        next.add(name);
      }
      return next;
    });
  };

  const toggleMention = (name: string) => {
    setMentionOnly((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  };

  const toggleChannel = (channel: string) => {
    setSelectedChannels((prev) => {
      const next = new Set(prev);
      if (next.has(channel)) next.delete(channel); else next.add(channel);
      return next;
    });
  };

  const handleRefreshChannels = async () => {
    setRefreshingChannels(true);
    await loadChannels();
    setRefreshingChannels(false);
  };

  const handleDone = async () => {
    setSaving(true);
    try {
      // Telegram: only subscribe to real discovered channels. Never invent
      // telegram:general — DMs arrive as telegram:<username|chat_id>.
      // Other platforms keep the historical platform:general default for now.
      const targets = isTelegram
        ? [...selectedChannels]
        : [`${platform}:general`];

      if (targets.length > 0 && selected.size > 0) {
        await Promise.all(
          targets.flatMap((channel) =>
            [...selected].map((agent) =>
              api.subscribe(channel, agent, mentionOnly.has(agent)).catch(() => {}),
            ),
          ),
        );
      }
    } catch { /* best effort */ }
    setSaving(false);
    onDone();
  };

  const stateColor = (state: string) => {
    if (state === "working" || state === "running") return "var(--mycel-success)";
    if (state === "idle") return "var(--mycel-warning)";
    return "var(--mycel-muted)";
  };

  const channelLeaf = (ch: string) => {
    const i = ch.lastIndexOf(":");
    return i >= 0 ? ch.slice(i + 1) : ch;
  };

  return (
    <div>
      <div className="p-4 border-b border-mycel-border">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-[10px] font-semibold text-mycel-accent bg-mycel-accent-subtle px-2 py-0.5 rounded-full uppercase tracking-wider">Step 2</span>
        </div>
        <h3 className="text-base font-semibold text-mycel-text">Add agents to {platformLabel}</h3>
        <p className="text-xs text-mycel-muted mt-1">
          {isTelegram
            ? "Select agents and the Telegram chats that should deliver to them."
            : "Select which agents should receive notifications from this platform."}
        </p>
      </div>

      <div className="p-4 max-h-[340px] overflow-auto space-y-4">
        {isTelegram && (
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-[11px] font-semibold uppercase tracking-wider text-mycel-muted">Channels</span>
              <button
                type="button"
                onClick={handleRefreshChannels}
                disabled={refreshingChannels}
                className="text-[11px] text-mycel-accent hover:underline disabled:opacity-50"
              >
                {refreshingChannels ? "Refreshing…" : "Refresh"}
              </button>
            </div>
            {loading ? (
              <div className="text-xs text-mycel-muted bg-mycel-surface-hover border border-mycel-border rounded-md px-3 py-2">
                Loading channels…
              </div>
            ) : channels.length === 0 ? (
              <div className="text-xs text-mycel-muted bg-mycel-surface-hover border border-mycel-border rounded-md px-3 py-2">
                No Telegram chats discovered yet. Message the bot in a DM (or a group),
                then click Refresh. Agents subscribed to a fake <code className="text-mycel-text-2">telegram:general</code> channel
                never receive real traffic — first message also auto-migrates legacy
                <code className="text-mycel-text-2"> :general</code> subscriptions.
              </div>
            ) : (
              <div className="space-y-1">
                {channels.map((ch) => (
                  <label
                    key={ch}
                    className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-mycel-surface-hover cursor-pointer transition-colors"
                  >
                    <input
                      type="checkbox"
                      checked={selectedChannels.has(ch)}
                      onChange={() => toggleChannel(ch)}
                      className="shrink-0 accent-[var(--mycel-accent)]"
                    />
                    <span className="text-sm text-mycel-text flex-1 min-w-0 truncate" title={ch}>
                      {channelLeaf(ch)}
                    </span>
                    <span className="text-[10px] text-mycel-muted font-mono truncate max-w-[40%]">{ch}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
        )}

        {loading ? (
          <div className="text-center py-6 text-mycel-muted text-xs">Loading agents...</div>
        ) : agents.length === 0 ? (
          <div className="text-center py-6 text-mycel-muted text-xs">No agents found</div>
        ) : (
          <div>
            {isTelegram && (
              <div className="text-[11px] font-semibold uppercase tracking-wider text-mycel-muted mb-2">Agents</div>
            )}
            <div className="space-y-1">
              {agents.filter((a) => !a.archived_at).map((agent) => (
                <label
                  key={agent.name}
                  className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-mycel-surface-hover cursor-pointer transition-colors"
                >
                  <input
                    type="checkbox"
                    checked={selected.has(agent.name)}
                    onChange={() => toggleAgent(agent.name)}
                    className="shrink-0 accent-[var(--mycel-accent)]"
                  />
                  <span
                    className="shrink-0 w-2 h-2 rounded-full"
                    style={{ backgroundColor: stateColor(agent.state) }}
                    title={agent.state}
                  />
                  <span className="text-sm text-mycel-text flex-1 min-w-0 truncate">{agent.name}</span>
                  {selected.has(agent.name) && (
                    <button
                      type="button"
                      onClick={(e) => { e.preventDefault(); e.stopPropagation(); toggleMention(agent.name); }}
                      className="shrink-0 text-[10px] px-2 py-0.5 rounded-full border transition-colors"
                      style={{
                        borderColor: mentionOnly.has(agent.name) ? "color-mix(in oklab, var(--mycel-accent) 40%, transparent)" : "var(--mycel-border)",
                        color: mentionOnly.has(agent.name) ? "var(--mycel-accent)" : "var(--mycel-muted)",
                        background: mentionOnly.has(agent.name) ? "var(--mycel-accent-subtle)" : "transparent",
                      }}
                      title={mentionOnly.has(agent.name) ? "Mention only: ON" : "Mention only: OFF"}
                    >
                      @mention only
                    </button>
                  )}
                </label>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="flex justify-between items-center gap-2 p-4 border-t border-mycel-border">
        <span className="text-xs text-mycel-text-2">
          {selected.size} agent{selected.size !== 1 ? "s" : ""}
          {isTelegram ? ` · ${selectedChannels.size} channel${selectedChannels.size !== 1 ? "s" : ""}` : ""} selected
        </span>
        <button
          type="button"
          onClick={handleDone}
          disabled={saving}
          className="inline-flex items-center h-9 px-3 text-sm text-mycel-accent-fg bg-mycel-accent hover:bg-mycel-accent-hover shadow-mycel-sm rounded-md font-medium transition-colors disabled:opacity-50"
        >
          {saving ? "Saving..." : "Done"}
        </button>
      </div>
    </div>
  );
}

/* ---------- Linkify URLs in doc strings ---------- */

function linkifyDoc(text: string): React.ReactNode {
  const urlRe = /(https?:\/\/[^\s,)]+)/g;
  const parts = text.split(urlRe);
  if (parts.length === 1) return text;
  return parts.map((part, i) =>
    /^https?:\/\//.test(part) ? (
      <a key={i} href={part} target="_blank" rel="noopener noreferrer" style={{ color: "var(--mycel-accent)", textDecoration: "underline" }}>
        {part.replace(/^https?:\/\//, "")}
      </a>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

/* ---------- Setup wizard (credential form + agent subscription) ---------- */

export function SetupWizard({
  platform,
  onClose,
  onConnected,
}: {
  platform: string;
  onClose: () => void;
  onConnected: () => void;
}) {
  const config = PLATFORM_MAP[platform];
  const [values, setValues] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [step, setStep] = useState<"credentials" | "agents">("credentials");
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [pairState, setPairState] = useState<string>("idle");
  const qrPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const qrTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Escape to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [onClose]);

  // Cleanup QR poll interval on unmount.
  useEffect(() => {
    return () => {
      if (qrPollRef.current) clearInterval(qrPollRef.current);
      if (qrTimeoutRef.current) clearTimeout(qrTimeoutRef.current);
    };
  }, []);

  if (!config) {
    return createPortal(
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-mycel-overlay backdrop-blur-sm">
        <div className="bg-mycel-surface-2 border border-mycel-border rounded-lg p-6 max-w-md w-full mx-4 shadow-mycel-lg">
          <p className="text-mycel-muted">Unknown platform: {platform}</p>
          <button type="button" onClick={onClose} className="mt-4 text-sm text-mycel-accent">
            Close
          </button>
        </div>
      </div>,
      document.body,
    );
  }

  const startQRPairing = async () => {
    setPairState("loading");
    setError(null);
    try {
      const resp = await fetch(`/api/gateways/${platform}/pair`, { method: "POST" });
      const data = await resp.json();
      if (!resp.ok) { setError(data.error || "Failed to start pairing"); setPairState("error"); return; }
      if (data.state === "connected") { setPairState("connected"); onConnected(); return; }
      if (data.qr_data_url) { setQrDataUrl(data.qr_data_url); setPairState("qr_ready"); }
      // Poll for connection.
      const pollId = setInterval(async () => {
        const s = await fetch(`/api/gateways/${platform}/pair/status`).then(r => r.json());
        if (s.state === "connected") { clearInterval(pollId); qrPollRef.current = null; setPairState("connected"); onConnected(); }
        else if (s.state === "error") { clearInterval(pollId); qrPollRef.current = null; setPairState("error"); setError(s.error); }
      }, 2000);
      qrPollRef.current = pollId;
      // Stop polling after 2 minutes.
      const timeoutId = setTimeout(() => { clearInterval(pollId); qrPollRef.current = null; }, 120000);
      qrTimeoutRef.current = timeoutId;
    } catch (e) { setError(String(e)); setPairState("error"); }
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      const body: Record<string, unknown> = { enabled: true };

      if (platform === "slack") body.mode = "socket";
      else if (platform === "telegram" || platform === "discord") body.mode = "polling";

      for (const field of config.fields) {
        const val = (values[field.key] ?? "").trim();
        if (field.required !== false && !val) {
          setError(`${field.label} is required`);
          setSaving(false);
          return;
        }
        if (val) {
          if (field.key === "channels") {
            body[field.key] = val.split(",").map((s: string) => s.trim()).filter(Boolean);
          } else {
            body[field.key] = field.type === "number" ? Number(val) : val;
          }
        }
      }

      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ gateways: { [platform]: body } }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `HTTP ${res.status}`);
      }

      setStep("agents");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    }
    setSaving(false);
  };

  const handleAgentsDone = () => {
    onConnected();
    onClose();
  };

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-mycel-overlay backdrop-blur-sm" style={{ animation: 'fadeIn 120ms ease-out' }}>
      <div className="bg-mycel-surface-2 border border-mycel-border rounded-lg max-w-lg w-full mx-4 max-h-[85vh] overflow-auto shadow-mycel-lg">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-mycel-border">
          <h2 className="text-base font-semibold text-mycel-text flex items-center gap-2">
            {(() => {
              const WizIcon = PLATFORM_ICON_MAP[platform];
              return WizIcon
                ? <span className="flex items-center"><WizIcon size={18} /></span>
                : <span>{config.icon}</span>;
            })()}
            {step === "credentials" ? `Connect ${config.label}` : `${config.label} Setup`}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center rounded-lg text-mycel-muted hover:text-mycel-text hover:bg-mycel-surface-hover transition-colors"
            aria-label="Close"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
              <path d="M4 4l8 8M12 4l-8 8" />
            </svg>
          </button>
        </div>

        {/* Step indicator */}
        <div className="px-4 pt-3 flex items-center gap-2">
          <span
            className="text-[10px] font-semibold px-2 py-0.5 rounded-full uppercase tracking-wider"
            style={{
              color: step === "credentials" ? "var(--mycel-accent)" : "var(--mycel-success)",
              background: step === "credentials" ? "var(--mycel-accent-subtle)" : "var(--mycel-success-subtle)",
            }}
          >
            Step 1
          </span>
          <span className="text-xs text-mycel-muted">
            {step === "credentials" ? "Enter credentials" : "Connected"}
          </span>
          <span className="text-mycel-muted mx-1">&rarr;</span>
          <span
            className="text-[10px] font-semibold px-2 py-0.5 rounded-full uppercase tracking-wider"
            style={{
              color: step === "agents" ? "var(--mycel-accent)" : "var(--mycel-muted)",
              background: step === "agents" ? "var(--mycel-accent-subtle)" : "transparent",
              border: step === "agents" ? "none" : "1px solid var(--mycel-border)",
            }}
          >
            Step 2
          </span>
          <span className="text-xs text-mycel-muted">Add agents</span>
        </div>

        {step === "credentials" ? (
          <>
            {/* Setup docs */}
            <div className="p-4 border-b border-mycel-border">
              <h3 className="text-[11px] font-medium text-mycel-muted uppercase tracking-[0.08em] mb-2">
                Setup Steps
              </h3>
              <ol className="space-y-1.5">
                {config.docs.map((docStep, i) => (
                  <li key={i} className="flex gap-2 text-xs text-mycel-text-2">
                    <span className="text-mycel-accent tabular-nums shrink-0">{i + 1}.</span>
                    <span>{linkifyDoc(docStep)}</span>
                  </li>
                ))}
              </ol>
            </div>

            {/* QR code pairing flow */}
            {"pairFlow" in config && config.pairFlow === "qr" ? (
              <div className="p-6 flex flex-col items-center gap-4">
                {pairState === "idle" && (
                  <button
                    type="button"
                    onClick={startQRPairing}
                    className="inline-flex items-center h-9 px-6 bg-[#25D366] hover:bg-[#20bd5a] text-white rounded-md font-medium text-sm shadow-mycel-sm transition-colors"
                  >
                    Generate QR Code
                  </button>
                )}
                {pairState === "loading" && (
                  <div className="text-mycel-muted text-sm animate-pulse">Generating QR code...</div>
                )}
                {pairState === "qr_ready" && qrDataUrl && (
                  <div className="flex flex-col items-center gap-3">
                    <img src={qrDataUrl} alt="WhatsApp QR Code" className="w-56 h-56 rounded-lg border border-mycel-border" />
                    <p className="text-xs text-mycel-muted text-center">
                      Open WhatsApp → Linked Devices → Link a Device<br />
                      Scan this QR code with your phone
                    </p>
                    <div className="flex items-center gap-2 text-xs text-mycel-muted">
                      <span className="w-2 h-2 bg-mycel-warning rounded-full animate-pulse" />
                      Waiting for scan...
                    </div>
                  </div>
                )}
                {pairState === "connected" && (
                  <div className="flex items-center gap-2 text-sm text-mycel-success">
                    <span className="text-lg">✓</span> WhatsApp connected!
                  </div>
                )}
              </div>
            ) : (
            /* Token inputs */
            <div className="p-4 space-y-3">
              {config.fields.map((field) => (
                <div key={field.key}>
                  <label className="block text-sm font-medium text-mycel-text-2 mb-1">
                    {field.label}
                    {field.required === false && <span className="text-mycel-muted ml-1">(optional)</span>}
                  </label>
                  <input
                    type={field.type === "url" ? "url" : field.type === "number" ? "text" : "password"}
                    value={values[field.key] ?? ""}
                    onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                    placeholder={field.placeholder}
                    className="w-full px-3 py-2 bg-mycel-surface border border-mycel-border rounded-md text-sm text-mycel-text placeholder:text-mycel-muted focus:border-mycel-accent focus:outline-none transition-colors"
                  />
                </div>
              ))}
            </div>
            )}

            {/* Error */}
            {error && (
              <div className="mx-4 mb-3 px-3 py-2 bg-mycel-error-subtle border border-mycel-error rounded-md text-xs text-mycel-error">
                {error}
              </div>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-2 p-4 border-t border-mycel-border">
              <button
                type="button"
                onClick={onClose}
                className="inline-flex items-center h-9 px-3 text-sm bg-mycel-surface border border-mycel-border text-mycel-text-2 hover:text-mycel-text hover:bg-mycel-surface-hover rounded-md transition-colors"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleSave}
                disabled={saving}
                className="inline-flex items-center h-9 px-3 text-sm text-mycel-accent-fg bg-mycel-accent hover:bg-mycel-accent-hover shadow-mycel-sm rounded-md font-medium transition-colors disabled:opacity-50"
              >
                {saving ? "Connecting..." : "Connect"}
              </button>
            </div>
          </>
        ) : (
          <AgentSubscriptionStep
            platform={platform}
            platformLabel={config.label}
            onDone={handleAgentsDone}
          />
        )}
      </div>
    </div>,
    document.body,
  );
}
