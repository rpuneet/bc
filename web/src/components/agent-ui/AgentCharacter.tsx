/* ── AgentCharacter ─────────────────────────────────────────────────
   The agent's living identity: an organic, mycelium-inspired creature
   rendered as layered inline SVG. Deterministic from the agent name
   (see identity.ts), animated by state (animations.css) and by
   transient event pulses. Pure given (name, state, size, pulse, tool)
   and memoized so SSE traffic never re-renders idle characters.

   Layering scales with size:
     ≥ 0px  silhouette + face
     ≥ 28px species detail + surface marks (freckles) + mouth
     ≥ 44px species texture + highlight sheen
     ≥ 56px provider tool-glyph chip */

import { memo } from "react";
import type { ReactElement } from "react";
import {
  bodyColor,
  deepColor,
  deriveIdentity,
  stateAnimClass,
} from "./identity";
import type { AgentIdentity } from "./identity";
import type { PulseKind } from "./agentEventBus";
import { useAgentPulse } from "./useAgentPulse";

export interface AgentCharacterProps {
  name: string;
  state?: string;
  size?: number;
  /** Provider tool (claude, agy, …) — glyph chip at large sizes. */
  tool?: string;
  /** Transient event pulse (from useAgentPulse). */
  pulse?: PulseKind | null;
  className?: string;
}

const INK = "var(--agent-ink, rgba(24, 20, 15, 0.82))";
/* Warm cream, not pure white — eye glints stay in the brand's paper tone. */
const PAPER = "rgba(253, 250, 243, 0.88)";

/* ── Body silhouettes (64-unit viewBox, centred ~(32,36)) ───────────
   One component per species. Every species keeps the family rules:
   2-unit deep-tone outline, bodyColor fill, silhouette reads at 16px.
   `detail` (≥28px) adds species surface texture; `texture` (≥44px)
   adds the subtlest shading strokes. */

interface BodyProps {
  id: AgentIdentity;
  detail: boolean;
  texture: boolean;
}

function SporeBody({ id, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* rounded spore with a tiny germ nub on top */}
      <path
        d="M32 12c8.5 0 15.2 5.4 17.2 13.6 1.8 7.4 0.6 15.4-3.4 20.8C41.8 51.9 37 55 32 55s-9.8-3.1-13.8-8.6c-4-5.4-5.2-13.4-3.4-20.8C16.8 17.4 23.5 12 32 12z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      <circle cx="32" cy="10.5" r="2.4" fill={deep} />
      {texture && (
        <path
          d="M24 48.5c2.6 2.2 5.4 3.3 8 3.3s5.4-1.1 8-3.3"
          stroke={deep}
          strokeWidth="1.4"
          strokeLinecap="round"
          fill="none"
          opacity="0.3"
        />
      )}
    </>
  );
}

function CapBody({ id, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* stem */}
      <path
        d="M24.5 40h15v6.5c0 4.4-3.4 7.5-7.5 7.5s-7.5-3.1-7.5-7.5z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
        style={{ filter: "brightness(1.18)" }}
      />
      {/* cap dome */}
      <path
        d="M11 34.5C11 21.5 20.4 12 32 12s21 9.5 21 22.5c0 3-2.2 5.5-5.2 5.5H16.2c-3 0-5.2-2.5-5.2-5.5z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {texture && (
        <g stroke={deep} strokeWidth="1.4" strokeLinecap="round" opacity="0.4">
          {/* gill ticks peeking from under the rim */}
          <path d="M17 42v2.2" />
          <path d="M21.5 42.5v2.6" />
          <path d="M42.5 42.5v2.6" />
          <path d="M47 42v2.2" />
        </g>
      )}
    </>
  );
}

