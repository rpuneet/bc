/**
 * Shared gateway/app connection-status helpers.
 *
 * Extracted from Layout's NotificationNavTree so the notifications home
 * page and the drawer render the exact same status semantics: one
 * status ladder, one disconnect-reason mapping, one dot.
 */

import type { GatewayHealth, GatewayStatus } from "../../api/client";

export type AppStatus = "connected" | "connecting" | "error" | "idle";

export function getAppStatus(gw?: GatewayStatus, h?: GatewayHealth): AppStatus {
  if (h) {
    if (h.connected) return "connected";
    return h.status === "connecting" ? "connecting" : "error";
  }
  // Enabled gateway whose health has not reported yet reads as connecting.
  if (gw?.enabled) return "connecting";
  return "idle";
}

/** Map raw gateway errors to a short human-readable reason. */
export function disconnectReason(base: string, h?: GatewayHealth): string {
  if (base === "whatsapp") return "Scan QR to re-pair";
  const err = h?.error ?? "";
  if (/\b402\b|payment required|quota/i.test(err)) return "API quota/payment required";
  if (/\b401\b|unauthorized|invalid[ _-]?(auth|token|credentials)/i.test(err)) return "Invalid credentials";
  if (/\b403\b|forbidden/i.test(err)) return "Access denied";
  if (/\b429\b|rate[ _-]?limit/i.test(err)) return "Rate limited";
  if (/timeout|timed out|refused|unreachable|network|no such host|dns/i.test(err)) return "Connection failed";
  return err || "Disconnected";
}

export const STATUS_DOT_TOKEN: Record<AppStatus, string> = {
  connected: "var(--mycel-success)",
  connecting: "var(--mycel-warning)",
  error: "var(--mycel-error)",
  idle: "var(--mycel-muted)",
};

/** 6px connection dot — the only status signal a healthy app shows. */
export function StatusDot({ status, title }: { status: AppStatus; title?: string }) {
  const token = STATUS_DOT_TOKEN[status];
  return (
    <span
      className="shrink-0"
      title={title}
      style={{
        width: 6,
        height: 6,
        borderRadius: 999,
        background: token,
        opacity: status === "idle" ? 0.35 : 1,
        boxShadow: status === "connected" ? `0 0 5px color-mix(in srgb, ${token} 50%, transparent)` : "none",
      }}
    />
  );
}

// Parse an ISO timestamp, guarding zero / unset / pre-2001 values that
// produce nonsensical "17753690h ago" strings.
export function parseActivityTs(iso?: string): number | null {
  if (!iso) return null;
  const ts = new Date(iso).getTime();
  return Number.isFinite(ts) && ts > 978307200000 ? ts : null;
}

/** Compact relative time: "now", "5m", "3h", "2d". */
export function formatAgoShort(ts: number): string {
  const mins = Math.floor((Date.now() - ts) / 60000);
  if (mins < 1) return "now";
  if (mins < 60) return `${String(mins)}m`;
  if (mins < 1440) return `${String(Math.floor(mins / 60))}h`;
  return `${String(Math.floor(mins / 1440))}d`;
}
