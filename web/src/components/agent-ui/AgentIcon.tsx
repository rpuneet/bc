import { AVATAR_COLORS } from "./utils/colorFromName";

interface AgentIconProps {
  name: string;
  variant: "geometric" | "organic" | "monogram";
  color: number;
  state: string;
  size?: number;
}

function stateClass(state: string): string {
  switch (state) {
    case "stopped":
      return "agent-anim-stopped";
    case "error":
      return "agent-anim-error";
    case "stuck":
      return "agent-anim-stuck";
    case "waiting":
      return "agent-anim-waiting";
    case "working":
      return "agent-anim-working";
    case "idle":
      return "agent-anim-idle";
    default:
      return "agent-anim-idle";
  }
}

function HexagonIcon({ fill, size }: { fill: string; size: number }) {
  const cx = size / 2;
  const cy = size / 2;
  const r = size * 0.42;
  const points = Array.from({ length: 6 }, (_, i) => {
    const angle = (Math.PI / 3) * i - Math.PI / 2;
    return `${cx + r * Math.cos(angle)},${cy + r * Math.sin(angle)}`;
  }).join(" ");

  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      <polygon points={points} fill={fill} />
    </svg>
  );
}

function BlobIcon({ fill, size }: { fill: string; size: number }) {
  const s = size;
  const r = s * 0.38;
  return (
    <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
      <circle cx={s / 2} cy={s / 2} r={r} fill={fill} opacity={0.85} />
      <circle cx={s / 2} cy={s / 2} r={r * 0.75} fill={fill} opacity={0.5} />
    </svg>
  );
}

function MonogramIcon({
  letter,
  fill,
  size,
}: {
  letter: string;
  fill: string;
  size: number;
}) {
  const r = size * 0.42;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
      <circle cx={size / 2} cy={size / 2} r={r} fill={fill} />
      <text
        x={size / 2}
        y={size / 2}
        textAnchor="middle"
        dominantBaseline="central"
        fill="white"
        fontSize={size * 0.4}
        fontWeight="bold"
        fontFamily="system-ui, sans-serif"
      >
        {letter}
      </text>
    </svg>
  );
}

export function AgentIcon({
  name,
  variant,
  color,
  state,
  size = 32,
}: AgentIconProps) {
  const fill = AVATAR_COLORS[color % AVATAR_COLORS.length] ?? AVATAR_COLORS[0]!;
  const cls = stateClass(state);
  const letter = name.charAt(0).toUpperCase();

  return (
    <span className={cls} style={{ display: "inline-flex", lineHeight: 0 }}>
      {variant === "geometric" && <HexagonIcon fill={fill} size={size} />}
      {variant === "organic" && <BlobIcon fill={fill} size={size} />}
      {variant === "monogram" && (
        <MonogramIcon letter={letter} fill={fill} size={size} />
      )}
    </span>
  );
}
