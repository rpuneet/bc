import { useState } from "react";
import { createPortal } from "react-dom";

export interface PlatformDef {
  key: string;
  label: string;
  icon: string;
  description: string;
  color: string;
  category: string;
  fields: { key: string; label: string; placeholder: string; required?: boolean; type?: string }[];
  docs: string[];
}

export const PLATFORMS: PlatformDef[] = [
  // --- Chat ---
  {
    key: "slack",
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
      "Create a Slack app at api.slack.com/apps, enable Socket Mode.",
      "Add scopes: channels:read, chat:write, connections:write.",
      "Copy Bot Token from OAuth & Permissions, App Token from Basic Information.",
      "Install the app and invite the bot to your channels.",
    ],
  },
  {
    key: "telegram",
    label: "Telegram",
    icon: "\u2708\uFE0F",
    description: "Bot messages via long polling",
    color: "#26A5E4",
    category: "Chat",
    fields: [{ key: "bot_token", label: "Bot Token", placeholder: "1234567890:AAH..." }],
    docs: [
      "Message @BotFather on Telegram, send /newbot.",
      "Copy the bot token and add the bot to your group.",
    ],
  },
  {
    key: "discord",
    label: "Discord",
    icon: "\u{1F3AE}",
    description: "Bot messages from Discord servers",
    color: "#5865F2",
    category: "Chat",
    fields: [{ key: "bot_token", label: "Bot Token", placeholder: "MTIz..." }],
    docs: [
      "Create an app at discord.com/developers/applications.",
      "Enable MESSAGE CONTENT INTENT, copy the bot token.",
      "Generate an invite URL with bot scope and add to your server.",
    ],
  },

  // --- Code & DevOps ---
  {
    key: "github",
    label: "GitHub",
    icon: "\u{1F419}",
    description: "PR, issue, and push webhooks",
    color: "#8B949E",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Webhook Secret", placeholder: "your-webhook-secret" }],
    docs: [
      "Create a webhook in your repo settings (Settings > Webhooks).",
      "Set the payload URL to your bc server\u2019s /hooks/github endpoint.",
      "Set the secret here to match the webhook secret.",
    ],
  },
  {
    key: "gitlab",
    label: "GitLab",
    icon: "\u{1F98A}",
    description: "Merge request and pipeline webhooks",
    color: "#FC6D26",
    category: "Code & DevOps",
    fields: [{ key: "token", label: "Token", placeholder: "webhook-secret-token" }],
    docs: [
      "Go to your GitLab project > Settings > Webhooks.",
      "Set the URL to your bc server\u2019s /hooks/gitlab endpoint.",
      "Copy the secret token and paste it here.",
    ],
  },
  {
    key: "bitbucket",
    label: "Bitbucket",
    icon: "\u{1FAA3}",
    description: "Push and PR webhooks",
    color: "#0052CC",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Secret", placeholder: "webhook-secret" }],
    docs: [
      "Go to your Bitbucket repo > Settings > Webhooks.",
      "Add a webhook pointing to your bc server\u2019s /hooks/bitbucket endpoint.",
      "Set the secret here for payload verification.",
    ],
  },
  {
    key: "jira",
    label: "Jira",
    icon: "\u{1F4CB}",
    description: "Issue and sprint webhooks",
    color: "#0052CC",
    category: "Code & DevOps",
    fields: [{ key: "token", label: "Token", placeholder: "webhook-secret" }],
    docs: [
      "Go to Jira > Settings > System > Webhooks.",
      "Create a webhook pointing to your bc server\u2019s /hooks/jira endpoint.",
      "Set the secret token for verification.",
    ],
  },
  {
    key: "linear",
    label: "Linear",
    icon: "\u{1F4D0}",
    description: "Issue and project webhooks",
    color: "#5E6AD2",
    category: "Code & DevOps",
    fields: [{ key: "api_key", label: "API Key", placeholder: "lin_api_..." }],
    docs: [
      "Go to Linear > Settings > API > Webhooks.",
      "Create a webhook pointing to your bc server\u2019s /hooks/linear endpoint.",
      "Copy your API key from Settings > API > Personal API keys.",
    ],
  },
  {
    key: "vercel",
    label: "Vercel",
    icon: "\u25B2",
    description: "Deployment and build webhooks",
    color: "#000000",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Secret", placeholder: "whsec_..." }],
    docs: [
      "Go to your Vercel project > Settings > Webhooks.",
      "Add a webhook endpoint pointing to /hooks/vercel on your bc server.",
      "Copy the signing secret and paste it here.",
    ],
  },
  {
    key: "netlify",
    label: "Netlify",
    icon: "\u25C6",
    description: "Deploy and build notifications",
    color: "#00C7B7",
    category: "Code & DevOps",
    fields: [{ key: "secret", label: "Secret", placeholder: "webhook-secret" }],
    docs: [
      "Go to your Netlify site > Site settings > Notifications.",
      "Add an outgoing webhook pointing to /hooks/netlify on your bc server.",
      "Set and copy the secret for payload verification.",
    ],
  },

  // --- Monitoring ---
  {
    key: "sentry",
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
    key: "pagerduty",
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
    key: "datadog",
    label: "Datadog",
    icon: "\u{1F415}",
    description: "Monitor and event webhooks",
    color: "#632CA6",
    category: "Monitoring",
    fields: [{ key: "api_key", label: "API Key", placeholder: "datadog-api-key" }],
    docs: [
      "Go to Datadog > Integrations > Webhooks.",
      "Create a webhook pointing to /hooks/datadog on your bc server.",
      "Copy your API key from Organization Settings > API Keys.",
    ],
  },
  {
    key: "grafana",
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

  // --- Payments ---
  {
    key: "stripe",
    label: "Stripe",
    icon: "\u{1F4B3}",
    description: "Payment and subscription events",
    color: "#635BFF",
    category: "Payments",
    fields: [{ key: "webhook_secret", label: "Webhook Secret", placeholder: "whsec_..." }],
    docs: [
      "Go to Stripe Dashboard > Developers > Webhooks.",
      "Add an endpoint pointing to /hooks/stripe on your bc server.",
      "Copy the signing secret and paste it here.",
    ],
  },

  // --- Content ---
  {
    key: "rss",
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
    key: "notion",
    label: "Notion",
    icon: "\u{1F4DD}",
    description: "Database and page change polling",
    color: "#000000",
    category: "Content",
    fields: [
      { key: "api_token", label: "API Token", placeholder: "secret_..." },
      { key: "interval", label: "Poll Interval (seconds)", placeholder: "300", type: "number" },
    ],
    docs: [
      "Create an internal integration at notion.so/my-integrations.",
      "Copy the API token and share target pages/databases with the integration.",
      "Set a poll interval in seconds (default: 300).",
    ],
  },

  // --- Custom ---
  {
    key: "webhook",
    label: "Generic Webhook",
    icon: "\u{1F517}",
    description: "Receive any JSON webhook payload",
    color: "#8c7e72",
    category: "Custom",
    fields: [{ key: "secret", label: "Shared Secret (optional)", placeholder: "optional-secret", required: false }],
    docs: [
      "POST JSON to /hooks/webhook on your bc server.",
      "Optionally set a shared secret for HMAC signature verification.",
    ],
  },
];

