/* ── Agent identity derivation ──────────────────────────────────────
   Every agent gets a deterministic organic character derived from its
   name alone: same name → same creature everywhere, zero configuration.
   The hash picks a body silhouette from a small family of mycelium
   forms, a hue rotated within the mycel palette, an eye style and a few
   distinguishing surface marks. */

export type BodyForm = "spore" | "cap" | "sprout";
export type EyeStyle = "round" | "bead" | "oval";

export interface AgentMark {
  x: number;
  y: number;
  r: number;
}

export interface AgentIdentity {
  form: BodyForm;
  /** Hue within the mycel-adjacent organic palette. */
  hue: number;
  /** Saturation percentage. */
  sat: number;
  eyes: EyeStyle;
  /** 1-3 deterministic freckles/spots on the body. */
  marks: AgentMark[];
  /** Slight whole-body tilt, degrees (-4..4). */
  tilt: number;
}

/** FNV-1a 32-bit — stable, fast, good distribution for short names. */
export function hashName(name: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < name.length; i++) {
    h ^= name.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

const FORMS: BodyForm[] = ["spore", "cap", "sprout"];
const EYE_STYLES: EyeStyle[] = ["round", "bead", "oval"];

/* Hue family rotated around the mycel orange accent (~24°) plus the
   earthy/fungal neighbours: ambers, mosses, lichens, spore blues and
   mycelial violets. Chosen to sit comfortably on both Sand themes. */
const HUES = [24, 36, 14, 48, 84, 145, 168, 192, 214, 258, 288, 335];
const SATS = [46, 54, 62];

/* Body-local safe zones for surface marks, per form (64-unit viewBox).
   Kept clear of the face band so freckles never collide with eyes. */
const MARK_ZONES: Record<BodyForm, { x: number; y: number }[]> = {
  spore: [
    { x: 22, y: 22 },
    { x: 42, y: 24 },
    { x: 24, y: 46 },
    { x: 41, y: 45 },
    { x: 32, y: 20 },
  ],
  cap: [
    { x: 20, y: 20 },
    { x: 43, y: 19 },
    { x: 32, y: 15 },
    { x: 25, y: 42 },
    { x: 39, y: 42 },
  ],
  sprout: [
    { x: 24, y: 28 },
    { x: 40, y: 29 },
    { x: 26, y: 48 },
    { x: 39, y: 47 },
    { x: 32, y: 26 },
  ],
};

const IDENTITY_CACHE = new Map<string, AgentIdentity>();

export function deriveIdentity(name: string): AgentIdentity {
  const cached = IDENTITY_CACHE.get(name);
  if (cached) return cached;

  const h = hashName(name);
  const form = FORMS[h % FORMS.length]!;
  const hue = HUES[(h >>> 2) % HUES.length]!;
  const sat = SATS[(h >>> 6) % SATS.length]!;
  const eyes = EYE_STYLES[(h >>> 9) % EYE_STYLES.length]!;
  const tilt = ((h >>> 12) % 9) - 4;

  const zones = MARK_ZONES[form];
  const markCount = 1 + ((h >>> 16) % 3);
  const start = (h >>> 19) % zones.length;
  const marks: AgentMark[] = [];
  for (let i = 0; i < markCount; i++) {
    const z = zones[(start + i * 2) % zones.length]!;
    marks.push({ x: z.x, y: z.y, r: 1.1 + (((h >>> (21 + i * 2)) % 3) * 0.4) });
  }

  const identity: AgentIdentity = { form, hue, sat, eyes, marks, tilt };
  IDENTITY_CACHE.set(name, identity);
  return identity;
}

/** Body fill for an identity. Lightness rides a theme CSS var so the
 *  same hue reads on both Sand themes. */
export function bodyColor(id: AgentIdentity): string {
  return `hsl(${String(id.hue)} ${String(id.sat)}% var(--agent-body-l, 60%))`;
}

/** Deeper shade — rims, tendrils, stems, the orbiting tool mote. */
export function deepColor(id: AgentIdentity): string {
  return `hsl(${String(id.hue)} ${String(id.sat)}% var(--agent-deep-l, 42%))`;
}

/** Continuous-state → full-body animation mood class. */
export function stateAnimClass(state: string): string {
  switch (state) {
    case "working":
      return "agent-anim-working";
    case "starting":
      return "agent-anim-starting";
    case "done":
      return "agent-anim-done";
    case "stuck":
      return "agent-anim-stuck";
    case "error":
      return "agent-anim-error";
    case "waiting":
      return "agent-anim-waiting";
    case "stopped":
      return "agent-anim-stopped";
    case "idle":
    default:
      return "agent-anim-idle";
  }
}
