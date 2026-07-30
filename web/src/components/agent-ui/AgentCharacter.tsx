/* ── AgentCharacter ─────────────────────────────────────────────────
   The agent's living identity: an organic, mycelium-inspired creature
   rendered as layered inline SVG. Deterministic from the agent name
   (see identity.ts), animated by state (animations.css) and by
   transient event pulses. Pure given (name, state, size, pulse, tool)
   and memoized so SSE traffic never re-renders idle characters.

   Layering scales with size:
     ≥ 0px  silhouette + face
     ≥ 28px surface marks (freckles) + mouth
     ≥ 44px highlight sheen
     ≥ 56px provider tool-glyph chip */

import { memo } from "react";
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

/* ── Body silhouettes (64-unit viewBox, centred ~(32,36)) ─────────── */

function SporeBody({ id }: { id: AgentIdentity }) {
  return (
    <>
      {/* rounded spore with a tiny germ nub on top */}
      <path
        d="M32 12c8.5 0 15.2 5.4 17.2 13.6 1.8 7.4 0.6 15.4-3.4 20.8C41.8 51.9 37 55 32 55s-9.8-3.1-13.8-8.6c-4-5.4-5.2-13.4-3.4-20.8C16.8 17.4 23.5 12 32 12z"
        fill={bodyColor(id)}
        stroke={deepColor(id)}
        strokeWidth="2"
      />
      <circle cx="32" cy="10.5" r="2.4" fill={deepColor(id)} />
    </>
  );
}

function CapBody({ id }: { id: AgentIdentity }) {
  return (
    <>
      {/* stem */}
      <path
        d="M24.5 40h15v6.5c0 4.4-3.4 7.5-7.5 7.5s-7.5-3.1-7.5-7.5z"
        fill={bodyColor(id)}
        stroke={deepColor(id)}
        strokeWidth="2"
        style={{ filter: "brightness(1.18)" }}
      />
      {/* cap dome */}
      <path
        d="M11 34.5C11 21.5 20.4 12 32 12s21 9.5 21 22.5c0 3-2.2 5.5-5.2 5.5H16.2c-3 0-5.2-2.5-5.2-5.5z"
        fill={bodyColor(id)}
        stroke={deepColor(id)}
        strokeWidth="2"
      />
    </>
  );
}

function SproutBody({ id }: { id: AgentIdentity }) {
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
    </>
  );
}

/* ── Face ─────────────────────────────────────────────────────────── */

const EYE_Y: Record<AgentIdentity["form"], number> = {
  spore: 34,
  cap: 28,
  sprout: 36,
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

function Highlight({ id }: { id: AgentIdentity }) {
  const y = id.form === "cap" ? 20 : 24;
  return (
    <ellipse
      cx="24"
      cy={y}
      rx="6"
      ry="3.6"
      fill="#fdfaf3"
      opacity="0.18"
      transform={`rotate(-24 24 ${String(y)})`}
    />
  );
}

const TOOL_GLYPHS: Record<string, string> = {
  claude: "A",
  agy: "✦",
  cursor: "▶",
  codex: "◆",
  pi: "π",
  openclaw: "⌘",
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
          {id.form === "spore" && <SporeBody id={id} />}
          {id.form === "cap" && <CapBody id={id} />}
          {id.form === "sprout" && <SproutBody id={id} />}
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
