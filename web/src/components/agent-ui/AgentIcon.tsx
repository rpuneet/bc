export type AgentShape = "hexagon" | "circle" | "square";

interface AgentIconProps {
  shape?: AgentShape;
  state: string;
  size?: number;
}

const ACCENT = "var(--bc-accent, #f97316)";

function stateClass(state: string): string {
  switch (state) {
    case "working":
      return "agent-anim-working";
    case "idle":
      return "agent-anim-idle";
    case "stuck":
      return "agent-anim-stuck";
    case "error":
      return "agent-anim-error";
    case "waiting":
      return "agent-anim-waiting";
    case "stopped":
      return "agent-anim-stopped";
    default:
      return "agent-anim-idle";
  }
}

function Hexagon({ size, fill }: { size: number; fill: string }) {
  const cx = size / 2;
  const cy = size / 2;
  const r = size * 0.44;
  const points = Array.from({ length: 6 }, (_, i) => {
    const angle = (Math.PI / 3) * i - Math.PI / 2;
    return `${String(cx + r * Math.cos(angle))},${String(cy + r * Math.sin(angle))}`;
  }).join(" ");
  return (
    <svg width={size} height={size} viewBox={`0 0 ${String(size)} ${String(size)}`}>
      <polygon points={points} fill={fill} />
    </svg>
  );
}

function Circle({ size, fill }: { size: number; fill: string }) {
  const r = size * 0.42;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${String(size)} ${String(size)}`}>
      <circle cx={size / 2} cy={size / 2} r={r} fill={fill} />
    </svg>
  );
}

function Square({ size, fill }: { size: number; fill: string }) {
  const inset = size * 0.12;
  const s = size - inset * 2;
  const rx = size * 0.08;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${String(size)} ${String(size)}`}>
      <rect x={inset} y={inset} width={s} height={s} rx={rx} fill={fill} />
    </svg>
  );
}

export function AgentIcon({ shape = "hexagon", state, size = 32 }: AgentIconProps) {
  const cls = stateClass(state);
  return (
    <span className={cls} style={{ display: "inline-flex", lineHeight: 0 }}>
      {shape === "hexagon" && <Hexagon size={size} fill={ACCENT} />}
      {shape === "circle" && <Circle size={size} fill={ACCENT} />}
      {shape === "square" && <Square size={size} fill={ACCENT} />}
    </span>
  );
}
