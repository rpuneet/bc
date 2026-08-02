import { useId } from "react";

/* ── ProviderLogo ─────────────────────────────────────────────────────
 *
 * A recognizable brand mark for each AI provider, rendered as inline SVG
 * (a strict CSP blocks external assets — no <img src=url>, no icon CDNs).
 * Nominative use: the marks identify which integration a row is for.
 *
 * Brand marks are drawn in each brand's own accent color where that reads
 * cleanly on both the dark (espresso) and light (porcelain) mycel grounds;
 * monochrome brands (OpenAI, Cursor) inherit the theme's text color via
 * currentColor so they stay legible in both. Anything we don't have a mark
 * for falls back to a polished monogram tile in the mycel accent family.
 *
 * Sizes are driven off a single `size` prop so the same component scales
 * from a 20px list row to a 44px detail hero without redefining geometry.
 */

interface Props {
  name: string;
  /** Tile edge length in px. The mark inside scales to ~60% of this. */
  size?: number;
  className?: string;
}

/** Fold brand aliases onto the canonical provider key used for lookup. */
function canonical(name: string): string {
  const n = name.toLowerCase().trim();
  if (n === "anthropic") return "claude";
  if (n === "openai" || n === "gpt") return "codex";
  if (n === "google" || n === "gemini-cli") return "gemini";
  if (n === "antigravity") return "agy";
  return n;
}

/* Which providers have a real vector mark (vs. the monogram fallback). */
const KNOWN = new Set(["claude", "codex", "cursor", "gemini", "agy", "aider", "openclaw", "pi"]);

export function hasProviderMark(name: string): boolean {
  return KNOWN.has(canonical(name));
}

/* Each mark is authored on a 24×24 grid and scaled by the wrapper. */
function Mark({ id, kind, s }: { id: string; kind: string; s: number }) {
  const common = { width: s, height: s, viewBox: "0 0 24 24", fill: "none", "aria-hidden": true, focusable: "false" as const };

  switch (kind) {
    /* Anthropic — the sunburst, in Claude's clay orange (reads on both grounds). */
    case "claude":
      return (
        <svg {...common}>
          <g fill="#D97757">
            {Array.from({ length: 12 }).map((_, i) => {
              const a = (i * 30 * Math.PI) / 180;
              const r0 = 3.2;
              const r1 = i % 2 === 0 ? 10.5 : 8.4;
              const cx = 12;
              const cy = 12;
              return (
                <rect
                  key={i}
                  x={cx - 0.85}
                  y={cy - r1}
                  width={1.7}
                  height={r1 - r0}
                  rx={0.85}
                  transform={`rotate(${(a * 180) / Math.PI} ${cx} ${cy})`}
                />
              );
            })}
          </g>
        </svg>
      );

    /* OpenAI — a six-petal blossom knot, monochrome (inherits theme text). */
    case "codex":
      return (
        <svg {...common} stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round">
          <g>
            {Array.from({ length: 6 }).map((_, i) => (
              <ellipse
                key={i}
                cx={12}
                cy={12}
                rx={3.1}
                ry={7.2}
                transform={`rotate(${i * 60} 12 12)`}
                opacity={0.9}
              />
            ))}
          </g>
        </svg>
      );

    /* Cursor — a layered pointer, monochrome (inherits theme text). */
    case "cursor":
      return (
        <svg {...common}>
          <path
            d="M5 3.2 19 11l-6.3 1.3L9.4 18 5 3.2Z"
            fill="currentColor"
            opacity={0.9}
            stroke="currentColor"
            strokeWidth={0.6}
            strokeLinejoin="round"
          />
        </svg>
      );

    /* Google Gemini — the four-point spark, blue→purple gradient. */
    case "gemini":
      return (
        <svg {...common}>
          <defs>
            <linearGradient id={`${id}-gem`} x1="2" y1="3" x2="22" y2="21" gradientUnits="userSpaceOnUse">
              <stop offset="0" stopColor="#4285F4" />
              <stop offset="0.5" stopColor="#7C6BD6" />
              <stop offset="1" stopColor="#9B72CB" />
            </linearGradient>
          </defs>
          <path
            d="M12 2c.6 4.9 4.1 8.4 9 9-4.9.6-8.4 4.1-9 9-.6-4.9-4.1-8.4-9-9 4.9-.6 8.4-4.1 9-9Z"
            fill={`url(#${id}-gem)`}
          />
        </svg>
      );

    /* Antigravity — a rising orbit: ring + upward arrow. */
    case "agy":
      return (
        <svg {...common} stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round">
          <circle cx={12} cy={12} r={8.4} opacity={0.35} />
          <path d="M12 16.5V7.5M8.4 11 12 7.4 15.6 11" />
        </svg>
      );

    /* aider — a CLI prompt mark (">_"), fitting a terminal pair-programmer. */
    case "aider":
      return (
        <svg {...common} stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
          <path d="M5.5 7.5 10 12l-4.5 4.5" />
          <path d="M12.5 16.5h6" opacity={0.75} />
        </svg>
      );

    /* OpenClaw — three talon strokes. */
    case "openclaw":
      return (
        <svg {...common} stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round">
          <path d="M7 4.5c-1.6 3.2-2 6.8-1 10.5M12 4c-.9 3.6-.9 7.4 0 11M17 4.5c1.6 3.2 2 6.8 1 10.5" opacity={0.9} />
          <path d="M6.4 15.5c1.4 2.4 3.4 3.8 5.6 3.8s4.2-1.4 5.6-3.8" />
        </svg>
      );

    /* Pi — the π glyph, in the mycel amber accent. */
    case "pi":
      return (
        <svg {...common} stroke="var(--mycel-accent)" strokeWidth={1.9} strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 8h14" />
          <path d="M9 8v8.5" />
          <path d="M16 8v6.5c0 1.2.5 2 1.6 2" />
        </svg>
      );

    default:
      return null;
  }
}

export function ProviderLogo({ name, size = 24, className = "" }: Props) {
  const id = useId();
  const key = canonical(name);
  const markSize = Math.round(size * 0.62);
  const radius = Math.max(6, Math.round(size * 0.28));

  if (!KNOWN.has(key)) {
    // Polished monogram — brand-neutral rounded square in the accent family.
    return (
      <span
        role="img"
        aria-label={`${name} logo`}
        className={`inline-flex shrink-0 items-center justify-center bg-mycel-accent-subtle text-mycel-accent font-semibold leading-none ${className}`}
        style={{ width: size, height: size, borderRadius: radius, fontSize: Math.round(size * 0.42) }}
      >
        {name.charAt(0).toUpperCase()}
      </span>
    );
  }

  return (
    <span
      role="img"
      aria-label={`${name} logo`}
      className={`inline-flex shrink-0 items-center justify-center bg-mycel-surface border border-mycel-border text-mycel-text ${className}`}
      style={{ width: size, height: size, borderRadius: radius }}
    >
      <Mark id={id} kind={key} s={markSize} />
    </span>
  );
}