export const PLATFORM_MAP = Object.fromEntries(PLATFORMS.map((p) => [p.key, p]));

const CATEGORIES = ["Chat", "Code & DevOps", "Monitoring", "Payments", "Content", "Custom"] as const;

/* ---------- Platform chooser grid ---------- */

export function PlatformChooser({ onSelect, onClose }: { onSelect: (key: string) => void; onClose: () => void }) {
  const [search, setSearch] = useState("");
  const q = search.toLowerCase();
  const filtered = q ? PLATFORMS.filter((p) => p.label.toLowerCase().includes(q) || p.description.toLowerCase().includes(q) || p.category.toLowerCase().includes(q)) : PLATFORMS;

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md"
      onClick={onClose}
    >
      <div
        className="bg-bc-bg border border-bc-border rounded-2xl w-full max-w-2xl mx-4 shadow-2xl max-h-[90vh] flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-6 pt-5 pb-4 border-b border-bc-border/50">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-bold text-bc-text">Connect a platform</h2>
            <button type="button" onClick={onClose} className="text-bc-muted hover:text-bc-text transition-colors text-xl leading-none">&times;</button>
          </div>
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search platforms..."
            autoFocus
            className="w-full px-3 py-2 text-sm rounded-lg border border-bc-border/50 bg-bc-surface/30 text-bc-text placeholder:text-bc-muted/50 focus:outline-none focus:ring-1 focus:ring-bc-accent"
          />
        </div>

        {/* Platform grid */}
        <div className="px-6 py-4 overflow-auto flex-1">
          {CATEGORIES.map((cat) => {
            const items = filtered.filter((p) => p.category === cat);
            if (items.length === 0) return null;
            return (
              <div key={cat} className="mb-5">
                <h3 className="text-[11px] font-bold text-bc-muted uppercase tracking-wider mb-3">{cat}</h3>
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                  {items.map((p) => (
                    <button
                      key={p.key}
                      type="button"
                      onClick={() => onSelect(p.key)}
                      className="p-4 border border-bc-border/40 rounded-xl hover:border-bc-accent/60 hover:bg-bc-accent/5 transition-all text-left group relative"
                    >
                      <div className="flex items-center gap-3 mb-1.5">
                        <span className="text-2xl leading-none">{p.icon}</span>
                        <span className="text-sm font-semibold text-bc-text group-hover:text-bc-accent transition-colors">{p.label}</span>
                      </div>
                      <p className="text-xs text-bc-muted leading-snug">{p.description}</p>
                      <div
                        className="absolute top-2 right-2 w-2 h-2 rounded-full"
                        style={{ backgroundColor: p.color, opacity: 0.6 }}
                      />
                    </button>
                  ))}
                </div>
              </div>
            );
          })}
          {filtered.length === 0 && (
            <div className="text-center py-10 text-bc-muted">No platforms match &ldquo;{search}&rdquo;</div>
          )}
        </div>
      </div>
    </div>,
    document.body,
  );
}