function SproutBody({ id, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* hyphae tendrils */}
      <g stroke={deep} strokeWidth="2.2" strokeLinecap="round" fill="none">
        <path d="M25 21c-2.5-3.5-6-5.5-10.5-5.5" />
        <path d="M32 18.5V9" />
        <path d="M39 21c2.5-3.5 6-5.5 10.5-5.5" />
      </g>
      <circle cx="14" cy="15.5" r="2" fill={deep} />
      <circle cx="32" cy="8" r="2.2" fill={deep} />
      <circle cx="50" cy="15.5" r="2" fill={deep} />
      {/* body node */}
      <path
        d="M32 19c9.4 0 15.5 6.2 15.5 15.5C47.5 44.8 41.4 55 32 55s-15.5-10.2-15.5-20.5C16.5 25.2 22.6 19 32 19z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {texture && (
        <path
          d="M25 49.5c2.2 1.9 4.6 2.9 7 2.9s4.8-1 7-2.9"
          stroke={deep}
          strokeWidth="1.4"
          strokeLinecap="round"
          fill="none"
          opacity="0.3"
        />
      )}
    </>
  );
}

function MorelBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* stub stem */}
      <path
        d="M25 44h14v4c0 4-3.1 6.5-7 6.5s-7-2.5-7-6.5z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
        style={{ filter: "brightness(1.18)" }}
      />
      {/* tall wrinkled cone */}
      <path
        d="M32 8c5.8 0 10.3 4.6 11.8 11.4l2.6 12.4c1.3 6-1.6 12.2-7 12.2H24.6c-5.4 0-8.3-6.2-7-12.2l2.6-12.4C21.7 12.6 26.2 8 32 8z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {detail && (
        <g stroke={deep} strokeWidth="1.3" fill="none" opacity="0.5">
          {/* honeycomb pits */}
          <ellipse cx="28" cy="16.5" rx="1.7" ry="2.4" />
          <ellipse cx="36" cy="16.5" rx="1.7" ry="2.4" />
          <ellipse cx="32" cy="23" rx="1.9" ry="2.6" />
          <ellipse cx="24.5" cy="24" rx="1.6" ry="2.2" />
          <ellipse cx="39.5" cy="24" rx="1.6" ry="2.2" />
        </g>
      )}
      {texture && (
        <g stroke={deep} strokeWidth="1.2" strokeLinecap="round" fill="none" opacity="0.35">
          <path d="M26.5 12.5c-1.8 5.4-2.8 10.8-3 16" />
          <path d="M37.5 12.5c1.8 5.4 2.8 10.8 3 16" />
        </g>
      )}
    </>
  );
}

function PuffballBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* burst speckles escaping the spout — part of the silhouette */}
      <g fill={deep}>
        <circle cx="32" cy="19.5" r="1.7" />
        <circle cx="26.5" cy="21.5" r="1.2" />
        <circle cx="37.5" cy="21.5" r="1.2" />
      </g>
      {/* low round puff, wider than tall */}
      <path
        d="M32 24c9.6 0 16.5 6.3 16.5 15 0 8.4-6.9 14.5-16.5 14.5S15.5 47.4 15.5 39c0-8.7 6.9-15 16.5-15z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {detail && (
        <g fill={deep} opacity="0.5">
          {/* fine warts across the crown */}
          <circle cx="24" cy="30.5" r="1" />
          <circle cx="32" cy="28.5" r="1" />
          <circle cx="40" cy="30.5" r="1" />
          <circle cx="28" cy="33" r="0.8" />
          <circle cx="36" cy="33" r="0.8" />
        </g>
      )}
      {texture && (
        <g stroke={deep} strokeWidth="1.4" strokeLinecap="round" fill="none" opacity="0.35">
          {/* root tufts */}
          <path d="M25.5 52.5c-1 1.8-2.4 2.9-4.2 3.3" />
          <path d="M38.5 52.5c1 1.8 2.4 2.9 4.2 3.3" />
        </g>
      )}
    </>
  );
}

function ChanterelleBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* trumpet: wavy flared rim narrowing to the foot */}
      <path
        d="M14.5 15c5 3.4 11 5.2 17.5 5.2S44.5 18.4 49.5 15c1.8 4.4.9 9.7-2.6 14.8-2.7 4-4.4 8.8-4.9 14.2-.4 4.9-4.6 8.5-10 8.5s-9.6-3.6-10-8.5c-.5-5.4-2.2-10.2-4.9-14.2C13.6 24.7 12.7 19.4 14.5 15z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {detail && (
        <g stroke={deep} strokeWidth="1.3" strokeLinecap="round" fill="none" opacity="0.45">
          {/* false gills running down the flanks */}
          <path d="M21.5 22c-.6 5.2-2 9.6-3.8 13.2" />
          <path d="M42.5 22c.6 5.2 2 9.6 3.8 13.2" />
        </g>
      )}
      {texture && (
        <path
          d="M27 21.5c1.6.5 3.3.7 5 .7s3.4-.2 5-.7"
          stroke={deep}
          strokeWidth="1.2"
          strokeLinecap="round"
          fill="none"
          opacity="0.35"
        />
      )}
    </>
  );
}

function BracketBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  const shelf = {
    fill: bodyColor(id),
    stroke: deep,
    strokeWidth: 2,
  };
  return (
    <>
      {/* three stacked shelf ledges, widest at the bottom */}
      <path d="M14 38h36v1.6c0 7.8-8 13.4-18 13.4s-18-5.6-18-13.4z" {...shelf} />
      <path
        d="M17 27h30v1.3c0 5.8-6.7 9.7-15 9.7s-15-3.9-15-9.7z"
        {...shelf}
        style={{ filter: "brightness(1.1)" }}
      />
      <path
        d="M22 17h20v1.1c0 4.8-4.5 8.4-10 8.4s-10-3.6-10-8.4z"
        {...shelf}
        style={{ filter: "brightness(1.18)" }}
      />
      {detail && (
        <g stroke={deep} strokeWidth="1.3" strokeLinecap="round" fill="none" opacity="0.45">
          {/* shelf lip shading */}
          <path d="M20 39.5c1.8.8 3.8 1.3 6 1.6" />
          <path d="M22.5 28.5c1.5.7 3.2 1.1 5 1.4" />
        </g>
      )}
      {texture && (
        <g stroke={deep} strokeWidth="1.2" strokeLinecap="round" fill="none" opacity="0.3">
          {/* growth rings on the big shelf */}
          <path d="M18 44c1 2.6 3 4.7 5.6 6.2" />
          <path d="M46 44c-1 2.6-3 4.7-5.6 6.2" />
        </g>
      )}
    </>
  );
}

function CoralBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  const antlers = [
    "M24.5 34c-1.6-4.2-1.8-8-.6-11.8",
    "M23.2 27.5c-2.4-1.6-4-3.8-4.8-6.6",
    "M32 32V21.5",
    "M32 25.5c2-1.4 3.2-3.2 3.6-5.6",
    "M32 25.5c-2-1.4-3.2-3.2-3.6-5.6",
    "M39.5 34c1.6-4.2 1.8-8 .6-11.8",
    "M40.8 27.5c2.4-1.6 4-3.8 4.8-6.6",
  ];
  return (
    <>
      {/* thick forked antler branches: deep outline under a body-tone core */}
      <g stroke={deep} strokeWidth="5.4" strokeLinecap="round" fill="none">
        {antlers.map((d) => (
          <path key={d} d={d} />
        ))}
      </g>
      <g stroke={bodyColor(id)} strokeWidth="2.6" strokeLinecap="round" fill="none">
        {antlers.map((d) => (
          <path key={d} d={d} />
        ))}
      </g>
      {/* squat mound base */}
      <path
        d="M32 30c9.8 0 16.5 4.6 16.5 12 0 7-6.7 12.5-16.5 12.5S15.5 49 15.5 42c0-7.4 6.7-12 16.5-12z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {detail && (
        <g fill={PAPER} opacity="0.6">
          {/* pale branch tips */}
          <circle cx="23.9" cy="22.2" r="1" />
          <circle cx="18.4" cy="20.9" r="0.9" />
          <circle cx="32" cy="21.5" r="1" />
          <circle cx="35.6" cy="19.9" r="0.9" />
          <circle cx="28.4" cy="19.9" r="0.9" />
          <circle cx="40.1" cy="22.2" r="1" />
          <circle cx="45.6" cy="20.9" r="0.9" />
        </g>
      )}
      {texture && (
        <path
          d="M24 50.5c2.4 1.6 5.2 2.5 8 2.5s5.6-.9 8-2.5"
          stroke={deep}
          strokeWidth="1.4"
          strokeLinecap="round"
          fill="none"
          opacity="0.3"
        />
      )}
    </>
  );
}

function EnokiBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  const stems = ["M32 39V17.5", "M25 40c-.8-5.5-1.2-10-1-14", "M39 40c.8-5.5 1.2-10 1-14"];
  return (
    <>
      {/* three thin stems: deep outline under a body-tone core */}
      <g stroke={deep} strokeWidth="4.6" strokeLinecap="round" fill="none">
        {stems.map((d) => (
          <path key={d} d={d} />
        ))}
      </g>
      <g stroke={bodyColor(id)} strokeWidth="2" strokeLinecap="round" fill="none">
        {stems.map((d) => (
          <path key={d} d={d} />
        ))}
      </g>
      {/* tiny caps — the cluster reads as a many-headed critter */}
      <circle cx="32" cy="14.5" r="4.6" fill={bodyColor(id)} stroke={deep} strokeWidth="2" />
      <circle cx="23.8" cy="22" r="3.7" fill={bodyColor(id)} stroke={deep} strokeWidth="2" />
      <circle cx="40.2" cy="22" r="3.7" fill={bodyColor(id)} stroke={deep} strokeWidth="2" />
      {/* shared base clump */}
      <path
        d="M32 36c9.2 0 15.5 4 15.5 9.6 0 5.6-6.3 9.4-15.5 9.4s-15.5-3.8-15.5-9.4c0-5.6 6.3-9.6 15.5-9.6z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
      />
      {detail && (
        <g fill={INK} opacity="0.65">
          {/* cap eye-dots — the multi-eyed look */}
          <circle cx="32" cy="14.5" r="1.2" />
          <circle cx="23.8" cy="22" r="1" />
          <circle cx="40.2" cy="22" r="1" />
        </g>
      )}
      {texture && (
        <path
          d="M25 52.5c2.2 1.5 4.6 2.3 7 2.3s4.8-.8 7-2.3"
          stroke={deep}
          strokeWidth="1.4"
          strokeLinecap="round"
          fill="none"
          opacity="0.3"
        />
      )}
    </>
  );
}

function LichenBody({ id, detail, texture }: BodyProps) {
  const deep = deepColor(id);
  return (
    <>
      {/* eight-lobed rosette disc */}
      <path
        d="M46.5 36Q51.4 44 42.25 46.25Q40 55.4 32 50.5Q24 55.4 21.75 46.25Q12.6 44 17.5 36Q12.6 28 21.75 25.75Q24 16.6 32 21.5Q40 16.6 42.25 25.75Q51.4 28 46.5 36Z"
        fill={bodyColor(id)}
        stroke={deep}
        strokeWidth="2"
        strokeLinejoin="round"
      />
      {detail && (
        <path
          d="M25 26.5c4.2-2.6 9.8-2.6 14 0"
          stroke={deep}
          strokeWidth="1.3"
          strokeLinecap="round"
          fill="none"
          opacity="0.45"
        />
      )}
      {texture && (
        <path
          d="M23.5 46.5c5.2 3 11.8 3 17 0"
          stroke={deep}
          strokeWidth="1.2"
          strokeLinecap="round"
          fill="none"
          opacity="0.3"
        />
      )}
    </>
  );
}

const BODIES: Record<AgentIdentity["form"], (props: BodyProps) => ReactElement> = {
  spore: SporeBody,
  cap: CapBody,
  sprout: SproutBody,
  morel: MorelBody,
  puffball: PuffballBody,
  chanterelle: ChanterelleBody,
  bracket: BracketBody,
  coral: CoralBody,
  enoki: EnokiBody,
  lichen: LichenBody,
};

