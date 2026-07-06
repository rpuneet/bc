import React from "react";

export type AgentShape = "hexagon" | "circle" | "square";

interface AgentIconProps {
  shape?: AgentShape;
  state: string;
  size?: number;
  letter?: string;
  tool?: string;
}

const ACCENT = "var(--mycel-accent)";

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
  return <polygon points={points} fill={fill} />;
}

function Circle({ size, fill }: { size: number; fill: string }) {
  const r = size * 0.42;
  return <circle cx={size / 2} cy={size / 2} r={r} fill={fill} />;
}

function Square({ size, fill }: { size: number; fill: string }) {
  const inset = size * 0.12;
  const s = size - inset * 2;
  const rx = size * 0.08;
  return <rect x={inset} y={inset} width={s} height={s} rx={rx} fill={fill} />;
}

function providerIcon(tool: string | undefined, size: number): React.ReactNode {
  const s = size * 0.45;
  const cx = size / 2;
  const cy = size / 2;

  switch (tool?.toLowerCase()) {
    case "claude":
      return (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="central"
          fill="rgba(255,255,255,0.85)"
          fontSize={s}
          fontWeight="900"
          fontFamily="system-ui, sans-serif"
        >
          A
        </text>
      );
    case "agy":
      return (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="central"
          fill="rgba(255,255,255,0.85)"
          fontSize={s}
          fontWeight="400"
        >
          ✦
        </text>
      );
    case "cursor":
      return (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="central"
          fill="rgba(255,255,255,0.85)"
          fontSize={s * 0.85}
          fontWeight="400"
        >
          ▶
        </text>
      );
    case "codex":
      return (
        <text
          x={cx}
          y={cy}
          textAnchor="middle"
          dominantBaseline="central"
          fill="rgba(255,255,255,0.85)"
          fontSize={s * 0.85}
          fontWeight="400"
        >
          ◆
        </text>
      );
    default:
      if (tool) {
        return (
          <text
            x={cx}
            y={cy}
            textAnchor="middle"
            dominantBaseline="central"
            fill="rgba(255,255,255,0.7)"
            fontSize={s}
            fontWeight="800"
            fontFamily="system-ui, sans-serif"
          >
            {tool.charAt(0).toUpperCase()}
          </text>
        );
      }
      return null;
  }
}

export function AgentIcon({ shape = "hexagon", state, size = 32, letter, tool }: AgentIconProps) {
  const cls = stateClass(state);
  // Use tool-based provider icon when tool is provided, otherwise fall back to letter
  const icon = tool ? providerIcon(tool, size) : letter ? (
    <text
      x={size / 2}
      y={size / 2}
      textAnchor="middle"
      dominantBaseline="central"
      fill="rgba(0,0,0,0.6)"
      fontSize={size * 0.38}
      fontWeight="700"
      fontFamily="system-ui, sans-serif"
    >
      {letter}
    </text>
  ) : null;

  return (
    <span className={cls} style={{ display: "inline-flex", lineHeight: 0 }}>
      <svg width={size} height={size} viewBox={`0 0 ${String(size)} ${String(size)}`}>
        {shape === "hexagon" && <Hexagon size={size} fill={ACCENT} />}
        {shape === "circle" && <Circle size={size} fill={ACCENT} />}
        {shape === "square" && <Square size={size} fill={ACCENT} />}
        {icon}
      </svg>
    </span>
  );
}
