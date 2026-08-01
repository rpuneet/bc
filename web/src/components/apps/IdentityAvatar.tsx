/**
 * IdentityAvatar — a real-identity avatar for people, chats and app
 * participants shown on the Apps page. Renders the platform's own
 * profile picture when a URL is available; otherwise a deterministic
 * initials chip whose colour is derived from the name (same name →
 * same colour everywhere). This is deliberately NOT the mycel
 * AgentCharacter mushroom: notifications and channels surface real
 * chat participants, which have nothing to do with mycel agents.
 *
 * No avatar URLs are exposed by the notifications/channels API today,
 * so the initials fallback is what renders in practice — when the
 * backend starts resolving real profile pictures, pass `src` and they
 * appear with zero further changes here.
 */

import { hashName } from "../agent-ui/identity";
import type { ChannelKind } from "./AppsHome";

/** First one-or-two letters of a human name: "Puneet Rai" → "PR",
 *  "zen-zebra" → "ZZ", "general" → "GE". Non-alphanumerics separate
 *  words; a single word contributes its first two letters. */
export function initialsFor(name: string): string {
  const words = name.trim().split(/[^\p{L}\p{N}]+/u).filter(Boolean);
  if (words.length === 0) return "?";
  if (words.length === 1) {
    const w = words[0] ?? "";
    return (w.slice(0, 2) || "?").toUpperCase();
  }
  return ((words[0]?.[0] ?? "") + (words[1]?.[0] ?? "")).toUpperCase();
}

/** Deterministic mid-tone HSL that keeps white text legible in both
 *  light and dark themes (fixed saturation/lightness, hue from name). */
export function avatarColor(name: string): string {
  const hue = hashName(name) % 360;
  return `hsl(${String(hue)} 52% 45%)`;
}

export interface IdentityAvatarProps {
  /** Human-readable name — drives initials and fallback colour. */
  name: string;
  /** Real profile-picture URL when the platform provides one. */
  src?: string;
  size?: number;
  /** Round for people, rounded-square for group chats. */
  kind?: ChannelKind;
  title?: string;
  className?: string;
}

export function IdentityAvatar({ name, src, size = 20, kind = null, title, className = "" }: IdentityAvatarProps) {
  const radius = kind === "group" ? Math.round(size * 0.28) : size / 2;
  const shared = {
    width: size,
    height: size,
    borderRadius: radius,
  } as const;

  if (src) {
    return (
      <img
        src={src}
        alt={name}
        title={title ?? name}
        loading="lazy"
        className={`shrink-0 object-cover bg-mycel-surface-hover ${className}`}
        style={shared}
      />
    );
  }

  return (
    <span
      aria-hidden={title ? undefined : true}
      title={title ?? name}
      className={`shrink-0 inline-flex items-center justify-center font-semibold text-white select-none ${className}`}
      style={{
        ...shared,
        backgroundColor: avatarColor(name),
        fontSize: Math.max(9, Math.round(size * 0.42)),
        lineHeight: 1,
      }}
    >
      {initialsFor(name)}
    </span>
  );
}