/* ── Face ─────────────────────────────────────────────────────────── */

/* Face band centre per species — eyes sit here, mouth 7 units below.
   Every silhouette keeps this band clear of structural strokes. */
const EYE_Y: Record<AgentIdentity["form"], number> = {
  spore: 34,
  cap: 28,
  sprout: 36,
  morel: 33,
  puffball: 38,
  chanterelle: 32,
  bracket: 44,
  coral: 41,
  enoki: 44,
  lichen: 34,
};

function Eyes({ id, closed }: { id: AgentIdentity; closed: boolean }) {
  const y = EYE_Y[id.form];
  if (closed) {
    return (
      <g
        className="agent-eyes"
        stroke={INK}
        strokeWidth="2"
        strokeLinecap="round"
        fill="none"
      >
        <path d={`M23 ${String(y)}q3 2.6 6 0`} />
        <path d={`M35 ${String(y)}q3 2.6 6 0`} />
      </g>
    );
  }
  if (id.eyes === "oval") {
    return (
      <g className="agent-eyes" fill={INK}>
        <ellipse cx="26" cy={y} rx="2.6" ry="3.7" />
        <ellipse cx="38" cy={y} rx="2.6" ry="3.7" />
        <circle cx="26.9" cy={y - 1.2} r="0.9" fill={PAPER} />
        <circle cx="38.9" cy={y - 1.2} r="0.9" fill={PAPER} />
      </g>
    );
  }
  const r = id.eyes === "round" ? 3.2 : 2.2;
  return (
    <g className="agent-eyes" fill={INK}>
      <circle cx="26" cy={y} r={r} />
      <circle cx="38" cy={y} r={r} />
      {id.eyes === "round" && (
        <>
          <circle cx="27" cy={y - 1.1} r="1" fill={PAPER} />
          <circle cx="39" cy={y - 1.1} r="1" fill={PAPER} />
        </>
      )}
    </g>
  );
}

function Mouth({ id, state }: { id: AgentIdentity; state: string }) {
  const y = EYE_Y[id.form] + 7;
  let d: string;
  if (state === "error") {
    d = `M29 ${String(y + 1)}q3 -2.4 6 0`; // small frown
  } else if (state === "stuck") {
    d = `M29.5 ${String(y)}h5`; // flat line
  } else if (state === "done" || state === "working") {
    d = `M28.5 ${String(y - 1)}q3.5 3 7 0`; // content smile
  } else {
    d = `M30 ${String(y - 0.5)}q2 1.8 4 0`; // faint neutral curve
  }
  return (
    <path
      d={d}
      stroke={INK}
      strokeWidth="1.7"
      strokeLinecap="round"
      fill="none"
      opacity="0.75"
    />
  );
}

/* ── Detail layers ────────────────────────────────────────────────── */

function Marks({ id }: { id: AgentIdentity }) {
  return (
    <g fill={deepColor(id)} opacity="0.55">
      {id.marks.map((m, i) => (
        <circle key={i} cx={m.x} cy={m.y} r={m.r} />
      ))}
    </g>
  );
}

/* Sheen position per species — upper-left of the main body mass. */
const HIGHLIGHT: Record<AgentIdentity["form"], { x: number; y: number }> = {
  spore: { x: 24, y: 24 },
  cap: { x: 24, y: 20 },
  sprout: { x: 25, y: 26 },
  morel: { x: 27, y: 18 },
  puffball: { x: 25, y: 30 },
  chanterelle: { x: 24, y: 22 },
  bracket: { x: 24, y: 43 },
  coral: { x: 25, y: 36 },
  enoki: { x: 25, y: 41 },
  lichen: { x: 24, y: 27 },
};

