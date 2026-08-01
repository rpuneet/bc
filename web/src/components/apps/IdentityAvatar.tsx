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

import { useState } from "react";
import { hashName } from "../agent-ui/identity";
import type { ChannelKind } from "./AppsHome";

/** First code point of a word, Unicode-safe (handles emoji/multibyte). */
function firstCodePoint(word: string): string {
  return Array.from(word)[0] ?? "";
}

/** First one-or-two glyphs of a human name: "Puneet Rai" → "PR",
 *  "zen-zebra" → "ZZ", "general" → "GE". Non-alphanumerics separate
 *  words; a single word contributes its first two code points. Uses
 *  code-point iteration so multibyte/emoji names never split a glyph. */
export function initialsFor(name: string): string {
  const words = name.trim().split(/[^\p{L}\p{N}]+/u).filter(Boolean);
  if (words.length === 0) return "?";
  if (words.length === 1) {
    const w = Array.from(words[0] ?? "").slice(0, 2).join("");
    return (w || "?").toUpperCase();
  }
  return (firstCodePoint(words[0] ?? "") + firstCodePoint(words[1] ?? "")).toUpperCase();
}

/** Deterministic HSL that keeps white text legible in both light and
 *  dark themes: hue/saturation come from the name hash, but lightness is
 *  clamped dark enough (38%) that even the yellow band (hue ≈ 60) clears
 *  contrast against the white initials. */
export function avatarColor(name: string): string {
  const hue = hashName(name) % 360;
  return `hsl(${String(hue)} 52% 38%)`;
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
  // A broken/failed profile-picture URL falls back to the initials chip.
  const [imgFailed, setImgFailed] = useState(false);
  const radius = kind === "group" ? Math.round(size * 0.28) : size / 2;
  const shared = {
    width: size,
    height: size,
    borderRadius: radius,
  } as const;

  if (src && !imgFailed) {
    return (
      <img
        src={src}
        alt={name}
        title={title ?? name}
        loading="lazy"
        onError={() => { setImgFailed(true); }}
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