/* ---------- Setup wizard (credential form) ---------- */

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
  const [success, setSuccess] = useState(false);

  if (!config) {
    return createPortal(
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
        <div className="bg-bc-bg border border-bc-border/50 rounded-xl p-6 max-w-md w-full mx-4 shadow-2xl">
          <p className="text-bc-muted">Unknown platform: {platform}</p>
          <button type="button" onClick={onClose} className="mt-4 text-sm text-bc-accent">
            Close
          </button>
        </div>
      </div>,
      document.body,
    );
  }

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      const body: Record<string, unknown> = { enabled: true };

      // Set mode for known adapter types
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
          body[field.key] = field.type === "number" ? Number(val) : val;
        }
      }

      // Save gateway config via settings API
      const res = await fetch("/api/settings", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ gateways: { [platform]: body } }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `HTTP ${res.status}`);
      }

      setSuccess(true);
      setTimeout(() => {
        onConnected();
        onClose();
      }, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    }
    setSaving(false);
  };

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/70 backdrop-blur-sm" style={{ animation: 'fadeIn 120ms ease-out' }}>
      <div className="bg-bc-bg border border-bc-border/50 rounded-xl max-w-lg w-full mx-4 max-h-[85vh] overflow-auto shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-bc-border">
          <h2 className="text-[15px] font-semibold text-bc-text flex items-center gap-2">
            <span>{config.icon}</span>
            Connect {config.label}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="text-bc-muted hover:text-bc-text text-lg transition-colors"
          >
            &times;
          </button>
        </div>

        {/* Steps */}
        <div className="p-4 border-b border-bc-border/50">
          <h3 className="text-[11px] font-semibold text-bc-muted uppercase tracking-widest mb-2">
            Setup Steps
          </h3>
          <ol className="space-y-1.5">
            {config.docs.map((step, i) => (
              <li key={i} className="flex gap-2 text-[12px] text-bc-text/70">
                <span className="text-bc-accent font-mono shrink-0">{i + 1}.</span>
                <span>{step}</span>
              </li>
            ))}
          </ol>
        </div>

        {/* Token inputs */}
        <div className="p-4 space-y-3">
          {config.fields.map((field) => (
            <div key={field.key}>
              <label className="block text-[11px] font-medium text-bc-muted mb-1">
                {field.label}
                {field.required === false && <span className="text-bc-muted/30 ml-1">(optional)</span>}
              </label>
              <input
                type={field.type === "url" ? "url" : field.type === "number" ? "text" : "password"}
                value={values[field.key] ?? ""}
                onChange={(e) => setValues((v) => ({ ...v, [field.key]: e.target.value }))}
                placeholder={field.placeholder}
                className="w-full px-3 py-2 bg-bc-surface border border-bc-border rounded text-[13px] text-bc-text placeholder:text-bc-muted/30 focus:border-bc-accent focus:outline-none transition-colors"
              />
            </div>
          ))}
        </div>

        {/* Error / Success */}
        {error && (
          <div className="mx-4 mb-3 px-3 py-2 bg-bc-error/10 border border-bc-error/20 rounded text-[12px] text-bc-error">
            {error}
          </div>
        )}
        {success && (
          <div className="mx-4 mb-3 px-3 py-2 bg-bc-success/10 border border-bc-success/20 rounded text-[12px] text-bc-success">
            Connected! Restarting gateway adapter...
          </div>
        )}

        {/* Actions */}
        <div className="flex justify-end gap-2 p-4 border-t border-bc-border">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-1.5 text-[12px] text-bc-muted hover:text-bc-text border border-bc-border rounded transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || success}
            className="px-4 py-1.5 text-[12px] text-bc-bg bg-bc-accent hover:bg-bc-accent-hover rounded font-medium transition-colors disabled:opacity-50"
          >
            {saving ? "Saving..." : success ? "Connected!" : "Connect"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
