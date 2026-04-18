import { useState } from "react";
import { createPortal } from "react-dom";

const PLATFORM_STEPS: Record<
  string,
  { label: string; fields: { key: string; label: string; placeholder: string }[]; docs: string[] }
> = {
  slack: {
    label: "Slack",
    fields: [
      { key: "bot_token", label: "Bot Token", placeholder: "xoxb-..." },
      { key: "app_token", label: "App Token", placeholder: "xapp-..." },
    ],
    docs: [
      "Go to api.slack.com/apps → your app (or create one)",
      "Enable Socket Mode: Settings → Socket Mode → toggle ON",
      "Add scopes: OAuth → Bot Token Scopes → channels:read, chat:write, connections:write",
      "Copy Bot Token from OAuth & Permissions page",
      "Generate App Token from Basic Information → App-Level Tokens (connections:write scope)",
      "Install/reinstall the app to your workspace",
      "Invite the bot to channels: /invite @your-bot",
    ],
  },
  telegram: {
    label: "Telegram",
    fields: [{ key: "bot_token", label: "Bot Token", placeholder: "1234567890:AAH..." }],
    docs: [
      "Open Telegram, message @BotFather",
      "Send /newbot to create a new bot (or /mybots for existing ones)",
      "Copy the bot token BotFather gives you",
      "Add the bot to your group chat",
      "Optional: Send /setprivacy → Disable (so bot sees all messages, not just commands)",
    ],
  },
  discord: {
    label: "Discord",
    fields: [{ key: "bot_token", label: "Bot Token", placeholder: "MTIz..." }],
    docs: [
      "Go to discord.com/developers/applications",
      "Create or select your application",
      "Go to Bot → enable MESSAGE CONTENT INTENT (privileged)",
      "Copy the bot token from the Bot page",
      "Generate an invite URL: OAuth2 → URL Generator → bot scope + Send Messages + Read Message History permissions",
      "Open the invite URL to add the bot to your server",
    ],
  },
  github: {
    label: "GitHub",
    fields: [{ key: "token", label: "Token / Webhook Secret", placeholder: "ghp_... or webhook secret" }],
    docs: [
      "Go to github.com/settings/apps (or create a GitHub App)",
      "Configure webhook events: Pull request, Issue comment, Pull request review",
      "Set a webhook secret for verification",
      "Copy the token or webhook secret",
    ],
  },
  gmail: {
    label: "Gmail",
    fields: [{ key: "token", label: "OAuth Token", placeholder: "OAuth access token" }],
    docs: [
      "Set up a Google Cloud project with Gmail API enabled",
      "Create OAuth 2.0 credentials",
      "Authorize with Gmail scope",
    ],
  },
};

export function SetupWizard({
  platform,
  onClose,
  onConnected,
}: {
  platform: string;
  onClose: () => void;
  onConnected: () => void;
}) {
  const config = PLATFORM_STEPS[platform];
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
      const body: Record<string, unknown> = { enabled: true, mode: platform === "slack" ? "socket" : "polling" };
      for (const field of config.fields) {
        if (!values[field.key]?.trim()) {
          setError(`${field.label} is required`);
          setSaving(false);
          return;
        }
        body[field.key] = (values[field.key] ?? "").trim();
      }

      const res = await fetch(`/api/gateways/${platform}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
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
          <h2 className="text-[15px] font-semibold text-bc-text">
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
              </label>
              <input
                type="password"
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