function Highlight({ id }: { id: AgentIdentity }) {
  const { x, y } = HIGHLIGHT[id.form];
  return (
    <ellipse
      cx={x}
      cy={y}
      rx="6"
      ry="3.6"
      fill="#fdfaf3"
      opacity="0.18"
      transform={`rotate(-24 ${String(x)} ${String(y)})`}
    />
  );
}

const TOOL_GLYPHS: Record<string, string> = {
  claude: "A",
  agy: "✦",
  cursor: "▶",
  codex: "◆",
  pi: "π",
};

function ToolChip({ id, tool }: { id: AgentIdentity; tool: string }) {
  const glyph = TOOL_GLYPHS[tool.toLowerCase()] ?? tool.charAt(0).toUpperCase();
  return (
    <g>
      <circle
        cx="50"
        cy="50"
        r="8"
        fill="var(--mycel-surface-2, #222)"
        stroke={deepColor(id)}
        strokeWidth="1.5"
      />
      <text
        x="50"
        y="50.6"
        textAnchor="middle"
        dominantBaseline="central"
        fill="var(--mycel-text-2, #b5b3ad)"
        fontSize="8.5"
        fontWeight="700"
        fontFamily="'Geist Mono', ui-monospace, monospace"
      >
        {glyph}
      </text>
    </g>
  );
}

/* ── The character ────────────────────────────────────────────────── */

function AgentCharacterInner({
  name,
  state = "idle",
  size = 32,
  tool,
  pulse = null,
  className = "",
}: AgentCharacterProps) {
  const id = deriveIdentity(name);
  const Body = BODIES[id.form];
  const anim = stateAnimClass(state);
  const pulseClass = pulse ? ` agent-pulse-${pulse}` : "";
  const eyesClosed = state === "stopped";
  const showOrbit = state === "working" || pulse === "tool";

  return (
    <span
      className={`agent-character ${anim}${pulseClass} ${className}`.trim()}
      style={{ display: "inline-flex", lineHeight: 0 }}
      role="img"
      aria-label={`${name} — ${state}`}
      data-agent-form={id.form}
    >
      <svg
        width={size}
        height={size}
        viewBox="0 0 64 64"
        style={{ overflow: "visible" }}
        aria-hidden
      >
        <g className="agent-body" transform={`rotate(${String(id.tilt)} 32 34)`}>
          <Body id={id} detail={size >= 28} texture={size >= 44} />
          {size >= 44 && <Highlight id={id} />}
          {size >= 28 && <Marks id={id} />}
          <Eyes id={id} closed={eyesClosed} />
          {size >= 28 && !eyesClosed && <Mouth id={id} state={state} />}
        </g>
        {/* Orbiting tool mote — working state loop, or a one-shot burst
            on tool pulses. Spins around the body via CSS. */}
        {showOrbit && (
          <g className="agent-orbit">
            <circle cx="32" cy="7" r="2.6" fill={deepColor(id)} />
            <circle cx="32" cy="7" r="1" fill={PAPER} opacity="0.8" />
          </g>
        )}
        {/* Speech dots — flicker above the head when a message arrives. */}
        {pulse === "message" && (
          <g className="agent-speech" fill={deepColor(id)}>
            <circle cx="46" cy="12" r="1.8" />
            <circle cx="52" cy="9" r="2.2" />
            <circle cx="58" cy="5" r="2.6" />
          </g>
        )}
        {size >= 56 && tool && <ToolChip id={id} tool={tool} />}
      </svg>
    </span>
  );
}

/** Pure, memoized character. Same name → same creature, everywhere. */
export const AgentCharacter = memo(AgentCharacterInner);

/** Drop-in live character: wires the shared SSE pulse bus so the
 *  creature reacts to messages, tool activity and state changes.
 *  Memoized per name — bus events only re-render the mentioned agent. */
export const LiveAgentCharacter = memo(function LiveAgentCharacter(
  props: Omit<AgentCharacterProps, "pulse">,
) {
  const pulse = useAgentPulse(props.name);
  return <AgentCharacter {...props} pulse={pulse} />;
});
